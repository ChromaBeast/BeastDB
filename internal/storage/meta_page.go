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
	//   1: original layout.
	//   2: every page ends with a checksum trailer (see page_checksum.go), and
	//      the meta page also records the free page list, the free-space map
	//      and the search index snapshot.
	MetaVersion uint32 = 2

	// metaSize is the number of meta page bytes in use.
	metaSize = 64
)

// MetaData holds persistent database-level metadata stored at Page 0.
type MetaData struct {
	Magic             uint32
	Version           uint32
	RootPageID        uint64
	ActiveDataPageID  uint64
	LastCheckpointLSN uint64

	// Version 2 fields (zero in a version 1 file).
	FreeListHead  uint64 // first page of the free page list, 0 if empty
	SpaceMapHead  uint64 // first page of the saved free-space map, 0 if none
	IndexSnapHead uint64 // first page of the saved search index, 0 if none
	IndexSnapLSN  uint64 // checkpoint LSN the saved search index matches
}

// EncodeMeta serializes MetaData into the first 64 bytes of Page 0.
func EncodeMeta(page []byte, m MetaData) {
	binary.LittleEndian.PutUint32(page[0:4], m.Magic)
	binary.LittleEndian.PutUint32(page[4:8], m.Version)
	binary.LittleEndian.PutUint64(page[8:16], m.RootPageID)
	binary.LittleEndian.PutUint64(page[16:24], m.ActiveDataPageID)
	binary.LittleEndian.PutUint64(page[24:32], m.LastCheckpointLSN)
	binary.LittleEndian.PutUint64(page[32:40], m.FreeListHead)
	binary.LittleEndian.PutUint64(page[40:48], m.SpaceMapHead)
	binary.LittleEndian.PutUint64(page[48:56], m.IndexSnapHead)
	binary.LittleEndian.PutUint64(page[56:64], m.IndexSnapLSN)
}

// metaVersionOf returns the format version page 0 claims, if it carries the magic.
func metaVersionOf(page []byte) (uint32, bool) {
	if len(page) < 8 || binary.LittleEndian.Uint32(page[0:4]) != MetaMagic {
		return 0, false
	}
	return binary.LittleEndian.Uint32(page[4:8]), true
}

// MetaVersionOf returns the format version page 0 claims, if it carries the magic.
func MetaVersionOf(page []byte) (uint32, bool) { return metaVersionOf(page) }

// DecodeMeta deserializes and validates MetaData from Page 0 bytes.
func DecodeMeta(page []byte) (MetaData, error) {
	if len(page) < metaSize {
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
		FreeListHead:      binary.LittleEndian.Uint64(page[32:40]),
		SpaceMapHead:      binary.LittleEndian.Uint64(page[40:48]),
		IndexSnapHead:     binary.LittleEndian.Uint64(page[48:56]),
		IndexSnapLSN:      binary.LittleEndian.Uint64(page[56:64]),
	}, nil
}

// DecodeMetaV1 reads the fields a version 1 meta page holds. It is used only
// to migrate such a file to the current version.
func DecodeMetaV1(page []byte) (MetaData, error) {
	v, ok := metaVersionOf(page)
	if !ok {
		return MetaData{}, ErrInvalidMetaMagic
	}
	if v != 1 {
		return MetaData{}, ErrInvalidMetaVersion
	}
	return MetaData{
		Magic:             MetaMagic,
		Version:           1,
		RootPageID:        binary.LittleEndian.Uint64(page[8:16]),
		ActiveDataPageID:  binary.LittleEndian.Uint64(page[16:24]),
		LastCheckpointLSN: binary.LittleEndian.Uint64(page[24:32]),
	}, nil
}
