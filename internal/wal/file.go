package wal

import (
	"os"
	"path/filepath"
)

// File is a durable WAL on the real filesystem (Phase 5 real-disk smoke).
type File struct {
	path   string
	synced int64
}

func OpenFile(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "wal.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	return &File{path: path, synced: st.Size()}, nil
}

func (f *File) Append(payload []byte) error {
	file, err := os.OpenFile(f.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	frame := Encode(nil, payload)
	_, err = file.Write(frame)
	return err
}

func (f *File) Sync() error {
	file, err := os.OpenFile(f.path, os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return err
	}
	st, err := file.Stat()
	if err != nil {
		return err
	}
	f.synced = st.Size()
	return nil
}

func (f *File) LoadSynced() ([][]byte, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > f.synced {
		data = data[:f.synced]
	}
	return Recover(data)
}
