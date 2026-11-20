package index

import "unicode/utf8"

// Field value kinds understood by ExtractField.
const (
	KindInt    = 1
	KindString = 2
)

// MaxIndexedString is the longest string value (after unescaping) that is indexed.
const MaxIndexedString = 256

// FieldValue is one extracted JSON member value. Str is only set for KindString
// and aliases the input when the string has no escapes; callers that keep it
// must copy.
type FieldValue struct {
	Int int64
	Str []byte
}

// ExtractField finds the member at path (segments of object member names) in
// the JSON document doc and returns it when it has the wanted kind. ok is
// false when doc is not a JSON object, the path does not resolve, the value
// has another type, an integer does not fit int64, or a string exceeds
// MaxIndexedString. It does not allocate for documents whose names and
// strings contain no escapes. It is a scanner, not a validator: malformed
// input yields ok=false or a best-effort answer for the prefix it could read.
func ExtractField(doc []byte, path [][]byte, kind int) (v FieldValue, ok bool) {
	i := skipWS(doc, 0)
	for _, seg := range path {
		if i >= len(doc) || doc[i] != '{' {
			return v, false
		}
		i++
		found := false
		for {
			i = skipWS(doc, i)
			if i >= len(doc) {
				return v, false
			}
			if doc[i] == '}' {
				return v, false
			}
			if doc[i] != '"' {
				return v, false
			}
			name, next, nameOK := readString(doc, i)
			if !nameOK {
				return v, false
			}
			i = skipWS(doc, next)
			if i >= len(doc) || doc[i] != ':' {
				return v, false
			}
			i = skipWS(doc, i+1)
			if equalSegment(name, seg) {
				found = true
				break
			}
			var skipped bool
			if i, skipped = skipValue(doc, i); !skipped {
				return v, false
			}
			i = skipWS(doc, i)
			if i < len(doc) && doc[i] == ',' {
				i++
				continue
			}
			return v, false
		}
		if !found {
			return v, false
		}
	}
	return parseScalar(doc, i, kind)
}

// segment is the raw (still escaped) JSON text of a member name; paths only
// contain [A-Za-z0-9_] so an escaped name equals the segment only if it has
// no escapes at all, except for pathological \u00XX spellings, which we do
// not match (they simply do not resolve).
func equalSegment(name rawString, seg []byte) bool {
	if name.escaped {
		return false
	}
	if len(name.b) != len(seg) {
		return false
	}
	for i := range seg {
		if name.b[i] != seg[i] {
			return false
		}
	}
	return true
}

type rawString struct {
	b       []byte
	escaped bool
}

// readString reads the JSON string starting at doc[i]=='"'. It returns the raw
// bytes between the quotes and the index after the closing quote.
func readString(doc []byte, i int) (rawString, int, bool) {
	start := i + 1
	esc := false
	for j := start; j < len(doc); j++ {
		switch doc[j] {
		case '\\':
			esc = true
			j++ // skip the escaped byte
		case '"':
			return rawString{b: doc[start:j], escaped: esc}, j + 1, true
		}
	}
	return rawString{}, len(doc), false
}

func parseScalar(doc []byte, i int, kind int) (FieldValue, bool) {
	var v FieldValue
	if i >= len(doc) {
		return v, false
	}
	switch kind {
	case KindString:
		if doc[i] != '"' {
			return v, false
		}
		s, _, ok := readString(doc, i)
		if !ok {
			return v, false
		}
		if !s.escaped {
			if len(s.b) > MaxIndexedString {
				return v, false
			}
			v.Str = s.b
			return v, true
		}
		out, ok := unescape(s.b)
		if !ok || len(out) > MaxIndexedString {
			return v, false
		}
		v.Str = out
		return v, true
	case KindInt:
		j := i
		neg := false
		if doc[j] == '-' {
			neg = true
			j++
		}
		if j >= len(doc) || doc[j] < '0' || doc[j] > '9' {
			return v, false
		}
		if doc[j] == '0' && j+1 < len(doc) && doc[j+1] >= '0' && doc[j+1] <= '9' {
			return v, false // leading zero is not valid JSON
		}
		var n uint64
		for ; j < len(doc) && doc[j] >= '0' && doc[j] <= '9'; j++ {
			d := uint64(doc[j] - '0')
			if n > (1<<63)/10 || (n == (1<<63)/10 && d > (1<<63)%10) {
				return v, false
			}
			n = n*10 + d
		}
		if j < len(doc) && (doc[j] == '.' || doc[j] == 'e' || doc[j] == 'E') {
			return v, false // not written as an integer
		}
		if neg {
			v.Int = -int64(n) // n <= 2^63; the negation of 2^63 wraps to MinInt64, which is correct
		} else {
			if n > 1<<63-1 {
				return v, false
			}
			v.Int = int64(n)
		}
		return v, true
	}
	return v, false
}

// unescape decodes JSON string escapes. It allocates; escaped strings are rare.
func unescape(b []byte) ([]byte, bool) {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c != '\\' {
			out = append(out, c)
			continue
		}
		i++
		if i >= len(b) {
			return nil, false
		}
		switch b[i] {
		case '"', '\\', '/':
			out = append(out, b[i])
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			r, n, ok := hex4(b, i+1)
			if !ok {
				return nil, false
			}
			i += n
			if r >= 0xD800 && r < 0xDC00 { // surrogate pair
				if i+6 < len(b) && b[i+1] == '\\' && b[i+2] == 'u' {
					r2, _, ok2 := hex4(b, i+3)
					if ok2 && r2 >= 0xDC00 && r2 < 0xE000 {
						r = 0x10000 + (r-0xD800)<<10 + (r2 - 0xDC00)
						i += 6
					} else {
						r = utf8.RuneError
					}
				} else {
					r = utf8.RuneError
				}
			} else if r >= 0xDC00 && r < 0xE000 {
				r = utf8.RuneError
			}
			out = utf8.AppendRune(out, r)
		default:
			return nil, false
		}
	}
	return out, true
}

func hex4(b []byte, i int) (rune, int, bool) {
	if i+4 > len(b) {
		return 0, 0, false
	}
	var r rune
	for _, c := range b[i : i+4] {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, 0, false
		}
	}
	return r, 4, true
}

func skipWS(doc []byte, i int) int {
	for i < len(doc) {
		switch doc[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

// skipValue skips one JSON value starting at doc[i] and returns the index
// after it.
func skipValue(doc []byte, i int) (int, bool) {
	if i >= len(doc) {
		return i, false
	}
	switch doc[i] {
	case '"':
		_, next, ok := readString(doc, i)
		return next, ok
	case '{', '[':
		depth := 0
		for j := i; j < len(doc); j++ {
			switch doc[j] {
			case '"':
				_, next, ok := readString(doc, j)
				if !ok {
					return len(doc), false
				}
				j = next - 1
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return j + 1, true
				}
			}
		}
		return len(doc), false
	default: // number, true, false, null
		j := i
		for j < len(doc) {
			c := doc[j]
			if c == ',' || c == '}' || c == ']' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
				break
			}
			j++
		}
		return j, j > i
	}
}
