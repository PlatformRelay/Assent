package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// REQ-P4-E1-S01-01: cmd/assent reads the GitLab CI environment
// (CI_PROJECT_ID, CI_MERGE_REQUEST_IID, CI_MERGE_REQUEST_SOURCE_BRANCH_SHA,
// CI_MERGE_REQUEST_TARGET_BRANCH_SHA/ref) plus CLI flags and assembles an
// EvaluationInput carrying the MR metadata that validates against the frozen
// schemas/decision/v1alpha1/evaluation-input.schema.json. The pinned SHAs +
// project/MR identity are carried in a separate cmd-level Pins struct destined
// for DecisionRecord.pins (the frozen EvaluationInput schema, top-level and mr
// both additionalProperties:false, has no SHA field — never edit the schema).
// The clock used for freshness is injected via a seam, never time.Now() in core.
func TestAssembleEvaluationInput(t *testing.T) {
	// A fixture GitLab CI environment (what cmd/assent reads from os.Getenv in
	// main, injected here for testability — the only place env is read).
	env := CIEnv{
		ProjectID:       "4242",
		MergeRequestIID: "17",
		SourceBranch:    "topic/orders-partitions",
		TargetBranch:    "main",
		SourceBranchSHA: "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678",
		TargetBranchSHA: "0f1e2d3c4b5a69788796a5b4c3d2e1f001234567",
		Author:          "alice",
	}

	// The differ (S02) fills changeSet/facts/require later; the adapter takes
	// them as parameters. Supply a one-entry fixture so the doc validates
	// (changeSet.changes has minItems: 1).
	fixture := AssemblyInputs{
		Changes: []Change{{
			Subject: "file:topics/prod/orders.events.v1.yaml",
			File:    "topics/prod/orders.events.v1.yaml",
			Path:    "/partitions",
			Kind:    "modify",
			Old:     6,
			New:     12,
		}},
		Require: []string{"non-destructive"},
		Labels:  []string{"kafka"},
	}

	// Injected clock seam — never time.Now() inside core.
	clock := func() time.Time { return time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC) }

	input, pins, err := AssembleEvaluationInput(env, fixture, clock)
	if err != nil {
		t.Fatalf("AssembleEvaluationInput: %v", err)
	}

	// --- The EvaluationInput validates against the frozen schema. ---
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal EvaluationInput: %v", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unmarshal for validation: %v", err)
	}
	if err := schemas.EvaluationInputSchema.Validate(doc); err != nil {
		t.Fatalf("assembled EvaluationInput does not validate against the frozen schema: %v\ndoc: %s", err, raw)
	}

	// --- MR metadata is carried on the EvaluationInput (branch names + author). ---
	if input.MR.Author != "alice" {
		t.Errorf("mr.author = %q, want alice", input.MR.Author)
	}
	if input.MR.SourceBranch != "topic/orders-partitions" {
		t.Errorf("mr.sourceBranch = %q, want topic/orders-partitions", input.MR.SourceBranch)
	}
	if input.MR.TargetBranch != "main" {
		t.Errorf("mr.targetBranch = %q, want main", input.MR.TargetBranch)
	}
	if input.APIVersion != "assent.dev/v1alpha1" || input.Kind != "EvaluationInput" {
		t.Errorf("apiVersion/kind = %q/%q, want assent.dev/v1alpha1/EvaluationInput", input.APIVersion, input.Kind)
	}

	// --- Pinned SHAs + project/MR identity are carried out-of-band (for pins). ---
	if pins.SourceSHA != env.SourceBranchSHA {
		t.Errorf("pins.SourceSHA = %q, want %q", pins.SourceSHA, env.SourceBranchSHA)
	}
	if pins.TargetSHA != env.TargetBranchSHA {
		t.Errorf("pins.TargetSHA = %q, want %q", pins.TargetSHA, env.TargetBranchSHA)
	}
	if pins.ProjectID != "4242" || pins.MergeRequestIID != "17" {
		t.Errorf("pins project/iid = %q/%q, want 4242/17", pins.ProjectID, pins.MergeRequestIID)
	}

	// --- The SHAs must NOT leak into the schema-valid EvaluationInput JSON
	// (the frozen schema has no place for them; additionalProperties:false). ---
	if bytes.Contains(raw, []byte(env.SourceBranchSHA)) || bytes.Contains(raw, []byte(env.TargetBranchSHA)) {
		t.Errorf("EvaluationInput JSON must not carry pinned SHAs (they belong in DecisionRecord.pins): %s", raw)
	}

	// --- The injected clock is threaded (not time.Now); the freshness anchor
	// the adapter stamps must equal the injected value. ---
	if !pins.EvaluatedAt.Equal(clock()) {
		t.Errorf("pins.EvaluatedAt = %v, want injected clock %v", pins.EvaluatedAt, clock())
	}
}

// A missing required CI variable is an assembly error, not a silent empty field
// that would produce an unpinned decision (fail-safe, GUIDELINES §2).
func TestAssembleEvaluationInputRejectsMissingPin(t *testing.T) {
	env := CIEnv{
		ProjectID:       "4242",
		MergeRequestIID: "17",
		SourceBranch:    "feature",
		TargetBranch:    "main",
		// SourceBranchSHA intentionally absent.
		TargetBranchSHA: "0f1e2d3c4b5a69788796a5b4c3d2e1f001234567",
		Author:          "alice",
	}
	fixture := AssemblyInputs{
		Changes: []Change{{Subject: "file:x", File: "x", Path: "", Kind: "modify", Old: 1, New: 2}},
		Require: []string{},
	}
	clock := func() time.Time { return time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC) }
	if _, _, err := AssembleEvaluationInput(env, fixture, clock); err == nil {
		t.Fatal("expected an error when a required pinned SHA is missing, got nil")
	}
}
