package net

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

const (
	// MagicByte identifies CustomDB protocol frames (0xDB).
	MagicByte byte = 0xDB
	// HeaderSize is the fixed 10-byte protocol header.
	HeaderSize = 10
	// MaxPayloadSize protects against out-of-memory attacks (16MB).
	MaxPayloadSize = 16 * 1024 * 1024

	// OpCodes
	OpPing     byte = 0x01
	OpGet      byte = 0x02
	OpSet      byte = 0x03
	OpDel      byte = 0x04
	OpResponse byte = 0x05
	OpError    byte = 0x06
)

var (
	ErrInvalidMagic     = errors.New("net: invalid magic byte")
	ErrPayloadTooLarge  = errors.New("net: payload exceeds maximum size")
	ErrChecksumMismatch = errors.New("net: CRC32 checksum mismatch")
)

// Frame represents a single parsed protocol message.
type Frame struct {
	OpCode  byte
	Payload []byte
}

// EncodeFrame serializes a message into a 10-byte header followed by the payload.
func EncodeFrame(op byte, payload []byte) []byte {
	payloadLen := len(payload)
	buf := make([]byte, HeaderSize+payloadLen)

	buf[0] = MagicByte
	buf[1] = op
	binary.BigEndian.PutUint32(buf[2:6], uint32(payloadLen))

	checksum := crc32.ChecksumIEEE(payload)
	binary.BigEndian.PutUint32(buf[6:10], checksum)

	copy(buf[10:], payload)
	return buf
}

// DecodeFrame reads a 10-byte header and its payload, validating magic and CRC32.
func DecodeFrame(r io.Reader) (*Frame, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	if header[0] != MagicByte {
		return nil, ErrInvalidMagic
	}

	op := header[1]
	payloadLen := binary.BigEndian.Uint32(header[2:6])
	expectedChecksum := binary.BigEndian.Uint32(header[6:10])

	if payloadLen > MaxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	if crc32.ChecksumIEEE(payload) != expectedChecksum {
		return nil, ErrChecksumMismatch
	}

	return &Frame{
		OpCode:  op,
		Payload: payload,
	}, nil
}
