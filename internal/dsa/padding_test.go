package dsa

import (
	"testing"
)

// PoorlyAlignedStruct wastes memory due to poor field ordering.
// bool (1) + pad (7) + int64 (8) + bool (1) + pad (7) = 24 bytes.
type PoorlyAlignedStruct struct {
	FlagA bool
	Num   int64
	FlagB bool
}

// WellAlignedStruct orders fields descending by alignment requirement.
// int64 (8) + bool (1) + bool (1) + pad (6) = 16 bytes.
type WellAlignedStruct struct {
	Num   int64
	FlagA bool
	FlagB bool
}

func TestInspectStruct(t *testing.T) {
	poorLayout, err := InspectStruct(PoorlyAlignedStruct{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if poorLayout.TotalSize != 24 {
		t.Fatalf("expected PoorlyAlignedStruct size 24, got %d", poorLayout.TotalSize)
	}
	if poorLayout.WastedBytes != 14 {
		t.Fatalf("expected 14 wasted bytes in PoorlyAlignedStruct, got %d", poorLayout.WastedBytes)
	}

	wellLayout, err := InspectStruct(WellAlignedStruct{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wellLayout.TotalSize != 16 {
		t.Fatalf("expected WellAlignedStruct size 16, got %d", wellLayout.TotalSize)
	}
	if wellLayout.WastedBytes != 6 {
		t.Fatalf("expected 6 wasted bytes in WellAlignedStruct, got %d", wellLayout.WastedBytes)
	}
}

func TestInspectStructInvalid(t *testing.T) {
	_, err := InspectStruct(42)
	if err == nil {
		t.Fatalf("expected error when inspecting primitive int")
	}

	_, err = InspectStruct(nil)
	if err == nil {
		t.Fatalf("expected error when inspecting nil")
	}
}
