// Package policy loads assent's frozen authored contracts — MergePolicy,
// RulesetBinding, Config, and Pack (schemas/policy/v1alpha1/**) — into engine
// types under strict decode, replacing the P4-E1 toy YAML (cmd/assent/policy.go).
//
// Strict decode reuses the FROZEN JSON Schemas (the schemas package's compiled
// validators) as the single authority for unknown-field / unknown-enum /
// duplicate-id / required rejection, so those rules are never re-implemented
// here and cannot drift (E2-S01). The prove.when assertTree is decoded
// STRUCTURALLY only (bare-string | leaf | combinator); cel.Compile is deferred
// to E2-S02, so this package stays off cel-go and pure (no clock/env/net/random,
// enforced by internal/core/purity_test.go).
//
// Design constraint: this package is self-contained and does NOT import
// internal/core/aggregate — E2-S02 makes the evaluator consume these types, so a
// policy -> aggregate import now would create a cycle. The Effect/Phase/rule
// vocabulary therefore lives here.
package policy

import "fmt"

// Effect is a rule's non-obligation effect (rule.effect) or an obligation
// rule's onFailure.effect. require-review is valid only on onFailure
// (satisfied solely by forge-proven ApprovalEvidence, ADR-0017 §3).
type Effect string

// Effect values (the frozen merge-policy effect / onFailure.effect enums).
const (
	EffectComment       Effect = "comment"
	EffectChallenge     Effect = "challenge"
	EffectBlock         Effect = "block"
	EffectRequireReview Effect = "require-review"
)

// Phase is a rule/pack rollout phase (ADR-0018 §1): off = never evaluated;
// observe = evaluated, recorded, excluded from aggregation; enforce = feeds the
// decision. Required with no default (no-implicit-enforce).
type Phase string

// Phase values (the frozen off/observe/enforce rollout phases).
const (
	PhaseOff     Phase = "off"
	PhaseObserve Phase = "observe"
	PhaseEnforce Phase = "enforce"
)

// MergePolicy is a loaded, schema-validated authored pack (spec.entries + rules).
type MergePolicy struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   Metadata        `yaml:"metadata"`
	Spec       MergePolicySpec `yaml:"spec"`
}

// Metadata is the shared name-carrying header (MergePolicy, Pack).
type Metadata struct {
	Name string `yaml:"name"`
}

// MergePolicySpec holds a pack's governed-entry declarations and rules.
type MergePolicySpec struct {
	Entries map[string]Entry `yaml:"entries"`
	Rules   []Rule           `yaml:"rules"`
}

// Entry declares a governed collection: its parsing mode, the root pointer, and
// the content-derived identity pointer (ADR-0017 §5).
type Entry struct {
	Mode     string   `yaml:"mode"`
	Root     string   `yaml:"root"`
	Identity Identity `yaml:"identity"`
}

// Identity is the content-derived stable-identity pointer for an Entry.
type Identity struct {
	Pointer string `yaml:"pointer"`
}

// Rule is one authored rule. An obligation rule carries prove+onFailure; a
// non-obligation rule carries effect (the frozen schema's oneOf enforces exactly
// one shape). points is author-declared risk weight (accrues per firing, S06).
type Rule struct {
	Name      string     `yaml:"name"`
	Phase     Phase      `yaml:"phase"`
	Match     Match      `yaml:"match"`
	Prove     *Prove     `yaml:"prove"`
	OnFailure *OnFailure `yaml:"onFailure"`
	Effect    Effect     `yaml:"effect"`
	Message   string     `yaml:"message"`
	Points    int        `yaml:"points"`
}

// Match is restricted to exactly one of four ADR-0017 §5 domains. FileEvents is
// modelled so an authored use is DETECTED and rejected at load — E1 deferred
// whole-file fileEvents, so E2 supports only files/values/valueChanges.
type Match struct {
	Files        *FilesMatch        `yaml:"files"`
	Values       *ValuesMatch       `yaml:"values"`
	FileEvents   *FileEventsMatch   `yaml:"fileEvents"`
	ValueChanges *ValueChangesMatch `yaml:"valueChanges"`
}

// FilesMatch is a whole-file glob scope.
type FilesMatch struct {
	Paths []string `yaml:"paths"`
}

// ValuesMatch is a value-level modify match by JSON Pointer (implicit modify).
type ValuesMatch struct {
	Paths    []string `yaml:"paths"`
	Pointers []string `yaml:"pointers"`
}

// FileEventsMatch is the E1-deferred whole-file lifecycle domain (rejected).
type FileEventsMatch struct {
	Paths []string `yaml:"paths"`
	Kinds []string `yaml:"kinds"`
}

// ValueChangesMatch is a value-level match with explicit kind discrimination.
type ValueChangesMatch struct {
	Paths    []string `yaml:"paths"`
	Pointers []string `yaml:"pointers"`
	Kinds    []string `yaml:"kinds"`
}

// Prove is an obligation rule's proof: the obligation it contributes and the
// assert tree that, when true, proves it.
type Prove struct {
	Obligation string     `yaml:"obligation"`
	When       AssertTree `yaml:"when"`
}

// OnFailure is the effect applied when an obligation rule's When is cleanly false.
type OnFailure struct {
	Effect Effect `yaml:"effect"`
	Code   string `yaml:"code"`
}

// AssertTree is a structurally-decoded CEL condition tree (ADR-0013): exactly
// one of Leaf / All / Any / Not is set. A bare YAML/JSON string decodes to a
// Leaf shorthand. The CEL text is NOT compiled here — E2-S02 owns cel.Compile.
type AssertTree struct {
	Leaf *Leaf
	All  []AssertTree
	Any  []AssertTree
	Not  *AssertTree
}

// Leaf is a single CEL expression with an optional failure message.
type Leaf struct {
	CEL     string
	Message string
}

// yamlNode is the subset of *yaml.Node the AssertTree decoder needs, kept as an
// interface-free local type via the gopkg.in/yaml.v3 UnmarshalYAML hook below.

// RulesetBinding routes (class, environment) to packs, a risk threshold, and the
// obligations that must be proven (ADR-0008).
type RulesetBinding struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Bindings   []Binding `yaml:"bindings"`
}

// Binding is one (class, environment) routing entry.
type Binding struct {
	Class       string   `yaml:"class"`
	Environment string   `yaml:"environment"`
	Packs       []string `yaml:"packs"`
	Risk        Risk     `yaml:"risk"`
	Require     []string `yaml:"require"`
}

// Risk carries the per-binding approve threshold: sum(points) <= threshold
// approves (ADR-0007).
type Risk struct {
	Threshold int `yaml:"threshold"`
}

// Config is the repo-level wiring: environments, classes, providers, and the
// optional ordered profile precedence table.
type Config struct {
	APIVersion   string              `yaml:"apiVersion"`
	Kind         string              `yaml:"kind"`
	Environments []NamedMatch        `yaml:"environments"`
	Classes      []NamedMatch        `yaml:"classes"`
	Providers    map[string]Provider `yaml:"providers"`
	Profiles     []ProfileRef        `yaml:"profiles"`
	Presentation *Presentation       `yaml:"presentation,omitempty"`
}

// Presentation holds tier-0 renderer knobs from config (ADR-0016 §1, D-088).
type Presentation struct {
	Verbosity         string                    `yaml:"verbosity,omitempty"`
	Emoji             *bool                     `yaml:"emoji,omitempty"`
	CollapseThreshold *int                      `yaml:"collapseThreshold,omitempty"`
	Locale            string                    `yaml:"locale,omitempty"`
	Environments      []PresentationEnvOverride `yaml:"environments,omitempty"`
}

// PresentationEnvOverride overrides global presentation knobs for one environment.
type PresentationEnvOverride struct {
	Name              string `yaml:"name"`
	Verbosity         string `yaml:"verbosity,omitempty"`
	Emoji             *bool  `yaml:"emoji,omitempty"`
	CollapseThreshold *int   `yaml:"collapseThreshold,omitempty"`
	Locale            string `yaml:"locale,omitempty"`
}

// NamedMatch is a named path-glob scope (an environment or a class).
type NamedMatch struct {
	Name  string    `yaml:"name"`
	Match PathMatch `yaml:"match"`
}

// PathMatch is a bare path-glob list.
type PathMatch struct {
	Paths []string `yaml:"paths"`
}

// Provider declares a fact source and its failure posture (closed | open).
type Provider struct {
	Type    string `yaml:"type"`
	URL     string `yaml:"url"`
	Failure string `yaml:"failure"`
}

// ProfileRef is one row of the ordered profile precedence table.
type ProfileRef struct {
	Name string `yaml:"name"`
}

// Pack is a pack manifest; spec.phase is the ceiling over the pack's rules.
type Pack struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       PackSpec `yaml:"spec"`
}

// PackSpec carries the pack-level phase ceiling and version/description.
type PackSpec struct {
	Phase       Phase  `yaml:"phase"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

// fmtAssertErr is a tiny helper so the UnmarshalYAML hook (in loader.go, which
// imports yaml) keeps a shared, testable error string.
func fmtAssertErr(detail string) error {
	return fmt.Errorf("assert tree: %s (expected a bare CEL string, a {cel[,message]} leaf, or one of all/any/not)", detail)
}
