package forge

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// Resolver maps a require-review subject and pinned SHAs to forge-proven evidence
// or an explicit capability gap — never silent APPROVE on missing proof (E4-S01).
type Resolver interface {
	Resolve(req ResolveRequest) (ResolveResult, error)
}

// ResolveRequest identifies the MR, governed subject, and evaluation pins Resolve
// must honour when fetching approval evidence.
type ResolveRequest struct {
	Project           string
	MR                string
	Subject           string
	SourceSha         string
	TargetSha         string
	MergeResultDigest string
	MRAuthor          string
}

// ResolveResult is a sum type: exactly one of Evidence or Gap is populated.
type ResolveResult struct {
	Evidence *aggregate.ApprovalEvidence
	Gap      *CapabilityGap
}

// HasEvidence reports whether forge-proven ApprovalEvidence was returned.
func (r ResolveResult) HasEvidence() bool { return r.Evidence != nil }

// HasGap reports whether Resolve returned an explicit capability gap.
func (r ResolveResult) HasGap() bool { return r.Gap != nil }

// WellFormed reports whether exactly one of Evidence or Gap is populated (INBOX P2).
func (r ResolveResult) WellFormed() error {
	switch {
	case r.Evidence != nil && r.Gap != nil:
		return fmt.Errorf("forge resolve: both evidence and gap set")
	case r.Evidence == nil && r.Gap == nil:
		return fmt.Errorf("forge resolve: neither evidence nor gap set")
	default:
		return nil
	}
}

// CapabilityGapReason is a typed forge capability absence (never map[string]any).
type CapabilityGapReason string

const (
	// GapApprovalRulesUnavailable — Premium approval-rules API absent (dossier C6/C7).
	GapApprovalRulesUnavailable CapabilityGapReason = "approval-rules-api-unavailable"
	// GapFreeTierRequireReview — Free tier cannot prove eligible approval (judgment call c).
	GapFreeTierRequireReview CapabilityGapReason = "free-tier-require-review-unsatisfiable"

	// GapEligibilityUnprovable — the forge exposes no computed eligible-approver
	// set the adapter could prove set-equality with, so require-review evidence
	// can never be minted (OQ-34: an adapter-computed CODEOWNERS/PR-review set
	// proves who DID review, never who WAS eligible — GitHub exposes no API
	// returning the per-PR eligible code owners, dossier §2 step (b), graded
	// partial). The adapter may still fetch and evaluate the review chain
	// (transport fail-closed), but it must return this gap instead of evidence
	// until OQ-34 is answered.
	GapEligibilityUnprovable CapabilityGapReason = "eligible-approver-set-unprovable"
)

// CapabilityGap records why require-review evidence cannot be proven on this tier.
type CapabilityGap struct {
	Reason  CapabilityGapReason
	Subject string
}

// ResolveWithEvidence builds a result carrying schema-ready aggregate evidence.
func ResolveWithEvidence(ev aggregate.ApprovalEvidence) ResolveResult {
	return ResolveResult{Evidence: &ev}
}

// ResolveWithGap builds a result carrying an explicit capability gap.
func ResolveWithGap(gap CapabilityGap) ResolveResult {
	return ResolveResult{Gap: &gap}
}
