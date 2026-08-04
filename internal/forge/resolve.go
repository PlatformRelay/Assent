package forge

import "github.com/PlatformRelay/assent/internal/core/aggregate"

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

// CapabilityGapReason is a typed forge capability absence (never map[string]any).
type CapabilityGapReason string

const (
	// GapApprovalRulesUnavailable — Premium approval-rules API absent (dossier C6/C7).
	GapApprovalRulesUnavailable CapabilityGapReason = "approval-rules-api-unavailable"
	// GapFreeTierRequireReview — Free tier cannot prove eligible approval (judgment call c).
	GapFreeTierRequireReview CapabilityGapReason = "free-tier-require-review-unsatisfiable"
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
