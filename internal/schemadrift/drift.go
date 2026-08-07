// Package schemadrift helpers verify git-tracked schema changes stay within
// allowed epic fences: the E8 D-088 presentation block in config.schema.json,
// and the D-120 toolDigest description annotation in decision-record.schema.json.
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

	// d120DecisionRecordSchemaPath is the second (and only other) frozen schema
	// permitted to drift from the baseline, and only in the one annotation D-120
	// authorises: $defs.pins.properties.toolDigest.description. AUD-S04 makes the
	// toolDigest value a Go-build-info digest, and the published description has
	// to say so — an annotation, never a validation keyword, so records emitted by
	// v0.1.0 stay valid.
	d120DecisionRecordSchemaPath = "schemas/decision/v1alpha1/decision-record.schema.json"

	// d120DescriptionSentinel replaces the toolDigest description in BOTH
	// documents before they are compared, so "only that string differs" is one
	// total assertion over the whole document rather than a key walk that could
	// silently tolerate other annotation edits.
	d120DescriptionSentinel = "\x00d120-tooldigest-description\x00"
)

// d120ToolDigestPath is the JSON pointer, in segments, to the sole annotation the
// D-120 allowance may change.
var d120ToolDigestPath = []string{"$defs", "pins", "properties", "toolDigest"}

var d088AllowedPropertyKeys = map[string]struct{}{
	"presentation": {},
}

var d088AllowedDefKeys = map[string]struct{}{
	"presentation":            {},
	"presentationEnvOverride": {},
}

// CheckGitFrozenOrD088PresentationOnly reports whether schemas/ drift relative to
// origin/main is absent or limited to the two fenced edits: the E8 D-088
// presentation block in config.schema.json, and the D-120 toolDigest description
// annotation in decision-record.schema.json.
//
// The name is retained for its three call sites (one of which is outside this
// lane's ownership); it now means "frozen, or within an explicitly decided fence".
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
	// ValidateSchemaPathDrift has already restricted this set to the two fenced
	// literals, so each path is checked against exactly the change its decision
	// authorises — and an unrecognised one is a hard error, never a pass.
	for _, path := range schemaJSONPaths(changed) {
		oldJSON, err := gitShow(repoRoot, d088GitBaseline+":"+path)
		if err != nil {
			return fmt.Errorf("read baseline %s:%s: %w", d088GitBaseline, path, err)
		}
		newPath := filepath.Join(repoRoot, path)
		newJSON, err := os.ReadFile(newPath) //nolint:gosec // path is one of the two fenced schema literals
		if err != nil {
			return fmt.Errorf("read %s: %w", newPath, err)
		}
		switch path {
		case d088ConfigSchemaPath:
			if err := AllowedD088ConfigSchemaChange(oldJSON, newJSON); err != nil {
				return fmt.Errorf("config.schema.json: %w", err)
			}
		case d120DecisionRecordSchemaPath:
			if err := AllowedD120ToolDigestDescriptionChange(oldJSON, newJSON); err != nil {
				return fmt.Errorf("decision-record.schema.json: %w", err)
			}
		default:
			return fmt.Errorf("unfenced schema drift: %q", path)
		}
	}
	return nil
}

// AllowedD120ToolDigestDescriptionChange reports whether newJSON differs from
// oldJSON ONLY at $defs.pins.properties.toolDigest.description (D-120: annotation,
// not validation — published v0.1.0 records must stay valid).
//
// Both documents have that one string overwritten with a sentinel and are then
// compared whole. That deliberately does NOT normalise descriptions generally: a
// change to any other description — policySha's, say — survives normalisation and
// is rejected, so the fence stays a single-field fence rather than a blanket
// licence to edit annotations.
func AllowedD120ToolDigestDescriptionChange(oldJSON, newJSON []byte) error {
	oldDoc, err := parseJSONObject(oldJSON)
	if err != nil {
		return fmt.Errorf("parse baseline: %w", err)
	}
	newDoc, err := parseJSONObject(newJSON)
	if err != nil {
		return fmt.Errorf("parse candidate: %w", err)
	}
	if err := sealToolDigestDescription(oldDoc); err != nil {
		return fmt.Errorf("baseline: %w", err)
	}
	if err := sealToolDigestDescription(newDoc); err != nil {
		return fmt.Errorf("candidate: %w", err)
	}
	if !reflect.DeepEqual(oldDoc, newDoc) {
		return fmt.Errorf("changed more than %s.description (D-120 permits that annotation only)",
			strings.Join(d120ToolDigestPath, "."))
	}
	return nil
}

// sealToolDigestDescription replaces the toolDigest description with the sentinel
// in place. A missing node or a non-string description is an error, so removing
// the annotation (or retyping the toolDigest subschema) cannot pass as "unchanged".
func sealToolDigestDescription(doc map[string]any) error {
	node := doc
	for _, key := range d120ToolDigestPath {
		next, ok := node[key].(map[string]any)
		if !ok {
			return fmt.Errorf("no object at %s", strings.Join(d120ToolDigestPath, "."))
		}
		node = next
	}
	if _, ok := node["description"].(string); !ok {
		return fmt.Errorf("%s has no description string", strings.Join(d120ToolDigestPath, "."))
	}
	node["description"] = d120DescriptionSentinel
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
// schemas/ is one of the two fenced files — D-088 config.schema.json or D-120
// decision-record.schema.json — or that the set is empty.
func ValidateSchemaPathDrift(changed []string) error {
	changed = schemaJSONPaths(changed)
	if len(changed) == 0 {
		return nil
	}
	for _, path := range changed {
		if path != d088ConfigSchemaPath && path != d120DecisionRecordSchemaPath {
			return fmt.Errorf("schemas/ drift must be D-088 %s or D-120 %s only; also changed: %q",
				d088ConfigSchemaPath, d120DecisionRecordSchemaPath, path)
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
