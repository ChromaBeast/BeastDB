package api

// walBatchOpOverhead is the WAL batch framing per operation: type (1), key
// length (2), the 8-byte key and value length (4); see wal.EncodeBatchPayload.
const walBatchOpOverhead = 15

// checkValueSizes enforces per-value and per-batch limits before anything is
// logged. The batch limit covers the encoded WAL frame, framing included, so a
// committed batch always fits in one replication message.
func checkValueSizes(ops []BatchOperation) error {
	total := 4 // operation count
	for _, op := range ops {
		total += walBatchOpOverhead
		if op.Type != BatchOpPut {
			continue
		}
		if len(op.Value) > MaxValueSize {
			return ErrRecordTooLarge
		}
		total += len(op.Value)
	}
	if total > MaxBatchBytes {
		return ErrBatchTooLarge
	}
	return nil
}
