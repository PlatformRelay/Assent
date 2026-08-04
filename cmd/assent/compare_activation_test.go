package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const profileYAML = `apiVersion: assent.dev/v1alpha1
kind: PolicyProfile
metadata:
  name: %s
spec:
  writes: false
  environments: ["*"]
  classes: ["*"]
  packs: [%s]
`

const strictPackRuleYAML = `apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: strict-rules
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
          cel: "new >= old"
          message: "must not shrink"
      onFailure:
        effect: block
        code: partitions.shrunk
`

const permissivePackRuleYAML = `apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: permissive-rules
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
          cel: "true"
          message: "must not shrink"
      onFailure:
        effect: block
        code: partitions.shrunk
`

func writeCompareActivationDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("bundle.json", compareBundleJSON)
	write("binding.yaml", compareBindingYAML)
	write("baseline.yaml", fmtProfile("baseline-strict", "strict"))
	write("candidate.yaml", fmtProfile("candidate-permissive", "permissive"))
	write(".assent/packs/strict/rules/non-destructive.yaml", strictPackRuleYAML)
	write(".assent/packs/permissive/rules/non-destructive.yaml", permissivePackRuleYAML)
	return dir
}

func fmtProfile(name, pack string) string {
	return fmt.Sprintf(profileYAML, name, pack)
}

// REQ-PCS-S01-05: profiles differing only in spec.packs produce observable decision deltas.
func TestCompareUsesResolvedProfilePacks(t *testing.T) {
	dir := writeCompareActivationDir(t)
	var out, errb bytes.Buffer
	code := runCompare([]string{dir}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (baseline BLOCK, candidate APPROVE via pack activation); stdout=%q stderr=%q", code, out.String(), errb.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("baseline=BLOCK")) || !bytes.Contains(out.Bytes(), []byte("candidate=APPROVE")) {
		t.Errorf("stdout = %q, want baseline=BLOCK and candidate=APPROVE from resolved packs", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("delta=newly-auto-mergeable")) {
		t.Errorf("stdout = %q, want newly-auto-mergeable delta from pack-only profile difference", out.String())
	}
}
