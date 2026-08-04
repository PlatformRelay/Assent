package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/decision"
)

// RenderFindingThread renders one finding's ADR-0012 default-theme forge thread
// body (headline, resolve CTA, collapsible docs + evaluation details). The caller
// wraps the returned body with Envelope for forge markers (E8-S04).
//
//nolint:revive // spec name RenderFindingThread (E8-S08) — package stutter intentional
func RenderFindingThread(pm decision.PresentationModel, finding decision.Finding, ctx Context) (string, error) {
	var b strings.Builder

	headline, err := renderHeadline(finding, ctx)
	if err != nil {
		return "", err
	}
	b.WriteString(headline)
	b.WriteString("\n\n")

	resolve, _ := Chrome(ctx.Options, "resolve_thread")
	b.WriteString(EscapeAndClamp(resolve))
	b.WriteString(" `")
	b.WriteString(EscapeAndClamp(finding.Subject))
	b.WriteString("` · rule `")
	b.WriteString(EscapeAndClamp(finding.Rule))
	b.WriteString("`")

	if docs := renderDocsSection(finding, ctx); docs != "" {
		b.WriteString("\n\n")
		b.WriteString(docs)
	}

	if ctx.Options.Verbosity != "minimal" {
		if details := renderEvaluationDetails(pm, finding, ctx); details != "" {
			b.WriteString("\n\n")
			b.WriteString(details)
		}
	}

	return b.String(), nil
}

func renderHeadline(finding decision.Finding, ctx Context) (string, error) {
	text, err := ruleHeadlineText(finding, ctx)
	if err != nil {
		return "", err
	}
	if ctx.Options.Emoji {
		text = effectEmoji(finding.Effect) + text
	}
	return "**" + EscapeAndClamp(text) + "**", nil
}

func ruleHeadlineText(finding decision.Finding, ctx Context) (string, error) {
	meta, ok := ctx.Rules[finding.Rule]
	if ok && strings.TrimSpace(meta.Message) != "" {
		return EvalMessage(meta.Message, ctx)
	}
	if finding.Code != "" {
		return fmt.Sprintf("Review required: %s (%s)", finding.Rule, finding.Code), nil
	}
	return fmt.Sprintf("Review required: %s", finding.Rule), nil
}

func effectEmoji(effect string) string {
	switch effect {
	case "block", "challenge":
		return "⚠️ "
	case "require-review":
		return "👀 "
	default:
		return ""
	}
}

func renderDocsSection(finding decision.Finding, ctx Context) string {
	meta, ok := ctx.Rules[finding.Rule]
	if !ok {
		return ""
	}
	summary, err := EvalDocsSummary(finding.Rule, ctx)
	if err != nil {
		summary = ""
	}
	summary = strings.TrimSpace(summary)
	url := strings.TrimSpace(meta.Docs.URL)
	if summary == "" && url == "" {
		return ""
	}

	whyTitle, _ := Chrome(ctx.Options, "why_this_check")

	var b strings.Builder
	b.WriteString("<details>\n<summary>")
	b.WriteString(EscapeAndClamp(whyTitle))
	b.WriteString("</summary>\n\n")
	if summary != "" {
		b.WriteString(EscapeAndClamp(summary))
		b.WriteByte('\n')
	}
	if url != "" {
		linkLabel, _ := Chrome(ctx.Options, "full_documentation")
		b.WriteString("📖 [")
		b.WriteString(EscapeAndClamp(linkLabel))
		b.WriteString("](")
		b.WriteString(EscapeAndClamp(url))
		b.WriteString(")\n")
	}
	b.WriteString("\n</details>")
	return b.String()
}

func renderEvaluationDetails(pm decision.PresentationModel, finding decision.Finding, ctx Context) string {
	title, _ := Chrome(ctx.Options, "evaluation_details")
	var lines []string

	if meta, ok := ctx.Rules[finding.Rule]; ok {
		for i := range meta.Debug {
			line, err := EvalDebugLine(finding.Rule, i, ctx)
			if err != nil {
				continue
			}
			line = strings.TrimSpace(line)
			if line != "" {
				lines = append(lines, line)
			}
		}
	}
	lines = append(lines, defaultEvaluationLines(finding, ctx)...)
	if collapsed := collapsedPathsLine(pm, finding, ctx); collapsed != "" {
		lines = append(lines, collapsed)
	}
	if len(lines) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<details>\n<summary>")
	b.WriteString(EscapeAndClamp(title))
	b.WriteString("</summary>\n\n")
	for _, line := range lines {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n</details>")
	return b.String()
}

func defaultEvaluationLines(finding decision.Finding, ctx Context) []string {
	var out []string
	if matched := matchedChangeLine(ctx); matched != "" {
		label, _ := Chrome(ctx.Options, "matched_change")
		out = append(out, EscapeAndClamp(label)+": "+matched)
	}
	if facts := factsUsedLine(ctx); facts != "" {
		label, _ := Chrome(ctx.Options, "facts_used")
		out = append(out, EscapeAndClamp(label)+": "+facts)
	}
	label, _ := Chrome(ctx.Options, "score_contribution")
	out = append(out, fmt.Sprintf("%s: +%d · rule `%s` · code `%s`",
		EscapeAndClamp(label), finding.Points,
		EscapeAndClamp(finding.Rule), EscapeAndClamp(finding.Code)))
	return out
}

func matchedChangeLine(ctx Context) string {
	act, ok := ctx.Activation.(map[string]any)
	if !ok {
		return ""
	}
	path, _ := act["path"].(string)
	kind, _ := act["kind"].(string)
	if path == "" && kind == "" {
		return ""
	}
	old := formatActivationScalar(act["old"])
	newV := formatActivationScalar(act["new"])
	if path != "" {
		return fmt.Sprintf("`%s` %s `%s` -> `%s`",
			EscapeAndClamp(path), EscapeAndClamp(kind), old, newV)
	}
	return fmt.Sprintf("%s `%s` -> `%s`", EscapeAndClamp(kind), old, newV)
}

func factsUsedLine(ctx Context) string {
	act, ok := ctx.Activation.(map[string]any)
	if !ok {
		return ""
	}
	rawFacts, ok := act["facts"].(map[string]any)
	if !ok || len(rawFacts) == 0 {
		return ""
	}
	var paths []string
	for provider, byName := range rawFacts {
		names, ok := byName.(map[string]any)
		if !ok {
			continue
		}
		for name := range names {
			paths = append(paths, provider+"."+name)
		}
	}
	sort.Strings(paths)
	var parts []string
	for _, path := range paths {
		provider, name, ok := strings.Cut(path, ".")
		if !ok {
			continue
		}
		byProvider, ok := rawFacts[provider].(map[string]any)
		if !ok {
			continue
		}
		val := formatFactActivationValue(byProvider[name])
		parts = append(parts, fmt.Sprintf("`%s`=%s", EscapeAndClamp(path), val))
	}
	return strings.Join(parts, ", ")
}

func formatFactActivationValue(v any) string {
	if fm, ok := v.(map[string]any); ok {
		if _, hasState := fm["state"]; hasState {
			f := aggregate.Fact{State: fmt.Sprint(fm["state"])}
			if s, ok := fm["sensitive"].(bool); ok {
				f.Sensitive = s
			}
			if val, ok := fm["value"]; ok {
				f.Value = val
			}
			return EscapeAndClamp(displayFactValue(f))
		}
	}
	return EscapeAndClamp(formatMessageScalar(v))
}

func formatActivationScalar(v any) string {
	if v == nil {
		return "∅"
	}
	return EscapeAndClamp(formatMessageScalar(v))
}

func collapsedPathsLine(pm decision.PresentationModel, finding decision.Finding, ctx Context) string {
	if ctx.Options.CollapseThreshold < 1 {
		return ""
	}
	var sameCode []string
	for _, f := range pm.Findings {
		if f.Code == finding.Code && f.Rule == finding.Rule {
			sameCode = append(sameCode, f.Subject)
		}
	}
	sort.Strings(sameCode)
	if len(sameCode) <= ctx.Options.CollapseThreshold {
		return ""
	}
	hidden := sameCode[ctx.Options.CollapseThreshold:]
	label, _ := Chrome(ctx.Options, "collapsed_paths")
	return fmt.Sprintf("%s (%d): %s", EscapeAndClamp(label), len(hidden), EscapeAndClamp(strings.Join(hidden, ", ")))
}
