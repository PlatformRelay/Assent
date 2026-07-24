// Package hash provides schema-version domain-separated digests over canonical JSON.
//
// Canonicalization (assent-jcs-v1) follows RFC 8785 JCS for the shapes assent uses:
// object keys sorted lexicographically by UTF-8 byte order, no insignificant whitespace,
// numbers normalized to a deterministic decimal form, arrays preserving element order.
// The digest domain is: "assent.canonical-json.v1\n" + schemaVersion + "\n" + canonicalJSON.
package hash

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

const domainPrefix = "assent.canonical-json.v1"

// Digest returns the lowercase hex SHA-256 of domain-separated canonical JSON.
// schemaVersion is folded into the hash domain so identical bodies under different
// versions never collide (ADR-0017 §9 / P3-E2-S03).
func Digest(schemaVersion string, raw []byte) (string, error) {
	canonical, err := Canonicalize(raw)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = io.WriteString(h, domainPrefix)
	_, _ = io.WriteString(h, "\n")
	_, _ = io.WriteString(h, schemaVersion)
	_, _ = io.WriteString(h, "\n")
	_, _ = h.Write(canonical)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Canonicalize returns compact JSON with sorted object keys and normalized numbers.
func Canonicalize(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("hash: decode: %w", err)
	}
	// Reject trailing junk so non-JSON payloads cannot sneak into digests.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("hash: trailing data after JSON value")
		}
		return nil, fmt.Errorf("hash: trailing data after JSON value: %w", err)
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(w *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		w.WriteString("null")
		return nil
	case bool:
		if x {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
		return nil
	case json.Number:
		n, err := formatNumber(x)
		if err != nil {
			return err
		}
		w.WriteString(n)
		return nil
	case string:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Errorf("hash: string: %w", err)
		}
		w.Write(b)
		return nil
	case []any:
		w.WriteByte('[')
		for i, el := range x {
			if i > 0 {
				w.WriteByte(',')
			}
			if err := writeCanonical(w, el); err != nil {
				return err
			}
		}
		w.WriteByte(']')
		return nil
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				w.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return fmt.Errorf("hash: key: %w", err)
			}
			w.Write(kb)
			w.WriteByte(':')
			if err := writeCanonical(w, x[k]); err != nil {
				return err
			}
		}
		w.WriteByte('}')
		return nil
	default:
		return fmt.Errorf("hash: unsupported JSON type %T", v)
	}
}

// formatNumber normalizes JSON numbers so 1, 1.0, and 1.00 serialize identically.
// Integers in the int64 range (no fractional part after float parse) use decimal
// integer form; otherwise a shortest round-trip float form is used.
func formatNumber(n json.Number) (string, error) {
	s := strings.TrimSpace(string(n))
	if s == "" {
		return "", fmt.Errorf("hash: empty number")
	}
	// Prefer exact integer path when the literal has no fraction/exponent and fits int64.
	if !strings.ContainsAny(s, ".eE") {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return strconv.FormatInt(i, 10), nil
		}
	}
	f, err := n.Float64()
	if err != nil {
		// encoding/json rejects NaN/Inf literals; out-of-range exponents surface here.
		return "", fmt.Errorf("hash: number %q: %w", s, err)
	}
	if f == 0 {
		return "0", nil
	}
	// Collapse integral floats (1.0, 2.00) to integer form when exact in int64 range.
	if f == math.Trunc(f) && f >= float64(math.MinInt64) && f <= float64(math.MaxInt64) {
		return strconv.FormatInt(int64(f), 10), nil
	}
	return strconv.FormatFloat(f, 'g', -1, 64), nil
}
