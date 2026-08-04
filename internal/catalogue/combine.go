package catalogue

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// CombinePolicies unions a pack's MergePolicy documents (the example packs author one
// rule file per obligation under .assent/packs/<pack>/rules/**, so a pack is SEVERAL
// MergePolicy docs) into the SINGLE MergePolicy whole-pack replay evaluates via
// aggregate.Cover: the union of every doc's spec.rules plus the merged spec.entries.
//
// Fail-closed on a real defect: every rule doc of a governed pack authors the SAME
// spec.entries config, so combining REQUIRES them to AGREE — two docs giving the same
// entry label DIFFERENT configs is a pack defect that errors here, never a silent pick
// of one. A nil/empty input yields a nil policy (the caller skips it).
func CombinePolicies(ps []*policy.MergePolicy) (*policy.MergePolicy, error) {
	var nonNil int
	out := &policy.MergePolicy{}
	entries := map[string]policy.Entry{}
	for _, p := range ps {
		if p == nil {
			continue
		}
		nonNil++
		for label, e := range p.Spec.Entries {
			if prev, ok := entries[label]; ok && prev != e {
				return nil, fmt.Errorf("conflicting entries config for %q across rule documents (whole-pack replay needs one collection config per label)", label)
			}
			entries[label] = e
		}
		out.Spec.Rules = append(out.Spec.Rules, p.Spec.Rules...)
	}
	if nonNil == 0 {
		return nil, nil
	}
	if len(entries) > 0 {
		out.Spec.Entries = entries
	}
	return out, nil
}
