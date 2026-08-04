package policy

// profile.go is the E2-S09 Profile contract (ADR-0018 §2, frozen
// schemas/policy/v1alpha1/profile.schema.json): a named activation of packs with
// a REQUIRED writes boolean (true = holds forge write authority for its scope;
// false = recorder-only) and an environments/classes coverage scope.
//
// This file owns three self-contained concerns (no cel-go, no aggregate import —
// the policy → aggregate direction would cycle, per this package's header):
//   - the Profile Go type mirroring the frozen schema (LoadProfile decodes
//     under the frozen ProfileSchema, so writes-required / unknown-field / scope
//     rules are the schema's authority, never re-implemented);
//   - Covers, the per-profile coverage predicate over a (environment, class) binding;
//   - CoveringProfiles, which walks the Config.profiles PRECEDENCE TABLE (never a
//     map) and enforces the SINGLE-WRITER invariant at load — at most one covering
//     writes:true profile per binding; two covering writers are REJECTED (never
//     last-one-wins, ADR-0018 §2).
//
// Winner-selection among the covering profiles (specificity → config-order) lives
// in internal/core/aggregate/profile.go, which reads a covering set from here.

import (
	"fmt"

	"github.com/PlatformRelay/assent/schemas"
)

// Profile is a loaded, schema-validated named profile (ADR-0018 §2).
type Profile struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   Metadata    `yaml:"metadata"`
	Spec       ProfileSpec `yaml:"spec"`
}

// ProfileSpec carries the required writes authority flag plus the
// environments/classes coverage scope (each entry a concrete name or "*").
type ProfileSpec struct {
	// Writes is the required forge-write-authority flag: true = this profile may
	// arm/merge for bindings in its scope; false = recorder-only (never Reconcile).
	Writes bool `yaml:"writes"`
	// Environments this profile covers (Config environment names, or "*" for all).
	Environments []string `yaml:"environments"`
	// Classes this profile covers (Config class names, or "*" for all).
	Classes []string `yaml:"classes"`
	// Packs is the optional coherent activation set (not a parallel route).
	Packs []string `yaml:"packs"`
}

// LoadProfile validates raw (YAML or JSON) against the frozen Profile
// schema, then decodes it into the engine type. The schema is the single
// authority for the required writes boolean, required scope, and unknown-field
// rejection (E2-S01 discipline); no rule is re-implemented here.
func LoadProfile(raw []byte) (*Profile, error) {
	if err := validate("profile", schemas.ProfileSchema, raw); err != nil {
		return nil, err
	}
	var p Profile
	if err := decode(raw, &p); err != nil {
		return nil, fmt.Errorf("profile decode: %w", err)
	}
	return &p, nil
}

// Covers reports whether this profile's scope covers the (environment, class)
// binding: an environments entry equal to env or "*", AND a classes entry equal
// to class or "*". Both dimensions must be covered (ADR-0018 §2).
func (p *Profile) Covers(env, class string) bool {
	return scopeCovers(p.Spec.Environments, env) && scopeCovers(p.Spec.Classes, class)
}

// scopeCovers reports whether a scope list covers v — an exact name or "*".
func scopeCovers(scope []string, v string) bool {
	for _, s := range scope {
		if s == "*" || s == v {
			return true
		}
	}
	return false
}

// CoveringProfiles returns, IN PRECEDENCE-TABLE ORDER, the profiles that cover
// the (environment, class) binding, after enforcing the single-writer invariant.
//
// Participation is defined by the Config.profiles precedence table: each ProfileRef
// is resolved (by name) to a handed Profile document. A ref naming a profile
// not in the handed set is a DANGLING reference and is rejected (fail-closed — a
// missing profile is never silently skipped). A profile document not named in the
// table does not participate.
//
// Iterating the ordered table (never a map) makes resolution deterministic — the
// table decides, not Go map iteration order.
//
// SINGLE-WRITER INVARIANT (ADR-0018 §2): at most one covering profile may declare
// writes:true. Two or more covering writers are REJECTED here at load — never
// last-one-wins, never two profiles racing to arm the same MR. Zero covering
// writers is allowed (recorder-only / no-write is the safe default).
func CoveringProfiles(precedence []ProfileRef, profiles []*Profile, env, class string) ([]*Profile, error) {
	byName := make(map[string]*Profile, len(profiles))
	for _, p := range profiles {
		if p != nil {
			byName[p.Metadata.Name] = p
		}
	}

	var covering []*Profile
	writers := 0
	for _, ref := range precedence {
		p, ok := byName[ref.Name]
		if !ok {
			return nil, fmt.Errorf("profile resolution: precedence references unknown profile %q (no matching Profile document)", ref.Name)
		}
		if !p.Covers(env, class) {
			continue
		}
		covering = append(covering, p)
		if p.Spec.Writes {
			writers++
		}
	}
	if writers > 1 {
		return nil, fmt.Errorf("profile resolution: single-writer invariant violated for binding (environment=%q, class=%q): %d covering profiles declare writes:true (at most one allowed)", env, class, writers)
	}
	return covering, nil
}
