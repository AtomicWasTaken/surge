package review

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/AtomicWasTaken/surge/internal/config"
	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/AtomicWasTaken/surge/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPRClient struct {
	comments                []*model.PRComment
	reviews                 []*model.PRReview
	reviewComments          map[int64][]*model.PRReviewComment
	fileContents            map[string]string
	fileContentErrs         map[string]error
	deletedIssueCommentIDs  []int64
	deletedReviewCommentIDs []int64
	deletedReviewIDs        []int64
	dismissedReviewIDs      []int64
	dismissMessages         []string
	postedCommentBodies     []string
	postedReviews           []*model.ReviewInput
	listReviewCommentsIDs   []int64
	listCommentsErr         error
	listReviewsErr          error
	listReviewCommentsErrs  map[int64]error
	deleteCommentErrs       map[int64]error
	deleteReviewCommentErrs map[int64]error
	deleteReviewErrs        map[int64]error
	dismissReviewErrs       map[int64]error
}

func (s *stubPRClient) GetPR(ctx context.Context, owner, repo string, prNumber int) (*model.PR, error) {
	return nil, nil
}

func (s *stubPRClient) GetDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	return "", nil
}

func (s *stubPRClient) GetFiles(ctx context.Context, owner, repo string, prNumber int) ([]model.FileChange, error) {
	return nil, nil
}

func (s *stubPRClient) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	key := path + "@" + ref
	if err := s.fileContentErrs[key]; err != nil {
		return "", err
	}
	return s.fileContents[key], nil
}

func (s *stubPRClient) PostReview(ctx context.Context, owner, repo string, prNumber int, review *model.ReviewInput) error {
	s.postedReviews = append(s.postedReviews, review)
	return nil
}

func (s *stubPRClient) PostComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	s.postedCommentBodies = append(s.postedCommentBodies, body)
	return nil
}

func (s *stubPRClient) ListComments(ctx context.Context, owner, repo string, prNumber int) ([]*model.PRComment, error) {
	if s.listCommentsErr != nil {
		return nil, s.listCommentsErr
	}
	return s.comments, nil
}

func (s *stubPRClient) DeleteComment(ctx context.Context, owner, repo string, commentID int64) error {
	if err := s.deleteCommentErrs[commentID]; err != nil {
		return err
	}
	s.deletedIssueCommentIDs = append(s.deletedIssueCommentIDs, commentID)
	return nil
}

func (s *stubPRClient) ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]*model.PRReview, error) {
	if s.listReviewsErr != nil {
		return nil, s.listReviewsErr
	}
	return s.reviews, nil
}

func (s *stubPRClient) DeleteReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64) error {
	if err := s.deleteReviewErrs[reviewID]; err != nil {
		return err
	}
	s.deletedReviewIDs = append(s.deletedReviewIDs, reviewID)
	return nil
}

func (s *stubPRClient) DismissReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64, message string) error {
	if err := s.dismissReviewErrs[reviewID]; err != nil {
		return err
	}
	s.dismissedReviewIDs = append(s.dismissedReviewIDs, reviewID)
	s.dismissMessages = append(s.dismissMessages, message)
	return nil
}

func (s *stubPRClient) ListReviewComments(ctx context.Context, owner, repo string, prNumber int, reviewID int64) ([]*model.PRReviewComment, error) {
	if err := s.listReviewCommentsErrs[reviewID]; err != nil {
		return nil, err
	}
	s.listReviewCommentsIDs = append(s.listReviewCommentsIDs, reviewID)
	return s.reviewComments[reviewID], nil
}

func (s *stubPRClient) DeleteReviewComment(ctx context.Context, owner, repo string, commentID int64) error {
	if err := s.deleteReviewCommentErrs[commentID]; err != nil {
		return err
	}
	s.deletedReviewCommentIDs = append(s.deletedReviewCommentIDs, commentID)
	return nil
}

func (s *stubPRClient) ListLabels(ctx context.Context, owner, repo string, prNumber int) ([]string, error) {
	return nil, nil
}

func (s *stubPRClient) AddLabels(ctx context.Context, owner, repo string, prNumber int, labels []string) error {
	return nil
}

func (s *stubPRClient) RemoveLabel(ctx context.Context, owner, repo string, prNumber int, label string) error {
	return nil
}

func (s *stubPRClient) UpsertLabel(ctx context.Context, owner, repo, name, color, description string) error {
	return nil
}

func TestDeleteOldCommentsCleansIssueCommentsAndSupersedesReviews(t *testing.T) {
	client := &stubPRClient{
		comments: []*model.PRComment{
			{ID: 1, Body: "<!-- SURGE -->\nsummary"},
			{ID: 2, Body: "human comment"},
		},
		reviews: []*model.PRReview{
			{ID: 10, Body: "<!-- SURGE_INLINE -->", State: "COMMENTED"},
			{ID: 11, Body: "<!-- SURGE_INLINE -->", State: "PENDING"},
			{ID: 12, Body: "other bot", State: "APPROVED"},
		},
		reviewComments: map[int64][]*model.PRReviewComment{
			10: {{ID: 101}, {ID: 102}},
			11: {{ID: 111}},
		},
	}
	cfg := &config.Config{CommentMarker: "SURGE"}
	o := NewOrchestrator(nil, client, cfg)

	stats, err := o.deleteOldComments(context.Background(), "o", "r", 1)
	require.NoError(t, err)

	assert.Equal(t, cleanupStats{
		deletedIssueComments:  1,
		deletedReviewComments: 3,
		deletedReviews:        1,
		dismissedReviews:      1,
	}, stats)
	assert.ElementsMatch(t, []int64{1}, client.deletedIssueCommentIDs)
	assert.ElementsMatch(t, []int64{101, 102, 111}, client.deletedReviewCommentIDs)
	assert.ElementsMatch(t, []int64{10}, client.dismissedReviewIDs)
	assert.ElementsMatch(t, []int64{11}, client.deletedReviewIDs)
	assert.ElementsMatch(t, []int64{10, 11}, client.listReviewCommentsIDs)
	assert.Len(t, client.dismissMessages, 1)
	assert.Equal(t, reviewDismissalMessage, client.dismissMessages[0])
}

func TestDeleteOldCommentsDismissesApprovedAndChangesRequestedReviews(t *testing.T) {
	client := &stubPRClient{
		reviews: []*model.PRReview{
			{ID: 20, Body: "<!-- SURGE_INLINE -->", State: "APPROVED"},
			{ID: 21, Body: "<!-- SURGE_INLINE -->", State: "CHANGES_REQUESTED"},
		},
		reviewComments: map[int64][]*model.PRReviewComment{
			20: {},
			21: {},
		},
	}
	cfg := &config.Config{CommentMarker: "SURGE"}
	o := NewOrchestrator(nil, client, cfg)

	stats, err := o.deleteOldComments(context.Background(), "o", "r", 1)
	require.NoError(t, err)

	assert.Equal(t, cleanupStats{
		dismissedReviews: 2,
	}, stats)
	assert.Empty(t, client.deletedReviewIDs)
	assert.ElementsMatch(t, []int64{20, 21}, client.dismissedReviewIDs)
	assert.ElementsMatch(t, []int64{20, 21}, client.listReviewCommentsIDs)
}

func TestDeleteOldCommentsSkipsUnknownAndEmptyStates(t *testing.T) {
	client := &stubPRClient{
		reviews: []*model.PRReview{
			{ID: 30, Body: "<!-- SURGE_INLINE -->", State: ""},
			{ID: 31, Body: "<!-- SURGE_INLINE -->", State: "COMMENTED"},
			{ID: 32, Body: "<!-- SURGE_INLINE -->", State: "SOMETHING_NEW"},
		},
		reviewComments: map[int64][]*model.PRReviewComment{
			30: {},
			31: {},
			32: {},
		},
	}
	cfg := &config.Config{CommentMarker: "SURGE"}
	o := NewOrchestrator(nil, client, cfg)

	stats, err := o.deleteOldComments(context.Background(), "o", "r", 1)
	require.NoError(t, err)

	assert.Empty(t, client.deletedReviewIDs)
	assert.ElementsMatch(t, []int64{31}, client.dismissedReviewIDs)
	assert.ElementsMatch(t, []int64{30, 31, 32}, client.listReviewCommentsIDs)
	assert.Equal(t, cleanupStats{
		dismissedReviews: 1,
		skippedReviews:   2,
	}, stats)
}

func TestDeleteOldCommentsContinuesAfterCleanupFailures(t *testing.T) {
	client := &stubPRClient{
		comments: []*model.PRComment{
			{ID: 1, Body: "<!-- SURGE -->"},
			{ID: 2, Body: "<!-- SURGE -->"},
		},
		reviews: []*model.PRReview{
			{ID: 10, Body: "<!-- SURGE_INLINE -->", State: "COMMENTED"},
			{ID: 11, Body: "<!-- SURGE_INLINE -->", State: "PENDING"},
		},
		reviewComments: map[int64][]*model.PRReviewComment{
			10: {{ID: 101}},
			11: {{ID: 111}},
		},
		deleteCommentErrs: map[int64]error{
			1: errors.New("no issue delete permission"),
		},
		deleteReviewCommentErrs: map[int64]error{
			101: errors.New("no review comment delete permission"),
		},
		dismissReviewErrs: map[int64]error{
			10: errors.New("dismiss denied"),
		},
		deleteReviewErrs: map[int64]error{
			11: errors.New("delete denied"),
		},
	}
	cfg := &config.Config{CommentMarker: "SURGE"}
	o := NewOrchestrator(nil, client, cfg)

	stats, err := o.deleteOldComments(context.Background(), "o", "r", 1)
	require.Error(t, err)
	assert.ErrorContains(t, err, "delete issue comment 1")
	assert.ErrorContains(t, err, "delete review comment 101")
	assert.ErrorContains(t, err, "dismiss review 10")
	assert.ErrorContains(t, err, "delete review 11")
	assert.ElementsMatch(t, []int64{2}, client.deletedIssueCommentIDs)
	assert.ElementsMatch(t, []int64{111}, client.deletedReviewCommentIDs)
	assert.ElementsMatch(t, []int64{10, 11}, client.listReviewCommentsIDs)
	assert.Equal(t, cleanupStats{
		deletedIssueComments:  1,
		deletedReviewComments: 1,
		failedOperations:      4,
	}, stats)
}

func TestPostReviewLogsCleanupStatsOnPermissionFailures(t *testing.T) {
	client := &stubPRClient{
		comments: []*model.PRComment{
			{ID: 1, Body: "<!-- SURGE -->"},
		},
		reviews: []*model.PRReview{
			{ID: 10, Body: "<!-- SURGE_INLINE -->", State: "COMMENTED"},
		},
		reviewComments: map[int64][]*model.PRReviewComment{
			10: {{ID: 101}},
		},
		deleteCommentErrs: map[int64]error{
			1: errors.New("forbidden"),
		},
		deleteReviewCommentErrs: map[int64]error{
			101: errors.New("forbidden"),
		},
		dismissReviewErrs: map[int64]error{
			10: errors.New("forbidden"),
		},
	}
	cfg := &config.Config{
		CommentMarker:         "SURGE",
		EnablePRLabels:        false,
		DisableInlineComments: true,
	}
	o := NewOrchestrator(nil, client, cfg)

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	postErr := o.postReview(context.Background(), "o", "r", 1, &model.ReviewResult{}, nil)

	require.NoError(t, w.Close())
	os.Stdout = stdout
	require.NoError(t, postErr)

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	out := buf.String()
	assert.Contains(t, out, "Warning: surge cleanup completed with errors")
	assert.Contains(t, out, "deleted_issue_comments=0")
	assert.Contains(t, out, "deleted_review_comments=0")
	assert.Contains(t, out, "dismissed_reviews=0")
	assert.Contains(t, out, "failures=3")
}

func TestPostReviewUsesDismissibleReviewEventForInlineComments(t *testing.T) {
	client := &stubPRClient{reviewComments: map[int64][]*model.PRReviewComment{}}
	cfg := &config.Config{
		CommentMarker:         "SURGE",
		EnablePRLabels:        false,
		MaxInlineComments:     10,
		DisableSummaryComment: false,
	}
	o := NewOrchestrator(nil, client, cfg)

	result := &model.ReviewResult{
		Approve: false,
		Findings: []model.Finding{
			{
				Severity: model.SeverityHigh,
				File:     "a.go",
				Line:     2,
				Title:    "Bug",
				Body:     "Fix it",
			},
		},
	}
	files := []model.FileChange{
		{Path: "a.go", Patch: "@@ -1,2 +1,2 @@\n line1\n+line2"},
	}

	err := o.postReview(context.Background(), "o", "r", 1, result, files)
	require.NoError(t, err)
	require.Len(t, client.postedCommentBodies, 1)
	require.Len(t, client.postedReviews, 1)
	assert.Equal(t, "COMMENT", client.postedReviews[0].Event)
	assert.Equal(t, output.ScopedCommentMarker("SURGE", output.CommentScopeInline), client.postedReviews[0].Body)
}

func TestIsSurgeCommentRecognizesCurrentAndLegacyMarkers(t *testing.T) {
	cfg := &config.Config{CommentMarker: "SURGE"}
	o := NewOrchestrator(nil, &stubPRClient{}, cfg)

	assert.True(t, o.isSurgeComment("<!-- SURGE -->"))
	assert.True(t, o.isSurgeComment("<!-- SURGE_SUMMARY -->"))
	assert.True(t, o.isSurgeComment(output.ScopedCommentMarker("SURGE", output.CommentScopeInline)))
	assert.False(t, o.isSurgeComment("plain comment"))
}

func TestOrchestratorFilterCategoriesUsesEnabledConfig(t *testing.T) {
	o := NewOrchestrator(nil, &stubPRClient{}, &config.Config{
		Categories: config.CategoriesConfig{
			Security:        true,
			Performance:     false,
			Logic:           true,
			Maintainability: false,
			Vibe:            false,
		},
	})

	prompt := o.filterCategories()

	assert.Contains(t, prompt, "- security:")
	assert.Contains(t, prompt, "- logic:")
	assert.NotContains(t, prompt, "- performance:")
	assert.NotContains(t, prompt, "- maintainability:")
	assert.NotContains(t, prompt, "- vibe:")
}

func TestEnrichPRContextLoadsFullFileContent(t *testing.T) {
	client := &stubPRClient{
		fileContents: map[string]string{
			"internal/app.go@abc123": "package app\n",
		},
	}
	o := NewOrchestrator(nil, client, &config.Config{})
	pr := &model.PR{HeadSHA: "abc123"}
	prCtx := &PRContext{
		Files: []FileContext{
			{Path: "internal/app.go", Status: string(model.FileStatusModified)},
			{Path: "internal/deleted.go", Status: string(model.FileStatusDeleted)},
		},
	}

	warnings, err := o.enrichPRContext(context.Background(), "octo", "surge", pr, prCtx, ContextDepthFull)
	require.NoError(t, err)
	assert.Empty(t, warnings)

	assert.Equal(t, "package app\n", prCtx.Files[0].Content)
	assert.Empty(t, prCtx.Files[1].Content)
}

func TestEnrichPRContextContinuesWhenFileContentLookupFails(t *testing.T) {
	client := &stubPRClient{
		fileContents: map[string]string{
			"internal/ok.go@abc123": "package ok\n",
		},
		fileContentErrs: map[string]error{
			"internal/missing.go@abc123": errors.New("not found"),
		},
	}
	o := NewOrchestrator(nil, client, &config.Config{})
	pr := &model.PR{HeadSHA: "abc123"}
	prCtx := &PRContext{
		Files: []FileContext{
			{Path: "internal/ok.go", Status: string(model.FileStatusModified)},
			{Path: "internal/missing.go", Status: string(model.FileStatusModified)},
		},
	}

	warnings, err := o.enrichPRContext(context.Background(), "octo", "surge", pr, prCtx, ContextDepthFull)
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Equal(t, "internal/missing.go", warnings[0].path)

	assert.Equal(t, "package ok\n", prCtx.Files[0].Content)
	assert.Empty(t, prCtx.Files[1].Content)
}

func TestFormatContextWarnings(t *testing.T) {
	warnings := formatContextWarnings([]contextWarning{
		{path: "a.go", err: errors.New("not found")},
		{path: "b.go", err: errors.New("forbidden")},
	})

	require.Len(t, warnings, 1)
	assert.Equal(t, "Full context was only partially loaded; skipped 2 file(s): a.go, b.go.", warnings[0])
}
