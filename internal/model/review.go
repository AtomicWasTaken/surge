package model

// ReviewResult is the structured output from the AI code review.
type ReviewResult struct {
	Summary         string         `json:"summary"`
	FilesOverview   []FileOverview `json:"filesOverview"`
	Findings        []Finding      `json:"findings"`
	VibeCheck       VibeCheck      `json:"vibeCheck"`
	Recommendations []string       `json:"recommendations"`
	Approve         bool           `json:"approve"`
	Stats           ReviewStats    `json:"stats,omitempty"`
}

// FileOverview provides a summary of each changed file.
type FileOverview struct {
	Path    string `json:"path"`
	Changes string `json:"changes"`
	Risk    string `json:"risk"` // low, medium, high
}

// Finding represents a single issue found in the code review.
type Finding struct {
	Severity Severity `json:"severity"`
	Category Category `json:"category"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
}

// Severity represents how severe a finding is.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// Category represents the type of issue found.
type Category string

const (
	CategorySecurity        Category = "security"
	CategoryPerformance     Category = "performance"
	CategoryLogic           Category = "logic"
	CategoryMaintainability Category = "maintainability"
	CategoryVibe            Category = "vibe"
)

// VibeCheck represents the AI-generated vibe codability assessment.
type VibeCheck struct {
	Score   int      `json:"score"`
	Verdict string   `json:"verdict"`
	Flags   []string `json:"flags"`
}

// ReviewStats holds metadata about the review execution.
type ReviewStats struct {
	FilesReviewed int     `json:"filesReviewed"`
	TokensIn      int     `json:"tokensIn"`
	TokensOut     int     `json:"tokensOut"`
	Duration      float64 `json:"duration"` // seconds
}

// ReviewInput represents a review to be posted to GitHub.
type ReviewInput struct {
	Body     string
	Event    string // "COMMENT", "APPROVE", "REQUEST_CHANGES"
	Comments []ReviewComment
}

// ReviewComment represents an inline comment on a specific file/line.
type ReviewComment struct {
	Path     string `json:"path"`
	Position int    `json:"position"` // line in the diff patch
	Body     string `json:"body"`
}

// PRComment represents a comment on a PR.
type PRComment struct {
	ID        int64
	Body      string
	Author    string
	IsBot     bool
	CreatedAt string
}

// PRReview represents a pull request review.
type PRReview struct {
	ID        int64
	Body      string
	State     string
	Author    string
	IsBot     bool
	CreatedAt string
}

// PRReviewComment represents an inline comment attached to a PR review.
type PRReviewComment struct {
	ID   int64
	Body string
}
