package storage

import "testing"

func BenchmarkPageSeal(b *testing.B) {
	c := testCipherB(b)
	plain, dst := make([]byte, PageSize), make([]byte, PageSize)
	b.SetBytes(PageSize)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := c.Seal(dst, plain, uint64(i)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPageOpen(b *testing.B) {
	c := testCipherB(b)
	sealed, work := make([]byte, PageSize), make([]byte, PageSize)
	_ = c.Seal(sealed, make([]byte, PageSize), 7)
	b.SetBytes(PageSize)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		copy(work, sealed)
		if err := c.Open(work, 7); err != nil {
			b.Fatal(err)
		}
	}
}
