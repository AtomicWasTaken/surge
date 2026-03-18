package ai

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

func TestClaudeClientCompleteUsesTopLevelSystemPrompt(t *testing.T) {
	var requestBody map[string]interface{}
	var dangerousHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &requestBody))

		dangerousHeader = r.Header.Get("anthropic-dangerous-direct-browser-access")

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{
			"content": [{"type":"text","text":"{\"summary\":\"ok\",\"filesOverview\":[],\"findings\":[],\"vibeCheck\":{\"score\":8,\"verdict\":\"good\",\"flags\":[]},\"recommendations\":[],\"approve\":true}"}],
			"usage": {"input_tokens": 10, "output_tokens": 20},
			"stop_reason": "end_turn"
		}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClaudeClient("test-key", "claude-test")
	client.baseURL = server.URL
	client.client = server.Client()

	resp, err := client.Complete(context.Background(), &CompletionRequest{
		Model:       "ignored-by-client",
		System:      "system instructions",
		Messages:    []Message{{Role: "user", Content: "review this"}},
		MaxTokens:   1024,
		Temperature: 0.3,
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Content, `"summary":"ok"`)

	assert.Equal(t, "system instructions", requestBody["system"])

	messages, ok := requestBody["messages"].([]interface{})
	require.True(t, ok)
	require.Len(t, messages, 1)

	firstMessage, ok := messages[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "user", firstMessage["role"])
	assert.Equal(t, "review this", firstMessage["content"])
	assert.Empty(t, dangerousHeader)
}
