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
