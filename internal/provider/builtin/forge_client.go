// Package builtin registers hermetic Phase-5 provider fact sources (forge groups, repo-file walk).
package builtin

import (
	"context"
	"errors"
	"fmt"
)

// ErrMembershipUnknown reports that the forge has no membership record for the
// subject. The gitlab-groups builtin maps this to a non-resolved fact — never
// an empty-resolved allow (REQ-E5-S06-02).
var ErrMembershipUnknown = errors.New("forge group membership unknown")

// GroupsClient is the forge seam for group-membership lookup. Production may
// wrap a live GitLab client behind an infra gate; L0/L1 tests inject FakeForgeGroups.
type GroupsClient interface {
	// UserGroups returns the subject's group names when membership is known.
	// Unknown/missing membership must return ErrMembershipUnknown (not an empty
	// slice) so the builtin cannot empty-resolve an unresolved principal.
	UserGroups(ctx context.Context, username string) ([]string, error)
}

// FakeForgeGroups is the hermetic forge client for L0/L1 (REQ-E5-S06-01).
// A present Membership key is known membership (possibly empty); an absent key
// is unknown → ErrMembershipUnknown.
type FakeForgeGroups struct {
	Membership map[string][]string
	// Err, when set, is returned from every UserGroups call (transport failure).
	Err error
}

// UserGroups implements GroupsClient.
func (f *FakeForgeGroups) UserGroups(_ context.Context, username string) ([]string, error) {
	if f == nil {
		return nil, fmt.Errorf("FakeForgeGroups is nil")
	}
	if f.Err != nil {
		return nil, f.Err
	}
	groups, ok := f.Membership[username]
	if !ok {
		return nil, ErrMembershipUnknown
	}
	out := make([]string, len(groups))
	copy(out, groups)
	return out, nil
}
