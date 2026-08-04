package render

import "strings"

// markdownScalarReplacer neutralizes HTML and markdown link injection in untrusted
// interpolated scalars (ADR-0012 amendment, E8-S05). Order matters: & first.
var markdownScalarReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"[", "&#91;",
	"]", "&#93;",
)

// EscapeMarkdown renders a scalar safe for forge-facing markdown: raw HTML and
// markdown link syntax are neutralized so values cannot forge approvals or break
// renderer-owned <details> regions.
func EscapeMarkdown(s string) string {
	return markdownScalarReplacer.Replace(s)
}

// EscapeAndClamp applies EscapeMarkdown then Clamp at DefaultClampRunes (D-091).
// Layout assembly for interpolated scalars must use this helper (or both steps).
func EscapeAndClamp(s string) string {
	return Clamp(EscapeMarkdown(s), DefaultClampRunes)
}
