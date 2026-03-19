package review

import (
	"testing"

	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestOutputParser_ParseValidJSON(t *testing.T) {
	p := NewOutputParser()

	json := `{
  "summary": "This PR adds user authentication using JWT tokens.",
  "filesOverview": [{"path": "auth.go", "changes": "added JWT handling", "risk": "high"}],
  "findings": [
    {
      "severity": "critical",
      "category": "security",
      "file": "auth/credentials.go",
      "line": 15,
      "title": "Hardcoded API key",
      "body": "API keys must be loaded from environment variables."
    }
  ],
  "vibeCheck": {
    "score": 7,
    "verdict": "Good with some issues",
    "flags": ["over_engineered"]
  },
  "recommendations": ["Move API key to env var"],
  "approve": false
}`

	result, err := p.Parse(json)
	assert.NoError(t, err)
	assert.Contains(t, result.Summary, "JWT tokens")
	assert.Len(t, result.Findings, 1)
	assert.Equal(t, model.SeverityCritical, result.Findings[0].Severity)
	assert.Equal(t, 7, result.VibeCheck.Score)
	assert.False(t, result.Approve)
}

func TestOutputParser_ParseWithCodeFences(t *testing.T) {
	p := NewOutputParser()

	json := "```json\n{\"summary\": \"Test summary\", \"filesOverview\": [], \"findings\": [], \"vibeCheck\": {\"score\": 5, \"verdict\": \"ok\", \"flags\": []}, \"recommendations\": [], \"approve\": true}\n```"

	result, err := p.Parse(json)
	assert.NoError(t, err)
	assert.Equal(t, "Test summary", result.Summary)
	assert.True(t, result.Approve)
}

func TestOutputParser_ParseInvalid(t *testing.T) {
	p := NewOutputParser()

	_, err := p.Parse("not json at all")
	assert.Error(t, err)
}

func TestVibeDetector_Detect(t *testing.T) {
	d := NewVibeDetector()

	result := &model.ReviewResult{
		Summary: "This PR adds authentication. Looks good, well done!",
		VibeCheck: model.VibeCheck{
			Score:   10,
			Verdict: "Perfect",
			Flags:   []string{},
		},
		Findings: []model.Finding{
			{Category: model.CategoryVibe, Title: "over_engineered"},
			{Category: model.CategoryVibe, Title: "over_engineered"},
		},
	}

	d.Detect(result, result.Summary)

	// Score should be reduced due to "well done" phrase and over-engineering
	assert.Less(t, result.VibeCheck.Score, 10)
	assert.Contains(t, result.VibeCheck.Flags, "ai_fluff")
}

func TestVibeDetector_NoDeductionForCleanCode(t *testing.T) {
	d := NewVibeDetector()

	result := &model.ReviewResult{
		Summary: "Clean, focused PR that adds a single helper function.",
		VibeCheck: model.VibeCheck{
			Score:   9,
			Verdict: "Good",
			Flags:   []string{},
		},
		Findings: []model.Finding{},
	}

	d.Detect(result, result.Summary)

	// Score should stay the same or only change slightly
	assert.GreaterOrEqual(t, result.VibeCheck.Score, 8)
}

func TestPromptBuilder_SystemPrompt(t *testing.T) {
	pb := NewPromptBuilder()
	prompt := pb.SystemPrompt()

	assert.Contains(t, prompt, "surge")
	assert.Contains(t, prompt, "CRITICAL")
	assert.Contains(t, prompt, "vibe")
	assert.Contains(t, prompt, "10: Perfect")
	assert.Contains(t, prompt, "generic_boilerplate")
	assert.Contains(t, prompt, "over_engineered")
	assert.Contains(t, prompt, "context_blind")
}

func TestPromptBuilder_SystemPromptForCategories(t *testing.T) {
	pb := NewPromptBuilder()
	prompt := pb.SystemPromptForCategories([]model.Category{model.CategorySecurity, model.CategoryLogic})

	assert.Contains(t, prompt, "- security:")
	assert.Contains(t, prompt, "- logic:")
	assert.NotContains(t, prompt, "- performance:")
	assert.NotContains(t, prompt, "- maintainability:")
	assert.NotContains(t, prompt, "- vibe:")
	assert.Contains(t, prompt, "Only emit findings whose category is in the enabled category list above.")
}

func TestPromptBuilder_SystemPromptForCategoriesEmptySet(t *testing.T) {
	pb := NewPromptBuilder()
	prompt := pb.SystemPromptForCategories([]model.Category{})

	assert.Contains(t, prompt, "- none: No review categories are enabled for this run.")
	assert.NotContains(t, prompt, "- security:")
	assert.NotContains(t, prompt, "- performance:")
	assert.NotContains(t, prompt, "- logic:")
	assert.NotContains(t, prompt, "- maintainability:")
	assert.NotContains(t, prompt, "- vibe:")
}

func TestPromptBuilderFormatCategoryDefinitionsEmptySet(t *testing.T) {
	pb := NewPromptBuilder()
	assert.Equal(t, "- none: No review categories are enabled for this run.", pb.formatCategoryDefinitions([]model.Category{}))
}

func TestPromptBuilderSystemPromptForCategoriesNilUsesDefaults(t *testing.T) {
	pb := NewPromptBuilder()
	prompt := pb.SystemPromptForCategories(nil)

	assert.Contains(t, prompt, "- security:")
	assert.Contains(t, prompt, "- performance:")
	assert.Contains(t, prompt, "- logic:")
	assert.Contains(t, prompt, "- maintainability:")
	assert.Contains(t, prompt, "- vibe:")
	assert.NotContains(t, prompt, "- none:")
}

func TestPromptBuilder_BuildUserPrompt(t *testing.T) {
	pb := NewPromptBuilder()

	prCtx := &PRContext{
		Title:        "Add user authentication",
		Body:         "Implements JWT-based auth",
		ChangedFiles: 2,
		Files: []FileContext{
			{Path: "auth.go", Status: "modified", Additions: 10, Deletions: 2, Patch: "+ JWT auth"},
			{Path: "main.go", Status: "modified", Additions: 5, Deletions: 1, Patch: "+ call auth"},
		},
	}

	prompt := pb.BuildUserPrompt(prCtx, ContextDepthDiffOnly)

	assert.Contains(t, prompt, "Add user authentication")
	assert.Contains(t, prompt, "JWT-based auth")
	assert.Contains(t, prompt, "auth.go")
	assert.Contains(t, prompt, "main.go")
	assert.Contains(t, prompt, "Diff:")
	assert.Contains(t, prompt, "+ JWT auth")
}

func TestPromptBuilder_BuildUserPrompt_RelevantContext(t *testing.T) {
	pb := NewPromptBuilder()

	prCtx := &PRContext{
		Title:        "Refactor database layer",
		ChangedFiles: 1,
		Files: []FileContext{
			{
				Path:    "db.go",
				Status:  "modified",
				Patch:   "@@ -1,3 +1,4 @@",
				Content: "full file content here",
			},
		},
	}

	prompt := pb.BuildUserPrompt(prCtx, ContextDepthRelevant)

	assert.Contains(t, prompt, "=== FILE: db.go ===")
	assert.Contains(t, prompt, "--- Diff ---")
	// For relevant context, full content is included for "full" depth only
	assert.NotContains(t, prompt, "--- Full file content ---")
}
