package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// AUD2-S02 / REL-03 — `providers/<name>.json` absence vs. an unanswering forge.
//
// resolveRunFacts used to `continue` on ANY FileAtRef error for the host
// declaration, so a 503, a throttle or a mis-scoped token was indistinguishable
// from "this provider declares nothing". The fail-safe DIRECTION held (the fact
// never binds, so CEL sees an absent attribute and the run degrades to REVIEW),
// but the path was invisible: the operator read a `predicate.error` about a
// missing attribute, and a `has()`-tolerant policy quietly took its fallback
// branch. D-130 already ruled this exact conflation for the ownership registry
// at loadResourceOwnerRegistry; these tests pin the sibling call site.
//
// The declaration path under test is `<dir(config)>/providers/<name>.json` at the
// TARGET ref (D-065) — `.assent/providers/quota.json` for the fixtures below.
const quotaDeclPath = ".assent/providers/quota.json"

// declErrPort is the whole forge with ONE read broken: the host declaration for
// `quota` fails with the supplied error, everything else answers normally. Same
// shape as registry503Port (provider_host_registry_test.go) — a stub covering
// only the declaration read could not express a forge that is otherwise healthy,
// which is precisely the live shape of a blip or a scope-limited token.
type declErrPort struct {
	forgePort
	declPath string
	err      error
}

func (c declErrPort) FileAtRef(project, p, ref string) ([]byte, error) {
	if p == c.declPath {
		return nil, c.err
	}
	return c.forgePort.FileAtRef(project, p, ref)
}

// TestProviderDeclarationAbsentSkipsProvider — REQ-AUD2-S02-01.
//
// Absence stays absence: today's behaviour, preserved byte for byte. The fake
// serves no declaration for `quota`, so the request 404s and the REAL gitlab
// adapter wraps it onto forge.ErrNotFound through its own chain — this proves
// the wrap survives, which a hand-rolled sentinel stub could not.
func TestProviderDeclarationAbsentSkipsProvider(t *testing.T) {
	f := newFakeGitLab(t)
	f.config = configQuotaRepoFile()
	f.providerDecls = nil // nothing declared → the fake answers 404
	client := f.factory()("", "tok", "assent-bot")

	conf, err := policy.LoadConfig([]byte(configQuotaRepoFile()))
	if err != nil {
		t.Fatal(err)
	}

	facts, resolvedAt, err := resolveRunFacts(
		context.Background(), conf, ".assent/config.yaml", client,
		"42", "main", t.TempDir(), "file:topics/prod/orders.yaml", time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("an absent declaration must stay a skip, not an error: %v", err)
	}
	if len(facts) != 0 || len(resolvedAt) != 0 {
		t.Fatalf("a skipped provider must bind no facts; got facts=%v resolvedAt=%v", facts, resolvedAt)
	}
}

// TestProviderDeclarationForgeErrorAbortsResolveRunFacts — REQ-AUD2-S02-02.
//
// A 503 on the declaration read is NOT an absent file. Before this story the run
// continued with an empty fact map and never mentioned the forge failure; the
// operator saw only a missing-attribute predicate error, and a `has()`-tolerant
// policy took its fallback branch on a forge blip. Consistency argument: the very
// next statement in resolveRunFacts already hard-fails on a MALFORMED declaration,
// so failing on an UNREADABLE one is the same policy applied to the same input.
func TestProviderDeclarationForgeErrorAbortsResolveRunFacts(t *testing.T) {
	f := newFakeGitLab(t)
	f.config = configQuotaRepoFile()
	f.providerDecls = map[string]string{"quota": quotaDeclarationJSON}
	client := declErrPort{
		forgePort: f.factory()("", "tok", "assent-bot"),
		declPath:  quotaDeclPath,
		err:       brokenForge(quotaDeclPath, "main", 503),
	}

	conf, err := policy.LoadConfig([]byte(configQuotaRepoFile()))
	if err != nil {
		t.Fatal(err)
	}

	facts, resolvedAt, err := resolveRunFacts(
		context.Background(), conf, ".assent/config.yaml", client,
		"42", "main", t.TempDir(), "file:topics/prod/orders.yaml", time.Now().UTC(),
	)
	if err == nil {
		t.Fatalf("a 503 on the host declaration must abort fact resolution; "+
			"got facts=%v resolvedAt=%v", facts, resolvedAt)
	}
	// The message has to be actionable: WHICH provider, WHICH path, WHICH ref.
	// Without the ref an operator cannot tell a target-ref read from a head read.
	//
	// Asserted as ONE CONTIGUOUS substring on purpose. brokenForge already renders
	// the path and the ref inside the wrapped cause, so three separate Contains
	// checks would pass against a bare `provider %q: %w` wrap — two of the three
	// could not fail, and the test would measure the fake instead of the code.
	// Only the outer wrap can produce this exact run of bytes.
	wantWrap := fmt.Sprintf("provider %q declaration %q at ref %q", "quota", quotaDeclPath, "main")
	if !strings.Contains(err.Error(), wantWrap) {
		t.Fatalf("error %q must contain %q — the provider, the declaration path and the ref", err, wantWrap)
	}
	// ...and the forge's own cause must survive the wrap (%w, not %v-and-drop).
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("error %q must carry the forge's own failure", err)
	}
	// Fail-closed shape: no partial fact map escapes alongside the error, so no
	// caller can evaluate a provider-configured policy on an empty Facts map.
	if facts != nil || resolvedAt != nil {
		t.Fatalf("error path must return no facts; got %v / %v", facts, resolvedAt)
	}
}

// TestProviderDeclarationUnauthorizedAbortsResolveRunFacts — REQ-AUD2-S02-03.
//
// A 401/403 is deterministic, not transient: a token scoped away from the
// governance repo would have made EVERY provider silently vanish on EVERY run,
// converting approvable MRs to REVIEW forever with nothing in the logs to say
// why. This is the misconfiguration the old `continue` hid best.
func TestProviderDeclarationUnauthorizedAbortsResolveRunFacts(t *testing.T) {
	for _, status := range []int{401, 403} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			f := newFakeGitLab(t)
			f.config = configQuotaRepoFile()
			f.providerDecls = map[string]string{"quota": quotaDeclarationJSON}
			client := declErrPort{
				forgePort: f.factory()("", "tok", "assent-bot"),
				declPath:  quotaDeclPath,
				err:       brokenForge(quotaDeclPath, "main", status),
			}

			conf, err := policy.LoadConfig([]byte(configQuotaRepoFile()))
			if err != nil {
				t.Fatal(err)
			}

			facts, resolvedAt, err := resolveRunFacts(
				context.Background(), conf, ".assent/config.yaml", client,
				"42", "main", t.TempDir(), "file:topics/prod/orders.yaml", time.Now().UTC(),
			)
			if err == nil {
				t.Fatalf("a %d on the host declaration must abort fact resolution; "+
					"got facts=%v resolvedAt=%v", status, facts, resolvedAt)
			}
			if !strings.Contains(err.Error(), `provider "quota"`) {
				t.Fatalf("error %q must name the provider whose declaration could not be read", err)
			}
			if facts != nil || resolvedAt != nil {
				t.Fatalf("error path must return no facts; got %v / %v", facts, resolvedAt)
			}
		})
	}
}
