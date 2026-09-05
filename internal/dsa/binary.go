package dsa

import (
	"encoding/binary"
	"errors"
)

var (
	// ErrBufferTooSmall is returned when a byte buffer is smaller than the required type size.
	ErrBufferTooSmall = errors.New("buffer too small for decoded type")
)

// PutUint16LE encodes a uint16 into little-endian format at buf[offset:].
func PutUint16LE(buf []byte, offset int, v uint16) {
	binary.LittleEndian.PutUint16(buf[offset:], v)
}

// GetUint16LE decodes a little-endian uint16 from buf[offset:].
func GetUint16LE(buf []byte, offset int) uint16 {
	return binary.LittleEndian.Uint16(buf[offset:])
}

// PutUint32LE encodes a uint32 into little-endian format at buf[offset:].
func PutUint32LE(buf []byte, offset int, v uint32) {
	binary.LittleEndian.PutUint32(buf[offset:], v)
}

// GetUint32LE decodes a little-endian uint32 from buf[offset:].
func GetUint32LE(buf []byte, offset int) uint32 {
	return binary.LittleEndian.Uint32(buf[offset:])
}

// PutUint64LE encodes a uint64 into little-endian format at buf[offset:].
func PutUint64LE(buf []byte, offset int, v uint64) {
	binary.LittleEndian.PutUint64(buf[offset:], v)
}

// GetUint64LE decodes a little-endian uint64 from buf[offset:].
func GetUint64LE(buf []byte, offset int) uint64 {
	return binary.LittleEndian.Uint64(buf[offset:])
}

// PutUint32BE encodes a uint32 into big-endian format (used for network frames & CRC32).
func PutUint32BE(buf []byte, offset int, v uint32) {
	binary.BigEndian.PutUint32(buf[offset:], v)
}

// GetUint32BE decodes a big-endian uint32 from buf[offset:].
func GetUint32BE(buf []byte, offset int) uint32 {
	return binary.BigEndian.Uint32(buf[offset:])
}
