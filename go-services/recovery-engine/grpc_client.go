package main

import (
	"context"
	"log"
)

type PythonGRPCClient struct {
	serverAddr string
}

func NewPythonGRPCClient(addr string) *PythonGRPCClient {
	return &PythonGRPCClient{
		serverAddr: addr,
	}
}

func (c *PythonGRPCClient) Connect(ctx context.Context) error {
	log.Printf("[grpc-client] Connecting to Python Decision Engine at %s...", c.serverAddr)
	return nil
}
