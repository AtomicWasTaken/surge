package review

import (
	"fmt"
	"strings"

	"github.com/AtomicWasTaken/surge/internal/model"
)

// ContextDepth controls how much codebase context the AI receives.
type ContextDepth string

const (
	ContextDepthDiffOnly ContextDepth = "diff-only"
	ContextDepthRelevant ContextDepth = "relevant"
	ContextDepthFull     ContextDepth = "full"
)

// PRContext holds the context needed to build a review prompt.
type PRContext struct {
	Title        string
	Body         string
	ChangedFiles int
	Files        []FileContext
}

// FileContext holds context for a single changed file.
type FileContext struct {
	Path      string
	Status    string
	Additions int
	Deletions int
	Patch     string
	Content   string // full file content (for relevant/full context depth)
}

// PromptBuilder constructs prompts for the AI code reviewer.
type PromptBuilder struct{}

var categoryDefinitions = []struct {
	name        model.Category
	description string
}{
	{name: model.CategorySecurity, description: "Vulnerabilities, auth issues, data exposure, injection risks"},
	{name: model.CategoryPerformance, description: "N+1 queries, missing indexes, inefficient algorithms, memory leaks"},
	{name: model.CategoryLogic, description: "Off-by-one errors, race conditions, incorrect business logic"},
	{name: model.CategoryMaintainability, description: "Code duplication, complex functions, poor naming, missing tests"},
	{name: model.CategoryVibe, description: "Generic AI-generated patterns, context blindness, over-engineering, wrong/confused outputs"},
}

// NewPromptBuilder creates a new prompt builder.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// SystemPrompt returns the system prompt for the AI reviewer.
func (pb *PromptBuilder) SystemPrompt() string {
	return pb.SystemPromptForCategories(nil)
}

// SystemPromptForCategories returns the system prompt scoped to the configured categories.
func (pb *PromptBuilder) SystemPromptForCategories(categories []model.Category) string {
	enabledCategories := categories
	if len(enabledCategories) == 0 {
		enabledCategories = enabledReviewCategories()
	}

	return `You are surge, an expert AI-powered code reviewer. You analyze pull request
diffs and provide structured, actionable feedback. You are direct, specific,
and avoid generic advice. You never say "looks good" without specifics.

CRITICAL: You MUST respond with a single valid JSON object. Do not include any
text before or after the JSON. The JSON schema is:

{
  "summary": "2-3 paragraph executive summary of the PR - what changed, key risks, overall quality",
  "filesOverview": [{ "path": "relative/path", "changes": "brief description of what changed", "risk": "low|medium|high" }],
  "findings": [
    {
      "severity": "critical|high|medium|low|info",
      "category": "security|performance|logic|maintainability|vibe",
      "file": "relative/path/to/file.ext",
      "line": 42,
      "title": "Brief finding title",
      "body": "Detailed explanation (markdown supported, 1-3 sentences)"
    }
  ],
  "vibeCheck": {
    "score": 7,
    "verdict": "short one-line vibe assessment",
    "flags": ["over_engineered", "generic_boilerplate"]
  },
  "recommendations": ["prioritized list of actionable next steps"],
  "approve": true
}

SEVERITY DEFINITIONS:
- CRITICAL: SQL injection, remote code execution, credential exposure, data loss risk, security vulnerabilities
- HIGH: Auth bypass, injection, missing validation, resource exhaustion, major performance issues
- MEDIUM: Error handling gaps, missing tests, unclear naming, suboptimal algorithms
- LOW: Style inconsistencies, minor duplication, missing comments, cosmetic issues
- INFO: Suggestions, tips, improvements, optimizations

CATEGORIES:
` + pb.formatCategoryDefinitions(enabledCategories) + `

VIBE CODABILITY SCORING:
- 10: Perfect. Code that feels hand-crafted, idiomatic, project-aware.
- 8-9: Good. Minor issues. Code works and follows conventions.
- 6-7: Acceptable. Some generic patterns or slight over-engineering.
- 4-5: Concerning. AI fingerprints visible. Over-abstracted or context-blind.
- 1-3: Bad. Confused output, wrong approach, completely generic.

VIBE FLAGS TO USE IN THE flags ARRAY:
- generic_boilerplate: Uses generic try-catch-wrapper patterns with no specific error handling
- over_engineered: Introduces unnecessary abstraction layers, factories, or interfaces for simple code
- context_blind: Makes changes that ignore existing patterns, naming conventions, or architectural decisions
- wrong_approach: Uses a technically correct but idiomatically wrong approach for the language/framework
- inconsistent_naming: Introduces naming that conflicts with existing conventions
- confused_about_context: References things that don't exist in the codebase or misunderstands the domain
- ai_fluff: Contains generic praise ("great job", "well done") or vague suggestions
- shotgun_approaches: Suggests multiple alternative implementations without clear rationale
- missing_tests: No test coverage mentioned for logic changes
- magic_numbers: Introduces unexplained constants or configuration without justification

RULES:
- Focus on findings that matter. Don't flag style issues as high severity.
- For each finding, include the specific file path and line number if possible.
- If the PR is good, approve=true with a brief note. If there are issues, approve=false.
- Always include a vibe check. Every codebase deserves a vibe assessment.
- Only emit findings whose category is in the enabled category list above.
- JSON must be valid. No trailing commas. No comments in the JSON.`
}

// BuildUserPrompt constructs the user prompt for a given PR review.
func (pb *PromptBuilder) BuildUserPrompt(pr *PRContext, depth ContextDepth) string {
	var sb strings.Builder

	sb.WriteString("Review the following pull request.\n\n")
	sb.WriteString("Title: ")
	sb.WriteString(pr.Title)
	sb.WriteString("\n")
	if pr.Body != "" {
		sb.WriteString("Description: ")
		sb.WriteString(pr.Body)
		sb.WriteString("\n")
	} else {
		sb.WriteString("Description: No description provided.\n")
	}
	sb.WriteString("\n")

	sb.WriteString("Changed files (")
	sb.WriteString(fmt.Sprintf("%d", pr.ChangedFiles))
	sb.WriteString(" files):\n")
	for _, f := range pr.Files {
		sb.WriteString("- [")
		sb.WriteString(f.Status)
		sb.WriteString("] ")
		sb.WriteString(f.Path)
		sb.WriteString(" (+")
		sb.WriteString(fmt.Sprintf("%d", f.Additions))
		sb.WriteString(" -")
		sb.WriteString(fmt.Sprintf("%d", f.Deletions))
		sb.WriteString(")\n")
	}
	sb.WriteString("\n")

	switch depth {
	case "", ContextDepthDiffOnly:
		sb.WriteString("Diff:\n")
		for _, f := range pr.Files {
			sb.WriteString("\n--- ")
			sb.WriteString(f.Path)
			sb.WriteString("\n")
			sb.WriteString(f.Patch)
			sb.WriteString("\n")
		}
	case ContextDepthRelevant, ContextDepthFull:
		for _, f := range pr.Files {
			sb.WriteString("\n=== FILE: ")
			sb.WriteString(f.Path)
			sb.WriteString(" ===\n")
			if depth == ContextDepthFull && f.Content != "" {
				sb.WriteString("--- Full file content ---\n")
				sb.WriteString(f.Content)
				sb.WriteString("\n--- End of file ---\n")
			}
			sb.WriteString("--- Diff ---\n")
			sb.WriteString(f.Patch)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nProvide your review in the required JSON format.")

	return sb.String()
}

func (pb *PromptBuilder) formatCategoryDefinitions(categories []model.Category) string {
	enabled := make(map[model.Category]struct{}, len(categories))
	for _, category := range categories {
		enabled[category] = struct{}{}
	}

	lines := make([]string, 0, len(categoryDefinitions))
	for _, definition := range categoryDefinitions {
		if _, ok := enabled[definition.name]; !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", definition.name, definition.description))
	}

	if len(lines) == 0 {
		return "- maintainability: Code duplication, complex functions, poor naming, missing tests"
	}

	return strings.Join(lines, "\n")
}

func enabledReviewCategories() []model.Category {
	return []model.Category{
		model.CategorySecurity,
		model.CategoryPerformance,
		model.CategoryLogic,
		model.CategoryMaintainability,
		model.CategoryVibe,
	}
}
