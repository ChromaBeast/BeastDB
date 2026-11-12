package api

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/ChromaBeast/beastdb/internal/metrics"
)

// MetricsUnaryInterceptor records count, status code and latency of unary RPCs.
func MetricsUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		metrics.InFlight.Add(1)
		start := time.Now()
		resp, err := h(ctx, req)
		metrics.InFlight.Add(-1)
		metrics.Default.Observe("grpc", info.FullMethod, status.Code(err).String(), time.Since(start))
		return resp, err
	}
}

// MetricsStreamInterceptor records count, status code and total duration of streaming RPCs.
func MetricsStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		metrics.InFlight.Add(1)
		start := time.Now()
		err := h(srv, ss)
		metrics.InFlight.Add(-1)
		metrics.Default.Observe("grpc", info.FullMethod, status.Code(err).String(), time.Since(start))
		return err
	}
}
