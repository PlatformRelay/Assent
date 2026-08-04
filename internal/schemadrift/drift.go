// Package schemadrift helpers verify git-tracked schema changes stay within
// allowed epic fences (E8 D-088 presentation block in config.schema.json).
package schemadrift

import (
	"fmt"
	"os/exec"
	"strings"
)

const d088ConfigSchemaPath = "schemas/policy/v1alpha1/config.schema.json"

// CheckGitFrozenOrD088PresentationOnly reports whether schemas/ drift is absent or
// limited to the E8 D-088 presentation block in config.schema.json.
func CheckGitFrozenOrD088PresentationOnly(repoRoot string) error {
	nameCmd := exec.Command("git", "diff", "--name-only", "--", "schemas/")
	nameCmd.Dir = repoRoot
	nameOut, err := nameCmd.Output()
	if err != nil {
		return fmt.Errorf("git diff --name-only schemas/: %w", err)
	}
	changed := splitNonEmptyLines(string(nameOut))
	if len(changed) == 0 {
		return nil
	}
	if len(changed) != 1 || changed[0] != d088ConfigSchemaPath {
		return fmt.Errorf("schemas/ drift must be D-088 %s only; changed: %v", d088ConfigSchemaPath, changed)
	}
	diffCmd := exec.Command("git", "diff", "--", d088ConfigSchemaPath)
	diffCmd.Dir = repoRoot
	diffOut, err := diffCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git diff %s: %w\n%s", d088ConfigSchemaPath, err, diffOut)
	}
	diff := string(diffOut)
	if !strings.Contains(diff, "presentation") {
		return fmt.Errorf("config.schema.json drift must be presentation block (D-088), got:\n%s", diff)
	}
	return nil
}

func splitNonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
