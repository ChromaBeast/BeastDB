package api

import (
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MaxGRPCMessageSize is the gRPC message limit servers and clients should use
// so a maximal batch (MaxBatchBytes of values plus keys and framing) fits.
const MaxGRPCMessageSize = MaxBatchBytes + 1<<20

// MessageSizeServerOptions raises gRPC's 4 MiB default to MaxGRPCMessageSize.
func MessageSizeServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{grpc.MaxRecvMsgSize(MaxGRPCMessageSize), grpc.MaxSendMsgSize(MaxGRPCMessageSize)}
}

// MessageSizeDialOption is the client-side counterpart of MessageSizeServerOptions.
func MessageSizeDialOption() grpc.DialOption {
	return grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxGRPCMessageSize), grpc.MaxCallSendMsgSize(MaxGRPCMessageSize))
}

// writeStatus maps engine write errors to gRPC status codes.
func writeStatus(err error, op string) error {
	switch {
	case errors.Is(err, ErrConditionFailed):
		return status.Errorf(codes.Aborted, "precondition failed")
	case errors.Is(err, ErrReadOnlyReplica):
		return status.Errorf(codes.FailedPrecondition, "writes are not permitted on read replica")
	case errors.Is(err, ErrRecordTooLarge), errors.Is(err, ErrBatchTooLarge):
		return status.Errorf(codes.InvalidArgument, "%s: %v", op, err)
	default:
		return status.Errorf(codes.Internal, "%s error: %v", op, err)
	}
}
