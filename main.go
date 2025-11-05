package main

import (
	"fmt"

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

	node := raft.New(1, peers)
	node.Start()
	select {}
}
