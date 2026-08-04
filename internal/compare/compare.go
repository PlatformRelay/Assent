// Package compare is the PURE promotion-comparison engine behind `assent compare`
// (P5-E6-S09 seed, PCS epic extensions). Given ONE immutable ReplayBundle (its
// pre-built EvaluationInput) and a baseline vs a candidate profile, it evaluates
// both through the reused decision engine (aggregate.CoverWithProfile), classifies
// the resulting decision delta as one member of the frozen closed taxonomy, applies
// ONE promotion gate (the seed applies bounded-auto-merge-widening only; the full
// suite runner adds the remaining gates), and reports a pass/fail verdict a CLI shell
// maps to an exit code.
//
// PCS-S02/S03 grow the seed classifiers incrementally (ADR-0018 / D-117 priority
// table); PCS-S04 emits schema-valid ComparisonRecords per case. The seed still
// applies only bounded-auto-merge-widening. explanation-only never trips a gate;
// unclassified real deltas FAIL CLOSED. acceptedDeltas allowlist and the full
// five-gate evaluator are owed to PCS-S05+ (decisions.md D-057).
//
// It sits UNDER internal/ (not internal/core): like internal/adoptertest it may
// import internal/core/aggregate + internal/core/policy while internal/core stays
// change-/forge-free. It performs NO I/O — the ReplayBundle file read lives in the
// cmd/assent/compare.go shell. Every function is pure and deterministic: the same
// bundle + profiles double-run byte-identical (no clock/env/network/random).
package compare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/schemas"
)

// Kind is a member of the frozen closed delta taxonomy
// (schemas/comparison/v1alpha1/comparison-record.schema.json #/$defs/deltaKind).
// The constants below MUST equal that enum verbatim; a drift guard test
// (TestKindConstantsAreFrozenTaxonomy) validates each against the frozen schema.
// The zero value "" is NOT a taxonomy member — it means "no delta" (baseline and
// candidate fully agree), a gate-pass just like explanation-only.
type Kind string

const (
	// KindStricterInterventionAdded is when the candidate adds a stricter
	// intervention the baseline lacked (PCS-S02).
	KindStricterInterventionAdded Kind = "stricter-intervention-added"
	// KindDestructiveOrAuthorizationInterventionMissed is when the candidate misses
	// a destructive or authorization/ownership intervention the baseline had (PCS-S02).
	KindDestructiveOrAuthorizationInterventionMissed Kind = "destructive-or-authorization-intervention-missed"
	// KindSubjectOrObligationUncovered is when a subject/obligation covered by the
	// baseline is uncovered by the candidate (PCS-S03).
	KindSubjectOrObligationUncovered Kind = "subject-or-obligation-uncovered"
	// KindNewlyAutoMergeable is when the candidate newly permits auto-merge
	// (APPROVE) where the baseline intervened with a different finding set.
	KindNewlyAutoMergeable Kind = "newly-auto-mergeable"
	// KindScoreThresholdChange is when score/threshold or rule.points arithmetic
	// changed the outcome with identical intervention identities (PCS-S03).
	KindScoreThresholdChange Kind = "score-threshold-change"
	// KindExplanationOnly is wording-only / non-semantic presentation drift.
	// Classified by the seed and, by contract, NEVER trips a promotion gate.
	KindExplanationOnly Kind = "explanation-only"
)

// GateID is a frozen comparison-suite promotionGate identity
// (schemas/comparison/v1alpha1/comparison-suite.schema.json #/$defs/promotionGate).
type GateID string

// GateBoundedAutoMergeWidening is the ONE promotion gate the seed applies: it
// fails on a newly-auto-mergeable delta (failOnKinds: [newly-auto-mergeable],
// defaultVerdict: fail). The seed does not consult the acceptedDeltas allowlist
// (that per-delta escape is the full runner's job) — a widening always fails here.
const GateBoundedAutoMergeWidening GateID = "bounded-auto-merge-widening"

// Verdict is the promotion-gate outcome the CLI maps to an exit code.
type Verdict string

const (
	// VerdictPass means no gate-tripping delta — the candidate may be promoted.
	VerdictPass Verdict = "PASS"
	// VerdictFail means a gate-tripping delta — the candidate must NOT be promoted.
	VerdictFail Verdict = "FAIL"
)

// ErrUnclassifiable is the fail-closed sentinel: a real decision difference that
// matches none of the classified taxonomy kinds. It is NEVER a silent pass — the
// caller must surface it as a non-zero, non-gate outcome (a silent pass here would
// be a silent-approve of an unreviewed promotion).
var ErrUnclassifiable = errors.New("compare: decision delta matches none of the classified kinds (fail-closed)")

// Profile is one side of the comparison: the policy activation a named
// PolicyProfile stands for, plus everything aggregate.CoverWithProfile needs.
//
// NOTE (reuse boundary): CoverWithProfile resolves only WRITE AUTHORITY from the
// precedence/profiles table — it does NOT switch the evaluated policy by profile.
// So the decision delta flows from the Policy/Bind/Ceiling each profile activates,
// which the caller supplies explicitly. Wiring profile->pack activation so the
// decision itself flows from the resolved profile is owed to the full runner epic.
type Profile struct {
	// Name is the profile identity reported in the comparison (e.g. "prod@7").
	Name string
	// Policy is the MergePolicy this profile activates.
	Policy *policy.MergePolicy
	// Bind is the routing binding (require/environment/class) evaluated under.
	Bind *policy.Binding
	// Ceiling is the pack-level phase ceiling (empty normalizes to enforce).
	Ceiling policy.Phase
	// Approval is the optional injected approval context (nil = none).
	Approval *aggregate.ApprovalContext
	// Precedence/Profiles are the profile precedence table + documents used ONLY
	// for write-authority resolution; empty is the safe default (no write authority).
	Precedence []policy.ProfileRef
	Profiles   []*policy.Profile
}

// Comparison is the seed's result over one ReplayBundle.
type Comparison struct {
	// BaselineProfile / CandidateProfile echo the compared profile identities.
	BaselineProfile  string
	CandidateProfile string
	// Baseline / Candidate are the two produced decisions.
	Baseline  aggregate.Decision
	Candidate aggregate.Decision
	// Kind is the classified delta ("" when the two agree — no delta).
	Kind Kind
	// Gate is the promotion gate applied (always GateBoundedAutoMergeWidening in
	// the seed — the gate that owns the ONE semantic kind the seed classifies).
	Gate GateID
	// Verdict is the gate outcome (PASS unless a newly-auto-mergeable widening).
	Verdict Verdict
}

// LoadBundle strict-decodes raw JSON against the FROZEN replay-bundle schema and
// returns the pre-built EvaluationInput it carries. The schema's top-level
// additionalProperties:true is intentional (additive-tolerant reports, ADR-0017
// §9) — LoadBundle does NOT out-strict it; the nested EvaluationInput is
// re-validated by aggregate.LoadEvaluationInput (numeric-discipline decode). Pure.
func LoadBundle(raw []byte) (*aggregate.EvaluationInput, error) {
	doc, err := jsonNumberDoc(raw)
	if err != nil {
		return nil, fmt.Errorf("replay-bundle: %w", err)
	}
	if err := schemas.ReplayBundleSchema.Validate(doc); err != nil {
		return nil, fmt.Errorf("replay-bundle: %w", err)
	}
	var wrap struct {
		EvaluationInput json.RawMessage `json:"evaluationInput"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("replay-bundle decode: %w", err)
	}
	in, err := aggregate.LoadEvaluationInput(wrap.EvaluationInput)
	if err != nil {
		return nil, fmt.Errorf("replay-bundle: %w", err)
	}
	return in, nil
}

// Compare evaluates the shared ReplayBundle input under the baseline and candidate
// profiles via the reused engine, classifies the decision delta, and applies the
// bounded-auto-merge-widening gate. It returns a wrapped ErrUnclassifiable (never a
// PASS) when the delta is real but not one the seed classifies (fail-closed).
func Compare(in *aggregate.EvaluationInput, baseline, candidate Profile) (Comparison, error) {
	if in == nil {
		return Comparison{}, errors.New("compare: nil evaluation input")
	}
	baseRes, err := evaluate(in, baseline)
	if err != nil {
		return Comparison{}, fmt.Errorf("compare: baseline %q: %w", baseline.Name, err)
	}
	candRes, err := evaluate(in, candidate)
	if err != nil {
		return Comparison{}, fmt.Errorf("compare: candidate %q: %w", candidate.Name, err)
	}

	kind, err := classify(baseRes, candRes)
	if err != nil {
		return Comparison{}, err
	}
	return Comparison{
		BaselineProfile:  baseline.Name,
		CandidateProfile: candidate.Name,
		Baseline:         baseRes.Decision,
		Candidate:        candRes.Decision,
		Kind:             kind,
		Gate:             GateBoundedAutoMergeWidening,
		Verdict:          gateVerdict(kind),
	}, nil
}

// evaluate runs one profile's activation through the reused engine entry.
func evaluate(in *aggregate.EvaluationInput, p Profile) (aggregate.Result, error) {
	return aggregate.CoverWithProfile(p.Policy, p.Bind, in, p.Approval, p.Ceiling, p.Precedence, p.Profiles)
}

// gateVerdict applies the ONE seed gate (bounded-auto-merge-widening): a
// newly-auto-mergeable delta FAILs; every other classified kind (including
// explanation-only and the no-delta "") PASSes.
func gateVerdict(k Kind) Verdict {
	if k == KindNewlyAutoMergeable {
		return VerdictFail
	}
	return VerdictPass
}

// findingKeys returns the canonically sorted per-finding keys of a finding set.
// withMessage=false keys by full identity (rule|obligation|effect|subject|code|points)
// — including authored points weight; withMessage=true appends the rendered message,
// so a wording-only change shows up as an identity-equal / full-different pair.
// Score/threshold classification compares interventionKeys instead (points excluded).
func findingKeys(findings []aggregate.Finding, withMessage bool) []string {
	keys := make([]string, 0, len(findings))
	for _, f := range findings {
		k := fmt.Sprintf("%s|%s|%s|%s|%s|%d", f.Rule, f.Obligation, f.Effect, f.Subject, f.Code, f.Points)
		if withMessage {
			k += "|" + f.Message
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// interventionKeys returns canonically sorted intervention identity keys for each
// finding: rule|obligation|effect|subject|code. Points and message are excluded so
// a points-only or threshold-only arithmetic delta classifies as score-threshold-
// change rather than newly-auto-mergeable widening.
func interventionKeys(findings []aggregate.Finding) []string {
	keys := make([]string, 0, len(findings))
	for _, f := range findings {
		keys = append(keys, fmt.Sprintf("%s|%s|%s|%s|%s", f.Rule, f.Obligation, f.Effect, f.Subject, f.Code))
	}
	sort.Strings(keys)
	return keys
}

// equalStrings reports whether two already-canonical string slices are identical.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// jsonNumberDoc decodes raw into the any-tree jsonschema validates, numbers as
// json.Number (the shape the frozen schemas expect).
func jsonNumberDoc(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return doc, nil
}
