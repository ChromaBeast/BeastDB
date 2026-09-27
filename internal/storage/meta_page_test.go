package storage

import (
	"testing"
)

func TestMetaPageEncodeDecode(t *testing.T) {
	buf := make([]byte, PageSize)

	original := MetaData{
		Magic:             MetaMagic,
		Version:           MetaVersion,
		RootPageID:        42,
		ActiveDataPageID:  7,
		LastCheckpointLSN: 1005,
	}

	EncodeMeta(buf, original)

	decoded, err := DecodeMeta(buf)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if decoded != original {
		t.Fatalf("decoded meta %+v does not match original %+v", decoded, original)
	}
}

func TestMetaPageInvalidMagic(t *testing.T) {
	buf := make([]byte, PageSize)
	original := MetaData{
		Magic:   0xDEADBEEF,
		Version: MetaVersion,
	}
	EncodeMeta(buf, original)

	_, err := DecodeMeta(buf)
	if err != ErrInvalidMetaMagic {
		t.Fatalf("expected ErrInvalidMetaMagic, got %v", err)
	}
}

func TestMetaPageInvalidVersion(t *testing.T) {
	buf := make([]byte, PageSize)
	original := MetaData{
		Magic:   MetaMagic,
		Version: 99,
	}
	EncodeMeta(buf, original)

	_, err := DecodeMeta(buf)
	if err != ErrInvalidMetaVersion {
		t.Fatalf("expected ErrInvalidMetaVersion, got %v", err)
	}
}
