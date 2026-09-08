package net

import (
	"bytes"
	"net"
	"testing"
)

func TestServerPingPong(t *testing.T) {
	// Simple database mock handler
	handler := func(req *Frame) *Frame {
		if req.OpCode == OpPing {
			return &Frame{OpCode: OpResponse, Payload: []byte("PONG")}
		}
		return &Frame{OpCode: OpError, Payload: []byte("unknown op")}
	}

	server := NewServer("127.0.0.1:0", handler)
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	// Connect client socket
	conn, err := net.Dial("tcp", server.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Send PING frame
	reqBytes := EncodeFrame(OpPing, []byte("PING"))
	if _, err := conn.Write(reqBytes); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// Receive response frame
	resp, err := DecodeFrame(conn)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if resp.OpCode != OpResponse {
		t.Fatalf("unexpected OpCode: got %d, want %d", resp.OpCode, OpResponse)
	}

	if !bytes.Equal(resp.Payload, []byte("PONG")) {
		t.Fatalf("unexpected payload: got %q, want PONG", resp.Payload)
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	server := NewServer("127.0.0.1:0", func(req *Frame) *Frame { return nil })
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	if err := server.Stop(); err != nil {
		t.Fatalf("failed to stop server gracefully: %v", err)
	}
}
