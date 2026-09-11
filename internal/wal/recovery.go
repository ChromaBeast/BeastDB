package wal

import (
	"io"
	"os"
)

// RecoveryReport summarizes the results of replaying a WAL file.
type RecoveryReport struct {
	TotalRecords      int
	LastLSN           uint64
	TornWriteDetected bool
}

// Replay scans the WAL file from beginning to end, executing applyFn for each valid record.
// If a torn write is encountered at the tail, it stops safely and reports it.
func Replay(path string, applyFn func(rec *Record) error) (RecoveryReport, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RecoveryReport{}, nil
		}
		return RecoveryReport{}, err
	}
	defer file.Close()

	report := RecoveryReport{}

	for {
		rec, err := DecodeRecord(file)
		if err != nil {
			if err == io.EOF {
				// Reached clean end of log
				break
			}
			if err == ErrTornWrite || err == io.ErrUnexpectedEOF || err == ErrRecordTooSmall {
				// Torn write detected at tail (e.g. process killed mid-write)
				report.TornWriteDetected = true
				break
			}
			return report, err
		}

		if err := applyFn(rec); err != nil {
			return report, err
		}

		report.TotalRecords++
		if rec.LSN > report.LastLSN {
			report.LastLSN = rec.LSN
		}
	}

	return report, nil
}
