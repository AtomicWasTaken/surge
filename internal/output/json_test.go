package output

import (
	"errors"
	"strings"
	"testing"

	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestJSONOutputRenderAndCompact(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "Review summary",
		VibeCheck: model.VibeCheck{
			Score:   7,
			Verdict: "Reasonable",
			Flags:   []string{"flag-a"},
		},
		Findings: []model.Finding{
			{Severity: model.SeverityLow, Title: "Minor issue", Body: "Details"},
		},
		Warnings:        []string{"partial context"},
		Recommendations: []string{"add tests"},
		Approve:         true,
		Stats: model.ReviewStats{
			FilesReviewed: 2,
			TokensIn:      10,
			TokensOut:     20,
			Duration:      1.25,
		},
	}

	out := NewJSONOutput()
	rendered := out.Render(result)
	compact := out.RenderCompact(result)

	assert.Contains(t, rendered, `"version": "1.0"`)
	assert.Contains(t, rendered, `"summary": "Review summary"`)
	assert.Contains(t, rendered, `"duration": "1.2s"`)
	assert.Contains(t, compact, `"summary":"Review summary"`)
	assert.NotContains(t, compact, "\n")
}

func TestJSONOutputRenderCompactMarshalError(t *testing.T) {
	result := &model.ReviewResult{
		Warnings: []string{"contains invalid utf8: " + string([]byte{0xff})},
	}

	rendered := NewJSONOutput().RenderCompact(result)
	assert.True(t, strings.HasPrefix(rendered, "{"))
}

func TestJSONOutputMarshalFailures(t *testing.T) {
	prevIndent := jsonMarshalIndent
	prevMarshal := jsonMarshal
	t.Cleanup(func() {
		jsonMarshalIndent = prevIndent
		jsonMarshal = prevMarshal
	})

	jsonMarshalIndent = func(v interface{}, prefix, indent string) ([]byte, error) {
		return nil, errors.New("indent failed")
	}
	jsonMarshal = func(v interface{}) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}

	out := NewJSONOutput()
	assert.Contains(t, out.Render(&model.ReviewResult{}), `failed to marshal JSON: indent failed`)
	assert.Contains(t, out.RenderCompact(&model.ReviewResult{}), `failed to marshal JSON: marshal failed`)
}
