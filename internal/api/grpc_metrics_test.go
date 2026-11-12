package api

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ChromaBeast/beastdb/internal/metrics"
)

func TestMetricsInterceptors(t *testing.T) {
	u := MetricsUnaryInterceptor()
	_, err := u(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/t.S/Unary"},
		func(context.Context, any) (any, error) { return nil, status.Error(codes.PermissionDenied, "no") })
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("error not propagated: %v", err)
	}
	s := MetricsStreamInterceptor()
	if err := s(nil, nil, &grpc.StreamServerInfo{FullMethod: "/t.S/Stream"},
		func(any, grpc.ServerStream) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	metrics.Default.WriteRequests(metrics.NewWriter(&b))
	out := b.String()
	for _, want := range []string{
		`beastdb_requests_total{protocol="grpc",method="/t.S/Unary",code="PermissionDenied"} 1`,
		`beastdb_requests_total{protocol="grpc",method="/t.S/Stream",code="OK"} 1`,
		`beastdb_request_duration_seconds_count{protocol="grpc",method="/t.S/Unary"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if metrics.InFlight.Load() != 0 {
		t.Fatalf("in-flight leaked: %d", metrics.InFlight.Load())
	}
}
