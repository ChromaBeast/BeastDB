package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"testing"
)

func testCipher(t *testing.T) *PageCipher { return testCipherB(t) }

func testCipherB(t testing.TB) *PageCipher {
	t.Helper()
	block, _ := aes.NewCipher(make([]byte, 32))
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	return NewPageCipher(aead, 1)
}

func TestPageSealOpenRoundTrip(t *testing.T) {
	c := testCipher(t)
	plain := make([]byte, PageSize)
	for i := range PageDataEnd {
		plain[i] = byte(i * 7)
	}
	sealed := make([]byte, PageSize)
	if err := c.Seal(sealed, plain, 9); err != nil {
		t.Fatal(err)
	}
	if string(sealed[:PageDataEnd]) == string(plain[:PageDataEnd]) {
		t.Fatal("sealed page equals plaintext")
	}
	if !IsEncryptedPage(sealed) || IsEncryptedPage(plain) {
		t.Fatal("IsEncryptedPage wrong")
	}
	// The same plaintext sealed twice differs (fresh nonce).
	again := make([]byte, PageSize)
	_ = c.Seal(again, plain, 9)
	if string(again) == string(sealed) {
		t.Fatal("nonce reused")
	}
	// Wrong page ID fails: a page moved to another position is rejected.
	moved := append([]byte(nil), sealed...)
	if err := c.Open(moved, 10); !errors.Is(err, ErrPageAuth) {
		t.Fatalf("moved page: %v", err)
	}
	if err := c.Open(sealed, 9); err != nil {
		t.Fatal(err)
	}
	if string(sealed[:PageDataEnd]) != string(plain[:PageDataEnd]) {
		t.Fatal("round trip mismatch")
	}
	if err := c.Open(make([]byte, PageSize), 0); !errors.Is(err, ErrPageAuth) {
		t.Fatalf("all-zero page: %v", err)
	}
}

func TestSlottedPageHonoursReservedTail(t *testing.T) {
	p := NewSlottedPageEnd(1, PageDataEnd)
	if got := p.FreeSpace(); got != PageDataEnd-PageHeaderSize {
		t.Fatalf("free space %d", got)
	}
	for {
		if _, err := p.InsertTuple(make([]byte, 100)); err != nil {
			break
		}
	}
	p.Compact()
	if h := p.Header(); int(h.Upper) > PageDataEnd {
		t.Fatalf("Upper %d past reserve", h.Upper)
	}
	for _, b := range p.Data()[PageDataEnd:] {
		if b != 0 {
			t.Fatal("tuple data written into the reserved tail")
		}
	}
	if _, err := p.InsertTuple(make([]byte, 4000)); err == nil {
		// an empty page must still take MaxRecordSize
		t.Log("page was not empty; checking fresh page below")
	}
	fresh := NewSlottedPageEnd(2, PageDataEnd)
	if _, err := fresh.InsertTuple(make([]byte, 4000)); err != nil {
		t.Fatalf("MaxRecordSize must fit an encrypted page: %v", err)
	}
}
