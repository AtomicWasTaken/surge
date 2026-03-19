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
