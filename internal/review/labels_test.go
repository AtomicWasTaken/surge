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

func TestBuildSurgeLabelSpecs(t *testing.T) {
	labels := buildSurgeLabelSpecs("surge", &model.ReviewResult{
		Approve: true,
		Stats:   model.ReviewStats{FilesReviewed: 2},
	})

	assert.Len(t, labels, 4)
	assert.Equal(t, "Surge: Reviewed", labels[0].Name)
	assert.Equal(t, "1f6feb", labels[0].Color)
	assert.Equal(t, "Surge: Effort / Low", labels[1].Name)
	assert.Equal(t, "2da44e", labels[1].Color)
	assert.Equal(t, "Surge: Decision / Approved", labels[2].Name)
	assert.Equal(t, "2da44e", labels[2].Color)
	assert.Equal(t, "Surge: Findings / None", labels[3].Name)
	assert.Equal(t, "2da44e", labels[3].Color)
}

func TestIsManagedSurgeLabel(t *testing.T) {
	assert.True(t, isManagedSurgeLabel("surge", "Surge: Reviewed"))
	assert.True(t, isManagedSurgeLabel("surge", "Surge: Effort / Medium"))
	assert.True(t, isManagedSurgeLabel("surge", "Surge: Decision / Changes Requested"))
	assert.True(t, isManagedSurgeLabel("surge", "Surge: Findings / Present"))
	assert.False(t, isManagedSurgeLabel("surge", "bug"))
	assert.False(t, isManagedSurgeLabel("surge", "needs-review"))
}

func TestLabelColors(t *testing.T) {
	assert.Equal(t, "b60205", reviewEffortColor("high"))
	assert.Equal(t, "fbca04", reviewEffortColor("medium"))
	assert.Equal(t, "2da44e", reviewEffortColor("low"))

	assert.Equal(t, "2da44e", decisionColor("approved"))
	assert.Equal(t, "d73a4a", decisionColor("changes requested"))

	assert.Equal(t, "2da44e", findingsColor("none found"))
	assert.Equal(t, "fb8500", findingsColor("present"))
}
