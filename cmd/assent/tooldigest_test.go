package main

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"runtime/debug"
	"testing"
)

// AUD-S04 / D-120: pins.toolDigest must be a build-CONTENT proxy derived from Go
// build info, not sha256 of the version string. These tests are written to fail
// if the digest computation is deleted or collapsed to a constant: every
// discriminating case asserts that two DIFFERENT build inputs produce DIFFERENT
// digests, which no constant implementation can satisfy.

var digestGrammar = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// mod is a terse debug.Module constructor for table entries.
func mod(path, version, sum string) debug.Module {
	return debug.Module{Path: path, Version: version, Sum: sum}
}

// buildInfo assembles a synthetic *debug.BuildInfo. Real build info cannot be
// forged by the toolchain inside a test binary, so every branch of the digest
// (content-bearing vs. fallback) is exercised through injection.
func buildInfo(main debug.Module, deps []*debug.Module, settings ...debug.BuildSetting) *debug.BuildInfo {
	return &debug.BuildInfo{Main: main, Deps: deps, Settings: settings}
}

func vcs(revision, modified string) []debug.BuildSetting {
	return []debug.BuildSetting{
		{Key: "-buildmode", Value: "exe"},
		{Key: "GOARCH", Value: "arm64"},
		{Key: "vcs", Value: "git"},
		{Key: "vcs.revision", Value: revision},
		{Key: "vcs.time", Value: "2026-08-07T10:59:15Z"},
		{Key: "vcs.modified", Value: modified},
	}
}

// goBuildInfo mirrors what `go build` of this repo actually reports: pseudo-version
// main module with an EMPTY sum, dep sums, and VCS stamps (probed on 6bf55ba).
func goBuildInfo(revision, modified string) *debug.BuildInfo {
	return buildInfo(
		mod("github.com/PlatformRelay/assent", "v0.1.1-0.20260807105915-6bf55ba42aa7", ""),
		[]*debug.Module{
			{Path: "cel.dev/expr", Version: "v0.25.1", Sum: "h1:1KrZg61W6TWSxuNZ37Xy49ps13NUovb66QLprthtwi4="},
			{Path: "go.yaml.in/yaml/v3", Version: "v3.0.4", Sum: "h1:tfq32ie2Jv2UxXFdLJdh3jXuOzWiL1fo0bu/FbuKpbc="},
		},
		vcs(revision, modified)...,
	)
}

// goInstallInfo mirrors `go install ...@v0.1.0`: module SUM present, no VCS stamps
// (probed against the published v0.1.0).
func goInstallInfo(sum string) *debug.BuildInfo {
	return buildInfo(
		mod("github.com/PlatformRelay/assent", "v0.1.0", sum),
		[]*debug.Module{{Path: "cel.dev/expr", Version: "v0.25.1", Sum: "h1:1KrZg61W6TWSxuNZ37Xy49ps13NUovb66QLprthtwi4="}},
		debug.BuildSetting{Key: "GOARCH", Value: "arm64"},
	)
}

// testBinaryInfo mirrors what a `go test` binary reports: ok==true, but the main
// module carries NO content identity — "(devel)", empty sum, and no VCS stamps
// (probed: the toolchain does not VCS-stamp test binaries). Dep sums ARE present,
// which is exactly the trap: a naive `if !ok` fallback would hash deps only and
// emit the SAME digest for every source revision of the main module.
func testBinaryInfo() *debug.BuildInfo {
	return buildInfo(
		mod("github.com/PlatformRelay/assent", "(devel)", ""),
		[]*debug.Module{
			{Path: "cel.dev/expr", Version: "v0.25.1", Sum: "h1:1KrZg61W6TWSxuNZ37Xy49ps13NUovb66QLprthtwi4="},
			{Path: "go.yaml.in/yaml/v3", Version: "v3.0.4", Sum: "h1:tfq32ie2Jv2UxXFdLJdh3jXuOzWiL1fo0bu/FbuKpbc="},
		},
		debug.BuildSetting{Key: "-buildmode", Value: "exe"},
		debug.BuildSetting{Key: "GOARCH", Value: "arm64"},
	)
}

// TestToolDigestContract — REQ-AUD-S04-01: the digest is a build-content proxy.
// Different build content ⇒ different digest; identical build content ⇒ identical
// digest; grammar always ^sha256:[0-9a-f]{64}$.
func TestToolDigestContract(t *testing.T) {
	const version = "0.1.0"

	t.Run("grammar", func(t *testing.T) {
		for name, got := range map[string]string{
			"go build":    toolDigestFrom(goBuildInfo("aaaa1111", "false"), true, version),
			"go install":  toolDigestFrom(goInstallInfo("h1:H0+BdEsVRGoDudAWQ3POMmOlaN4+7xI8txDOwtOoEAI="), true, version),
			"test binary": toolDigestFrom(testBinaryInfo(), true, version),
			"no info":     toolDigestFrom(nil, false, version),
			"this binary": toolDigest(version),
		} {
			if !digestGrammar.MatchString(got) {
				t.Errorf("%s: toolDigest = %q, want ^sha256:[0-9a-f]{64}$", name, got)
			}
		}
	})

	// The heart of ARCH-03: two builds claiming the SAME version string but built
	// from different content must not share a digest. Each case varies exactly one
	// content-bearing input.
	t.Run("different content, same version, different digest", func(t *testing.T) {
		cases := map[string][2]*debug.BuildInfo{
			"vcs revision": {goBuildInfo("aaaa1111", "false"), goBuildInfo("bbbb2222", "false")},
			"dirty flag":   {goBuildInfo("aaaa1111", "false"), goBuildInfo("aaaa1111", "true")},
			"module sum":   {goInstallInfo("h1:aaa="), goInstallInfo("h1:bbb=")},
			"dependency sum": {
				buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{{Path: "d", Version: "v1", Sum: "h1:aaa="}}),
				buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{{Path: "d", Version: "v1", Sum: "h1:bbb="}}),
			},
			"dependency version": {
				buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{{Path: "d", Version: "v1", Sum: "h1:aaa="}}),
				buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{{Path: "d", Version: "v2", Sum: "h1:aaa="}}),
			},
			"dependency set": {
				buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{{Path: "d", Version: "v1", Sum: "h1:aaa="}}),
				buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{
					{Path: "d", Version: "v1", Sum: "h1:aaa="},
					{Path: "e", Version: "v1", Sum: "h1:eee="},
				}),
			},
			"module path": {
				buildInfo(mod("m", "v1", "h1:m="), nil),
				buildInfo(mod("n", "v1", "h1:m="), nil),
			},
			"module version": {
				buildInfo(mod("m", "v1", "h1:m="), nil),
				buildInfo(mod("m", "v2", "h1:m="), nil),
			},
			"replaced dependency": {
				buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{{
					Path: "d", Version: "v1", Sum: "h1:aaa=",
					Replace: &debug.Module{Path: "d", Version: "v1.0.1", Sum: "h1:r1="},
				}}),
				buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{{
					Path: "d", Version: "v1", Sum: "h1:aaa=",
					Replace: &debug.Module{Path: "d", Version: "v1.0.2", Sum: "h1:r2="},
				}}),
			},
		}
		for name, pair := range cases {
			a := toolDigestFrom(pair[0], true, version)
			b := toolDigestFrom(pair[1], true, version)
			if a == b {
				t.Errorf("%s: digests collide (%s) — differing build content must differ", name, a)
			}
		}
	})

	t.Run("same build info, same digest (determinism)", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			if got, want := toolDigest(version), toolDigest(version); got != want {
				t.Fatalf("toolDigest not stable within one binary: %q vs %q", got, want)
			}
		}
		if got, want := toolDigestFrom(goBuildInfo("aaaa1111", "false"), true, version),
			toolDigestFrom(goBuildInfo("aaaa1111", "false"), true, version); got != want {
			t.Fatalf("identical build info produced %q and %q", got, want)
		}
	})

	// Dependency ORDER is not build content: the toolchain sorts Deps, but the
	// digest must not depend on that promise.
	t.Run("dependency order is not content", func(t *testing.T) {
		d := &debug.Module{Path: "d", Version: "v1", Sum: "h1:aaa="}
		e := &debug.Module{Path: "e", Version: "v1", Sum: "h1:eee="}
		a := toolDigestFrom(buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{d, e}), true, version)
		b := toolDigestFrom(buildInfo(mod("m", "v1", "h1:m="), []*debug.Module{e, d}), true, version)
		if a != b {
			t.Fatalf("dep order changed the digest: %q vs %q", a, b)
		}
	})

	// The digest identifies the BUILD, not the run: the version string is not an
	// input on the content-bearing path (toolVersion is a separate pin).
	t.Run("content-bearing digest ignores the version string", func(t *testing.T) {
		a := toolDigestFrom(goBuildInfo("aaaa1111", "false"), true, "0.1.0")
		b := toolDigestFrom(goBuildInfo("aaaa1111", "false"), true, "9.9.9")
		if a != b {
			t.Fatalf("version string leaked into the content-bearing digest: %q vs %q", a, b)
		}
	})

	// Regression guard for the pre-D-120 behaviour this story removes.
	t.Run("not sha256 of the version string", func(t *testing.T) {
		legacy := sha256Prefix + sha256Hex([]byte(version))
		if got := toolDigestFrom(goBuildInfo("aaaa1111", "false"), true, version); got == legacy {
			t.Fatal("toolDigest is still sha256(version) — the ARCH-03 finding is not fixed")
		}
		if got := toolDigest(version); got == legacy {
			t.Fatal("toolDigest is still sha256(version) — the ARCH-03 finding is not fixed")
		}
	})
}

// TestToolDigestFallback — REQ-AUD-S04-02 (edge): build info that carries no
// main-module content identity yields the honestly LABELLED fallback, never a
// fabricated content claim and never a silent constant.
func TestToolDigestFallback(t *testing.T) {
	// Formula pinned by value, computed independently of the implementation:
	//   printf 'buildinfo-unavailable\n0.0.0-dev' | shasum -a 256
	const (
		devFallback  = "sha256:33495ff6935ffb66715f8367e41c447d3d088ffef6e201aca555ee75c9e1de4a"
		v123Fallback = "sha256:55d6c9c3dde3ff43d902ce303182374cf4712506c11ff07736f3ae75832f13e8"
	)

	t.Run("build info absent", func(t *testing.T) {
		if got := toolDigestFrom(nil, false, "0.0.0-dev"); got != devFallback {
			t.Errorf("toolDigestFrom(nil,false,%q) = %q, want %q", "0.0.0-dev", got, devFallback)
		}
		if got := toolDigestFrom(nil, true, "1.2.3"); got != v123Fallback {
			t.Errorf("nil build info with ok=true = %q, want the fallback %q", got, v123Fallback)
		}
	})

	// The fallback is labelled AND version-bound: it must not degrade to one
	// constant shared by every build in the world.
	t.Run("fallback is not a constant", func(t *testing.T) {
		if devFallback == v123Fallback {
			t.Fatal("fixture error")
		}
		if got := toolDigestFrom(nil, false, "1.2.3"); got != v123Fallback {
			t.Errorf("fallback did not vary with version: %q", got)
		}
	})

	// THE discriminating case: ok==true, deps carry sums, but the main module has
	// no sum and no VCS revision — i.e. a `go test` binary or `-buildvcs=false`.
	// Hashing that as if it were content would claim provenance the build info
	// cannot support, so it must take the labelled fallback instead.
	t.Run("content-free build info takes the fallback", func(t *testing.T) {
		cases := map[string]*debug.BuildInfo{
			"go test binary":     testBinaryInfo(),
			"buildvcs=false":     buildInfo(mod("m", "(devel)", ""), []*debug.Module{{Path: "d", Version: "v1", Sum: "h1:aaa="}}),
			"empty vcs.revision": buildInfo(mod("m", "(devel)", ""), nil, vcs("", "false")...),
			"no modules at all":  buildInfo(mod("", "", ""), nil),
		}
		for name, info := range cases {
			if got := toolDigestFrom(info, true, "0.0.0-dev"); got != devFallback {
				t.Errorf("%s: got %q, want the labelled fallback %q", name, got, devFallback)
			}
		}
	})

	// …and the complement: either content-bearing signal alone lifts it off the
	// fallback, so the predicate cannot be satisfied by always falling back.
	t.Run("content-bearing build info does not take the fallback", func(t *testing.T) {
		cases := map[string]*debug.BuildInfo{
			"vcs.revision only (go build)": buildInfo(mod("m", "(devel)", ""), nil, vcs("aaaa1111", "false")...),
			"module sum only (go install)": goInstallInfo("h1:H0+BdEsVRGoDudAWQ3POMmOlaN4+7xI8txDOwtOoEAI="),
			"both":                         goBuildInfo("aaaa1111", "false"),
		}
		for name, info := range cases {
			if got := toolDigestFrom(info, true, "0.0.0-dev"); got == devFallback {
				t.Errorf("%s: took the fallback despite content-bearing build info", name)
			}
		}
	})

	// Whatever this binary is, the pin is schema-legal (minLength: 1) and stable.
	t.Run("this binary emits a schema-legal pin", func(t *testing.T) {
		got := toolDigest("0.0.0-dev")
		if !digestGrammar.MatchString(got) || got == "" {
			t.Fatalf("toolDigest() = %q, want a non-empty sha256 pin", got)
		}
		if got == devFallback {
			t.Logf("this test binary has no main-module content identity; fallback in use (expected under `go test`)")
		}
	})
}

// TestRunRecordPinsToolDigest — REQ-AUD-S04-01 wiring: the digest function is not
// merely correct, it is what `assent run` actually pins. Without this, reverting
// cmd/assent/run.go's pins line to the pre-D-120 sha256(version) leaves every
// other test in this file green while ARCH-03 is unfixed.
//
// The emitted record also passed orchestrate's pre-write schema validation, so a
// pin that violated the frozen minLength could not reach the file.
func TestRunRecordPinsToolDigest(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 6\n"

	emitPath := t.TempDir() + "/record.json"
	var out bytes.Buffer
	if code := runRun(runArgs("--emit", emitPath), env("tok"), fixedClock(), &out, &out, f.factory()); code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	raw, err := os.ReadFile(emitPath) // #nosec G304 -- test-controlled temp path, not user input.
	if err != nil {
		t.Fatalf("read emitted record: %v", err)
	}
	var rec struct {
		Pins struct {
			ToolVersion string `json:"toolVersion"`
			ToolDigest  string `json:"toolDigest"`
		} `json:"pins"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("parse emitted record: %v", err)
	}

	if want := toolDigest(rec.Pins.ToolVersion); rec.Pins.ToolDigest != want {
		t.Errorf("pins.toolDigest = %q, want the build-info digest %q — run.go is not wired to toolDigest()",
			rec.Pins.ToolDigest, want)
	}
	if legacy := sha256Prefix + sha256Hex([]byte(rec.Pins.ToolVersion)); rec.Pins.ToolDigest == legacy {
		t.Errorf("pins.toolDigest is still sha256(toolVersion) — ARCH-03 unfixed on the run path")
	}
	if !digestGrammar.MatchString(rec.Pins.ToolDigest) {
		t.Errorf("pins.toolDigest = %q, want ^sha256:[0-9a-f]{64}$", rec.Pins.ToolDigest)
	}
}
