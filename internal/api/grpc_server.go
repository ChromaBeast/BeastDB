package api

import (
	"context"
	"net"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/index"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCServer exposes the BeastDB database engine over high-performance gRPC.
type GRPCServer struct {
	beastv1.UnimplementedBeastDBServiceServer
	engine     *Engine
	grpcServer *grpc.Server
}

// NewGRPCServer initializes a new gRPC service wrapping the engine.
func NewGRPCServer(engine *Engine) *GRPCServer {
	s := &GRPCServer{
		engine:     engine,
		grpcServer: grpc.NewServer(),
	}
	beastv1.RegisterBeastDBServiceServer(s.grpcServer, s)
	return s
}

// Serve starts accepting gRPC requests on listener.
func (s *GRPCServer) Serve(lis net.Listener) error {
	return s.grpcServer.Serve(lis)
}

// GracefulStop shuts down the gRPC server gracefully.
func (s *GRPCServer) GracefulStop() {
	s.grpcServer.GracefulStop()
}

// Stop terminates the gRPC server immediately.
func (s *GRPCServer) Stop() {
	s.grpcServer.Stop()
}

// Get handles unary key lookup requests.
func (s *GRPCServer) Get(ctx context.Context, req *beastv1.GetRequest) (*beastv1.GetResponse, error) {
	val, found, err := s.engine.Get(req.Key)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get error: %v", err)
	}
	return &beastv1.GetResponse{
		Key:   req.Key,
		Value: val,
		Found: found,
	}, nil
}

// Put handles unary key insertion/update requests.
func (s *GRPCServer) Put(ctx context.Context, req *beastv1.PutRequest) (*beastv1.PutResponse, error) {
	if err := s.engine.Put(req.Key, req.Value); err != nil {
		return nil, status.Errorf(codes.Internal, "put error: %v", err)
	}
	return &beastv1.PutResponse{Success: true}, nil
}

// Delete handles unary key deletion requests.
func (s *GRPCServer) Delete(ctx context.Context, req *beastv1.DeleteRequest) (*beastv1.DeleteResponse, error) {
	err := s.engine.Delete(req.Key)
	if err == index.ErrKeyNotFound {
		return nil, status.Errorf(codes.NotFound, "key not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete error: %v", err)
	}
	return &beastv1.DeleteResponse{Success: true}, nil
}

// Scan handles server-streaming range scan requests.
func (s *GRPCServer) Scan(req *beastv1.ScanRequest, stream grpc.ServerStreamingServer[beastv1.KeyValueResponse]) error {
	cursor, err := s.engine.Scan(req.StartKey, req.EndKey)
	if err != nil {
		return status.Errorf(codes.Internal, "scan error: %v", err)
	}
	defer cursor.Close()

	for {
		if stream.Context().Err() != nil {
			return stream.Context().Err()
		}

		key, rid, ok, err := cursor.Next()
		if err != nil {
			return status.Errorf(codes.Internal, "cursor error: %v", err)
		}
		if !ok {
			break
		}

		val, err := s.engine.ReadTuple(rid)
		if err != nil {
			return status.Errorf(codes.Internal, "read tuple error: %v", err)
		}

		resp := &beastv1.KeyValueResponse{
			Key:   key,
			Value: val,
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
}
