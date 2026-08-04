package schemadrift_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/schemadrift"
)

func TestCheckGitFrozenOrD088PresentationOnly(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	if err := schemadrift.CheckGitFrozenOrD088PresentationOnly(repoRoot); err != nil {
		diffCmd := exec.Command("git", "diff", "--name-only", "--", "schemas/")
		diffCmd.Dir = repoRoot
		diff, _ := diffCmd.Output()
		t.Fatalf("schema drift check: %v\nchanged files:\n%s", err, diff)
	}
}
