package render

import (
	"html"
	"regexp"
	"strings"
	"testing"
)

// formsInlineLink reports whether s contains an unescaped CommonMark inline link
// [text](url) after a naive scan (mirrors post-entity-decode inline parsing).
func formsInlineLink(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '[' || (i > 0 && s[i-1] == '\\') {
			continue
		}
		j := i + 1
		for j < len(s) && s[j] != ']' {
			if s[j] == '\\' {
				j += 2
				continue
			}
			j++
		}
		if j >= len(s) || s[j] != ']' || (j > 0 && s[j-1] == '\\') {
			continue
		}
		if j+1 < len(s) && s[j+1] == '(' {
			return true
		}
	}
	return false
}

var backslashBracketRE = regexp.MustCompile(`\\[\[\]]`)

// assertRenderInert proves escaped output stays link-inert even after a forge-style
// HTML entity decode pass (GitLab/CommonMark decode &#…; before inline parsing).
func assertRenderInert(t *testing.T, input, escaped string) {
	t.Helper()
	if strings.Contains(escaped, "<script") || strings.Contains(escaped, "<details") {
		t.Fatalf("raw HTML leaked in %q", escaped)
	}
	if strings.Contains(escaped, "<") || strings.Contains(escaped, ">") {
		t.Fatalf("unescaped angle brackets in %q", escaped)
	}
	decoded := html.UnescapeString(escaped)
	if formsInlineLink(decoded) {
		t.Fatalf("forms inline link after entity decode:\n input=%q\n escaped=%q\n decoded=%q",
			input, escaped, decoded)
	}
	// Bracket-bearing inputs must use backslash escapes, not numeric entities.
	if strings.ContainsAny(input, "[]") && !backslashBracketRE.MatchString(escaped) {
		t.Fatalf("want \\[ / \\] backslash escapes, got %q", escaped)
	}
}

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
		{name: "numeric entity bypass", input: `&#91;APPROVED&#93;(https://evil.example/)`},
		{name: "ATX heading", input: "# APPROVED — merge now"},
		{name: "code span", input: "`rm -rf /`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRenderInert(t, tc.input, EscapeMarkdown(tc.input))
		})
	}
}

// TestEscapeMarkdownMutation proves adversarial inputs form links without escaping.
func TestEscapeMarkdownMutation(t *testing.T) {
	t.Parallel()
	input := `[APPROVED](https://evil.example/forge-approve)`
	if !formsInlineLink(input) {
		t.Fatal("test setup: raw input must form inline link")
	}
	if !formsInlineLink(html.UnescapeString(input)) {
		t.Fatal("test setup: entity-decoded raw input must still form link")
	}
	escaped := EscapeMarkdown(input)
	assertRenderInert(t, input, escaped)
	// Without EscapeMarkdown the same scalar is injectable.
	if !formsInlineLink(html.UnescapeString(input)) {
		t.Fatal("mutation guard: unescaped input must form link")
	}
}

func TestEscapeMarkdownPreservesPlainText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{input: "partitions: 12 → 6 (topic orders.yaml)", want: `partitions: 12 → 6 \(topic orders.yaml\)`},
		{input: "retention unchanged", want: "retention unchanged"},
		{input: "mid #hashtag ok", want: "mid #hashtag ok"},
	}
	for _, tc := range cases {
		got := EscapeMarkdown(tc.input)
		if got != tc.want {
			t.Fatalf("EscapeMarkdown(%q) = %q, want %q", tc.input, got, tc.want)
		}
		decoded := html.UnescapeString(got)
		if formsInlineLink(decoded) {
			t.Fatalf("plain scalar forms link after decode: %q", decoded)
		}
	}
}

func TestEscapeMarkdownLineStartHeading(t *testing.T) {
	t.Parallel()
	got := EscapeMarkdown("# forged headline")
	if !strings.HasPrefix(got, `\#`) {
		t.Fatalf("line-start # must be backslash-escaped, got %q", got)
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
	assertRenderInert(t, `<script>[pwn](javascript:1)</script>`, injected)
}
