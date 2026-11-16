package storage

import "testing"

// A slot that points outside the page must be an error, not a panic.
func TestGetTupleRejectsOutOfBoundsSlot(t *testing.T) {
	p := NewSlottedPage(1)
	id, err := p.InsertTuple([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	WriteSlot(p.Data(), id, Slot{Offset: PageSize - 2, Length: 100})
	if _, err := p.GetTuple(id); err != ErrCorruptPage {
		t.Fatalf("got %v, want ErrCorruptPage", err)
	}
	WriteSlot(p.Data(), id, Slot{Offset: 3, Length: 4})
	if _, err := p.GetTuple(id); err != ErrCorruptPage {
		t.Fatalf("slot inside the header: got %v, want ErrCorruptPage", err)
	}
}
