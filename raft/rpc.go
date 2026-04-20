package raft

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/marksmwl/raft-kv/proto"
	"google.golang.org/grpc"
)

type LogEntry struct {
	Term    int
	Index   int
	Command any // KV op
}



func (rf *Raft) ServeRPC(listenAddr string) error {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	proto.RegisterRaftServer(s, rf)

	log.Printf("[Node %d] Listening on %s", rf.id, listenAddr)

	go func() {
		if err := s.Serve(ln); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return nil
}

// RPC Methods
func (rf *Raft) RequestVote(_ context.Context, req *proto.RequestVoteRequest) (*proto.RequestVoteResponse, error) {
	if rf.killed() {
		return nil, fmt.Errorf("node is dead")
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()

	reply := &proto.RequestVoteResponse{
		Term:        int64(rf.currentTerm),
		VoteGranted: false,
	}

	// If candidate's term is less than current term, reject
	if req.Term < int64(rf.currentTerm) {
		return reply, nil
	}

	// If candidate's term is greater, update term and become follower
	if req.Term > int64(rf.currentTerm) {
		rf.currentTerm = int(req.Term)
		rf.state = Follower
		rf.votedFor = -1
		reply.Term = int64(rf.currentTerm)
	}

	// Grant vote if haven't voted for anyone else this term
	if rf.votedFor == -1 || rf.votedFor == int(req.CandidateId) {
		rf.votedFor = int(req.CandidateId)
		rf.lastHeartbeatTime = time.Now()
		reply.VoteGranted = true
		fmt.Printf("[Node %d] Voted for candidate %d in term %d\n", rf.id, req.CandidateId, req.Term)
	}

	return reply, nil
}

func (rf *Raft) AppendEntries(_ context.Context, req *proto.AppendEntriesRequest) (*proto.AppendEntriesResponse, error) {
	if rf.killed() {
		return nil, fmt.Errorf("node is dead")
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()

	reply := &proto.AppendEntriesResponse{
		Term:    int64(rf.currentTerm),
		Success: false,
	}

	// If leader's term is less than current term, reject
	if req.Term < int64(rf.currentTerm) {
		return reply, nil
	}

	// If leader's term is greater, update term and become follower
	if req.Term > int64(rf.currentTerm) {
		rf.currentTerm = int(req.Term)
		rf.state = Follower
		rf.votedFor = -1
		reply.Term = int64(rf.currentTerm)
	} else if req.Term == int64(rf.currentTerm) && rf.state == Candidate {
		rf.state = Follower
	}

	// Reset election timer since we received a heartbeat from the leader
	rf.lastHeartbeatTime = time.Now()

	// If this is a heartbeat (empty entries), accept it
	if len(req.Entries) == 0 {
		// Update commit index if leader's commit index is higher
		if req.LeaderCommit > int64(rf.commitIndex) {
			if req.LeaderCommit < int64(len(rf.log)) {
				rf.commitIndex = int(req.LeaderCommit)
			} else {
				rf.commitIndex = len(rf.log)
			}
		}
		reply.Success = true
		return reply, nil
	}

	// Check if previous log entry matches
	if req.PrevLogIndex > 0 {
		if req.PrevLogIndex > int64(len(rf.log)) {
			// Previous log entry doesn't exist
			reply.Success = false
			return reply, nil
		}
		if rf.log[req.PrevLogIndex-1].Term != int(req.PrevLogTerm) {
			// Previous log entry term doesn't match
			reply.Success = false
			return reply, nil
		}
	}

	// Append new entries
	// If there are conflicting entries, delete them and append new ones
	if req.PrevLogIndex+int64(len(req.Entries)) <= int64(len(rf.log)) {
		// Check if entries already match
		match := true
		for i, entry := range req.Entries {
			if req.PrevLogIndex+int64(i) >= int64(len(rf.log)) || rf.log[req.PrevLogIndex+int64(i)].Term != int(entry.Term) {
				match = false
				break
			}
		}
		if match {
			reply.Success = true
			// Update commit index
			if req.LeaderCommit > int64(rf.commitIndex) {
				if req.LeaderCommit < int64(len(rf.log)) {
					rf.commitIndex = int(req.LeaderCommit)
				} else {
					rf.commitIndex = len(rf.log)
				}
			}
			return reply, nil
		}
	}

	// Truncate log if necessary and append new entries
	if req.PrevLogIndex < int64(len(rf.log)) {
		rf.log = rf.log[:req.PrevLogIndex]
	}

	// Append new entries
	for _, entry := range req.Entries {
		var cmd interface{}
		if len(entry.Command) > 0 {
			json.Unmarshal(entry.Command, &cmd)
		}
		newEntry := LogEntry{
			Term:    int(entry.Term),
			Index:   len(rf.log) + 1,
			Command: cmd,
		}
		rf.log = append(rf.log, newEntry)
		fmt.Printf("[Node %d] Appended entry at index %d from leader %d\n", rf.id, newEntry.Index, req.LeaderId)
	}

	// Update commit index
	if req.LeaderCommit > int64(rf.commitIndex) {
		if req.LeaderCommit < int64(len(rf.log)) {
			rf.commitIndex = int(req.LeaderCommit)
		} else {
			rf.commitIndex = len(rf.log)
		}
	}

	reply.Success = true
	return reply, nil
}
