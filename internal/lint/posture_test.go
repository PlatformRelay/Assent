package lint

import (
	"fmt"
	"strings"
	"testing"
)

// posture_test.go is the E3-S05 config-posture suite: the WIDENED fail-open hard
// error (three controlling-provider archetypes) and the single-writer-profile
// invariant. Each fixture isolates one archetype so the reuse boundary is proven:
// REQ-01 is caught ONLY by the reused policy.ValidateProviderPosture (a
// require-review proof provider); REQ-02 is caught ONLY by the widening (an
// entries-identity and an approval-eligibility provider the E2-S05 scan cannot
// reach); the advisory provider (failure: open, referenced by no controlling
// rule) stays clean.
//
// Tests drive checkConfigPosture directly over a tolerantly-ingested model so the
// assertions are not perturbed by the other Lint() checks; TestConfigPostureWired
// proves the wiring into Lint() itself.

// runPosture ingests the sources (the tolerant S01 bridge) and runs ONLY the
// S05 config-posture checks, returning the report.
func runPosture(sources []Source) *Report {
	rep := &Report{}
	m := ingest(sources, rep)
	checkConfigPosture(sources, m, rep)
	return rep
}

// postureDiags returns the report's diagnostics carrying code (reusing the
// package-shared diagsWithCode from scope_test.go over the sorted set).
func postureDiags(rep *Report, code string) []Diagnostic {
	return diagsWithCode(rep.Diagnostics(), code)
}

// assertNoSchemaFaults fails the test if a fixture is malformed (a schema-invalid
// or parse diagnostic means the fixture, not the check, is wrong).
func assertNoSchemaFaults(t *testing.T, rep *Report) {
	t.Helper()
	for _, code := range []string{CodeSchemaInvalid, CodeParseError} {
		if d := postureDiags(rep, code); len(d) > 0 {
			t.Fatalf("fixture is malformed — unexpected %s diagnostics: %#v", code, d)
		}
	}
}

// --- fixture builders ------------------------------------------------------

// cfgSrc builds a schema-valid Config over environment "e" / class "c" with the
// given (already-indented) providers and profiles blocks (either may be empty).
func cfgSrc(providersBlock, profilesBlock string) Source {
	body := `apiVersion: assent.dev/v1alpha1
kind: Config
environments:
  - name: e
    match: { paths: ["**"] }
classes:
  - name: c
    match: { paths: ["**"] }
`
	if providersBlock != "" {
		body += "providers:\n" + providersBlock
	}
	if profilesBlock != "" {
		body += "profiles:\n" + profilesBlock
	}
	return Source{Path: ".assent/config.yaml", Bytes: []byte(body)}
}

// provider renders one indented Config.providers entry with a failure posture.
func provider(name, failure string) string {
	return fmt.Sprintf("  %s:\n    type: http\n    failure: %s\n", name, failure)
}

// proveRuleSrc builds a schema-valid single-rule MergePolicy under pack: an
// obligation proof whose prove.when is the given CEL leaf and whose onFailure
// escalates with onEffect.
func proveRuleSrc(pack, name, obligation, onEffect, when string) Source {
	return Source{
		Path: ".assent/packs/" + pack + "/rules/" + name + ".yaml",
		Bytes: []byte(fmt.Sprintf(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: %s
spec:
  rules:
    - name: %s
      phase: enforce
      match:
        files:
          paths: ["**/*.yaml"]
      prove:
        obligation: %s
        when: %q
      onFailure:
        effect: %s
        code: %s.finding
`, name, name, obligation, when, onEffect, obligation)),
	}
}

// bindingSrcP builds a RulesetBinding routing (class, env) to pack, optionally
// requiring obligations (a comma-separated list, or "" for none).
func bindingSrcP(class, env, pack, require string) Source {
	body := fmt.Sprintf(`apiVersion: assent.dev/v1alpha1
kind: RulesetBinding
bindings:
  - class: %s
    environment: %s
    packs: [%s]
    risk: { threshold: 10 }
`, class, env, pack)
	if require != "" {
		body += "    require: [" + require + "]\n"
	}
	return Source{Path: ".assent/bindings.yaml", Bytes: []byte(body)}
}

// profileSrc builds a PolicyProfile covering (env, class) with the given write
// authority. NOTE the kind const is PolicyProfile (not Profile) — a mismatch
// would make the single-writer check silently no-op.
func profileSrc(name string, writes bool, env, class string) Source {
	return Source{
		Path: ".assent/profiles/" + name + ".yaml",
		Bytes: []byte(fmt.Sprintf(`apiVersion: assent.dev/v1alpha1
kind: PolicyProfile
metadata:
  name: %s
spec:
  writes: %t
  environments: [%s]
  classes: [%s]
`, name, writes, env, class)),
	}
}

// --- REQ-E3-S05-01: fail-open, require-review-proof archetype (reused VPP) --

// TestFailOpenControllingProofProvider — a provider whose facts prove a
// require-review obligation and is configured failure: open → fail-open (caught by
// the reused policy.ValidateProviderPosture). An advisory provider (also failure:
// open, but referenced only by a non-controlling block proof) stays clean.
func TestFailOpenControllingProofProvider(t *testing.T) {
	rep := runPosture([]Source{
		// owner: read by a require-review proof → controlling → must fail closed.
		// quota: read by a plain block proof, obligation not required, no entry
		// scope → advisory → may fail open.
		cfgSrc(provider("owner", "open")+provider("quota", "open"), ""),
		proveRuleSrc("p", "authz", "authorized", "require-review", "facts.owner.approved == true"),
		proveRuleSrc("p", "quota", "quota-ok", "block", "facts.quota.remaining > 0"),
	})
	assertNoSchemaFaults(t, rep)

	fo := postureDiags(rep, CodeFailOpen)
	if len(fo) != 1 {
		t.Fatalf("want exactly 1 fail-open (owner only), got %d: %#v", len(fo), fo)
	}
	if !strings.Contains(fo[0].Message, "owner") {
		t.Errorf("fail-open should name the controlling provider owner; got %q", fo[0].Message)
	}
	if strings.Contains(fo[0].Message, "quota") {
		t.Errorf("advisory provider quota must NOT be flagged (it may fail open); got %q", fo[0].Message)
	}
}

// --- REQ-E3-S05-02: fail-open widened to entries-identity + eligibility -----

// TestFailOpenEntriesIdentityAndEligibilityProviders — the widening the E2-S05
// scan does NOT reach: an entries-identity provider (read by a proof authorizing
// an identified entry via the entry scope) and an approval-eligibility provider
// (proving an obligation a binding requires), BOTH on onFailure: block so
// policy.ValidateProviderPosture (require-review only) never sees them.
func TestFailOpenEntriesIdentityAndEligibilityProviders(t *testing.T) {
	rep := runPosture([]Source{
		cfgSrc(provider("idp", "open")+provider("elig", "open"), ""),
		// (b) entries-identity: reads the entry scope + facts.idp.
		proveRuleSrc("p", "owns", "ownership", "block", "entry.owner in facts.idp.groups"),
		// (c) approval-eligibility: proves a REQUIRED obligation, no entry scope.
		proveRuleSrc("p", "sign", "sign-off", "block", "facts.elig.approved == true"),
		bindingSrcP("c", "e", "p", "sign-off"),
	})
	assertNoSchemaFaults(t, rep)

	fo := postureDiags(rep, CodeFailOpen)
	if len(fo) != 2 {
		t.Fatalf("want 2 fail-open (idp entries-identity + elig approval-eligibility), got %d: %#v", len(fo), fo)
	}
	var sawIDP, sawElig bool
	for _, d := range fo {
		if strings.Contains(d.Message, "idp") && strings.Contains(d.Message, "entries-identity") {
			sawIDP = true
		}
		if strings.Contains(d.Message, "elig") && strings.Contains(d.Message, "approval-eligibility") {
			sawElig = true
		}
	}
	if !sawIDP {
		t.Errorf("expected an entries-identity fail-open naming idp; got %#v", fo)
	}
	if !sawElig {
		t.Errorf("expected an approval-eligibility fail-open naming elig; got %#v", fo)
	}
}

// TestFailOpenAdvisoryProviderClean — a provider referenced only by a
// non-controlling proof (block, obligation not required, no entry scope) may be
// configured failure: open with no diagnostic (ADR-0017 §6 restricts only
// controlling facts).
func TestFailOpenAdvisoryProviderClean(t *testing.T) {
	rep := runPosture([]Source{
		cfgSrc(provider("hint", "open"), ""),
		proveRuleSrc("p", "hint", "hint-ok", "block", "facts.hint.value > 0"),
	})
	assertNoSchemaFaults(t, rep)
	if fo := postureDiags(rep, CodeFailOpen); len(fo) != 0 {
		t.Fatalf("advisory provider must lint clean, got %d fail-open: %#v", len(fo), fo)
	}
}

// --- REQ-E3-S05-03: single-writer-profile invariant ------------------------

// TestSingleWriterProfileInvariant — per (environment, class) binding, zero or
// more-than-one covering writes:true PolicyProfile is rejected; exactly one is
// clean (ADR-0018 §2, via the reused aggregate.ResolveProfile).
func TestSingleWriterProfileInvariant(t *testing.T) {
	binding := bindingSrcP("c", "e", "p", "")

	t.Run("two writers rejected", func(t *testing.T) {
		rep := runPosture([]Source{
			cfgSrc("", "  - name: w1\n  - name: w2\n"),
			profileSrc("w1", true, "e", "c"),
			profileSrc("w2", true, "e", "c"),
			binding,
		})
		assertNoSchemaFaults(t, rep)
		if sw := postureDiags(rep, CodeSingleWriterProfile); len(sw) != 1 {
			t.Fatalf("two covering writers → exactly one single-writer-profile, got %d: %#v", len(sw), sw)
		}
	})

	t.Run("zero writers with profiles present rejected", func(t *testing.T) {
		rep := runPosture([]Source{
			cfgSrc("", "  - name: r1\n"),
			profileSrc("r1", false, "e", "c"),
			binding,
		})
		assertNoSchemaFaults(t, rep)
		if sw := postureDiags(rep, CodeSingleWriterProfile); len(sw) != 1 {
			t.Fatalf("zero covering writers (profiles present) → single-writer-profile, got %d: %#v", len(sw), sw)
		}
	})

	t.Run("exactly one writer clean", func(t *testing.T) {
		rep := runPosture([]Source{
			cfgSrc("", "  - name: w1\n"),
			profileSrc("w1", true, "e", "c"),
			binding,
		})
		assertNoSchemaFaults(t, rep)
		if sw := postureDiags(rep, CodeSingleWriterProfile); len(sw) != 0 {
			t.Fatalf("exactly one covering writer → clean, got %d single-writer-profile: %#v", len(sw), sw)
		}
	})

	t.Run("no profiles table → check does not fire", func(t *testing.T) {
		rep := runPosture([]Source{
			cfgSrc("", ""),
			binding,
		})
		assertNoSchemaFaults(t, rep)
		if sw := postureDiags(rep, CodeSingleWriterProfile); len(sw) != 0 {
			t.Fatalf("absent Config.profiles → no single-writer requirement, got %#v", sw)
		}
	})
}

// --- REQ-E3-S05-04: determinism --------------------------------------------

// TestPostureChecksDoubleRunStable — the whole config-posture pass is byte-stable
// across runs (resolution driven by the Config.profiles precedence table, output
// canonically sorted): no map/clock/env/random leaks.
func TestPostureChecksDoubleRunStable(t *testing.T) {
	sources := []Source{
		cfgSrc(provider("idp", "open")+provider("quota", "closed"),
			"  - name: w1\n  - name: w2\n"),
		profileSrc("w1", true, "e", "c"),
		profileSrc("w2", true, "e", "c"),
		proveRuleSrc("p", "owns", "ownership", "block", "entry.owner in facts.idp.groups"),
		bindingSrcP("c", "e", "p", ""),
	}
	first := runPosture(sources).Render()
	second := runPosture(sources).Render()
	if first != second {
		t.Fatalf("posture render not stable across runs:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	// Sanity: the fixture actually exercises both checks (else "stable" is vacuous).
	rep := runPosture(sources)
	if len(postureDiags(rep, CodeFailOpen)) == 0 || len(postureDiags(rep, CodeSingleWriterProfile)) == 0 {
		t.Fatalf("determinism fixture should trigger both a fail-open and a single-writer-profile; got:\n%s", rep.Render())
	}
}

// TestConfigPostureWired — the posture checks are reachable through Lint()
// itself, not only the direct helper (call-site wiring in lint.go).
func TestConfigPostureWired(t *testing.T) {
	rep := Lint([]Source{
		cfgSrc(provider("owner", "open"), ""),
		proveRuleSrc("p", "authz", "authorized", "require-review", "facts.owner.approved == true"),
	})
	if len(postureDiags(rep, CodeFailOpen)) == 0 {
		t.Fatalf("checkConfigPosture is not wired into Lint(): no fail-open diagnostic:\n%s", rep.Render())
	}
}
