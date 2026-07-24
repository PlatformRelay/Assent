package hash

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDigest_rejectsBadJSON(t *testing.T) {
	if _, err := Digest("v1alpha1", []byte(`{`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestCanonicalize_trailingNonEOF(t *testing.T) {
	// Second token is invalid → Decode returns a non-EOF error after the first value.
	if _, err := Canonicalize([]byte(`true #`)); err == nil {
		t.Fatal("expected trailing-data error")
	}
}

func TestFormatNumber_edges(t *testing.T) {
	cases := []struct {
		in      json.Number
		want    string
		wantErr bool
	}{
		{json.Number(""), "", true},
		{json.Number("1e2"), "100", false},
		{json.Number("3.5"), "3.5", false},
		{json.Number("-0.0"), "0", false},
		{json.Number("1e309"), "", true}, // overflows float64
	}
	for _, tc := range cases {
		got, err := formatNumber(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestWriteCanonical_unsupportedAndNestedErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCanonical(&buf, 42); err == nil {
		t.Fatal("expected unsupported type error")
	}
	buf.Reset()
	if err := writeCanonical(&buf, []any{map[string]any{"a": 1}}); err == nil {
		t.Fatal("expected nested error for int inside object")
	}
	buf.Reset()
	if err := writeCanonical(&buf, []any{42}); err == nil {
		t.Fatal("expected nested error for int inside array")
	}
}
