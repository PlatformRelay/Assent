package adoptertest_test

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
)

// TestExpectYamlStrictDecodedAgainstFrozenSchema (REQ-E6-S01-03) proves expect.yaml is
// strict-decoded against the FROZEN test-expectation contract (schemas.ExpectationSchema,
// the #/$defs/expectation fragment — no new schema is authored): a valid expectation
// decodes; an unknown field or a non-{APPROVE,REVIEW,BLOCK} decision is a LOCATED
// rejection.
func TestExpectYamlStrictDecodedAgainstFrozenSchema(t *testing.T) {
	t.Run("valid expectation decodes to its decision", func(t *testing.T) {
		e, err := adoptertest.LoadExpectation([]byte("decision: APPROVE\nfindings: []\n"))
		if err != nil {
			t.Fatalf("LoadExpectation: %v", err)
		}
		if e.Decision != "APPROVE" {
			t.Fatalf("decision = %q, want APPROVE", e.Decision)
		}
	})

	t.Run("unknown field is a located rejection", func(t *testing.T) {
		_, err := adoptertest.LoadExpectation([]byte("decision: APPROVE\nbogus: true\n"))
		if err == nil {
			t.Fatal("expected an unknown field to be rejected")
		}
		if !strings.Contains(err.Error(), "bogus") {
			t.Fatalf("error does not locate the offending field %q: %v", "bogus", err)
		}
	})

	t.Run("bad decision enum is a located rejection", func(t *testing.T) {
		_, err := adoptertest.LoadExpectation([]byte("decision: MAYBE\n"))
		if err == nil {
			t.Fatal("expected a non-{APPROVE,REVIEW,BLOCK} decision to be rejected")
		}
		if !strings.Contains(err.Error(), "decision") {
			t.Fatalf("error does not locate the offending /decision: %v", err)
		}
	})

	t.Run("missing required decision is rejected", func(t *testing.T) {
		_, err := adoptertest.LoadExpectation([]byte("findings: []\n"))
		if err == nil {
			t.Fatal("expected a missing decision to be rejected")
		}
	})
}

// TestLoadExpectationProjectsMatcherFields proves the S03 matcher fields survive the
// YAML load path intact — the struct tags (including the exotic `message~` tilde key)
// bind, so a real expect.yaml reaches Match with every assertion populated. Without
// this a silently-unbound tag would DROP a safety assertion (absent/exact) and the
// case would pass vacuously — exactly the silent-approve D-054 guards against.
func TestLoadExpectationProjectsMatcherFields(t *testing.T) {
	e, err := adoptertest.LoadExpectation([]byte(`decision: REVIEW
exact: true
absent: [never-fires, also-absent]
findings:
  - rule: size-bounded
    obligation: bounded
    effect: challenge
    message~: "grew by [0-9]+"
score:
  total: 3
  threshold: 5
`))
	if err != nil {
		t.Fatalf("LoadExpectation: %v", err)
	}
	if !e.Exact {
		t.Error("exact:true did not bind")
	}
	if len(e.Absent) != 2 || e.Absent[0] != "never-fires" || e.Absent[1] != "also-absent" {
		t.Errorf("absent did not bind: %v", e.Absent)
	}
	if len(e.Findings) != 1 {
		t.Fatalf("findings did not bind: %v", e.Findings)
	}
	f := e.Findings[0]
	if f.Rule != "size-bounded" || f.Obligation != "bounded" || f.Effect != "challenge" {
		t.Errorf("finding identity did not bind: %+v", f)
	}
	if f.Message != "grew by [0-9]+" {
		t.Errorf("message~ tilde tag did not bind: %q", f.Message)
	}
	if e.Score == nil || e.Score.Total != 3 || e.Score.Threshold != 5 {
		t.Errorf("score did not bind: %+v", e.Score)
	}
}
