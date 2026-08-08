package compare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/hash"
	"github.com/PlatformRelay/assent/schemas"
)

// ErrDigestMismatch is returned when supplied bundle bytes do not hash to the
// case's pinned replayBundleDigest (fail-closed before evaluation).
var ErrDigestMismatch = errors.New("compare: replay bundle digest mismatch (fail-closed)")

// SuiteCase is one immutable corpus entry from a loaded PolicyComparisonSuite.
type SuiteCase struct {
	CaseID             string
	ReplayBundleDigest string
}

// PolicyComparisonSuite is the strict-decoded in-memory suite document (PCS-S06).
type PolicyComparisonSuite struct {
	Name                string
	Version             string
	BaselineProfileRef  string
	CandidateProfileRef string
	Cases               []SuiteCase
	Spec                PolicyComparisonSuiteSpec
}

// SuiteRunResult is the aggregate output of RunSuite: per-case ComparisonRecords
// (sorted by caseId) plus gate evaluation over the full suite.
type SuiteRunResult struct {
	Records []ComparisonRecord `json:"records"`
	Gates   GateEvaluation     `json:"gates"`
}

// ReplayBundleDigest returns the domain-separated `assent-jcs-v1` digest of raw
// ReplayBundle bytes: hash.Digest(<replay-bundle schema $id>, raw), lowercase hex
// (D-121, ADR-0017 §9).
//
// A ReplayBundle is a SCHEMA-OWNED JSON DOCUMENT that consumers re-parse and
// re-verify, so its digest is over the canonical form of the document in the
// schema's hash domain — two byte-different but JCS-equivalent encodings of the
// same bundle pin the same case. This is deliberately NOT the raw `sha256:<hex>`
// used for BYTE artifacts (`pins.policySha` over policy bytes, the ADR-0019 marker
// occurrence/decision digests, `pins.toolDigest`), where byte identity is the point
// and canonicalization would be wrong. The value carries no `sha256:` tag precisely
// because it is not sha256(bytes).
//
// D-121 corrects the D-114 drift: the shipped implementation computed an undomained
// sha256(json.Marshal(decoded)). Pre-v1 migration regenerated the corpus pins in the
// same commit; D-113 corpus immutability holds because the algorithm is versioned by
// the D-121 row, not by the caseId. Pure.
func ReplayBundleDigest(raw []byte) (string, error) {
	digest, err := hash.Digest(schemas.ReplayBundleSchemaID, raw)
	if err != nil {
		return "", fmt.Errorf("replay-bundle digest: %w", err)
	}
	return digest, nil
}

// LoadSuite strict-decodes raw PolicyComparisonSuite JSON against the frozen schema
// and returns an in-memory suite. acceptedDeltas entries must reference existing
// caseIds (fail-closed). Pure.
func LoadSuite(raw []byte) (PolicyComparisonSuite, error) {
	doc, err := jsonNumberDoc(raw)
	if err != nil {
		return PolicyComparisonSuite{}, fmt.Errorf("comparison-suite: %w", err)
	}
	if err := schemas.ComparisonSuiteSchema.Validate(doc); err != nil {
		return PolicyComparisonSuite{}, fmt.Errorf("comparison-suite: %w", err)
	}

	var wire struct {
		Metadata struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"metadata"`
		Spec struct {
			BaselineProfile  string `json:"baselineProfile"`
			CandidateProfile string `json:"candidateProfile"`
			Cases            []struct {
				CaseID             string `json:"caseId"`
				ReplayBundleDigest string `json:"replayBundleDigest"`
			} `json:"cases"`
			PromotionGates []struct {
				GateID         string   `json:"gateId"`
				FailOnKinds    []string `json:"failOnKinds"`
				DefaultVerdict string   `json:"defaultVerdict"`
				Acceptance     string   `json:"acceptance"`
			} `json:"promotionGates"`
			AcceptedDeltas []struct {
				CaseID     string `json:"caseId"`
				Kind       string `json:"kind"`
				Rule       string `json:"rule"`
				Subject    string `json:"subject"`
				Obligation string `json:"obligation"`
				Rationale  string `json:"rationale"`
			} `json:"acceptedDeltas"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return PolicyComparisonSuite{}, fmt.Errorf("comparison-suite decode: %w", err)
	}

	caseIDs := make(map[string]struct{}, len(wire.Spec.Cases))
	cases := make([]SuiteCase, 0, len(wire.Spec.Cases))
	for _, c := range wire.Spec.Cases {
		caseIDs[c.CaseID] = struct{}{}
		cases = append(cases, SuiteCase{CaseID: c.CaseID, ReplayBundleDigest: c.ReplayBundleDigest})
	}

	accepted := make([]AcceptedDelta, 0, len(wire.Spec.AcceptedDeltas))
	for _, ad := range wire.Spec.AcceptedDeltas {
		if _, ok := caseIDs[ad.CaseID]; !ok {
			return PolicyComparisonSuite{}, fmt.Errorf("comparison-suite: acceptedDeltas references unknown caseId %q (fail-closed)", ad.CaseID)
		}
		accepted = append(accepted, AcceptedDelta{
			CaseID:     ad.CaseID,
			Kind:       Kind(ad.Kind),
			Rule:       ad.Rule,
			Subject:    ad.Subject,
			Obligation: ad.Obligation,
			Rationale:  ad.Rationale,
		})
	}

	gates := make([]PromotionGate, 0, len(wire.Spec.PromotionGates))
	for _, g := range wire.Spec.PromotionGates {
		kinds := make([]Kind, 0, len(g.FailOnKinds))
		for _, k := range g.FailOnKinds {
			kinds = append(kinds, Kind(k))
		}
		gates = append(gates, PromotionGate{
			GateID:         GateID(g.GateID),
			FailOnKinds:    kinds,
			DefaultVerdict: g.DefaultVerdict,
			Acceptance:     g.Acceptance,
		})
	}

	return PolicyComparisonSuite{
		Name:                wire.Metadata.Name,
		Version:             wire.Metadata.Version,
		BaselineProfileRef:  wire.Spec.BaselineProfile,
		CandidateProfileRef: wire.Spec.CandidateProfile,
		Cases:               cases,
		Spec: PolicyComparisonSuiteSpec{
			PromotionGates: gates,
			AcceptedDeltas: accepted,
		},
	}, nil
}

// RunSuite evaluates every suite case under baseline and candidate profiles,
// verifies replayBundleDigest for each supplied bundle, builds ComparisonRecords,
// and evaluates promotion gates. cases must contain an entry for every suite
// caseId; extra entries are ignored. Pure (no filesystem I/O).
func RunSuite(suite PolicyComparisonSuite, cases map[string][]byte, baseline, candidate Profile) (SuiteRunResult, error) {
	records := make([]ComparisonRecord, 0, len(suite.Cases))
	for _, sc := range suite.Cases {
		raw, ok := cases[sc.CaseID]
		if !ok {
			return SuiteRunResult{}, fmt.Errorf("compare: missing bundle bytes for caseId %q (fail-closed)", sc.CaseID)
		}
		digest, err := ReplayBundleDigest(raw)
		if err != nil {
			return SuiteRunResult{}, fmt.Errorf("compare: case %q: %w", sc.CaseID, err)
		}
		if digest != sc.ReplayBundleDigest {
			return SuiteRunResult{}, fmt.Errorf("compare: case %q: %w (want %s, got %s)", sc.CaseID, ErrDigestMismatch, sc.ReplayBundleDigest, digest)
		}
		in, err := LoadBundle(raw)
		if err != nil {
			return SuiteRunResult{}, fmt.Errorf("compare: case %q: %w", sc.CaseID, err)
		}
		baseRes, err := evaluate(in, baseline)
		if err != nil {
			return SuiteRunResult{}, fmt.Errorf("compare: case %q baseline %q: %w", sc.CaseID, baseline.Name, err)
		}
		candRes, err := evaluate(in, candidate)
		if err != nil {
			return SuiteRunResult{}, fmt.Errorf("compare: case %q candidate %q: %w", sc.CaseID, candidate.Name, err)
		}
		rec, err := BuildComparisonRecord(sc.CaseID, baseline.Name, candidate.Name, baseRes, candRes, bundleSubjects(in))
		if err != nil {
			return SuiteRunResult{}, fmt.Errorf("compare: case %q: %w", sc.CaseID, err)
		}
		records = append(records, rec)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].CaseID < records[j].CaseID
	})

	gates := EvaluateGates(suite.Spec, records)
	return SuiteRunResult{Records: records, Gates: gates}, nil
}

func bundleSubjects(in *aggregate.EvaluationInput) []string {
	if in == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, ch := range in.ChangeSet.Changes {
		if ch.Subject == "" {
			continue
		}
		seen[ch.Subject] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// MarshalJSON emits deterministic suite-run bytes (sorted records; stable gate map).
func (r SuiteRunResult) MarshalJSON() ([]byte, error) {
	type gateRow struct {
		GateID  GateID  `json:"gateId"`
		Verdict Verdict `json:"verdict"`
	}
	rows := make([]gateRow, 0, len(r.Gates.Results))
	for id, v := range r.Gates.Results {
		rows = append(rows, gateRow{GateID: id, Verdict: v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].GateID < rows[j].GateID })

	recs := r.Records
	if recs == nil {
		recs = []ComparisonRecord{}
	}
	wire := struct {
		Records []ComparisonRecord `json:"records"`
		Gates   struct {
			Results      []gateRow `json:"results"`
			FirstFailure GateID    `json:"firstFailure"`
		} `json:"gates"`
	}{
		Records: recs,
		Gates: struct {
			Results      []gateRow `json:"results"`
			FirstFailure GateID    `json:"firstFailure"`
		}{
			Results:      rows,
			FirstFailure: r.Gates.FirstFailure,
		},
	}
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(wire); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}
