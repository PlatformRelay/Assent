package provider

import (
	"bytes"
	"strings"
	"testing"
)

// TestBoundedCaptureRetainsAtMostLimitPlusOne is the WHITE-BOX half of REL-01,
// and it is the only test that can see the finding as stated.
//
// REL-01 is *unbounded memory*, not "a missing error". The black-box exec tests
// in transport_test.go observe the error, which readBounded produces from the
// bytes that were kept — so they stay green even if the capture itself grows
// without limit and the runner OOMs before the verdict is ever reached. That is
// exactly how this finding survived three audits: the tests measured the wrong
// surface. This one measures retained bytes.
func TestBoundedCaptureRetainsAtMostLimitPlusOne(t *testing.T) {
	c := newBoundedCapture(MaxResponseBytes)

	const chunks = 3
	chunk := bytes.Repeat([]byte("x"), MaxResponseBytes) // 3x the limit in total
	for i := 0; i < chunks; i++ {
		n, err := c.Write(chunk)
		if err != nil {
			// os/exec's copier aborts on a writer error and reports it instead of
			// the limit error the caller must see, so a short write is a defect.
			t.Fatalf("write %d: %v", i, err)
		}
		if n != len(chunk) {
			t.Fatalf("write %d reported %d of %d bytes — a short write aborts os/exec's copier", i, n, len(chunk))
		}
	}

	// limit+1 is the whole point: it is what readBounded needs to tell an
	// at-limit response (legitimate) from an over-limit one (refused).
	if got, want := int64(c.buf.Len()), int64(MaxResponseBytes)+1; got != want {
		t.Fatalf("retained %d bytes after writing %d — the capture must hold exactly %d (the bound plus the one byte that proves it was exceeded)",
			got, chunks*len(chunk), want)
	}

	raw, err := c.bytesOrError()
	if err == nil {
		t.Fatal("an over-limit capture must fail closed")
	}
	if raw != nil {
		t.Fatalf("an over-limit capture must yield no bytes, got %d", len(raw))
	}
	if !c.overflowed() {
		t.Fatal("overflowed() must report an over-limit stream")
	}
}

// TestBoundedCaptureAtLimitIsIntact pins the boundary from the inside: a stream
// of exactly the limit is legitimate traffic, returned byte-for-byte.
func TestBoundedCaptureAtLimitIsIntact(t *testing.T) {
	c := newBoundedCapture(16)
	if _, err := c.Write([]byte("0123456789abcdef")); err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, err := c.bytesOrError()
	if err != nil {
		t.Fatalf("a stream of exactly the limit is legitimate: %v", err)
	}
	if string(raw) != "0123456789abcdef" {
		t.Fatalf("at-limit capture = %q, want it intact", raw)
	}
	if c.overflowed() {
		t.Fatal("an at-limit stream has not overflowed")
	}
}

// TestBoundedCaptureExcerptTruncates covers the stderr side (REL-07): the
// diagnostic buffer is bounded too — REL-01 must not be reopened through the
// back door — and a cut-off excerpt says that it was cut off.
func TestBoundedCaptureExcerptTruncates(t *testing.T) {
	c := newBoundedCapture(maxStderrExcerptBytes)
	if _, err := c.Write(bytes.Repeat([]byte("N"), 4<<20)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := int64(c.buf.Len()); got != maxStderrExcerptBytes+1 {
		t.Fatalf("stderr capture retained %d bytes — it must stay bounded at %d", got, maxStderrExcerptBytes+1)
	}
	excerpt := c.excerpt()
	if !strings.Contains(excerpt, "truncated") {
		t.Fatalf("a truncated excerpt must say so, got %d bytes", len(excerpt))
	}
	if len(excerpt) > maxStderrExcerptBytes+64 {
		t.Fatalf("excerpt is %d bytes, want at most the bound plus the marker", len(excerpt))
	}

	quiet := newBoundedCapture(maxStderrExcerptBytes)
	if _, err := quiet.Write([]byte("   \n ")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if quiet.excerpt() != "" {
		t.Fatalf("whitespace-only stderr must not decorate an error, got %q", quiet.excerpt())
	}
}
