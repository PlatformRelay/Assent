package render

// DefaultClampRunes is the D-091 per-field rune limit for forge-facing markdown.
const DefaultClampRunes = 500

// ClampEllipsis is the stable suffix appended when Clamp truncates (D-091).
const ClampEllipsis = "…"

// Clamp shortens s to at most maxRunes runes, appending ClampEllipsis when truncated.
// Values within the limit are returned unchanged.
func Clamp(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	ell := []rune(ClampEllipsis)
	if maxRunes <= len(ell) {
		return string(ell[:maxRunes])
	}
	budget := maxRunes - len(ell)
	return string(runes[:budget]) + ClampEllipsis
}
