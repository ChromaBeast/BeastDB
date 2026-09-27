package index

import (
	"strings"
	"unicode"
)

const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// HashToken computes the 64-bit FNV-1a hash of a lowercase normalized token.
func HashToken(token string) uint64 {
	var h uint64 = fnvOffset64
	for i := 0; i < len(token); i++ {
		c := token[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		h ^= uint64(c)
		h *= fnvPrime64
	}
	return h
}

// TokenizeWords splits input text into unique normalized 64-bit word hashes.
func TokenizeWords(text string) []uint64 {
	if len(text) == 0 {
		return nil
	}

	seen := make(map[uint64]struct{})
	var tokens []uint64

	start := -1
	for i, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if start < 0 {
				start = i
			}
		} else {
			if start >= 0 {
				word := strings.ToLower(text[start:i])
				if len(word) >= 2 {
					h := HashToken(word)
					if _, exists := seen[h]; !exists {
						seen[h] = struct{}{}
						tokens = append(tokens, h)
					}
				}
				start = -1
			}
		}
	}

	if start >= 0 {
		word := strings.ToLower(text[start:])
		if len(word) >= 2 {
			h := HashToken(word)
			if _, exists := seen[h]; !exists {
				seen[h] = struct{}{}
				tokens = append(tokens, h)
			}
		}
	}

	return tokens
}
