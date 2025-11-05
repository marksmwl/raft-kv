package main

import (
	"fmt"
	"log"

	"github.com/marksmwl/raft-kv/raft"
)

const (
	DefaultListenAddr      = ":8080"
	DefaultMaxWorkers      = 100
	DefaultMaxQueue        = 1000
	DefaultMaxRequest      = 1000
	DefaultMaxResponse     = 1000
	DefaultMaxResponseTime = 1000
)

func main() {
	fmt.Println("Hello, World!")

	peers := []string{
		"localhost:8080",
		"localhost:8081",
		"localhost:8082",
	}

	// Store node references so we can kill them
	nodes := make(map[int]*raft.Raft)

	for i, addr := range peers {
		id := i + 1
		node := raft.New(id, peers)
		nodes[id] = node

		go func(id int, addr string, node *raft.Raft) {
			if err := node.ServeRPC(addr); err != nil {
				log.Fatalf("node %d serve: %v", id, err)
			}
			node.Start() // now the loop runs
		}(id, addr, node)
	}

	// Start interactive command handler

	select {}
}
