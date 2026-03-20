package output

import (
	"encoding/json"
	"fmt"

	"github.com/AtomicWasTaken/surge/internal/model"
)

// JSONOutput renders review results as structured JSON.
type JSONOutput struct{}

var (
	jsonMarshalIndent = json.MarshalIndent
	jsonMarshal       = json.Marshal
)

// NewJSONOutput creates a new JSON output renderer.
func NewJSONOutput() *JSONOutput {
	return &JSONOutput{}
}

// Render renders the review result as JSON.
func (j *JSONOutput) Render(result *model.ReviewResult) string {
	// Use a map to control the output structure
	out := map[string]interface{}{
		"version": "1.0",
		"summary": result.Summary,
		"vibeCheck": map[string]interface{}{
			"score":   result.VibeCheck.Score,
			"verdict": result.VibeCheck.Verdict,
			"flags":   result.VibeCheck.Flags,
		},
		"findings":        result.Findings,
		"warnings":        result.Warnings,
		"recommendations": result.Recommendations,
		"approve":         result.Approve,
		"stats": map[string]interface{}{
			"filesReviewed": result.Stats.FilesReviewed,
			"tokensIn":      result.Stats.TokensIn,
			"tokensOut":     result.Stats.TokensOut,
			"duration":      fmt.Sprintf("%.1fs", result.Stats.Duration),
		},
	}

	data, err := jsonMarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal JSON: %v"}`, err)
	}

	return string(data)
}

// RenderCompact renders the review result as compact JSON (single line).
func (j *JSONOutput) RenderCompact(result *model.ReviewResult) string {
	data, err := jsonMarshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal JSON: %v"}`, err)
	}
	return string(data)
}
