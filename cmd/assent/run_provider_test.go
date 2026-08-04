package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
)

// ownerDeclarationJSON is the host-owned provider declaration (D-065) served
// from the TARGET ref beside Config — not a silent config.schema.json widen.
const ownerDeclarationJSON = `{
  "name": "owner",
  "requests": {"values": {"pointers": []}},
  "outputs": {
    "team": {
      "type": "string",
      "cardinality": "single",
      "subject": "entry",
      "sensitive": false,
      "maxAge": "1h"
    }
  }
}`

// configOwnerHTTP points the controlling owner provider at a hermetic HTTP fake.
func configOwnerHTTP(url string) string {
	return `apiVersion: assent.dev/v1alpha1
kind: Config
environments:
  - name: prod
    match: { paths: ["topics/**"] }
classes:
  - name: topic-registry
    match: { paths: ["topics/**.yaml"] }
providers:
  owner:
    type: http
    url: ` + url + `
    failure: closed
`
}

// fakeOwnerProvider serves a schema-valid FactResponse with facts.owner.team
// resolved — the hermetic fake for REQ-E5-S05-01.
func fakeOwnerProvider(t *testing.T, asOf time.Time) *httptest.Server {
	t.Helper()
	decl := provider.Declaration{
		Type:        "string",
		Cardinality: "single",
		Subject:     "entry",
		Sensitive:   false,
		MaxAge:      "1h",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var q provider.FactQuery
		if err := json.Unmarshal(body, &q); err != nil {
			http.Error(w, "bad query: "+err.Error(), http.StatusBadRequest)
			return
		}
		expires := asOf.Add(time.Hour)
		resp := provider.FactResponse{
			APIVersion: provider.APIVersion,
			Kind:       provider.KindFactResponse,
			QueryID:    q.QueryID,
			Facts: []provider.Fact{{
				Name:        "team",
				Declaration: decl,
				State:       provider.StateResolved,
				Subject:     q.Subject,
				ObservedAt:  asOf,
				ExpiresAt:   &expires,
				Value:       "platform-team",
			}},
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestRunResolvesProviderFacts — REQ-E5-S05-01: assent run with a fake provider
// fills EvaluationInput.Facts + pins.factsResolvedAt; the decision reflects the
// resolved fact (ownership proves → APPROVE). AutoMergeEligible is NOT consulted
// for arming (INBOX P2) — --arm + APPROVE remains the arming gate; fact states
// are authoritative for CEL.
func TestRunResolvesProviderFacts(t *testing.T) {
	asOf := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	prov := fakeOwnerProvider(t, asOf)

	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyOwnership
	f.rulesetBinding = rulesetBindingOwnership
	f.config = configOwnerHTTP(prov.URL)
	f.providerDecls = map[string]string{"owner": ownerDeclarationJSON}
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs("--arm", "--config", ".assent/config.yaml"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), `"decision":"APPROVE"`) {
		t.Fatalf("resolved owner.team must prove ownership → APPROVE:\n%s", out.String())
	}
	// pins.factsResolvedAt must carry the provider's resolution instant.
	if !strings.Contains(out.String(), `"factsResolvedAt":{"owner":"2026-07-27T12:00:00Z"}`) &&
		!strings.Contains(out.String(), `"owner":"2026-07-27T12:00:00Z"`) {
		t.Fatalf("expected pins.factsResolvedAt.owner = evaluation instant:\n%s", out.String())
	}
	// Arming still requires --arm (already set); AME must not be a silent arm gate.
	if f.approvals != 1 || f.merges != 1 {
		t.Errorf("APPROVE+--arm must write: approvals=%d merges=%d", f.approvals, f.merges)
	}
}

// TestRunProviderlessUnchanged — REQ-E5-S05-02: provider-less / no --config path
// remains byte-identical to pre-S05 (empty Facts, empty factsResolvedAt). Double-run
// determinism pins the dry path.
func TestRunProviderlessUnchanged(t *testing.T) {
	record := func() string {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 6\n" // shrink → REVIEW
		var out bytes.Buffer
		code := runRun(runArgs(), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		return out.String()
	}
	a, b := record(), record()
	if a != b {
		t.Fatalf("provider-less double-run must be byte-identical\n--- run1 ---\n%s\n--- run2 ---\n%s", a, b)
	}
	if !strings.Contains(a, `"decision":"REVIEW"`) {
		t.Fatalf("expected REVIEW on partition shrink:\n%s", a)
	}
	if !strings.Contains(a, `"factsResolvedAt":{}`) {
		t.Fatalf("provider-less path must keep empty factsResolvedAt (pre-S05):\n%s", a)
	}
	// No provider keys may appear under factsResolvedAt.
	if strings.Contains(a, `"factsResolvedAt":{"`) {
		t.Fatalf("provider-less factsResolvedAt must stay empty object:\n%s", a)
	}
}
