package review

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AtomicWasTaken/surge/internal/ai"
	"github.com/AtomicWasTaken/surge/internal/config"
	"github.com/AtomicWasTaken/surge/internal/diff"
	"github.com/AtomicWasTaken/surge/internal/github"
	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/AtomicWasTaken/surge/internal/output"
)

// Orchestrator coordinates the full review pipeline.
type Orchestrator struct {
	aiClient      ai.AIClient
	ghClient      github.PRClient
	prompts       *PromptBuilder
	parser        *OutputParser
	vibe          *VibeDetector
	mdOut         *output.MarkdownOutput
	stdOut        *output.TerminalOutput
	jsonOut       *output.JSONOutput
	cfg           *config.Config
	commentMarker string
}

// NewOrchestrator creates a new review orchestrator.
func NewOrchestrator(aiClient ai.AIClient, ghClient github.PRClient, cfg *config.Config) *Orchestrator {
	return &Orchestrator{
		aiClient:      aiClient,
		ghClient:      ghClient,
		prompts:       NewPromptBuilder(),
		parser:        NewOutputParser(),
		vibe:          NewVibeDetector(),
		mdOut:         output.NewMarkdownOutput(cfg.CommentMarker),
		stdOut:        output.NewTerminalOutput(),
		jsonOut:       output.NewJSONOutput(),
		cfg:           cfg,
		commentMarker: "<!-- " + cfg.CommentMarker + " -->",
	}
}

// Review runs a full code review on a PR.
func (o *Orchestrator) Review(ctx context.Context, owner, repo string, prNumber int, dryRun bool) (*model.ReviewResult, error) {
	start := time.Now()

	// Step 1: Fetch PR metadata
	pr, err := o.ghClient.GetPR(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR: %w", err)
	}

	// Step 2: Fetch changed files
	files, err := o.ghClient.GetFiles(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch files: %w", err)
	}

	// Step 3: Filter files based on config
	files = diff.FilterPaths(files, o.cfg.IncludePaths, o.cfg.ExcludePaths)
	if o.cfg.Verbose {
		fmt.Printf("[debug] fetched pr title=%q changed_files=%d filtered_files=%d\n", pr.Title, pr.ChangedFiles, len(files))
	}

	// Step 4: Build PR context for the prompt
	prCtx := o.buildPRContext(pr, files)
	depth := ContextDepth(o.cfg.ContextDepth)
	if depth == "" {
		depth = ContextDepthDiffOnly
	}

	// Step 5: Build and send AI request
	systemPrompt := o.prompts.SystemPrompt()
	userPrompt := o.prompts.BuildUserPrompt(prCtx, depth)

	// Filter categories based on config
	categories := o.filterCategories(systemPrompt)

	aiReq := &ai.CompletionRequest{
		Model:  o.cfg.AI.Model,
		System: categories,
		Messages: []ai.Message{
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   o.cfg.MaxTokens,
		Temperature: o.cfg.Temperature,
		Debug:       o.cfg.Verbose,
	}
	if o.cfg.Verbose {
		fmt.Printf("[debug] prompt sizes system_chars=%d user_chars=%d\n", len(categories), len(userPrompt))
	}

	aiResp, err := o.aiClient.Complete(ctx, aiReq)
	if err != nil {
		return nil, fmt.Errorf("AI request failed: %w", err)
	}

	// Step 6: Parse AI response
	result, err := o.parser.Parse(aiResp.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w\n\nRaw response:\n%s", err, aiResp.Content)
	}

	// Step 7: Apply vibe detection heuristics
	o.vibe.Detect(result, aiResp.Content)

	// Step 8: Set stats
	result.Stats = model.ReviewStats{
		FilesReviewed: len(files),
		TokensIn:      aiResp.TokensIn,
		TokensOut:     aiResp.TokensOut,
		Duration:      time.Since(start).Seconds(),
	}

	// Step 9: Output
	if o.cfg.Output.Format == "json" {
		fmt.Println(o.jsonOut.Render(result))
	} else {
		// Terminal output
		o.stdOut.Render(result)
	}

	// Step 10: Post to GitHub (unless dry run)
	if !dryRun {
		if err := o.postReview(ctx, owner, repo, prNumber, result, files); err != nil {
			return nil, fmt.Errorf("failed to post review: %w", err)
		}
	}

	return result, nil
}

func (o *Orchestrator) buildPRContext(pr *model.PR, files []model.FileChange) *PRContext {
	prCtx := &PRContext{
		Title:        pr.Title,
		Body:         pr.Body,
		ChangedFiles: len(files),
		Files:        make([]FileContext, len(files)),
	}

	for i, f := range files {
		prCtx.Files[i] = FileContext{
			Path:      f.Path,
			Status:    string(f.Status),
			Additions: f.Additions,
			Deletions: f.Deletions,
			Patch:     f.Patch,
		}
	}

	return prCtx
}

func (o *Orchestrator) filterCategories(systemPrompt string) string {
	// The system prompt already includes all categories.
	// We could strip unused categories from the prompt, but for simplicity
	// we include all and let the AI focus on what matters.
	// For a more optimized approach, we could modify the prompt here.
	_ = systemPrompt
	return o.prompts.SystemPrompt()
}

func (o *Orchestrator) postReview(ctx context.Context, owner, repo string, prNumber int, result *model.ReviewResult, files []model.FileChange) error {
	// Delete old surge comments first (idempotency)
	if err := o.deleteOldComments(ctx, owner, repo, prNumber); err != nil {
		// Log but don't fail - the old comments will just pile up
		fmt.Printf("Warning: failed to delete old comments: %v\n", err)
	}

	// Build inline comments
	var comments []model.ReviewComment
	if !o.cfg.DisableInlineComments {
		comments = o.buildInlineComments(result, files)
		if len(comments) > o.cfg.MaxInlineComments && o.cfg.MaxInlineComments > 0 {
			comments = comments[:o.cfg.MaxInlineComments]
		}
	}

	// Post summary as an issue comment so reruns can replace it cleanly.
	// GitHub does not allow deleting submitted PR reviews via API.
	if !o.cfg.DisableSummaryComment {
		body := o.mdOut.RenderSummary(result)
		if err := o.ghClient.PostComment(ctx, owner, repo, prNumber, body); err != nil {
			return err
		}
	}

	// Post inline comments as a review only when needed.
	if len(comments) > 0 {
		event := "COMMENT"
		if result.Approve {
			event = "APPROVE"
		}

		reviewInput := &model.ReviewInput{
			Body:     "<!-- " + o.cfg.CommentMarker + "_INLINE -->",
			Event:    event,
			Comments: comments,
		}
		if err := o.ghClient.PostReview(ctx, owner, repo, prNumber, reviewInput); err != nil {
			return err
		}
	}

	if err := o.syncPRLabels(ctx, owner, repo, prNumber, result); err != nil {
		fmt.Printf("Warning: failed to sync PR labels: %v\n", err)
	}

	return nil
}

func (o *Orchestrator) buildInlineComments(result *model.ReviewResult, files []model.FileChange) []model.ReviewComment {
	var comments []model.ReviewComment

	// Build a map of file paths to their patches for position lookup
	filePatches := make(map[string]string)
	for _, f := range files {
		filePatches[f.Path] = f.Patch
	}

	for _, finding := range result.Findings {
		if finding.File == "" || finding.Line <= 0 {
			continue
		}

		// Find the position in the diff for this file/line
		position := findPositionInPatch(filePatches[finding.File], finding.Line)
		if position <= 0 {
			continue
		}

		body := fmt.Sprintf("**%s** %s\n\n%s",
			strings.ToUpper(string(finding.Severity)),
			finding.Title,
			finding.Body,
		)

		comments = append(comments, model.ReviewComment{
			Path:     finding.File,
			Position: position,
			Body:     body,
		})
	}

	return comments
}

func (o *Orchestrator) deleteOldComments(ctx context.Context, owner, repo string, prNumber int) error {
	comments, err := o.ghClient.ListComments(ctx, owner, repo, prNumber)
	if err != nil {
		return err
	}

	for _, c := range comments {
		if o.isSurgeComment(c.Body) {
			if err := o.ghClient.DeleteComment(ctx, owner, repo, c.ID); err != nil {
				return err
			}
		}
	}

	reviews, err := o.ghClient.ListReviews(ctx, owner, repo, prNumber)
	if err != nil {
		return err
	}

	for _, r := range reviews {
		if !o.isSurgeComment(r.Body) {
			continue
		}

		reviewComments, err := o.ghClient.ListReviewComments(ctx, owner, repo, prNumber, r.ID)
		if err != nil {
			return err
		}

		for _, rc := range reviewComments {
			if err := o.ghClient.DeleteReviewComment(ctx, owner, repo, rc.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

func (o *Orchestrator) isSurgeComment(body string) bool {
	legacySummaryMarker := "<!-- SURGE_SUMMARY -->"
	return strings.Contains(body, o.commentMarker) ||
		strings.Contains(body, "<!-- "+o.cfg.CommentMarker+"_") ||
		strings.Contains(body, legacySummaryMarker)
}

// findPositionInPatch finds the diff position for a given file line number.
// This is approximate - it scans the patch for the line and returns its position.
func findPositionInPatch(patch string, targetLine int) int {
	if patch == "" {
		return 0
	}

	lines := strings.Split(patch, "\n")
	currentFileLine := 0
	position := 0

	for _, line := range lines {
		position++
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") {
			currentFileLine++
			if currentFileLine == targetLine && strings.HasPrefix(line, "+") {
				return position
			}
		}
	}

	return 0
}

func (o *Orchestrator) syncPRLabels(ctx context.Context, owner, repo string, prNumber int, result *model.ReviewResult) error {
	if !o.cfg.EnablePRLabels {
		return nil
	}

	prefix := strings.TrimSpace(o.cfg.PRLabelPrefix)
	if prefix == "" {
		prefix = "surge"
	}

	labelSpecs := buildSurgeLabelSpecs(prefix, result)
	desired := make([]string, 0, len(labelSpecs))
	for _, spec := range labelSpecs {
		if err := o.ghClient.UpsertLabel(ctx, owner, repo, spec.Name, spec.Color, spec.Description); err != nil {
			return err
		}
		desired = append(desired, spec.Name)
	}

	desiredSet := make(map[string]struct{}, len(desired))
	for _, l := range desired {
		desiredSet[l] = struct{}{}
	}

	existing, err := o.ghClient.ListLabels(ctx, owner, repo, prNumber)
	if err != nil {
		return err
	}

	for _, label := range existing {
		if !isManagedSurgeLabel(prefix, label) {
			continue
		}
		if _, keep := desiredSet[label]; keep {
			continue
		}
		if err := o.ghClient.RemoveLabel(ctx, owner, repo, prNumber, label); err != nil {
			return err
		}
	}

	return o.ghClient.AddLabels(ctx, owner, repo, prNumber, desired)
}

type labelSpec struct {
	Name        string
	Color       string
	Description string
}

func buildSurgeLabelSpecs(prefix string, result *model.ReviewResult) []labelSpec {
	decision := "changes requested"
	if result.Approve {
		decision = "approved"
	}

	findings := "present"
	if len(result.Findings) == 0 {
		findings = "none found"
	}

	effort := classifyReviewEffort(result)

	return []labelSpec{
		{
			Name:        labelName(prefix, "Reviewed"),
			Color:       "1f6feb",
			Description: "PR has been reviewed by surge",
		},
		{
			Name:        labelName(prefix, "Effort / "+titleWord(effort)),
			Color:       reviewEffortColor(effort),
			Description: "Estimated review effort from surge analysis",
		},
		{
			Name:        labelName(prefix, "Decision / "+decisionTitle(decision)),
			Color:       decisionColor(decision),
			Description: "Review decision from surge",
		},
		{
			Name:        labelName(prefix, "Findings / "+findingsTitle(findings)),
			Color:       findingsColor(findings),
			Description: "Whether surge reported actionable findings",
		},
	}
}

func isManagedSurgeLabel(prefix, label string) bool {
	base := titleWord(prefix)
	return label == base+": Reviewed" ||
		strings.HasPrefix(label, base+": Effort / ") ||
		strings.HasPrefix(label, base+": Decision / ") ||
		strings.HasPrefix(label, base+": Findings / ")
}

func classifyReviewEffort(result *model.ReviewResult) string {
	critical := 0
	high := 0
	medium := 0
	for _, f := range result.Findings {
		switch f.Severity {
		case model.SeverityCritical:
			critical++
		case model.SeverityHigh:
			high++
		case model.SeverityMedium:
			medium++
		}
	}

	files := result.Stats.FilesReviewed
	findings := len(result.Findings)

	if files >= 20 || critical > 0 || high >= 2 || findings >= 12 {
		return "high"
	}
	if files >= 8 || high > 0 || medium >= 2 || findings >= 5 {
		return "medium"
	}
	return "low"
}

func reviewEffortColor(effort string) string {
	switch effort {
	case "high":
		return "b60205"
	case "medium":
		return "fbca04"
	default:
		return "2da44e"
	}
}

func decisionColor(decision string) string {
	switch decision {
	case "approved":
		return "2da44e"
	default:
		return "d73a4a"
	}
}

func findingsColor(findings string) string {
	switch findings {
	case "none found":
		return "2da44e"
	default:
		return "fb8500"
	}
}

func labelName(prefix, suffix string) string {
	return titleWord(prefix) + ": " + suffix
}

func decisionTitle(decision string) string {
	switch decision {
	case "approved":
		return "Approved"
	default:
		return "Changes Requested"
	}
}

func findingsTitle(findings string) string {
	switch findings {
	case "none found":
		return "None"
	default:
		return "Present"
	}
}

func titleWord(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}
