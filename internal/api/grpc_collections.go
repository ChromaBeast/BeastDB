package api

import (
	"context"
	"errors"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func collectionToProto(c Collection) *beastv1.Collection {
	return &beastv1.Collection{Name: c.Name, Prefix: uint32(c.Prefix), CreatedAt: c.CreatedAt}
}

// collectionStatus maps catalog errors to gRPC status codes.
func collectionStatus(err error, op string) error {
	switch {
	case errors.Is(err, ErrInvalidCollectionName), errors.Is(err, ErrReservedPrefix):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrCollectionExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, ErrCollectionNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrCollectionNotEmpty):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrNoFreePrefix):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, ErrConditionFailed):
		return status.Error(codes.Aborted, "catalog changed concurrently; retry")
	default:
		return writeStatus(err, op)
	}
}

// CreateCollection binds a name to a key prefix.
func (s *GRPCServer) CreateCollection(ctx context.Context, req *beastv1.CreateCollectionRequest) (*beastv1.CreateCollectionResponse, error) {
	var prefix *uint8
	if req.Prefix != nil {
		if *req.Prefix > 255 {
			return nil, status.Error(codes.InvalidArgument, "prefix must be 0-255")
		}
		p := uint8(*req.Prefix)
		prefix = &p
	}
	col, err := s.engine.CreateCollection(req.Name, prefix)
	if err != nil {
		return nil, collectionStatus(err, "create collection")
	}
	return &beastv1.CreateCollectionResponse{Collection: collectionToProto(col)}, nil
}

// ListCollections returns every named collection ordered by prefix.
func (s *GRPCServer) ListCollections(ctx context.Context, _ *beastv1.ListCollectionsRequest) (*beastv1.ListCollectionsResponse, error) {
	cols, err := s.engine.ListCollections()
	if err != nil {
		return nil, collectionStatus(err, "list collections")
	}
	out := make([]*beastv1.Collection, len(cols))
	for i, c := range cols {
		out[i] = collectionToProto(c)
	}
	return &beastv1.ListCollectionsResponse{Collections: out}, nil
}

// DropCollection unbinds a collection name; the prefix must hold no records.
func (s *GRPCServer) DropCollection(ctx context.Context, req *beastv1.DropCollectionRequest) (*beastv1.DropCollectionResponse, error) {
	if err := s.engine.DropCollection(req.Name); err != nil {
		return nil, collectionStatus(err, "drop collection")
	}
	return &beastv1.DropCollectionResponse{Success: true}, nil
}
