package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

const stateResolved = "resolved"

// RedactedDisplay is the forge-facing placeholder for sensitive fact values (D-090).
const RedactedDisplay = "[redacted]"

// displayFactValue returns the semantic display string before forge escaping/clamping.
func displayFactValue(f aggregate.Fact) string {
	if f.State != stateResolved || f.Value == nil {
		return f.State
	}
	if f.Sensitive {
		return RedactedDisplay
	}
	return factValueString(f.Value)
}

// FormatFactValue returns the forge-facing display string for one fact value: redaction
// decision first, then EscapeAndClamp (D-090 / D-091).
func FormatFactValue(f aggregate.Fact) string {
	return EscapeAndClamp(displayFactValue(f))
}

// RedactFacts builds a provider→name→semantic display map for presentation. Values are
// not escaped — apply FormatFactValue or EscapeAndClamp at layout assembly. CEL activation
// retains raw values (D-068 / D-090 handoff).
func RedactFacts(facts map[string]map[string]aggregate.Fact) map[string]map[string]string {
	out := make(map[string]map[string]string, len(facts))
	for provider, byName := range facts {
		pm := make(map[string]string, len(byName))
		for name, f := range byName {
			pm[name] = displayFactValue(f)
		}
		out[provider] = pm
	}
	return out
}

// RenderFactsSection renders a minimal evaluation-details markdown block listing fact
// paths and display values. Full finding-thread layout lands in E8-S08; this API is
// the redaction seam exercised by S06 tests.
//
//nolint:revive // spec name RenderFactsSection (E8-S06) — package stutter intentional
func RenderFactsSection(facts map[string]map[string]aggregate.Fact, opts Options) string {
	title, _ := Chrome(opts, "evaluation_details")
	display := RedactFacts(facts)

	var paths []string
	for provider, byName := range facts {
		for name := range byName {
			paths = append(paths, provider+"."+name)
		}
	}
	sort.Strings(paths)

	var b strings.Builder
	b.WriteString("<details>\n<summary>")
	b.WriteString(EscapeAndClamp(title))
	b.WriteString("</summary>\n\n")
	for _, path := range paths {
		provider, name, ok := strings.Cut(path, ".")
		if !ok {
			continue
		}
		b.WriteString("- `")
		b.WriteString(EscapeAndClamp(path))
		b.WriteString("`: ")
		b.WriteString(EscapeAndClamp(display[provider][name]))
		b.WriteByte('\n')
	}
	b.WriteString("\n</details>")
	return b.String()
}

func factValueString(v any) string {
	switch x := v.(type) {
	case json.Number:
		return x.String()
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(raw)
	}
}
