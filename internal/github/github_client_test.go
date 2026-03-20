package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errReadCloser) Close() error             { return nil }

func TestDismissReviewUsesDocumentedPayload(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &gotBody))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewGitHubClient("test-token")
	client.apiURL = server.URL

	err := client.DismissReview(context.Background(), "octo", "surge", 26, 99, "Superseded by a newer surge review run.")
	require.NoError(t, err)

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/repos/octo/surge/pulls/26/reviews/99/dismissals", gotPath)
	assert.Equal(t, map[string]string{
		"message": "Superseded by a newer surge review run.",
	}, gotBody)
}

func TestGetFileContentEscapesPathAndRef(t *testing.T) {
	var (
		gotEscapedPath string
		gotURI         string
		gotRawQuery    string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEscapedPath = r.URL.EscapedPath()
		gotURI = r.RequestURI
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":"aGVsbG8=","encoding":"base64"}`))
	}))
	defer server.Close()

	client := NewGitHubClient("test-token")
	client.apiURL = server.URL

	content, err := client.GetFileContent(context.Background(), "octo", "surge", "dir with spaces/a#b?.go", "feature/test branch")
	require.NoError(t, err)

	assert.Equal(t, "hello", content)
	assert.Equal(t, "/repos/octo/surge/contents/dir%20with%20spaces/a%23b%3F.go", gotEscapedPath)
	assert.Equal(t, "/repos/octo/surge/contents/dir%20with%20spaces/a%23b%3F.go?ref=feature%2Ftest+branch", gotURI)
	assert.Equal(t, "ref=feature%2Ftest+branch", gotRawQuery)
}

func TestExpectStatus(t *testing.T) {
	assert.True(t, expectStatus(http.StatusCreated, http.StatusOK, http.StatusCreated))
	assert.False(t, expectStatus(http.StatusBadRequest, http.StatusOK, http.StatusCreated))
}

func TestGitHubAPIError(t *testing.T) {
	err := githubAPIError(http.StatusBadGateway, []byte(`{"message":"upstream failed"}`))
	require.Error(t, err)
	assert.Equal(t, `GitHub API error (502): {"message":"upstream failed"}`, err.Error())
}

func TestDoRequestAndParseJSONResponseErrors(t *testing.T) {
	client := NewGitHubClient("test-token")

	_, _, err := client.doRequest(context.Background(), http.MethodGet, "://bad-url", githubJSONAcceptHeader, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to create request")

	var target map[string]string
	err = parseJSONResponse([]byte(`{`), &target, "broken response")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to parse broken response")

	_, _, err = client.doRequest(context.Background(), http.MethodPost, "http://example.com", githubJSONAcceptHeader, map[string]interface{}{"bad": math.NaN()})
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to marshal body")

	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	_, _, err = client.doRequest(context.Background(), http.MethodGet, "http://example.com", githubJSONAcceptHeader, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "request failed")

	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: errReadCloser{}, Header: make(http.Header)}, nil
	})}
	_, _, err = client.doRequest(context.Background(), http.MethodGet, "http://example.com", githubJSONAcceptHeader, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to read response")
}

func TestGetPRSuccessAndErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErr    string
		wantNumber int
	}{
		{
			name:   "success",
			status: http.StatusOK,
			body: `{
				"number": 7,
				"title": "Improve tests",
				"body": "details",
				"state": "open",
				"user": {"login": "octocat"},
				"base": {"ref": "main", "sha": "base"},
				"head": {"ref": "feature", "sha": "head"},
				"additions": 3,
				"deletions": 1,
				"changed_files": 2,
				"html_url": "https://github.com/octo/surge/pull/7",
				"created_at": "2026-03-19T10:00:00Z",
				"updated_at": "2026-03-19T11:00:00Z"
			}`,
			wantNumber: 7,
		},
		{name: "not found", status: http.StatusNotFound, body: `{}`, wantErr: "PR not found: octo/surge#7"},
		{name: "auth", status: http.StatusForbidden, body: `{}`, wantErr: "authentication failed"},
		{name: "auth unauthorized", status: http.StatusUnauthorized, body: `{}`, wantErr: "authentication failed"},
		{name: "other", status: http.StatusBadGateway, body: `boom`, wantErr: "GitHub API error (502): boom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := NewGitHubClient("test-token")
			client.apiURL = server.URL

			pr, err := client.GetPR(context.Background(), "octo", "surge", 7)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, pr)
			assert.Equal(t, tt.wantNumber, pr.Number)
			assert.Equal(t, "Improve tests", pr.Title)
			assert.Equal(t, "octocat", pr.Author)
			assert.Equal(t, "main", pr.BaseRef)
			assert.Equal(t, "feature", pr.HeadRef)
			assert.WithinDuration(t, time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC), pr.CreatedAt, 0)
		})
	}

	t.Run("parse error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{`))
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		_, err := client.GetPR(context.Background(), "octo", "surge", 7)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to parse PR response")
	})
}

func TestGetDiffAndFiles(t *testing.T) {
	t.Run("diff success and error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") == githubDiffAcceptHeader {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("diff --git a.go b.go"))
				return
			}
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("boom"))
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		diff, err := client.GetDiff(context.Background(), "octo", "surge", 1)
		require.NoError(t, err)
		assert.Equal(t, "diff --git a.go b.go", diff)
	})

	t.Run("diff error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("boom"))
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		_, err := client.GetDiff(context.Background(), "octo", "surge", 1)
		require.Error(t, err)
		assert.ErrorContains(t, err, "GitHub API error (502): boom")
	})

	t.Run("files success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{"filename":"a.go","status":"modified","additions":2,"deletions":1,"patch":"@@"},
				{"filename":"b.go","status":"deleted","additions":0,"deletions":5,"patch":""}
			]`))
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		files, err := client.GetFiles(context.Background(), "octo", "surge", 1)
		require.NoError(t, err)
		require.Len(t, files, 2)
		assert.Equal(t, model.FileStatusModified, files[0].Status)
		assert.Equal(t, model.FileStatusDeleted, files[1].Status)
	})

	t.Run("files error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("boom"))
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		_, err := client.GetFiles(context.Background(), "octo", "surge", 1)
		require.Error(t, err)
		assert.ErrorContains(t, err, "GitHub API error (502): boom")
	})

	t.Run("files parse error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{`))
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		_, err := client.GetFiles(context.Background(), "octo", "surge", 1)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to parse files response")
	})
}

func TestGetFileContentErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "not found", status: http.StatusNotFound, body: `{}`, wantErr: "file not found: path.go at ref"},
		{name: "api error", status: http.StatusBadGateway, body: `boom`, wantErr: "GitHub API error (502): boom"},
		{name: "decode error", status: http.StatusOK, body: `{"content":"not-base64","encoding":"base64"}`, wantErr: "failed to decode file content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := NewGitHubClient("test-token")
			client.apiURL = server.URL

			_, err := client.GetFileContent(context.Background(), "octo", "surge", "path.go", "ref")
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}

	t.Run("parse error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{`))
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		_, err := client.GetFileContent(context.Background(), "octo", "surge", "path.go", "ref")
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to parse file response")
	})
}

func TestReviewAndCommentOperations(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		method   string
		status   int
		body     string
		run      func(*GitHubClient) error
		wantErr  string
		wantBody string
	}{
		{
			name:   "post review success",
			path:   "/repos/octo/surge/pulls/1/reviews",
			method: http.MethodPost,
			status: http.StatusCreated,
			body:   `{}`,
			run: func(c *GitHubClient) error {
				return c.PostReview(context.Background(), "octo", "surge", 1, &model.ReviewInput{
					Event: "COMMENT",
					Body:  "summary",
					Comments: []model.ReviewComment{
						{Path: "a.go", Position: 2, Body: "inline"},
					},
				})
			},
			wantBody: `"comments":[{"body":"inline","path":"a.go","position":2}]`,
		},
		{
			name:   "post review success ok",
			path:   "/repos/octo/surge/pulls/1/reviews",
			method: http.MethodPost,
			status: http.StatusOK,
			body:   `{}`,
			run: func(c *GitHubClient) error {
				return c.PostReview(context.Background(), "octo", "surge", 1, &model.ReviewInput{Event: "COMMENT", Body: "summary"})
			},
			wantBody: `"event":"COMMENT"`,
		},
		{
			name:   "post review stale",
			path:   "/repos/octo/surge/pulls/1/reviews",
			method: http.MethodPost,
			status: http.StatusUnprocessableEntity,
			body:   `{}`,
			run: func(c *GitHubClient) error {
				return c.PostReview(context.Background(), "octo", "surge", 1, &model.ReviewInput{Event: "COMMENT", Body: "summary"})
			},
			wantErr: "review position is stale",
		},
		{
			name:   "post comment success",
			path:   "/repos/octo/surge/issues/1/comments",
			method: http.MethodPost,
			status: http.StatusCreated,
			body:   `{}`,
			run: func(c *GitHubClient) error {
				return c.PostComment(context.Background(), "octo", "surge", 1, "hello")
			},
			wantBody: `"body":"hello"`,
		},
		{
			name:   "post comment error",
			path:   "/repos/octo/surge/issues/1/comments",
			method: http.MethodPost,
			status: http.StatusBadGateway,
			body:   `boom`,
			run: func(c *GitHubClient) error {
				return c.PostComment(context.Background(), "octo", "surge", 1, "hello")
			},
			wantErr: "GitHub API error (502): boom",
		},
		{
			name:   "post review generic error",
			path:   "/repos/octo/surge/pulls/1/reviews",
			method: http.MethodPost,
			status: http.StatusUnauthorized,
			body:   `denied`,
			run: func(c *GitHubClient) error {
				return c.PostReview(context.Background(), "octo", "surge", 1, &model.ReviewInput{Event: "COMMENT", Body: "summary"})
			},
			wantErr: "GitHub API error (401): denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.method, r.Method)
				assert.Equal(t, tt.path, r.URL.Path)
				data, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				gotBody = string(data)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := NewGitHubClient("test-token")
			client.apiURL = server.URL

			err := tt.run(client)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, gotBody, tt.wantBody)
		})
	}
}

func TestListAndDeleteOperations(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		method  string
		status  int
		body    string
		run     func(*GitHubClient) error
		wantErr string
	}{
		{
			name:   "list comments",
			path:   "/repos/octo/surge/issues/1/comments",
			method: http.MethodGet,
			status: http.StatusOK,
			body:   `[{"id":1,"body":"hi","user":{"login":"bot","type":"Bot"},"created_at":"now"}]`,
			run: func(c *GitHubClient) error {
				comments, err := c.ListComments(context.Background(), "octo", "surge", 1)
				if err == nil {
					require.Len(t, comments, 1)
					assert.True(t, comments[0].IsBot)
				}
				return err
			},
		},
		{
			name:   "list comments parse error",
			path:   "/repos/octo/surge/issues/1/comments",
			method: http.MethodGet,
			status: http.StatusOK,
			body:   `{`,
			run: func(c *GitHubClient) error {
				_, err := c.ListComments(context.Background(), "octo", "surge", 1)
				return err
			},
			wantErr: "failed to parse comments",
		},
		{
			name:   "list comments status error",
			path:   "/repos/octo/surge/issues/1/comments",
			method: http.MethodGet,
			status: http.StatusBadGateway,
			body:   `boom`,
			run: func(c *GitHubClient) error {
				_, err := c.ListComments(context.Background(), "octo", "surge", 1)
				return err
			},
			wantErr: "GitHub API error (502): boom",
		},
		{
			name:   "delete comment",
			path:   "/repos/octo/surge/issues/comments/9",
			method: http.MethodDelete,
			status: http.StatusNoContent,
			body:   ``,
			run: func(c *GitHubClient) error {
				return c.DeleteComment(context.Background(), "octo", "surge", 9)
			},
		},
		{
			name:   "delete comment error",
			path:   "/repos/octo/surge/issues/comments/9",
			method: http.MethodDelete,
			status: http.StatusBadGateway,
			body:   ``,
			run: func(c *GitHubClient) error {
				return c.DeleteComment(context.Background(), "octo", "surge", 9)
			},
			wantErr: "GitHub API error (502)",
		},
		{
			name:   "list reviews parse error",
			path:   "/repos/octo/surge/pulls/1/reviews",
			method: http.MethodGet,
			status: http.StatusOK,
			body:   `{`,
			run: func(c *GitHubClient) error {
				_, err := c.ListReviews(context.Background(), "octo", "surge", 1)
				return err
			},
			wantErr: "failed to parse reviews",
		},
		{
			name:   "list reviews status error",
			path:   "/repos/octo/surge/pulls/1/reviews",
			method: http.MethodGet,
			status: http.StatusBadGateway,
			body:   `boom`,
			run: func(c *GitHubClient) error {
				_, err := c.ListReviews(context.Background(), "octo", "surge", 1)
				return err
			},
			wantErr: "GitHub API error (502): boom",
		},
		{
			name:   "list reviews",
			path:   "/repos/octo/surge/pulls/1/reviews",
			method: http.MethodGet,
			status: http.StatusOK,
			body:   `[{"id":2,"body":"summary","state":"COMMENTED","user":{"login":"bot","type":"Bot"},"created_at":"now"}]`,
			run: func(c *GitHubClient) error {
				reviews, err := c.ListReviews(context.Background(), "octo", "surge", 1)
				if err == nil {
					require.Len(t, reviews, 1)
					assert.True(t, reviews[0].IsBot)
				}
				return err
			},
		},
		{
			name:   "delete review",
			path:   "/repos/octo/surge/pulls/1/reviews/2",
			method: http.MethodDelete,
			status: http.StatusNoContent,
			body:   `{}`,
			run: func(c *GitHubClient) error {
				return c.DeleteReview(context.Background(), "octo", "surge", 1, 2)
			},
		},
		{
			name:   "delete review ok",
			path:   "/repos/octo/surge/pulls/1/reviews/2",
			method: http.MethodDelete,
			status: http.StatusOK,
			body:   `{}`,
			run: func(c *GitHubClient) error {
				return c.DeleteReview(context.Background(), "octo", "surge", 1, 2)
			},
		},
		{
			name:   "delete review error",
			path:   "/repos/octo/surge/pulls/1/reviews/2",
			method: http.MethodDelete,
			status: http.StatusBadGateway,
			body:   `boom`,
			run: func(c *GitHubClient) error {
				return c.DeleteReview(context.Background(), "octo", "surge", 1, 2)
			},
			wantErr: "GitHub API error (502): boom",
		},
		{
			name:   "delete review comment",
			path:   "/repos/octo/surge/pulls/comments/3",
			method: http.MethodDelete,
			status: http.StatusNoContent,
			body:   `{}`,
			run: func(c *GitHubClient) error {
				return c.DeleteReviewComment(context.Background(), "octo", "surge", 3)
			},
		},
		{
			name:   "delete review comment error",
			path:   "/repos/octo/surge/pulls/comments/3",
			method: http.MethodDelete,
			status: http.StatusBadGateway,
			body:   `boom`,
			run: func(c *GitHubClient) error {
				return c.DeleteReviewComment(context.Background(), "octo", "surge", 3)
			},
			wantErr: "GitHub API error (502): boom",
		},
		{
			name:   "list review comments",
			path:   "/repos/octo/surge/pulls/1/reviews/2/comments",
			method: http.MethodGet,
			status: http.StatusOK,
			body:   `[{"id":3,"body":"inline"}]`,
			run: func(c *GitHubClient) error {
				comments, err := c.ListReviewComments(context.Background(), "octo", "surge", 1, 2)
				if err == nil {
					require.Len(t, comments, 1)
					assert.Equal(t, int64(3), comments[0].ID)
				}
				return err
			},
		},
		{
			name:   "list review comments parse error",
			path:   "/repos/octo/surge/pulls/1/reviews/2/comments",
			method: http.MethodGet,
			status: http.StatusOK,
			body:   `{`,
			run: func(c *GitHubClient) error {
				_, err := c.ListReviewComments(context.Background(), "octo", "surge", 1, 2)
				return err
			},
			wantErr: "failed to parse review comments",
		},
		{
			name:   "list review comments status error",
			path:   "/repos/octo/surge/pulls/1/reviews/2/comments",
			method: http.MethodGet,
			status: http.StatusBadGateway,
			body:   `boom`,
			run: func(c *GitHubClient) error {
				_, err := c.ListReviewComments(context.Background(), "octo", "surge", 1, 2)
				return err
			},
			wantErr: "GitHub API error (502): boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.method, r.Method)
				assert.Equal(t, tt.path, r.URL.Path)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := NewGitHubClient("test-token")
			client.apiURL = server.URL

			err := tt.run(client)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestLabelOperations(t *testing.T) {
	t.Run("list add remove labels", func(t *testing.T) {
		var callCount int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			switch callCount {
			case 1:
				assert.Equal(t, "/repos/octo/surge/issues/1/labels", r.URL.Path)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[{"name":"surge:logic"},{"name":""}]`))
			case 2:
				assert.Equal(t, http.MethodPost, r.Method)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`[]`))
			case 3:
				assert.Equal(t, "/repos/octo/surge/issues/1/labels/surge:logic", r.URL.EscapedPath())
				w.WriteHeader(http.StatusNotFound)
			default:
				t.Fatalf("unexpected request %d", callCount)
			}
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		labels, err := client.ListLabels(context.Background(), "octo", "surge", 1)
		require.NoError(t, err)
		assert.Equal(t, []string{"surge:logic"}, labels)

		require.NoError(t, client.AddLabels(context.Background(), "octo", "surge", 1, []string{"surge:logic"}))
		require.NoError(t, client.RemoveLabel(context.Background(), "octo", "surge", 1, "surge:logic"))
	})

	t.Run("label errors and alt statuses", func(t *testing.T) {
		var callCount int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			switch callCount {
			case 1:
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{`))
			case 2:
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			case 3:
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			case 4:
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`boom`))
			default:
				t.Fatalf("unexpected request %d", callCount)
			}
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		_, err := client.ListLabels(context.Background(), "octo", "surge", 1)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to parse labels")

		require.NoError(t, client.AddLabels(context.Background(), "octo", "surge", 1, []string{"a"}))
		require.NoError(t, client.RemoveLabel(context.Background(), "octo", "surge", 1, "a"))

		err = client.AddLabels(context.Background(), "octo", "surge", 1, []string{"a"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "GitHub API error (502): boom")
	})

	t.Run("add labels empty short-circuits", func(t *testing.T) {
		client := NewGitHubClient("test-token")
		require.NoError(t, client.AddLabels(context.Background(), "octo", "surge", 1, nil))
	})

	t.Run("remove label alternate statuses and error", func(t *testing.T) {
		t.Run("ok status", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			client := NewGitHubClient("test-token")
			client.apiURL = server.URL
			require.NoError(t, client.RemoveLabel(context.Background(), "octo", "surge", 1, "a"))
		})

		t.Run("error status", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`boom`))
			}))
			defer server.Close()
			client := NewGitHubClient("test-token")
			client.apiURL = server.URL
			err := client.RemoveLabel(context.Background(), "octo", "surge", 1, "a")
			require.Error(t, err)
			assert.ErrorContains(t, err, "GitHub API error (502): boom")
		})
	})

	t.Run("upsert label create then patch", func(t *testing.T) {
		var methods []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			methods = append(methods, r.Method)
			if len(methods) == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`exists`))
				return
			}
			assert.Equal(t, "/repos/octo/surge/labels/surge:logic", r.URL.EscapedPath())
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL

		err := client.UpsertLabel(context.Background(), "octo", "surge", "surge:logic", "ffffff", "desc")
		require.NoError(t, err)
		assert.Equal(t, []string{http.MethodPost, http.MethodPatch}, methods)
	})

	t.Run("upsert label create success and failures", func(t *testing.T) {
		t.Run("create success", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			client := NewGitHubClient("test-token")
			client.apiURL = server.URL
			require.NoError(t, client.UpsertLabel(context.Background(), "octo", "surge", "surge:logic", "ffffff", "desc"))
		})

		t.Run("create hard error", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`boom`))
			}))
			defer server.Close()
			client := NewGitHubClient("test-token")
			client.apiURL = server.URL
			err := client.UpsertLabel(context.Background(), "octo", "surge", "surge:logic", "ffffff", "desc")
			require.Error(t, err)
			assert.ErrorContains(t, err, "GitHub API error (502): boom")
		})

		t.Run("patch error", func(t *testing.T) {
			var callCount int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				if callCount == 1 {
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`exists`))
					return
				}
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`boom`))
			}))
			defer server.Close()
			client := NewGitHubClient("test-token")
			client.apiURL = server.URL
			err := client.UpsertLabel(context.Background(), "octo", "surge", "surge:logic", "ffffff", "desc")
			require.Error(t, err)
			assert.ErrorContains(t, err, "GitHub API error (502): boom")
		})
	})
}

func TestDismissReviewError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer server.Close()

	client := NewGitHubClient("test-token")
	client.apiURL = server.URL

	err := client.DismissReview(context.Background(), "octo", "surge", 1, 2, "msg")
	require.Error(t, err)
	assert.ErrorContains(t, err, "GitHub API error (502): boom")
}

func TestGitHubClientWrapperRequestErrors(t *testing.T) {
	client := NewGitHubClient("test-token")
	client.apiURL = "://bad"

	tests := []struct {
		name string
		run  func() error
	}{
		{"get pr", func() error { _, err := client.GetPR(context.Background(), "o", "r", 1); return err }},
		{"get diff", func() error { _, err := client.GetDiff(context.Background(), "o", "r", 1); return err }},
		{"get files", func() error { _, err := client.GetFiles(context.Background(), "o", "r", 1); return err }},
		{"get file content", func() error { _, err := client.GetFileContent(context.Background(), "o", "r", "p", "ref"); return err }},
		{"post review", func() error { return client.PostReview(context.Background(), "o", "r", 1, &model.ReviewInput{}) }},
		{"post comment", func() error { return client.PostComment(context.Background(), "o", "r", 1, "body") }},
		{"list comments", func() error { _, err := client.ListComments(context.Background(), "o", "r", 1); return err }},
		{"delete comment", func() error { return client.DeleteComment(context.Background(), "o", "r", 1) }},
		{"list reviews", func() error { _, err := client.ListReviews(context.Background(), "o", "r", 1); return err }},
		{"delete review", func() error { return client.DeleteReview(context.Background(), "o", "r", 1, 2) }},
		{"dismiss review", func() error { return client.DismissReview(context.Background(), "o", "r", 1, 2, "msg") }},
		{"list review comments", func() error { _, err := client.ListReviewComments(context.Background(), "o", "r", 1, 2); return err }},
		{"delete review comment", func() error { return client.DeleteReviewComment(context.Background(), "o", "r", 1) }},
		{"list labels", func() error { _, err := client.ListLabels(context.Background(), "o", "r", 1); return err }},
		{"add labels", func() error { return client.AddLabels(context.Background(), "o", "r", 1, []string{"a"}) }},
		{"remove label", func() error { return client.RemoveLabel(context.Background(), "o", "r", 1, "a") }},
		{"upsert label", func() error { return client.UpsertLabel(context.Background(), "o", "r", "a", "fff", "d") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			require.Error(t, err)
			assert.ErrorContains(t, err, "failed to create request")
		})
	}
}

func TestListLabelsStatusErrorAndUpsertPatchRequestError(t *testing.T) {
	t.Run("list labels non-200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`boom`))
		}))
		defer server.Close()

		client := NewGitHubClient("test-token")
		client.apiURL = server.URL
		_, err := client.ListLabels(context.Background(), "octo", "surge", 1)
		require.Error(t, err)
		assert.ErrorContains(t, err, "GitHub API error (502): boom")
	})

	t.Run("upsert label patch request error", func(t *testing.T) {
		client := NewGitHubClient("test-token")
		client.apiURL = "http://example.com"
		call := 0
		client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			call++
			if call == 1 {
				return &http.Response{
					StatusCode: http.StatusUnprocessableEntity,
					Status:     "422 Unprocessable Entity",
					Body:       io.NopCloser(strings.NewReader("exists")),
					Header:     make(http.Header),
				}, nil
			}
			return nil, errors.New("network down")
		})}

		err := client.UpsertLabel(context.Background(), "octo", "surge", "surge:logic", "ffffff", "desc")
		require.Error(t, err)
		assert.ErrorContains(t, err, "network down")
	})
}
