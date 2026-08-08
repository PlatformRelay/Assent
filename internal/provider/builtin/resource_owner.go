package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
	"gopkg.in/yaml.v3"
)

// TypeResourceOwner is the Config.providers[].type string for the resource→owner
// builtin (closes REF-GAP-1 / REQ-E5-S08).
const TypeResourceOwner = "builtin/resource-owner"

// OutputOwner is the fact name bound as facts.<provider>.owner for referenced
// resource ownership lookups (C7 / ownership / require-review).
const OutputOwner = "owner"

// ownerMaxAge matches provider-contract.md string registry lookup default (24h).
const ownerMaxAge = 24 * time.Hour

// ErrResourceOwnerUnknown reports that the registry has no ownership record for
// the resource. The resource-owner builtin maps this to a non-resolved fact —
// never an empty-resolved allow (REQ-E5-S08-02).
var ErrResourceOwnerUnknown = errors.New("resource owner unknown")

// OwnerDeclaration is the echoed output declaration for resource.owner
// (string registry lookup per provider-contract.md).
func OwnerDeclaration() provider.Declaration {
	return provider.Declaration{
		Type:        "string",
		Cardinality: "single",
		Subject:     "entry",
		Sensitive:   false,
		MaxAge:      "24h",
	}
}

// ResourceOwnerClient resolves a referenced resource ID to its owning
// principal/group. Hermetic tests inject FakeResourceOwner or a map loaded
// from testdata; live HTTP registry is deferred beyond v1.
type ResourceOwnerClient interface {
	// Owner returns the owning principal/group when the resource is known.
	// Unknown/missing ownership must return ErrResourceOwnerUnknown (not an
	// empty string) so the builtin cannot empty-resolve an unresolved owner.
	Owner(ctx context.Context, resourceID string) (string, error)
}

// FakeResourceOwner is the hermetic registry client for L0/L1 (REQ-E5-S08-01).
// A present Owners key is known ownership (possibly empty); an absent key is
// unknown → ErrResourceOwnerUnknown.
type FakeResourceOwner struct {
	Owners map[string]string
	// Err, when set, is returned from every Owner call (transport failure).
	Err error
}

// Owner implements ResourceOwnerClient.
func (f *FakeResourceOwner) Owner(_ context.Context, resourceID string) (string, error) {
	if f == nil {
		return "", fmt.Errorf("FakeResourceOwner is nil")
	}
	if f.Err != nil {
		return "", f.Err
	}
	owner, ok := f.Owners[resourceID]
	if !ok {
		return "", ErrResourceOwnerUnknown
	}
	return owner, nil
}

// mapResourceOwner wraps a loaded ownership map as a ResourceOwnerClient.
type mapResourceOwner struct {
	owners map[string]string
}

func (m *mapResourceOwner) Owner(_ context.Context, resourceID string) (string, error) {
	owner, ok := m.owners[resourceID]
	if !ok {
		return "", ErrResourceOwnerUnknown
	}
	return owner, nil
}

// LoadResourceOwnerMap reads a YAML ownership registry from fsys.
// Expected shape: top-level "owners" mapping resource ID → owner string.
//
// fsys MUST be a symlink-safe root (see RepoFileOpts.FS / OpenRepoRoot, D-129):
// this registry decides who may approve, and it is read with NO roots clip, so a
// registry reached through a symlink is refused outright. Refusal is an error —
// the caller gets no client, and the owner fact then never resolves — rather than
// a partially trusted map.
func LoadResourceOwnerMap(fsys fs.FS, file string) (ResourceOwnerClient, error) {
	if classifyCandidate(fsys, file) == candidateSymlink {
		return nil, fmt.Errorf("builtin/resource-owner: refusing registry reached through a symlink: %s", file)
	}
	raw, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, fmt.Errorf("builtin/resource-owner: read %s: %w", file, err)
	}
	var doc struct {
		Owners map[string]string `yaml:"owners"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("builtin/resource-owner: decode %s: %w", file, err)
	}
	if doc.Owners == nil {
		return nil, fmt.Errorf("builtin/resource-owner: %s: owners map is required", file)
	}
	owners := make(map[string]string, len(doc.Owners))
	for k, v := range doc.Owners {
		owners[k] = v
	}
	return &mapResourceOwner{owners: owners}, nil
}

// ResolveResourceOwner resolves referenced-resource ownership into a host Result
// via ResolveFacts (schema + classifier). Unknown resource → unavailable (never
// resolved with "" — REQ-E5-S08-02).
func ResolveResourceOwner(ctx context.Context, client ResourceOwnerClient, q provider.FactQuery) provider.Result {
	call := CallResourceOwner(client, q)
	return provider.ResolveFacts(ctx, call, q, q.AsOf)
}

// CallResourceOwner returns a CallFunc that answers q from client.
func CallResourceOwner(client ResourceOwnerClient, q provider.FactQuery) provider.CallFunc {
	return func(ctx context.Context) ([]byte, error) {
		resp, err := answerResourceOwner(ctx, client, q)
		if err != nil {
			return nil, err
		}
		return json.Marshal(resp)
	}
}

func answerResourceOwner(ctx context.Context, client ResourceOwnerClient, q provider.FactQuery) (provider.FactResponse, error) {
	facts := make([]provider.Fact, 0, len(q.Outputs))
	for _, name := range q.Outputs {
		fact := provider.Fact{
			Name:        name,
			Declaration: OwnerDeclaration(),
			Subject:     q.Subject,
			ObservedAt:  q.AsOf,
		}
		if name != OutputOwner {
			fact.State = provider.StateInvalid
			fact.Reason = "output not declared by builtin/resource-owner"
			facts = append(facts, fact)
			continue
		}
		if q.Subject.Kind != "entry" || strings.TrimSpace(q.Subject.ID) == "" {
			fact.State = provider.StateInvalid
			fact.Reason = "resource-owner requires subject.kind=entry with a non-empty id"
			facts = append(facts, fact)
			continue
		}
		if client == nil {
			return provider.FactResponse{}, errors.New("resource-owner: ResourceOwnerClient is nil")
		}
		owner, err := client.Owner(ctx, q.Subject.ID)
		if errors.Is(err, ErrResourceOwnerUnknown) {
			fact.State = provider.StateUnavailable
			fact.Reason = "resource ownership unknown"
			facts = append(facts, fact)
			continue
		}
		if err != nil {
			return provider.FactResponse{}, err
		}
		expires := q.AsOf.Add(ownerMaxAge)
		fact.State = provider.StateResolved
		fact.Value = owner
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
