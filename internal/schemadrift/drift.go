// Package schemadrift helpers verify git-tracked schema changes stay within
// allowed epic fences (E8 D-088 presentation block in config.schema.json).
package schemadrift

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	d088ConfigSchemaPath = "schemas/policy/v1alpha1/config.schema.json"
	d088GitBaseline      = "origin/main"
)

var d088AllowedPropertyKeys = map[string]struct{}{
	"presentation": {},
}

var d088AllowedDefKeys = map[string]struct{}{
	"presentation":            {},
	"presentationEnvOverride": {},
}

// CheckGitFrozenOrD088PresentationOnly reports whether schemas/ drift relative to
// origin/main is absent or limited to the E8 D-088 presentation block in
// config.schema.json.
func CheckGitFrozenOrD088PresentationOnly(repoRoot string) error {
	nameCmd := exec.Command("git", "diff", d088GitBaseline, "--name-only", "--", "schemas/")
	nameCmd.Dir = repoRoot
	nameOut, err := nameCmd.Output()
	if err != nil {
		return fmt.Errorf("git diff %s --name-only schemas/: %w", d088GitBaseline, err)
	}
	changed := splitNonEmptyLines(string(nameOut))
	if err := ValidateSchemaPathDrift(changed); err != nil {
		return err
	}
	if len(changed) == 0 {
		return nil
	}
	oldJSON, err := gitShow(repoRoot, d088GitBaseline+":"+d088ConfigSchemaPath)
	if err != nil {
		return fmt.Errorf("read baseline %s:%s: %w", d088GitBaseline, d088ConfigSchemaPath, err)
	}
	newPath := filepath.Join(repoRoot, d088ConfigSchemaPath)
	newJSON, err := os.ReadFile(newPath) //nolint:gosec // path joins repoRoot with a fixed schema literal
	if err != nil {
		return fmt.Errorf("read %s: %w", newPath, err)
	}
	if err := AllowedD088ConfigSchemaChange(oldJSON, newJSON); err != nil {
		return fmt.Errorf("config.schema.json: %w", err)
	}
	return nil
}

// AllowedD088ConfigSchemaChange reports whether newJSON differs from oldJSON only
// by adding or updating the D-088 presentation property and its dedicated $defs.
func AllowedD088ConfigSchemaChange(oldJSON, newJSON []byte) error {
	oldDoc, err := parseJSONObject(oldJSON)
	if err != nil {
		return fmt.Errorf("parse baseline: %w", err)
	}
	newDoc, err := parseJSONObject(newJSON)
	if err != nil {
		return fmt.Errorf("parse candidate: %w", err)
	}
	for key, oldVal := range oldDoc {
		switch key {
		case "properties", "$defs":
			continue
		default:
			newVal, ok := newDoc[key]
			if !ok {
				return fmt.Errorf("removed top-level %q (only presentation additions allowed)", key)
			}
			if !reflect.DeepEqual(oldVal, newVal) {
				return fmt.Errorf("changed top-level %q (only presentation additions allowed)", key)
			}
		}
	}
	for key, newVal := range newDoc {
		switch key {
		case "properties", "$defs":
			continue
		default:
			if _, ok := oldDoc[key]; !ok {
				return fmt.Errorf("added top-level %q (only presentation additions allowed)", key)
			}
			if !reflect.DeepEqual(oldDoc[key], newVal) {
				return fmt.Errorf("changed top-level %q (only presentation additions allowed)", key)
			}
		}
	}
	if err := allowedObjectDiff(oldDoc["properties"], newDoc["properties"], "properties", d088AllowedPropertyKeys); err != nil {
		return err
	}
	if err := allowedObjectDiff(oldDoc["$defs"], newDoc["$defs"], "$defs", d088AllowedDefKeys); err != nil {
		return err
	}
	return nil
}

// ValidateSchemaPathDrift ensures every changed frozen schema JSON path under
// schemas/ is the lone D-088 config.schema.json file (or the set is empty).
func ValidateSchemaPathDrift(changed []string) error {
	changed = schemaJSONPaths(changed)
	if len(changed) == 0 {
		return nil
	}
	for _, path := range changed {
		if path != d088ConfigSchemaPath {
			return fmt.Errorf("schemas/ drift must be D-088 %s only; also changed: %q", d088ConfigSchemaPath, path)
		}
	}
	return nil
}

func schemaJSONPaths(paths []string) []string {
	var out []string
	for _, path := range paths {
		if strings.HasSuffix(path, ".schema.json") {
			out = append(out, path)
		}
	}
	return out
}

func allowedObjectDiff(oldVal, newVal any, label string, allowedKeys map[string]struct{}) error {
	oldObj, err := asObject(oldVal, label)
	if err != nil {
		return err
	}
	newObj, err := asObject(newVal, label)
	if err != nil {
		return err
	}
	for key, oldEntry := range oldObj {
		if _, ok := allowedKeys[key]; ok {
			continue
		}
		newEntry, ok := newObj[key]
		if !ok {
			return fmt.Errorf("%s: removed %q (only presentation keys may change)", label, key)
		}
		if !reflect.DeepEqual(oldEntry, newEntry) {
			return fmt.Errorf("%s: changed %q (only presentation keys may change)", label, key)
		}
	}
	for key, newEntry := range newObj {
		if _, ok := allowedKeys[key]; ok {
			continue
		}
		oldEntry, ok := oldObj[key]
		if !ok {
			return fmt.Errorf("%s: added %q (only presentation keys may change)", label, key)
		}
		if !reflect.DeepEqual(oldEntry, newEntry) {
			return fmt.Errorf("%s: changed %q (only presentation keys may change)", label, key)
		}
	}
	return nil
}

func parseJSONObject(raw []byte) (map[string]any, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func asObject(val any, label string) (map[string]any, error) {
	if val == nil {
		return map[string]any{}, nil
	}
	obj, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected object, got %T", label, val)
	}
	return obj, nil
}

func gitShow(repoRoot, spec string) ([]byte, error) {
	cmd := exec.Command("git", "show", spec) //nolint:gosec // spec is repoRoot + fixed schema path literals only
	cmd.Dir = repoRoot
	return cmd.Output()
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
