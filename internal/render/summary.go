package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/core/decision"
)

// RenderSummary renders the ADR-0012 default-theme per-MR summary comment body
// (decision headline, score vs threshold, finding index). The caller wraps the
// returned body with Envelope for forge markers (E8-S13); forge UpsertComment
// owns Envelope — do not double-wrap at buildDesired.
//
//nolint:revive // spec name RenderSummary (E8-S13) — package stutter intentional
func RenderSummary(pm decision.PresentationModel, ctx Context) (string, error) {
	var b strings.Builder

	headline, _ := Chrome(ctx.Options, "summary_headline")
	if ctx.Options.Emoji {
		headline = decisionEmoji(pm.Decision) + headline
	}
	b.WriteString("**")
	b.WriteString(EscapeAndClamp(headline))
	b.WriteString("**\n\n")

	decisionLabel, _ := Chrome(ctx.Options, "summary_decision")
	scoreLabel, _ := Chrome(ctx.Options, "summary_score")
	thresholdLabel, _ := Chrome(ctx.Options, "summary_threshold")
	score := presentationScore(pm.Findings)
	b.WriteString("**")
	b.WriteString(EscapeAndClamp(decisionLabel))
	b.WriteString(":** ")
	b.WriteString(EscapeAndClamp(pm.Decision))
	b.WriteString(" · **")
	b.WriteString(EscapeAndClamp(scoreLabel))
	b.WriteString(":** ")
	fmt.Fprintf(&b, "%d/%d", score, ctx.RiskThreshold)
	b.WriteString(" · **")
	b.WriteString(EscapeAndClamp(thresholdLabel))
	b.WriteString(":** ")
	fmt.Fprintf(&b, "%d", ctx.RiskThreshold)
	b.WriteByte('\n')

	findingsLabel, _ := Chrome(ctx.Options, "summary_findings")
	b.WriteString("\n**")
	b.WriteString(EscapeAndClamp(findingsLabel))
	b.WriteString(":**")
	if len(pm.Findings) == 0 {
		b.WriteString(" none")
		return b.String(), nil
	}
	b.WriteByte('\n')

	findings := append([]decision.Finding(nil), pm.Findings...)
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Rule != findings[j].Rule {
			return findings[i].Rule < findings[j].Rule
		}
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].Subject < findings[j].Subject
	})

	for _, f := range findings {
		b.WriteString("- ")
		if ctx.Options.Emoji {
			b.WriteString(effectEmoji(f.Effect))
		}
		b.WriteString("`")
		b.WriteString(EscapeAndClamp(f.Rule))
		b.WriteString("` · `")
		b.WriteString(EscapeAndClamp(f.Subject))
		b.WriteString("` · `")
		b.WriteString(EscapeAndClamp(f.Code))
		b.WriteString("` (+")
		fmt.Fprintf(&b, "%d", f.Points)
		b.WriteString(")\n")
	}

	return b.String(), nil
}

func presentationScore(findings []decision.Finding) int {
	sum := 0
	for _, f := range findings {
		sum += f.Points
	}
	return sum
}

func decisionEmoji(decision string) string {
	switch decision {
	case "APPROVE":
		return "✅ "
	case "BLOCK":
		return "⛔ "
	default:
		return "📋 "
	}
}
