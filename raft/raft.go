package raft

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/marksmwl/raft-kv/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

// StateMachine represents the interface for applying committed entries
type StateMachine interface {
	ApplyCommand(command interface{})
}

type Raft struct {
	proto.UnimplementedRaftServer

	mu sync.Mutex

	peers   []string
	clients []proto.RaftClient
	id      int
	state   NodeState

	currentTerm int
	votedFor    int

	log         []LogEntry
	commitIndex int
	lastApplied int

	nextIndex  []int
	matchIndex []int

	lastHeartbeatTime time.Time
	stateMachine      StateMachine

	dead int32
}

func (rf *Raft) GetID() int {
	return rf.id
}

func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	fmt.Printf("[Node %d] Has been killed\n", rf.id)
}

func (rf *Raft) Revive() {
	rf.mu.Lock()
	rf.state = Follower
	rf.votedFor = -1
	rf.resetElectionTimer()
	rf.mu.Unlock()
	
	atomic.StoreInt32(&rf.dead, 0)
	fmt.Printf("[Node %d] Has been revived\n", rf.id)
	go rf.Start()
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
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
	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))

	clients := make([]proto.RaftClient, len(peers))
	for i, peer := range peers {
		if i == id-1 {
			continue
		}
		conn, err := grpc.NewClient(peer, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			clients[i] = proto.NewRaftClient(conn)
		} else {
			fmt.Printf("Failed to dial peer %s: %v\n", peer, err)
		}
	}
	rf.clients = clients

	fmt.Println("Node", id, "is now a", rf.state)
	return rf
}

// SetStateMachine sets the state machine to apply committed entries
func (rf *Raft) SetStateMachine(sm StateMachine) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.stateMachine = sm
}

func (rf *Raft) Start() {
	// Start applying committed entries
	go rf.applyCommittedEntries()

	for {
		if rf.killed() {
			return
		}
		time.Sleep(5 * time.Millisecond) // 1 second heartbeat
		rf.mu.Lock()

		elapsedTime := time.Since(rf.lastHeartbeatTime)

		if rf.state != Leader && elapsedTime > rf.getElectionTimeout() {
			if rf.state == Follower {
				fmt.Printf("[Node %d] Haven't heard from leader in %v, should start election...\n", rf.id, elapsedTime)
			} else {
				fmt.Printf("[Node %d] Election timeout in %v, restarting election...\n", rf.id, elapsedTime)
			}

			rf.mu.Unlock()
			rf.startElection()
		} else {
			rf.mu.Unlock()
		}
	}
}

// applyCommittedEntries applies committed entries to the state machine
func (rf *Raft) applyCommittedEntries() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		if rf.killed() {
			return
		}
		rf.mu.Lock()
		for rf.lastApplied < rf.commitIndex {
			rf.lastApplied++
			entry := rf.log[rf.lastApplied-1] // log is 1-indexed conceptually
			if rf.stateMachine != nil {
				rf.mu.Unlock()
				rf.stateMachine.ApplyCommand(entry.Command)
				rf.mu.Lock()
			}
		}
		rf.mu.Unlock()
	}
}

// StartCommand starts agreement on a new log entry. Returns the index of the entry.
// If this server isn't the leader, returns -1, false.
func (rf *Raft) StartCommand(command interface{}) (int, bool) {
	if rf.killed() {
		return -1, false
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.state != Leader {
		return -1, false
	}

	// Append entry to local log
	index := len(rf.log) + 1
	entry := LogEntry{
		Term:    rf.currentTerm,
		Index:   index,
		Command: command,
	}
	rf.log = append(rf.log, entry)
	fmt.Printf("[Node %d] Leader appended entry at index %d\n", rf.id, index)

	return index, true
}

// IsLeader returns true if this node is the leader
func (rf *Raft) IsLeader() bool {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.state == Leader
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
		if rf.killed() {
			return
		}
		rf.mu.Lock()
		// Only send heartbeats if we're still the leader
		if rf.state != Leader {
			rf.mu.Unlock()
			return
		}
		currentTerm := rf.currentTerm
		commitIndex := rf.commitIndex
		rf.mu.Unlock()

		// Send AppendEntries RPC to all peers (skip self)
		ownIndex := leaderId - 1
		for i, peer := range peers {
			// Skip sending heartbeat to ourselves
			if i == ownIndex {
				continue
			}

			go func(peerIndex int, peer string) {
				rf.mu.Lock()
				nextIdx := rf.nextIndex[peerIndex]
				prevLogIndex := nextIdx - 1
				prevLogTerm := 0
				if prevLogIndex > 0 && prevLogIndex <= len(rf.log) {
					prevLogTerm = rf.log[prevLogIndex-1].Term
				}
				entries := []LogEntry{}
				if nextIdx <= len(rf.log) {
					entries = rf.log[nextIdx-1:]
				}
				rf.mu.Unlock()

				protoEntries := make([]*proto.LogEntry, len(entries))
				for idx, e := range entries {
					cmdBytes, _ := json.Marshal(e.Command)
					protoEntries[idx] = &proto.LogEntry{
						Term:    int64(e.Term),
						Index:   int64(e.Index),
						Command: cmdBytes,
					}
				}

				args := &proto.AppendEntriesRequest{
					Term:         int64(currentTerm),
					LeaderId:     int32(leaderId),
					PrevLogIndex: int64(prevLogIndex),
					PrevLogTerm:  int64(prevLogTerm),
					Entries:      protoEntries,
					LeaderCommit: int64(commitIndex),
				}
				
				client := rf.clients[peerIndex]
				if client == nil {
					return
				}
				
				reply, err := client.AppendEntries(context.Background(), args)
				if err != nil {
					fmt.Printf("[Node %d] AppendEntries RPC to %s failed: %v\n", leaderId, peer, err)
				} else {
					rf.mu.Lock()
					// If we got a reply with a higher term, step down
					if reply.Term > int64(rf.currentTerm) {
						rf.currentTerm = int(reply.Term)
						rf.state = Follower
						rf.votedFor = -1
						rf.resetElectionTimer()
						rf.mu.Unlock()
						return
					}
					if reply.Success {
						rf.matchIndex[peerIndex] = prevLogIndex + len(entries)
						rf.nextIndex[peerIndex] = rf.matchIndex[peerIndex] + 1
						// Update commit index if majority has replicated
						for n := rf.commitIndex + 1; n <= len(rf.log); n++ {
							count := 1 // leader itself
							for j := range rf.peers {
								if j != ownIndex && rf.matchIndex[j] >= n {
									count++
								}
							}
							if count > len(rf.peers)/2 && rf.log[n-1].Term == currentTerm {
								rf.commitIndex = n
							}
						}
					} else {
						// Decrement nextIndex and retry
						if rf.nextIndex[peerIndex] > 1 {
							rf.nextIndex[peerIndex]--
						}
					}
					rf.mu.Unlock()
				}
			}(i, peer)
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

		go func(peerIndex int, peer string) {
			args := &proto.RequestVoteRequest{
				Term:        int64(currentTerm),
				CandidateId: int32(candidateId),
			}
			
			client := rf.clients[peerIndex]
			if client == nil {
				return
			}
			
			reply, err := client.RequestVote(context.Background(), args)
			if err != nil {
				fmt.Printf("[Node %d] RequestVote RPC to %s failed: %v\n", candidateId, peer, err)
				return
			}

			rf.mu.Lock()
			defer rf.mu.Unlock()

			// If reply has higher term, step down
			if reply.Term > int64(rf.currentTerm) {
				rf.currentTerm = int(reply.Term)
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
					// Initialize leader state
					for i := range rf.peers {
						rf.nextIndex[i] = len(rf.log) + 1
						rf.matchIndex[i] = 0
					}
					fmt.Printf("[Node %d] Became leader in term %d\n", candidateId, rf.currentTerm)
					rf.mu.Unlock()
					// Start sending heartbeats
					go rf.leaderHeartbeat()
					rf.mu.Lock()
				}
			}
		}(i, peer)
	}
}
