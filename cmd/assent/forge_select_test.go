package main

import (
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
			name:     "autodetect_gitlab_com",
			wantKind: factory.KindGitLab,
			wantHost: "gitlab.com",
		},
		{
			name:     "unrecognised_host_fails_closed",
			endpoint: "https://forge.example.org",
			wantErr:  true,
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
