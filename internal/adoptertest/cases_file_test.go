package adoptertest_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// casesFilePath is the first authored inline cases.yaml fixture (E6-S06): four
// single-file cases (a proving edit, an over-cap edit, a new-file and a deleted-file
// case) over the same capped pack the directory cases use.
const casesFilePath = repoRoot + "/tests/capped/cases.yaml"

// loadCappedPack loads the capped fixture pack + its first binding — the pack the
// inline cases.yaml cases evaluate against. Reading the files is the test's I/O
// boundary; the loader under test stays pure (takes bytes).
func loadCappedPack(t *testing.T) (*policy.MergePolicy, *policy.Binding) {
	t.Helper()
	mp, err := policy.LoadMergePolicy(readFile(t, filepath.Join(repoRoot, "packs", "capped", "rules", "capped.yaml")))
	if err != nil {
		t.Fatalf("load pack policy: %v", err)
	}
	rb, err := policy.LoadRulesetBinding(readFile(t, filepath.Join(repoRoot, "bindings.yaml")))
	if err != nil {
		t.Fatalf("load binding: %v", err)
	}
	if len(rb.Bindings) == 0 {
		t.Fatal("fixture binding declares no bindings")
	}
	return mp, &rb.Bindings[0]
}

// loadInlineCases strict-decodes the authored cases.yaml fixture into runnable Cases.
func loadInlineCases(t *testing.T) []adoptertest.Case {
	t.Helper()
	mp, bind := loadCappedPack(t)
	cases, err := adoptertest.LoadInlineCases(readFile(t, casesFilePath), mp, bind)
	if err != nil {
		t.Fatalf("LoadInlineCases: %v", err)
	}
	return cases
}

func findInlineCase(t *testing.T, cases []adoptertest.Case, name string) adoptertest.Case {
	t.Helper()
	for _, c := range cases {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("inline case %q not found in fixture", name)
	return adoptertest.Case{}
}

// TestInlineCasesFileEvaluates (REQ-E6-S06-01) proves an inline cases.yaml strict-
// decodes against the frozen #/$defs/casesFile fragment and each case evaluates —
// through the SAME assembler + matcher a directory case uses — to its pinned
// expect.decision. Every fixture case must PASS; the run is double-run stable.
func TestInlineCasesFileEvaluates(t *testing.T) {
	cases := loadInlineCases(t)

	want := map[string]string{
		"partition-increase-ok":       "APPROVE",
		"partition-increase-over-cap": "REVIEW",
		"new-file":                    "REVIEW",
		"deleted-file":                "REVIEW",
	}
	if len(cases) != len(want) {
		t.Fatalf("loaded %d inline cases, want %d", len(cases), len(want))
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			wantDec, ok := want[c.Name]
			if !ok {
				t.Fatalf("unexpected inline case %q", c.Name)
			}
			out, err := adoptertest.RunCase(c)
			if err != nil {
				t.Fatalf("RunCase: %v", err)
			}
			if out.Actual != wantDec {
				t.Fatalf("decision = %q, want %q", out.Actual, wantDec)
			}
			if !out.Pass {
				t.Fatalf("case did not pass its own expect: expected %q, got %q (reasons: %v)", out.Expected, out.Actual, out.Reasons)
			}

			// Double-run stable: the shared pipeline is deterministic (ADR-0014 L0).
			first, err := adoptertest.Evaluate(c)
			if err != nil {
				t.Fatalf("Evaluate #1: %v", err)
			}
			second, err := adoptertest.Evaluate(c)
			if err != nil {
				t.Fatalf("Evaluate #2: %v", err)
			}
			if !bytes.Equal(mustJSON(t, first), mustJSON(t, second)) {
				t.Fatalf("double run not byte-identical:\n#1 %s\n#2 %s", mustJSON(t, first), mustJSON(t, second))
			}
		})
	}
}

// TestInlineNewAndDeletedFileCases (REQ-E6-S06-02) proves a `null` base (new-file
// case) and a `null` head (deleted-file case) route through the PRODUCTION differ and
// fail safe. A whole-file add/delete has an unparseable absent side, so change.Diff
// returns an OPAQUE ChangeSet and Evaluate maps it to REVIEW — never a silent APPROVE.
// The mechanism (opaqueness), not just the REVIEW verdict, is asserted, and contrasted
// against the same head with a NON-null base (which evaluates the content to APPROVE),
// so the REVIEW cannot pass for a spurious reason.
func TestInlineNewAndDeletedFileCases(t *testing.T) {
	cases := loadInlineCases(t)

	t.Run("new-file: null base is opaque -> fail-safe REVIEW", func(t *testing.T) {
		c := findInlineCase(t, cases, "new-file")
		if c.Base != nil {
			t.Fatalf("null base must marshal to absent bytes, got %q", c.Base)
		}
		// The production differ itself goes opaque on the absent base side.
		cs, err := change.Diff(c.File, c.Base, c.Head)
		if err == nil || !cs.Opaque {
			t.Fatalf("change.Diff on a null base must be opaque, got opaque=%v err=%v", cs.Opaque, err)
		}
		res, err := adoptertest.Evaluate(c)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if string(res.Decision) != "REVIEW" {
			t.Fatalf("new-file decision = %q, want REVIEW (fail-safe opaque)", res.Decision)
		}
		// Contrast: the SAME head with a non-null base evaluates the content -> APPROVE.
		contrast := c
		contrast.Base = []byte(`{"partitions":12}`)
		cres, err := adoptertest.Evaluate(contrast)
		if err != nil {
			t.Fatalf("Evaluate contrast: %v", err)
		}
		if string(cres.Decision) != "APPROVE" {
			t.Fatalf("non-null base contrast decision = %q, want APPROVE (proves the null base drove the REVIEW)", cres.Decision)
		}
	})

	t.Run("deleted-file: null head is opaque -> fail-safe REVIEW", func(t *testing.T) {
		c := findInlineCase(t, cases, "deleted-file")
		if c.Head != nil {
			t.Fatalf("null head must marshal to absent bytes, got %q", c.Head)
		}
		cs, err := change.Diff(c.File, c.Base, c.Head)
		if err == nil || !cs.Opaque {
			t.Fatalf("change.Diff on a null head must be opaque, got opaque=%v err=%v", cs.Opaque, err)
		}
		res, err := adoptertest.Evaluate(c)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if string(res.Decision) != "REVIEW" {
			t.Fatalf("deleted-file decision = %q, want REVIEW (fail-safe opaque)", res.Decision)
		}
	})
}

// TestInlineFileFormatSelection proves the inline front-end marshals each case's
// base/head to the `file`'s OWN format so change.Diff's extension-driven producer
// parses them: a `.yaml` file routes through the YAML producer (and agrees with the
// `.json` spelling of the same edit), while a `.tfvars` file — whose HCL producer has
// no lossless inline marshal — is a clear error rather than a silently mis-parsed side.
func TestInlineFileFormatSelection(t *testing.T) {
	mp, bind := loadCappedPack(t)

	t.Run("a .yaml file diffs through the YAML producer and agrees with .json", func(t *testing.T) {
		yamlCase := `cases:
  - name: yaml-edit
    file: config.yaml
    base: { partitions: 12 }
    head: { partitions: 16 }
    facts: { limits: { maxPartitions: 32 } }
    expect: { decision: APPROVE }
`
		jsonCase := `cases:
  - name: json-edit
    file: config.json
    base: { partitions: 12 }
    head: { partitions: 16 }
    facts: { limits: { maxPartitions: 32 } }
    expect: { decision: APPROVE }
`
		yc, err := adoptertest.LoadInlineCases([]byte(yamlCase), mp, bind)
		if err != nil {
			t.Fatalf("LoadInlineCases yaml: %v", err)
		}
		jc, err := adoptertest.LoadInlineCases([]byte(jsonCase), mp, bind)
		if err != nil {
			t.Fatalf("LoadInlineCases json: %v", err)
		}
		yout, err := adoptertest.RunCase(yc[0])
		if err != nil {
			t.Fatalf("RunCase yaml: %v", err)
		}
		jout, err := adoptertest.RunCase(jc[0])
		if err != nil {
			t.Fatalf("RunCase json: %v", err)
		}
		if !yout.Pass || yout.Actual != "APPROVE" {
			t.Fatalf("yaml case did not APPROVE: %+v", yout)
		}
		if yout.Actual != jout.Actual {
			t.Fatalf("yaml (%s) and json (%s) forms disagree", yout.Actual, jout.Actual)
		}
	})

	t.Run("a .tfvars file inline base/head is an unsupported error", func(t *testing.T) {
		raw := `cases:
  - name: tfvars-edit
    file: main.tfvars
    base: { partitions: 12 }
    head: { partitions: 16 }
    expect: { decision: APPROVE }
`
		_, err := adoptertest.LoadInlineCases([]byte(raw), mp, bind)
		if err == nil {
			t.Fatal("expected inline .tfvars base/head to be an error")
		}
		if !strings.Contains(err.Error(), "tfvars") {
			t.Fatalf("error does not name the unsupported format: %v", err)
		}
	})

	t.Run("unparseable cases.yaml bytes are rejected", func(t *testing.T) {
		_, err := adoptertest.LoadInlineCases([]byte("\tnot: [valid yaml"), mp, bind)
		if err == nil {
			t.Fatal("expected unparseable cases.yaml to be rejected")
		}
	})
}

// TestInlineCasesFileMalformedRejected (REQ-E6-S06-03) proves a malformed cases.yaml is
// a LOCATED rejection against the frozen #/$defs/casesFile fragment — never a silent
// skip: an unknown top-level key, an unknown per-case key, and a bad inline `expect`
// (a non-enum decision) each error at their offending instance location.
func TestInlineCasesFileMalformedRejected(t *testing.T) {
	mp, bind := loadCappedPack(t)

	cases := []struct {
		name   string
		raw    string
		locate string
	}{
		{
			name: "unknown top-level key",
			raw: `bogus: 1
cases:
  - name: x
    file: config.json
    base: { partitions: 1 }
    head: { partitions: 2 }
    expect: { decision: APPROVE }
`,
			locate: "bogus",
		},
		{
			name: "unknown per-case key",
			raw: `cases:
  - name: x
    file: config.json
    base: { partitions: 1 }
    head: { partitions: 2 }
    surprise: true
    expect: { decision: APPROVE }
`,
			locate: "surprise",
		},
		{
			name: "bad inline expect decision enum",
			raw: `cases:
  - name: x
    file: config.json
    base: { partitions: 1 }
    head: { partitions: 2 }
    expect: { decision: MAYBE }
`,
			locate: "decision",
		},
		{
			name: "missing required inline field (head)",
			raw: `cases:
  - name: x
    file: config.json
    base: { partitions: 1 }
    expect: { decision: APPROVE }
`,
			locate: "head",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adoptertest.LoadInlineCases([]byte(tc.raw), mp, bind)
			if err == nil {
				t.Fatalf("expected a located rejection for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.locate) {
				t.Fatalf("error does not locate %q: %v", tc.locate, err)
			}
		})
	}
}

// TestInlineAndDirectoryFormsAgree (REQ-E6-S06-04) proves the inline shorthand and the
// directory form share ONE assembler + matcher: an inline case and a directory case
// with identical content (the same base/head edit, facts, and expect) produce
// byte-identical engine Results and identical Outcomes. The inline form is only an
// alternate front-end, never a second pipeline.
func TestInlineAndDirectoryFormsAgree(t *testing.T) {
	// Directory form: the on-disk capped/within-cap case (base/head files).
	dirCase := loadCase(t, "within-cap")

	// Inline form: the SAME edit expressed inline.
	mp, bind := loadCappedPack(t)
	inline := `cases:
  - name: within-cap
    file: config.json
    base: { partitions: 12 }
    head: { partitions: 16 }
    facts: { limits: { maxPartitions: 32 } }
    expect: { decision: APPROVE }
`
	loaded, err := adoptertest.LoadInlineCases([]byte(inline), mp, bind)
	if err != nil {
		t.Fatalf("LoadInlineCases: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d inline cases, want 1", len(loaded))
	}
	inlineCase := loaded[0]

	// The assembled EvaluationInput/Result must be byte-identical across both forms.
	dirRes, err := adoptertest.Evaluate(dirCase)
	if err != nil {
		t.Fatalf("Evaluate directory: %v", err)
	}
	inlineRes, err := adoptertest.Evaluate(inlineCase)
	if err != nil {
		t.Fatalf("Evaluate inline: %v", err)
	}
	if !bytes.Equal(mustJSON(t, dirRes), mustJSON(t, inlineRes)) {
		t.Fatalf("inline and directory Results differ:\ndir:    %s\ninline: %s", mustJSON(t, dirRes), mustJSON(t, inlineRes))
	}

	// And the matched Outcome agrees (pass, decision, reasons).
	dirOut, err := adoptertest.RunCase(dirCase)
	if err != nil {
		t.Fatalf("RunCase directory: %v", err)
	}
	inlineOut, err := adoptertest.RunCase(inlineCase)
	if err != nil {
		t.Fatalf("RunCase inline: %v", err)
	}
	if dirOut.Pass != inlineOut.Pass || dirOut.Actual != inlineOut.Actual || dirOut.Expected != inlineOut.Expected {
		t.Fatalf("inline and directory Outcomes differ:\ndir:    %+v\ninline: %+v", dirOut, inlineOut)
	}
	if !bytes.Equal(mustJSON(t, dirOut.Reasons), mustJSON(t, inlineOut.Reasons)) {
		t.Fatalf("inline and directory reasons differ:\ndir:    %v\ninline: %v", dirOut.Reasons, inlineOut.Reasons)
	}
}
