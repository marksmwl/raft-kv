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

	log         []LogEntry
	commitIndex int
	lastApplied int

	nextIndex  []int
	matchIndex []int

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

	rf.log = make([]LogEntry, 0)
	rf.commitIndex = 0
	rf.lastApplied = 0

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

			rf.mu.Unlock()
			rf.startElection()
		} else {
			rf.mu.Unlock()
		}
	}
}

// getElectionTimeout returns a random timeout between 150-300ms
func (rf *Raft) getElectionTimeout() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

// resetElectionTimer resets the election timeout timer
func (rf *Raft) resetElectionTimer() {
	rf.lastHeartbeatTime = time.Now()
}

func (rf *Raft) leaderHeartbeat() {
	t := time.NewTicker(80 * time.Millisecond)
	defer t.Stop()

	leaderId := rf.id
	peers := rf.peers

	for range t.C {
		rf.mu.Lock()
		// Only send heartbeats if we're still the leader
		if rf.state != Leader {
			rf.mu.Unlock()
			return
		}
		currentTerm := rf.currentTerm
		rf.mu.Unlock()

		// Send AppendEntries RPC with empty entries (heartbeat) to all peers (skip self)
		ownIndex := leaderId - 1
		for i, peer := range peers {
			// Skip sending heartbeat to ourselves
			if i == ownIndex {
				continue
			}

			go func(peer string) {
				args := &AppendEntriesArgs{
					Term:         currentTerm,
					LeaderId:     leaderId,
					PrevLogIndex: -1, // No previous log entry for heartbeat
					PrevLogTerm:  -1,
					Entries:      []LogEntry{}, // Empty entries for heartbeat
					LeaderCommit: -1,
				}
				reply := &AppendEntriesReply{}
				if err := call(peer, "Raft.AppendEntries", args, reply); err != nil {
					fmt.Printf("[Node %d] AppendEntries heartbeat RPC to %s failed: %v\n", leaderId, peer, err)
				} else {
					rf.mu.Lock()
					// If we got a reply with a higher term, step down
					if reply.Term > rf.currentTerm {
						rf.currentTerm = reply.Term
						rf.state = Follower
						rf.votedFor = -1
						rf.resetElectionTimer()
					}
					rf.mu.Unlock()
				}
			}(peer)
		}
	}
}

func (rf *Raft) startElection() {
	rf.mu.Lock()

	// Step 1: Increment term and become candidate
	rf.currentTerm++
	rf.state = Candidate
	rf.votedFor = rf.id

	// Reset election timer so we don't timeout mid-election
	rf.resetElectionTimer()

	currentTerm := rf.currentTerm
	candidateId := rf.id
	rf.mu.Unlock()

	fmt.Printf("[Node %d] Starting election in term %d\n", candidateId, currentTerm)

	// Step 2: Send RequestVote RPCs to all other nodes (skip self)
	votes := 1 // Count our own vote
	votesMutex := &sync.Mutex{}
	totalNodes := len(rf.peers)
	requiredVotes := (totalNodes + 1) / 2 // Majority

	// Skip our own address (node id i corresponds to peers[i-1])
	ownIndex := candidateId - 1

	for i, peer := range rf.peers {
		// Skip sending RequestVote to ourselves
		if i == ownIndex {
			continue
		}

		go func(peer string) {
			args := &RequestVoteArgs{
				Term:        currentTerm,
				CandidateId: candidateId,
			}
			reply := &RequestVoteReply{}
			if err := call(peer, "Raft.RequestVote", args, reply); err != nil {
				fmt.Printf("[Node %d] RequestVote RPC to %s failed: %v\n", candidateId, peer, err)
				return
			}

			rf.mu.Lock()
			defer rf.mu.Unlock()

			// If reply has higher term, step down
			if reply.Term > rf.currentTerm {
				rf.currentTerm = reply.Term
				rf.state = Follower
				rf.votedFor = -1
				rf.resetElectionTimer()
				return
			}

			// If we're no longer a candidate, ignore the vote
			if rf.state != Candidate || rf.currentTerm != currentTerm {
				return
			}

			if reply.VoteGranted {
				votesMutex.Lock()
				votes++
				gotVotes := votes
				votesMutex.Unlock()

				fmt.Printf("[Node %d] Received vote from %s (total: %d/%d)\n", candidateId, peer, gotVotes, totalNodes)

				// If we have majority, become leader
				if gotVotes >= requiredVotes {
					rf.state = Leader
					rf.votedFor = -1
					fmt.Printf("[Node %d] Became leader in term %d\n", candidateId, rf.currentTerm)
					rf.mu.Unlock()
					// Start sending heartbeats
					go rf.leaderHeartbeat()
					rf.mu.Lock()
				}
			}
		}(peer)
	}
}
