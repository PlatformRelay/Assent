package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/classify"
	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/gitlab"
	"github.com/PlatformRelay/assent/schemas"
)

// runConfig is the parsed `assent run` flag set. The GitLab PAT is deliberately
// NOT a field here — it is read from the GITLAB_TOKEN env var at the boundary and
// handed straight to the adapter, never stored where it could be logged.
type runConfig struct {
	endpoint  string
	project   string
	mr        string
	policy    string
	botAuthor string
	arm       bool
	emit      string

	// checkout is the OPT-IN local-checkout dir (base/ + head/ subtrees) the
	// E1-S08 changed-file-set enumeration reads (ADR-0008 §4: local checkout
	// only, never a forge API). When empty, orchestrate diffs only the governed
	// subject — the exact pre-E1-S08 single-file path — so existing runs and
	// tests are unaffected.
	checkout string
}

// forgePort is the subset of behaviour `assent run` needs from a forge: the
// full write port plus the two orchestration reads (GetMR, FileAtRef). The
// concrete *gitlab.Client satisfies it; tests drive it against an httptest
// GitLab through the same concrete client (no live network).
type forgePort interface {
	forge.Forge
	GetMR(project, mr string) (gitlab.MRInfo, error)
	FileAtRef(project, path, ref string) ([]byte, error)
}

// runClock is the injected time seam: cmd/assent binds it to time.Now and threads
// the value down as data (never time.Now inside the engine or the receipt).
type runClock func() time.Time

// clockAdapter adapts a runClock to the forge.Clock interface.
type clockAdapter struct{ now runClock }

func (c clockAdapter) Now() time.Time { return c.now() }

// runRun is the testable entry point for `assent run`. It parses args, reads the
// token from the environment, and drives the orchestration against the supplied
// forge. Returning a process exit code:
//
//	0  the run completed and produced a valid receipt (regardless of decision);
//	   an advisory REVIEW/BLOCK, or an APPROVE without --arm, is still a clean 0.
//	non-zero  a HARD error: a missing flag, an unparseable policy, a schema
//	   invalid record, or a forge/IO failure. No forge write happens on a hard
//	   error before the decision.
//
// forgeFactory builds the forge from the resolved endpoint+token+botAuthor; the
// production path passes gitlab.New, tests pass a fake pointed at httptest.
func runRun(args []string, getenv func(string) string, clock runClock, stdout, stderr io.Writer,
	forgeFactory func(endpoint, token, botAuthor string) forgePort) int {
	cfg, err := parseRunFlags(args, stderr)
	if err != nil {
		// flag parsing already printed usage; -h/--help returns a clean 2.
		if errors.Is(err, flag.ErrHelp) {
			return 2
		}
		_, _ = fmt.Fprintln(stderr, "assent run:", err)
		return 2
	}

	// The ONLY secret. Read at the boundary; handed straight to the adapter.
	token := getenv("GITLAB_TOKEN")
	if token == "" {
		_, _ = fmt.Fprintln(stderr, "assent run: GITLAB_TOKEN is required (the PAT is never a flag)")
		return 2
	}

	client := forgeFactory(cfg.endpoint, token, cfg.botAuthor)

	if err := orchestrate(cfg, client, clock, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, "assent run:", err)
		return 1
	}
	return 0
}

// parseRunFlags parses the `run` subcommand flags. A missing required flag is an
// error (exit non-zero); the token is never a flag.
func parseRunFlags(args []string, stderr io.Writer) (runConfig, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg runConfig
	fs.StringVar(&cfg.endpoint, "gitlab-endpoint", "https://gitlab.com", "GitLab instance base URL")
	fs.StringVar(&cfg.project, "project", "", "GitLab numeric project id (required)")
	fs.StringVar(&cfg.mr, "mr", "", "merge-request IID (required)")
	fs.StringVar(&cfg.policy, "policy", ".assent/policy.yaml", "policy path (loaded from the TARGET ref)")
	fs.StringVar(&cfg.botAuthor, "bot-author", "", "bot username for the author-identity filter (required)")
	fs.BoolVar(&cfg.arm, "arm", false, "SANDBOX arming override (see D-034 note) — approve+merge only when set AND decision APPROVE")
	fs.StringVar(&cfg.emit, "emit", "", "path to write the DecisionRecord JSON (default: stdout)")
	fs.StringVar(&cfg.checkout, "checkout", "", "local checkout dir (base/ + head/ subtrees) to enumerate the MR's full changed-file set (E1-S08); when unset, only the governed subject is diffed")
	if err := fs.Parse(args); err != nil {
		return runConfig{}, err
	}
	for _, req := range []struct{ name, val string }{
		{"--project", cfg.project},
		{"--mr", cfg.mr},
		{"--bot-author", cfg.botAuthor},
	} {
		if req.val == "" {
			return runConfig{}, fmt.Errorf("%s is required", req.name)
		}
	}
	return cfg, nil
}

// orchestrate ties the engine to the forge. It is the whole walking-skeleton
// path: read MR → load policy from the TARGET ref → diff the governed file →
// classify → aggregate → build+validate the DecisionRecord → reconcile → emit.
// Every fail-safe axis (opaque diff → REVIEW, unparseable policy → error/no
// write, ArmEligible false → no write) is enforced here.
func orchestrate(cfg runConfig, client forgePort, clock runClock, stdout io.Writer) error {
	// 1. MR metadata: source/target branches + pinned SHAs.
	info, err := client.GetMR(cfg.project, cfg.mr)
	if err != nil {
		return fmt.Errorf("read MR: %w", err)
	}

	// 2. Load policy from the TARGET ref (ADR-0015 §1) — NEVER the source branch.
	policyBytes, err := client.FileAtRef(cfg.project, cfg.policy, info.TargetBranch)
	if err != nil {
		return fmt.Errorf("load policy from target ref %q: %w", info.TargetBranch, err)
	}
	binding, err := ParsePolicy(policyBytes)
	if err != nil {
		// Fail closed on an unparseable policy: no writes.
		return fmt.Errorf("policy: %w", err)
	}

	// 3. The governed file path is the binding subject with the "file:" prefix
	//    stripped. Fetch base (target ref) and head (source ref) content.
	governed := strings.TrimPrefix(binding.Subject, "file:")
	if governed == binding.Subject {
		return fmt.Errorf("policy subject %q must be a file:<path> entryRef", binding.Subject)
	}
	// KNOWN ADR-0008 §4 GAP (pre-existing, tracked separately — NOT E1-S08's job):
	// the governed file's content is fetched via the forge API (client.FileAtRef)
	// rather than a local checkout. ADR-0008 §4 requires evaluation against a local
	// checkout with no API-only file fetching. E1-S08 adds the local-checkout
	// changed-file-set enumeration below WITHOUT inheriting or multiplying this
	// pre-existing single-file API read; fixing this read is out of scope here.
	base, err := client.FileAtRef(cfg.project, governed, info.TargetBranch)
	if err != nil {
		return fmt.Errorf("load governed base %q: %w", governed, err)
	}
	head, err := client.FileAtRef(cfg.project, governed, info.SourceBranch)
	if err != nil {
		return fmt.Errorf("load governed head %q: %w", governed, err)
	}

	// 4. Diff → ChangeSet. change.Diff returns an opaque ChangeSet ACCOMPANIED BY a wrapped
	//    ErrOpaque, so an undecidable governed-subject diff takes this error branch and fails
	//    CLOSED as a hard error (exit non-zero) — it never reaches the aggregator, so there is no
	//    approve/merge. (The opaque → REVIEW-thread mapping applies on the E1-S08 checkout-fold
	//    path, which sets changeSet.Opaque without erroring; see step 5b.)
	changeSet, err := change.Diff(governed, base, head)
	if err != nil {
		return fmt.Errorf("diff governed file %q: %w", governed, err)
	}

	// 5. Classify (path-only) and aggregate.
	subjectClass := classify.Classify(changeSet)

	// 5b. E1-S08 (OPT-IN): when --checkout is set, enumerate the MR's FULL
	//     changed-file set from the LOCAL CHECKOUT (ADR-0008 §4) and fold it into
	//     the two safety signals the aggregator honours BEFORE any predicate:
	//       - any `.assent/**` path dominates the class to assent-policy -> BLOCK
	//         (a smuggled policy edit cannot vouch itself, matching the engine
	//         golden on the adapter path);
	//       - any opaque changed file forces the run fail-safe (never dropped).
	//     The governed subject's own ChangeSet stays the predicate input (see
	//     foldCheckout doc: a multi-file union would leave old/new unbound and
	//     could leak unrelated scalars into the subject's predicate). When
	//     --checkout is unset this whole step is skipped — exactly the pre-E1-S08
	//     single-file path.
	if cfg.checkout != "" {
		fold, ferr := foldCheckout(dirCheckout{root: cfg.checkout})
		if ferr != nil {
			return fmt.Errorf("enumerate changed-file set: %w", ferr)
		}
		if fold.class == classify.ClassAssentPolicy {
			subjectClass = classify.ClassAssentPolicy
		}
		if fold.opaque && !changeSet.Opaque {
			changeSet.Opaque = true
			changeSet.OpaqueReason = "changed-file set opaque: " + fold.opaqueReason
		}
	}

	result, err := aggregate.Aggregate(binding, changeSet, subjectClass)
	if err != nil {
		return fmt.Errorf("aggregate: %w", err)
	}

	// 6. Build the DecisionRecord. GitLab plain-merge exposes no merge-result
	//    digest, so the record honestly carries mergeResultDigest:null +
	//    capabilityGap (ADR-0017 §1). Tool/policy digests are non-empty.
	mergeGap, err := decision.MergeResultGap("gitlab plain-merge exposes no merge-result digest (no merge train); ADR-0017 §1 capabilityGap")
	if err != nil {
		return fmt.Errorf("merge-result gap: %w", err)
	}
	pins := decision.Pins{
		ToolVersion:     version,
		ToolDigest:      "sha256:" + sha256Hex([]byte(version)),
		PolicySha:       "sha256:" + sha256Hex(policyBytes),
		SourceSha:       info.SourceSHA,
		TargetSha:       info.TargetSHA,
		MergeResult:     mergeGap,
		FactsResolvedAt: map[string]string{},
	}
	report, err := decision.Build(result, pins)
	if err != nil {
		return fmt.Errorf("build decision record: %w", err)
	}

	recordJSON, err := report.MarshalRecord()
	if err != nil {
		return fmt.Errorf("marshal decision record: %w", err)
	}
	// 7. Validate against the frozen schema BEFORE any write.
	if err := validateRecord(recordJSON); err != nil {
		return fmt.Errorf("decision record failed schema validation: %w", err)
	}

	// 8. Map the decision → forge intent, and reconcile. The marker occurrence is
	//    derived from the JUDGED HEAD CONTENT (stable across tool/policy bumps —
	//    the grammar's "occurrence = judged-content digest"), the decision digest
	//    from the DecisionRecord bytes, and entryRef from the governed subject.
	desired, pre := buildDesired(cfg, info, binding.Subject, head, result, recordJSON)
	receipt, recErr := forge.Reconcile(client, clockAdapter{now: clock}, desired, pre)

	// 9. Emit the DecisionRecord + a one-line summary. A refusal from Reconcile
	//    that is an EXPECTED fail-closed outcome (arming unmet, SHA moved) is NOT
	//    a hard error — it is the advisory/no-write result the run is designed to
	//    produce; report it in the summary and still exit 0.
	if err := emitRecord(cfg, recordJSON, stdout); err != nil {
		return fmt.Errorf("emit decision record: %w", err)
	}
	summary := summarize(result.Decision, cfg.arm, receipt, recErr)
	_, _ = fmt.Fprintln(stdout, summary)

	// A hard forge error (not a fail-closed refusal) IS a failure.
	if recErr != nil && !isFailClosed(recErr) {
		return fmt.Errorf("reconcile: %w", recErr)
	}
	return nil
}

// buildDesired maps the aggregate decision to a forge.DesiredReviewState +
// Preconditions.
//
//   - REVIEW/BLOCK → exactly one resolvable thread carrying the marker plus a
//     human body naming the driving rule/code. No approve/merge.
//   - APPROVE → approve + a SHA-pinned merge, gated by arming.
//
// Arming: ArmEligible is TRUE only when --arm was passed AND the decision is
// APPROVE. Without --arm the APPROVE path degrades to advisory (Reconcile
// returns ErrArmingRefused → no write).
//
// The CAS pins (SourceSha/TargetSha/MergeResultDigest) are the values the
// adapter's CurrentHeads reports. The merge-result digest is the adapter's
// SYNTHETIC digest (GitLab exposes no real one) — non-empty and consistent with
// what forge.Reconcile re-reads via CurrentHeads, so the merge is reachable
// while the DecisionRecord separately carries the honest null+capabilityGap.
func buildDesired(cfg runConfig, info gitlab.MRInfo, subject string, head []byte, result aggregate.Result, recordJSON []byte) (forge.DesiredReviewState, forge.Preconditions) {
	digest := gitlab.SyntheticDigest(info.SourceSHA, info.TargetSHA)
	desired := forge.DesiredReviewState{Project: cfg.project, MR: cfg.mr}

	if result.Decision == aggregate.DecisionApprove {
		desired.Approve = true
		desired.Merge = &forge.DesiredMerge{
			SourceSha:         info.SourceSHA,
			TargetSha:         info.TargetSHA,
			MergeResultDigest: digest,
		}
		pre := forge.Preconditions{
			// D-034 SEAM: --arm is a SANDBOX override for the walking skeleton. It
			// does NOT satisfy real protected-source verification (S05
			// INSECURE-PLACEHOLDER still deferred): never wire readPipelineDescription
			// into a real merge. ArmEligible is TRUE only when --arm AND APPROVE.
			ArmEligible:       cfg.arm,
			SourceSha:         info.SourceSHA,
			TargetSha:         info.TargetSHA,
			MergeResultDigest: digest,
		}
		return desired, pre
	}

	// REVIEW/BLOCK: one resolvable thread. Per the ADR-0019 marker grammar,
	// occurrence = digest of the JUDGED HEAD CONTENT (stable across tool/policy
	// bumps, so a rerun after a tool-version change recognises the existing thread
	// rather than posting a duplicate), decision = digest of the DecisionRecord,
	// entryRef = the governed-subject identity (the binding subject, a valid
	// file:<path> idString), effect = the driving finding's effect.
	effect, rule, code := drivingFinding(result)
	marker := forge.Marker{
		Slot: forge.Slot{
			Project:  cfg.project,
			MR:       cfg.mr,
			Rule:     rule,
			EntryRef: subject,
			Effect:   effect,
		},
		Occurrence: "sha256:" + sha256Hex(head),
		Decision:   "sha256:" + sha256Hex(recordJSON),
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
	body := fmt.Sprintf("assent review required: rule %q (%s) — decision %s. Resolve after addressing the finding.",
		rule, code, result.Decision)
	desired.Thread = &forge.DesiredThread{Marker: marker, Body: body}
	return desired, forge.Preconditions{}
}

// drivingFinding picks the finding that drives the decision, for the thread's
// human body + marker effect. Findings are already canonically sorted; the
// first one is deterministic. Falls back to a review-effect placeholder if there
// are somehow no findings (a REVIEW/BLOCK always has at least one).
func drivingFinding(result aggregate.Result) (effect, rule, code string) {
	if len(result.Findings) == 0 {
		return "require-review", "aggregate.changeset", "changeset.undecidable"
	}
	f := result.Findings[0]
	return string(f.Effect), f.Rule, f.Code
}

// summarize builds the one-line run summary from the decision, arming, and the
// reconcile outcome.
func summarize(dec aggregate.Decision, arm bool, receipt forge.PublicationReceipt, recErr error) string {
	switch {
	case recErr != nil && errors.Is(recErr, forge.ErrArmingRefused):
		return fmt.Sprintf("decision=%s arm=%t → advisory-only (arming precondition unmet, no approve/merge)", dec, arm)
	case recErr != nil && errors.Is(recErr, forge.ErrSHAMoved):
		return fmt.Sprintf("decision=%s → SHA moved since evaluation, no merge (fail-closed)", dec)
	case recErr != nil && errors.Is(recErr, forge.ErrIncompletePreconditions):
		return fmt.Sprintf("decision=%s → incomplete merge preconditions, no merge (fail-closed)", dec)
	case recErr != nil:
		return fmt.Sprintf("decision=%s → reconcile error: %v", dec, recErr)
	default:
		return fmt.Sprintf("decision=%s arm=%t → %d forge operation(s) written", dec, arm, len(receipt.Operations))
	}
}

// isFailClosed reports whether a reconcile error is an EXPECTED fail-closed
// refusal (advisory/no-write), which is a clean exit-0 outcome — not a hard
// error. Arming-unmet, SHA-moved, and incomplete-preconditions are the designed
// refusals; anything else (a transport failure, an unsupported decision) is hard.
func isFailClosed(err error) bool {
	return errors.Is(err, forge.ErrArmingRefused) ||
		errors.Is(err, forge.ErrSHAMoved) ||
		errors.Is(err, forge.ErrIncompletePreconditions)
}

// emitRecord writes the DecisionRecord JSON to --emit (a file) or, when --emit
// is empty, to the stdout writer ahead of the summary line.
func emitRecord(cfg runConfig, recordJSON []byte, stdout io.Writer) error {
	if cfg.emit == "" {
		_, err := stdout.Write(append(recordJSON, '\n'))
		return err
	}
	return os.WriteFile(cfg.emit, recordJSON, 0o600)
}

// validateRecord validates DecisionRecord JSON against the frozen schema.
func validateRecord(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("record is not JSON: %w", err)
	}
	return schemas.DecisionRecordSchema.Validate(doc)
}

// sha256Hex returns the lowercase hex sha256 of b (a real 64-hex digest so the
// marker's ^sha256:[0-9a-f]{64}$ grammar and the non-empty pins are satisfied).
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
