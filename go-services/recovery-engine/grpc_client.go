package main

import (
	"context"
	"log"
	"sync"
	"time"

	recoverypb "github.com/aegis/go-services/shared/pb/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type PythonGRPCClient struct {
	serverAddr string
	conn       *grpc.ClientConn
	recoveryCl recoverypb.RecoveryServiceClient
	mu         sync.RWMutex
}

func NewPythonGRPCClient(addr string) *PythonGRPCClient {
	if addr == "" {
		addr = "localhost:50051"
	}
	return &PythonGRPCClient{
		serverAddr: addr,
	}
}

func (c *PythonGRPCClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("[grpc-client] Connecting to Python Decision Engine at %s...", c.serverAddr)
	conn, err := grpc.DialContext(ctx, c.serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	c.conn = conn
	c.recoveryCl = recoverypb.NewRecoveryServiceClient(conn)
	return nil
}

func (c *PythonGRPCClient) ExecutePlan(ctx context.Context, plan *recoverypb.RecoveryPlan) (*recoverypb.RecoveryResult, error) {
	c.mu.RLock()
	if c.conn == nil {
		c.mu.RUnlock()
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
	} else {
		c.mu.RUnlock()
	}
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.recoveryCl.ExecutePlan(callCtx, plan)
}
