package compare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/schemas"
)

const (
	recordAPIVersion = "assent.dev/v1alpha1"
	recordKind       = "ComparisonRecord"
)

// ComparisonRecord is the schema-valid per-case promotion comparison artifact
// (schemas/comparison/v1alpha1/comparison-record.schema.json). Gate outcomes are
// NOT embedded — explanation-only deltas appear like any other delta without
// implying gate failure in this structure (S05 evaluates gates separately).
type ComparisonRecord struct {
	APIVersion       string  `json:"apiVersion"`
	Kind             string  `json:"kind"`
	BaselineProfile  string  `json:"baselineProfile"`
	CandidateProfile string  `json:"candidateProfile"`
	CaseID           string  `json:"caseId"`
	Deltas           []Delta `json:"deltas"`
}

// Delta is one classified difference with per-delta identity (kind, rule, subject
// [/ obligation]) and baseline vs candidate outcome sides.
type Delta struct {
	Kind       Kind        `json:"kind"`
	Rule       string      `json:"rule"`
	Subject    string      `json:"subject"`
	Obligation string      `json:"obligation,omitempty"`
	Baseline   OutcomeSide `json:"baseline"`
	Candidate  OutcomeSide `json:"candidate"`
}

// OutcomeSide is the baseline or candidate slice for a delta identity.
type OutcomeSide struct {
	Present  bool   `json:"present"`
	Decision string `json:"decision,omitempty"`
	Effect   string `json:"effect,omitempty"`
	Points   *int   `json:"points,omitempty"`
}

var errDuplicateDelta = errors.New("compare: duplicate delta identity (kind, rule, subject)")

type deltaUniqueKey struct {
	kind    Kind
	rule    string
	subject string
}

// BuildComparisonRecord assembles a ComparisonRecord for one suite case from the
// two evaluated Results. bundleSubjects supplies governed-subject entryRefs from
// the ReplayBundle changeSet when obligation.uncovered findings omit subject
// (CoverWithProfile). It fail-closes on unclassifiable deltas and on duplicate
// (kind, rule, subject) keys. Pure and deterministic.
func BuildComparisonRecord(caseID, baselineProfile, candidateProfile string, baseline, candidate aggregate.Result, bundleSubjects []string) (ComparisonRecord, error) {
	if caseID == "" {
		return ComparisonRecord{}, errors.New("compare: caseId is required")
	}
	if baselineProfile == "" || candidateProfile == "" {
		return ComparisonRecord{}, errors.New("compare: baselineProfile and candidateProfile are required")
	}

	deltas, err := collectDeltas(baseline, candidate, bundleSubjects)
	if err != nil {
		return ComparisonRecord{}, err
	}

	rec := ComparisonRecord{
		APIVersion:       recordAPIVersion,
		Kind:             recordKind,
		BaselineProfile:  baselineProfile,
		CandidateProfile: candidateProfile,
		CaseID:           caseID,
		Deltas:           deltas,
	}
	if err := rec.Validate(); err != nil {
		return ComparisonRecord{}, err
	}
	return rec, nil
}

// Validate checks x-uniqueKeys on deltas and validates against the frozen schema.
func (rec ComparisonRecord) Validate() error {
	if err := validateDeltaUniqueKeys(rec.Deltas); err != nil {
		return err
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("compare: marshal ComparisonRecord: %w", err)
	}
	doc, err := jsonNumberDoc(raw)
	if err != nil {
		return err
	}
	if err := schemas.ComparisonRecordSchema.Validate(doc); err != nil {
		return fmt.Errorf("compare: ComparisonRecord schema: %w", err)
	}
	return nil
}

func validateDeltaUniqueKeys(deltas []Delta) error {
	seen := make(map[deltaUniqueKey]struct{}, len(deltas))
	for _, d := range deltas {
		k := deltaUniqueKey{kind: d.Kind, rule: d.Rule, subject: d.Subject}
		if _, dup := seen[k]; dup {
			return fmt.Errorf("%w: kind=%q rule=%q subject=%q", errDuplicateDelta, d.Kind, d.Rule, d.Subject)
		}
		seen[k] = struct{}{}
	}
	return nil
}

func collectDeltas(baseline, candidate aggregate.Result, bundleSubjects []string) ([]Delta, error) {
	caseKind, err := classify(baseline, candidate)
	if err != nil {
		return nil, err
	}
	if caseKind == "" {
		return nil, nil
	}

	byKey := map[deltaUniqueKey]Delta{}
	baseByID := findingsByIdentity(baseline.Findings)
	candByID := findingsByIdentity(candidate.Findings)

	for id := range unionIdentityKeys(baseByID, candByID) {
		baseF, baseOK := baseByID[id]
		candF, candOK := candByID[id]
		var bf, cf *aggregate.Finding
		if baseOK {
			bf = &baseF
		}
		if candOK {
			cf = &candF
		}
		kind, ok := classifyIdentityDelta(baseline, candidate, bf, cf)
		if !ok {
			continue
		}
		d := buildDelta(kind, bf, cf, baseline.Decision, candidate.Decision, baseline, candidate, bundleSubjects)
		if err := insertDelta(byKey, d); err != nil {
			return nil, err
		}
	}

	if len(byKey) == 0 {
		return nil, fmt.Errorf("%w: case kind %q produced no per-delta identities", ErrUnclassifiable, caseKind)
	}

	out := make([]Delta, 0, len(byKey))
	for _, d := range byKey {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

func insertDelta(byKey map[deltaUniqueKey]Delta, d Delta) error {
	k := deltaUniqueKey{kind: d.Kind, rule: d.Rule, subject: d.Subject}
	if _, dup := byKey[k]; dup {
		return fmt.Errorf("%w: kind=%q rule=%q subject=%q", errDuplicateDelta, d.Kind, d.Rule, d.Subject)
	}
	byKey[k] = d
	return nil
}

func unionIdentityKeys(base, cand map[string]aggregate.Finding) map[string]struct{} {
	out := make(map[string]struct{}, len(base)+len(cand))
	for k := range base {
		out[k] = struct{}{}
	}
	for k := range cand {
		out[k] = struct{}{}
	}
	return out
}

func classifyIdentityDelta(baseline, candidate aggregate.Result, baseF, candF *aggregate.Finding) (Kind, bool) {
	baseOK := baseF != nil
	candOK := candF != nil

	if baseOK && candOK {
		if interventionIdentityEqual(baseF, candF) {
			if baseline.Decision == candidate.Decision && baseF.Message != candF.Message {
				return KindExplanationOnly, true
			}
			if baseline.Decision != candidate.Decision &&
				equalStrings(interventionKeys(baseline.Findings), interventionKeys(candidate.Findings)) {
				return KindScoreThresholdChange, true
			}
		}
		return "", false
	}

	if baseOK && !candOK {
		if isMissedInterventionEffect(baseF.Effect) && candidate.Decision != aggregate.DecisionApprove {
			return KindDestructiveOrAuthorizationInterventionMissed, true
		}
		if candidate.Decision == aggregate.DecisionApprove && baseline.Decision != aggregate.DecisionApprove {
			return KindNewlyAutoMergeable, true
		}
		return "", false
	}

	if !baseOK && candOK {
		if candF.Code == "obligation.uncovered" && !uncoveredObligations(baseline)[candF.Obligation] &&
			decisionWorsened(baseline.Decision, candidate.Decision) {
			return KindSubjectOrObligationUncovered, true
		}
		if isStricterInterventionEffect(candF.Effect) && baseline.Decision == aggregate.DecisionApprove {
			return KindStricterInterventionAdded, true
		}
		return "", false
	}

	return "", false
}

func interventionIdentityEqual(a, b *aggregate.Finding) bool {
	return a.Rule == b.Rule &&
		a.Obligation == b.Obligation &&
		a.Effect == b.Effect &&
		a.Subject == b.Subject &&
		a.Code == b.Code
}

func buildDelta(kind Kind, baseF, candF *aggregate.Finding, baseDecision, candDecision aggregate.Decision, baseline, candidate aggregate.Result, bundleSubjects []string) Delta {
	d := Delta{Kind: kind}
	if baseF != nil {
		d.Rule = baseF.Rule
		d.Subject = baseF.Subject
		d.Obligation = baseF.Obligation
		d.Baseline = outcomeSide(true, baseF, baseDecision)
	} else if candF != nil {
		d.Rule = candF.Rule
		d.Subject = candF.Subject
		d.Obligation = candF.Obligation
		d.Baseline = outcomeSide(false, nil, baseDecision)
	}
	if candF != nil {
		if d.Rule == "" {
			d.Rule = candF.Rule
		}
		if d.Subject == "" {
			d.Subject = candF.Subject
		}
		if d.Obligation == "" {
			d.Obligation = candF.Obligation
		}
		d.Candidate = outcomeSide(true, candF, candDecision)
	} else if baseF != nil {
		d.Candidate = outcomeSide(false, nil, candDecision)
	}
	if d.Subject == "" {
		d.Subject = resolveDeltaSubject(d.Obligation, baseline.Findings, candidate.Findings, bundleSubjects)
	}
	return d
}

// resolveDeltaSubject picks a governed-subject entryRef when a finding omits subject.
func resolveDeltaSubject(obligation string, baselineFindings, candidateFindings []aggregate.Finding, bundleSubjects []string) string {
	for _, findings := range [][]aggregate.Finding{baselineFindings, candidateFindings} {
		for _, f := range findings {
			if obligation != "" && f.Obligation == obligation && f.Subject != "" {
				return f.Subject
			}
		}
	}
	for _, findings := range [][]aggregate.Finding{baselineFindings, candidateFindings} {
		for _, f := range findings {
			if f.Subject != "" {
				return f.Subject
			}
		}
	}
	if len(bundleSubjects) > 0 {
		return bundleSubjects[0]
	}
	return ""
}

func outcomeSide(present bool, f *aggregate.Finding, decision aggregate.Decision) OutcomeSide {
	side := OutcomeSide{
		Present:  present,
		Decision: string(decision),
	}
	if present && f != nil {
		side.Effect = string(f.Effect)
		if f.Points != 0 {
			p := f.Points
			side.Points = &p
		}
	}
	return side
}

// MarshalJSON ensures deterministic delta ordering on every encode.
func (rec ComparisonRecord) MarshalJSON() ([]byte, error) {
	type alias ComparisonRecord
	cp := alias(rec)
	if cp.Deltas == nil {
		cp.Deltas = []Delta{}
	}
	sort.Slice(cp.Deltas, func(i, j int) bool {
		if cp.Deltas[i].Rule != cp.Deltas[j].Rule {
			return cp.Deltas[i].Rule < cp.Deltas[j].Rule
		}
		if cp.Deltas[i].Subject != cp.Deltas[j].Subject {
			return cp.Deltas[i].Subject < cp.Deltas[j].Subject
		}
		return cp.Deltas[i].Kind < cp.Deltas[j].Kind
	})
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(cp); err != nil {
		return nil, err
	}
	// Encoder adds trailing newline; trim for stable bytes in tests.
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}
