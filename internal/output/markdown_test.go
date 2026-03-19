package output

import (
	"strings"
	"testing"

	"github.com/AtomicWasTaken/surge/internal/model"
)

func TestRenderSummary_ModernLayoutIncludesCoreSections(t *testing.T) {
	out := NewMarkdownOutput("SURGE")
	result := &model.ReviewResult{
		Summary: "This PR improves auth session handling.",
		FilesOverview: []model.FileOverview{
			{Path: "internal/auth/session.go", Changes: "Refactor token refresh", Risk: "medium"},
		},
		Findings: []model.Finding{
			{
				Severity:   model.SeverityHigh,
				Category:   model.CategorySecurity,
				File:       "internal/auth/session.go",
				Line:       42,
				Title:      "Refresh token can be replayed",
				Body:       "Rotate refresh tokens after use to prevent replay.",
				Suggestion: "In handleTokenRefresh(), generate a new refresh token after each use and invalidate the previous one.",
			},
		},
		VibeCheck: model.VibeCheck{
			Score:   8,
			Verdict: "Solid structure, a couple of risky auth edges.",
			Flags:   []string{"tight-coupling"},
		},
		Recommendations: []string{"Add replay protection test"},
		Approve:         false,
		Stats: model.ReviewStats{
			FilesReviewed: 3,
		},
	}

	rendered := out.RenderSummary(result)

	assertContains(t, rendered, "<!-- SURGE -->")
	assertContains(t, rendered, "## ⚡ surge")
	assertContains(t, rendered, "Changes Requested")
	assertContains(t, rendered, "### Findings")
	assertContains(t, rendered, "🟠")
	assertContains(t, rendered, "<strong>Refresh token can be replayed</strong>")
	assertContains(t, rendered, "<code>internal/auth/session.go:42</code>")
	assertContains(t, rendered, "🎯 Vibe Check")
	assertContains(t, rendered, "8/10")
	assertContains(t, rendered, "████████░░")
	assertContains(t, rendered, "Recommended next steps")
	assertContains(t, rendered, "- [ ] Add replay protection test")
	assertContains(t, rendered, "🤖 Agent fix prompt:")
	assertContains(t, rendered, "generate a new refresh token")
}

func TestRenderSummary_NoSuggestionOmitsAgentPrompt(t *testing.T) {
	out := NewMarkdownOutput("SURGE")
	result := &model.ReviewResult{
		Summary: "Minor style fixes.",
		Findings: []model.Finding{
			{
				Severity: model.SeverityLow,
				Category: model.CategoryMaintainability,
				File:     "main.go",
				Title:    "Unused import",
				Body:     "Remove unused import.",
			},
		},
		VibeCheck: model.VibeCheck{Score: 9, Verdict: "Clean"},
		Approve:   true,
		Stats:     model.ReviewStats{FilesReviewed: 1},
	}

	rendered := out.RenderSummary(result)
	if strings.Contains(rendered, "Agent fix prompt") {
		t.Fatal("expected no agent fix prompt when suggestion is empty")
	}
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected rendered markdown to contain %q", want)
	}
}
