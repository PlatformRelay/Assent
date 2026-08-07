package fake

import (
	"sort"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/forge"
)

// ResolveMode selects which Resolve fixture the fake returns.
type ResolveMode string

const (
	// ResolveEligible returns forge-proven eligible approval evidence (Premium tier).
	ResolveEligible ResolveMode = "eligible"
	// ResolveGapFreeTier returns a typed capability gap for Free-tier unsatisfiable require-review.
	ResolveGapFreeTier ResolveMode = "gap-free-tier"
)

// Snapshot implements forge.Snapshotter with deterministic, sorted outputs.
func (f *Forge) Snapshot(_, _ string) (forge.Snapshot, error) {
	// ADR-0020 §3: a diff-endpoint 404/5xx is a HARD ERROR — no snapshot, so no
	// decision and no forge write can follow from an unenumerable change set.
	if f.ChangedFilesErr != nil {
		return forge.Snapshot{}, f.ChangedFilesErr
	}

	threads, err := f.ListBotThreads("", "")
	if err != nil {
		return forge.Snapshot{}, err
	}
	sort.Slice(threads, func(i, j int) bool { return threads[i].ID < threads[j].ID })

	files := append([]string(nil), f.ChangedFiles...)
	sort.Strings(files)

	return forge.Snapshot{
		Heads: forge.MRHeads{
			SourceSHA:         f.CurrentSourceSha,
			TargetSHA:         f.CurrentTargetSha,
			SourceBranch:      f.SourceBranch,
			TargetBranch:      f.TargetBranch,
			MergeResultDigest: f.CurrentMergeResultDigest,
			Author:            f.MRAuthor,
		},
		ChangedFiles: files,
		// Set EXPLICITLY (ADR-0020 §1): the zero value would fail safe to REVIEW,
		// but a fake that relies on that would silently degrade every test that
		// uses it. ChangedFilesGap is the truncation knob; empty = complete.
		ChangedFilesComplete: f.ChangedFilesGap == "",
		ChangedFilesGap:      f.ChangedFilesGap,
		Capabilities:         f.Capabilities,
		BotThreads:           threads,
	}, nil
}

// Resolve implements forge.Resolver. It never returns silent evidence — only explicit
// aggregate.ApprovalEvidence or a typed CapabilityGap.
func (f *Forge) Resolve(req forge.ResolveRequest) (forge.ResolveResult, error) {
	switch f.resolveMode() {
	case ResolveGapFreeTier:
		return forge.ResolveWithGap(forge.CapabilityGap{
			Reason:  forge.GapFreeTierRequireReview,
			Subject: req.Subject,
		}), nil
	default:
		return forge.ResolveWithEvidence(f.eligibleEvidence(req)), nil
	}
}

func (f *Forge) resolveMode() ResolveMode {
	if f.ResolveMode != "" {
		return f.ResolveMode
	}
	if !f.Capabilities.HasApprovalRulesAPI || f.Capabilities.Tier == forge.TierFree {
		return ResolveGapFreeTier
	}
	return ResolveEligible
}

func (f *Forge) eligibleEvidence(req forge.ResolveRequest) aggregate.ApprovalEvidence {
	return aggregate.ApprovalEvidence{
		VerifyingCapability: "approval-rules-api",
		ApprovalsRequired:   1,
		ApprovedBy: []aggregate.Approver{
			{ID: "202", Username: "bob", IsAuthor: false},
		},
		Eligibility: []string{"101", "202"},
		Pins:        aggregate.ApprovalPins{SourceSha: req.SourceSha},
		Expired:     false,
	}
}

var (
	_ forge.Snapshotter = (*Forge)(nil)
	_ forge.Resolver    = (*Forge)(nil)
)
