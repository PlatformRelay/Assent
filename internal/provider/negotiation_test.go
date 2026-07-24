package provider_test

import (
	"strconv"
	"testing"

	"github.com/PlatformRelay/assent/internal/provider"
)

func itoa(n int) string { return strconv.Itoa(n) }

// REQ-P3-E2-S04-01/02/03 — executable provider major-version negotiation matrix.
// Host major H vs provider major P: accept iff P == H; else capability gap →
// provider refused, every requested fact unavailable, never auto-merge.
func TestMajorNegotiation(t *testing.T) {
	const hostMajor = provider.HostMajor // frozen envelope: provider.assent.dev/v1alpha1 → 1
	outputs := []string{"isOwner", "members"}

	t.Run("match", func(t *testing.T) {
		// REQ-P3-E2-S04-01: provider declares the host's current major → accept.
		got := provider.Negotiate(hostMajor, "provider.assent.dev/v1alpha1", outputs)
		if got.Outcome != provider.OutcomeAccept {
			t.Fatalf("outcome: got %q, want %q", got.Outcome, provider.OutcomeAccept)
		}
		if got.ProviderRefused {
			t.Fatal("matched major must not refuse the provider")
		}
		if !got.AutoMergeEligible {
			t.Fatal("matched major must leave auto-merge eligibility open (facts process normally)")
		}
		if len(got.FactStates) != 0 {
			t.Fatalf("accept must not force fact states; got %v", got.FactStates)
		}
		if got.ProviderMajor == nil || *got.ProviderMajor != hostMajor {
			t.Fatalf("ProviderMajor: got %v, want %d", got.ProviderMajor, hostMajor)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		// REQ-P3-E2-S04-02: older or newer major → capability gap + facts unavailable.
		cases := []struct {
			name       string
			apiVersion string
			wantMajor  int
		}{
			{name: "older", apiVersion: "provider.assent.dev/v0alpha1", wantMajor: 0},
			{name: "newer", apiVersion: "provider.assent.dev/v2beta1", wantMajor: 2},
			{name: "newer_stable", apiVersion: "provider.assent.dev/v2", wantMajor: 2},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assertCapabilityGap(t, provider.Negotiate(hostMajor, tc.apiVersion, outputs), outputs, &tc.wantMajor)
			})
		}
	})

	t.Run("missing", func(t *testing.T) {
		// REQ-P3-E2-S04-03: missing or unparseable major → same as mismatch (fail closed).
		cases := []struct {
			name       string
			apiVersion string
		}{
			{name: "empty", apiVersion: ""},
			{name: "omitted_dodge", apiVersion: "provider.assent.dev/"}, // adversarial: omit version to dodge
			{name: "no_group_slash", apiVersion: "v1alpha1"},
			{name: "garbage", apiVersion: "not-a-version"},
			{name: "no_leading_digits", apiVersion: "provider.assent.dev/alpha1"},
			{name: "whitespace", apiVersion: "   "},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assertCapabilityGap(t, provider.Negotiate(hostMajor, tc.apiVersion, outputs), outputs, nil)
			})
		}
	})
}

func assertCapabilityGap(t *testing.T, got provider.Negotiation, outputs []string, wantMajor *int) {
	t.Helper()
	if got.Outcome != provider.OutcomeCapabilityGap {
		t.Fatalf("outcome: got %q, want %q", got.Outcome, provider.OutcomeCapabilityGap)
	}
	if !got.ProviderRefused {
		t.Fatal("capability gap must refuse the provider")
	}
	if got.AutoMergeEligible {
		t.Fatal("capability-gap facts must never remain auto-merge eligible")
	}
	if len(got.FactStates) != len(outputs) {
		t.Fatalf("FactStates len: got %d, want %d (%v)", len(got.FactStates), len(outputs), got.FactStates)
	}
	for _, name := range outputs {
		if st := got.FactStates[name]; st != provider.FactStateUnavailable {
			t.Fatalf("fact %q state: got %q, want %q", name, st, provider.FactStateUnavailable)
		}
	}
	if wantMajor == nil {
		if got.ProviderMajor != nil {
			t.Fatalf("ProviderMajor: got %v, want nil (unparseable)", *got.ProviderMajor)
		}
		return
	}
	if got.ProviderMajor == nil || *got.ProviderMajor != *wantMajor {
		t.Fatalf("ProviderMajor: got %v, want %d", got.ProviderMajor, *wantMajor)
	}
}

// Matrix covers host current major plus at least one older and one newer cell (DoD).
func TestMajorNegotiationMatrix(t *testing.T) {
	outputs := []string{"factA"}
	type cell struct {
		h, p    int
		accept  bool
		apiVers string
	}
	cells := []cell{
		{h: 1, p: 1, accept: true, apiVers: "provider.assent.dev/v1alpha1"},
		{h: 1, p: 0, accept: false, apiVers: "provider.assent.dev/v0"},
		{h: 1, p: 2, accept: false, apiVers: "provider.assent.dev/v2alpha1"},
		{h: 2, p: 2, accept: true, apiVers: "provider.assent.dev/v2"},
		{h: 2, p: 1, accept: false, apiVers: "provider.assent.dev/v1alpha1"},
	}
	for _, c := range cells {
		name := "H" + itoa(c.h) + "_P" + itoa(c.p)
		if c.accept {
			name += "_accept"
		} else {
			name += "_gap"
		}
		t.Run(name, func(t *testing.T) {
			got := provider.Negotiate(c.h, c.apiVers, outputs)
			if c.accept {
				if got.Outcome != provider.OutcomeAccept {
					t.Fatalf("H=%d P=%d: got %q, want accept", c.h, c.p, got.Outcome)
				}
				return
			}
			assertCapabilityGap(t, got, outputs, &c.p)
		})
	}
}

func TestParseMajor(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"provider.assent.dev/v1alpha1", 1, true},
		{"provider.assent.dev/v2", 2, true},
		{"provider.assent.dev/v0beta1", 0, true},
		{"provider.assent.dev/v10alpha2", 10, true},
		{"", 0, false},
		{"provider.assent.dev/", 0, false},
		{"garbage", 0, false},
		{"provider.assent.dev/alpha1", 0, false},
	}
	for _, tc := range cases {
		got, ok := provider.ParseMajor(tc.in)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Errorf("ParseMajor(%q) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}
