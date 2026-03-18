package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUseResponsesAPI(t *testing.T) {
	assert.True(t, useResponsesAPI("gpt-5.1-codex"))
	assert.True(t, useResponsesAPI("my-codex-model"))
	assert.False(t, useResponsesAPI("claude-sonnet-4-6"))
}

func TestParseResponsesSSE(t *testing.T) {
	sse := "" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello \"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":11,\"output_tokens\":22}}}\n\n" +
		"data: [DONE]\n\n"

	content, in, out, reason, err := parseResponsesSSE([]byte(sse))
	require.NoError(t, err)
	assert.Equal(t, "Hello world", content)
	assert.Equal(t, 11, in)
	assert.Equal(t, 22, out)
	assert.Equal(t, "completed", reason)
}

func TestLiteLLMClientCompleteUsesResponsesForCodexModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/responses", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprint(w,
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"+
				"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":5,\"output_tokens\":1}}}\n\n"+
				"data: [DONE]\n\n",
		)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "test-key", "gpt-5.1-codex")
	client.client = server.Client()

	resp, err := client.Complete(context.Background(), &CompletionRequest{
		Model:       "gpt-5.1-codex",
		System:      "be concise",
		Messages:    []Message{{Role: "user", Content: "say hi"}},
		MaxTokens:   128,
		Temperature: 0.3,
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Content)
	assert.Equal(t, 5, resp.TokensIn)
	assert.Equal(t, 1, resp.TokensOut)
}
