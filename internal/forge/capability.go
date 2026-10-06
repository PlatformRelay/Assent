package forge

import (
	"fmt"
	"sort"
	"strings"
)

// capability.go is the NEUTRAL capability model (E10-S04 / ADR-0021 item 3):
// `Capability` is a closed, port-owned enum seeded from the GitHub dossier §4's
// eleven flags; adapters return a CapabilityReport of
// supported | absent | unknown per capability with an adapter-supplied reason;
// the arming gap is computed AT THE PORT from that report, never by an adapter;
// and `unknown` is treated exactly as `absent` for arming — unprobed is not
// proof (ADR-0021 §3, epic judgment call (d)).
//
// The design inputs are S00's Q2 table (docs/planning/github-addressing-model.md):
// every flag has an operationally decidable predicate per forge of exactly two
// kinds — `P` (a named read-only probe) or `C` (a constant licensed by a named
// passing conformance case). Where no predicate exists, the flag is `unknown`
// and the gate cannot be armed on it — a product limitation, never papered over.

// Capability is one forge capability the port reasons about.
type Capability string

const (
	// CapabilityResolvableThreads — findings threads can be resolved by the bot.
	// GitLab: constant supported, licensed by the p3e5-* reconciliation cases.
	// GitHub: constant supported, licensed by threads-resolvable-graphql.
	CapabilityResolvableThreads Capability = "resolvable-threads"
	// CapabilityThreadsBlockMerge — unresolved discussions block the merge.
	CapabilityThreadsBlockMerge Capability = "threads-block-merge"
	// CapabilityBlockingReview — a REQUEST_CHANGES review blocks the merge.
	CapabilityBlockingReview Capability = "blocking-review"
	// CapabilityReviewDismissalRestrictions — review dismissal is restricted
	// such that the MR author cannot dismiss a blocking review.
	CapabilityReviewDismissalRestrictions Capability = "review-dismissal-restrictions"
	// CapabilitySHAGuardedMerge — the merge API honours a SHA pin (CAS).
	CapabilitySHAGuardedMerge Capability = "sha-guarded-merge"
	// CapabilityDeferredMergeArming — deferred merge arming exists (GitLab MWPS,
	// GitHub enablePullRequestAutoMerge).
	CapabilityDeferredMergeArming Capability = "deferred-merge-arming"
	// CapabilityArmingRevokedOnPush — a new push revokes deferred arming.
	CapabilityArmingRevokedOnPush Capability = "arming-revoked-on-push"
	// CapabilityMergeResultPinning — a real merge-result digest is readable.
	CapabilityMergeResultPinning Capability = "merge-result-pinning"
	// CapabilityEligibleApprovalEvidence — the forge can prove WHO may approve
	// (typed eligible principals), the property require-review needs. Sub-grading
	// full|aggregate is carried in the reason, never in the state.
	CapabilityEligibleApprovalEvidence Capability = "eligible-approval-evidence"
	// CapabilityApprovalResetOnPush — approvals reset when a push lands.
	CapabilityApprovalResetOnPush Capability = "approval-reset-on-push"
	// CapabilityProtectedPipelineSource — ADR-0015 §4's arming prerequisite:
	// the CI config that drives assent comes from a source the MR author cannot
	// edit. No heuristic may stand in for a probe (the SEC-04 shape).
	CapabilityProtectedPipelineSource Capability = "protected-pipeline-source"
)

// AllCapabilities is the closed set, in a deterministic (declared) order.
func AllCapabilities() []Capability {
	return []Capability{
		CapabilityResolvableThreads,
		CapabilityThreadsBlockMerge,
		CapabilityBlockingReview,
		CapabilityReviewDismissalRestrictions,
		CapabilitySHAGuardedMerge,
		CapabilityDeferredMergeArming,
		CapabilityArmingRevokedOnPush,
		CapabilityMergeResultPinning,
		CapabilityEligibleApprovalEvidence,
		CapabilityApprovalResetOnPush,
		CapabilityProtectedPipelineSource,
	}
}

// ParseCapability decodes a capability name. An unknown name is an ERROR, not a
// skip — the enum is closed (REQ-E10-S04-01).
func ParseCapability(name string) (Capability, error) {
	for _, c := range AllCapabilities() {
		if string(c) == name {
			return c, nil
		}
	}
	return "", fmt.Errorf("forge: unknown capability %q (the enum is closed; %d known capabilities)", name, len(AllCapabilities()))
}

// CapabilityState is the tri-state S00's model requires: probed `supported` /
// probed `absent` / `unknown` (unprobed or unverified). `unknown` is treated
// exactly as `absent` for arming, with a DISTINGUISHABLE reason string.
type CapabilityState string

const (
	// CapabilitySupported — the probe (or a named conformance case) proved it.
	CapabilitySupported CapabilityState = "supported"
	// CapabilityAbsent — the forge verifiably lacks it.
	CapabilityAbsent CapabilityState = "absent"
	// CapabilityUnknown — not probed, or the probe's real-world behaviour is
	// unverified against a live API (S00's `unverified` rule). Treats as absent
	// for arming; the reason names what is open.
	CapabilityUnknown CapabilityState = "unknown"
)

// CapabilityEntry is one capability's state plus the reason the state is what
// it is. The reason is what doctor and the arming-refusal comment render, so it
// must be contributor-legible.
type CapabilityEntry struct {
	State  CapabilityState
	Reason string
}

// CapabilityReport is the adapter's typed answer for the closed enum. Every
// capability carries an entry; an adapter that does not probe one reports
// `unknown` with the reason naming the open item (S00 Q2, ADR-0021 §3).
type CapabilityReport struct {
	entries map[Capability]CapabilityEntry
}

// NewCapabilityReport builds a report from entries, rejecting duplicate or
// unknown capability names (strict decode, P3-E2).
func NewCapabilityReport(entries map[Capability]CapabilityEntry) (CapabilityReport, error) {
	report := CapabilityReport{entries: make(map[Capability]CapabilityEntry, len(AllCapabilities()))}
	for name, entry := range entries {
		switch name {
		case CapabilityResolvableThreads,
			CapabilityThreadsBlockMerge,
			CapabilityBlockingReview,
			CapabilityReviewDismissalRestrictions,
			CapabilitySHAGuardedMerge,
			CapabilityDeferredMergeArming,
			CapabilityArmingRevokedOnPush,
			CapabilityMergeResultPinning,
			CapabilityEligibleApprovalEvidence,
			CapabilityApprovalResetOnPush,
			CapabilityProtectedPipelineSource:
			// A member of the closed enum.
		default:
			return CapabilityReport{}, fmt.Errorf("forge: unknown capability %q (the enum is closed)", name)
		}
		report.entries[name] = CapabilityEntry{State: normalizeState(entry.State), Reason: entry.Reason}
	}
	if err := report.reportMissing(); err != nil {
		return CapabilityReport{}, err
	}
	return report, nil
}

func normalizeState(s CapabilityState) CapabilityState {
	switch s {
	case CapabilitySupported, CapabilityAbsent, CapabilityUnknown:
		return s
	default:
		return CapabilityUnknown
	}
}

// reportMissing fails a report that does not answer every member of the enum:
// a capability without an entry is exactly the "unprobed is proof" defect the
// model exists to make impossible.
func (r CapabilityReport) reportMissing() error {
	var missing []string
	for _, c := range AllCapabilities() {
		if _, ok := r.entries[c]; !ok {
			missing = append(missing, string(c))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("forge: capability report missing entries for %s", strings.Join(missing, ", "))
	}
	return nil
}

// AllUnknownCapabilityReport is the report an adapter that answers NOTHING
// would give: every capability unknown with the honest reason. The fake's zero
// fixture uses it — the all-unknown report refuses to arm (ADR-0021 §3), the
// fail-safe default.
func AllUnknownCapabilityReport() CapabilityReport {
	entries := make(map[Capability]CapabilityEntry, len(AllCapabilities()))
	for _, c := range AllCapabilities() {
		entries[c] = CapabilityEntry{
			State:  CapabilityUnknown,
			Reason: "unprobed by this fixture — unknown never arms (ADR-0021 §3)",
		}
	}
	return CapabilityReport{entries: entries}
}

// UnknownCapabilityEntry is the honest entry for a capability an adapter does
// not probe: state unknown, with a reason naming that fact.
func UnknownCapabilityEntry(reason string) CapabilityEntry {
	return CapabilityEntry{State: CapabilityUnknown, Reason: reason}
}

// SupportedCapabilityEntry and AbsentCapabilityEntry are the other two shapes,
// so adapters spell the tri-state uniformly.
func SupportedCapabilityEntry(reason string) CapabilityEntry {
	return CapabilityEntry{State: CapabilitySupported, Reason: reason}
}

// AbsentCapabilityEntry builds the absent-state entry (the forge verifiably
// lacks the capability; the reason names the evidence).
func AbsentCapabilityEntry(reason string) CapabilityEntry {
	return CapabilityEntry{State: CapabilityAbsent, Reason: reason}
}

// State returns the entry's state (unknown when the capability is absent from
// the report — an adapter cannot dodge an answer by omission).
func (r CapabilityReport) State(c Capability) CapabilityState {
	e, ok := r.entries[c]
	if !ok {
		return CapabilityUnknown
	}
	return e.State
}

// IsZero reports whether the report carries no entries — a fixture default, to
// be replaced by forge.AllUnknownCapabilityReport() before consultation.
func (r CapabilityReport) IsZero() bool { return r.entries == nil }

// Reason returns the entry's reason, or "" when the report carries none.
func (r CapabilityReport) Reason(c Capability) string {
	e, ok := r.entries[c]
	if !ok {
		return ""
	}
	return e.Reason
}

// ArmedBlocks reports whether this capability's state REFUSES arming at a
// consultation point: only a probe-proven `supported` allows arming;
// `absent` and `unknown` are identical refusals, with distinguishable reasons
// (REQ-E10-S04-02).
func (r CapabilityReport) ArmedBlocks(c Capability) bool {
	return r.State(c) != CapabilitySupported
}

// Sorted returns the entries in capability order — deterministic output for
// doctor and tests.
func (r CapabilityReport) Sorted() []CapabilityEntryView {
	views := make([]CapabilityEntryView, 0, len(r.entries))
	for c, e := range r.entries {
		views = append(views, CapabilityEntryView{Capability: string(c), State: string(e.State), Reason: e.Reason})
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].Capability < views[j].Capability
	})
	return views
}

// CapabilityEntryView is the doctor-facing serialisable form of one entry.
type CapabilityEntryView struct {
	Capability string
	State      string
	Reason     string
}

// ReportMissingErr sentinel message fragment is exported for tests.
