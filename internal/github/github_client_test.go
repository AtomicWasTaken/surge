package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		gotPath     string
		gotURI      string
		gotRawQuery string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
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
	assert.Equal(t, "/repos/octo/surge/contents/dir with spaces/a#b?.go", gotPath)
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
