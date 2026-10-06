package forge

// Snapshotter reads forge MR state without mutating it (E4-S01).
type Snapshotter interface {
	Snapshot(project, mr string) (Snapshot, error)
}

// EnumerationIncompletePrefix is the normative OpaqueReason prefix a
// checkout-less run stamps on the change set when ChangedFilesComplete is false
// (ADR-0020 §4, D-119). It is exported so the adapter contract, the run-path
// wiring and their tests cannot drift apart; the specific gap reason
// (ChangedFilesGap) is appended to it verbatim.
const EnumerationIncompletePrefix = "forge changed-file enumeration incomplete: "

// Snapshot is the typed read-side view of an MR at observation time.
// No map[string]any at this boundary — every field is explicit.
type Snapshot struct {
	Heads        MRHeads
	ChangedFiles []string
	// Capabilities is the NEUTRAL capability report (E10-S04, ADR-0021 item 3):
	// supported | absent | unknown per capability of the closed eleven-flag
	// enum, with the reason the state is what it is. `unknown` never arms
	// (ADR-0021 §3, unprobed is not proof).
	Capabilities CapabilityReport
	BotThreads   []Thread

	// ChangedFilesComplete reports whether ChangedFiles is the PROVABLY COMPLETE
	// set of paths the MR touches (ADR-0020 §1, D-119). In checkout-less runs
	// ChangedFiles is the SOLE `.assent/**` detector, so a silently truncated
	// list would starve the D-042 self-vouch guard and let a padded MR
	// approve+merge its own policy edit.
	//
	// The ZERO VALUE (false) FAILS SAFE BY DESIGN: an adapter or fake that
	// forgets to set this degrades the run to fail-safe REVIEW, never to a
	// fail-open APPROVE. Every adapter and fake MUST therefore set it
	// EXPLICITLY on the success path — the required conformance cases and the
	// byte-identical happy-path regression tests fail loudly against one that
	// never reports completeness.
	ChangedFilesComplete bool

	// ChangedFilesGap is the specific, human-readable reason completeness could
	// not be proven. It is non-empty IFF ChangedFilesComplete is false
	// (ADR-0020 §1; mirrors the ADR-0017 §1 mergeResultDigest/capabilityGap
	// honesty pattern — an honest declared gap, never a silent short list).
	ChangedFilesGap string
}

// EnumerationOpaqueReason returns the OpaqueReason a checkout-less run must
// stamp on the change set for this snapshot, or "" when the enumeration is
// provably complete. Deriving it here (rather than concatenating at each call
// site) keeps the ADR-0020 §4 prefix single-sourced.
func (s Snapshot) EnumerationOpaqueReason() string {
	if s.ChangedFilesComplete {
		return ""
	}
	return EnumerationIncompletePrefix + s.ChangedFilesGap
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
	// Labels are the MR's forge labels (GitLab's string array), surfaced so the
	// typed read view carries the same MR metadata the engine's `mr` scope needs.
	Labels []string
	// ForkMR is true when the MR source project differs from the target project
	// (GitLab fork workflow). ADR-0015 §8: fork/untrusted context is advisory-only.
	ForkMR bool
}
