package api

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestAuthInterceptor(t *testing.T) {
	ai := NewAuthInterceptor("secret-token-123")

	// 1. Missing metadata -> unauthenticated
	err := ai.authorize(context.Background())
	if err == nil {
		t.Fatalf("expected error for missing metadata, got nil")
	}

	// 2. Wrong token -> unauthenticated
	badCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer wrong"))
	if err := ai.authorize(badCtx); err == nil {
		t.Fatalf("expected error for bad token, got nil")
	}

	// 3. Valid token -> allowed
	goodCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer secret-token-123"))
	if err := ai.authorize(goodCtx); err != nil {
		t.Fatalf("expected success for valid token, got %v", err)
	}

	// 4. Empty configured token -> open access
	openAI := NewAuthInterceptor("")
	if err := openAI.authorize(context.Background()); err != nil {
		t.Fatalf("expected success when token is empty, got %v", err)
	}
}
