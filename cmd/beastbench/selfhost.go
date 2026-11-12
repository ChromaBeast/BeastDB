package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/ChromaBeast/beastdb/internal/api"
)

// StartSelfHosted runs an in-process engine and gRPC server on a temp dir
// listening on loopback. The returned stop func removes everything.
func StartSelfHosted(poolSize int) (addr string, stop func(), err error) {
	dir, err := os.MkdirTemp("", "beastbench-*")
	if err != nil {
		return "", nil, err
	}
	eng, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), poolSize)
	if err != nil {
		os.RemoveAll(dir)
		return "", nil, fmt.Errorf("start engine: %w", err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		eng.Close()
		os.RemoveAll(dir)
		return "", nil, err
	}
	srv := api.NewGRPCServer(eng, api.MessageSizeServerOptions()...)
	done := make(chan struct{})
	go func() { _ = srv.Serve(lis); close(done) }()
	return lis.Addr().String(), func() {
		srv.Stop()
		<-done
		eng.Close()
		os.RemoveAll(dir)
	}, nil
}
