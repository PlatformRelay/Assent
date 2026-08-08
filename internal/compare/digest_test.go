package compare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/hash"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/schemas"
)

// REQ-AUD-S16-01 / D-121: the replay-bundle digest is a digest over a SCHEMA-OWNED
// JSON DOCUMENT that consumers re-parse and re-verify, so it is the domain-separated
// `assent-jcs-v1` digest (internal/core/hash, ADR-0017 §9) keyed on the replay-bundle
// schema `$id` — NOT the undomained `sha256(json.Marshal(decoded))` that D-114 shipped.
//
// The corpus constants below are literals on purpose. A test that recomputes the
// expectation with the same expression the production code uses is self-consistent
// and cannot fail; these vectors move only if the algorithm really changes.
const (
	// corpusGrowAgreeBundle is the committed, immutable (D-113) corpus bundle whose
	// caseId is `partition-grow-agree`.
	corpusGrowAgreeBundle = "../../examples/comparison/promotion-gates/cases/partition-grow-agree/bundle.json"

	// corpusGrowAgreeDigestLegacy is the PRE-MIGRATION pin that used to sit in
	// examples/comparison/promotion-gates/suite.yaml — undomained
	// "sha256:" + sha256(json.Marshal(decoded)). It must now fail closed.
	corpusGrowAgreeDigestLegacy = "sha256:c1223668c50254d873361347ea6e30deec8baf75c010c855a9cc697a44011000"

	// corpusGrowAgreeDigestJCS is the POST-MIGRATION pin: the vector lock for
	// hash.Digest(schemas.ReplayBundleSchemaID, bundleBytes).
	corpusGrowAgreeDigestJCS = "7243562c4614e4bebe14f10db4d29ba64765ef1041a5c820518be852d0a0aef5"

	// replayBundleHashDomain is the ADR-0017 §9 domain, spelled out literally so a
	// silent edit of schemas.ReplayBundleSchemaID cannot slip past unnoticed.
	replayBundleHashDomain = "https://assent.dev/schemas/decision/v1alpha1/replay-bundle.schema.json"
)

func readCorpusBundle(t *testing.T, rel string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(rel)) //nolint:gosec // fixed, committed corpus path
	if err != nil {
		t.Fatalf("read corpus bundle %s: %v", rel, err)
	}
	return raw
}

// legacyUndomainedDigest reproduces the D-114 implementation that D-121 corrects.
// It exists so the tests can assert the two algorithms genuinely disagree.
func legacyUndomainedDigest(t *testing.T, raw []byte) string {
	t.Helper()
	doc, err := jsonNumberDoc(raw)
	if err != nil {
		t.Fatalf("legacy digest decode: %v", err)
	}
	canon, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("legacy digest marshal: %v", err)
	}
	sum := sha256.Sum256(canon)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// REQ-AUD-S16-01: the digest IS hash.Digest(<replay-bundle schema $id>, raw), and the
// domain is the schema $id itself.
func TestReplayBundleDigestIsDomainSeparated(t *testing.T) {
	if schemas.ReplayBundleSchemaID != replayBundleHashDomain {
		t.Fatalf("schemas.ReplayBundleSchemaID = %q, want the replay-bundle $id %q (D-121 domain)",
			schemas.ReplayBundleSchemaID, replayBundleHashDomain)
	}

	raw := readCorpusBundle(t, corpusGrowAgreeBundle)

	got, err := ReplayBundleDigest(raw)
	if err != nil {
		t.Fatalf("ReplayBundleDigest: %v", err)
	}
	want, err := hash.Digest(replayBundleHashDomain, raw)
	if err != nil {
		t.Fatalf("hash.Digest: %v", err)
	}
	if got != want {
		t.Fatalf("ReplayBundleDigest = %q, want hash.Digest(%q, raw) = %q (D-121)",
			got, replayBundleHashDomain, want)
	}
}

// REQ-AUD-S16-01 (vector lock): the committed corpus bundle hashes to a pinned value.
// This is the constant a real algorithm change has to break.
func TestReplayBundleDigestVectorLock(t *testing.T) {
	raw := readCorpusBundle(t, corpusGrowAgreeBundle)

	got, err := ReplayBundleDigest(raw)
	if err != nil {
		t.Fatalf("ReplayBundleDigest: %v", err)
	}
	if got != corpusGrowAgreeDigestJCS {
		t.Fatalf("ReplayBundleDigest(partition-grow-agree) = %q, want pinned vector %q",
			got, corpusGrowAgreeDigestJCS)
	}
	if strings.HasPrefix(got, "sha256:") {
		t.Fatalf("digest %q must not claim the raw-sha256 algorithm tag: it is assent-jcs-v1 over "+
			"canonical JSON in a schema domain, not sha256(bytes) (D-121 byte-vs-document split)", got)
	}
	if len(got) != 64 {
		t.Fatalf("digest %q: want 64 lowercase hex chars", got)
	}
	if strings.ToLower(got) != got {
		t.Fatalf("digest %q must be lowercase hex", got)
	}
}

// REQ-AUD-S16-01: the new digest genuinely differs from the D-114 undomained one —
// otherwise every other assertion in this file would be satisfiable by the old code.
func TestReplayBundleDigestDiffersFromUndomainedSha256(t *testing.T) {
	raw := readCorpusBundle(t, corpusGrowAgreeBundle)

	legacy := legacyUndomainedDigest(t, raw)
	if legacy != corpusGrowAgreeDigestLegacy {
		t.Fatalf("legacy reproduction = %q, want the pre-migration corpus pin %q "+
			"(this test's premise is broken, not the production code)", legacy, corpusGrowAgreeDigestLegacy)
	}

	got, err := ReplayBundleDigest(raw)
	if err != nil {
		t.Fatalf("ReplayBundleDigest: %v", err)
	}
	if got == legacy || "sha256:"+got == legacy {
		t.Fatalf("ReplayBundleDigest still returns the undomained D-114 digest %q — D-121 not applied", got)
	}
}

// REQ-AUD-S16-01: canonicalization is really in the path — two byte-different but
// JCS-equivalent encodings of the same document share a digest. This is the DOCUMENT
// half of the D-121 split: a raw sha256(bytes) digest could not satisfy it. (It does
// not by itself distinguish D-121 from the D-114 predecessor, which also canonicalized
// key order — TestReplayBundleDigestDiffersFromUndomainedSha256 is what pins that.)
func TestReplayBundleDigestCanonicalizesEquivalentEncodings(t *testing.T) {
	raw := readCorpusBundle(t, corpusGrowAgreeBundle)

	// Re-encode with different whitespace/key order by round-tripping through a map.
	var doc map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	reencoded, err := json.MarshalIndent(doc, "  ", "    ")
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if string(reencoded) == string(raw) {
		t.Fatal("fixture cannot discriminate: re-encoding produced identical bytes")
	}

	a, err := ReplayBundleDigest(raw)
	if err != nil {
		t.Fatalf("ReplayBundleDigest(raw): %v", err)
	}
	b, err := ReplayBundleDigest(reencoded)
	if err != nil {
		t.Fatalf("ReplayBundleDigest(reencoded): %v", err)
	}
	if a != b {
		t.Fatalf("JCS-equivalent encodings digest differently: %q vs %q", a, b)
	}
}

// digestSuiteJSON builds a one-case PolicyComparisonSuite pinning digest for the
// corpus `partition-grow-agree` bundle.
func digestSuiteJSON(digest string) []byte {
	return []byte(`{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "PolicyComparisonSuite",
  "metadata": {"name": "aud-s16-digest", "version": "1"},
  "spec": {
    "cases": [{"caseId": "partition-grow-agree", "replayBundleDigest": "` + digest + `"}],
    "promotionGates": [
      {"gateId": "zero-missed-destructive", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "zero-missed-authorization-ownership", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "no-unexpected-obligation-removal", "failOnKinds": ["subject-or-obligation-uncovered"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "bounded-auto-merge-widening", "failOnKinds": ["newly-auto-mergeable"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "explicitly-accepted-deltas", "failOnKinds": ["stricter-intervention-added", "score-threshold-change"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"}
    ]
  }
}`)
}

// REQ-AUD-S16-01 (both polarities): a suite pinned with the NEW digest runs; a suite
// pinned with the OLD-FORMAT (undomained) digest fails closed with ErrDigestMismatch.
// Both pins are hardcoded corpus literals, so neither branch can drift into agreement
// with whatever the production code happens to compute.
func TestReplayBundleDigestSuiteFailsClosedOnOldFormat(t *testing.T) {
	raw := readCorpusBundle(t, corpusGrowAgreeBundle)
	bundles := map[string][]byte{"partition-grow-agree": raw}
	baseline := mkProfile("prod@6", "new >= old", "must not shrink", policy.EffectBlock)
	candidate := mkProfile("prod@7", "new >= old", "must not shrink", policy.EffectBlock)

	t.Run("accepted/domain-separated", func(t *testing.T) {
		suite, err := LoadSuite(digestSuiteJSON(corpusGrowAgreeDigestJCS))
		if err != nil {
			t.Fatalf("LoadSuite: %v", err)
		}
		if _, err := RunSuite(suite, bundles, baseline, candidate); err != nil {
			t.Fatalf("RunSuite with the D-121 digest must succeed, got %v", err)
		}
	})

	t.Run("rejected/old-format-undomained", func(t *testing.T) {
		suite, err := LoadSuite(digestSuiteJSON(corpusGrowAgreeDigestLegacy))
		if err != nil {
			t.Fatalf("LoadSuite: %v", err)
		}
		_, err = RunSuite(suite, bundles, baseline, candidate)
		if err == nil {
			t.Fatal("RunSuite accepted the pre-D-121 undomained digest — must fail closed")
		}
		if !errors.Is(err, ErrDigestMismatch) {
			t.Fatalf("error = %v, want ErrDigestMismatch", err)
		}
	})
}

// REQ-AUD-S16-01: every committed corpus pin is the domain-separated digest of its
// own bundle bytes — the migration is complete, not partial.
func TestReplayBundleDigestCorpusPinsAreMigrated(t *testing.T) {
	corpus := map[string][]string{
		"promotion-gates": {
			"partition-grow-agree",
			"partition-shrink-widen-accepted",
			"named-consumer-compat-widen-accepted",
			"score-threshold-change-accepted",
		},
		"wording-only": {"partition-shrink-wording-only"},
	}
	checked := 0
	for suite, caseIDs := range corpus {
		suiteRaw, err := os.ReadFile(filepath.Clean(filepath.Join("../../examples/comparison", suite, "suite.yaml"))) //nolint:gosec // fixed corpus path
		if err != nil {
			t.Fatalf("read suite %s: %v", suite, err)
		}
		for _, caseID := range caseIDs {
			bundle := readCorpusBundle(t, filepath.Join("../../examples/comparison", suite, "cases", caseID, "bundle.json"))
			want, err := ReplayBundleDigest(bundle)
			if err != nil {
				t.Fatalf("%s/%s: ReplayBundleDigest: %v", suite, caseID, err)
			}
			if !strings.Contains(string(suiteRaw), "replayBundleDigest: "+want) {
				t.Errorf("%s/suite.yaml does not pin %s with the D-121 digest %q", suite, caseID, want)
			}
			if strings.Contains(string(suiteRaw), "replayBundleDigest: sha256:") {
				t.Errorf("%s/suite.yaml still carries an undomained sha256: pin (D-121 migration incomplete)", suite)
			}
			checked++
		}
	}
	if checked != 5 {
		t.Fatalf("checked %d corpus pins, want 5", checked)
	}
}
