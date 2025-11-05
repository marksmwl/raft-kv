package raft

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"time"
)

type RequestVoteArgs struct {
	Term        int
	CandidateId int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type LogEntry struct {
	Term    int
	Index   int
	Command any // KV op
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry // empty for heartbeat
	LeaderCommit int
}
type AppendEntriesReply struct {
	Term    int
	Success bool
	// Optimization fields:
	ConflictTerm  int
	ConflictIndex int
}

func call(addr string, method string, args any, reply any) error {
	c, err := rpc.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Call(method, args, reply)
}

func (rf *Raft) ServeRPC(listenAddr string) error {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	srv := rpc.NewServer() // per node server

	if err := srv.RegisterName("Raft", rf); err != nil {
		return fmt.Errorf("failed to register RPC: %v", err)
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	log.Printf("[Node %d] Listening on %s", rf.id, listenAddr)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("[Node %d] Failed to accept connection: %v", rf.id, err)
				continue
			}
			go srv.ServeConn(conn)
		}
	}()
	return nil
}

// RPC Methods
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) error {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	reply.Term = rf.currentTerm
	reply.VoteGranted = false

	// If candidate's term is less than current term, reject
	if args.Term < rf.currentTerm {
		return nil
	}

	// If candidate's term is greater, update term and become follower
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.state = Follower
		rf.votedFor = -1
		reply.Term = rf.currentTerm
	}

	// Grant vote if haven't voted for anyone else this term
	if rf.votedFor == -1 || rf.votedFor == args.CandidateId {
		rf.votedFor = args.CandidateId
		rf.lastHeartbeatTime = time.Now()
		reply.VoteGranted = true
		fmt.Printf("[Node %d] Voted for candidate %d in term %d\n", rf.id, args.CandidateId, args.Term)
	}

	return nil
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) error {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	reply.Term = rf.currentTerm
	reply.Success = false

	// If leader's term is less than current term, reject
	if args.Term < rf.currentTerm {
		return nil
	}

	// If leader's term is greater, update term and become follower
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.state = Follower
		rf.votedFor = -1
		reply.Term = rf.currentTerm
	}

	// Reset election timer since we received a heartbeat from the leader
	rf.lastHeartbeatTime = time.Now()

	// If this is a heartbeat (empty entries), accept it
	if len(args.Entries) == 0 {
		reply.Success = true
		return nil
	}

	// TODO: Handle actual log entries in future implementation
	reply.Success = true
	return nil
}
