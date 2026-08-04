package forge

// Snapshotter reads forge MR state without mutating it (E4-S01).
type Snapshotter interface {
	Snapshot(project, mr string) (Snapshot, error)
}

// Snapshot is the typed read-side view of an MR at observation time.
// No map[string]any at this boundary — every field is explicit.
type Snapshot struct {
	Heads        MRHeads
	ChangedFiles []string
	Capabilities CapabilityFlags
	BotThreads   []Thread
}

// MRHeads carries the SHAs and branch names the evaluation pins against.
// TargetSHA is the target branch tip, not the merge-base (GitLab dossier §2).
type MRHeads struct {
	SourceSHA         string
	TargetSHA         string
	SourceBranch      string
	TargetBranch      string
	MergeResultDigest string
	Author            string
	// ForkMR is true when the MR source project differs from the target project
	// (GitLab fork workflow). ADR-0015 §8: fork/untrusted context is advisory-only.
	ForkMR bool
}

// GitLabTier is the licensed tier detected from forge probe data (dossier §1).
type GitLabTier string

const (
	// TierFree is the GitLab Free licensed tier (dossier §1).
	TierFree GitLabTier = "free"
	// TierPremium is the GitLab Premium licensed tier.
	TierPremium GitLabTier = "premium"
	// TierUltimate is the GitLab Ultimate licensed tier.
	TierUltimate GitLabTier = "ultimate"
)

// CapabilityFlags exposes tier and merge-gate capabilities for doctor and Resolve
// fail-closed decisions (forge dossier §1 C3/C6/C7/C13/C14/C17).
type CapabilityFlags struct {
	Tier GitLabTier

	HasApprovalRulesAPI         bool
	DiscussionsResolvedGate     bool
	MergeResultDigestRecordable bool
	MergeTrainAvailable         bool
	ProtectedPipelineExternal   bool
}
