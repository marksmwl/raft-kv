package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/marksmwl/raft-kv/kvstore"
	"github.com/marksmwl/raft-kv/raft"
)

// KVCommand represents a command for the key-value store
type KVCommand struct {
	Op    string `json:"op"` // "put" or "delete"
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// KVStateMachine implements StateMachine interface for the KV store
type KVStateMachine struct {
	store *kvstore.Store
	mu    sync.Mutex
}

func NewKVStateMachine() *KVStateMachine {
	return &KVStateMachine{
		store: kvstore.New(),
	}
}

func (sm *KVStateMachine) ApplyCommand(command interface{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var cmd KVCommand

	// Try to convert command to KVCommand
	switch v := command.(type) {
	case KVCommand:
		cmd = v
	case map[string]interface{}:
		// Convert map to KVCommand via JSON marshaling/unmarshaling
		cmdBytes, err := json.Marshal(v)
		if err != nil {
			log.Printf("Failed to marshal command: %v", err)
			return
		}
		if err := json.Unmarshal(cmdBytes, &cmd); err != nil {
			log.Printf("Failed to unmarshal command: %v", err)
			return
		}
	default:
		log.Printf("Invalid command type: %T, value: %v", command, command)
		return
	}

	switch cmd.Op {
	case "put":
		sm.store.Put(cmd.Key, cmd.Value)
		log.Printf("Applied PUT: %s = %s", cmd.Key, cmd.Value)
	case "delete":
		sm.store.Delete(cmd.Key)
		log.Printf("Applied DELETE: %s", cmd.Key)
	default:
		log.Printf("Unknown operation: %s", cmd.Op)
	}
}

func (sm *KVStateMachine) Get(key string) (string, bool) {
	return sm.store.Get(key)
}

// Server represents the HTTP API server
type Server struct {
	raft *raft.Raft
	sm   *KVStateMachine
	mu   sync.Mutex
	mux  *http.ServeMux
}

func NewServer(raftNode *raft.Raft) *Server {
	sm := NewKVStateMachine()
	raftNode.SetStateMachine(sm)
	return &Server{
		raft: raftNode,
		sm:   sm,
		mux:  http.NewServeMux(),
	}
}

func (s *Server) StartHTTP(addr string) error {
	s.mux.HandleFunc("/key/", s.handleKey)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/kill", s.handleKill)
	s.mux.HandleFunc("/revive", s.handleRevive)
	log.Printf("[Node %d] HTTP API server starting on %s", s.raft.GetID(), addr)
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	isLeader := s.raft.IsLeader()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"isLeader": isLeader,
	})
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.raft.Kill()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "killed",
		"node":   s.raft.GetID(),
	})
}

func (s *Server) handleRevive(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.raft.Revive()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "revived",
		"node":   s.raft.GetID(),
	})
}

func (s *Server) handleKey(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path[len("/key/"):]
	if key == "" {
		http.Error(w, "Key is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		s.handleGet(w, r, key)
	case "PUT", "POST":
		s.handlePut(w, r, key)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, key string) {
	value, exists := s.sm.Get(key)
	if !exists {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"key":   key,
		"value": value,
	})
}

func (s *Server) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON body
	var req map[string]string
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	value, ok := req["value"]
	if !ok {
		http.Error(w, "Value is required", http.StatusBadRequest)
		return
	}

	// Check if this node is the leader
	if !s.raft.IsLeader() {
		http.Error(w, "Not the leader", http.StatusServiceUnavailable)
		return
	}

	// Create command
	cmd := KVCommand{
		Op:    "put",
		Key:   key,
		Value: value,
	}

	// Start the command (append to log and replicate)
	index, ok := s.raft.StartCommand(cmd)
	if !ok {
		http.Error(w, "Failed to start command", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"index":   index,
		"key":     key,
		"value":   value,
		"message": fmt.Sprintf("Entry appended at index %d, will be replicated via AppendEntries", index),
	})
}
