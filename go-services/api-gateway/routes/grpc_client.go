package routes

import (
	"context"
	"log"
	"sync"
	"time"

	predictpb "github.com/aegis/go-services/shared/pb/predict"
	recoverypb "github.com/aegis/go-services/shared/pb/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type PythonGRPCClient struct {
	addr       string
	conn       *grpc.ClientConn
	predictCl  predictpb.PredictionServiceClient
	recoveryCl recoverypb.RecoveryServiceClient
	mu         sync.RWMutex
}

var GlobalGRPCClient *PythonGRPCClient

func InitGRPCClient(addr string) *PythonGRPCClient {
	if addr == "" {
		addr = "localhost:50051"
	}
	client := &PythonGRPCClient{addr: addr}
	GlobalGRPCClient = client
	return client
}

func (c *PythonGRPCClient) getConn(ctx context.Context) (*grpc.ClientConn, error) {
	c.mu.RLock()
	if c.conn != nil {
		conn := c.conn
		c.mu.RUnlock()
		return conn, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn, nil
	}

	dialCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		// Non-blocking fallback so dial doesn't permanently fail if Python starts after gateway
		conn, err = grpc.Dial(c.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
	}

	c.conn = conn
	c.predictCl = predictpb.NewPredictionServiceClient(conn)
	c.recoveryCl = recoverypb.NewRecoveryServiceClient(conn)
	log.Printf("[grpc-client] Initialized connection to Python ML Engine at %s", c.addr)
	return c.conn, nil
}

func (c *PythonGRPCClient) Predict(ctx context.Context, req *predictpb.PredictRequest) (*predictpb.PredictResponse, error) {
	if _, err := c.getConn(ctx); err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.predictCl.Predict(callCtx, req)
}

func (c *PythonGRPCClient) GeneratePlan(ctx context.Context, req *predictpb.PredictResponse) (*recoverypb.RecoveryPlan, error) {
	if _, err := c.getConn(ctx); err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.recoveryCl.GeneratePlan(callCtx, req)
}
