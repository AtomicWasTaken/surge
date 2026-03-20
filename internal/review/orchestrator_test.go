package review

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/AtomicWasTaken/surge/internal/ai"
	"github.com/AtomicWasTaken/surge/internal/config"
	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/AtomicWasTaken/surge/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAIClient struct {
	response *ai.CompletionResponse
	err      error
	req      *ai.CompletionRequest
}

func (s *stubAIClient) Complete(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
	s.req = req
	if s.err != nil {
		return nil, s.err
	}
	return s.response, nil
}

func (s *stubAIClient) Name() string {
	return "stub"
}

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
	pr                      *model.PR
	prErr                   error
	files                   []model.FileChange
	filesErr                error
	labels                  []string
	listLabelsErr           error
	addLabelsCalls          [][]string
	removeLabelCalls        []string
	upsertedLabels          []labelSpec
	addLabelsErr            error
	removeLabelErr          error
	upsertLabelErr          error
}

func (s *stubPRClient) GetPR(ctx context.Context, owner, repo string, prNumber int) (*model.PR, error) {
	if s.prErr != nil {
		return nil, s.prErr
	}
	return s.pr, nil
}

func (s *stubPRClient) GetDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	return "", nil
}

func (s *stubPRClient) GetFiles(ctx context.Context, owner, repo string, prNumber int) ([]model.FileChange, error) {
	if s.filesErr != nil {
		return nil, s.filesErr
	}
	return s.files, nil
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
	if s.listLabelsErr != nil {
		return nil, s.listLabelsErr
	}
	return s.labels, nil
}

func (s *stubPRClient) AddLabels(ctx context.Context, owner, repo string, prNumber int, labels []string) error {
	if s.addLabelsErr != nil {
		return s.addLabelsErr
	}
	s.addLabelsCalls = append(s.addLabelsCalls, append([]string(nil), labels...))
	return nil
}

func (s *stubPRClient) RemoveLabel(ctx context.Context, owner, repo string, prNumber int, label string) error {
	if s.removeLabelErr != nil {
		return s.removeLabelErr
	}
	s.removeLabelCalls = append(s.removeLabelCalls, label)
	return nil
}

func (s *stubPRClient) UpsertLabel(ctx context.Context, owner, repo, name, color, description string) error {
	if s.upsertLabelErr != nil {
		return s.upsertLabelErr
	}
	s.upsertedLabels = append(s.upsertedLabels, labelSpec{Name: name, Color: color, Description: description})
	return nil
}

func TestReviewEndToEndDryRun(t *testing.T) {
	aiClient := &stubAIClient{
		response: &ai.CompletionResponse{
			Content:   `{"summary":"Looks good overall.","filesOverview":[{"path":"a.go","changes":"change","risk":"low"}],"findings":[{"severity":"medium","category":"logic","file":"a.go","line":2,"title":"Handle edge case","body":"A branch is missing."}],"vibeCheck":{"score":10,"verdict":"Perfect","flags":[]},"recommendations":["Add a regression test"],"approve":false}`,
			TokensIn:  12,
			TokensOut: 34,
		},
	}
	ghClient := &stubPRClient{
		pr: &model.PR{Title: "PR title", Body: "PR body", ChangedFiles: 1, HeadSHA: "abc123"},
		files: []model.FileChange{
			{Path: "a.go", Status: model.FileStatusModified, Additions: 1, Deletions: 0, Patch: "@@ -1,2 +1,2 @@\n line1\n+line2"},
		},
	}
	cfg := &config.Config{
		AI:                config.AIConfig{Model: "test-model"},
		ContextDepth:      string(ContextDepthFull),
		Output:            config.OutputConfig{Format: "json"},
		Categories:        config.CategoriesConfig{Security: true, Performance: true, Logic: true, Maintainability: true, Vibe: true},
		CommentMarker:     "SURGE",
		MaxTokens:         99,
		Temperature:       0.4,
		EnablePRLabels:    true,
		PRLabelPrefix:     "surge",
		MaxInlineComments: 10,
		Verbose:           true,
	}
	ghClient.fileContents = map[string]string{"a.go@abc123": "package main\n"}
	o := NewOrchestrator(aiClient, ghClient, cfg)

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	result, err := o.Review(context.Background(), "octo", "surge", 1, true)

	require.NoError(t, w.Close())
	os.Stdout = stdout
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, aiClient.req)

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	assert.Equal(t, "test-model", aiClient.req.Model)
	assert.Contains(t, aiClient.req.Messages[0].Content, "Full file content")
	assert.Contains(t, buf.String(), `"summary": "Looks good overall."`)
	assert.Equal(t, 1, result.Stats.FilesReviewed)
	assert.Equal(t, 12, result.Stats.TokensIn)
	assert.Contains(t, result.VibeCheck.Flags, "ai_fluff")
	assert.Equal(t, "Excellent. Hand-crafted, idiomatic code.", result.VibeCheck.Verdict)
}

func TestReviewEndToEndPostAndTerminal(t *testing.T) {
	aiClient := &stubAIClient{
		response: &ai.CompletionResponse{
			Content:   `{"summary":"Plain summary.","findings":[{"severity":"high","category":"logic","file":"a.go","line":2,"title":"Issue","body":"Fix it"}],"vibeCheck":{"score":6,"verdict":"ok","flags":[]},"recommendations":["Ship it"],"approve":true}`,
			TokensIn:  5,
			TokensOut: 7,
		},
	}
	ghClient := &stubPRClient{
		pr:             &model.PR{Title: "PR title", HeadSHA: "abc123"},
		files:          []model.FileChange{{Path: "a.go", Status: model.FileStatusModified, Patch: "@@ -1,2 +1,2 @@\n line1\n+line2"}},
		reviewComments: map[int64][]*model.PRReviewComment{},
	}
	cfg := &config.Config{
		AI:             config.AIConfig{Model: "test-model"},
		Output:         config.OutputConfig{Format: "terminal"},
		Categories:     config.CategoriesConfig{Security: true, Performance: true, Logic: true, Maintainability: true, Vibe: true},
		CommentMarker:  "SURGE",
		EnablePRLabels: false,
		ContextDepth:   string(ContextDepthDiffOnly),
	}
	o := NewOrchestrator(aiClient, ghClient, cfg)

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	result, err := o.Review(context.Background(), "octo", "surge", 1, false)

	require.NoError(t, w.Close())
	os.Stdout = stdout
	require.NoError(t, err)
	assert.True(t, result.Approve)
	require.Len(t, ghClient.postedCommentBodies, 1)
	require.Len(t, ghClient.postedReviews, 1)

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Contains(t, buf.String(), "Vibe Check")
}

func TestReviewErrors(t *testing.T) {
	cfg := &config.Config{
		AI:            config.AIConfig{Model: "test-model"},
		ContextDepth:  string(ContextDepthDiffOnly),
		Output:        config.OutputConfig{Format: "terminal"},
		Categories:    config.CategoriesConfig{Security: true},
		CommentMarker: "SURGE",
	}

	t.Run("pr fetch", func(t *testing.T) {
		o := NewOrchestrator(&stubAIClient{}, &stubPRClient{prErr: errors.New("boom")}, cfg)
		_, err := o.Review(context.Background(), "octo", "surge", 1, true)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to fetch PR")
	})

	t.Run("files fetch", func(t *testing.T) {
		o := NewOrchestrator(&stubAIClient{}, &stubPRClient{pr: &model.PR{}, filesErr: errors.New("boom")}, cfg)
		_, err := o.Review(context.Background(), "octo", "surge", 1, true)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to fetch files")
	})

	t.Run("ai failure", func(t *testing.T) {
		o := NewOrchestrator(&stubAIClient{err: errors.New("boom")}, &stubPRClient{pr: &model.PR{}, files: nil}, cfg)
		_, err := o.Review(context.Background(), "octo", "surge", 1, true)
		require.Error(t, err)
		assert.ErrorContains(t, err, "AI request failed")
	})

	t.Run("parse failure", func(t *testing.T) {
		o := NewOrchestrator(&stubAIClient{response: &ai.CompletionResponse{Content: `not json`}}, &stubPRClient{pr: &model.PR{}, files: nil}, cfg)
		_, err := o.Review(context.Background(), "octo", "surge", 1, true)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to parse AI response")
	})

	t.Run("post review failure", func(t *testing.T) {
		client := &stubPRClientWithPostError{
			stubPRClient: stubPRClient{
				pr:    &model.PR{},
				files: []model.FileChange{},
			},
			postCommentErr: errors.New("boom"),
		}
		o := NewOrchestrator(&stubAIClient{response: &ai.CompletionResponse{
			Content: `{"summary":"ok","vibeCheck":{"score":5,"verdict":"ok","flags":[]},"recommendations":[],"approve":true}`,
		}}, client, cfg)
		_, err := o.Review(context.Background(), "octo", "surge", 1, false)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to post review")
	})
}

func TestBuildPRContextAndHelpers(t *testing.T) {
	o := NewOrchestrator(nil, &stubPRClient{}, &config.Config{CommentMarker: "SURGE"})
	prCtx := o.buildPRContext(&model.PR{Title: "Title", Body: "Body"}, []model.FileChange{
		{Path: "a.go", Status: model.FileStatusAdded, Additions: 2, Deletions: 0, Patch: "@@"},
	})

	assert.Equal(t, "Title", prCtx.Title)
	assert.Len(t, prCtx.Files, 1)
	assert.Equal(t, "a.go", prCtx.Files[0].Path)
	assert.True(t, supportsFileContent(string(model.FileStatusModified)))
	assert.False(t, supportsFileContent(string(model.FileStatusDeleted)))
}

func TestSyncPRLabelsAndHelpers(t *testing.T) {
	client := &stubPRClient{
		labels: []string{"Surge: Reviewed", "Surge: Decision / Changes Requested", "other"},
	}
	o := NewOrchestrator(nil, client, &config.Config{
		CommentMarker:  "SURGE",
		EnablePRLabels: true,
		PRLabelPrefix:  "surge",
	})
	result := &model.ReviewResult{
		Approve: true,
		Findings: []model.Finding{
			{Severity: model.SeverityHigh},
			{Severity: model.SeverityMedium},
		},
		Stats: model.ReviewStats{FilesReviewed: 9},
	}

	err := o.syncPRLabels(context.Background(), "octo", "surge", 1, result)
	require.NoError(t, err)
	require.Len(t, client.upsertedLabels, 4)
	assert.Equal(t, []string{"Surge: Decision / Changes Requested"}, client.removeLabelCalls)
	require.Len(t, client.addLabelsCalls, 1)
	assert.Len(t, client.addLabelsCalls[0], 4)

	assert.True(t, isManagedSurgeLabel("surge", "Surge: Reviewed"))
	assert.False(t, isManagedSurgeLabel("surge", "other"))
	assert.Equal(t, "high", classifyReviewEffort(&model.ReviewResult{
		Findings: []model.Finding{{Severity: model.SeverityCritical}},
	}))
	assert.Equal(t, "medium", classifyReviewEffort(&model.ReviewResult{
		Stats: model.ReviewStats{FilesReviewed: 8},
	}))
	assert.Equal(t, "low", classifyReviewEffort(&model.ReviewResult{}))
	assert.Equal(t, "Approved", decisionTitle("approved"))
	assert.Equal(t, "Changes Requested", decisionTitle("other"))
	assert.Equal(t, "None", findingsTitle("none found"))
	assert.Equal(t, "Present", findingsTitle("present"))
	assert.Equal(t, "", titleWord(""))
}

func TestSyncPRLabelsErrorsAndEnabledCategories(t *testing.T) {
	cfg := &config.Config{
		CommentMarker:  "SURGE",
		EnablePRLabels: true,
		PRLabelPrefix:  "",
		Categories: config.CategoriesConfig{
			Security: true,
			Vibe:     true,
		},
	}
	o := NewOrchestrator(nil, &stubPRClient{upsertLabelErr: errors.New("boom")}, cfg)
	err := o.syncPRLabels(context.Background(), "octo", "surge", 1, &model.ReviewResult{})
	require.Error(t, err)
	assert.ElementsMatch(t, []model.Category{model.CategorySecurity, model.CategoryVibe}, o.enabledCategories())
}

func TestSyncPRLabelsListRemoveAndAddErrors(t *testing.T) {
	result := &model.ReviewResult{}

	t.Run("list labels error", func(t *testing.T) {
		o := NewOrchestrator(nil, &stubPRClient{listLabelsErr: errors.New("boom")}, &config.Config{
			CommentMarker: "SURGE", EnablePRLabels: true,
		})
		err := o.syncPRLabels(context.Background(), "o", "r", 1, result)
		require.Error(t, err)
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("remove label error", func(t *testing.T) {
		o := NewOrchestrator(nil, &stubPRClient{
			labels:         []string{"Surge: Findings / Present"},
			removeLabelErr: errors.New("boom"),
		}, &config.Config{CommentMarker: "SURGE", EnablePRLabels: true})
		err := o.syncPRLabels(context.Background(), "o", "r", 1, result)
		require.Error(t, err)
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("add labels error", func(t *testing.T) {
		o := NewOrchestrator(nil, &stubPRClient{
			addLabelsErr: errors.New("boom"),
		}, &config.Config{CommentMarker: "SURGE", EnablePRLabels: true})
		err := o.syncPRLabels(context.Background(), "o", "r", 1, result)
		require.Error(t, err)
		assert.ErrorContains(t, err, "boom")
	})
}

func TestReviewEmptyDepthAndEnrichError(t *testing.T) {
	prev := enrichContext
	t.Cleanup(func() { enrichContext = prev })

	aiClient := &stubAIClient{
		response: &ai.CompletionResponse{
			Content: `{"summary":"ok","vibeCheck":{"score":5,"verdict":"ok","flags":[]},"recommendations":[],"approve":true}`,
		},
	}
	ghClient := &stubPRClient{
		pr:    &model.PR{Title: "PR title"},
		files: []model.FileChange{},
	}
	o := NewOrchestrator(aiClient, ghClient, &config.Config{
		AI:            config.AIConfig{Model: "m"},
		Output:        config.OutputConfig{Format: "json"},
		Categories:    config.CategoriesConfig{Security: true},
		CommentMarker: "SURGE",
		ContextDepth:  "",
	})

	_, err := o.Review(context.Background(), "o", "r", 1, true)
	require.NoError(t, err)
	assert.Contains(t, aiClient.req.Messages[0].Content, "Diff:")

	enrichContext = func(o *Orchestrator, ctx context.Context, owner, repo string, pr *model.PR, prCtx *PRContext, depth ContextDepth) ([]contextWarning, error) {
		return nil, errors.New("boom")
	}
	_, err = o.Review(context.Background(), "o", "r", 1, true)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to load PR context")
}

func TestDeleteOldCommentsListReviewCommentsError(t *testing.T) {
	client := &stubPRClient{
		reviews:                []*model.PRReview{{ID: 1, Body: "<!-- SURGE_INLINE -->", State: "COMMENTED"}},
		listReviewCommentsErrs: map[int64]error{1: errors.New("boom")},
	}
	o := NewOrchestrator(nil, client, &config.Config{CommentMarker: "SURGE"})
	stats, err := o.deleteOldComments(context.Background(), "o", "r", 1)
	require.Error(t, err)
	assert.Equal(t, 1, stats.failedOperations)
	assert.ErrorContains(t, err, "list review comments for review 1")
}

func TestTitleWordInvalidRune(t *testing.T) {
	assert.Equal(t, string([]byte{0xff}), titleWord(string([]byte{0xff})))
}

func TestPostReviewBranchesAndErrors(t *testing.T) {
	t.Run("approve event and label warning", func(t *testing.T) {
		client := &stubPRClient{
			reviewComments: map[int64][]*model.PRReviewComment{},
			listLabelsErr:  errors.New("labels unavailable"),
		}
		cfg := &config.Config{
			CommentMarker:         "SURGE",
			EnablePRLabels:        true,
			DisableSummaryComment: true,
			MaxInlineComments:     1,
		}
		o := NewOrchestrator(nil, client, cfg)
		result := &model.ReviewResult{
			Approve: true,
			Findings: []model.Finding{
				{Severity: model.SeverityHigh, File: "a.go", Line: 2, Title: "Bug", Body: "Fix"},
				{Severity: model.SeverityHigh, File: "a.go", Line: 2, Title: "Bug2", Body: "Fix"},
			},
		}
		files := []model.FileChange{{Path: "a.go", Patch: "@@ -1,2 +1,2 @@\n line1\n+line2"}}

		stdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w

		err = o.postReview(context.Background(), "o", "r", 1, result, files)

		require.NoError(t, w.Close())
		os.Stdout = stdout
		require.NoError(t, err)
		require.Len(t, client.postedReviews, 1)
		assert.Equal(t, "APPROVE", client.postedReviews[0].Event)
		assert.Len(t, client.postedReviews[0].Comments, 1)

		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		assert.Contains(t, buf.String(), "Warning: failed to sync PR labels")
	})

	t.Run("summary post error", func(t *testing.T) {
		client := &stubPRClientWithPostError{stubPRClient: stubPRClient{}, postCommentErr: errors.New("boom")}
		o := NewOrchestrator(nil, client, &config.Config{CommentMarker: "SURGE"})
		err := o.postReview(context.Background(), "o", "r", 1, &model.ReviewResult{}, nil)
		require.Error(t, err)
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("review post error", func(t *testing.T) {
		client := &stubPRClientWithPostError{stubPRClient: stubPRClient{}, postReviewErr: errors.New("boom")}
		o := NewOrchestrator(nil, client, &config.Config{CommentMarker: "SURGE", DisableSummaryComment: true})
		err := o.postReview(context.Background(), "o", "r", 1, &model.ReviewResult{
			Findings: []model.Finding{{Severity: model.SeverityHigh, File: "a.go", Line: 2, Title: "Bug", Body: "Fix"}},
		}, []model.FileChange{{Path: "a.go", Patch: "@@ -1,2 +1,2 @@\n line1\n+line2"}})
		require.Error(t, err)
		assert.ErrorContains(t, err, "boom")
	})
}

type stubPRClientWithPostError struct {
	stubPRClient
	postCommentErr error
	postReviewErr  error
}

func (s *stubPRClientWithPostError) PostComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	if s.postCommentErr != nil {
		return s.postCommentErr
	}
	return s.stubPRClient.PostComment(ctx, owner, repo, prNumber, body)
}

func (s *stubPRClientWithPostError) PostReview(ctx context.Context, owner, repo string, prNumber int, review *model.ReviewInput) error {
	if s.postReviewErr != nil {
		return s.postReviewErr
	}
	return s.stubPRClient.PostReview(ctx, owner, repo, prNumber, review)
}

func TestBuildInlineCommentsSkipsUnmappableFindings(t *testing.T) {
	o := NewOrchestrator(nil, &stubPRClient{}, &config.Config{CommentMarker: "SURGE"})
	comments := o.buildInlineComments(&model.ReviewResult{
		Findings: []model.Finding{
			{Severity: model.SeverityHigh, File: "", Line: 2, Title: "skip", Body: "skip"},
			{Severity: model.SeverityHigh, File: "a.go", Line: 0, Title: "skip", Body: "skip"},
			{Severity: model.SeverityHigh, File: "a.go", Line: 9, Title: "skip", Body: "skip"},
		},
	}, []model.FileChange{{Path: "a.go", Patch: "@@ -1,2 +1,2 @@\n line1\n+line2"}})
	assert.Empty(t, comments)
}

func TestDeleteOldCommentsListFailures(t *testing.T) {
	client := &stubPRClient{
		listCommentsErr: errors.New("boom comments"),
		listReviewsErr:  errors.New("boom reviews"),
	}
	o := NewOrchestrator(nil, client, &config.Config{CommentMarker: "SURGE"})
	stats, err := o.deleteOldComments(context.Background(), "o", "r", 1)
	require.Error(t, err)
	assert.Equal(t, 2, stats.failedOperations)
	assert.ErrorContains(t, err, "list issue comments")
	assert.ErrorContains(t, err, "list reviews")
}

func TestFindPositionInPatchBranches(t *testing.T) {
	assert.Equal(t, 0, findPositionInPatch("", 1))
	assert.Equal(t, 3, findPositionInPatch("@@ -1,2 +1,2 @@\n line1\n+line2", 2))
	assert.Equal(t, 0, findPositionInPatch("@@ -1,2 +1,2 @@\n-line1\n line2", 1))
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

func TestDeleteOldCommentsSkipsDismissedReviews(t *testing.T) {
	client := &stubPRClient{
		reviews: []*model.PRReview{
			{ID: 40, Body: "<!-- SURGE_INLINE -->", State: "DISMISSED"},
		},
		reviewComments: map[int64][]*model.PRReviewComment{
			40: {},
		},
	}
	o := NewOrchestrator(nil, client, &config.Config{CommentMarker: "SURGE"})
	stats, err := o.deleteOldComments(context.Background(), "o", "r", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.skippedReviews)
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

	require.Len(t, warnings, 2)
	assert.Equal(t, "Full context was only partially loaded; skipped 2 file(s): a.go, b.go.", warnings[0])
	assert.Equal(t, "Skip reasons: a.go (not found), b.go (permission denied).", warnings[1])
}

func TestBuildInlineCommentsWithSuggestion(t *testing.T) {
	cfg := &config.Config{CommentMarker: "SURGE"}
	o := NewOrchestrator(nil, &stubPRClient{}, cfg)

	result := &model.ReviewResult{
		Findings: []model.Finding{
			{
				Severity:   model.SeverityHigh,
				File:       "main.go",
				Line:       2,
				Title:      "SQL injection <script>",
				Body:       "User input is <b>unsanitized</b>",
				Suggestion: "Parameterize the query in `handler()` to prevent injection.",
			},
			{
				Severity: model.SeverityLow,
				File:     "main.go",
				Line:     2,
				Title:    "No suggestion finding",
				Body:     "Minor style issue.",
			},
		},
	}
	files := []model.FileChange{
		{Path: "main.go", Patch: "@@ -1,2 +1,2 @@\n line1\n+line2"},
	}

	comments := o.buildInlineComments(result, files)
	require.Len(t, comments, 2)

	// First comment: has suggestion, HTML escaped
	assert.Contains(t, comments[0].Body, "🟠")
	assert.Contains(t, comments[0].Body, "&lt;script&gt;")
	assert.Contains(t, comments[0].Body, "&lt;b&gt;unsanitized&lt;/b&gt;")
	assert.Contains(t, comments[0].Body, "🤖 Agent fix prompt:")
	assert.Contains(t, comments[0].Body, "Parameterize the query")

	// Second comment: no suggestion block
	assert.NotContains(t, comments[1].Body, "Agent fix prompt")
}

func TestSanitizeContextWarningReason(t *testing.T) {
	assert.Equal(t, "not found", sanitizeContextWarningReason(errors.New("file not found")))
	assert.Equal(t, "permission denied", sanitizeContextWarningReason(errors.New("forbidden by policy")))
	assert.Equal(t, "timeout", sanitizeContextWarningReason(errors.New("deadline exceeded")))
	assert.Equal(t, "request failed", sanitizeContextWarningReason(errors.New("bad gateway")))
	assert.Equal(t, "unknown error", sanitizeContextWarningReason(nil))
}
