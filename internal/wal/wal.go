// Package wal implements checksummed framed durable log records
// (docs/invariants.md D1). Recovery discards a torn unsynced suffix; a checksum
// failure in the middle of required history is corruption.
package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	magic   uint32 = 0x5a4e574c // "ZNWL"
	hdrSize        = 4 + 4 + 4  // magic + len + crc
)

// Encode appends one framed record for payload onto dst.
func Encode(dst, payload []byte) []byte {
	var hdr [hdrSize]byte
	binary.LittleEndian.PutUint32(hdr[0:4], magic)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	binary.LittleEndian.PutUint32(hdr[8:12], crc32.ChecksumIEEE(payload))
	dst = append(dst, hdr[:]...)
	dst = append(dst, payload...)
	return dst
}

// Recover parses complete valid records from data. A torn trailing frame
// (incomplete header/payload) is discarded. A bad checksum not at the
// extreme tail is returned as corruption.
func Recover(data []byte) (records [][]byte, err error) {
	i := 0
	for i < len(data) {
		if len(data)-i < hdrSize {
			// torn header — discard suffix
			return records, nil
		}
		m := binary.LittleEndian.Uint32(data[i : i+4])
		if m != magic {
			if len(records) == 0 && i == 0 {
				return nil, fmt.Errorf("wal: bad magic at 0")
			}
			// treat as torn/corrupt tail after valid prefix
			return records, nil
		}
		n := int(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		crc := binary.LittleEndian.Uint32(data[i+8 : i+12])
		i += hdrSize
		if n < 0 || i+n > len(data) {
			return records, nil // torn payload
		}
		payload := data[i : i+n]
		if crc32.ChecksumIEEE(payload) != crc {
			if i+n == len(data) {
				return records, nil // torn last record
			}
			return records, fmt.Errorf("wal: checksum failure at mid-log offset %d", i-hdrSize)
		}
		cp := make([]byte, n)
		copy(cp, payload)
		records = append(records, cp)
		i += n
	}
	return records, nil
}

// Memory is an in-memory WAL used by the simulator: Append buffers bytes;
// Sync advances the durable watermark. CrashUnsynced discards the unsynced
// suffix (one permitted outcome for unsynced data — docs/invariants.md D1).
type Memory struct {
	buf    []byte
	synced int // durable prefix length
}

func (m *Memory) Append(payload []byte) {
	m.buf = walAppend(m.buf, payload)
}

func walAppend(buf, payload []byte) []byte {
	return Encode(buf, payload)
}

func (m *Memory) Sync() { m.synced = len(m.buf) }

func (m *Memory) Bytes() []byte { return m.buf }

func (m *Memory) SyncedBytes() []byte { return m.buf[:m.synced] }

// CrashUnsynced drops bytes past the last Sync (sim crash before fsync).
func (m *Memory) CrashUnsynced() {
	m.buf = append([]byte(nil), m.buf[:m.synced]...)
}

// TruncatePrefix removes the first n durable bytes after a snapshot install.
// ponytail: only allowed when n <= synced.
func (m *Memory) TruncatePrefix(n int) error {
	if n < 0 || n > m.synced {
		return fmt.Errorf("wal: truncate %d past synced %d", n, m.synced)
	}
	m.buf = append([]byte(nil), m.buf[n:]...)
	m.synced -= n
	return nil
}

// LoadSynced recovers records from the durable prefix only.
func (m *Memory) LoadSynced() ([][]byte, error) {
	return Recover(m.SyncedBytes())
}
