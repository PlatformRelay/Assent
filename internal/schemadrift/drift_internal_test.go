package schemadrift

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitNonEmptyLines(t *testing.T) {
	got := splitNonEmptyLines("schemas/a\n\n schemas/b \n")
	if len(got) != 2 || got[0] != "schemas/a" || got[1] != "schemas/b" {
		t.Fatalf("got %#v", got)
	}
}

func TestAsObjectNil(t *testing.T) {
	obj, err := asObject(nil, "properties")
	if err != nil {
		t.Fatalf("asObject(nil): %v", err)
	}
	if len(obj) != 0 {
		t.Fatalf("expected empty object, got %#v", obj)
	}
}

func TestAsObjectNonObject(t *testing.T) {
	_, err := asObject("not-an-object", "properties")
	if err == nil {
		t.Fatal("expected error for non-object value")
	}
	if !strings.Contains(err.Error(), "expected object") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitShow(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	raw, err := gitShow(repoRoot, "HEAD:schemas/policy/v1alpha1/config.schema.json")
	if err != nil {
		t.Fatalf("gitShow: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty schema bytes")
	}
}

func TestCheckGitFrozenNoSchemaDrift(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@test.local")
	runGit(t, dir, "config", "user.name", "test")
	schemaDir := filepath.Join(dir, "schemas/policy/v1alpha1")
	if err := os.MkdirAll(schemaDir, 0o750); err != nil {
		t.Fatal(err)
	}
	schemaPath := filepath.Join(schemaDir, "config.schema.json")
	if err := os.WriteFile(schemaPath, []byte(`{"title":"Config"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	runGit(t, dir, "branch", "-M", "main")
	runGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	if err := CheckGitFrozenOrD088PresentationOnly(dir); err != nil {
		t.Fatalf("no drift must pass: %v", err)
	}
}

func TestCheckGitFrozenRejectsOtherSchema(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@test.local")
	runGit(t, dir, "config", "user.name", "test")
	schemaDir := filepath.Join(dir, "schemas/policy/v1alpha1")
	if err := os.MkdirAll(schemaDir, 0o750); err != nil {
		t.Fatal(err)
	}
	base := []byte(`{"title":"Config"}`)
	if err := os.WriteFile(filepath.Join(schemaDir, "config.schema.json"), base, 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	runGit(t, dir, "branch", "-M", "main")
	runGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	if err := os.WriteFile(filepath.Join(schemaDir, "other.schema.json"), base, 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", filepath.Join("schemas/policy/v1alpha1/other.schema.json"))
	err := CheckGitFrozenOrD088PresentationOnly(dir)
	if err == nil {
		t.Fatal("expected drift guard failure")
	}
	if !strings.Contains(err.Error(), "other.schema.json") {
		t.Fatalf("expected other schema named in error, got: %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) // #nosec G204 -- fixed program, test-controlled args building a throwaway repo.
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}
