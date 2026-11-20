package wal

import "encoding/binary"

// EpochMark is a leader epoch beginning at LSN: the record at LSN and every
// record after it, up to the next mark, was written by the leader of Epoch.
type EpochMark struct {
	LSN   uint64 `json:"lsn"`
	Epoch uint64 `json:"epoch"`
}

// EncodeEpoch is the value of an OpEpoch record.
func EncodeEpoch(epoch uint64) []byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], epoch)
	return b[:]
}

// DecodeEpoch reads the value of an OpEpoch record.
func DecodeEpoch(value []byte) (uint64, bool) {
	if len(value) != 8 {
		return 0, false
	}
	return binary.LittleEndian.Uint64(value), true
}

// EpochMarks lists the epoch markers in the logs at paths (oldest generation
// first), in LSN order. Missing files are skipped. A marker whose value is
// malformed is an error: the log is not one this code wrote.
func EpochMarks(c *Cipher, paths ...string) ([]EpochMark, error) {
	var out []EpochMark
	var last uint64
	for _, p := range paths {
		_, err := ReplayWith(p, c, func(rec *Record) error {
			if rec.Type != OpEpoch || rec.LSN <= last {
				return nil
			}
			e, ok := DecodeEpoch(rec.Value)
			if !ok {
				return ErrBadEpochMarker
			}
			out = append(out, EpochMark{LSN: rec.LSN, Epoch: e})
			last = rec.LSN
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
