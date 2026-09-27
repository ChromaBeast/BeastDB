package index

import (
	"reflect"
	"testing"
)

func TestSecondaryIndexBasic(t *testing.T) {
	idx := NewSecondaryIndex()

	tokens1 := TokenizeWords("apple banana cherry")
	tokens2 := TokenizeWords("banana date")
	tokens3 := TokenizeWords("cherry banana elderberry")

	idx.Index(1, tokens1)
	idx.Index(2, tokens2)
	idx.Index(3, tokens3)

	// "banana" should match 1, 2, 3
	bananaHash := HashToken("banana")
	matches := idx.Lookup(bananaHash)
	if !reflect.DeepEqual(matches, []uint64{1, 2, 3}) {
		t.Fatalf("expected [1 2 3], got %v", matches)
	}

	// "cherry" should match 1, 3
	cherryHash := HashToken("cherry")
	matches = idx.Lookup(cherryHash)
	if !reflect.DeepEqual(matches, []uint64{1, 3}) {
		t.Fatalf("expected [1 3], got %v", matches)
	}

	// Intersection of "banana" AND "cherry" should be [1, 3]
	queryTokens := TokenizeWords("banana cherry")
	intersected := idx.LookupIntersect(queryTokens)
	if !reflect.DeepEqual(intersected, []uint64{1, 3}) {
		t.Fatalf("expected [1 3] for intersection, got %v", intersected)
	}

	// Unindex 1
	idx.Unindex(1)
	matches = idx.Lookup(cherryHash)
	if !reflect.DeepEqual(matches, []uint64{3}) {
		t.Fatalf("expected [3] after unindexing 1, got %v", matches)
	}
}

func TestSecondaryIndexUpdate(t *testing.T) {
	idx := NewSecondaryIndex()

	idx.Index(1, TokenizeWords("red green"))
	idx.Index(1, TokenizeWords("blue green")) // update replaces "red" with "blue"

	redMatches := idx.Lookup(HashToken("red"))
	if len(redMatches) != 0 {
		t.Fatalf("expected 0 matches for old token red, got %v", redMatches)
	}

	blueMatches := idx.Lookup(HashToken("blue"))
	if !reflect.DeepEqual(blueMatches, []uint64{1}) {
		t.Fatalf("expected [1] for blue, got %v", blueMatches)
	}
}
