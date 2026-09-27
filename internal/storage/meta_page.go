package storage

import (
	"encoding/binary"
	"errors"
)

var (
	// ErrInvalidMetaMagic indicates that the storage file does not begin with the BeastDB magic number.
	ErrInvalidMetaMagic = errors.New("meta: invalid database magic bytes")
	// ErrInvalidMetaVersion indicates an unsupported database format version.
	ErrInvalidMetaVersion = errors.New("meta: unsupported database format version")
)

const (
	// MetaPageID is the fixed page ID reserved for database header and metadata.
	MetaPageID uint64 = 0

	// MetaMagic identifies a valid BeastDB storage file ("BEAS" in little-endian ASCII).
	MetaMagic uint32 = 0x53414542

	// MetaVersion is the current on-disk layout format version.
	MetaVersion uint32 = 1
)

// MetaData holds persistent database-level metadata stored at Page 0.
type MetaData struct {
	Magic             uint32
	Version           uint32
	RootPageID        uint64
	ActiveDataPageID  uint64
	LastCheckpointLSN uint64
}

// EncodeMeta serializes MetaData into the first 32 bytes of Page 0.
func EncodeMeta(page []byte, m MetaData) {
	binary.LittleEndian.PutUint32(page[0:4], m.Magic)
	binary.LittleEndian.PutUint32(page[4:8], m.Version)
	binary.LittleEndian.PutUint64(page[8:16], m.RootPageID)
	binary.LittleEndian.PutUint64(page[16:24], m.ActiveDataPageID)
	binary.LittleEndian.PutUint64(page[24:32], m.LastCheckpointLSN)
}

// DecodeMeta deserializes and validates MetaData from Page 0 bytes.
func DecodeMeta(page []byte) (MetaData, error) {
	if len(page) < 32 {
		return MetaData{}, ErrDiskBufferTooSmall
	}
	magic := binary.LittleEndian.Uint32(page[0:4])
	if magic != MetaMagic {
		return MetaData{}, ErrInvalidMetaMagic
	}
	version := binary.LittleEndian.Uint32(page[4:8])
	if version != MetaVersion {
		return MetaData{}, ErrInvalidMetaVersion
	}

	return MetaData{
		Magic:             magic,
		Version:           version,
		RootPageID:        binary.LittleEndian.Uint64(page[8:16]),
		ActiveDataPageID:  binary.LittleEndian.Uint64(page[16:24]),
		LastCheckpointLSN: binary.LittleEndian.Uint64(page[24:32]),
	}, nil
}
