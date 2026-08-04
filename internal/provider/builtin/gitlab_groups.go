package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
)

// OutputGroups is the fact name bound as facts.<provider>.groups (example packs:
// facts.author.groups).
const OutputGroups = "groups"

// groupsMaxAge matches provider-contract.md principal / membership default (1h).
const groupsMaxAge = time.Hour

// GroupsDeclaration is the echoed output declaration for author.groups
// (principal set on a user subject — membership per provider-contract.md).
func GroupsDeclaration() provider.Declaration {
	return provider.Declaration{
		Type:        "principal",
		Cardinality: "set",
		Subject:     "user",
		Sensitive:   false,
		MaxAge:      "1h",
	}
}

// ResolveGitlabGroups resolves forge group membership into a host Result via
// ResolveFacts (schema + classifier). Hermetic tests inject FakeForgeGroups;
// live GitLab is infra-gated and out of scope for the autonomous S06 path.
//
// Unknown/missing membership → unavailable (never resolved with [] — REQ-E5-S06-02).
func ResolveGitlabGroups(ctx context.Context, client GroupsClient, q provider.FactQuery) provider.Result {
	call := CallGitlabGroups(client, q)
	return provider.ResolveFacts(ctx, call, q, q.AsOf)
}

// CallGitlabGroups returns a CallFunc that answers q from client. Captures q so
// the host can reuse the same FactQuery it built for ResolveFacts.
func CallGitlabGroups(client GroupsClient, q provider.FactQuery) provider.CallFunc {
	return func(ctx context.Context) ([]byte, error) {
		resp, err := answerGitlabGroups(ctx, client, q)
		if err != nil {
			return nil, err
		}
		return json.Marshal(resp)
	}
}

func answerGitlabGroups(ctx context.Context, client GroupsClient, q provider.FactQuery) (provider.FactResponse, error) {
	facts := make([]provider.Fact, 0, len(q.Outputs))
	for _, name := range q.Outputs {
		fact := provider.Fact{
			Name:        name,
			Declaration: GroupsDeclaration(),
			Subject:     q.Subject,
			ObservedAt:  q.AsOf,
		}
		if name != OutputGroups {
			fact.State = provider.StateInvalid
			fact.Reason = "output not declared by builtin/gitlab-groups"
			facts = append(facts, fact)
			continue
		}
		if q.Subject.Kind != "user" || strings.TrimSpace(q.Subject.ID) == "" {
			fact.State = provider.StateInvalid
			fact.Reason = "gitlab-groups requires subject.kind=user with a non-empty id"
			facts = append(facts, fact)
			continue
		}
		if client == nil {
			return provider.FactResponse{}, errors.New("gitlab-groups: GroupsClient is nil")
		}
		groups, err := client.UserGroups(ctx, q.Subject.ID)
		if errors.Is(err, ErrMembershipUnknown) {
			fact.State = provider.StateUnavailable
			fact.Reason = "forge group membership unknown for subject"
			facts = append(facts, fact)
			continue
		}
		if err != nil {
			// Transport / forge error — surface as CallFunc failure so the host
			// classifier marks every requested output unavailable.
			return provider.FactResponse{}, err
		}
		slices.Sort(groups)
		expires := q.AsOf.Add(groupsMaxAge)
		fact.State = provider.StateResolved
		fact.Value = groups
		fact.ExpiresAt = &expires
		facts = append(facts, fact)
	}
	return provider.FactResponse{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactResponse,
		QueryID:    q.QueryID,
		Facts:      facts,
	}, nil
}
