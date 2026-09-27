package api

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthInterceptor enforces Bearer token authentication on incoming gRPC RPCs.
type AuthInterceptor struct {
	token string
}

// NewAuthInterceptor creates a new gRPC auth interceptor.
func NewAuthInterceptor(token string) *AuthInterceptor {
	return &AuthInterceptor{token: strings.TrimSpace(token)}
}

// Unary returns a grpc.UnaryServerInterceptor.
func (a *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := a.authorize(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// Stream returns a grpc.StreamServerInterceptor.
func (a *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := a.authorize(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func (a *AuthInterceptor) authorize(ctx context.Context) error {
	if a.token == "" {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "missing metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return status.Errorf(codes.Unauthenticated, "missing authorization header")
	}
	header := vals[0]
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return status.Errorf(codes.Unauthenticated, "authorization must be Bearer token")
	}
	token := strings.TrimSpace(header[7:])
	if token != a.token {
		return status.Errorf(codes.Unauthenticated, "invalid authorization token")
	}
	return nil
}
