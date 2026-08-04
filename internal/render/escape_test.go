package render

import (
	"strings"
	"testing"
)

// TestEscapeMarkdownAdversarial (REQ-E8-S05-01) proves untrusted scalar values cannot
// inject HTML, break <details> layout, or forge markdown links (ADR-0012 amendment).
func TestEscapeMarkdownAdversarial(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{name: "script tag", input: `<script>alert('xss')</script>`},
		{name: "details block", input: `<details open><summary>hide</summary>secret</details>`},
		{name: "markdown link", input: `[APPROVED ✅](https://evil.example/forge-approve)`},
		{name: "javascript link", input: `[click me](javascript:alert(1))`},
		{name: "autolink angle", input: `<https://evil.example/>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := EscapeMarkdown(tc.input)
			if strings.Contains(got, "<script") || strings.Contains(got, "<details") {
				t.Fatalf("raw HTML leaked in %q", got)
			}
			if strings.Contains(got, "<") || strings.Contains(got, ">") {
				t.Fatalf("unescaped angle brackets in %q", got)
			}
			// Markdown link syntax must not survive intact.
			if strings.Contains(got, "](") {
				t.Fatalf("markdown link injection survived in %q", got)
			}
			if strings.Contains(got, "[") && !strings.Contains(got, `\`) {
				t.Fatalf("unescaped markdown bracket in %q", got)
			}
		})
	}
}

func TestEscapeMarkdownPreservesPlainText(t *testing.T) {
	t.Parallel()
	input := "partitions: 12 → 6 (topic orders.yaml)"
	got := EscapeMarkdown(input)
	if got != input {
		t.Fatalf("plain text changed:\n got %q\nwant %q", got, input)
	}
}

func TestEscapeAndClamp(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", DefaultClampRunes+10)
	got := EscapeAndClamp(long)
	if len([]rune(got)) > DefaultClampRunes {
		t.Fatalf("EscapeAndClamp over limit: %d runes", len([]rune(got)))
	}
	if !strings.HasSuffix(got, ClampEllipsis) {
		t.Fatalf("want ellipsis suffix, got %q", got)
	}
	injected := EscapeAndClamp(`<script>[pwn](javascript:1)</script>`)
	if strings.Contains(injected, "<") || strings.Contains(injected, "](") {
		t.Fatalf("EscapeAndClamp left injection vectors in %q", injected)
	}
}
