package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// CIEnv is the GitLab CI environment cmd/assent reads (the ONLY place env/CI
// variables are read — internal/core and internal/change receive only pinned
// values, never the environment; ADR-0015 §1, GUIDELINES §5). Field names
// mirror the GitLab predefined variables:
//
//	ProjectID       <- CI_PROJECT_ID
//	MergeRequestIID <- CI_MERGE_REQUEST_IID
//	SourceBranch    <- CI_MERGE_REQUEST_SOURCE_BRANCH_NAME
//	TargetBranch    <- CI_MERGE_REQUEST_TARGET_BRANCH_NAME
//	SourceBranchSHA <- CI_MERGE_REQUEST_SOURCE_BRANCH_SHA
//	TargetBranchSHA <- CI_MERGE_REQUEST_TARGET_BRANCH_SHA
//	Author          <- GITLAB_USER_LOGIN (MR author)
type CIEnv struct {
	ProjectID       string
	MergeRequestIID string
	SourceBranch    string
	TargetBranch    string
	SourceBranchSHA string
	TargetBranchSHA string
	Author          string
}

// readCIEnv populates a CIEnv from the process environment. This is the single
// os.Getenv boundary; downstream code takes the returned struct by value.
func readCIEnv() CIEnv {
	return CIEnv{
		ProjectID:       os.Getenv("CI_PROJECT_ID"),
		MergeRequestIID: os.Getenv("CI_MERGE_REQUEST_IID"),
		SourceBranch:    os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"),
		TargetBranch:    os.Getenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME"),
		SourceBranchSHA: os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_SHA"),
		TargetBranchSHA: os.Getenv("CI_MERGE_REQUEST_TARGET_BRANCH_SHA"),
		Author:          os.Getenv("GITLAB_USER_LOGIN"),
	}
}

// Clock is the injected time seam (ADR-0017 §4): freshness/maxAge arming uses
// this, never time.Now() inside internal/core. cmd/assent binds it to
// time.Now (or a --now override) and threads the resolved value down as data.
type Clock func() time.Time

// Change is one canonical ChangeSet entry as the frozen EvaluationInput schema
// requires (schemas/decision/v1alpha1/evaluation-input.schema.json #/$defs/change).
// The S02 differ produces these; at S01 the adapter accepts them as input.
type Change struct {
	Subject string `json:"subject"`
	File    string `json:"file"`
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	// Old/New carry no omitempty: a real modify of a zero-value (replicas 3->0,
	// enabled true->false, "x"->"") must serialize the field, not drop it — an
	// absent new: 0 would read downstream as "no new value" (fail-open). For
	// modify entries both are always present; the schema permits any value.
	Old any `json:"old"`
	New any `json:"new"`
}

// AssemblyInputs are the non-environment inputs to EvaluationInput assembly:
// the differ's ChangeSet, resolved facts, the binding's require list, and MR
// labels. Kept separate from CIEnv so the environment/CI boundary is explicit.
type AssemblyInputs struct {
	Changes []Change
	Require []string
	Labels  []string
}

// mrMeta is the schema's #/$defs/mr object (branch names + author + labels).
type mrMeta struct {
	Author       string   `json:"author"`
	SourceBranch string   `json:"sourceBranch"`
	TargetBranch string   `json:"targetBranch"`
	Labels       []string `json:"labels,omitempty"`
}

// changeSet is the schema's #/$defs/changeSet object.
type changeSet struct {
	Changes []Change `json:"changes"`
}

// EvaluationInput is the Go projection of the frozen
// schemas/decision/v1alpha1/evaluation-input.schema.json contract. It carries
// ONLY what that schema permits (top-level additionalProperties:false): the
// pinned SHAs and project/MR identity are NOT here — the schema has no field
// for them; they travel in Pins to DecisionRecord.pins (S04).
type EvaluationInput struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	ChangeSet  changeSet      `json:"changeSet"`
	Facts      map[string]any `json:"facts"`
	MR         mrMeta         `json:"mr"`
	Require    []string       `json:"require"`
}

// Pins are the out-of-band pinned identity + SHAs the adapter derives from the
// CI environment, destined for DecisionRecord.pins (ADR-0015 §2 SHA-guard,
// S04). They are deliberately NOT part of the schema-valid EvaluationInput
// (the frozen schema has no place for them). EvaluatedAt records the injected
// clock value used for this evaluation, never a raw time.Now() reading.
type Pins struct {
	ProjectID       string
	MergeRequestIID string
	SourceSHA       string
	TargetSHA       string
	EvaluatedAt     time.Time
}

// AssembleEvaluationInput builds a schema-valid EvaluationInput (MR metadata +
// the supplied ChangeSet/facts/require) plus the out-of-band Pins carrying the
// pinned SHAs and project/MR identity, stamping the injected clock value onto
// Pins.EvaluatedAt. A missing required pinned value (source/target SHA, project,
// MR IID, target branch) is an error rather than a silently unpinned decision
// (fail-safe, GUIDELINES §2).
func AssembleEvaluationInput(env CIEnv, in AssemblyInputs, now Clock) (*EvaluationInput, *Pins, error) {
	for _, req := range []struct{ name, val string }{
		{"CI_PROJECT_ID", env.ProjectID},
		{"CI_MERGE_REQUEST_IID", env.MergeRequestIID},
		{"CI_MERGE_REQUEST_SOURCE_BRANCH_SHA", env.SourceBranchSHA},
		{"CI_MERGE_REQUEST_TARGET_BRANCH_SHA", env.TargetBranchSHA},
		{"CI_MERGE_REQUEST_TARGET_BRANCH_NAME", env.TargetBranch},
	} {
		if req.val == "" {
			return nil, nil, fmt.Errorf("assemble EvaluationInput: required CI variable %s is empty", req.name)
		}
	}
	if now == nil {
		return nil, nil, fmt.Errorf("assemble EvaluationInput: an injected clock is required (never time.Now in core)")
	}

	facts := in.Facts()
	input := &EvaluationInput{
		APIVersion: "assent.dev/v1alpha1",
		Kind:       "EvaluationInput",
		ChangeSet:  changeSet{Changes: in.Changes},
		Facts:      facts,
		MR: mrMeta{
			Author:       env.Author,
			SourceBranch: env.SourceBranch,
			TargetBranch: env.TargetBranch,
			Labels:       in.Labels,
		},
		Require: nonNilStrings(in.Require),
	}
	pins := &Pins{
		ProjectID:       env.ProjectID,
		MergeRequestIID: env.MergeRequestIID,
		SourceSHA:       env.SourceBranchSHA,
		TargetSHA:       env.TargetBranchSHA,
		EvaluatedAt:     now(),
	}
	return input, pins, nil
}

// Facts returns an always-non-nil facts map so the marshalled EvaluationInput
// carries "facts": {} rather than "facts": null (the schema requires facts as
// an object). At S01 the run is provider-less, so it is empty.
func (in AssemblyInputs) Facts() map[string]any { return map[string]any{} }

// nonNilStrings ensures require marshals to [] not null (schema wants an array).
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// LoadPolicyFromRef reads a policy file (.assent/**) from a git ref by shelling
// out to `git cat-file blob -- <ref>:<path>` in repoDir — ALWAYS the TARGET
// ref, never the MR source branch (ADR-0015 §1). Shelling to git is confined
// to cmd/assent (internal/core and internal/change stay pure); the loaded
// bytes are passed down as trusted data. The source branch's working-tree copy
// is deliberately ignored, closing the F1 self-vouching gap.
//
// The `--` end-of-options separator neutralizes argument injection: an
// attacker-influenced ref beginning with "-" (e.g. "--upload-pack=...") is
// forced to be treated as an object name, not a git option (F4 hardening). We
// use `cat-file blob` rather than `git show` because `git show -- <rev>:<path>`
// treats the argument as a pathspec and prints the wrong object, whereas
// `cat-file blob` resolves the <rev>:<path> object and still honours `--`.
func LoadPolicyFromRef(repoDir, targetRef, path string) ([]byte, error) {
	cmd := exec.Command("git", "cat-file", "blob", "--", targetRef+":"+path) // #nosec G204 -- fixed program; `--` blocks option injection; ref+path identify a trusted target-ref blob, read-only.
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("load policy %q from target ref %q: %w", path, targetRef, err)
	}
	return out, nil
}
