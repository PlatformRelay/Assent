package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge/factory"
)

// forge_select.go — explicit forge selection with remote-host autodetect
// (E10-S13). The rule is FAIL-CLOSED: ambiguity or an unrecognised host is an
// error — never a default to GitLab (REQ-E10-S13-01).
//
// The endpoint default follows the KIND, not the flag: -gitlab-endpoint carries
// a non-empty flag default, so an endpoint the operator never named must not be
// handed to the GitHub adapter — that would dispatch the GitHub token to
// gitlab.com. The resolved endpoint is forge-consistent or an error.

const gitlabDefaultEndpoint = "https://gitlab.com"

// selectForge resolves the forge and a forge-consistent endpoint.
func selectForge(kind, endpoint string) (forgeKind factory.Kind, resolvedEndpoint string, err error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case string(factory.KindGitLab):
		return factory.KindGitLab, defaultGitLabEndpoint(endpoint), nil
	case string(factory.KindGitHub):
		// The GitLab-named flag's DEFAULT is not an operator choice: handing
		// gitlab.com to the GitHub adapter would dispatch the GitHub token to
		// the wrong host. Only an endpoint the operator NAMED (or a
		// GitHub-shaped one) may override the GitHub default.
		if strings.TrimSpace(endpoint) == "" || endpoint == gitlabDefaultEndpoint {
			return factory.KindGitHub, "https://api.github.com", nil
		}
		if h := hostOf(endpoint); h != "api.github.com" && h != "github.com" {
			return "", "", fmt.Errorf("--forge github requires a GitHub endpoint (https://api.github.com); got %q — refusing to send the GitHub credential to a foreign host (E10-S13)", endpoint)
		}
		return factory.KindGitHub, strings.TrimRight(endpoint, "/"), nil
	case "":
		if strings.TrimSpace(endpoint) == "" {
			return factory.KindGitLab, defaultGitLabEndpoint(endpoint), nil
		}
		detected, err := detectForge(endpoint)
		if err != nil {
			return "", "", err
		}
		return detected, defaultGitLabEndpoint(endpoint), nil
	default:
		return "", "", fmt.Errorf("--forge %q is not one of %q or %q", kind, factory.KindGitLab, factory.KindGitHub)
	}
}

// hostOf extracts the lowercase host of an endpoint URL. A bare host string is
// returned as-is (operators pass --gitlab-endpoint as a URL in practice; the
// bare form only needs host matching to not crash).
func hostOf(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return strings.TrimSpace(endpoint)
	}
	return strings.ToLower(u.Host)
}

func defaultGitLabEndpoint(endpoint string) string {
	if strings.TrimSpace(endpoint) == "" {
		return gitlabDefaultEndpoint
	}
	return strings.TrimSuffix(strings.TrimRight(endpoint, "/"), "/api/v4")
}

// detectForge autodetects the forge from the endpoint host, matching EXACT
// hosts only — a substring match (contains "gitlab") would classify an
// attacker-chosen host, exactly the heuristic shape ADR-0021 forbids (SEC-04).
func detectForgeFromEndpoint(endpoint string) (factory.Kind, bool) {
	host := hostOf(endpoint)
	switch host {
	case "gitlab.com":
		return factory.KindGitLab, true
	case "api.github.com", "github.com":
		return factory.KindGitHub, true
	default:
		return "", false
	}
}

// detectForge is the fail-closed autodetect: an unrecognised host is an error,
// never a default (REQ-E10-S13-01). Self-hosted GitLab instances keep the
// explicit --forge gitlab (a self-hosted host is unrecognisable by design).
func detectForge(endpoint string) (factory.Kind, error) {
	host := hostOf(endpoint)
	switch {
	case host == "gitlab.com":
		return factory.KindGitLab, nil
	case host == "api.github.com" || host == "github.com":
		return factory.KindGitHub, nil
	default:
		return "", fmt.Errorf("cannot detect the forge from endpoint %q — supply --forge gitlab|github explicitly (E10-S13: ambiguity fails closed, never defaults)", endpoint)
	}
}
