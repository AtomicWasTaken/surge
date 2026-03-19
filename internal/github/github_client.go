package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/AtomicWasTaken/surge/internal/model"
)

const (
	githubJSONAcceptHeader = "application/vnd.github.v3+json"
	githubDiffAcceptHeader = "application/vnd.github.v3.diff"
	githubUserAgent        = "surge-ai-review/1.0"
)

// GitHubClient implements PRClient for GitHub.
type GitHubClient struct {
	client    *http.Client
	apiURL    string
	authToken string
}

// NewGitHubClient creates a new GitHub API client.
func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		client:    &http.Client{Timeout: 30 * time.Second},
		apiURL:    "https://api.github.com",
		authToken: token,
	}
}

func (c *GitHubClient) apiURLf(format string, args ...interface{}) string {
	return fmt.Sprintf(c.apiURL+format, args...)
}

func (c *GitHubClient) doRequest(ctx context.Context, method, requestURL, acceptHeader string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Authorization", "token "+c.authToken)
	req.Header.Set("User-Agent", githubUserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

func (c *GitHubClient) doJSONRequest(ctx context.Context, method, requestURL string, body interface{}) ([]byte, int, error) {
	return c.doRequest(ctx, method, requestURL, githubJSONAcceptHeader, body)
}

func parseJSONResponse[T any](body []byte, target *T, operation string) error {
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("failed to parse %s: %w", operation, err)
	}
	return nil
}

func expectStatus(status int, allowed ...int) bool {
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
}

func githubAPIError(status int, body []byte) error {
	return fmt.Errorf("GitHub API error (%d): %s", status, string(body))
}

// GetPR fetches the metadata for a pull request.
func (c *GitHubClient) GetPR(ctx context.Context, owner, repo string, prNumber int) (*model.PR, error) {
	requestURL := c.apiURLf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)

	body, status, err := c.doJSONRequest(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	if status == http.StatusNotFound {
		return nil, fmt.Errorf("PR not found: %s/%s#%d", owner, repo, prNumber)
	}
	if status == http.StatusForbidden || status == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: insufficient permissions or invalid token")
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error (%d): %s", status, string(body))
	}

	var prResp struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		User   struct {
			Login string `json:"login"`
		} `json:"user"`
		Base struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Additions    int       `json:"additions"`
		Deletions    int       `json:"deletions"`
		ChangedFiles int       `json:"changed_files"`
		URL          string    `json:"html_url"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
	}

	if err := parseJSONResponse(body, &prResp, "PR response"); err != nil {
		return nil, err
	}

	return &model.PR{
		Number:       prResp.Number,
		Title:        prResp.Title,
		Body:         prResp.Body,
		State:        prResp.State,
		Author:       prResp.User.Login,
		BaseRef:      prResp.Base.Ref,
		HeadRef:      prResp.Head.Ref,
		BaseSHA:      prResp.Base.SHA,
		HeadSHA:      prResp.Head.SHA,
		Additions:    prResp.Additions,
		Deletions:    prResp.Deletions,
		ChangedFiles: prResp.ChangedFiles,
		URL:          prResp.URL,
		CreatedAt:    prResp.CreatedAt,
		UpdatedAt:    prResp.UpdatedAt,
	}, nil
}

// GetDiff fetches the unified diff for a PR.
func (c *GitHubClient) GetDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	requestURL := c.apiURLf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	body, status, err := c.doRequest(ctx, http.MethodGet, requestURL, githubDiffAcceptHeader, nil)
	if err != nil {
		return "", err
	}

	if status != http.StatusOK {
		return "", githubAPIError(status, body)
	}

	return string(body), nil
}

// GetFiles fetches the list of changed files with their patches.
func (c *GitHubClient) GetFiles(ctx context.Context, owner, repo string, prNumber int) ([]model.FileChange, error) {
	requestURL := c.apiURLf("/repos/%s/%s/pulls/%d/files", owner, repo, prNumber)

	body, status, err := c.doJSONRequest(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		return nil, githubAPIError(status, body)
	}

	var files []struct {
		SHA       string `json:"sha"`
		Filename  string `json:"filename"`
		Status    string `json:"status"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Patch     string `json:"patch"`
	}

	if err := parseJSONResponse(body, &files, "files response"); err != nil {
		return nil, err
	}

	result := make([]model.FileChange, len(files))
	for i, f := range files {
		result[i] = model.FileChange{
			Path:      f.Filename,
			Status:    model.FileStatus(f.Status),
			Additions: f.Additions,
			Deletions: f.Deletions,
			Patch:     f.Patch,
		}
	}

	return result, nil
}

// GetFileContent fetches the content of a specific file at a given ref.
func (c *GitHubClient) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	requestURL := c.apiURLf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, ref)

	body, status, err := c.doJSONRequest(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}

	if status == http.StatusNotFound {
		return "", fmt.Errorf("file not found: %s at %s", path, ref)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("GitHub API error (%d): %s", status, string(body))
	}

	var fileResp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}

	if err := parseJSONResponse(body, &fileResp, "file response"); err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(fileResp.Content)
	if err != nil {
		return "", fmt.Errorf("failed to decode file content: %w", err)
	}

	return string(decoded), nil
}

// PostReview posts a review with inline comments and a summary.
func (c *GitHubClient) PostReview(ctx context.Context, owner, repo string, prNumber int, review *model.ReviewInput) error {
	requestURL := c.apiURLf("/repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)

	payload := map[string]interface{}{
		"event": review.Event,
		"body":  review.Body,
	}

	if len(review.Comments) > 0 {
		comments := make([]map[string]interface{}, len(review.Comments))
		for i, c := range review.Comments {
			comments[i] = map[string]interface{}{
				"path":     c.Path,
				"position": c.Position,
				"body":     c.Body,
			}
		}
		payload["comments"] = comments
	}

	body, status, err := c.doJSONRequest(ctx, http.MethodPost, requestURL, payload)
	if err != nil {
		return err
	}

	if status == http.StatusUnprocessableEntity {
		return fmt.Errorf("review position is stale (lines may have moved since the diff was generated)")
	}
	if !expectStatus(status, http.StatusOK, http.StatusCreated) {
		return githubAPIError(status, body)
	}

	return nil
}

// PostComment posts a general comment on the PR.
func (c *GitHubClient) PostComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	requestURL := c.apiURLf("/repos/%s/%s/issues/%d/comments", owner, repo, prNumber)

	payload := map[string]string{"body": body}

	respBody, status, err := c.doJSONRequest(ctx, http.MethodPost, requestURL, payload)
	if err != nil {
		return err
	}

	if status != http.StatusCreated {
		return githubAPIError(status, respBody)
	}

	return nil
}

// ListComments lists comments on a PR.
func (c *GitHubClient) ListComments(ctx context.Context, owner, repo string, prNumber int) ([]*model.PRComment, error) {
	requestURL := c.apiURLf("/repos/%s/%s/issues/%d/comments", owner, repo, prNumber)

	body, status, err := c.doJSONRequest(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		return nil, githubAPIError(status, body)
	}

	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
		User struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
		CreatedAt string `json:"created_at"`
	}

	if err := parseJSONResponse(body, &comments, "comments"); err != nil {
		return nil, err
	}

	result := make([]*model.PRComment, len(comments))
	for i, c := range comments {
		result[i] = &model.PRComment{
			ID:        c.ID,
			Body:      c.Body,
			Author:    c.User.Login,
			IsBot:     c.User.Type == "Bot",
			CreatedAt: c.CreatedAt,
		}
	}

	return result, nil
}

// DeleteComment deletes a comment by ID.
func (c *GitHubClient) DeleteComment(ctx context.Context, owner, repo string, commentID int64) error {
	requestURL := c.apiURLf("/repos/%s/%s/issues/comments/%d", owner, repo, commentID)

	_, status, err := c.doJSONRequest(ctx, http.MethodDelete, requestURL, nil)
	if err != nil {
		return err
	}

	if status != http.StatusNoContent {
		return fmt.Errorf("GitHub API error (%d)", status)
	}

	return nil
}

// ListReviews lists submitted reviews on a PR.
func (c *GitHubClient) ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]*model.PRReview, error) {
	requestURL := c.apiURLf("/repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)

	body, status, err := c.doJSONRequest(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		return nil, githubAPIError(status, body)
	}

	var reviews []struct {
		ID    int64  `json:"id"`
		Body  string `json:"body"`
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
		CreatedAt string `json:"created_at"`
	}

	if err := parseJSONResponse(body, &reviews, "reviews"); err != nil {
		return nil, err
	}

	result := make([]*model.PRReview, len(reviews))
	for i, r := range reviews {
		result[i] = &model.PRReview{
			ID:        r.ID,
			Body:      r.Body,
			State:     r.State,
			Author:    r.User.Login,
			IsBot:     r.User.Type == "Bot",
			CreatedAt: r.CreatedAt,
		}
	}

	return result, nil
}

// DeleteReview deletes a review by ID.
func (c *GitHubClient) DeleteReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64) error {
	requestURL := c.apiURLf("/repos/%s/%s/pulls/%d/reviews/%d", owner, repo, prNumber, reviewID)

	body, status, err := c.doJSONRequest(ctx, http.MethodDelete, requestURL, nil)
	if err != nil {
		return err
	}

	if !expectStatus(status, http.StatusOK, http.StatusNoContent) {
		return githubAPIError(status, body)
	}

	return nil
}

// DismissReview dismisses a submitted review so reruns can supersede it.
func (c *GitHubClient) DismissReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64, message string) error {
	requestURL := c.apiURLf("/repos/%s/%s/pulls/%d/reviews/%d/dismissals", owner, repo, prNumber, reviewID)

	payload := map[string]string{
		"message": message,
	}

	body, status, err := c.doJSONRequest(ctx, http.MethodPut, requestURL, payload)
	if err != nil {
		return err
	}

	if !expectStatus(status, http.StatusOK, http.StatusCreated) {
		return githubAPIError(status, body)
	}

	return nil
}

// ListReviewComments lists inline comments for a specific PR review.
func (c *GitHubClient) ListReviewComments(ctx context.Context, owner, repo string, prNumber int, reviewID int64) ([]*model.PRReviewComment, error) {
	requestURL := c.apiURLf("/repos/%s/%s/pulls/%d/reviews/%d/comments", owner, repo, prNumber, reviewID)

	body, status, err := c.doJSONRequest(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		return nil, githubAPIError(status, body)
	}

	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}

	if err := parseJSONResponse(body, &comments, "review comments"); err != nil {
		return nil, err
	}

	result := make([]*model.PRReviewComment, len(comments))
	for i, c := range comments {
		result[i] = &model.PRReviewComment{
			ID:   c.ID,
			Body: c.Body,
		}
	}

	return result, nil
}

// DeleteReviewComment deletes a PR inline review comment by ID.
func (c *GitHubClient) DeleteReviewComment(ctx context.Context, owner, repo string, commentID int64) error {
	requestURL := c.apiURLf("/repos/%s/%s/pulls/comments/%d", owner, repo, commentID)

	body, status, err := c.doJSONRequest(ctx, http.MethodDelete, requestURL, nil)
	if err != nil {
		return err
	}

	if status != http.StatusNoContent {
		return githubAPIError(status, body)
	}

	return nil
}

// ListLabels lists labels on a PR (issues API).
func (c *GitHubClient) ListLabels(ctx context.Context, owner, repo string, prNumber int) ([]string, error) {
	requestURL := c.apiURLf("/repos/%s/%s/issues/%d/labels", owner, repo, prNumber)

	body, status, err := c.doJSONRequest(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		return nil, githubAPIError(status, body)
	}

	var labels []struct {
		Name string `json:"name"`
	}
	if err := parseJSONResponse(body, &labels, "labels"); err != nil {
		return nil, err
	}

	result := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.Name == "" {
			continue
		}
		result = append(result, l.Name)
	}

	return result, nil
}

// AddLabels adds labels to a PR (issues API).
func (c *GitHubClient) AddLabels(ctx context.Context, owner, repo string, prNumber int, labels []string) error {
	if len(labels) == 0 {
		return nil
	}

	requestURL := c.apiURLf("/repos/%s/%s/issues/%d/labels", owner, repo, prNumber)
	payload := map[string][]string{
		"labels": labels,
	}

	body, status, err := c.doJSONRequest(ctx, http.MethodPost, requestURL, payload)
	if err != nil {
		return err
	}

	if !expectStatus(status, http.StatusOK, http.StatusCreated) {
		return githubAPIError(status, body)
	}

	return nil
}

// RemoveLabel removes a single label from a PR (issues API).
func (c *GitHubClient) RemoveLabel(ctx context.Context, owner, repo string, prNumber int, label string) error {
	requestURL := c.apiURLf("/repos/%s/%s/issues/%d/labels/%s", owner, repo, prNumber, url.PathEscape(label))

	body, status, err := c.doJSONRequest(ctx, http.MethodDelete, requestURL, nil)
	if err != nil {
		return err
	}

	if !expectStatus(status, http.StatusOK, http.StatusNoContent, http.StatusNotFound) {
		return githubAPIError(status, body)
	}

	return nil
}

// UpsertLabel creates or updates a repository label definition.
func (c *GitHubClient) UpsertLabel(ctx context.Context, owner, repo, name, color, description string) error {
	createURL := c.apiURLf("/repos/%s/%s/labels", owner, repo)
	payload := map[string]string{
		"name":        name,
		"color":       color,
		"description": description,
	}

	body, status, err := c.doJSONRequest(ctx, http.MethodPost, createURL, payload)
	if err != nil {
		return err
	}

	if status == http.StatusCreated {
		return nil
	}

	if status != http.StatusUnprocessableEntity {
		return githubAPIError(status, body)
	}

	updateURL := c.apiURLf("/repos/%s/%s/labels/%s", owner, repo, url.PathEscape(name))
	body, status, err = c.doJSONRequest(ctx, http.MethodPatch, updateURL, payload)
	if err != nil {
		return err
	}

	if status != http.StatusOK {
		return githubAPIError(status, body)
	}

	return nil
}
