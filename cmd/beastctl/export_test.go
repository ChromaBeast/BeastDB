package main

import (
	"testing"
)

func TestParsePartition(t *testing.T) {
	tests := []struct {
		input    string
		expected uint8
		wantErr  bool
	}{
		{"0x01", 1, false},
		{"0x10", 16, false},
		{"0xFF", 255, false},
		{"1", 1, false},
		{"16", 16, false},
		{"255", 255, false},
		{"256", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		got, err := parsePartition(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parsePartition(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.expected {
			t.Errorf("parsePartition(%q) = %d, expected %d", tt.input, got, tt.expected)
		}
	}
}
