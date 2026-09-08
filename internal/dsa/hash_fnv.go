package dsa

const (
	// FNV-1a 64-bit constants
	fnvOffsetBasis64 uint64 = 14695981039346656037
	fnvPrime64       uint64 = 1099511628211
)

// HashString computes the 64-bit FNV-1a hash of a string with zero heap allocations.
func HashString(s string) uint64 {
	hash := fnvOffsetBasis64
	for i := 0; i < len(s); i++ {
		hash ^= uint64(s[i])
		hash *= fnvPrime64
	}
	return hash
}

// HashBytes computes the 64-bit FNV-1a hash of a raw byte slice with zero allocations.
func HashBytes(b []byte) uint64 {
	hash := fnvOffsetBasis64
	for i := 0; i < len(b); i++ {
		hash ^= uint64(b[i])
		hash *= fnvPrime64
	}
	return hash
}
