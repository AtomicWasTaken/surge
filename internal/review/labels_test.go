package review

import (
	"testing"

	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestClassifyReviewEffort(t *testing.T) {
	low := classifyReviewEffort(&model.ReviewResult{
		Stats:    model.ReviewStats{FilesReviewed: 3},
		Findings: []model.Finding{{Severity: model.SeverityLow}},
	})
	assert.Equal(t, "low", low)

	medium := classifyReviewEffort(&model.ReviewResult{
		Stats: model.ReviewStats{FilesReviewed: 9},
	})
	assert.Equal(t, "medium", medium)

	high := classifyReviewEffort(&model.ReviewResult{
		Stats: model.ReviewStats{FilesReviewed: 4},
		Findings: []model.Finding{
			{Severity: model.SeverityCritical},
		},
	})
	assert.Equal(t, "high", high)
}

func TestBuildSurgeLabels(t *testing.T) {
	labels := buildSurgeLabels("surge", &model.ReviewResult{
		Approve: true,
		Stats:   model.ReviewStats{FilesReviewed: 2},
	})

	assert.Equal(t, []string{
		"surge reviewed",
		"surge effort: low",
		"surge decision: approved",
		"surge findings: none found",
	}, labels)
}

func TestIsManagedSurgeLabel(t *testing.T) {
	assert.True(t, isManagedSurgeLabel("surge", "surge reviewed"))
	assert.True(t, isManagedSurgeLabel("surge", "surge effort: medium"))
	assert.True(t, isManagedSurgeLabel("surge", "surge decision: changes requested"))
	assert.True(t, isManagedSurgeLabel("surge", "surge findings: present"))
	assert.False(t, isManagedSurgeLabel("surge", "bug"))
	assert.False(t, isManagedSurgeLabel("surge", "needs-review"))
}
