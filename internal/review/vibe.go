package review

import (
	"strings"

	"github.com/AtomicWasTaken/surge/internal/model"
)

// VibePattern defines a single vibe detection rule.
type VibePattern struct {
	Name        string
	Description string
	Weight      int // 1-5, higher = more damaging to vibe score
}

// VibePatterns is the registry of all vibe detection patterns.
var VibePatterns = []VibePattern{
	{"generic_boilerplate", "Uses generic try-catch-wrapper patterns with no specific error handling", 3},
	{"over_engineered", "Introduces unnecessary abstraction layers, factories, or interfaces for simple code", 4},
	{"context_blind", "Makes changes that ignore existing patterns, naming conventions, or architectural decisions", 4},
	{"wrong_approach", "Uses a technically correct but idiomatically wrong approach for the language/framework", 3},
	{"magic_numbers", "Introduces unexplained constants or configuration without justification", 2},
	{"ai_fluff", "Response contains generic praise ('great job', 'well done') or vague suggestions", 2},
	{"inconsistent_naming", "Introduces naming that conflicts with existing conventions", 3},
	{"shotgun_approaches", "Suggests multiple alternative implementations without clear rationale", 2},
	{"missing_tests", "No test coverage mentioned for logic changes", 2},
	{"confused_about_context", "AI references things that don't exist in the codebase or misunderstands the domain", 5},
}

// VibeDetector applies heuristics to adjust and validate the AI's vibe assessment.
type VibeDetector struct{}

// NewVibeDetector creates a new vibe detector.
func NewVibeDetector() *VibeDetector {
	return &VibeDetector{}
}

// Detect analyzes the AI response and adjusts the vibe check.
func (d *VibeDetector) Detect(result *model.ReviewResult, aiResponse string) {
	// Start with the AI's vibe check
	flags := result.VibeCheck.Flags
	if flags == nil {
		flags = []string{}
	}

	// Apply heuristic adjustments
	deductions := 0

	// Check for generic praise in the summary
	lower := strings.ToLower(result.Summary)
	genericPhrases := []string{
		"looks good", "looks great", "well done", "great job",
		"nice work", "excellent work", "good job", "lgtm",
		"overall looks good", "overall looks great",
	}
	for _, phrase := range genericPhrases {
		if strings.Contains(lower, phrase) {
			flags = append(flags, "ai_fluff")
			deductions++
			break
		}
	}

	// Check for vague recommendations
	for _, rec := range result.Recommendations {
		lowerRec := strings.ToLower(rec)
		if len(rec) < 20 || strings.HasPrefix(lowerRec, "consider") || strings.HasPrefix(lowerRec, "you might") {
			// Very vague recommendation
		}
	}

	// Count over-engineering signals in findings
	overEngFindings := 0
	for _, f := range result.Findings {
		if f.Category == model.CategoryVibe && strings.Contains(strings.ToLower(f.Title), "over-engineer") {
			overEngFindings++
			deductions++
		}
	}

	// Check for suspiciously perfect scores without any flags
	if result.VibeCheck.Score == 10 && len(flags) == 0 && len(result.Findings) > 5 {
		// Very unlikely to be genuinely perfect with no flags
		deductions += 2
		flags = append(flags, "suspiciously_perfect")
	}

	// Apply deductions to the score
	newScore := result.VibeCheck.Score - deductions
	if newScore < 1 {
		newScore = 1
	}
	if newScore > 10 {
		newScore = 10
	}
	result.VibeCheck.Score = newScore

	// Deduplicate and update flags
	seen := make(map[string]bool)
	var dedupedFlags []string
	for _, f := range flags {
		if !seen[f] {
			seen[f] = true
			dedupedFlags = append(dedupedFlags, f)
		}
	}
	result.VibeCheck.Flags = dedupedFlags

	// Update verdict if score changed significantly
	if result.VibeCheck.Score >= 8 {
		result.VibeCheck.Verdict = "Excellent. Hand-crafted, idiomatic code."
	} else if result.VibeCheck.Score >= 6 {
		result.VibeCheck.Verdict = "Good with some room for improvement."
	} else if result.VibeCheck.Score >= 4 {
		result.VibeCheck.Verdict = "Mixed. Some AI fingerprints detected."
	} else {
		result.VibeCheck.Verdict = "Concerning. Significant over-engineering or context issues."
	}
}
