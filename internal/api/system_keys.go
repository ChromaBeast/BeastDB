package api

import "errors"

// SystemPartition is the key partition (top 8 bits of a key) reserved for the
// server's own records, such as console users and API tokens. Client-facing
// surfaces (gRPC and the web console's data routes) refuse to read or write
// it; otherwise any client allowed to write data could rewrite credentials.
const SystemPartition uint8 = 0x05

// ErrSystemKey is returned when a client addresses a key in SystemPartition.
var ErrSystemKey = errors.New("key is in the reserved system partition")

// IsSystemKey reports whether key lies in the reserved system partition.
func IsSystemKey(key uint64) bool { return uint8(key>>56) == SystemPartition }
