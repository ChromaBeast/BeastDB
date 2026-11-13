package replication

import (
	"context"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// tnode is one in-process BeastDB node: engine, replication server and cluster.
type tnode struct {
	t       *testing.T
	id      string
	dir     string
	engine  *api.Engine
	leader  *LeaderServer
	cluster *Cluster
	srv     *grpc.Server
	addr    string
	closeMu sync.Once
}

// startTNode opens (or reopens) a node in dir and serves gRPC on addr ("" picks a port).
func startTNode(t *testing.T, id, dir string, flag Role, addr string) *tnode {
	t.Helper()
	n := &tnode{t: t, id: id, dir: dir}
	eng, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 50)
	if err != nil {
		t.Fatalf("%s: engine: %v", id, err)
	}
	n.engine = eng
	n.leader = NewLeaderServer(eng.WALPath())
	n.cluster, err = NewCluster(eng, dir, id, flag, n.leader)
	if err != nil {
		_ = eng.Close()
		t.Fatalf("%s: cluster: %v", id, err)
	}
	n.serve(addr)
	t.Cleanup(n.stop)
	return n
}

func (n *tnode) serve(addr string) {
	n.t.Helper()
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	var lis net.Listener
	var err error
	for i := 0; i < 50; i++ { // a just-closed port can take a moment to free up
		if lis, err = net.Listen("tcp", addr); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		n.t.Fatalf("%s: listen %s: %v", n.id, addr, err)
	}
	n.srv = grpc.NewServer()
	n.leader.RegisterService(n.srv)
	n.addr = lis.Addr().String()
	go func() { _ = n.srv.Serve(lis) }()
}

// partition stops serving gRPC but keeps the engine running, like a network
// partition: clients of the node on the far side can no longer reach it.
func (n *tnode) partition() { n.srv.Stop() }

// heal serves the same node again at the same address.
func (n *tnode) heal() { n.serve(n.addr) }

// stop shuts the node down (engine flushed), as a clean process exit would.
func (n *tnode) stop() {
	n.closeMu.Do(func() {
		n.srv.Stop()
		_ = n.engine.Close()
	})
}

// follow dials leaderAddr and runs this node as its follower until the test ends.
func (n *tnode) follow(leaderAddr string) *FollowerClient {
	n.t.Helper()
	conn, err := grpc.NewClient(leaderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		n.t.Fatalf("%s: dial: %v", n.id, err)
	}
	n.t.Cleanup(func() { _ = conn.Close() })
	f := NewFollowerClient(n.id, n.engine, conn)
	ctx, cancel := context.WithCancel(context.Background())
	n.t.Cleanup(cancel)
	n.cluster.RunFollower(ctx, f)
	return f
}

// hasKey reports whether key holds want on this node.
func (n *tnode) hasKey(key uint64, want string) bool {
	v, ok, err := n.engine.Get(key)
	return err == nil && ok && string(v) == want
}

// caughtUp waits until the follower f has applied the leader's current LSN.
func caughtUp(t *testing.T, f *FollowerClient, leader *tnode) {
	t.Helper()
	if !waitUntil(t, func() bool { return f.LastAppliedLSN() >= leader.engine.CurrentLSN() }) {
		t.Fatalf("follower stuck at LSN %d, leader at %d", f.LastAppliedLSN(), leader.engine.CurrentLSN())
	}
}

// fastFencing shortens promotion timers for the duration of a test.
func fastFencing(t *testing.T) {
	t.Helper()
	oldRetry, oldFresh := fenceRetryEvery, leaderFreshness
	fenceRetryEvery = 50 * time.Millisecond
	t.Cleanup(func() { fenceRetryEvery, leaderFreshness = oldRetry, oldFresh })
}

// listen opens a TCP listener on a free local port.
func listen(t *testing.T) (net.Listener, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return lis, lis.Addr().String()
}

// NewClusterForTest runs only the startup role check against dir, without an engine.
func NewClusterForTest(t *testing.T, dir string, flag Role) (Role, error) {
	t.Helper()
	st, err := LoadStateStore(dir)
	if err != nil {
		return "", err
	}
	return ResolveRole(st.Get(), flag)
}
