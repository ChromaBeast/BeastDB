package replication

import (
	"context"
	"sync"
	"testing"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
)

func TestReplicasReportsConnectedAndAcked(t *testing.T) {
	l := NewLeaderServer("unused.wal")
	if got := l.Replicas(); len(got) != 0 {
		t.Fatalf("expected no replicas, got %+v", got)
	}
	_, unsub := l.broadcaster.Subscribe("b", 4)
	if _, err := l.Ack(context.Background(), &beastv1.ReplicationAck{ReplicaId: "a", AcknowledgedLsn: 7}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Ack(context.Background(), &beastv1.ReplicationAck{ReplicaId: "b", AcknowledgedLsn: 3}); err != nil {
		t.Fatal(err)
	}
	got := l.Replicas()
	if len(got) != 2 || got[0].ID != "a" || got[0].Connected || got[0].AckedLSN != 7 ||
		got[1].ID != "b" || !got[1].Connected || got[1].AckedLSN != 3 {
		t.Fatalf("unexpected replicas: %+v", got)
	}
	unsub()
	if got := l.Replicas(); got[1].Connected {
		t.Fatalf("b should be disconnected after unsubscribe: %+v", got)
	}
}

func TestReplicasConcurrentAccess(t *testing.T) {
	l := NewLeaderServer("unused.wal")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = l.Ack(context.Background(), &beastv1.ReplicationAck{ReplicaId: "r", AcknowledgedLsn: uint64(j)})
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = l.Replicas()
			}
		}()
	}
	wg.Wait()
}
