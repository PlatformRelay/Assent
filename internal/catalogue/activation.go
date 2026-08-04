package catalogue

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// MergePolicyForProfile unions Profile.spec.packs[] over an already-loaded catalogue
// Input — pure, no filesystem I/O. Each named pack is combined via CombinePolicies
// first (multi-doc packs), then the per-pack policies are unioned into one activation
// the decision engine evaluates. Unknown pack names fail closed with no partial policy
// (D-112 / PCS-S01).
func MergePolicyForProfile(profile *policy.Profile, in Input) (*policy.MergePolicy, error) {
	if profile == nil {
		return nil, fmt.Errorf("merge policy for profile: nil profile")
	}
	byName := indexPacks(in.Packs)
	var packPolicies []*policy.MergePolicy
	for _, name := range profile.Spec.Packs {
		pk, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("profile %q references unknown pack %q (not present in loaded catalogue)", profile.Metadata.Name, name)
		}
		combined, err := CombinePolicies(pk.Policies)
		if err != nil {
			return nil, fmt.Errorf("profile %q pack %q: %w", profile.Metadata.Name, name, err)
		}
		if combined != nil {
			packPolicies = append(packPolicies, combined)
		}
	}
	return CombinePolicies(packPolicies)
}

// PhaseCeilingForProfile returns the strictest (lowest-rank) phase ceiling across
// every pack the profile activates; absent manifests mean enforce (no cap).
func PhaseCeilingForProfile(profile *policy.Profile, in Input) policy.Phase {
	if profile == nil {
		return policy.PhaseEnforce
	}
	byName := indexPacks(in.Packs)
	ceiling := policy.PhaseEnforce
	for _, name := range profile.Spec.Packs {
		pk, ok := byName[name]
		if !ok {
			continue
		}
		ceiling = minPhaseCeiling(ceiling, packManifestCeiling(pk.Manifest))
	}
	return ceiling
}

func indexPacks(packs []Pack) map[string]Pack {
	out := make(map[string]Pack, len(packs))
	for _, pk := range packs {
		out[pk.Name] = pk
	}
	return out
}

func packManifestCeiling(m *policy.Pack) policy.Phase {
	if m == nil {
		return policy.PhaseEnforce
	}
	return m.Spec.Phase
}

func minPhaseCeiling(a, b policy.Phase) policy.Phase {
	if phaseRank(b) < phaseRank(a) {
		return b
	}
	return a
}
