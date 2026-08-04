package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// compareBundleJSON is a schema-valid ReplayBundle whose pre-built EvaluationInput
// carries a partitions SHRINK 12 -> 6 — a destructive change the baseline blocks.
const compareBundleJSON = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "ReplayBundle",
  "pins": {
    "toolVersion": "0.0.0-test",
    "toolDigest": "sha256:aaaa",
    "policySha": "sha256:bbbb",
    "sourceSha": "cccc",
    "targetSha": "dddd",
    "mergeResultDigest": "sha256:eeee",
    "factsResolvedAt": {}
  },
  "evaluationInput": {
    "apiVersion": "assent.dev/v1alpha1",
    "kind": "EvaluationInput",
    "changeSet": {"changes": [
      {"subject": "topic-registry:orders.events.v1", "file": "topics/prod/orders.events.v1.yaml", "path": "/partitions", "kind": "modify", "old": 12, "new": 6}
    ]},
    "facts": {},
    "mr": {"author": "alice", "sourceBranch": "topic/shrink", "targetBranch": "main"},
    "require": ["non-destructive"]
  }
}`

const compareBindingYAML = `apiVersion: assent.dev/v1alpha1
kind: RulesetBinding
bindings:
  - class: kafka-topic
    environment: prod
    packs: [topics]
    risk: { threshold: 4 }
    require: [non-destructive]
`

// mergePolicyYAML builds a single-rule MergePolicy proving non-destructive over
// the /partitions change with a block onFailure effect; cel is the leaf predicate
// and msg its leaf message.
func mergePolicyYAML(name, cel, msg string) string {
	return mergePolicyYAMLEffect(name, cel, msg, "block")
}

// mergePolicyYAMLEffect is mergePolicyYAML with an explicit onFailure effect, so a
// test can vary the effect (e.g. require-review) to produce a BLOCK->REVIEW delta.
func mergePolicyYAMLEffect(name, cel, msg, effect string) string {
	return `apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: ` + name + `
spec:
  rules:
    - name: non-destructive
      phase: enforce
      match:
        valueChanges:
          pointers: ["/partitions"]
          kinds: [modify]
      prove:
        obligation: non-destructive
        when:
          cel: "` + cel + `"
          message: "` + msg + `"
      onFailure:
        effect: ` + effect + `
        code: partitions.shrunk
`
}

func writeCompareDir(t *testing.T, baseline, candidate string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("bundle.json", compareBundleJSON)
	write("binding.yaml", compareBindingYAML)
	write("baseline.yaml", baseline)
	write("candidate.yaml", candidate)
	return dir
}

// REQ-E6-S09-03: the chosen promotion gate maps pass/fail to an exit code, and an
// explanation-only (wording-only) delta never trips the gate.
func TestCompareGateExitCodes(t *testing.T) {
	t.Run("newly-auto-mergeable widening fails the gate (exit 1)", func(t *testing.T) {
		// Candidate relaxes the guard (`true`) so the shrink now APPROVEs.
		dir := writeCompareDir(t,
			mergePolicyYAML("prod-strict-6", "new >= old", "must not shrink"),
			mergePolicyYAML("prod-strict-7", "true", "must not shrink"),
		)
		var out, errb bytes.Buffer
		code := runCompare([]string{dir}, &out, &errb)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1 (gate FAIL); stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		if !bytes.Contains(out.Bytes(), []byte("verdict=FAIL")) {
			t.Errorf("stdout = %q, want it to report verdict=FAIL", out.String())
		}
	})

	t.Run("explanation-only delta passes the gate (exit 0)", func(t *testing.T) {
		// Identical predicate/effect on both sides — only the leaf message differs.
		dir := writeCompareDir(t,
			mergePolicyYAML("prod-6", "new >= old", "partitions must not shrink below the baseline"),
			mergePolicyYAML("prod-7", "new >= old", "partitions may not be reduced"),
		)
		var out, errb bytes.Buffer
		code := runCompare([]string{dir}, &out, &errb)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (explanation-only never trips the gate); stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		if !bytes.Contains(out.Bytes(), []byte("delta=explanation-only")) || !bytes.Contains(out.Bytes(), []byte("verdict=PASS")) {
			t.Errorf("stdout = %q, want delta=explanation-only + verdict=PASS", out.String())
		}
	})

	t.Run("fail-closed classification exits non-zero, never a silent promote (exit 2)", func(t *testing.T) {
		// baseline BLOCKs; candidate downgrades the effect to require-review, so the
		// delta is BLOCK -> REVIEW — a real difference the seed does not classify. It
		// MUST exit non-zero (2, distinct from a gate FAIL) — never a silent exit-0 pass.
		dir := writeCompareDir(t,
			mergePolicyYAML("prod-6", "new >= old", "must not shrink"),
			mergePolicyYAMLEffect("prod-7", "new >= old", "must not shrink", "require-review"),
		)
		var out, errb bytes.Buffer
		code := runCompare([]string{dir}, &out, &errb)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2 (fail-closed classification error); stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		if !bytes.Contains(errb.Bytes(), []byte("classifies")) {
			t.Errorf("stderr = %q, want it to name the fail-closed classification error", errb.String())
		}
	})

	t.Run("missing argument is a usage error (exit 2)", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := runCompare(nil, &out, &errb); code != 2 {
			t.Fatalf("exit code = %d, want 2 (usage)", code)
		}
	})
}
