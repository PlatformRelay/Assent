package forge_test

// port_test.go is the REQ-AUD-S15-01 gate for the ARCH-02 port lift: the
// orchestration READ port speaks forge-neutral types only, and the GitLab
// adapter wraps its 404s onto the neutral forge.ErrNotFound sentinel.
//
// It lives in package forge_test (not forge) on purpose: proving the wrap chain
// requires driving the REAL adapter, and internal/forge/gitlab imports
// internal/forge — an in-package test would be an import cycle. Everything here
// runs against an httptest GitLab; no live network.

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/gitlab"
)

// gitlabPkgPath is the adapter package no type in the read port may come from.
const gitlabPkgPath = "github.com/PlatformRelay/assent/internal/forge/gitlab"

// runReadPort restates the orchestration read port `cmd/assent` depends on
// (cmd/assent.forgePort — unexported there, so it cannot be reflected over from
// here). Its whole point is that it is writable with NO reference to any
// adapter package: this file does not import gitlab for the port declaration,
// only to prove a concrete adapter satisfies it.
type runReadPort interface {
	forge.Forge
	forge.Snapshotter
	forge.Resolver
	GetMR(project, mr string) (forge.MRInfo, error)
	FileAtRef(project, path, ref string) ([]byte, error)
}

// The GitLab adapter must satisfy the neutral port. This assignment is the
// compile-time half of "the port is self-contained": if a gitlab-named type
// were still required in a signature, this would not build.
var _ runReadPort = (*gitlab.Client)(nil)

// newTestClient builds a real adapter pointed at srv, with the retry sleeper
// neutralised so a non-retryable status cannot slow the test down.
func newTestClient(srv *httptest.Server) *gitlab.Client {
	return gitlab.New(srv.URL, "test-token", "assent-bot", gitlab.WithSleeper(func(time.Duration) {}))
}

// TestFileAtRefAbsentMatchesNeutralSentinel (REQ-AUD-S15-01) drives a real
// *gitlab.Client against an httptest GitLab that 404s, and proves the error it
// produces matches forge.ErrNotFound THROUGH THE ADAPTER'S OWN WRAP CHAIN — the
// property cmd/assent's fileAtRefOrAbsent presence signal (EFE-S03) rests on.
//
// It deliberately does not hand-construct an error that satisfies errors.Is:
// the only error under test is the one the adapter returned.
func TestFileAtRefAbsentMatchesNeutralSentinel(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 File Not Found"}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv).FileAtRef("42", ".assent/merge-policy.yaml", "main")
	if err == nil {
		t.Fatal("FileAtRef on a 404 returned nil error — the absent-file signal is gone")
	}
	if gotPath == "" {
		t.Fatal("the httptest GitLab was never called — the adapter short-circuited and this test proved nothing")
	}

	// (a) the neutral port sentinel matches.
	if !errors.Is(err, forge.ErrNotFound) {
		t.Fatalf("errors.Is(err, forge.ErrNotFound) = false for %v (%T) — the adapter did not wrap onto the port sentinel", err, err)
	}

	// (b) it matches because of a REAL wrap, not a bespoke Is() shim: unwrapping
	//     the adapter's error must arrive at the identical sentinel value.
	if unwrapped := errors.Unwrap(err); unwrapped != forge.ErrNotFound { //nolint:errorlint // identity of the sentinel is exactly the assertion
		t.Fatalf("errors.Unwrap(err) = %v, want the forge.ErrNotFound value itself (the chain must be a genuine %%w wrap)", unwrapped)
	}

	// (c) the RENDERED message is byte-identical to the pre-lift adapter's. No
	//     golden covers the forge path (assent test/compare never build a forge),
	//     so the refactor gate for this string is pinned here.
	const wantMsg = `gitlab: resource not found (404): file ".assent/merge-policy.yaml" at ref "main"`
	if err.Error() != wantMsg {
		t.Fatalf("404 message drifted:\n got: %s\nwant: %s", err.Error(), wantMsg)
	}

	// (d) the transitional adapter alias is the SAME value, so callers still on
	//     gitlab.ErrNotFound keep matching (AUD-S13 / adapter tests).
	if gitlab.ErrNotFound != forge.ErrNotFound { //nolint:errorlint // value identity is the assertion
		t.Fatal("gitlab.ErrNotFound is no longer the forge.ErrNotFound value — the alias stopped being an alias")
	}
}

// TestNonAbsentForgeErrorDoesNotMatchNeutralSentinel is the negative control for
// the test above: a fail-CLOSED gate that matched every error would silently
// turn a broken forge into "file absent" — i.e. a decision on empty input.
func TestNonAbsentForgeErrorDoesNotMatchNeutralSentinel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
	}{
		{"server error", http.StatusInternalServerError},
		{"forbidden", http.StatusForbidden},
		{"gone", http.StatusGone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			_, err := newTestClient(srv).FileAtRef("42", "cfg.yaml", "main")
			if err == nil {
				t.Fatalf("FileAtRef on %d returned nil error", tc.status)
			}
			if errors.Is(err, forge.ErrNotFound) {
				t.Fatalf("a %d error matched forge.ErrNotFound (%v) — a broken forge would be read as an absent file", tc.status, err)
			}
		})
	}
}

// TestMRInfoIsOneTypeAcrossPortAndAdapter pins the transitional alias: the
// adapter's MRInfo must be the SAME type as the port's, not a structural copy
// that would silently diverge field-by-field.
func TestMRInfoIsOneTypeAcrossPortAndAdapter(t *testing.T) {
	portType := reflect.TypeOf(forge.MRInfo{})
	adapterType := reflect.TypeOf(gitlab.MRInfo{})
	if portType != adapterType {
		t.Fatalf("gitlab.MRInfo (%v) is not forge.MRInfo (%v) — the alias was replaced by a duplicate struct", adapterType, portType)
	}
	if portType.PkgPath() != "github.com/PlatformRelay/assent/internal/forge" {
		t.Fatalf("MRInfo is owned by %q, want internal/forge (the type must live on the port)", portType.PkgPath())
	}
	// The lift is behaviour-preserving only if the shape survived it verbatim.
	want := []string{"IID", "ProjectID", "SourceBranch", "TargetBranch", "SourceSHA", "TargetSHA", "ForkMR"}
	var got []string
	for i := range portType.NumField() {
		got = append(got, portType.Field(i).Name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("forge.MRInfo fields = %v, want %v (the move must be verbatim)", got, want)
	}
}

// adapterTypesIn returns every named type reachable from an interface's method
// signatures that is declared in pkg. It is the executable form of the ARCH-02
// binding constraint: "no new gitlab.* type may be added to the run port".
func adapterTypesIn(iface reflect.Type, pkg string) []string {
	var found []string
	seen := map[reflect.Type]bool{}

	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		if t == nil || seen[t] {
			return
		}
		seen[t] = true
		if t.PkgPath() == pkg {
			found = append(found, t.String())
		}
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Chan:
			walk(t.Elem())
		case reflect.Map:
			walk(t.Key())
			walk(t.Elem())
		case reflect.Struct:
			for i := range t.NumField() {
				walk(t.Field(i).Type)
			}
		case reflect.Func:
			for i := range t.NumIn() {
				walk(t.In(i))
			}
			for i := range t.NumOut() {
				walk(t.Out(i))
			}
		default:
		}
	}

	for i := range iface.NumMethod() {
		walk(iface.Method(i).Type)
	}
	return found
}

// probeAdapterPort is the POSITIVE CONTROL for adapterTypesIn: a port that DOES
// leak an adapter type. Without it, "no adapter types found" could equally mean
// the walker is broken and sweeping an empty set.
type probeAdapterPort interface {
	Configure(opt gitlab.Option) error
}

// TestRunReadPortCarriesNoAdapterType (REQ-AUD-S15-01) enforces the ARCH-02
// binding constraint structurally, at both polarities.
func TestRunReadPortCarriesNoAdapterType(t *testing.T) {
	// Polarity 1 (positive control): the walker FIRES on a leaking port.
	probe := reflect.TypeOf((*probeAdapterPort)(nil)).Elem()
	leaks := adapterTypesIn(probe, gitlabPkgPath)
	if len(leaks) == 0 {
		t.Fatal("adapterTypesIn found no gitlab type in a port that deliberately exposes gitlab.Option — the detector cannot fire, so its silence proves nothing")
	}
	if want := "gitlab.Option"; leaks[0] != want {
		t.Fatalf("positive control reported %v, want the leaking type %s", leaks, want)
	}

	// Polarity 2: the real read port is adapter-free.
	port := reflect.TypeOf((*runReadPort)(nil)).Elem()
	if port.NumMethod() < 4 {
		t.Fatalf("runReadPort exposes only %d methods — the port shrank and the sweep below would be near-vacuous", port.NumMethod())
	}
	if leaks := adapterTypesIn(port, gitlabPkgPath); len(leaks) > 0 {
		t.Fatalf("the orchestration read port exposes adapter types %v — ARCH-02: no gitlab.* type may cross the port", leaks)
	}
}

// TestNeutralSentinelWrapsUnprefixed documents (and pins) the one deliberate
// deviation from this package's `forge: `-prefixed sentinel convention: the
// adapter supplies the prefix, so the port sentinel must not.
func TestNeutralSentinelWrapsUnprefixed(t *testing.T) {
	if got, want := forge.ErrNotFound.Error(), "resource not found (404)"; got != want {
		t.Fatalf("forge.ErrNotFound = %q, want %q (an adapter prefixes it; a prefix here would double up)", got, want)
	}
	wrapped := fmt.Errorf("gitlab: %w: file %q at ref %q", forge.ErrNotFound, "p", "r")
	if got, want := wrapped.Error(), `gitlab: resource not found (404): file "p" at ref "r"`; got != want {
		t.Fatalf("adapter-wrapped rendering = %q, want %q", got, want)
	}
}
