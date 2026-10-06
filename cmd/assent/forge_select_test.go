package main

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge/factory"
)

// forge_select_test.go is REQ-E10-S13-01's named gate (TestForgeSelection).
// The selector's whole point is fail-closed resolution — every row below
// asserts the exact forge AND the exact endpoint handed to the factory, so a
// mis-routed credential (the GitHub token dispatched to gitlab.com) reds here
// rather than at adopter time.

func TestForgeSelection(t *testing.T) {
	for _, tc := range []struct {
		name       string
		forge      string
		endpoint   string
		wantKind   factory.Kind
		wantHost   string
		wantErr    bool
		wantErrSub string
	}{
		{
			name: "explicit_github_default_endpoint_targets_github",
			// The flag default (https://gitlab.com) is NOT an operator choice:
			// --forge github without a named endpoint must resolve
			// api.github.com, never the GitLab host (credential-leak shape).
			forge:    "github",
			wantKind: factory.KindGitHub,
			wantHost: "api.github.com",
		},
		{
			name:     "github_with_foreign_endpoint_fails_closed",
			forge:    "github",
			endpoint: "https://gitlab.example",
			wantErr:  true,
		},
		{
			name:     "github_with_explicit_github_endpoint",
			forge:    "github",
			endpoint: "https://api.github.com",
			wantKind: factory.KindGitHub,
			wantHost: "api.github.com",
		},
		{
			// The credential-leak guard in its REAL flag shape: the operator
			// names only --forge github and the -gitlab-endpoint default
			// (https://gitlab.com) rides along untouched — the endpoint must
			// still resolve to api.github.com.
			name:     "github_with_flag_default_endpoint_targets_github",
			forge:    "github",
			endpoint: "https://gitlab.com",
			wantKind: factory.KindGitHub,
			wantHost: "api.github.com",
		},
		{
			// The github.com WEB host is not an acceptable endpoint override:
			// the adapter needs the REST base, and a github.com endpoint would
			// 404 every REST call — the exact late-failure the error text
			// promises to prevent.
			name:       "github_with_web_host_fails_closed",
			forge:      "github",
			endpoint:   "https://github.com",
			wantErr:    true,
			wantErrSub: "REST base",
		},
		{
			// Autodetect with the REAL flag-default shape: the operator named
			// nothing and the endpoint is the flag default — a gitlab.com host
			// autodetects GitLab.
			name:     "autodetect_gitlab_com",
			endpoint: "https://gitlab.com",
			wantKind: factory.KindGitLab,
			wantHost: "gitlab.com",
		},
		{
			// Empty kind + empty endpoint names no forge at all: there is no
			// host to autodetect from, and silently defaulting to GitLab is
			// the fail-open REQ-E10-S13-01 forbids.
			name:       "no_kind_no_endpoint_fails_closed",
			wantErr:    true,
			wantErrSub: "--forge is required",
		},
		{
			name:     "autodetect_api_github_com",
			endpoint: "https://api.github.com",
			wantKind: factory.KindGitHub,
			wantHost: "api.github.com",
		},
		{
			name:     "unrecognised_host_fails_closed",
			endpoint: "https://forge.example.org",
			wantErr:  true,
		},
		{
			// The forge/host mismatch in the OTHER direction: GITLAB_TOKEN must
			// not be dispatched to a GitHub host (the mirror of the GitHub
			// arm's credential-leak guard).
			name:       "gitlab_with_github_host_fails_closed",
			forge:      "gitlab",
			endpoint:   "https://api.github.com",
			wantErr:    true,
			wantErrSub: "foreign host",
		},
		{
			name:    "explicit_gitea_fails_closed",
			forge:   "gitea",
			wantErr: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotKind, gotEndpoint, err := selectForge(tc.forge, tc.endpoint)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("selectForge(%q, %q) = (%q, %q), want error", tc.forge, tc.endpoint, gotKind, gotEndpoint)
				}
				if tc.wantErrSub != "" && !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("the refusal must name its cause (%q), got %v", tc.wantErrSub, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectForge: %v", err)
			}
			if gotKind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", gotKind, tc.wantKind)
			}
			if gotHost := hostOf(gotEndpoint); gotHost != tc.wantHost {
				t.Fatalf("endpoint host = %q, want %q (forge-consistent endpoint is the credential-leak guard)", gotHost, tc.wantHost)
			}
		})
	}
}
