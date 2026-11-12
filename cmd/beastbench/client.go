package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// tokenAuth mirrors cmd/beastctl: Bearer token in the authorization header.
type tokenAuth struct{ token string }

func (t tokenAuth) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	if t.token == "" {
		return nil, nil
	}
	return map[string]string{"authorization": "Bearer " + t.token}, nil
}

// RequireTransportSecurity is false to match beastctl, so tokens work on plain
// loopback test servers; use the TLS flags for anything non-local.
func (t tokenAuth) RequireTransportSecurity() bool { return false }

// ConnOpts describes how to reach the server.
type ConnOpts struct {
	Addr, Token            string
	TLSCert, TLSKey, TLSCA string
	TLSServerName          string
	Conns                  int
}

func (o ConnOpts) creds() (credentials.TransportCredentials, error) {
	if o.TLSCert == "" && o.TLSCA == "" {
		return insecure.NewCredentials(), nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: o.TLSServerName}
	if o.TLSCA != "" {
		pem, err := os.ReadFile(o.TLSCA)
		if err != nil {
			return nil, fmt.Errorf("read CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates in %s", o.TLSCA)
		}
		cfg.RootCAs = pool
	}
	if o.TLSCert != "" || o.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(o.TLSCert, o.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return credentials.NewTLS(cfg), nil
}

// Dial opens o.Conns connections and verifies the server answers.
func Dial(o ConnOpts) ([]beastv1.BeastDBServiceClient, func(), error) {
	creds, err := o.creds()
	if err != nil {
		return nil, nil, err
	}
	n := o.Conns
	if n < 1 {
		n = 1
	}
	var conns []*grpc.ClientConn
	closeAll := func() {
		for _, c := range conns {
			c.Close()
		}
	}
	var out []beastv1.BeastDBServiceClient
	for i := 0; i < n; i++ {
		opts := []grpc.DialOption{grpc.WithTransportCredentials(creds), api.MessageSizeDialOption()}
		if o.Token != "" {
			opts = append(opts, grpc.WithPerRPCCredentials(tokenAuth{o.Token}))
		}
		conn, err := grpc.NewClient(o.Addr, opts...)
		if err != nil {
			closeAll()
			return nil, nil, fmt.Errorf("dial %s: %w", o.Addr, err)
		}
		conns = append(conns, conn)
		out = append(out, beastv1.NewBeastDBServiceClient(conn))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := out[0].Get(ctx, &beastv1.GetRequest{Key: 0}); err != nil {
		closeAll()
		return nil, nil, fmt.Errorf("could not reach BeastDB at %s: %w", o.Addr, err)
	}
	return out, closeAll, nil
}
