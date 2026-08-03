// Package decision is the PURE serializer for the P4-E1 walking skeleton
// (P4-E1-S04, ADR-0016 §3, ADR-0017 §1/§9). It turns the S03 aggregate outcome
// (decision + findings) plus the S01 pins into two frozen-contract artifacts:
//
//   - a DecisionRecord that validates against
//     schemas/decision/v1alpha1/decision-record.schema.json — the redacted,
//     replayable audit/stats record; and
//   - a companion PresentationModel that validates against
//     presentation-model.schema.json — the renderer-only view.
//
// Redaction is BY CONSTRUCTION (ADR-0016 §3): the finding shape carries no
// raw-fact-value field, so no fact value can leak through this serializer, and
// the PresentationModel adds no rendered markdown (ADR-0016 §4 — not a declared
// property, deliberately omitted).
//
// The silent-widening trap this package closes (ADR-0017 §1). The frozen pins
// schema couples mergeResultDigest and capabilityGap with an allOf/if-then:
// a NULL mergeResultDigest REQUIRES a non-empty capabilityGap, and a STRING
// mergeResultDigest FORBIDS capabilityGap. A null digest with no gap would be a
// silent widening of what was actually pinned. This package makes that state
// UNREPRESENTABLE: MergeResult is a sum type with exactly two constructors
// (Pinned / Gap), and marshalPins derives BOTH JSON fields from that one value,
// so the serializer has no code path that emits null-without-gap. The
// coupling is decision-independent — an APPROVE emitted BEFORE the S08 merge
// path still legitimately carries a null digest + a capabilityGap explaining
// the merge-result digest is not yet computed in this slice.
//
// Purity (GUIDELINES §5, ADR-0013): this package reads no clock, randomness,
// environment, or network. Any timestamp (e.g. an S01 EvaluatedAt) enters as an
// injected value; findings are sorted by a total key so a double-run of the
// serializer is byte-identical.
package decision

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

const (
	apiVersion            = "assent.dev/v1alpha1"
	kindDecisionRecord    = "DecisionRecord"
	kindPresentationModel = "PresentationModel"
)

// MergeResult is the sum type modelling the frozen pins coupling between
// mergeResultDigest and capabilityGap (ADR-0017 §1). It has exactly two states,
// both built through constructors that this package controls, so the invalid
// "null digest, no capabilityGap" combination cannot be constructed. The zero
// value is deliberately NOT valid — callers MUST use PinnedMergeResult or
// MergeResultGap; a zero MergeResult is treated as a gap with an explicit
// "unspecified" reason so it can never serialize to a silent null.
type MergeResult struct {
	// digest is the evaluated merge-result digest when pinned; "" means "gap".
	digest string
	// gap explains which forge capability was absent when digest is unpinned.
	gap string
}

// PinnedMergeResult builds a MergeResult carrying a real, evaluated merge-result
// digest. The digest MUST be non-empty; an empty digest is a caller bug (a
// "pinned" nothing) and is rejected so it can never masquerade as a valid pin.
func PinnedMergeResult(digest string) (MergeResult, error) {
	if digest == "" {
		return MergeResult{}, fmt.Errorf("decision: PinnedMergeResult requires a non-empty digest")
	}
	return MergeResult{digest: digest}, nil
}

// MergeResultGap builds a MergeResult for the case where the forge capability to
// compute/pin the merge result is absent (ADR-0017 §1) — e.g. the S04-only
// slice before S08's merge path. The reason MUST be non-empty: a null digest is
// only ever emitted alongside a capabilityGap, never silently. An empty reason
// is rejected so the serializer can never widen what was pinned.
func MergeResultGap(reason string) (MergeResult, error) {
	if reason == "" {
		return MergeResult{}, fmt.Errorf("decision: MergeResultGap requires a non-empty capability-gap reason")
	}
	return MergeResult{gap: reason}, nil
}

// SkeletonMergeGap is the standard MergeResultGap reason for an S04-only run:
// the merge-result digest is not computed until the S08 approve+SHA-pinned-merge
// path exists. It is a convenience for the common walking-skeleton case; callers
// that DO have a digest (S08) use PinnedMergeResult instead.
func SkeletonMergeGap() MergeResult {
	// Constructed from a fixed non-empty reason; the error is impossible here.
	mr, _ := MergeResultGap("merge-result digest not computed in the P4-E1 walking skeleton (no merge path until S08, ADR-0017 §1)")
	return mr
}

// pinned reports whether this MergeResult carries a real digest. A zero-value
// MergeResult (never produced by a constructor, but defensively handled) is
// treated as a gap, so it can never serialize to null-without-capabilityGap.
func (m MergeResult) pinned() bool { return m.digest != "" }

// Pins are the INPUTS to the DecisionRecord pins block. Some come from the S01
// adapter's out-of-band Pins (SourceSHA/TargetSHA, and the injected EvaluatedAt
// if a provider timestamp is ever recorded); ToolVersion/ToolDigest/PolicySha
// are caller-supplied out-of-band build/policy values (cmd/assent populates them
// from build info + the loaded policy — this pure package never reads build info
// or env). MergeResult is the sum type above. FactsResolvedAt is keyed by
// provider name -> RFC3339 timestamp; the provider-less skeleton passes an empty
// map, which is valid (no required keys).
type Pins struct {
	ToolVersion     string
	ToolDigest      string
	PolicySha       string
	SourceSha       string
	TargetSha       string
	MergeResult     MergeResult
	FactsResolvedAt map[string]string
}

// finding is the JSON projection of one #/$defs/finding object. It mirrors the
// aggregate.Finding shape exactly (rule, effect, subject, points; optional
// obligation/code), carrying NO raw-fact-value field — redaction by
// construction (ADR-0016 §3). points is stamped from the aggregate finding
// (0 in the walking skeleton; S04 owns points provenance).
type finding struct {
	Rule       string `json:"rule"`
	Obligation string `json:"obligation,omitempty"`
	Effect     string `json:"effect"`
	Subject    string `json:"subject"`
	Points     int    `json:"points"`
	Code       string `json:"code,omitempty"`
}

// findingsObject is the DecisionRecord phase-split findings collection (D-017):
// {observed, enforcing}. The walking skeleton has no observe phase, so observed
// is always the empty (but non-nil) slice; enforcing carries the S03 findings.
type findingsObject struct {
	Observed  []finding `json:"observed"`
	Enforcing []finding `json:"enforcing"`
}

// pinsObject is the closed (additionalProperties:false) pins block. It is built
// only by marshalPins so the mergeResultDigest/capabilityGap coupling is always
// consistent. mergeResultDigest is a *string with NO omitempty (required; nil
// marshals to null); capabilityGap has omitempty (present iff the digest is
// null); factsResolvedAt is always a non-nil map so it marshals to {} not null.
type pinsObject struct {
	ToolVersion       string            `json:"toolVersion"`
	ToolDigest        string            `json:"toolDigest"`
	PolicySha         string            `json:"policySha"`
	SourceSha         string            `json:"sourceSha"`
	TargetSha         string            `json:"targetSha"`
	MergeResultDigest *string           `json:"mergeResultDigest"`
	CapabilityGap     string            `json:"capabilityGap,omitempty"`
	FactsResolvedAt   map[string]string `json:"factsResolvedAt"`
}

// Record is the top-level DecisionRecord audit artifact. Field order + json tags
// shape it exactly to decision-record.schema.json (top level is
// additive-tolerant, so only the required keys must be present and well-typed).
// Named Record (not DecisionRecord) to avoid the decision.DecisionRecord stutter.
type Record struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Decision   string         `json:"decision"`
	Findings   findingsObject `json:"findings"`
	Pins       pinsObject     `json:"pins"`
}

// PresentationModel is the redacted renderer-only view. Its findings are an
// ARRAY of the SAME finding shape (mirrored via $ref in the schema). It carries
// no raw fact value (the finding shape has none) and no rendered markdown
// (ADR-0016 §4 — deliberately not a declared property).
type PresentationModel struct {
	APIVersion string    `json:"apiVersion"`
	Kind       string    `json:"kind"`
	Decision   string    `json:"decision"`
	Findings   []finding `json:"findings"`
}

// Report is the pair of artifacts a run emits: the DecisionRecord (Record) and
// its companion PresentationModel. Both derive from the SAME sorted finding set
// and the SAME decision, so they never disagree.
type Report struct {
	Record       Record
	Presentation PresentationModel
}

// Build serializes the S03 aggregate outcome + the pins into the DecisionRecord
// and its companion PresentationModel.
//
// P3 SEAM NOTE (S03 review F3, not fixed here): the aggregator treats an empty
// require as APPROVE-by-design; the CLI wiring (a different lane) must guarantee
// require is non-empty before an APPROVE is armed. This serializer only reports
// whatever decision S03 produced — it does not re-derive or change it.
func Build(res aggregate.Result, pins Pins) (Report, error) {
	fs := toFindings(res.Findings)

	pinsObj, err := marshalPins(pins)
	if err != nil {
		return Report{}, err
	}

	rec := Record{
		APIVersion: apiVersion,
		Kind:       kindDecisionRecord,
		Decision:   string(res.Decision),
		Findings: findingsObject{
			// D-017 phase split (E2-S08): observed carries the OBSERVE-phase findings
			// the aggregator routed to res.Observed (structurally excluded from the
			// decision); enforcing carries the aggregated findings. toFindings returns
			// a non-nil slice, so a run with no observe rules still marshals observed
			// as [] (not null) — keeping the D-016 golden byte-identical.
			Observed:  toFindings(res.Observed),
			Enforcing: fs,
		},
		Pins: pinsObj,
	}
	pm := PresentationModel{
		APIVersion: apiVersion,
		Kind:       kindPresentationModel,
		Decision:   string(res.Decision),
		// Same finding shape as the DecisionRecord's enforcing set, as an array.
		Findings: fs,
	}
	return Report{Record: rec, Presentation: pm}, nil
}

// toFindings projects the aggregate findings into the wire finding shape, in a
// deterministic canonical order (the SAME total key aggregate.sortFindings uses
// — unexported there, reproduced here). Sorting here makes the serializer's
// output byte-stable independent of input order and across a double-run. The
// returned slice is always non-nil so it marshals to [] not null.
func toFindings(in []aggregate.Finding) []finding {
	out := make([]finding, len(in))
	for i, f := range in {
		out[i] = finding{
			Rule:       f.Rule,
			Obligation: f.Obligation,
			Effect:     string(f.Effect),
			Subject:    f.Subject,
			Points:     f.Points,
			Code:       f.Code,
		}
	}
	sortFindings(out)
	return out
}

// sortFindings orders findings by a TOTAL key (subject, rule, obligation, code,
// effect), mirroring aggregate.sortFindings so the report is order-independent
// and byte-stable on a double-run (ADR-0017 §9, ADR-0013).
func sortFindings(fs []finding) {
	sort.Slice(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Subject != b.Subject {
			return a.Subject < b.Subject
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Obligation != b.Obligation {
			return a.Obligation < b.Obligation
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Effect < b.Effect
	})
}

// marshalPins builds the closed pins block from the Pins inputs, deriving the
// mergeResultDigest/capabilityGap pair from the MergeResult sum type so the
// coupling is ALWAYS consistent (ADR-0017 §1). This is the single place the two
// coupled fields are set, and it has no branch that emits null-without-gap.
func marshalPins(p Pins) (pinsObject, error) {
	obj := pinsObject{
		ToolVersion: p.ToolVersion,
		ToolDigest:  p.ToolDigest,
		PolicySha:   p.PolicySha,
		SourceSha:   p.SourceSha,
		TargetSha:   p.TargetSha,
		// Always a non-nil map: a nil map marshals to null and would violate the
		// schema's object type; an empty map marshals to {} (valid — no required
		// provider keys in the provider-less skeleton).
		FactsResolvedAt: nonNilFacts(p.FactsResolvedAt),
	}
	if p.MergeResult.pinned() {
		d := p.MergeResult.digest
		obj.MergeResultDigest = &d // string digest; capabilityGap stays absent.
	} else {
		// Gap: mergeResultDigest is null (nil *string) and capabilityGap MUST be
		// present and non-empty. A zero-value MergeResult reaches here with an
		// empty gap; reject it rather than emit a silent null.
		if p.MergeResult.gap == "" {
			return pinsObject{}, fmt.Errorf("decision: unpinned merge result requires a capabilityGap (use MergeResultGap/SkeletonMergeGap; a zero-value MergeResult is not valid)")
		}
		obj.MergeResultDigest = nil
		obj.CapabilityGap = p.MergeResult.gap
	}
	return obj, nil
}

// nonNilFacts returns an always-non-nil factsResolvedAt map so it marshals to {}
// rather than null. The provider-less skeleton passes nil/empty.
func nonNilFacts(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// MarshalRecord serializes the DecisionRecord to canonical JSON bytes. It is a
// convenience for callers/tests that need the wire bytes (e.g. the double-run
// stability check and schema validation).
func (r Report) MarshalRecord() ([]byte, error) { return json.Marshal(r.Record) }

// MarshalPresentation serializes the PresentationModel to canonical JSON bytes.
func (r Report) MarshalPresentation() ([]byte, error) { return json.Marshal(r.Presentation) }
