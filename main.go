package main

import (
	"log"

	"github.com/marksmwl/raft-kv/api"
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
	peers := []string{
		"localhost:8080",
		"localhost:8081",
		"localhost:8082",
		"localhost:8083",
		"localhost:8084",
	}

	apiAddrs := []string{
		"localhost:9080",
		"localhost:9081",
		"localhost:9082",
		"localhost:9083",
		"localhost:9084",
	}

	// Store node references so we can kill them
	nodes := make(map[int]*raft.Raft)

	for i, addr := range peers {
		id := i + 1
		node := raft.New(id, peers)

		apiServer := api.NewServer(node)

		nodes[id] = node

		go func(addr string, id int) {
			if err := node.ServeRPC(addr); err != nil {
				log.Fatalf("node %d serve: %v", id, err)
			}
		}(addr, id)

		go func(apiAddr string, id int) {
			if err := apiServer.StartHTTP(apiAddr); err != nil {
				log.Fatalf("API server %d serve: %v", id, err)
			}
		}(apiAddrs[i], id)

		go node.Start()
	}

	select {}
}
