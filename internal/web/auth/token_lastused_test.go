package auth

import (
	"testing"
	"time"
)

func TestTokenLastUsedIsMemoryOnly(t *testing.T) {
	engine := newMockEngine()
	ts := NewTokenStore(engine, nil)

	used, usedTok, _ := ts.Create("used", "viewer", "t")
	_, idleTok, _ := ts.Create("idle", "viewer", "t")

	byID := func(id string) APIToken {
		for _, tok := range ts.List() {
			if tok.ID == id {
				return tok
			}
		}
		t.Fatalf("token %s missing from List", id)
		return APIToken{}
	}

	if byID(usedTok.ID).LastUsedAt != nil {
		t.Fatal("a token that never authenticated must have no last-used time")
	}

	before := time.Now().Add(-time.Second)
	writes := len(engine.data)
	if !ts.Validate(used) {
		t.Fatal("token should validate")
	}
	if len(engine.data) != writes {
		t.Fatal("authenticating must not write to the engine")
	}

	got := byID(usedTok.ID).LastUsedAt
	if got == nil || got.Before(before) {
		t.Fatalf("last-used not recorded: %v", got)
	}
	if byID(idleTok.ID).LastUsedAt != nil {
		t.Fatal("an unrelated token must stay unused")
	}

	// A rejected token must not be recorded.
	if ts.Validate("bst_live_not-a-real-token") {
		t.Fatal("bogus token validated")
	}

	// Revoked tokens drop their tracking.
	if err := ts.Delete(usedTok.ID); err != nil {
		t.Fatal(err)
	}
	for _, tok := range ts.List() {
		if tok.ID == usedTok.ID {
			t.Fatal("revoked token still listed")
		}
	}
}
