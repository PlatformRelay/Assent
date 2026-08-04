package main

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/provider"
)

// defaultProviderTimeout bounds one HTTP/exec ResolveFacts call on the live path.
const defaultProviderTimeout = 5 * time.Second

// resolveRunFacts resolves configured providers at the cmd/assent edge into
// EvaluationInput.Facts and pins.factsResolvedAt (REQ-E5-S05-01).
//
// Compatibility (REQ-E5-S05-02): nil/empty providers → empty maps (today's path).
// Builtin types are skipped until E5-S06 — no invented unavailable keys (absence
// keeps CEL fail-safe identical to pre-S05). Host declarations load from the
// TARGET ref beside Config (D-065): <dir(config)>/providers/<name>.json.
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

	for _, name := range names {
		p := conf.Providers[name]
		switch {
		case p.Type == "http" && strings.TrimSpace(p.URL) != "":
			// resolvable
		case p.Type == "exec":
			// resolvable when host declaration carries Exec pin
		default:
			// builtin/* and unknown: skip (S06+). Do not invent keys.
			continue
		}

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
			provider.Subject{Kind: "repo", ID: project},
			outputs,
			nil, // no change projections on the S05 wire path (minimization still applies)
		)

		call, err := providerCallFor(p, hostCfg, q)
		if err != nil {
			return nil, nil, fmt.Errorf("provider %q: %w", name, err)
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

// providerCallFor builds the transport CallFunc for one configured provider,
// capturing the FactQuery the host built (projection-minimized).
func providerCallFor(p policy.Provider, hostCfg provider.Config, q provider.FactQuery) (provider.CallFunc, error) {
	switch p.Type {
	case "http":
		url := strings.TrimSpace(p.URL)
		if url == "" {
			return nil, fmt.Errorf("http provider requires url")
		}
		return func(callCtx context.Context) ([]byte, error) {
			return provider.CallHTTP(callCtx, url, q, defaultProviderTimeout)
		}, nil
	case "exec":
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
	default:
		return nil, fmt.Errorf("unsupported provider type %q", p.Type)
	}
}
