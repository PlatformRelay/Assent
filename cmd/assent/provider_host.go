package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"
	"time"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/provider"
	"github.com/PlatformRelay/assent/internal/provider/builtin"
)

// defaultProviderTimeout bounds one HTTP/exec ResolveFacts call on the live path.
const defaultProviderTimeout = 5 * time.Second

// resolveRunFacts resolves configured providers at the cmd/assent edge into
// EvaluationInput.Facts and pins.factsResolvedAt (REQ-E5-S05-01).
//
// Compatibility (REQ-E5-S05-02): nil/empty providers → empty maps (today's path).
// Builtins (repo-file, resource-owner, forge-groups) resolve when the host
// declaration + checkout/target inputs are present (E5-S10). Host declarations
// load from the TARGET ref beside Config (D-065): <dir(config)>/providers/<name>.json.
//
// AutoMergeEligible is negotiation-scoped only (INBOX P2 / E5-S01): it is NEVER
// consulted for arming. Fact envelope states remain authoritative for CEL;
// ArmEligible stays --arm ∧ APPROVE in buildDesired.
func resolveRunFacts(
	ctx context.Context,
	conf *policy.Config,
	configPath string,
	client forgePort,
	project, targetRef string,
	checkoutRoot string,
	subject string,
	now time.Time,
) (map[string]map[string]aggregate.Fact, map[string]string, error) {
	facts := map[string]map[string]aggregate.Fact{}
	resolvedAt := map[string]string{}
	if conf == nil || len(conf.Providers) == 0 {
		return facts, resolvedAt, nil
	}

	names := make([]string, 0, len(conf.Providers))
	for name := range conf.Providers {
		names = append(names, name)
	}
	sort.Strings(names)

	declDir := path.Join(path.Dir(configPath), "providers")
	asOf := now.UTC()
	anchor := anchorFromSubject(subject)
	repoFS, repoRoot, err := checkoutFS(checkoutRoot)
	if err != nil {
		// A --checkout that cannot be opened as a containment root is an operator
		// error, and silently degrading to "no facts" would hide it. Fail loudly:
		// no decision is emitted, so nothing can be armed off a half-read tree.
		return nil, nil, err
	}
	if repoRoot != nil {
		// Every builtin read happens inside this loop; nothing captures repoFS
		// beyond it (providerCallFor's closures are invoked by ResolveFactsChecked
		// in-loop, and the resource-owner registry is read eagerly).
		defer func() { _ = repoRoot.Close() }()
	}

	for _, name := range names {
		p := conf.Providers[name]

		declPath := path.Join(declDir, name+".json")
		raw, err := client.FileAtRef(project, declPath, targetRef)
		if err != nil {
			// Missing host declaration → skip (cannot know outputs; inventing
			// unavailable keys would change CEL from "absent" to "false").
			continue
		}
		hostCfg, err := provider.LoadProviderConfig(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("provider %q declaration %q: %w", name, declPath, err)
		}

		outputs := make([]string, 0, len(hostCfg.Outputs))
		for outName := range hostCfg.Outputs {
			outputs = append(outputs, outName)
		}
		sort.Strings(outputs)

		q := provider.BuildQuery(
			hostCfg,
			"run-"+name,
			asOf,
			querySubject(p, hostCfg, anchor, project),
			outputs,
			nil, // no change projections on the run wire path (minimization still applies)
		)

		call, err := providerCallFor(ctx, p, hostCfg, q, client, project, targetRef, repoFS, anchor)
		if err != nil {
			return nil, nil, fmt.Errorf("provider %q: %w", name, err)
		}
		if call == nil {
			continue
		}

		// INBOX P2: Result.AutoMergeEligible() is intentionally unread — negotiation
		// accept ≠ "facts OK to arm". Bound fact states drive CEL; arming stays
		// --arm ∧ APPROVE in buildDesired.
		result := provider.ResolveFactsChecked(ctx, call, q, asOf, hostCfg.Outputs)
		bound := make(map[string]aggregate.Fact, len(result.Facts))
		for outName, fact := range result.Facts {
			bound[outName] = provider.ToAggregateFact(fact)
		}
		facts[name] = bound
		resolvedAt[name] = asOf.Format(time.RFC3339)
	}
	return facts, resolvedAt, nil
}

func anchorFromSubject(subject string) string {
	s := strings.TrimSpace(subject)
	s = strings.TrimPrefix(s, "file:")
	return strings.TrimPrefix(s, "/")
}

// checkoutFS opens the checkout tree builtins read facts from, as a SYMLINK-SAFE
// root (D-129). The returned io.Closer is nil when there is no checkout.
//
// This tree is the MERGE-REQUEST HEAD — content under judgment, authored by the
// contributor. `os.DirFS` used to be handed out here, and it is documented in Go
// as not a security boundary: an MR could ship a symlink and read an arbitrary
// host file into a decision fact. `(*os.Root).FS()` refuses every traversal that
// leaves the root at the syscall level, for every consumer of this FS.
func checkoutFS(checkoutRoot string) (fs.FS, io.Closer, error) {
	root := strings.TrimSpace(checkoutRoot)
	if root == "" {
		return nil, nil, nil
	}
	// E1-S08 checkout layout: head/ is the MR source view; repo-file facts for
	// live runs read the checkout head tree (hermetic exit-gate fixtures mirror
	// in-repo quota/placement files beside the governed change).
	head := filepath.Join(root, "head")
	if st, err := os.Stat(head); err == nil && st.IsDir() {
		return builtin.OpenRepoRoot(head)
	}
	return builtin.OpenRepoRoot(root)
}

func querySubject(p policy.Provider, hostCfg provider.Config, anchor, project string) provider.Subject {
	switch {
	case builtin.IsResourceOwnerType(p.Type):
		id := anchor
		if hostCfg.ResourceOwner != nil && strings.TrimSpace(hostCfg.ResourceOwner.Registry) != "" {
			// Live path uses entry subject from the governed anchor when the
			// registry lookup is keyed by entry identity (C7 referenced-resource).
			id = strings.TrimSpace(anchor)
		}
		return provider.Subject{Kind: "entry", ID: id}
	case builtin.IsRepoFileType(p.Type):
		return provider.Subject{Kind: "repo", ID: project}
	default:
		return provider.Subject{Kind: "repo", ID: project}
	}
}

// providerCallFor builds the transport CallFunc for one configured provider,
// capturing the FactQuery the host built (projection-minimized).
func providerCallFor(
	ctx context.Context,
	p policy.Provider,
	hostCfg provider.Config,
	q provider.FactQuery,
	client forgePort,
	project, targetRef string,
	repoFS fs.FS,
	anchor string,
) (provider.CallFunc, error) {
	switch {
	case p.Type == "http" && strings.TrimSpace(p.URL) != "":
		url := strings.TrimSpace(p.URL)
		return func(callCtx context.Context) ([]byte, error) {
			return provider.CallHTTP(callCtx, url, q, defaultProviderTimeout)
		}, nil
	case p.Type == "exec":
		if hostCfg.Exec == nil {
			return nil, fmt.Errorf("exec provider requires host declaration exec.binary + exec.digest")
		}
		opts := provider.ExecOpts{
			Binary:  hostCfg.Exec.Binary,
			Digest:  hostCfg.Exec.Digest,
			Env:     hostCfg.Exec.Env,
			Args:    hostCfg.Exec.Args,
			Timeout: defaultProviderTimeout,
		}
		return func(callCtx context.Context) ([]byte, error) {
			return provider.CallExec(callCtx, opts, q)
		}, nil
	case builtin.IsRepoFileType(p.Type):
		if hostCfg.RepoFile == nil || strings.TrimSpace(hostCfg.RepoFile.File) == "" {
			return nil, fmt.Errorf("builtin/repo-file requires host declaration repoFile.file")
		}
		if repoFS == nil {
			// No checkout — cannot walk in-repo files; skip (pre-S10 compat).
			return nil, nil
		}
		opts := builtin.RepoFileOpts{
			FS:           repoFS,
			File:         hostCfg.RepoFile.File,
			Anchor:       anchor,
			Roots:        hostCfg.RepoFile.Roots,
			Declarations: hostCfg.Outputs,
		}
		return func(callCtx context.Context) ([]byte, error) {
			return builtin.CallRepoFile(callCtx, opts, q)
		}, nil
	case builtin.IsResourceOwnerType(p.Type):
		if hostCfg.ResourceOwner == nil || strings.TrimSpace(hostCfg.ResourceOwner.Registry) == "" {
			return nil, fmt.Errorf("builtin/resource-owner requires host declaration resourceOwner.registry")
		}
		regPath := strings.TrimSpace(hostCfg.ResourceOwner.Registry)
		ownerClient, err := loadResourceOwnerRegistry(ctx, client, project, targetRef, repoFS, regPath)
		if err != nil {
			return nil, err
		}
		return builtin.CallResourceOwner(ownerClient, q), nil
	case builtin.IsForgeGroupsType(p.Type):
		// Live forge-groups is infra-gated (E5-S06); hermetic run path skips until
		// a GroupsClient is wired from forge MR metadata.
		return nil, nil
	default:
		return nil, nil
	}
}

// refFilePort is the slice of the forge this function needs: one read at a ref.
type refFilePort interface {
	FileAtRef(project, path, ref string) ([]byte, error)
}

// loadResourceOwnerRegistry loads the resource→owner registry.
//
// D-130 / GUIDELINES §Safety 3: the registry decides WHO MAY APPROVE, so it is a
// decision input and loads from the TARGET ref FIRST. It used to prefer repoFS —
// which under `--checkout` is the merge request's own head tree — letting an MR
// ship a registry naming its author as owner of the resource it is changing.
// The checkout is now a FALLBACK only, for runs whose target ref carries no
// registry (hermetic fixtures, local trees); it can no longer shadow the target.
func loadResourceOwnerRegistry(
	ctx context.Context,
	client refFilePort,
	project, targetRef string,
	repoFS fs.FS,
	regPath string,
) (builtin.ResourceOwnerClient, error) {
	_ = ctx
	raw, err := client.FileAtRef(project, regPath, targetRef)
	if err == nil {
		fsys := fstest.MapFS{
			path.Base(regPath): &fstest.MapFile{Data: raw},
		}
		return builtin.LoadResourceOwnerMap(fsys, path.Base(regPath))
	}
	if repoFS != nil {
		if _, statErr := fs.Stat(repoFS, regPath); statErr == nil {
			// D-129: LoadResourceOwnerMap refuses a symlinked registry; repoFS is
			// itself a symlink-safe root (checkoutFS).
			return builtin.LoadResourceOwnerMap(repoFS, regPath)
		}
	}
	return nil, fmt.Errorf("resource-owner registry %q: %w", regPath, err)
}
