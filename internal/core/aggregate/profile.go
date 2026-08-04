package aggregate

// profile.go is the E2-S09 profile-resolution + write-authority surfacing
// (ADR-0018 §2). It selects the SINGLE covering profile for a (environment,
// class) binding and stamps its identity + writes flag onto the decision Result.
//
// Split of concerns (the policy → aggregate import direction is one-way, so the
// self-contained pieces live in policy and are consumed here):
//   - policy.CoveringProfiles walks the Config.profiles PRECEDENCE TABLE, resolves
//     coverage, and enforces the SINGLE-WRITER invariant (two covering writes:true
//     profiles ⇒ rejected at load). Determinism comes from iterating that ordered
//     table, never a map.
//   - ResolveProfile (here) picks the winner among the covering set by SPECIFICITY
//     (narrower scope wins) → config-order tie-break, and reports the resolved
//     identity + writes.
//   - WithProfile stamps the resolution onto a Result: write-authority is the SAFE
//     value (false) unless a single covering writes:true profile resolved — a
//     recorder-only (writes:false) profile surfaces its identity but NEVER claims
//     write authority, and an uncovered binding defaults to no authority.

import "github.com/PlatformRelay/assent/internal/core/policy"

// ResolvedProfile is the outcome of profile resolution for one binding: the
// resolved profile's identity and whether it holds forge write authority.
type ResolvedProfile struct {
	// Name is the resolved profile's metadata.name (empty when none resolved).
	Name string
	// Writes is the resolved profile's spec.writes — true only for a covering
	// writes:true profile; a recorder-only profile is false.
	Writes bool
}

// ResolveProfile resolves the single covering profile for the (environment,
// class) binding from the Config.profiles precedence table and the handed profile
// documents. It returns the resolution, whether any profile covered the binding,
// and an error only when the config is invalid (single-writer violation or a
// dangling precedence ref — both fail-closed at load, never a silent pick).
//
// Resolution order (ADR-0018 §2): coverage → SPECIFICITY (narrower scope wins) →
// config-order tie-break. The covering set arrives IN PRECEDENCE ORDER from
// policy.CoveringProfiles, so a true specificity tie is broken by keeping the
// FIRST (earlier-in-table) covering profile — deterministic and driven by the
// precedence table, not input-slice or map iteration order.
func ResolveProfile(precedence []policy.ProfileRef, profiles []*policy.Profile, env, class string) (ResolvedProfile, bool, error) {
	covering, err := policy.CoveringProfiles(precedence, profiles, env, class)
	if err != nil {
		return ResolvedProfile{}, false, err
	}
	if len(covering) == 0 {
		return ResolvedProfile{}, false, nil
	}

	// covering is in precedence order; keep the first, replace only on STRICTLY
	// greater specificity so an equal-specificity later row never displaces an
	// earlier one (config-order tie-break).
	winner := covering[0]
	best := specificity(winner)
	for _, p := range covering[1:] {
		if s := specificity(p); s > best {
			winner, best = p, s
		}
	}
	return ResolvedProfile{Name: winner.Metadata.Name, Writes: winner.Spec.Writes}, true, nil
}

// specificity scores how NARROW a covering profile's scope is: +1 for each
// dimension (environments, classes) that is a concrete match rather than a "*"
// wildcard. A higher score is narrower and wins. This is only meaningful for a
// COVERING profile (guaranteed by the caller): a covering profile matches a
// dimension either concretely (narrower, +1) or via "*" (broader, +0).
func specificity(p *policy.Profile) int {
	s := 0
	if !hasWildcard(p.Spec.Environments) {
		s++
	}
	if !hasWildcard(p.Spec.Classes) {
		s++
	}
	return s
}

// hasWildcard reports whether a scope list contains the "*" all-match entry.
func hasWildcard(scope []string) bool {
	for _, s := range scope {
		if s == "*" {
			return true
		}
	}
	return false
}

// WithProfile stamps a resolution onto the Result: the resolved profile identity
// and its write authority. Write authority is the SAFE default (false) unless a
// single covering writes:true profile resolved — so a recorder-only profile
// surfaces its identity but never sets WriteAllowed, and an unresolved binding
// leaves both zero (recorder-only / no-write default). A downstream forge step
// reads WriteAllowed to know whether this run may arm/merge or is recorder-only.
func (r Result) WithProfile(rp ResolvedProfile, resolved bool) Result {
	if resolved {
		r.Profile = rp.Name
		r.WriteAllowed = rp.Writes
	}
	return r
}

// CoverWithProfile is the E2-S09 profile-aware decision entry: it resolves the
// covering profile for the binding's (environment, class) BEFORE producing the
// decision, so a single-writer violation or a dangling precedence ref fails the
// whole run closed (no Result is returned), then evaluates the coverage loop and
// stamps the resolved write-authority + identity onto the Result. Profile
// resolution never alters the decision or the finding set — it only surfaces
// whether this run may write. A caller with no profiles passes an empty precedence
// table (⇒ no covering profile ⇒ no write authority, the safe default).
func CoverWithProfile(pol *policy.MergePolicy, bind *policy.Binding, in *EvaluationInput, appr *ApprovalContext, ceiling policy.Phase, precedence []policy.ProfileRef, profiles []*policy.Profile) (Result, error) {
	rp, resolved, err := ResolveProfile(precedence, profiles, bind.Environment, bind.Class)
	if err != nil {
		return Result{}, err
	}
	res, err := cover(pol, bind, in, appr, ceiling)
	if err != nil {
		return res, err
	}
	return res.WithProfile(rp, resolved), nil
}
