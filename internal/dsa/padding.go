package dsa

import (
	"fmt"
	"reflect"
)

// CacheLineSize represents standard x86_64 / ARM64 L1 cache line size (64 bytes).
const CacheLineSize = 64

// CacheLinePad is used to prevent false sharing between CPU cores on concurrent counters.
type CacheLinePad [CacheLineSize]byte

// FieldLayout holds memory layout diagnostics for a single struct field.
type FieldLayout struct {
	Name         string
	TypeName     string
	Size         uintptr
	Offset       uintptr
	Align        uintptr
	PaddingAfter uintptr
}

// StructLayout represents the full memory footprint of a struct.
type StructLayout struct {
	TypeName     string
	TotalSize    uintptr
	Alignment    uintptr
	WastedBytes  uintptr
	Fields       []FieldLayout
}

// InspectStruct inspects the memory alignment and padding overhead of any struct.
func InspectStruct(val any) (StructLayout, error) {
	t := reflect.TypeOf(val)
	if t == nil {
		return StructLayout{}, fmt.Errorf("cannot inspect nil value")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return StructLayout{}, fmt.Errorf("expected struct, got %s", t.Kind())
	}

	layout := StructLayout{
		TypeName:  t.Name(),
		TotalSize: t.Size(),
		Alignment: uintptr(t.Align()),
		Fields:    make([]FieldLayout, t.NumField()),
	}

	var wasted uintptr
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fl := FieldLayout{
			Name:     f.Name,
			TypeName: f.Type.String(),
			Size:     f.Type.Size(),
			Offset:   f.Offset,
			Align:    uintptr(f.Type.Align()),
		}

		if i < t.NumField()-1 {
			nextField := t.Field(i + 1)
			fl.PaddingAfter = nextField.Offset - (f.Offset + f.Type.Size())
		} else {
			fl.PaddingAfter = t.Size() - (f.Offset + f.Type.Size())
		}
		wasted += fl.PaddingAfter
		layout.Fields[i] = fl
	}

	layout.WastedBytes = wasted
	return layout, nil
}
