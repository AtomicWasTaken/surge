package ai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

func TestClaudeClientComplete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var gotAPIKey string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAPIKey = r.Header.Get("x-api-key")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"content":[{"type":"text","text":"Hello "},{"type":"tool","text":"ignored"},{"type":"text","text":"world"}],
				"usage":{"input_tokens":7,"output_tokens":3},
				"stop_reason":"end_turn"
			}`))
		}))
		defer server.Close()

		client := NewClaudeClient("test-key", "claude-test")
		client.baseURL = server.URL
		client.client = server.Client()

		resp, err := client.Complete(context.Background(), &CompletionRequest{
			System:      "system",
			Messages:    []Message{{Role: "user", Content: "hi"}},
			MaxTokens:   55,
			Temperature: 0.2,
		})
		require.NoError(t, err)
		assert.Equal(t, "Hello world", resp.Content)
		assert.Equal(t, 7, resp.TokensIn)
		assert.Equal(t, 3, resp.TokensOut)
		assert.Equal(t, "end_turn", resp.FinishReason)
		assert.Equal(t, "test-key", gotAPIKey)
		assert.Equal(t, "claude", client.Name())
	})

	t.Run("api error json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"type":"bad_request","message":"nope"}}`))
		}))
		defer server.Close()

		client := NewClaudeClient("test-key", "claude-test")
		client.baseURL = server.URL
		client.client = server.Client()

		_, err := client.Complete(context.Background(), &CompletionRequest{MaxTokens: 1, Debug: true})
		require.Error(t, err)
		assert.ErrorContains(t, err, "claude API error (400 Bad Request): nope")
	})

	t.Run("api error raw body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`boom`))
		}))
		defer server.Close()

		client := NewClaudeClient("test-key", "claude-test")
		client.baseURL = server.URL
		client.client = server.Client()

		_, err := client.Complete(context.Background(), &CompletionRequest{MaxTokens: 1})
		require.Error(t, err)
		assert.ErrorContains(t, err, "claude API error (502 Bad Gateway): boom")
	})

	t.Run("parse error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{`))
		}))
		defer server.Close()

		client := NewClaudeClient("test-key", "claude-test")
		client.baseURL = server.URL
		client.client = server.Client()

		_, err := client.Complete(context.Background(), &CompletionRequest{MaxTokens: 1})
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to parse claude response")
	})
}

func TestClaudeClientTransportErrors(t *testing.T) {
	client := NewClaudeClient("test-key", "claude-test")
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}

	_, err := client.Complete(context.Background(), &CompletionRequest{MaxTokens: 1})
	require.Error(t, err)
	assert.ErrorContains(t, err, "claude request failed: Post")

	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       errReadCloser{},
			Header:     make(http.Header),
		}, nil
	})}

	_, err = client.Complete(context.Background(), &CompletionRequest{MaxTokens: 1})
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to read response")

	client.baseURL = "://bad"
	client.client = &http.Client{}
	_, err = client.Complete(context.Background(), &CompletionRequest{MaxTokens: 1})
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to create request")
}

func TestClaudeDebugOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1},"stop_reason":"done"}`))
	}))
	defer server.Close()

	client := NewClaudeClient("test-key", "claude-test")
	client.baseURL = server.URL
	client.client = server.Client()

	stderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	_, err = client.Complete(context.Background(), &CompletionRequest{MaxTokens: 1, Debug: true})
	require.NoError(t, err)

	require.NoError(t, w.Close())
	os.Stderr = stderr
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Contains(t, buf.String(), "[debug] claude request")
	assert.Contains(t, buf.String(), "[debug] claude response status=200 OK")
}

func TestClaudeMarshalError(t *testing.T) {
	prev := claudeMarshal
	claudeMarshal = func(v interface{}) ([]byte, error) { return nil, errors.New("marshal failed") }
	t.Cleanup(func() { claudeMarshal = prev })

	client := NewClaudeClient("test-key", "claude-test")
	_, err := client.Complete(context.Background(), &CompletionRequest{MaxTokens: 1})
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to marshal request")
}
