package catalogue_test

import (
	"reflect"
	"testing"

	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// loadMP loads a MergePolicy from YAML via the E2 strict loader — the catalogue
// derives entirely from loaded policy.* types, so the fixtures go through the
// same loader production uses (REQ-E3-S07-01: "derived from the loaded packs").
func loadMP(t *testing.T, raw string) *policy.MergePolicy {
	t.Helper()
	mp, err := policy.LoadMergePolicy([]byte(raw))
	if err != nil {
		t.Fatalf("load merge-policy fixture: %v", err)
	}
	return mp
}

func loadRB(t *testing.T, raw string) *policy.RulesetBinding {
	t.Helper()
	rb, err := policy.LoadRulesetBinding([]byte(raw))
	if err != nil {
		t.Fatalf("load ruleset-binding fixture: %v", err)
	}
	return rb
}

// ownershipPack is a two-rule obligation pack exercising every derived field:
// a require-review rule with a facts ref + finding code, and an observe-phase
// challenge rule.
const ownershipPack = `
apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata: { name: topics-ownership }
spec:
  rules:
    - name: author-owns-entry
      phase: enforce
      match: { files: { paths: ["topics/**/*.yaml"] } }
      prove:
        obligation: ownership
        when: "entry.owner in facts.author.groups"
      onFailure: { effect: require-review, code: ownership.unauthorized }
    - name: bounded-change
      phase: observe
      match: { values: { pointers: ["/retentionMs"] } }
      prove:
        obligation: bounded-change
        when: "facts.quota.max_partitions >= 1"
      onFailure: { effect: challenge, code: bounded.exceeded }
`

const topicsBinding = `
apiVersion: assent.dev/v1alpha1
kind: RulesetBinding
bindings:
  - { class: kafka-topic, environment: prod, packs: [topics], risk: { threshold: 4 }, require: [ownership, bounded-change] }
  - { class: kafka-topic, environment: dev, packs: [topics], risk: { threshold: 10 }, require: [ownership, bounded-change] }
`

func topicsInput(t *testing.T) catalogue.Input {
	return catalogue.Input{
		Packs:    []catalogue.Pack{{Name: "topics", Policies: []*policy.MergePolicy{loadMP(t, ownershipPack)}}},
		Bindings: []*policy.RulesetBinding{loadRB(t, topicsBinding)},
	}
}

// TestCatalogueFieldsFromLoadedPack asserts each rule is catalogued with the full
// D-017 B10 field set, derived from the loaded pack (REQ-E3-S07-01). The class
// field is the load-bearing binding-graph join: it is present ONLY because the
// pack name keying the entry matches binding.packs[].
func TestCatalogueFieldsFromLoadedPack(t *testing.T) {
	cat := catalogue.Build(topicsInput(t))

	if len(cat.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cat.Rules))
	}

	// Canonically sorted by ID: bounded-change sorts before author-... no —
	// "topics/author-owns-entry" < "topics/bounded-change".
	want := catalogue.RuleEntry{
		ID:            "topics/author-owns-entry",
		Pack:          "topics",
		Rule:          "author-owns-entry",
		DocsURL:       catalogue.DocsBase + "/topics/author-owns-entry",
		Phase:         "enforce",
		Obligation:    "ownership",
		RequiredFacts: []string{"author.groups"},
		Capabilities:  []string{catalogue.CapabilityApprovalEvidence},
		Classes:       []string{"kafka-topic"}, // deduped across the two bindings
		MatchDomains:  []string{"files"},
		FindingCodes:  []string{"ownership.unauthorized"},
		Effects:       []string{"require-review"},
		Deprecated:    false,
	}
	if got := cat.Rules[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("entry[0] mismatch:\n got  %+v\n want %+v", got, want)
	}

	// The second rule proves the facts ref + code + effect derivation on the
	// observe/challenge shape.
	second := cat.Rules[1]
	if second.ID != "topics/bounded-change" {
		t.Fatalf("entry[1] id = %q, want topics/bounded-change", second.ID)
	}
	if !reflect.DeepEqual(second.RequiredFacts, []string{"quota.max_partitions"}) {
		t.Errorf("entry[1] requiredFacts = %v", second.RequiredFacts)
	}
	if !reflect.DeepEqual(second.Effects, []string{"challenge"}) {
		t.Errorf("entry[1] effects = %v", second.Effects)
	}
	if len(second.Capabilities) != 0 {
		t.Errorf("entry[1] capabilities = %v, want none (not require-review)", second.Capabilities)
	}
}

// TestCatalogueAdditiveTolerant is THE key property (REQ-E3-S07-02): adding one
// rule extends the report — every pre-existing entry is byte-identical and its
// relative order among the pre-existing entries is unchanged, regardless of where
// the new rule's stable ID sorts (here it sorts in the MIDDLE, the adversarial
// case for an index-based consumer).
func TestCatalogueAdditiveTolerant(t *testing.T) {
	before := catalogue.Build(topicsInput(t))

	// Add a rule whose ID ("topics/blast-radius") sorts BETWEEN the two existing
	// IDs (author-owns-entry < blast-radius < bounded-change) — the hardest case:
	// an append-only consumer would be fine, an index-keyed one must not break.
	const withNew = ownershipPack + `    - name: blast-radius
      phase: enforce
      match: { files: { paths: ["topics/**"] } }
      effect: comment
      points: 1
`
	after := catalogue.Build(catalogue.Input{
		Packs:    []catalogue.Pack{{Name: "topics", Policies: []*policy.MergePolicy{loadMP(t, withNew)}}},
		Bindings: []*policy.RulesetBinding{loadRB(t, topicsBinding)},
	})

	if len(after.Rules) != len(before.Rules)+1 {
		t.Fatalf("after has %d rules, want %d", len(after.Rules), len(before.Rules)+1)
	}

	// Index the after-report by ID.
	afterByID := map[string]catalogue.RuleEntry{}
	for _, e := range after.Rules {
		afterByID[e.ID] = e
	}

	// (1) Every pre-existing entry is present and byte-identical (no field churn).
	for _, b := range before.Rules {
		a, ok := afterByID[b.ID]
		if !ok {
			t.Fatalf("pre-existing entry %q vanished after adding a rule", b.ID)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("pre-existing entry %q changed after adding a rule:\n before %+v\n after  %+v", b.ID, b, a)
		}
	}

	// (2) The subsequence of `after` filtered to the pre-existing IDs equals
	//     `before` exactly — the pre-existing entries are neither reordered nor
	//     renumbered relative to each other (additive-tolerance).
	beforeIDs := map[string]struct{}{}
	for _, b := range before.Rules {
		beforeIDs[b.ID] = struct{}{}
	}
	var subseq []catalogue.RuleEntry
	for _, a := range after.Rules {
		if _, ok := beforeIDs[a.ID]; ok {
			subseq = append(subseq, a)
		}
	}
	if !reflect.DeepEqual(subseq, before.Rules) {
		t.Fatalf("pre-existing subsequence reordered:\n before %+v\n subseq %+v", before.Rules, subseq)
	}

	// (3) The new entry is present.
	if _, ok := afterByID["topics/blast-radius"]; !ok {
		t.Fatalf("new rule topics/blast-radius not catalogued")
	}
}

// TestDeprecationMetadataSurfaced asserts a phase:off rule surfaces its
// deprecation metadata, and a phase:enforce rule does not (REQ-E3-S07-03).
func TestDeprecationMetadataSurfaced(t *testing.T) {
	const withRetired = `
apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata: { name: topics-legacy }
spec:
  rules:
    - name: legacy-check
      phase: off
      match: { files: { paths: ["topics/**"] } }
      effect: comment
    - name: live-check
      phase: enforce
      match: { files: { paths: ["topics/**"] } }
      effect: block
`
	cat := catalogue.Build(catalogue.Input{
		Packs: []catalogue.Pack{{Name: "topics", Policies: []*policy.MergePolicy{loadMP(t, withRetired)}}},
	})

	byID := map[string]catalogue.RuleEntry{}
	for _, e := range cat.Rules {
		byID[e.ID] = e
	}
	legacy := byID["topics/legacy-check"]
	if !legacy.Deprecated {
		t.Errorf("phase:off rule not marked deprecated")
	}
	if legacy.Deprecation == "" {
		t.Errorf("deprecated rule has empty deprecation reason")
	}
	live := byID["topics/live-check"]
	if live.Deprecated {
		t.Errorf("phase:enforce rule wrongly marked deprecated")
	}
	if live.Deprecation != "" {
		t.Errorf("non-deprecated rule has a deprecation reason: %q", live.Deprecation)
	}
}

// TestCatalogueDoubleRunStable asserts the serialized report is byte-identical
// across two independent builds — no clock/env/net/random, no map-iteration order
// leaking through the derived slices (REQ-E3-S07-05).
func TestCatalogueDoubleRunStable(t *testing.T) {
	a, err := catalogue.Build(topicsInput(t)).Marshal()
	if err != nil {
		t.Fatalf("marshal a: %v", err)
	}
	b, err := catalogue.Build(topicsInput(t)).Marshal()
	if err != nil {
		t.Fatalf("marshal b: %v", err)
	}
	if string(a) != string(b) {
		t.Fatalf("catalogue not byte-identical across runs:\n a=%s\n b=%s", a, b)
	}
}

// TestRequiredFactsWalkNestedAssertTree proves the facts extraction descends
// all/any/not combinators, not just the top-level leaf.
func TestRequiredFactsWalkNestedAssertTree(t *testing.T) {
	const nested = `
apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata: { name: topics-nested }
spec:
  rules:
    - name: nested-rule
      phase: enforce
      match: { files: { paths: ["topics/**"] } }
      prove:
        obligation: composite
        when:
          all:
            - "entry.owner in facts.author.groups"
            - any:
                - "facts.quota.max_partitions >= 1"
                - not: "facts.freeze.active"
      onFailure: { effect: block, code: composite.failed }
`
	cat := catalogue.Build(catalogue.Input{
		Packs: []catalogue.Pack{{Name: "topics", Policies: []*policy.MergePolicy{loadMP(t, nested)}}},
	})
	want := []string{"author.groups", "freeze.active", "quota.max_partitions"}
	if got := cat.Rules[0].RequiredFacts; !reflect.DeepEqual(got, want) {
		t.Fatalf("nested facts = %v, want %v", got, want)
	}
}
