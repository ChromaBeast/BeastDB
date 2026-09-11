package wal

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

const (
	// HeaderSize represents the fixed 23-byte WAL record header:
	// RecLen (4B) + LSN (8B) + Type (1B) + KeyLen (2B) + ValLen (4B) + CRC32 (4B) = 23 Bytes.
	HeaderSize = 23

	// Operation Types
	OpPut        byte = 0x01
	OpDelete     byte = 0x02
	OpCheckpoint byte = 0x03
)

var (
	ErrTornWrite       = errors.New("wal: torn write or CRC32 checksum mismatch")
	ErrRecordTooSmall  = errors.New("wal: record length smaller than header")
	ErrPayloadTooLarge = errors.New("wal: record payload exceeds max limit")
)

// Record represents a single state mutation logged for durability.
type Record struct {
	LSN   uint64
	Type  byte
	Key   []byte
	Value []byte
}

// EncodeRecord serializes a log record into a 23-byte header followed by key and value.
func EncodeRecord(rec *Record) []byte {
	keyLen := len(rec.Key)
	valLen := len(rec.Value)
	payloadLen := keyLen + valLen
	recLen := HeaderSize + payloadLen

	buf := make([]byte, recLen)

	binary.LittleEndian.PutUint32(buf[0:4], uint32(recLen))
	binary.LittleEndian.PutUint64(buf[4:12], rec.LSN)
	buf[12] = rec.Type
	binary.LittleEndian.PutUint16(buf[13:15], uint16(keyLen))
	binary.LittleEndian.PutUint32(buf[15:19], uint32(valLen))

	// Copy key and value into payload space
	copy(buf[HeaderSize:HeaderSize+keyLen], rec.Key)
	copy(buf[HeaderSize+keyLen:recLen], rec.Value)

	// Calculate CRC32 over the payload (Key + Value)
	checksum := crc32.ChecksumIEEE(buf[HeaderSize:recLen])
	binary.LittleEndian.PutUint32(buf[19:23], checksum)

	return buf
}

// DecodeRecord reads the next record from the stream, validating header and CRC32.
func DecodeRecord(r io.Reader) (*Record, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	recLen := binary.LittleEndian.Uint32(header[0:4])
	if recLen < HeaderSize {
		return nil, ErrRecordTooSmall
	}

	lsn := binary.LittleEndian.Uint64(header[4:12])
	opType := header[12]
	keyLen := int(binary.LittleEndian.Uint16(header[13:15]))
	valLen := int(binary.LittleEndian.Uint32(header[15:19]))
	expectedChecksum := binary.LittleEndian.Uint32(header[19:23])

	payloadLen := recLen - HeaderSize
	if int(payloadLen) != keyLen+valLen {
		return nil, ErrTornWrite
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, ErrTornWrite
	}

	if crc32.ChecksumIEEE(payload) != expectedChecksum {
		return nil, ErrTornWrite
	}

	return &Record{
		LSN:   lsn,
		Type:  opType,
		Key:   payload[:keyLen],
		Value: payload[keyLen:],
	}, nil
}
