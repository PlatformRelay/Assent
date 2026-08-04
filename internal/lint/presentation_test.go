package lint

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// presentationDiags runs predicate-scope + presentation checks over a one-pack model.
func presentationDiags(rules ...policy.Rule) []Diagnostic {
	m := &model{packs: map[string]*loadedPack{"p": {rules: rules}}}
	rep := &Report{}
	checkPredicateScope(m, rep)
	return rep.Diagnostics()
}

// TestLintUnknownDocsSummaryField — REQ-E8-S11-01: unknown CEL field in
// docs.summary fails lint with rule location (same message-template-scope machinery
// as E3-S04).
func TestLintUnknownDocsSummaryField(t *testing.T) {
	bad := policy.Rule{
		Name:   "retention-shrink",
		Effect: policy.EffectChallenge,
		Phase:  policy.PhaseEnforce,
		Match:  policy.Match{Files: &policy.FilesMatch{Paths: []string{"**/*.yaml"}}},
		Docs: policy.RuleDocs{
			Summary: "Shrinking retention deletes data — limit was {{ quota.max }}",
		},
	}
	scoped := diagsWithCode(presentationDiags(bad), CodeMessageTemplateScope)
	if len(scoped) != 1 {
		t.Fatalf("unknown field in docs.summary must yield message-template-scope, got %d: %v", len(scoped), scoped)
	}
	if !strings.Contains(scoped[0].Message, `"quota"`) {
		t.Errorf("diagnostic must name out-of-scope identifier quota, got %q", scoped[0].Message)
	}
	if scoped[0].Location.Name != "retention-shrink" {
		t.Errorf("location must name the rule, got %q", scoped[0].Location.Name)
	}

	// debug: line with out-of-scope field is also scope-checked.
	debugBad := policy.Rule{
		Name:   "debug-rule",
		Effect: policy.EffectBlock,
		Phase:  policy.PhaseEnforce,
		Match:  policy.Match{Files: &policy.FilesMatch{Paths: []string{"**/*.yaml"}}},
		Debug:  []string{"consumers: {{ object.size }}"},
	}
	scoped = diagsWithCode(presentationDiags(debugBad), CodeMessageTemplateScope)
	if len(scoped) != 1 {
		t.Fatalf("unknown field in debug: must yield message-template-scope, got %d", len(scoped))
	}
	if !strings.Contains(scoped[0].Message, `"object"`) {
		t.Errorf("debug diagnostic must name object, got %q", scoped[0].Message)
	}

	// In-scope docs.summary and debug lines lint clean.
	clean := policy.Rule{
		Name:   "clean",
		Effect: policy.EffectChallenge,
		Phase:  policy.PhaseEnforce,
		Match:  policy.Match{Files: &policy.FilesMatch{Paths: []string{"**/*.yaml"}}},
		Docs:   policy.RuleDocs{Summary: "was {{ old }} now {{ new }}"},
		Debug:  []string{"partitions: {{ facts.quota.max_partitions }}"},
	}
	if scoped := diagsWithCode(presentationDiags(clean), CodeMessageTemplateScope); len(scoped) != 0 {
		t.Errorf("in-scope docs.summary/debug must lint clean, got %v", scoped)
	}
}

// TestLintRejectsTier1Templates — REQ-E8-S11-02: presence of .assent/templates/
// returns tier-1 deferred error (not silent ignore).
func TestLintRejectsTier1Templates(t *testing.T) {
	sources := []Source{
		{
			Path: ".assent/templates/finding.md.tmpl",
			Bytes: []byte(`# tier-1 override stub
{{ .Finding.Rule }}
`),
		},
	}
	rep := Lint(sources)
	tier1 := diagsWithCode(rep.Diagnostics(), CodeTier1Deferred)
	if len(tier1) != 1 {
		t.Fatalf("want exactly one tier-1-deferred diagnostic, got %d: %v", len(tier1), rep.Diagnostics())
	}
	if tier1[0].Location.File != ".assent/templates/" {
		t.Errorf("location file = %q, want .assent/templates/", tier1[0].Location.File)
	}
	if !strings.Contains(tier1[0].Message, "tier 1") || !strings.Contains(tier1[0].Message, "deferred") {
		t.Errorf("message must explain tier-1 deferral, got %q", tier1[0].Message)
	}
}
