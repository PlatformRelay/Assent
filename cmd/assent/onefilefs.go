package main

import (
	"io"
	"io/fs"
	"time"
)

// oneFileFS is a minimal read-only fs.FS exposing a single in-memory regular file
// at name. It replaces testing/fstest.MapFS on the shipped run path (D18): the
// provider host wraps bytes fetched from the forge in a one-file filesystem so
// builtin.LoadResourceOwnerMap can apply its symlink-safe classification, and
// testing/fstest has no business being linked into the release binary.
//
// It implements fs.StatFS and fs.ReadFileFS so fs.Stat / fs.ReadFile / fs.Lstat
// resolve without opening the file, matching the shape the loader's
// classifyCandidate/isRegular expect.
type oneFileFS struct {
	name string
	data []byte
}

func (f oneFileFS) Open(name string) (fs.File, error) {
	if name != f.name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &oneFile{name: f.name, data: f.data}, nil
}

func (f oneFileFS) Stat(name string) (fs.FileInfo, error) {
	if name != f.name {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return oneFileInfo{name: f.name, size: int64(len(f.data))}, nil
}

func (f oneFileFS) ReadFile(name string) ([]byte, error) {
	if name != f.name {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}
	return f.data, nil
}

// oneFile is the fs.File for oneFileFS's single entry.
type oneFile struct {
	name string
	data []byte
	off  int
}

func (f *oneFile) Stat() (fs.FileInfo, error) {
	return oneFileInfo{name: f.name, size: int64(len(f.data))}, nil
}

func (f *oneFile) Read(p []byte) (int, error) {
	if f.off >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.off:])
	f.off += n
	return n, nil
}

func (f *oneFile) Close() error { return nil }

// oneFileInfo is the fs.FileInfo for oneFileFS's single entry: a regular,
// read-only, zero-mtime file. IsDir/Sys are the zero values.
type oneFileInfo struct {
	name string
	size int64
}

func (i oneFileInfo) Name() string       { return i.name }
func (i oneFileInfo) Size() int64        { return i.size }
func (i oneFileInfo) Mode() fs.FileMode  { return 0o444 }
func (i oneFileInfo) ModTime() time.Time { return time.Time{} }
func (i oneFileInfo) IsDir() bool        { return false }
func (i oneFileInfo) Sys() any           { return nil }
