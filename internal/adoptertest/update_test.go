package adoptertest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// update_test.go covers the PURE half of the S05 --update golden-refresh flow:
// UpdateExpectationBlock rewrites the expectation payload of an authored expect.yaml
// with the produced actuals IN PLACE, preserving surrounding authored comments
// (yaml.v3 Node surgery, not a comment-clobbering re-marshal), and fail-closes on a
// result that does not re-validate against the frozen schema. The FS write itself
// lives in cmd/assent (the sanctioned I/O boundary); this library stays pure. The
// "passing case left byte-identical" orchestration property (REQ-03) is proven at the
// command level (cmd/assent/test_test.go) where the discovery walk lives.

// TestUpdateWritesValidExpectation (REQ-E6-S05-01): --update rewrites a failing case's
// expectation to the actuals; the rewritten bytes re-validate against the frozen schema
// AND match their own Result (a subsequent normal run then PASSES — the actual
// requirement, not merely schema-validity).
func TestUpdateWritesValidExpectation(t *testing.T) {
	const threshold = 4
	// The produced Result: a challenge fired -> REVIEW with one finding, score total 1.
	res := result(aggregate.DecisionReview,
		finding("partitions-within-cap", "capped", "challenge", 1),
	)
	actual := ActualExpectation(res, threshold)

	// The authored golden is STALE/WRONG: it still pins APPROVE with no findings.
	authored := []byte("# partitions 12 -> 64 exceeds the cap -> challenge -> REVIEW.\ndecision: APPROVE\nfindings: []\n")

	updated, err := UpdateExpectationBlock(authored, actual)
	if err != nil {
		t.Fatalf("UpdateExpectationBlock: %v", err)
	}

	// The rewritten file re-validates against the FROZEN schema (fail-closed).
	reDecoded, err := LoadExpectation(updated)
	if err != nil {
		t.Fatalf("rewritten expect.yaml does not strict-decode against the frozen schema: %v\n%s", err, updated)
	}
	if reDecoded.Decision != "REVIEW" {
		t.Fatalf("rewritten decision = %q, want REVIEW\n%s", reDecoded.Decision, updated)
	}

	// A subsequent normal run PASSES: the rewritten expectation matches its own Result.
	reasons, err := Match(reDecoded, res, threshold)
	if err != nil {
		t.Fatalf("Match on rewritten golden: %v", err)
	}
	if len(reasons) != 0 {
		t.Fatalf("rewritten golden did not match its own Result, reasons: %v\n%s", reasons, updated)
	}
}

// TestUpdatePreservesAuthoredComments (REQ-E6-S05-02): authored comments around the
// expectation block survive --update (review-by-diff preserved). A naive
// yaml.Marshal(actual) would clobber them; the Node-surgery write keeps them on the
// key nodes they were authored against.
func TestUpdatePreservesAuthoredComments(t *testing.T) {
	const threshold = 4
	res := result(aggregate.DecisionReview,
		finding("partitions-within-cap", "capped", "challenge", 1),
	)
	actual := ActualExpectation(res, threshold)

	// Comments on the LEADING block (attaches to the first key, decision) AND on a
	// RETAINED non-first key (score) — proving key-level preservation generalizes past
	// decision, not just the document head.
	authored := []byte("# partitions 12 -> 64 exceeds the resolved cap 32 -> challenge fires -> REVIEW.\n" +
		"# NB: no findings[].path — D-054 makes a path assertion error-as-unsupported.\n" +
		"decision: APPROVE\nfindings: []\n" +
		"# score pins the risk arithmetic (points sum vs threshold).\n" +
		"score:\n  total: 0\n  threshold: 4\n")

	updated, err := UpdateExpectationBlock(authored, actual)
	if err != nil {
		t.Fatalf("UpdateExpectationBlock: %v", err)
	}

	for _, want := range []string{
		"# partitions 12 -> 64 exceeds the resolved cap 32 -> challenge fires -> REVIEW.",
		"# NB: no findings[].path — D-054 makes a path assertion error-as-unsupported.",
		"# score pins the risk arithmetic (points sum vs threshold).",
	} {
		if !strings.Contains(string(updated), want) {
			t.Fatalf("authored comment did not survive --update:\nwant substring: %s\ngot:\n%s", want, updated)
		}
	}
	// And it still re-validates (comment survival must not cost schema-validity).
	if _, err := LoadExpectation(updated); err != nil {
		t.Fatalf("comment-preserving rewrite broke schema-validity: %v\n%s", err, updated)
	}
}

// TestUpdateWriteDoubleRunStable (REQ-E6-S05-04): the --update write is deterministic —
// the same actuals over the same authored bytes yield byte-identical output.
func TestUpdateWriteDoubleRunStable(t *testing.T) {
	const threshold = 4
	res := result(aggregate.DecisionReview,
		finding("partitions-within-cap", "capped", "challenge", 1),
	)
	actual := ActualExpectation(res, threshold)
	authored := []byte("# keep me\ndecision: APPROVE\nfindings: []\n")

	first, err := UpdateExpectationBlock(authored, actual)
	if err != nil {
		t.Fatalf("UpdateExpectationBlock (first): %v", err)
	}
	second, err := UpdateExpectationBlock(authored, actual)
	if err != nil {
		t.Fatalf("UpdateExpectationBlock (second): %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("--update write not deterministic:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestUpdateRejectsUnrewritableOriginal hardens the fail-closed guards: an original
// that is not a single expectation MAPPING (a bare sequence) and malformed YAML are
// both errors, never a silent/garbled write.
func TestUpdateRejectsUnrewritableOriginal(t *testing.T) {
	actual := ActualExpectation(result(aggregate.DecisionApprove), 4)

	for name, original := range map[string][]byte{
		"a bare sequence, not an expectation mapping": []byte("- decision: APPROVE\n- findings: []\n"),
		"malformed yaml": []byte("decision: APPROVE\n: : :\n"),
	} {
		if _, err := UpdateExpectationBlock(original, actual); err == nil {
			t.Fatalf("%s: UpdateExpectationBlock succeeded, want error", name)
		}
	}
}
