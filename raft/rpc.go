package raft

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
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

func (rf *Raft) ServeRPC(listenAddr string) error {
	if err := rpc.RegisterName("Raft", rf); err != nil {
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
			go rpc.ServeConn(conn)
		}
	}()
	return nil
}

func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) error {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return nil
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) error {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return nil
}
