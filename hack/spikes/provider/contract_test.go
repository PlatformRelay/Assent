package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"
)

func mustCanonicalJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func writeJSON(t *testing.T, w io.Writer, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode: %v", err)
	}
}

// fixedAsOf is the injected evaluation instant — no wall clock in assertions.
var fixedAsOf = time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

func groupQuery(t *testing.T) FactQuery {
	t.Helper()
	return FactQuery{
		APIVersion: APIVersion,
		Kind:       KindFactQuery,
		QueryID:    "q-contract-1",
		AsOf:       fixedAsOf,
		Subject:    Subject{Kind: "user", ID: "alice"},
		Outputs:    []string{"groups"},
	}
}

// REQ-P2-E3-S01-01: the same FactQuery over HTTP and exec yields responses that
// both validate against response.schema.json and are byte-identical after
// canonicalization.
func TestContract(t *testing.T) {
	q := groupQuery(t)

	srv := httptest.NewServer(ToyHTTPHandler())
	defer srv.Close()

	httpRaw, err := CallHTTP(t.Context(), srv.URL, q, 5*time.Second)
	if err != nil {
		t.Fatalf("HTTP call: %v", err)
	}
	execRaw, err := CallExec(t.Context(), toyExecBin, nil, q, 5*time.Second)
	if err != nil {
		t.Fatalf("exec call: %v", err)
	}

	for name, raw := range map[string][]byte{"http": httpRaw, "exec": execRaw} {
		if err := ValidateRequest(mustCanonicalJSON(t, q)); err != nil {
			t.Fatalf("request invalid against request.schema.json: %v", err)
		}
		if err := ValidateResponse(raw); err != nil {
			t.Errorf("%s response invalid against response.schema.json: %v", name, err)
		}
	}

	httpCanon, err := Canonicalize(httpRaw)
	if err != nil {
		t.Fatalf("canonicalize http: %v", err)
	}
	execCanon, err := Canonicalize(execRaw)
	if err != nil {
		t.Fatalf("canonicalize exec: %v", err)
	}
	if !bytes.Equal(httpCanon, execCanon) {
		t.Errorf("transports disagree after canonicalization:\nhttp: %s\nexec: %s", httpCanon, execCanon)
	}
}
