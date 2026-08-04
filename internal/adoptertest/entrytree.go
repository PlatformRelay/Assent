package adoptertest

// entrytree.go is the P5-E6-S02 Part-B input-side seam: it reconstructs, from a
// case's base/↔head/, the whole-entry OBJECT for each change's EntryRef and
// populates the Part-A aggregate.EvalChange.Entry/OldEntry so an entry-scoped
// predicate (`entry.owner`) binds against the object — NOT the change's scalar
// New/Old. It reuses change.Entries (which shares change.DiffEntries' identity
// keying, single source of truth, no drift) and threads the optional case mr.yaml →
// aggregate.MR and approval.yaml → aggregate.ApprovalContext. Pure; no I/O.

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// singleEntryConfig returns the pack's governed-collection config mapped to a
// change.EntryConfig, and whether exactly one is declared. The harness reconstructs
// entries only for an UNAMBIGUOUS single-entry pack:
//   - 0 entries ⇒ (…, false): the pack is document-mode (the S01 path).
//   - 1 entry   ⇒ (cfg, true): reconstruct + diff via that config.
//   - >1 entries ⇒ (…, false): AMBIGUOUS — policy.Entry carries no file selector,
//     so which config governs the changed file is undecided (logged as an OQ). The
//     harness falls back to document mode (fail-safe: no entry object is fabricated
//     for the wrong config; entry-scoped predicates then fail safe to REVIEW).
//
// The change.EntryConfig.Label is the entry's map key, so the reconstructed keys
// (`<label>:<id>`) match the EntryRef DiffEntries stamps onto each change.
func singleEntryConfig(pol *policy.MergePolicy) (change.EntryConfig, bool) {
	if pol == nil || len(pol.Spec.Entries) != 1 {
		return change.EntryConfig{}, false
	}
	for label, e := range pol.Spec.Entries {
		return change.EntryConfig{
			Mode:     change.EntryMode(e.Mode),
			Root:     e.Root,
			Identity: e.Identity.Pointer,
			Label:    label,
		}, true
	}
	return change.EntryConfig{}, false
}

// populateEntries reconstructs the head/base entry objects for the changed file and
// sets EvalChange.Entry/OldEntry for each change whose EntryRef resolves to a FULLY
// reconstructed entry. It is index-aligned with cs.Changes (BuildEvaluationInput
// preserves order 1:1) so an entry binds the change it was tagged to.
//
// FAIL-SAFE (REQ-E6-S02-07): if change.Entries cannot FULLY reconstruct the tree
// (an unprojectable/partial entry, a bad identity), it returns an error and every
// Entry/OldEntry is left nil — Part A then binds the scalar New/Old, on which
// `has(entry.x)` ERRORS → REVIEW, never a permissive partial-map branch. A change
// whose EntryRef is present on only ONE side (an add has no base entry; a delete no
// head entry) leaves the absent side nil (never substitutes an empty {} object).
func populateEntries(in *aggregate.EvaluationInput, cs change.ChangeSet, c Case, cfg change.EntryConfig) {
	headEntries, herr := change.Entries(c.File, c.Head, cfg)
	baseEntries, berr := change.Entries(c.File, c.Base, cfg)
	if herr != nil || berr != nil {
		return // fail-safe: leave every Entry nil (scalar fallback)
	}
	for i := range cs.Changes {
		ref := cs.Changes[i].EntryRef
		if ref == "" {
			continue
		}
		if obj, ok := headEntries[ref]; ok {
			in.ChangeSet.Changes[i].Entry = obj
		}
		if obj, ok := baseEntries[ref]; ok {
			in.ChangeSet.Changes[i].OldEntry = obj
		}
	}
}

// MapMR lifts an optional case mr.yaml (author, sourceBranch, targetBranch, labels)
// into aggregate.MR bound to the `mr` predicate scope. Empty/whitespace input (the
// mr.yaml-absent case) yields the zero MR — the ADR-0014 default (no author, no
// labels, no branches): the fail-safe default, since ADR-0014 declares defaults
// exist but pins no specific values. An unknown field is a located rejection.
func MapMR(raw []byte) (aggregate.MR, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return aggregate.MR{}, nil
	}
	var m struct {
		Author       string   `yaml:"author"`
		SourceBranch string   `yaml:"sourceBranch"`
		TargetBranch string   `yaml:"targetBranch"`
		Labels       []string `yaml:"labels"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return aggregate.MR{}, fmt.Errorf("mr.yaml: %w", err)
	}
	return aggregate.MR{
		Author:       m.Author,
		SourceBranch: m.SourceBranch,
		TargetBranch: m.TargetBranch,
		Labels:       m.Labels,
	}, nil
}

// MapApproval lifts an optional case approval.yaml into an aggregate.ApprovalContext
// so a require-review obligation can be SATISFIED without a live forge (the forge
// tier E4 owns the real fetch). The stub carries the evaluated sourceSha and, per
// governed subject entryRef, an ApprovalEvidence whose pins.sourceSha must MATCH for
// satisfaction (a mismatch/absent sha never satisfies — the top fail-open trap).
// Empty/whitespace input (approval.yaml absent) yields a nil context: no evidence,
// so a require-review obligation stays unsatisfied → REVIEW (never satisfied by
// absence). An unknown field is a located rejection.
func MapApproval(raw []byte) (*aggregate.ApprovalContext, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var doc struct {
		SourceSha string `yaml:"sourceSha"`
		Evidence  []struct {
			Subject             string `yaml:"subject"`
			VerifyingCapability string `yaml:"verifyingCapability"`
			ApprovalsRequired   int    `yaml:"approvalsRequired"`
			ApprovedBy          []struct {
				ID       string `yaml:"id"`
				Username string `yaml:"username"`
				IsAuthor bool   `yaml:"isAuthor"`
			} `yaml:"approvedBy"`
			Eligibility []string `yaml:"eligibility"`
			Expired     bool     `yaml:"expired"`
			Pins        struct {
				SourceSha string `yaml:"sourceSha"`
			} `yaml:"pins"`
		} `yaml:"evidence"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("approval.yaml: %w", err)
	}

	evidence := map[string]*aggregate.ApprovalEvidence{}
	for _, e := range doc.Evidence {
		if e.Subject == "" {
			return nil, fmt.Errorf("approval.yaml: an evidence entry declares no subject")
		}
		approvers := make([]aggregate.Approver, len(e.ApprovedBy))
		for i, a := range e.ApprovedBy {
			approvers[i] = aggregate.Approver{ID: a.ID, Username: a.Username, IsAuthor: a.IsAuthor}
		}
		evidence[e.Subject] = &aggregate.ApprovalEvidence{
			VerifyingCapability: e.VerifyingCapability,
			ApprovalsRequired:   e.ApprovalsRequired,
			ApprovedBy:          approvers,
			Eligibility:         e.Eligibility,
			Expired:             e.Expired,
			Pins:                aggregate.ApprovalPins{SourceSha: e.Pins.SourceSha},
		}
	}
	return &aggregate.ApprovalContext{SourceSha: doc.SourceSha, Evidence: evidence}, nil
}
