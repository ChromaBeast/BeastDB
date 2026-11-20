package index

import (
	"math/rand"
	"sort"
	"testing"
)

func TestValueIndexAgainstModel(t *testing.T) {
	for _, kind := range []int{KindInt, KindString} {
		r := rand.New(rand.NewSource(7))
		x := NewValueIndex(kind)
		model := map[uint64]int64{}
		val := func(i int64) FieldValue {
			if kind == KindInt {
				return FieldValue{Int: i}
			}
			return FieldValue{Str: []byte{byte('a' + i/26), byte('a' + i%26)}}
		}
		for n := 0; n < 20000; n++ {
			k := uint64(r.Intn(3000))
			if r.Intn(4) == 0 {
				x.Remove(k)
				delete(model, k)
			} else {
				v := int64(r.Intn(600)) // 600 = 23*26+2: two-letter strings stay distinct
				x.Set(k, val(v))
				model[k] = v
			}
		}
		if x.Len() != len(model) {
			t.Fatalf("kind %d: len %d want %d", kind, x.Len(), len(model))
		}
		check := func(lo, hi int64, useLo, useHi bool) {
			var want []uint64
			for k, v := range model {
				if (!useLo || v >= lo) && (!useHi || v <= hi) {
					want = append(want, k)
				}
			}
			sort.Slice(want, func(i, j int) bool {
				if model[want[i]] != model[want[j]] {
					return model[want[i]] < model[want[j]]
				}
				return want[i] < want[j]
			})
			var blo, bhi Bound
			if useLo {
				blo = Bound{Set: true, Int: lo, Str: string(val(lo).Str)}
			}
			if useHi {
				bhi = Bound{Set: true, Int: hi, Str: string(val(hi).Str)}
			}
			got := x.Range(blo, bhi)
			if len(got) != len(want) {
				t.Fatalf("kind %d [%d,%d] %v %v: got %d want %d", kind, lo, hi, useLo, useHi, len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("kind %d: order differs at %d", kind, i)
				}
			}
		}
		check(0, 0, false, false)
		check(100, 100, true, true)
		check(50, 300, true, true)
		check(400, 0, true, false)
		check(0, 120, false, true)
		check(700, 800, true, true)
	}
}
