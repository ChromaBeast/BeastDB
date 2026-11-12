package api

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeRoleValidator map[string]string

func (f fakeRoleValidator) Validate(raw string) bool { _, ok := f[raw]; return ok }
func (f fakeRoleValidator) ValidateRole(raw string) (string, bool) {
	r, ok := f[raw]
	return r, ok
}

type legacyValidator struct{ tok string }

func (l legacyValidator) Validate(raw string) bool { return raw == l.tok }

func bearer(tok string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+tok))
}

func TestAuthorizeMethodRoleMatrix(t *testing.T) {
	const svc = "/beastdb.v1.BeastDBService/"
	const rep = "/beastdb.v1.ReplicationService/"
	v := fakeRoleValidator{"v": "viewer", "w": "writer", "a": "admin", "r": "replication", "x": "bogus"}
	ai := NewAuthInterceptor("static", v)

	allow := map[string]map[string]bool{
		svc + "Get":        {"viewer": true, "writer": true, "admin": true},
		svc + "Scan":       {"viewer": true, "writer": true, "admin": true},
		svc + "Put":        {"writer": true, "admin": true},
		svc + "Delete":     {"writer": true, "admin": true},
		svc + "BatchWrite": {"writer": true, "admin": true},
		rep + "StreamWAL":  {"replication": true, "admin": true},
		rep + "Ack":        {"replication": true, "admin": true},
	}
	tokens := map[string]string{"viewer": "v", "writer": "w", "admin": "a", "replication": "r", "bogus": "x", "static": "static"}
	for method, ok := range allow {
		for role, tok := range tokens {
			want := ok[role] || role == "static" && ok["admin"]
			err := ai.authorize(bearer(tok), method)
			if want && err != nil {
				t.Errorf("%s %s: expected allow, got %v", role, method, err)
			}
			if !want {
				if status.Code(err) != codes.PermissionDenied {
					t.Errorf("%s %s: expected PermissionDenied, got %v", role, method, err)
				}
			}
		}
	}
}

func TestAuthorizeDenyByDefaultAndCodes(t *testing.T) {
	ai := NewAuthInterceptor("static", fakeRoleValidator{"a": "admin"})
	for _, tok := range []string{"static", "a"} {
		if c := status.Code(ai.authorize(bearer(tok), "/beastdb.v1.BeastDBService/Unknown")); c != codes.PermissionDenied {
			t.Errorf("unknown method with %s: got %v", tok, c)
		}
	}
	if c := status.Code(ai.authorize(bearer("nope"), "/beastdb.v1.BeastDBService/Get")); c != codes.Unauthenticated {
		t.Errorf("bad token: got %v", c)
	}
	if c := status.Code(ai.authorize(context.Background(), "/beastdb.v1.BeastDBService/Get")); c != codes.Unauthenticated {
		t.Errorf("no metadata: got %v", c)
	}
}

func TestAuthorizeLegacyValidatorIsAdmin(t *testing.T) {
	ai := NewAuthInterceptor("", legacyValidator{"legacy"})
	if err := ai.authorize(bearer("legacy"), "/beastdb.v1.BeastDBService/Put"); err != nil {
		t.Fatalf("legacy validator should act as admin: %v", err)
	}
	if c := status.Code(ai.authorize(bearer("bad"), "/beastdb.v1.BeastDBService/Put")); c != codes.Unauthenticated {
		t.Fatalf("got %v", c)
	}
}
