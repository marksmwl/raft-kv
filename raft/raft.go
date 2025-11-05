package raft

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type NodeState int

const (
	Follower NodeState = iota
	Leader
	Candidate
)

func (s NodeState) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Leader:
		return "Leader"
	case Candidate:
		return "Candidate"
	}
	return "Unknown"
}

type Raft struct {
	mu sync.Mutex

	peers []string
	id    int
	state NodeState

	currentTerm int
	votedFor    int

	lastHeartbeatTime time.Time
}

func New(id int, peers []string) *Raft {
	rf := &Raft{
		id:                id,
		peers:             peers,
		state:             Follower,
		currentTerm:       0,
		votedFor:          -1,
		lastHeartbeatTime: time.Now(),
	}

	fmt.Println("Node", id, "is now a", rf.state)
	return rf
}

func (rf *Raft) Start() {
	for {
		time.Sleep(5 * time.Millisecond) // 1 second heartbeat
		rf.mu.Lock()

		elapsedTime := time.Since(rf.lastHeartbeatTime)

		if rf.state == Follower && elapsedTime > rf.getElectionTimeout() {
			fmt.Printf("[Node %d] Haven't heard from leader in %v, should start election...\n",
				rf.id, elapsedTime)
			rf.startElection()
		}
		rf.mu.Unlock()
	}
}

// getElectionTimeout returns a random timeout between 150-300ms
func (rf *Raft) getElectionTimeout() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

func (rf *Raft) startElection() {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	rf.currentTerm++
	rf.state = Candidate
	rf.votedFor = rf.id

	fmt.Printf("[Node %d] Starting election in term %d\n", rf.id, rf.currentTerm)

	rf.lastHeartbeatTime = time.Now()

	rf.mu.Unlock()
	return
}
