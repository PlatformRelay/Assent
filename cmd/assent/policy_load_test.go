package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// REQ-P4-E1-S01-02: policy under .assent/** is loaded from the TARGET ref only
// (ADR-0015 §1), never the MR source branch — the source branch contributes
// only the material under judgment. A golden builds a temp git repo whose
// .assent/ policy differs between the source branch and the target ref and
// asserts the loaded bytes equal the target-ref version.
func TestPolicyLoadsFromTargetRef(t *testing.T) {
	repo := t.TempDir()

	targetPolicy := "class: strict\n# target-ref policy (trusted)\n"
	sourcePolicy := "class: permissive\n# source-branch policy (MUST NOT be loaded)\n"

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) // #nosec G204 -- fixed program, test-controlled args building a throwaway repo.
		cmd.Dir = repo
		// Deterministic identity so commits succeed without global git config.
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	git("init", "-q", "-b", "main")
	// Target ref (main) carries the trusted policy.
	writeFile(".assent/config.yaml", targetPolicy)
	git("add", ".")
	git("commit", "-q", "-m", "target policy")

	// Source branch edits its own policy — the classic F1 attack.
	git("checkout", "-q", "-b", "feature")
	writeFile(".assent/config.yaml", sourcePolicy)
	git("add", ".")
	git("commit", "-q", "-m", "source edits policy")

	// Resolve the target ref SHA and check out the source branch (so the
	// working tree holds the SOURCE version — proving the loader ignores it).
	targetSHA := revParse(t, repo, "main")
	git("checkout", "-q", "feature")

	got, err := LoadPolicyFromRef(repo, targetSHA, ".assent/config.yaml")
	if err != nil {
		t.Fatalf("LoadPolicyFromRef: %v", err)
	}
	if string(got) != targetPolicy {
		t.Fatalf("policy loaded from wrong ref:\n got: %q\nwant: %q (target-ref version, ADR-0015 §1)", got, targetPolicy)
	}
	if string(got) == sourcePolicy {
		t.Fatal("loader returned the SOURCE-branch policy — trust boundary violated (ADR-0015 §1 F1)")
	}
}

func revParse(t *testing.T, repo, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref) // #nosec G204 -- fixed program; ref is test-controlled.
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v", ref, err)
	}
	return string(bytes.TrimSpace(out))
}
