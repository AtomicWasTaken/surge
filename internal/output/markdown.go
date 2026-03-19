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
	} else {
		sb.WriteString("> ⚪ 0 findings\n")
	}
	sb.WriteString("\n")

	// Summary
	sb.WriteString(SanitizeHTML(result.Summary))
	sb.WriteString("\n\n")

	if len(result.Warnings) > 0 {
		sb.WriteString("<blockquote>\n\n")
		sb.WriteString("⚠️ **Warnings**\n\n")
		for _, warning := range result.Warnings {
			sb.WriteString(fmt.Sprintf("- %s\n", SanitizeHTML(warning)))
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
				emoji := SeverityEmoji(sev)
				sb.WriteString(fmt.Sprintf("<details>\n<summary>%s <strong>%s</strong> &nbsp;<code>%s</code></summary>\n\n",
					emoji, sanitizeTableCell(f.Title), sanitizeInlineCode(location)))
				sb.WriteString(fmt.Sprintf("**Category:** `%s` &nbsp;·&nbsp; **Severity:** %s\n\n", f.Category, severityLabel(sev)))
				sb.WriteString(SanitizeHTML(f.Body))
				sb.WriteString("\n")
				if suggestion := RenderAgentSuggestion(f.Suggestion); suggestion != "" {
					sb.WriteString("\n")
					sb.WriteString(suggestion)
					sb.WriteString("\n")
				}
				sb.WriteString("\n</details>\n\n")
			}
		}
	} else {
		sb.WriteString("### Findings\n\nNo issues found — nice work.\n\n")
	}

	// Vibe Check (compact)
	sb.WriteString("<details>\n")
	sb.WriteString(fmt.Sprintf("<summary>🎯 Vibe Check &nbsp;·&nbsp; %d/10 &nbsp;—&nbsp; %s</summary>\n\n", result.VibeCheck.Score, SanitizeHTML(result.VibeCheck.Verdict)))
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
			sb.WriteString(fmt.Sprintf("- [ ] %s\n", SanitizeHTML(rec)))
		}
		sb.WriteString("\n</details>\n\n")
	}

	// Footer
	sb.WriteString("---\n")
	sb.WriteString("<sub>Generated by <a href=\"https://github.com/AtomicWasTaken/surge\">surge</a> · AI-powered PR reviews</sub>\n")

	return sb.String()
}

// SeverityEmoji returns the colored circle emoji for a severity level.
func SeverityEmoji(s model.Severity) string {
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

// Markdown sanitization helpers — each targets a specific rendering context.
// Use the appropriate helper for the output context to ensure consistent escaping.

// sanitizeTableCell escapes pipe characters, HTML tags, and collapses newlines for table cells.
func sanitizeTableCell(v string) string {
	v = SanitizeHTML(v)
	v = strings.ReplaceAll(v, "|", "\\|")
	v = strings.ReplaceAll(v, "\n", "<br>")
	return v
}

// sanitizeInlineCode strips backticks to prevent breaking out of <code> spans.
func sanitizeInlineCode(v string) string {
	return strings.ReplaceAll(v, "`", "")
}

// SanitizeBlockquote escapes markdown/HTML in text intended for blockquote rendering.
// It collapses newlines into spaces so the blockquote stays on a single line,
// escapes HTML tags, and removes leading > characters that could nest quotes.
func SanitizeBlockquote(v string) string {
	v = strings.ReplaceAll(v, "\n", " ")
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "<", "&lt;")
	v = strings.ReplaceAll(v, ">", "&gt;")
	return strings.TrimSpace(v)
}

// SanitizeHTML escapes raw HTML tags in model-generated text while preserving
// markdown formatting (newlines, backticks, etc.). Use this for free-text fields
// like summary, body, verdict, and warnings that render as markdown paragraphs.
func SanitizeHTML(v string) string {
	v = strings.ReplaceAll(v, "<", "&lt;")
	v = strings.ReplaceAll(v, ">", "&gt;")
	return v
}

// RenderAgentSuggestion formats a finding suggestion as a markdown agent fix prompt block.
// The suggestion is rendered inside a fenced code block to display as literal text,
// preventing any markdown/HTML interpretation. Returns an empty string if the suggestion
// is empty. Callers are responsible for surrounding whitespace/newlines.
func RenderAgentSuggestion(suggestion string) string {
	if suggestion == "" {
		return ""
	}
	// Use a fenced code block so suggestion text is rendered literally,
	// immune to markdown metacharacters, HTML, and blockquote nesting.
	sanitized := strings.ReplaceAll(suggestion, "```", "` ` `")
	return fmt.Sprintf("**🤖 Agent fix prompt:**\n```text\n%s\n```", sanitized)
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
