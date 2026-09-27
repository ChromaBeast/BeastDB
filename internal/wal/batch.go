package wal

import (
	"encoding/binary"
	"errors"
)

var (
	// ErrInvalidBatchPayload indicates corrupted or malformed batch record data.
	ErrInvalidBatchPayload = errors.New("wal: invalid batch payload format")
	// ErrEmptyBatch indicates an attempt to log an empty batch of operations.
	ErrEmptyBatch = errors.New("wal: batch contains zero operations")
)

// BatchOp represents a single sub-operation within an atomic batch WAL record.
type BatchOp struct {
	Type  byte // OpPut or OpDelete
	Key   []byte
	Value []byte
}

// EncodeBatchPayload serializes a slice of BatchOps into a contiguous byte slice.
// Wire format:
// [0..4]: Count (uint32)
// For each op:
//   [0]: Type (1 byte)
//   [1..3]: KeyLen (uint16)
//   [3..3+KeyLen]: Key
//   [3+KeyLen..7+KeyLen]: ValLen (uint32)
//   [7+KeyLen..7+KeyLen+ValLen]: Value
func EncodeBatchPayload(ops []BatchOp) ([]byte, error) {
	if len(ops) == 0 {
		return nil, ErrEmptyBatch
	}

	totalLen := 4
	for _, op := range ops {
		totalLen += 1 + 2 + len(op.Key) + 4 + len(op.Value)
	}

	buf := make([]byte, totalLen)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(ops)))

	offset := 4
	for _, op := range ops {
		buf[offset] = op.Type
		offset++

		kLen := len(op.Key)
		binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(kLen))
		offset += 2
		copy(buf[offset:offset+kLen], op.Key)
		offset += kLen

		vLen := len(op.Value)
		binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(vLen))
		offset += 4
		copy(buf[offset:offset+vLen], op.Value)
		offset += vLen
	}

	return buf, nil
}

// DecodeBatchPayload unpacks a serialized batch payload into a slice of BatchOps.
func DecodeBatchPayload(data []byte) ([]BatchOp, error) {
	if len(data) < 4 {
		return nil, ErrInvalidBatchPayload
	}

	count := binary.LittleEndian.Uint32(data[0:4])
	if count == 0 {
		return nil, nil
	}

	ops := make([]BatchOp, 0, count)
	offset := 4

	for i := uint32(0); i < count; i++ {
		if offset+7 > len(data) {
			return nil, ErrInvalidBatchPayload
		}

		opType := data[offset]
		offset++

		kLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if offset+kLen+4 > len(data) {
			return nil, ErrInvalidBatchPayload
		}
		key := data[offset : offset+kLen]
		offset += kLen

		vLen := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4
		if offset+vLen > len(data) {
			return nil, ErrInvalidBatchPayload
		}
		val := data[offset : offset+vLen]
		offset += vLen

		ops = append(ops, BatchOp{
			Type:  opType,
			Key:   key,
			Value: val,
		})
	}

	return ops, nil
}
