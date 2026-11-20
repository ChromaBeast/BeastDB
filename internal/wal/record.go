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

	// MaxFrameSize bounds one encoded record. The engine caps batches at 8 MiB
	// of values, so anything larger on disk can only be a torn or corrupt header.
	MaxFrameSize = 32 << 20

	// Operation Types
	OpPut        byte = 0x01
	OpDelete     byte = 0x02
	OpCheckpoint byte = 0x03
	OpBatch      byte = 0x04
	// OpEpoch marks the point in the log where a new leader epoch begins (ADR
	// 0004). It carries no key; the value is the epoch as 8 little-endian
	// bytes. Every later record belongs to that epoch until the next marker.
	// It is an ordinary frame (plaintext or encrypted, same CRC): logs written
	// before it existed hold none and stay readable, and a reader that does not
	// know the type skips it on replay.
	OpEpoch byte = 0x05

	// headerCRCFlag is set in the on-disk type byte of records whose CRC covers
	// the header (length, LSN, type, key and value lengths) as well as the
	// payload. Records written before this flag existed checksum the payload
	// only and are still readable. Operation types therefore stay below 0x80.
	headerCRCFlag byte = 0x80
	crcOffset          = 19 // header bytes before the CRC field
)

var (
	ErrTornWrite       = errors.New("wal: torn write or CRC32 checksum mismatch")
	ErrRecordTooSmall  = errors.New("wal: record length smaller than header")
	ErrPayloadTooLarge = errors.New("wal: record payload exceeds max limit")
	ErrFrameTooLarge   = errors.New("wal: encoded record exceeds MaxFrameSize")
	// ErrBadEpochMarker means an OpEpoch record does not hold an 8-byte epoch.
	ErrBadEpochMarker = errors.New("wal: malformed epoch marker")
)

// Record represents a single state mutation logged for durability.
type Record struct {
	LSN   uint64
	Type  byte
	Key   []byte
	Value []byte
}

// EncodeRecord serializes a log record into a 23-byte header followed by key and value.
// rec.Type must be below 0x80.
func EncodeRecord(rec *Record) []byte {
	keyLen := len(rec.Key)
	valLen := len(rec.Value)
	payloadLen := keyLen + valLen
	recLen := HeaderSize + payloadLen

	buf := make([]byte, recLen)

	binary.LittleEndian.PutUint32(buf[0:4], uint32(recLen))
	binary.LittleEndian.PutUint64(buf[4:12], rec.LSN)
	buf[12] = rec.Type | headerCRCFlag
	binary.LittleEndian.PutUint16(buf[13:15], uint16(keyLen))
	binary.LittleEndian.PutUint32(buf[15:19], uint32(valLen))

	// Copy key and value into payload space
	copy(buf[HeaderSize:HeaderSize+keyLen], rec.Key)
	copy(buf[HeaderSize+keyLen:recLen], rec.Value)

	// CRC32 over the header fields and the payload (Key + Value).
	checksum := crc32.Update(crc32.ChecksumIEEE(buf[:crcOffset]), crc32.IEEETable, buf[HeaderSize:recLen])
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
	if recLen > MaxFrameSize {
		// A torn header can claim up to 4 GiB; treat it as a torn tail instead of allocating.
		return nil, ErrTornWrite
	}

	lsn := binary.LittleEndian.Uint64(header[4:12])
	opType := header[12] &^ headerCRCFlag
	headerCovered := header[12]&headerCRCFlag != 0
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

	var sum uint32
	if headerCovered {
		sum = crc32.Update(crc32.ChecksumIEEE(header[:crcOffset]), crc32.IEEETable, payload)
	} else {
		sum = crc32.ChecksumIEEE(payload) // record written before headerCRCFlag
	}
	if sum != expectedChecksum {
		return nil, ErrTornWrite
	}

	return &Record{
		LSN:   lsn,
		Type:  opType,
		Key:   payload[:keyLen],
		Value: payload[keyLen:],
	}, nil
}
