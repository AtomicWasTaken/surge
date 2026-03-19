package output

import (
	"fmt"
	"strings"

	"github.com/AtomicWasTaken/surge/internal/model"
)

// MarkdownOutput renders review results as GitHub-flavored markdown.
type MarkdownOutput struct {
	commentMarker string
}

const (
	CommentScopeSummary = "SUMMARY"
	CommentScopeInline  = "INLINE"
)

// ScopedCommentMarkers returns the scoped surge markers used across markdown output and cleanup.
func ScopedCommentMarkers(marker string) []string {
	return []string{
		ScopedCommentMarker(marker, CommentScopeSummary),
		ScopedCommentMarker(marker, CommentScopeInline),
	}
}

// CommentMarker returns the base marker used to tag surge-authored comments.
func CommentMarker(marker string) string {
	return "<!-- " + marker + " -->"
}

// ScopedCommentMarker returns a scoped marker for a specific surge comment type.
func ScopedCommentMarker(marker, scope string) string {
	return "<!-- " + marker + "_" + scope + " -->"
}

// NewMarkdownOutput creates a new markdown output renderer.
func NewMarkdownOutput(commentMarker string) *MarkdownOutput {
	return &MarkdownOutput{
		commentMarker: commentMarker,
	}
}

// RenderSummary renders the full review summary as a markdown comment.
func (m *MarkdownOutput) RenderSummary(result *model.ReviewResult) string {
	var sb strings.Builder
	bySeverity := groupBySeverity(result.Findings)
	findingCount := len(result.Findings)
	decision := "✅ Approved"
	if !result.Approve {
		decision = "❌ Changes Requested"
	}

	sb.WriteString(CommentMarker(m.commentMarker))
	sb.WriteString("\n")
	sb.WriteString(ScopedCommentMarker(m.commentMarker, CommentScopeSummary))
	sb.WriteString("\n")

	// Header with decision badge
	sb.WriteString(fmt.Sprintf("## ⚡ surge &nbsp;·&nbsp; %s\n\n", decision))

	// Compact stats line
	sb.WriteString(fmt.Sprintf("> **%d** findings across **%d** files &nbsp;·&nbsp; Vibe **%d/10** %s\n",
		findingCount, result.Stats.FilesReviewed, result.VibeCheck.Score, vibeBar(result.VibeCheck.Score)))

	// Severity badges inline
	critCount := len(bySeverity[model.SeverityCritical])
	highCount := len(bySeverity[model.SeverityHigh])
	medCount := len(bySeverity[model.SeverityMedium])
	lowCount := len(bySeverity[model.SeverityLow])
	infoCount := len(bySeverity[model.SeverityInfo])

	var badges []string
	if critCount > 0 {
		badges = append(badges, fmt.Sprintf("🔴 %d critical", critCount))
	}
	if highCount > 0 {
		badges = append(badges, fmt.Sprintf("🟠 %d high", highCount))
	}
	if medCount > 0 {
		badges = append(badges, fmt.Sprintf("🟡 %d medium", medCount))
	}
	if lowCount > 0 {
		badges = append(badges, fmt.Sprintf("🔵 %d low", lowCount))
	}
	if infoCount > 0 {
		badges = append(badges, fmt.Sprintf("⚪ %d info", infoCount))
	}
	if len(badges) > 0 {
		sb.WriteString(fmt.Sprintf("> %s\n", strings.Join(badges, " &nbsp;·&nbsp; ")))
	}
	sb.WriteString("\n")

	// Summary
	sb.WriteString(result.Summary)
	sb.WriteString("\n\n")

	if len(result.Warnings) > 0 {
		sb.WriteString("<blockquote>\n\n")
		sb.WriteString("⚠️ **Warnings**\n\n")
		for _, warning := range result.Warnings {
			sb.WriteString(fmt.Sprintf("- %s\n", warning))
		}
		sb.WriteString("\n</blockquote>\n\n")
	}

	// Files Overview (collapsed)
	if len(result.FilesOverview) > 0 {
		sb.WriteString("<details>\n")
		sb.WriteString("<summary>📁 Files changed</summary>\n\n")
		sb.WriteString("| File | Changes | Risk |\n")
		sb.WriteString("|---|---|---:|\n")
		for _, f := range result.FilesOverview {
			risk := riskEmoji(f.Risk) + " " + f.Risk
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", sanitizeTableCell(f.Path), sanitizeTableCell(f.Changes), sanitizeTableCell(risk)))
		}
		sb.WriteString("\n</details>\n\n")
	}

	// Findings by Severity
	if len(result.Findings) > 0 {
		sb.WriteString("### Findings\n\n")
		for _, sev := range []model.Severity{model.SeverityCritical, model.SeverityHigh, model.SeverityMedium, model.SeverityLow, model.SeverityInfo} {
			findings := bySeverity[sev]
			if len(findings) == 0 {
				continue
			}
			for _, f := range findings {
				location := f.File
				if f.Line > 0 {
					location = fmt.Sprintf("%s:%d", f.File, f.Line)
				}
				emoji := severityEmoji(sev)
				sb.WriteString(fmt.Sprintf("<details>\n<summary>%s <strong>%s</strong> &nbsp;<code>%s</code></summary>\n\n",
					emoji, sanitizeTableCell(f.Title), sanitizeInlineCode(location)))
				sb.WriteString(fmt.Sprintf("**Category:** `%s` &nbsp;·&nbsp; **Severity:** %s\n\n", f.Category, severityLabel(sev)))
				sb.WriteString(f.Body)
				sb.WriteString("\n")
				if f.Suggestion != "" {
					sb.WriteString("\n**🤖 Agent fix prompt:**\n")
					sb.WriteString(fmt.Sprintf("> %s\n", f.Suggestion))
				}
				sb.WriteString("\n</details>\n\n")
			}
		}
	} else {
		sb.WriteString("### Findings\n\nNo issues found — nice work.\n\n")
	}

	// Vibe Check (compact)
	sb.WriteString("<details>\n")
	sb.WriteString(fmt.Sprintf("<summary>🎯 Vibe Check &nbsp;·&nbsp; %d/10 &nbsp;—&nbsp; %s</summary>\n\n", result.VibeCheck.Score, result.VibeCheck.Verdict))
	if len(result.VibeCheck.Flags) > 0 {
		sb.WriteString("**Flags:** ")
		for i, flag := range result.VibeCheck.Flags {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("`%s`", flag))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n</details>\n\n")

	// Recommendations
	if len(result.Recommendations) > 0 {
		sb.WriteString("<details>\n")
		sb.WriteString("<summary>✅ Recommended next steps</summary>\n\n")
		for _, rec := range result.Recommendations {
			sb.WriteString(fmt.Sprintf("- [ ] %s\n", rec))
		}
		sb.WriteString("\n</details>\n\n")
	}

	// Footer
	sb.WriteString("---\n")
	sb.WriteString("<sub>Generated by <a href=\"https://github.com/AtomicWasTaken/surge\">surge</a> · AI-powered PR reviews</sub>\n")

	return sb.String()
}

func severityEmoji(s model.Severity) string {
	switch s {
	case model.SeverityCritical:
		return "🔴"
	case model.SeverityHigh:
		return "🟠"
	case model.SeverityMedium:
		return "🟡"
	case model.SeverityLow:
		return "🔵"
	default:
		return "⚪"
	}
}

func riskEmoji(r string) string {
	switch r {
	case "high":
		return "🔴"
	case "medium":
		return "🟡"
	default:
		return "🟢"
	}
}

func severityLabel(s model.Severity) string {
	switch s {
	case model.SeverityCritical:
		return "CRITICAL"
	case model.SeverityHigh:
		return "HIGH"
	case model.SeverityMedium:
		return "MEDIUM"
	case model.SeverityLow:
		return "LOW"
	default:
		return "INFO"
	}
}

func groupBySeverity(findings []model.Finding) map[model.Severity][]model.Finding {
	result := make(map[model.Severity][]model.Finding)
	for _, f := range findings {
		result[f.Severity] = append(result[f.Severity], f)
	}
	return result
}

func sanitizeTableCell(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, "|", "\\|"), "\n", "<br>")
}

func sanitizeInlineCode(v string) string {
	return strings.ReplaceAll(v, "`", "")
}

func vibeBar(score int) string {
	if score < 0 {
		score = 0
	}
	if score > 10 {
		score = 10
	}

	filled := score
	empty := 10 - filled
	return strings.Repeat("█", filled) + strings.Repeat("░", empty)
}
