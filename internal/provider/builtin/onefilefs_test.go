package builtin

import (
	"errors"
	"io"
	"io/fs"
	"testing"
)

// TestOneFileFS pins the fs contract LoadResourceOwnerMap relies on (D18): the one
// entry opens, stats and reads as a read-only regular file; any other name is
// fs.ErrNotExist on every access path.
func TestOneFileFS(t *testing.T) {
	data := []byte("owners:\n  a: team-a\n")
	fsys := oneFileFS{name: "owners.yaml", data: data}

	info, err := fs.Stat(fsys, "owners.yaml")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.Mode().IsRegular() || info.IsDir() || info.Mode().Perm() != 0o444 {
		t.Fatalf("Stat mode = %v, want read-only regular file", info.Mode())
	}
	if info.Name() != "owners.yaml" || info.Size() != int64(len(data)) || !info.ModTime().IsZero() || info.Sys() != nil {
		t.Fatalf("Stat info = %q/%d/%v/%v", info.Name(), info.Size(), info.ModTime(), info.Sys())
	}

	got, err := fs.ReadFile(fsys, "owners.yaml")
	if err != nil || string(got) != string(data) {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}

	f, err := fsys.Open("owners.yaml")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if fi, err := f.Stat(); err != nil || fi.Size() != int64(len(data)) {
		t.Fatalf("File.Stat = %v, %v", fi, err)
	}
	// A 4-byte buffer forces several Reads, so the offset bookkeeping is exercised.
	var read []byte
	buf := make([]byte, 4)
	for {
		n, err := f.Read(buf)
		read = append(read, buf[:n]...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}
	if string(read) != string(data) {
		t.Fatalf("Read = %q, want %q", read, data)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for _, name := range []string{"other.yaml", ".", "dir/owners.yaml"} {
		if _, err := fsys.Open(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Open(%q) err = %v, want ErrNotExist", name, err)
		}
		if _, err := fs.Stat(fsys, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%q) err = %v, want ErrNotExist", name, err)
		}
		if _, err := fs.ReadFile(fsys, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ReadFile(%q) err = %v, want ErrNotExist", name, err)
		}
	}
}

// TestLoadResourceOwnerMapBytes pins the forge-bytes entry the provider host uses:
// a valid registry resolves its owners, a malformed one is an error (no client).
func TestLoadResourceOwnerMapBytes(t *testing.T) {
	c, err := LoadResourceOwnerMapBytes("owners.yaml", []byte("owners:\n  topic/a: team-a\n"))
	if err != nil {
		t.Fatalf("LoadResourceOwnerMapBytes: %v", err)
	}
	m, ok := c.(*mapResourceOwner)
	if !ok || m.owners["topic/a"] != "team-a" {
		t.Fatalf("client = %#v, want owner team-a for topic/a", c)
	}

	for name, raw := range map[string]string{
		"no owners key": "other: 1\n",
		"invalid yaml":  "owners: [\n",
	} {
		if c, err := LoadResourceOwnerMapBytes("owners.yaml", []byte(raw)); err == nil || c != nil {
			t.Errorf("%s: got client %v, err %v; want refusal", name, c, err)
		}
	}
}
