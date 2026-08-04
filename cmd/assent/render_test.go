package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var renderGoldenCases = []string{"challenge", "block", "require-review"}

const renderExamplesDir = "../../examples/render"

// REQ-E8-S10-01: `assent render --finding …` stdout equals the committed golden.
func TestRenderCLI(t *testing.T) {
	for _, name := range renderGoldenCases {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(renderExamplesDir, name)
			expectRaw, err := os.ReadFile(filepath.Join(dir, "expect.finding-thread.md")) //nolint:gosec // fixture tree
			if err != nil {
				t.Fatalf("read expect: %v", err)
			}
			want := normalizeRenderCLIOutput(string(expectRaw))

			var stdout, stderr bytes.Buffer
			code := runRender([]string{"--finding", dir}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("successful render must not print to stderr, got:\n%s", stderr.String())
			}
			got := normalizeRenderCLIOutput(stdout.String())
			if got != want {
				t.Fatalf("stdout mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}

	t.Run("fixture alias undocumented", func(t *testing.T) {
		dir := filepath.Join(renderExamplesDir, "challenge")
		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--fixture", dir}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("--fixture alias exit = %d, want 0; stderr:\n%s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Resolve this thread") {
			t.Fatalf("--fixture alias must render finding-thread markdown, got:\n%s", stdout.String())
		}
	})

	t.Run("presentation-minimal omits evaluation details", func(t *testing.T) {
		dir := filepath.Join(renderExamplesDir, "challenge")
		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir, "--presentation-minimal"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr.String())
		}
		out := stdout.String()
		if strings.Contains(out, "Evaluation details") {
			t.Fatalf("minimal presentation must omit evaluation details block:\n%s", out)
		}
	})

	t.Run("summary artifact matches golden", func(t *testing.T) {
		dir := filepath.Join(renderExamplesDir, "challenge")
		expectRaw, err := os.ReadFile(filepath.Join(dir, "expect.summary.md")) //nolint:gosec // fixture tree
		if err != nil {
			t.Fatalf("read expect.summary.md: %v", err)
		}
		want := normalizeRenderCLIOutput(string(expectRaw))

		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir, "--artifact", "summary"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr.String())
		}
		got := normalizeRenderCLIOutput(stdout.String())
		if got != want {
			t.Fatalf("summary stdout mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
		}
	})

	t.Run("presentation-full keeps evaluation details", func(t *testing.T) {
		dir := filepath.Join(renderExamplesDir, "challenge")
		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir, "--presentation-full"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Evaluation details") {
			t.Fatalf("full presentation must include evaluation details:\n%s", stdout.String())
		}
	})

	t.Run("fixture equals form alias", func(t *testing.T) {
		dir := filepath.Join(renderExamplesDir, "challenge")
		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--fixture=" + dir}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("--fixture= alias exit = %d, want 0; stderr:\n%s", code, stderr.String())
		}
	})

	t.Run("unknown artifact is usage error", func(t *testing.T) {
		dir := filepath.Join(renderExamplesDir, "challenge")
		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir, "--artifact", "report"}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("unknown artifact exit = %d, want 2", code)
		}
	})

	t.Run("mutually exclusive presentation flags", func(t *testing.T) {
		dir := filepath.Join(renderExamplesDir, "challenge")
		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir, "--presentation-minimal", "--presentation-full"}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("conflicting flags exit = %d, want 2", code)
		}
	})

	t.Run("help exits usage", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runRender([]string{"-h"}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("help exit = %d, want 2", code)
		}
	})
}

// REQ-E8-S10-02: invalid fixture path exits non-zero with a located error on stderr.
func TestRenderCLIInvalidFixture(t *testing.T) {
	t.Run("missing finding flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runRender(nil, &stdout, &stderr)
		if code == 0 {
			t.Fatal("missing --finding must exit non-zero")
		}
		if !strings.Contains(stderr.String(), "assent render:") {
			t.Errorf("stderr must be prefixed with assent render:, got:\n%s", stderr.String())
		}
	})

	t.Run("nonexistent fixture directory", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", filepath.Join(renderExamplesDir, "no-such-case")}, &stdout, &stderr)
		if code == 0 {
			t.Fatal("nonexistent fixture must exit non-zero")
		}
		out := stderr.String()
		if !strings.Contains(out, "assent render:") {
			t.Errorf("stderr must be prefixed with assent render:, got:\n%s", out)
		}
		if !strings.Contains(out, "presentation-model.json") {
			t.Errorf("stderr must locate the missing fixture file, got:\n%s", out)
		}
	})

	t.Run("invalid presentation-model json", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "presentation-model.json"), "{not json")
		writeFile(t, filepath.Join(dir, "render-context.json"), "{}")

		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir}, &stdout, &stderr)
		if code == 0 {
			t.Fatal("invalid presentation-model must exit non-zero")
		}
		out := stderr.String()
		if !strings.Contains(out, "presentation-model") {
			t.Errorf("stderr must locate presentation-model error, got:\n%s", out)
		}
	})

	t.Run("missing render-context json", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "presentation-model.json"), `{"apiVersion":"assent.dev/v1alpha1","kind":"PresentationModel","decision":"BLOCK","findings":[{"subject":"x","code":"c","effect":"block","score":1}]}`)

		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir}, &stdout, &stderr)
		if code == 0 {
			t.Fatal("missing render-context must exit non-zero")
		}
		if !strings.Contains(stderr.String(), "render-context.json") {
			t.Errorf("stderr must locate render-context.json, got:\n%s", stderr.String())
		}
	})

	t.Run("presentation-model with no findings", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "presentation-model.json"), `{"apiVersion":"assent.dev/v1alpha1","kind":"PresentationModel","decision":"APPROVE","findings":[]}`)
		writeFile(t, filepath.Join(dir, "render-context.json"), "{}")

		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir}, &stdout, &stderr)
		if code == 0 {
			t.Fatal("empty findings must exit non-zero")
		}
		if !strings.Contains(stderr.String(), "no findings") {
			t.Errorf("stderr must mention missing findings, got:\n%s", stderr.String())
		}
	})

	t.Run("invalid render-context json", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "presentation-model.json"), `{"apiVersion":"assent.dev/v1alpha1","kind":"PresentationModel","decision":"REVIEW","findings":[{"rule":"r","obligation":"o","effect":"challenge","subject":"s","points":1,"code":"c"}]}`)
		writeFile(t, filepath.Join(dir, "render-context.json"), "{bad")

		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir}, &stdout, &stderr)
		if code == 0 {
			t.Fatal("invalid render-context must exit non-zero")
		}
		if !strings.Contains(stderr.String(), "render-context") {
			t.Errorf("stderr must locate render-context error, got:\n%s", stderr.String())
		}
	})

	t.Run("render failure surfaces located error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "presentation-model.json"), `{"apiVersion":"assent.dev/v1alpha1","kind":"PresentationModel","decision":"REVIEW","findings":[{"rule":"bad-rule","obligation":"o","effect":"challenge","subject":"s","points":1,"code":"c"}]}`)
		writeFile(t, filepath.Join(dir, "render-context.json"), `{"rules":{"bad-rule":{"message":"{{ no_such_field }}"}}}`)

		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--finding", dir}, &stdout, &stderr)
		if code == 0 {
			t.Fatal("render compile failure must exit non-zero")
		}
		if !strings.Contains(stderr.String(), dir) {
			t.Errorf("stderr must locate the fixture dir on render failure, got:\n%s", stderr.String())
		}
	})

	t.Run("bare fixture alias without value is usage error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runRender([]string{"--fixture"}, &stdout, &stderr)
		if code == 0 {
			t.Fatal("bare --fixture must exit non-zero")
		}
	})
}

func normalizeRenderCLIOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n") + "\n"
}
