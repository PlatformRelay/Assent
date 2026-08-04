package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// catalogueDoc mirrors the JSON `assent catalogue` emits, for assertion. Only the
// fields this L1 command test needs are decoded.
type catalogueDoc struct {
	Rules []struct {
		ID          string   `json:"id"`
		Pack        string   `json:"pack"`
		Rule        string   `json:"rule"`
		DocsURL     string   `json:"docsUrl"`
		Phase       string   `json:"phase"`
		Obligation  string   `json:"obligation"`
		Classes     []string `json:"classes"`
		Deprecated  bool     `json:"deprecated"`
		Deprecation string   `json:"deprecation"`
	} `json:"rules"`
}

// TestCatalogueCommand asserts `assent catalogue <dir>` discovers the `.assent/**`
// tree, generates the report from the loaded packs, prints it to stdout, and
// exits 0 (REQ-E3-S07-04, L1). It also confirms the load-bearing binding-graph
// join (classes) and the phase:off deprecation surface end-to-end.
func TestCatalogueCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCatalogue([]string{"testdata/catalogue"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}

	var doc catalogueDoc
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not valid catalogue JSON: %v\n%s", err, stdout.String())
	}
	if len(doc.Rules) != 2 {
		t.Fatalf("expected 2 catalogued rules, got %d: %s", len(doc.Rules), stdout.String())
	}

	byID := map[string]int{}
	for i, r := range doc.Rules {
		byID[r.ID] = i
	}

	oi, ok := byID["topics/author-owns-entry"]
	if !ok {
		t.Fatalf("ownership rule not catalogued: %s", stdout.String())
	}
	own := doc.Rules[oi]
	if own.Obligation != "ownership" {
		t.Errorf("obligation = %q, want ownership", own.Obligation)
	}
	// The class arrives ONLY through the binding.packs[topics] -> class join,
	// keyed by the packs/topics/ directory name.
	if len(own.Classes) != 1 || own.Classes[0] != "kafka-topic" {
		t.Errorf("classes = %v, want [kafka-topic] (binding-graph join)", own.Classes)
	}
	if own.DocsURL == "" || own.Phase != "enforce" {
		t.Errorf("docsUrl/phase not surfaced: %+v", own)
	}

	ri, ok := byID["topics/legacy-naming"]
	if !ok {
		t.Fatalf("retired rule not catalogued")
	}
	retired := doc.Rules[ri]
	if !retired.Deprecated || retired.Deprecation == "" {
		t.Errorf("phase:off rule not surfaced as deprecated: %+v", retired)
	}
}

// TestCatalogueCommandDeterministicOutput asserts the command's stdout is
// byte-identical across two runs (REQ-E3-S07-05 at the L1 boundary).
func TestCatalogueCommandDeterministicOutput(t *testing.T) {
	var a, b bytes.Buffer
	if code := runCatalogue([]string{"testdata/catalogue"}, &a, &bytes.Buffer{}); code != 0 {
		t.Fatalf("run a exit %d", code)
	}
	if code := runCatalogue([]string{"testdata/catalogue"}, &b, &bytes.Buffer{}); code != 0 {
		t.Fatalf("run b exit %d", code)
	}
	if a.String() != b.String() {
		t.Fatalf("catalogue command output not byte-identical:\n a=%s\n b=%s", a.String(), b.String())
	}
}

// TestCatalogueCommandMissingDir asserts a missing directory argument is a usage
// error (non-zero exit), not a panic or a silent empty report.
func TestCatalogueCommandMissingDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runCatalogue(nil, &stdout, &stderr); code == 0 {
		t.Fatalf("missing dir arg should be non-zero, got 0")
	}
	if code := runCatalogue([]string{"testdata/does-not-exist"}, &stdout, &stderr); code == 0 {
		t.Fatalf("nonexistent tree should be non-zero, got 0")
	}
}
