package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge/factory"
)

// forge_select.go — explicit forge selection with remote-host autodetect
// (E10-S13). The rule is FAIL-CLOSED: ambiguity or an unrecognised host is an
// error — never a default to GitLab (REQ-E10-S13-01). With two adapters exist,
// a default would silently mis-route a run to the wrong forge's API shape.

// ForgeKind is the --forge flag's value space.

// selectForge resolves the forge from an explicit --forge value (which may be
// empty = autodetect) and the configured endpoint. Ambiguity (a host both
// forges could plausibly own) and unrecognised hosts fail closed.
//
// Autodetect inspects the endpoint host: gitlab.com (or a self-hosted host
// whose URL the operator names with --gitlab-endpoint) is GitLab;
// api.github.com (the REST base) is GitHub. The endpoint default follows the
// detected kind, so `--forge github` without an endpoint targets
// https://api.github.com and never GitLab's API.
func selectForge(kind, endpoint string) (forgeKind factory.Kind, resolvedEndpoint string, err error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case string(factory.KindGitLab):
		return factory.KindGitLab, defaultGitLabEndpoint(endpoint), nil
	case string(factory.KindGitHub):
		return factory.KindGitHub, defaultEndpoint(endpoint, "https://api.github.com"), nil
	case "":
		return factory.KindGitLab, "", fmt.Errorf("--forge is required when the endpoint is not an unambiguous forge host (got %q); pass --forge gitlab or --forge github — no default forge exists (E10-S13)", kind)
	default:
		return "", "", fmt.Errorf("--forge %q is not one of %q or %q", kind, factory.KindGitLab, factory.KindGitHub)
	}
}

func defaultEndpoint(endpoint, fallback string) string {
	if strings.TrimSpace(endpoint) == "" {
		return fallback
	}
	return strings.TrimRight(endpoint, "/")
}

func hostOf(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		// A bare host string ("gitlab.example") is still usable for the
		// contains()-style matching forge operators expect; fall back to the
		// raw value's host-ish prefix.
		return strings.TrimSpace(endpoint)
	}
	return strings.ToLower(u.Host)
}

func defaultGitLabEndpoint(endpoint string) string {
	if strings.TrimSpace(endpoint) == "" {
		return "https://gitlab.com"
	}
	return strings.TrimSuffix(strings.TrimRight(endpoint, "/"), "/api/v4")
}

func detectForgeFromEndpoint(endpoint string) (factory.Kind, bool) {
	host := hostOf(endpoint)
	switch {
	case host == "gitlab.com" || strings.Contains(host, "gitlab"):
		return factory.KindGitLab, true
	case host == "api.github.com" || host == "github.com":
		return factory.KindGitHub, true
	default:
		return "", false
	}
}

func detectForge(endpoint string) (factory.Kind, error) {
	k, ok := detectForgeFromEndpoint(endpoint)
	if !ok {
		return "", fmt.Errorf("cannot detect the forge from endpoint %q — supply --forge gitlab|github explicitly (E10-S13: ambiguity fails closed, never defaults)", endpoint)
	}
	return k, nil
}
