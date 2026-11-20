package api

import (
	"context"
	"errors"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/index"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func kindToProto(k int) beastv1.IndexType {
	switch k {
	case index.KindInt:
		return beastv1.IndexType_INDEX_TYPE_INT
	case index.KindString:
		return beastv1.IndexType_INDEX_TYPE_STRING
	}
	return beastv1.IndexType_INDEX_TYPE_UNSPECIFIED
}

func kindFromProto(t beastv1.IndexType) int {
	switch t {
	case beastv1.IndexType_INDEX_TYPE_INT:
		return index.KindInt
	case beastv1.IndexType_INDEX_TYPE_STRING:
		return index.KindString
	}
	return 0
}

func indexInfoToProto(i IndexInfo) *beastv1.IndexInfo {
	return &beastv1.IndexInfo{
		Name: i.Name, Collection: i.Collection, Prefix: uint32(i.Prefix), Field: i.Field,
		Type: kindToProto(i.Kind), State: i.State, Entries: uint64(i.Entries),
		Skipped: uint64(i.Skipped), Examined: uint64(i.Examined), CreatedAt: i.CreatedAt,
	}
}

// indexStatus maps index and catalog errors to gRPC status codes.
func indexStatus(err error, op string) error {
	switch {
	case errors.Is(err, ErrInvalidIndex), errors.Is(err, ErrInvalidCollectionName), errors.Is(err, ErrIndexFilter):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrIndexExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, ErrIndexNotFound), errors.Is(err, ErrCollectionNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrIndexBuilding):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrTooManyIndexes):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, ErrConditionFailed):
		return status.Error(codes.Aborted, "index catalog changed concurrently; retry")
	default:
		return writeStatus(err, op)
	}
}

// CreateIndex declares an index and starts its background build.
func (s *GRPCServer) CreateIndex(ctx context.Context, req *beastv1.CreateIndexRequest) (*beastv1.CreateIndexResponse, error) {
	kind := kindFromProto(req.Type)
	if kind == 0 {
		return nil, status.Error(codes.InvalidArgument, "type must be INDEX_TYPE_INT or INDEX_TYPE_STRING")
	}
	if _, err := s.engine.CreateIndex(req.Name, req.Collection, req.Field, kind); err != nil {
		return nil, indexStatus(err, "create index")
	}
	infos, err := s.engine.ListIndexes()
	if err != nil {
		return nil, indexStatus(err, "list indexes")
	}
	for _, i := range infos {
		if i.Name == req.Name {
			return &beastv1.CreateIndexResponse{Index: indexInfoToProto(i)}, nil
		}
	}
	return &beastv1.CreateIndexResponse{}, nil
}

// ListIndexes reports every index with its live state.
func (s *GRPCServer) ListIndexes(ctx context.Context, _ *beastv1.ListIndexesRequest) (*beastv1.ListIndexesResponse, error) {
	infos, err := s.engine.ListIndexes()
	if err != nil {
		return nil, indexStatus(err, "list indexes")
	}
	out := make([]*beastv1.IndexInfo, len(infos))
	for i, x := range infos {
		out[i] = indexInfoToProto(x)
	}
	return &beastv1.ListIndexesResponse{Indexes: out}, nil
}

// DropIndex removes an index definition.
func (s *GRPCServer) DropIndex(ctx context.Context, req *beastv1.DropIndexRequest) (*beastv1.DropIndexResponse, error) {
	if err := s.engine.DropIndex(req.Name); err != nil {
		return nil, indexStatus(err, "drop index")
	}
	return &beastv1.DropIndexResponse{Success: true}, nil
}

func indexValueFromProto(v *beastv1.IndexValue) *IndexValue {
	if v == nil {
		return nil
	}
	if sv, ok := v.Value.(*beastv1.IndexValue_StringValue); ok {
		return &IndexValue{IsString: true, Str: sv.StringValue}
	}
	if iv, ok := v.Value.(*beastv1.IndexValue_IntValue); ok {
		return &IndexValue{Int: iv.IntValue}
	}
	return nil
}

// scanFiltered streams the records matching an index filter in key order.
func (s *GRPCServer) scanFiltered(req *beastv1.ScanRequest, stream grpc.ServerStreamingServer[beastv1.KeyValueResponse]) error {
	f := req.Filter
	filter := IndexFilter{Index: f.Index, Eq: indexValueFromProto(f.Eq), Gte: indexValueFromProto(f.Gte), Lte: indexValueFromProto(f.Lte)}
	end := req.EndKey
	if end == 0 {
		end = ^uint64(0) // documented: with a filter, end_key 0 means no upper bound
	}
	sc, err := s.engine.NewIndexScan(filter, req.StartKey, end)
	if err != nil {
		return indexStatus(err, "scan")
	}
	var sent uint32
	for sc.Remaining() > 0 {
		if err := stream.Context().Err(); err != nil {
			return err
		}
		want := scanChunkItems
		if req.Limit != 0 && int(req.Limit-sent) < want {
			want = int(req.Limit - sent)
		}
		items, _, err := sc.Next(want, scanChunkBytes)
		if err != nil {
			return indexStatus(err, "scan")
		}
		for _, it := range items {
			if err := stream.Send(&beastv1.KeyValueResponse{Key: it.Key, Value: it.Value}); err != nil {
				return err
			}
			sent++
		}
		if req.Limit != 0 && sent >= req.Limit {
			return nil
		}
	}
	return nil
}
