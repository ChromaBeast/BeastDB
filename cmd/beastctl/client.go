package main

import (
	"context"
	"fmt"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps gRPC connection and BeastDB service client.
type Client struct {
	conn   *grpc.ClientConn
	client beastv1.BeastDBServiceClient
}

// Dial creates a new client connection to BeastDB.
func Dial(addr string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("could not connect to BeastDB at %s: %w", addr, err)
	}

	return &Client{
		conn:   conn,
		client: beastv1.NewBeastDBServiceClient(conn),
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Service returns the proto service client.
func (c *Client) Service() beastv1.BeastDBServiceClient {
	return c.client
}
