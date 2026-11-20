package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
)

func runCollections(addr, action, name string, prefix int) error {
	client, err := Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc := client.Service()
	switch action {
	case "list":
		res, err := svc.ListCollections(ctx, &beastv1.ListCollectionsRequest{})
		if err != nil {
			return fmt.Errorf("list collections: %w", err)
		}
		fmt.Printf("%-24s %-8s\n", "NAME", "PREFIX")
		for _, c := range res.Collections {
			fmt.Printf("%-24s 0x%02X\n", c.Name, c.Prefix)
		}
	case "create":
		req := &beastv1.CreateCollectionRequest{Name: name}
		if prefix >= 0 {
			p := uint32(prefix)
			req.Prefix = &p
		}
		res, err := svc.CreateCollection(ctx, req)
		if err != nil {
			return fmt.Errorf("create collection: %w", err)
		}
		fmt.Printf("Created collection %q at prefix 0x%02X.\n", res.Collection.Name, res.Collection.Prefix)
	case "drop":
		if _, err := svc.DropCollection(ctx, &beastv1.DropCollectionRequest{Name: name}); err != nil {
			return fmt.Errorf("drop collection: %w", err)
		}
		fmt.Printf("Dropped collection %q.\n", name)
	default:
		return fmt.Errorf("unknown action %q (want list, create or drop)", action)
	}
	return nil
}

func runIndexCmd(addr, action, name, collection, field, typ string) error {
	client, err := Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc := client.Service()
	switch action {
	case "list":
		res, err := svc.ListIndexes(ctx, &beastv1.ListIndexesRequest{})
		if err != nil {
			return fmt.Errorf("list indexes: %w", err)
		}
		fmt.Printf("%-20s %-16s %-20s %-7s %-9s %8s %8s\n", "NAME", "COLLECTION", "FIELD", "TYPE", "STATE", "ENTRIES", "SKIPPED")
		for _, i := range res.Indexes {
			fmt.Printf("%-20s %-16s %-20s %-7s %-9s %8d %8d\n", i.Name, i.Collection, i.Field, typeName(i.Type), i.State, i.Entries, i.Skipped)
		}
	case "create":
		var t beastv1.IndexType
		switch typ {
		case "int":
			t = beastv1.IndexType_INDEX_TYPE_INT
		case "string":
			t = beastv1.IndexType_INDEX_TYPE_STRING
		default:
			return fmt.Errorf(`--type must be "int" or "string"`)
		}
		if _, err := svc.CreateIndex(ctx, &beastv1.CreateIndexRequest{Name: name, Collection: collection, Field: field, Type: t}); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
		fmt.Printf("Index %q created; it builds in the background (see: beastctl index list).\n", name)
	case "drop":
		if _, err := svc.DropIndex(ctx, &beastv1.DropIndexRequest{Name: name}); err != nil {
			return fmt.Errorf("drop index: %w", err)
		}
		fmt.Printf("Dropped index %q.\n", name)
	default:
		return fmt.Errorf("unknown action %q (want list, create or drop)", action)
	}
	return nil
}

func typeName(t beastv1.IndexType) string {
	switch t {
	case beastv1.IndexType_INDEX_TYPE_INT:
		return "int"
	case beastv1.IndexType_INDEX_TYPE_STRING:
		return "string"
	}
	return "?"
}

// scanArgs are the flags of `beastctl scan`.
type scanArgs struct {
	start, end, limit     uint64
	index, eq, gte, lte   string
	hasEq, hasGte, hasLte bool
}

func runScan(addr string, a scanArgs) error {
	client, err := Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	ctx := context.Background()
	req := &beastv1.ScanRequest{StartKey: a.start, EndKey: a.end, Limit: uint32(a.limit)}
	if a.index != "" {
		list, err := client.Service().ListIndexes(ctx, &beastv1.ListIndexesRequest{})
		if err != nil {
			return fmt.Errorf("list indexes: %w", err)
		}
		var typ beastv1.IndexType
		for _, i := range list.Indexes {
			if i.Name == a.index {
				typ = i.Type
			}
		}
		if typ == beastv1.IndexType_INDEX_TYPE_UNSPECIFIED {
			return fmt.Errorf("index %q not found", a.index)
		}
		val := func(s string) (*beastv1.IndexValue, error) {
			if typ == beastv1.IndexType_INDEX_TYPE_STRING {
				return &beastv1.IndexValue{Value: &beastv1.IndexValue_StringValue{StringValue: s}}, nil
			}
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("value %q is not an integer (index %q is int)", s, a.index)
			}
			return &beastv1.IndexValue{Value: &beastv1.IndexValue_IntValue{IntValue: n}}, nil
		}
		f := &beastv1.IndexFilter{Index: a.index}
		if a.hasEq {
			if f.Eq, err = val(a.eq); err != nil {
				return err
			}
		}
		if a.hasGte {
			if f.Gte, err = val(a.gte); err != nil {
				return err
			}
		}
		if a.hasLte {
			if f.Lte, err = val(a.lte); err != nil {
				return err
			}
		}
		req.Filter = f
	}
	stream, err := client.Service().Scan(ctx, req)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	n := 0
	for {
		r, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		fmt.Printf("%d\t%s\n", r.Key, r.Value)
		n++
	}
	fmt.Printf("(%d records)\n", n)
	return nil
}
