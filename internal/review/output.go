package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtomicWasTaken/surge/internal/model"
)

// OutputParser parses the AI's JSON response into a ReviewResult.
type OutputParser struct{}

// NewOutputParser creates a new output parser.
func NewOutputParser() *OutputParser {
	return &OutputParser{}
}

// Parse parses the AI response text into a ReviewResult.
func (p *OutputParser) Parse(text string) (*model.ReviewResult, error) {
	// Try direct JSON parse first
	result, err := p.parseJSON(text)
	if err == nil {
		return result, nil
	}

	// Try stripping markdown code fences
	cleaned := stripCodeFences(text)
	result, err = p.parseJSON(cleaned)
	if err == nil {
		return result, nil
	}

	// Try extracting JSON from within the text
	extracted := extractJSON(text)
	if extracted != "" {
		result, err = p.parseJSON(extracted)
		if err == nil {
			return result, nil
		}
	}

	return nil, fmt.Errorf("failed to parse AI response as JSON: %w (tried raw, stripped fences, and extraction)", err)
}

func (p *OutputParser) parseJSON(text string) (*model.ReviewResult, error) {
	// Normalize whitespace and trim
	text = strings.TrimSpace(text)

	var result model.ReviewResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("JSON unmarshal failed: %w", err)
	}

	// Validate required fields
	if result.Summary == "" {
		return nil, fmt.Errorf("missing required field: summary")
	}

	return &result, nil
}

func stripCodeFences(text string) string {
	text = strings.TrimSpace(text)

	// Handle ```json ... ```
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		if idx := strings.Index(text, "```"); idx >= 0 {
			text = text[:idx]
		}
	} else if strings.HasPrefix(text, "```") {
		// Handle generic ``` ... ```
		text = strings.TrimPrefix(text, "```")
		if idx := strings.Index(text, "```"); idx >= 0 {
			text = text[:idx]
		}
	}

	return strings.TrimSpace(text)
}

func extractJSON(text string) string {
	// Try to find a JSON object bounded by { and }
	text = strings.TrimSpace(text)

	start := strings.Index(text, "{")
	if start < 0 {
		return ""
	}

	// Find matching closing brace using a simple depth counter
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}

	return ""
}
