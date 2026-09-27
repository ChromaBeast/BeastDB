package main

import (
	"context"
	"fmt"
	"os"
	"strings"
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

type tokenAuth struct {
	token string
}

func (t tokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if t.token == "" {
		return nil, nil
	}
	return map[string]string{
		"authorization": "Bearer " + t.token,
	}, nil
}

func (t tokenAuth) RequireTransportSecurity() bool {
	return false
}

// Dial creates a new client connection to BeastDB, optionally authenticating with a token.
func Dial(addr string) (*Client, error) {
	token := strings.TrimSpace(os.Getenv("BEASTDB_API_TOKEN"))
	return DialWithToken(addr, token)
}

// DialWithToken creates a new client connection to BeastDB with explicit token.
func DialWithToken(addr, token string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	}
	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(tokenAuth{token: token}))
	}

	conn, err := grpc.DialContext(ctx, addr, opts...)
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
