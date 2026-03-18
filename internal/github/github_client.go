package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AtomicWasTaken/surge/internal/model"
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

func (c *GitHubClient) doRequest(ctx context.Context, method, url string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Authorization", "token "+c.authToken)
	req.Header.Set("User-Agent", "surge-ai-review/1.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// GetPR fetches the metadata for a pull request.
func (c *GitHubClient) GetPR(ctx context.Context, owner, repo string, prNumber int) (*model.PR, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.apiURL, owner, repo, prNumber)

	body, status, err := c.doRequest(ctx, http.MethodGet, url, nil)
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

	if err := json.Unmarshal(body, &prResp); err != nil {
		return nil, fmt.Errorf("failed to parse PR response: %w", err)
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
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.apiURL, owner, repo, prNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3.diff")
	req.Header.Set("Authorization", "token "+c.authToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch diff: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read diff: %w", err)
	}

	return string(data), nil
}

// GetFiles fetches the list of changed files with their patches.
func (c *GitHubClient) GetFiles(ctx context.Context, owner, repo string, prNumber int) ([]model.FileChange, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files", c.apiURL, owner, repo, prNumber)

	body, status, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error (%d): %s", status, string(body))
	}

	var files []struct {
		SHA       string `json:"sha"`
		Filename  string `json:"filename"`
		Status    string `json:"status"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Patch     string `json:"patch"`
	}

	if err := json.Unmarshal(body, &files); err != nil {
		return nil, fmt.Errorf("failed to parse files response: %w", err)
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
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", c.apiURL, owner, repo, path, ref)

	body, status, err := c.doRequest(ctx, http.MethodGet, url, nil)
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

	if err := json.Unmarshal(body, &fileResp); err != nil {
		return "", fmt.Errorf("failed to parse file response: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(fileResp.Content)
	if err != nil {
		return "", fmt.Errorf("failed to decode file content: %w", err)
	}

	return string(decoded), nil
}

// PostReview posts a review with inline comments and a summary.
func (c *GitHubClient) PostReview(ctx context.Context, owner, repo string, prNumber int, review *model.ReviewInput) error {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", c.apiURL, owner, repo, prNumber)

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

	body, status, err := c.doRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return err
	}

	if status == http.StatusUnprocessableEntity {
		return fmt.Errorf("review position is stale (lines may have moved since the diff was generated)")
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("GitHub API error (%d): %s", status, string(body))
	}

	return nil
}

// PostComment posts a general comment on the PR.
func (c *GitHubClient) PostComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.apiURL, owner, repo, prNumber)

	payload := map[string]string{"body": body}

	respBody, status, err := c.doRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return err
	}

	if status != http.StatusCreated {
		return fmt.Errorf("GitHub API error (%d): %s", status, string(respBody))
	}

	return nil
}

// ListComments lists comments on a PR.
func (c *GitHubClient) ListComments(ctx context.Context, owner, repo string, prNumber int) ([]*model.PRComment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.apiURL, owner, repo, prNumber)

	body, status, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error (%d): %s", status, string(body))
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

	if err := json.Unmarshal(body, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse comments: %w", err)
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
	url := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d", c.apiURL, owner, repo, commentID)

	_, status, err := c.doRequest(ctx, http.MethodDelete, url, nil)
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
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", c.apiURL, owner, repo, prNumber)

	body, status, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error (%d): %s", status, string(body))
	}

	var reviews []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
		User struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
		CreatedAt string `json:"created_at"`
	}

	if err := json.Unmarshal(body, &reviews); err != nil {
		return nil, fmt.Errorf("failed to parse reviews: %w", err)
	}

	result := make([]*model.PRReview, len(reviews))
	for i, r := range reviews {
		result[i] = &model.PRReview{
			ID:        r.ID,
			Body:      r.Body,
			Author:    r.User.Login,
			IsBot:     r.User.Type == "Bot",
			CreatedAt: r.CreatedAt,
		}
	}

	return result, nil
}

// DeleteReview deletes a review by ID.
func (c *GitHubClient) DeleteReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64) error {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d", c.apiURL, owner, repo, prNumber, reviewID)

	body, status, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("GitHub API error (%d): %s", status, string(body))
	}

	return nil
}

// ListReviewComments lists inline comments for a specific PR review.
func (c *GitHubClient) ListReviewComments(ctx context.Context, owner, repo string, prNumber int, reviewID int64) ([]*model.PRReviewComment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d/comments", c.apiURL, owner, repo, prNumber, reviewID)

	body, status, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error (%d): %s", status, string(body))
	}

	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}

	if err := json.Unmarshal(body, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse review comments: %w", err)
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
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/comments/%d", c.apiURL, owner, repo, commentID)

	body, status, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	if status != http.StatusNoContent {
		return fmt.Errorf("GitHub API error (%d): %s", status, string(body))
	}

	return nil
}
