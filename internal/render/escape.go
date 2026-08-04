package render

import "strings"

// EscapeMarkdown renders a scalar safe for forge-facing markdown: raw HTML and
// markdown link syntax are neutralized so values cannot forge approvals or break
// renderer-owned <details> regions (ADR-0012 amendment, E8-S05).
//
// Order is load-bearing: (1) backslash-escape markdown specials that enable
// links, emphasis, or headings; (2) HTML-entity-escape &, <, >. Numeric-entity
// encoding of brackets is insufficient — CommonMark/GitLab decode entities before
// inline parsing, reviving [text](url) links.
func EscapeMarkdown(s string) string {
	return htmlEscapeScalars(backslashEscapeMarkdown(s))
}

// backslashEscapeMarkdown prefixes CommonMark special runes so untrusted scalars
// cannot form inline links, code spans, or ATX headings after a later decode pass.
func backslashEscapeMarkdown(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	atLineStart := true
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
			atLineStart = false
		case '[', ']', '(', ')', '`':
			b.WriteRune('\\')
			b.WriteRune(r)
			atLineStart = false
		case '#':
			if atLineStart {
				b.WriteRune('\\')
			}
			b.WriteRune(r)
			atLineStart = false
		case '\n', '\r':
			b.WriteRune(r)
			atLineStart = true
		default:
			b.WriteRune(r)
			atLineStart = false
		}
	}
	return b.String()
}

func htmlEscapeScalars(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(s)
}

// EscapeAndClamp applies EscapeMarkdown then Clamp at DefaultClampRunes (D-091).
// Layout assembly for interpolated scalars must use this helper (or both steps).
func EscapeAndClamp(s string) string {
	return Clamp(EscapeMarkdown(s), DefaultClampRunes)
}
