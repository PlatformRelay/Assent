package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/classify"
	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/gitlab"
	"github.com/PlatformRelay/assent/internal/render"
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
	binding   string
	subject   string
	config    string
	pack      string
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
// full write port plus orchestration reads (GetMR, FileAtRef, Snapshot, Resolve).
// The concrete *gitlab.Client satisfies it; tests drive it against an httptest
// GitLab through the same concrete client (no live network).
type forgePort interface {
	forge.Forge
	forge.Snapshotter
	forge.Resolver
	GetMR(project, mr string) (gitlab.MRInfo, error)
	FileAtRef(project, path, ref string) ([]byte, error)
}

// runClock is the injected time seam: cmd/assent binds it to time.Now and threads
// the value down as data (never time.Now inside the engine or the receipt).
type runClock func() time.Time

// clockAdapter adapts a runClock to the forge.Clocker interface.
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
	fs.StringVar(&cfg.policy, "policy", ".assent/merge-policy.yaml", "MergePolicy path (loaded from the TARGET ref)")
	fs.StringVar(&cfg.binding, "binding", ".assent/ruleset-binding.yaml", "RulesetBinding path (loaded from the TARGET ref)")
	fs.StringVar(&cfg.subject, "subject", "", "governed-subject entryRef (file:<path>) — the file diffed for evaluation (required)")
	fs.StringVar(&cfg.config, "config", "", "optional Config path (loaded from the TARGET ref) — when set, provider posture is validated (ADR-0017 §6)")
	fs.StringVar(&cfg.pack, "pack", "", "optional Pack path (loaded from the TARGET ref) — when set, its spec.phase caps every rule's phase (ADR-0018 §1)")
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
		{"--subject", cfg.subject},
	} {
		if req.val == "" {
			return runConfig{}, fmt.Errorf("%s is required", req.name)
		}
	}
	return cfg, nil
}

// orchestrate ties the frozen decision engine to the forge (E2-S04 REQ-06). The
// path: read MR → load the frozen MergePolicy + RulesetBinding from the TARGET
// ref → route the covering binding → diff the governed file → classify → build the
// typed EvaluationInput from the live diff → evaluate via the E2 coverage loop →
// build+validate the DecisionRecord → reconcile → emit. Every fail-safe axis
// (opaque/empty diff → REVIEW, unloadable policy → error/no write, reserved-class
// self-edit → BLOCK, ArmEligible false → no write) is enforced here.
//
// REQ-06 changes policy LOADING + input SHAPE + the EVALUATOR only; it does NOT
// touch file SOURCING (the governed base/head is still read via FileAtRef, the
// pre-existing ADR-0008 §4 single-file API read tracked separately) nor the
// forge write path (buildDesired/Reconcile/arming/SHA-guard/token — GUARD 2).
func orchestrate(cfg runConfig, client forgePort, clock runClock, stdout io.Writer) error {
	// 1. MR metadata: source/target branches + pinned SHAs.
	info, err := client.GetMR(cfg.project, cfg.mr)
	if err != nil {
		return fmt.Errorf("read MR: %w", err)
	}

	// 1b. E4-S06: forge Snapshot — changed-file set for classifier (when --checkout
	//     unset) and capability flags for forge-probed arming (D-034/D-074 on the
	//     run path). Resolve reads the same MR; no env placeholder on writes.
	snapshot, err := client.Snapshot(cfg.project, cfg.mr)
	if err != nil {
		return fmt.Errorf("forge snapshot: %w", err)
	}
	mrAuthor := snapshot.Heads.Author
	mergeDigest := gitlab.SyntheticDigest(info.SourceSHA, info.TargetSHA)
	probe := forge.PreconditionFromCapabilities(snapshot.Capabilities)

	// 2. Load the frozen MergePolicy + RulesetBinding from the TARGET ref
	//    (ADR-0015 §1) — NEVER the source branch — under strict decode (E2-S01).
	//    Fail CLOSED on any load/validate error: no forge writes.
	mpBytes, err := client.FileAtRef(cfg.project, cfg.policy, info.TargetBranch)
	if err != nil {
		return fmt.Errorf("load merge-policy from target ref %q: %w", info.TargetBranch, err)
	}
	mp, err := policy.LoadMergePolicy(mpBytes)
	if err != nil {
		return fmt.Errorf("merge-policy: %w", err)
	}
	bindBytes, err := client.FileAtRef(cfg.project, cfg.binding, info.TargetBranch)
	if err != nil {
		return fmt.Errorf("load ruleset-binding from target ref %q: %w", info.TargetBranch, err)
	}
	rb, err := policy.LoadRulesetBinding(bindBytes)
	if err != nil {
		return fmt.Errorf("ruleset-binding: %w", err)
	}
	bind, err := selectBinding(rb)
	if err != nil {
		return err
	}

	// 2b. Provider posture (ADR-0017 §6): a controlling/authorization fact whose
	//     provider is configured `failure: open` is REJECTED — a controlling fact
	//     must fail closed. Wired only when a --config is supplied. The loaded
	//     Config is retained for E5-S05 fact resolution below.
	var conf *policy.Config
	if cfg.config != "" {
		confBytes, cerr := client.FileAtRef(cfg.project, cfg.config, info.TargetBranch)
		if cerr != nil {
			return fmt.Errorf("load config from target ref %q: %w", info.TargetBranch, cerr)
		}
		loaded, cerr := policy.LoadConfig(confBytes)
		if cerr != nil {
			return fmt.Errorf("config: %w", cerr)
		}
		conf = loaded
		if perr := policy.ValidateProviderPosture(conf, mp); perr != nil {
			return fmt.Errorf("provider posture: %w", perr)
		}
	}

	// 2c. Pack phase ceiling (ADR-0018 §1): when a --pack is supplied its spec.phase
	//     CAPS every rule's effective phase toward off (never additive). Absent ⇒
	//     enforce (no cap).
	ceiling := policy.PhaseEnforce
	if cfg.pack != "" {
		packBytes, perr := client.FileAtRef(cfg.project, cfg.pack, info.TargetBranch)
		if perr != nil {
			return fmt.Errorf("load pack from target ref %q: %w", info.TargetBranch, perr)
		}
		pk, perr := policy.LoadPack(packBytes)
		if perr != nil {
			return fmt.Errorf("pack: %w", perr)
		}
		ceiling = pk.Spec.Phase
	}

	// 3. The governed file path is the --subject entryRef with the "file:" prefix
	//    stripped (REQ-06: the frozen MergePolicy has no subject field, so the
	//    governed subject is supplied explicitly). Fetch base (target ref) and head
	//    (source ref) content. A 404 maps to nil bytes (absent) — the EFE-S03
	//    presence signal — rather than a hard error, so a true whole-file
	//    add/delete can mint a FileEvent below.
	governed := strings.TrimPrefix(cfg.subject, "file:")
	if governed == cfg.subject {
		return fmt.Errorf("--subject %q must be a file:<path> entryRef", cfg.subject)
	}
	base, err := fileAtRefOrAbsent(client, cfg.project, governed, info.TargetBranch)
	if err != nil {
		return fmt.Errorf("load governed base %q: %w", governed, err)
	}
	head, err := fileAtRefOrAbsent(client, cfg.project, governed, info.SourceBranch)
	if err != nil {
		return fmt.Errorf("load governed head %q: %w", governed, err)
	}
	// When --checkout is set, the LOCAL tree is the presence authority for the
	// governed subject (EFE-S03 / ADR-0008 §4): FileAtRef alone cannot express
	// nil-vs-empty through every forge mock, and the changed-file fold already
	// reads this tree. Override base/head from the checkout's FileContents.
	if cfg.checkout != "" {
		base, head, err = dirCheckout{root: cfg.checkout}.FileContents(governed)
		if err != nil {
			return fmt.Errorf("read governed %q from checkout: %w", governed, err)
		}
	}

	// 4. Diff → ChangeSet (or mint a whole-file FileEvent on one-sided presence).
	//    An opaque Diff is fail-safe REVIEW via decide (changeSet.Opaque), not a
	//    hard error — matching the E1-S08 checkout-fold soft-opaque path and the
	//    adoptertest undecidable guard. A non-opaque error still fails CLOSED.
	changeSet, err := changeSetForGoverned(governed, base, head)
	if err != nil && !errors.Is(err, change.ErrOpaque) {
		return fmt.Errorf("diff governed file %q: %w", governed, err)
	}

	// 5. Classify (path-only) and fold the checkout's safety signals.
	subjectClass := classify.Classify(changeSet)

	// 5b. Changed-file-set safety fold (E1-S08 + E4-S06 judgment call (e)):
	//       - `--checkout` set → LOCAL tree is sole authority (E1-S08); Snapshot
	//         path-list is NOT used for classifier input (D-077, no union);
	//       - `--checkout` unset → Snapshot changed files feed the path-only
	//         classifier (assent-policy dominance over the MR's full forge-reported
	//         set, not the governed subject alone).
	//     Byte-level opacity stays the local checkout fold's job (Snapshot paths
	//     carry no diff bytes), but the Snapshot fold DOES carry the ADR-0020 /
	//     D-119 enumeration-completeness signal: an enumeration the forge cannot
	//     prove complete is folded opaque → fail-safe REVIEW, because here that
	//     path list is the sole `.assent/**` detector and a truncated one would
	//     starve the D-042 self-vouch guard. The governed subject's ChangeSet
	//     stays the predicate input in both modes.
	if cfg.checkout != "" {
		fold, ferr := foldCheckout(dirCheckout{root: cfg.checkout}, governed)
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
	} else {
		fold := foldSnapshotPaths(snapshot)
		if fold.class == classify.ClassAssentPolicy {
			subjectClass = classify.ClassAssentPolicy
		}
		if fold.opaque && !changeSet.Opaque {
			changeSet.Opaque = true
			// The fold already carries the NORMATIVE ADR-0020 §4 reason; do not
			// re-prefix it (that would double the contractual prefix).
			changeSet.OpaqueReason = fold.opaqueReason
		}
	}

	// 5c. E5-S05: resolve configured providers into EvaluationInput.Facts +
	//     pins.factsResolvedAt at the cmd/assent edge (never inside internal/core).
	//     Nil/empty providers → empty maps (byte-identical to pre-S05).
	evalNow := clock().UTC()
	facts, factsResolvedAt, ferr := resolveRunFacts(
		context.Background(), conf, cfg.config, client, cfg.project, info.TargetBranch,
		cfg.checkout, cfg.subject, evalNow,
	)
	if ferr != nil {
		return fmt.Errorf("resolve providers: %w", ferr)
	}

	// 5d. E4-S06: Resolve forge-proven ApprovalEvidence (or explicit gap) before
	//     CoverWithApproval — never silent APPROVE on missing proof (E2-S07).
	appr, err := resolveRunApproval(client, cfg, info, mergeDigest, mrAuthor)
	if err != nil {
		return fmt.Errorf("resolve approval: %w", err)
	}

	// 6. Decide (E2-S04 coverage over the decoded EvaluationInput), with the two
	//    dominating fail-safe guards reasserted around it (see decide).
	result, err := decide(cfg.subject, subjectClass, changeSet, mp, bind, ceiling, info, facts, appr, mrAuthor)
	if err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}

	// 6. Build the DecisionRecord. GitLab plain-merge exposes no merge-result
	//    digest, so the record honestly carries mergeResultDigest:null +
	//    capabilityGap (ADR-0017 §1). Tool/policy digests are non-empty.
	mergeGap, err := decision.MergeResultGap("gitlab plain-merge exposes no merge-result digest (no merge train); ADR-0017 §1 capabilityGap")
	if err != nil {
		return fmt.Errorf("merge-result gap: %w", err)
	}
	if factsResolvedAt == nil {
		factsResolvedAt = map[string]string{}
	}
	pins := decision.Pins{
		ToolVersion:     version,
		ToolDigest:      toolDigest(version),
		PolicySha:       sha256Prefix + sha256Hex(mpBytes),
		SourceSha:       info.SourceSHA,
		TargetSha:       info.TargetSHA,
		MergeResult:     mergeGap,
		FactsResolvedAt: factsResolvedAt,
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

	// 8. Map the decision → forge intent, and reconcile — except GUARD 1
	//    (E4-S08 / D-042 F1): a self-modifying `.assent/**` MR earns BLOCK but
	//    ZERO forge writes (no thread, approve, or merge).
	//    GUARD 2 (E7-S03 / ADR-0015 §8): fork/untrusted MR context is advisory-only.
	var receipt forge.PublicationReceipt
	var recErr error
	switch {
	case reservedSelfEditBlock(result):
		// zero writes — summary only
	case untrustedExecutionContext(snapshot):
		// zero writes — fork MR must not hold write authority (ADR-0015 §8)
	default:
		armEligible := probe.ArmEligible
		if result.Decision == aggregate.DecisionApprove && !controllingFactsFreshAt(mp, facts, clock().UTC()) {
			armEligible = false
		}
		desired, pre := buildDesired(
			cfg, info, cfg.subject, head, result, recordJSON, armEligible,
			report.Presentation, buildRenderContext(renderOpts(conf, bind), mp, bind, changeSet, facts, info, mrAuthor),
		)
		receipt, recErr = forge.Reconcile(client, clockAdapter{now: clock}, desired, pre)
	}

	// 9. Emit the DecisionRecord + a one-line summary. A refusal from Reconcile
	//    that is an EXPECTED fail-closed outcome (arming unmet, SHA moved) is NOT
	//    a hard error — it is the advisory/no-write result the run is designed to
	//    produce; report it in the summary and still exit 0.
	if err := emitRecord(cfg, recordJSON, stdout); err != nil {
		return fmt.Errorf("emit decision record: %w", err)
	}
	var summary string
	switch {
	case reservedSelfEditBlock(result):
		summary = "decision=BLOCK → assent-policy self-edit, no forge writes (fail-closed)"
	case untrustedExecutionContext(snapshot):
		summary = fmt.Sprintf("decision=%s → fork/untrusted MR context, advisory-only (no forge writes, ADR-0015 §8)", result.Decision)
	default:
		summary = summarize(result.Decision, cfg.arm, receipt, recErr)
	}
	_, _ = fmt.Fprintln(stdout, summary)

	// A hard forge error (not a fail-closed refusal) IS a failure.
	if recErr != nil && !isFailClosed(recErr) {
		return fmt.Errorf("reconcile: %w", recErr)
	}
	return nil
}

// fileAtRefOrAbsent loads a file at ref, mapping a forge 404 to nil bytes (the
// file is ABSENT at that ref — EFE-S03 presence signal). Any other error
// propagates. Empty-but-present content (HTTP 200 with []byte{}) stays non-nil.
func fileAtRefOrAbsent(client forgePort, project, path, ref string) ([]byte, error) {
	raw, err := client.FileAtRef(project, path, ref)
	if err != nil {
		if errors.Is(err, gitlab.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return raw, nil
}

// changeSetForGoverned builds the governed subject's ChangeSet: on unambiguous
// one-sided presence it mints change.FileEvent (the sole path=="" minter); otherwise
// it falls through to change.Diff. Shared by the live Diff→ChangeSet path so the
// checkout fold's OneSidedLifecycle skip and the evaluator's input agree.
func changeSetForGoverned(file string, base, head []byte) (change.ChangeSet, error) {
	if kind, ok := change.OneSidedLifecycle(base, head); ok {
		return change.ChangeSet{Changes: []change.Change{change.FileEvent(file, kind)}}, nil
	}
	return change.Diff(file, base, head)
}

// selectBinding routes to the single covering RulesetBinding. Full
// (class, environment) routing needs the Config class-matcher (the domain class
// — e.g. topic-registry — is not one of classify's reserved assent-policy/
// unclassified classes), which is not wired in this lane; so a multi-binding
// document fails CLOSED (no write) rather than silently pick one. A zero-binding
// document likewise fails closed.
func selectBinding(rb *policy.RulesetBinding) (*policy.Binding, error) {
	if rb == nil || len(rb.Bindings) == 0 {
		return nil, fmt.Errorf("ruleset-binding declares no bindings")
	}
	if len(rb.Bindings) > 1 {
		return nil, fmt.Errorf("ruleset-binding declares %d bindings — (class, environment) routing needs the Config class-matcher (not wired in this lane); supply a single-binding RulesetBinding", len(rb.Bindings))
	}
	return &rb.Bindings[0], nil
}

// decide reduces the run to an aggregate.Result, with the two dominating fail-safe
// guards reasserted AROUND the E2-S04 coverage loop (which has no such hooks):
//
//   - GUARD 1 (reserved-class self-edit): an MR touching `.assent/**` dominates to
//     BLOCK before any predicate — it cannot vouch for itself (ADR-0015 §1, D-042).
//     This hook lives ONLY in the walking-skeleton Aggregate; Cover has none, so
//     the re-seat reasserts it here.
//   - undecidable changeset (opaque or empty): Cover treats "no matched change" as
//     an obligation that does not apply and could APPROVE, so an opaque/empty diff
//     short-circuits to the fail-safe REVIEW here, matching the skeleton's
//     failSafe(), never a silent APPROVE.
//
// Otherwise the frozen engine decides over the decoded EvaluationInput with
// forge-resolved ApprovalEvidence (E4-S06) and the pack phase ceiling. Every
// finding is then given a non-empty subject (N1).
//
// facts is the E5-S05 host-resolved envelope (provider → name → Fact). Nil/empty
// keeps the pre-S05 fail-safe empty map from buildEvaluationInput.
func decide(subject, subjectClass string, cs change.ChangeSet, mp *policy.MergePolicy, bind *policy.Binding, ceiling policy.Phase, info gitlab.MRInfo, facts map[string]map[string]aggregate.Fact, appr *aggregate.ApprovalContext, mrAuthor string) (aggregate.Result, error) {
	var res aggregate.Result
	switch {
	case subjectClass == classify.ClassAssentPolicy:
		res = reservedClassBlock(subject)
	case cs.Opaque || len(cs.Changes) == 0:
		res = undecidableReview(subject)
	default:
		in := buildEvaluationInput(cs, mrFrom(info, mrAuthor), bind.Require)
		if len(facts) > 0 {
			in.Facts = facts
		}
		r, err := aggregate.CoverWithPhaseCeiling(mp, bind, &in, appr, ceiling)
		if err != nil {
			return aggregate.Result{}, err
		}
		res = r
	}
	return sanitizeSubjects(res, subject), nil
}

// resolveRunApproval maps a require-review subject + pinned SHAs to forge-proven
// ApprovalEvidence or an explicit capability gap (never silent APPROVE).
func resolveRunApproval(client forgePort, cfg runConfig, info gitlab.MRInfo, mergeDigest, mrAuthor string) (*aggregate.ApprovalContext, error) {
	res, err := client.Resolve(forge.ResolveRequest{
		Project:           cfg.project,
		MR:                cfg.mr,
		Subject:           cfg.subject,
		SourceSha:         info.SourceSHA,
		TargetSha:         info.TargetSHA,
		MergeResultDigest: mergeDigest,
		MRAuthor:          mrAuthor,
	})
	if err != nil {
		return nil, err
	}
	if err := res.WellFormed(); err != nil {
		return nil, err
	}
	appr := &aggregate.ApprovalContext{
		SourceSha: info.SourceSHA,
		Evidence:  map[string]*aggregate.ApprovalEvidence{},
	}
	if res.HasEvidence() {
		appr.Evidence[cfg.subject] = res.Evidence
		return appr, nil
	}
	if res.HasGap() {
		// Tier / API gap → verifyingCapability:none so require-review records a
		// capability gap distinctly (never APPROVE on missing proof).
		appr.Evidence[cfg.subject] = &aggregate.ApprovalEvidence{
			VerifyingCapability: "none",
			Pins:                aggregate.ApprovalPins{SourceSha: info.SourceSHA},
		}
	}
	return appr, nil
}

// mrFrom builds the engine's aggregate.MR from the forge MR metadata plus the
// MR author (Snapshot heads) for require-review self-approval exclusion.
func mrFrom(info gitlab.MRInfo, author string) aggregate.MR {
	return aggregate.MR{
		Author:       author,
		SourceBranch: info.SourceBranch,
		TargetBranch: info.TargetBranch,
	}
}

// reservedSelfEditBlock reports GUARD 1 outcomes: BLOCK driven by the
// assent-policy.self-edit finding (E4-S08 / D-042 F1). These skip Reconcile.
func reservedSelfEditBlock(res aggregate.Result) bool {
	if res.Decision != aggregate.DecisionBlock {
		return false
	}
	for _, f := range res.Findings {
		if f.Code == "assent-policy.self-edit" {
			return true
		}
	}
	return false
}

// reservedClassBlock is the reserved-class self-edit BLOCK result, reconstructing
// exactly what aggregate.Aggregate's reserved-class short-circuit emits (a smuggled
// `.assent/**` edit can never vouch for itself, ADR-0015 §1).
func reservedClassBlock(subject string) aggregate.Result {
	return aggregate.Result{
		Decision: aggregate.DecisionBlock,
		Findings: []aggregate.Finding{{
			Rule:    aggregate.ReservedPolicyClass,
			Effect:  aggregate.EffectBlock,
			Subject: subject,
			Points:  0,
			Code:    "assent-policy.self-edit",
		}},
	}
}

// undecidableReview is the fail-safe REVIEW result for an opaque/empty changeset,
// reconstructing the skeleton aggregate.failSafe() finding so the outcome is
// auditable (never a silent APPROVE).
func undecidableReview(subject string) aggregate.Result {
	return aggregate.Result{
		Decision: aggregate.DecisionReview,
		Findings: []aggregate.Finding{{
			Rule:    "aggregate.changeset",
			Effect:  aggregate.EffectRequireReview,
			Subject: subject,
			Points:  0,
			Code:    "changeset.undecidable",
		}},
	}
}

// sanitizeSubjects gives every finding a non-empty subject (N1). The E2-S04
// coverage loop emits an uncovered-obligation finding with Subject:"" (no matched
// change to attribute it to), which violates the DecisionRecord finding subject
// (entryRef minLength:1) on serialization. A per-obligation sentinel keeps the
// (rule, subject) uniqueKey DISTINCT across two uncovered obligations (the schema's
// x-uniqueKeys is enforced); a finding with neither subject nor obligation falls
// back to the governed subject. Cover's own findings already carry subjects, so
// this is a no-op for them.
func sanitizeSubjects(res aggregate.Result, fallback string) aggregate.Result {
	fix := func(fs []aggregate.Finding) {
		for i := range fs {
			if fs[i].Subject != "" {
				continue
			}
			if fs[i].Obligation != "" {
				fs[i].Subject = "obligation:" + fs[i].Obligation
				continue
			}
			fs[i].Subject = fallback
		}
	}
	fix(res.Findings)
	fix(res.Observed)
	return res
}

// buildDesired maps the aggregate decision to a forge.DesiredReviewState +
// Preconditions.
//
//   - REVIEW/BLOCK → exactly one resolvable thread carrying the marker plus a
//     human body naming the driving rule/code. No approve/merge.
//   - APPROVE → approve + a SHA-pinned merge, gated by arming.
//
// Arming: ArmEligible comes from the forge-probed PreconditionReport (E4-S05/S06),
// NOT from --arm or readPipelineDescription() env vars (D-034/D-074). When the
// forge probe refuses, Reconcile returns ErrArmingRefused → no write even if
// --arm was passed.
func buildDesired(cfg runConfig, info gitlab.MRInfo, subject string, head []byte, result aggregate.Result, recordJSON []byte, armEligible bool, pm decision.PresentationModel, rctx render.Context) (forge.DesiredReviewState, forge.Preconditions) {
	digest := gitlab.SyntheticDigest(info.SourceSHA, info.TargetSHA)
	desired := forge.DesiredReviewState{Project: cfg.project, MR: cfg.mr}

	summaryBody, err := render.RenderSummary(pm, rctx)
	if err != nil {
		summaryBody = fmt.Sprintf("Decision: %s", pm.Decision)
	}
	desired.Summary = &forge.DesiredSummary{
		Marker: buildSummaryMarker(cfg, recordJSON),
		Body:   summaryBody,
	}

	if result.Decision == aggregate.DecisionApprove {
		desired.Approve = true
		desired.Merge = &forge.DesiredMerge{
			SourceSha:         info.SourceSHA,
			TargetSha:         info.TargetSHA,
			MergeResultDigest: digest,
		}
		pre := forge.Preconditions{
			ArmEligible:       armEligible,
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
	effect, rule, _ := drivingFinding(result)
	marker := forge.Marker{
		Slot: forge.Slot{
			Project:  cfg.project,
			MR:       cfg.mr,
			Rule:     rule,
			EntryRef: subject,
			Effect:   effect,
		},
		Occurrence: sha256Prefix + sha256Hex(head),
		Decision:   sha256Prefix + sha256Hex(recordJSON),
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
	finding := presentationFinding(result)
	if len(pm.Findings) > 0 {
		finding = pm.Findings[0]
	}
	threadBody, err := render.RenderFindingThread(pm, finding, rctx)
	if err != nil {
		threadBody = fmt.Sprintf("Review required: rule %q (%s)", rule, finding.Code)
	}
	desired.Thread = &forge.DesiredThread{Marker: marker, Body: threadBody}
	return desired, forge.Preconditions{}
}

func buildSummaryMarker(cfg runConfig, recordJSON []byte) forge.Marker {
	decisionDigest := sha256Prefix + sha256Hex(recordJSON)
	return forge.Marker{
		Slot: forge.Slot{
			Project: cfg.project,
			MR:      cfg.mr,
			Rule:    "assent/summary",
			Effect:  "comment",
		},
		Occurrence: decisionDigest,
		Decision:   decisionDigest,
		Artifact:   forge.Artifact{Kind: "summary-comment", SchemaVersion: "v1alpha1"},
	}
}

func renderOpts(conf *policy.Config, bind *policy.Binding) render.Options {
	opts := render.DefaultOptions()
	if conf == nil || bind == nil {
		return opts
	}
	return render.OptionsForEnvironment(conf, bind.Environment)
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
	return os.WriteFile(cfg.emit, recordJSON, 0o600) // #nosec G703 -- operator-supplied --emit path by design
}

// validateRecord validates DecisionRecord JSON against the frozen schema.
func validateRecord(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("record is not JSON: %w", err)
	}
	return schemas.DecisionRecordSchema.Validate(doc)
}

// sha256Prefix is the digest-algorithm tag prepended to every hex sha256 so the
// marker's ^sha256:[0-9a-f]{64}$ grammar and the pinned digests share one literal.
const sha256Prefix = "sha256:"

// sha256Hex returns the lowercase hex sha256 of b (a real 64-hex digest so the
// marker's ^sha256:[0-9a-f]{64}$ grammar and the non-empty pins are satisfied).
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// buildInfoUnavailableLabel prefixes the fallback digest input (D-120). It exists
// so the fallback is an HONESTLY LABELLED value rather than a digest that looks
// like a content claim: sha256("buildinfo-unavailable\n"+toolVersion) collides
// only with other builds that also had nothing to prove, and the schema's
// toolDigest description states this formula verbatim.
const buildInfoUnavailableLabel = "buildinfo-unavailable\n"

// toolDigest returns the pins.toolDigest value for THIS binary (D-120, AUD-S04 /
// audit finding ARCH-03). Before D-120 this was sha256(version), which made the
// schema's "content digest of the evaluating tool build" a false claim: every
// build carrying `-X main.version=1.2.3` shared one digest regardless of content.
func toolDigest(version string) string {
	bi, ok := debug.ReadBuildInfo()
	return toolDigestFrom(bi, ok, version)
}

// toolDigestFrom is toolDigest with the build info injected, so both branches are
// reachable from tests (a test binary cannot forge its own build info).
func toolDigestFrom(bi *debug.BuildInfo, ok bool, version string) string {
	if ok && bi != nil {
		if canonical, contentBearing := canonicalBuildInfo(bi); contentBearing {
			return sha256Prefix + sha256Hex([]byte(canonical))
		}
	}
	return sha256Prefix + sha256Hex([]byte(buildInfoUnavailableLabel+version))
}

// canonicalBuildInfo renders the D-120 content-bearing subset of Go build info —
// main module path/version/sum, dependency (and replacement) checksums, VCS
// revision and dirty flag — as deterministic text, and reports whether that text
// actually identifies the MAIN module's content.
//
// Only two signals do: a module sum (present for `go install path@version`) and a
// VCS revision (present for `go build`/goreleaser inside a checkout). Dependency
// sums deliberately do NOT count: a binary with pinned deps and no main-module
// identity — a `go test` binary, or `go build -buildvcs=false` — would otherwise
// hash to the same value for every revision of this repo, i.e. exactly the
// constant-digest false provenance ARCH-03 exists to remove. Such builds fall
// back to the labelled digest instead.
//
// Deliberately EXCLUDED: the toolchain's environment settings (GOOS/GOARCH,
// CGO_*, DefaultGODEBUG, -ldflags) and vcs.time. They are not part of the D-120
// input list the schema description publishes, and several vary with the
// toolchain rather than the source. Consequence, honestly: the per-platform
// release binaries of one commit share a digest, and (per D-120) an uncommitted
// edit's content is not captured — only flagged, via the hashed vcs.modified.
//
// The field separators are tab/newline, which module paths, versions, checksums
// and revisions cannot contain, so distinct build info yields distinct text.
func canonicalBuildInfo(bi *debug.BuildInfo) (string, bool) {
	revision := buildSetting(bi, "vcs.revision")

	var b strings.Builder
	fmt.Fprintf(&b, "mod\t%s\t%s\t%s\n", bi.Main.Path, bi.Main.Version, bi.Main.Sum)

	// Sort by path: the toolchain already emits Deps sorted, but dependency ORDER
	// is not build content and the digest must not rest on that promise.
	deps := make([]*debug.Module, 0, len(bi.Deps))
	for _, d := range bi.Deps {
		if d != nil {
			deps = append(deps, d)
		}
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Path != deps[j].Path {
			return deps[i].Path < deps[j].Path
		}
		return deps[i].Version < deps[j].Version
	})
	for _, d := range deps {
		fmt.Fprintf(&b, "dep\t%s\t%s\t%s\n", d.Path, d.Version, d.Sum)
		if d.Replace != nil {
			// A replace directive is where the code actually came from.
			fmt.Fprintf(&b, "rep\t%s\t%s\t%s\n", d.Replace.Path, d.Replace.Version, d.Replace.Sum)
		}
	}

	fmt.Fprintf(&b, "vcs.revision\t%s\n", revision)
	fmt.Fprintf(&b, "vcs.modified\t%s\n", buildSetting(bi, "vcs.modified"))

	return b.String(), bi.Main.Sum != "" || revision != ""
}

// buildSetting looks a build setting up by key, returning "" when absent. Keys are
// looked up by name rather than ranged over so toolchain- and platform-dependent
// settings can never leak into the digest.
func buildSetting(bi *debug.BuildInfo, key string) string {
	for _, s := range bi.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}
