package index

import (
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"testing"
)

func path(p string) [][]byte {
	var out [][]byte
	for _, s := range strings.Split(p, ".") {
		out = append(out, []byte(s))
	}
	return out
}

func TestExtractFieldBasics(t *testing.T) {
	doc := []byte(` { "a" : 1, "skip": {"x":[1,2,{"a":9}],"s":"}\"{"}, "name":"bobé\n", "n":{"age":-42,"f":1.5}, "big":9223372036854775808, "min":-9223372036854775808 } `)
	cases := []struct {
		path string
		kind int
		ok   bool
		i    int64
		s    string
	}{
		{"a", KindInt, true, 1, ""},
		{"name", KindString, true, 0, "bobé\n"},
		{"n.age", KindInt, true, -42, ""},
		{"n.f", KindInt, false, 0, ""},
		{"big", KindInt, false, 0, ""},
		{"min", KindInt, true, math.MinInt64, ""},
		{"a", KindString, false, 0, ""},
		{"name", KindInt, false, 0, ""},
		{"missing", KindInt, false, 0, ""},
		{"n.age.deeper", KindInt, false, 0, ""},
	}
	for _, c := range cases {
		v, ok := ExtractField(doc, path(c.path), c.kind)
		if ok != c.ok || (ok && (v.Int != c.i || string(v.Str) != c.s)) {
			t.Errorf("%s: got %v %+v want ok=%v %d %q", c.path, ok, v, c.ok, c.i, c.s)
		}
	}
	for _, bad := range []string{"", "[1]", "42", `{"a":`, `{"a"`, `{`, `{"a":01}`, `{"a":"x`} {
		if v, ok := ExtractField([]byte(bad), path("a"), KindInt); ok {
			t.Errorf("%q: unexpectedly ok %+v", bad, v)
		}
	}
	long := `{"s":"` + strings.Repeat("x", MaxIndexedString+1) + `"}`
	if _, ok := ExtractField([]byte(long), path("s"), KindString); ok {
		t.Error("over-long string indexed")
	}
}

func TestExtractFieldNoAlloc(t *testing.T) {
	doc := []byte(`{"id":7,"meta":{"tags":["a","b"],"city":"Oslo"},"age":31}`)
	p := path("meta.city")
	n := testing.AllocsPerRun(100, func() { ExtractField(doc, p, KindString) })
	if n != 0 {
		t.Fatalf("allocs = %v", n)
	}
}

// randomDoc builds a random JSON value and the reference answers.
func randValue(r *rand.Rand, depth int) any {
	switch r.Intn(7) {
	case 0:
		return r.Int63() - r.Int63()
	case 1:
		return r.Float64() * 100
	case 2:
		return strings.Repeat("é\"\\z", r.Intn(4))
	case 3:
		return nil
	case 4:
		return r.Intn(2) == 0
	case 5:
		if depth > 0 {
			a := make([]any, r.Intn(3))
			for i := range a {
				a[i] = randValue(r, depth-1)
			}
			return a
		}
		return 1
	default:
		if depth > 0 {
			m := map[string]any{}
			for i := 0; i < r.Intn(4); i++ {
				m[string(rune('a'+r.Intn(4)))] = randValue(r, depth-1)
			}
			return m
		}
		return "x"
	}
}

func TestExtractFieldMatchesEncodingJSON(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 5000; n++ {
		m := map[string]any{}
		for i := 0; i < 1+r.Intn(4); i++ {
			m[string(rune('a'+r.Intn(4)))] = randValue(r, 3)
		}
		raw, _ := json.Marshal(m)
		if r.Intn(2) == 0 {
			raw, _ = json.MarshalIndent(m, "", "  ")
		}
		p1 := string(rune('a' + r.Intn(4)))
		p2 := string(rune('a' + r.Intn(4)))
		for _, pstr := range []string{p1, p1 + "." + p2} {
			var want any = m
			ok := true
			for _, seg := range strings.Split(pstr, ".") {
				mm, isMap := want.(map[string]any)
				if !isMap {
					ok = false
					break
				}
				want, ok = mm[seg]
				if !ok {
					break
				}
			}
			var dec map[string]any
			d := json.NewDecoder(strings.NewReader(string(raw)))
			d.UseNumber()
			_ = d.Decode(&dec)
			var wantInt int64
			intOK := false
			if ok {
				// integers: re-marshal text of the value to see if written as an integer
				b, _ := json.Marshal(want)
				var num json.Number
				if json.Unmarshal(b, &num) == nil {
					if x, err := num.Int64(); err == nil && !strings.ContainsAny(string(b), ".eE") {
						wantInt, intOK = x, true
					}
				}
			}
			v, got := ExtractField(raw, path(pstr), KindInt)
			if got != intOK || (got && v.Int != wantInt) {
				t.Fatalf("doc %s path %s: got %v %d want %v %d", raw, pstr, got, v.Int, intOK, wantInt)
			}
			ws, isStr := want.(string)
			isStr = isStr && ok && len(ws) <= MaxIndexedString
			vs, gotS := ExtractField(raw, path(pstr), KindString)
			if gotS != isStr || (gotS && string(vs.Str) != ws) {
				t.Fatalf("doc %s path %s: string got %v %q want %v %q", raw, pstr, gotS, vs.Str, isStr, ws)
			}
		}
	}
}
