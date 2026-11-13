package api

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func openCollEngine(t *testing.T, dir string) *Engine {
	t.Helper()
	e, err := NewEngine(filepath.Join(dir, "db.bin"), filepath.Join(dir, "db.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCatalogCodecRoundTripAndStrictDecode(t *testing.T) {
	in := []Collection{{"users", 0x10, 5}, {"orders", 0x11, 6}}
	out, err := decodeCatalog(encodeCatalog(in))
	if err != nil || len(out) != 2 || out[0] != in[0] || out[1] != in[1] {
		t.Fatalf("round trip: %v %v", out, err)
	}
	raw := encodeCatalog(in)
	for _, bad := range [][]byte{nil, raw[:3], raw[:len(raw)-1], append(append([]byte{}, raw...), 0), append([]byte("XXXX"), raw[4:]...)} {
		if _, err := decodeCatalog(bad); !errors.Is(err, ErrCorruptCatalog) {
			t.Errorf("expected corrupt for %v, got %v", bad, err)
		}
	}
}

func TestCollectionLifecycleAndPersistence(t *testing.T) {
	dir := t.TempDir()
	e := openCollEngine(t, dir)

	// Existing plain keys keep working and do not collide with allocation.
	if err := e.Put(1, []byte("plain")); err != nil {
		t.Fatal(err)
	}
	// Occupy 0x10 so auto-allocation must skip it.
	k, _ := CollectionKey(0x10, 7)
	if err := e.Put(k, []byte("x")); err != nil {
		t.Fatal(err)
	}

	users, err := e.CreateCollection("users", nil)
	if err != nil || users.Prefix != 0x11 {
		t.Fatalf("auto alloc: %+v %v", users, err)
	}
	p01 := uint8(0x01)
	if c, err := e.CreateCollection("legacy-users", &p01); err != nil || c.Prefix != 1 {
		t.Fatalf("adopt explicit: %+v %v", c, err)
	}
	if _, err := e.CreateCollection("users", nil); !errors.Is(err, ErrCollectionExists) {
		t.Fatalf("dup name: %v", err)
	}
	if _, err := e.CreateCollection("other", &p01); !errors.Is(err, ErrCollectionExists) {
		t.Fatalf("dup prefix: %v", err)
	}
	for _, p := range []uint8{0x00, 0x05, 0xFF} {
		p := p
		if _, err := e.CreateCollection("r", &p); !errors.Is(err, ErrReservedPrefix) {
			t.Fatalf("prefix %#x: %v", p, err)
		}
	}
	for _, n := range []string{"", "Users", "-a", "a b", string(make([]byte, 65))} {
		if _, err := e.CreateCollection(n, nil); !errors.Is(err, ErrInvalidCollectionName) {
			t.Fatalf("name %q: %v", n, err)
		}
	}

	// Drop refuses while data exists, then succeeds once emptied.
	uk, _ := CollectionKey(users.Prefix, 1)
	_ = e.Put(uk, []byte("u"))
	if err := e.DropCollection("users"); !errors.Is(err, ErrCollectionNotEmpty) {
		t.Fatalf("drop non-empty: %v", err)
	}
	_ = e.Delete(uk)
	if err := e.DropCollection("users"); err != nil {
		t.Fatal(err)
	}
	if err := e.DropCollection("users"); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("drop twice: %v", err)
	}

	// Survives close/reopen (WAL replay and checkpoint), plain data intact.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e = openCollEngine(t, dir)
	defer e.Close()
	cols, err := e.ListCollections()
	if err != nil || len(cols) != 1 || cols[0].Name != "legacy-users" {
		t.Fatalf("after reopen: %+v %v", cols, err)
	}
	if v, ok, _ := e.Get(1); !ok || string(v) != "plain" {
		t.Fatalf("plain key lost: %q %v", v, ok)
	}
	// Dropping the last entry removes the record entirely.
	if err := e.DropCollection("legacy-users"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := e.Get(CatalogKey); ok {
		t.Fatal("catalog record should be gone")
	}
}

func TestCorruptCatalogIsNeverOverwritten(t *testing.T) {
	e := openCollEngine(t, t.TempDir())
	defer e.Close()
	if err := e.Put(CatalogKey, []byte("garbage")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CreateCollection("a", nil); !errors.Is(err, ErrCorruptCatalog) {
		t.Fatalf("got %v", err)
	}
	if v, _, _ := e.Get(CatalogKey); string(v) != "garbage" {
		t.Fatal("corrupt record was overwritten")
	}
}

func TestCollectionsReadOnlyReplica(t *testing.T) {
	e := openCollEngine(t, t.TempDir())
	defer e.Close()
	if _, err := e.CreateCollection("a", nil); err != nil {
		t.Fatal(err)
	}
	e.SetReadOnly(true)
	if _, err := e.CreateCollection("b", nil); !errors.Is(err, ErrReadOnlyReplica) {
		t.Fatalf("create on replica: %v", err)
	}
	if cols, err := e.ListCollections(); err != nil || len(cols) != 1 {
		t.Fatalf("list on replica: %v %v", cols, err)
	}
}

func TestGRPCCollections(t *testing.T) {
	client, cleanup := setupTestGRPCServer(t)
	defer cleanup()
	ctx := context.Background()

	created, err := client.CreateCollection(ctx, &beastv1.CreateCollectionRequest{Name: "orders"})
	if err != nil || created.Collection.Prefix != 0x10 {
		t.Fatalf("create: %v %v", created, err)
	}
	if _, err := client.CreateCollection(ctx, &beastv1.CreateCollectionRequest{Name: "orders"}); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("dup: %v", err)
	}
	five := uint32(5)
	if _, err := client.CreateCollection(ctx, &beastv1.CreateCollectionRequest{Name: "t", Prefix: &five}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("reserved: %v", err)
	}
	big := uint32(300)
	if _, err := client.CreateCollection(ctx, &beastv1.CreateCollectionRequest{Name: "t", Prefix: &big}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("range: %v", err)
	}
	list, err := client.ListCollections(ctx, &beastv1.ListCollectionsRequest{})
	if err != nil || len(list.Collections) != 1 || list.Collections[0].Name != "orders" {
		t.Fatalf("list: %v %v", list, err)
	}
	k, _ := CollectionKey(0x10, 1)
	if _, err := client.Put(ctx, &beastv1.PutRequest{Key: k, Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DropCollection(ctx, &beastv1.DropCollectionRequest{Name: "orders"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("drop non-empty: %v", err)
	}
	if _, err := client.Delete(ctx, &beastv1.DeleteRequest{Key: k}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DropCollection(ctx, &beastv1.DropCollectionRequest{Name: "orders"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DropCollection(ctx, &beastv1.DropCollectionRequest{Name: "orders"}); status.Code(err) != codes.NotFound {
		t.Fatalf("drop missing: %v", err)
	}
	// The catalog record is protected from the plain key API.
	if _, err := client.Put(ctx, &beastv1.PutRequest{Key: CatalogKey, Value: []byte("x")}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("put catalog key: %v", err)
	}
	if _, err := client.Delete(ctx, &beastv1.DeleteRequest{Key: CatalogKey}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("delete catalog key: %v", err)
	}
	if _, err := client.BatchWrite(ctx, &beastv1.BatchWriteRequest{Operations: []*beastv1.BatchOperation{{OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT, Key: CatalogKey, Value: []byte("x")}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("batch catalog key: %v", err)
	}
}

func TestCollectionRoleMatrix(t *testing.T) {
	const svc = "/beastdb.v1.BeastDBService/"
	ai := NewAuthInterceptor("static", fakeRoleValidator{"v": "viewer", "w": "writer", "a": "admin"})
	cases := map[string]map[string]bool{
		svc + "ListCollections":  {"v": true, "w": true, "a": true},
		svc + "CreateCollection": {"a": true},
		svc + "DropCollection":   {"a": true},
	}
	for m, allowed := range cases {
		for tok := range map[string]bool{"v": true, "w": true, "a": true} {
			err := ai.authorize(bearer(tok), m)
			if allowed[tok] && err != nil || !allowed[tok] && status.Code(err) != codes.PermissionDenied {
				t.Errorf("%s with %s: %v", m, tok, err)
			}
		}
	}
}
