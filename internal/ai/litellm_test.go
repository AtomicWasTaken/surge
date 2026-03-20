package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &payload))
		input, ok := payload["input"].([]interface{})
		require.True(t, ok)
		require.Len(t, input, 1)

		w.WriteHeader(http.StatusOK)
		_, err = fmt.Fprint(w,
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

func TestLiteLLMClientCompleteFallsBackToChatCompletions(t *testing.T) {
	hits := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++

		switch r.URL.Path {
		case "/v1/responses":
			w.WriteHeader(http.StatusBadRequest)
			_, err := fmt.Fprint(w, `{"detail":"Unsupported parameter: max_output_tokens"}`)
			require.NoError(t, err)
		case "/responses":
			w.WriteHeader(http.StatusNotFound)
			_, err := fmt.Fprint(w, `{"detail":"Not Found"}`)
			require.NoError(t, err)
		case "/v1/openai/v1/responses":
			w.WriteHeader(http.StatusNotFound)
			_, err := fmt.Fprint(w, `{"detail":"Not Found"}`)
			require.NoError(t, err)
		case "/v1/chat/completions":
			w.WriteHeader(http.StatusOK)
			_, err := fmt.Fprint(w, `{
				"choices":[{"message":{"content":"fallback-ok"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":9,"completion_tokens":3},
				"model":"gpt-5.1-codex"
			}`)
			require.NoError(t, err)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "test-key", "gpt-5.1-codex")
	client.client = server.Client()

	resp, err := client.Complete(context.Background(), &CompletionRequest{
		Model:     "gpt-5.1-codex",
		Messages:  []Message{{Role: "user", Content: "say hi"}},
		MaxTokens: 64,
	})
	require.NoError(t, err)
	assert.Equal(t, "fallback-ok", resp.Content)
	assert.GreaterOrEqual(t, hits["/v1/responses"], 1)
	assert.Equal(t, 1, hits["/v1/chat/completions"])
}

func TestLiteLLMHelpersAndErrors(t *testing.T) {
	resp, err := parseChatCompletionsResponse([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2},"model":"m"}`))
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Content)

	_, err = parseChatCompletionsResponse([]byte(`{`))
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to parse litellm response")

	_, err = parseChatCompletionsResponse([]byte(`{"choices":[]}`))
	require.Error(t, err)
	assert.ErrorContains(t, err, "no choices returned from litellm")

	assert.Equal(t, []string{"a", "b"}, uniqueStrings([]string{"a", "a", "b"}))
	assert.Contains(t, candidateResponsesURLs("http://x/v1"), "http://x/v1/responses")
	assert.Contains(t, candidateChatCompletionURLs("http://x/openai/v1"), "http://x/v1/chat/completions")
	assert.Equal(t, []string{"http://x/v1/openai/v1", "http://x/v1/openai", "http://x/v1", "http://x"}, normalizedBaseCandidates("http://x/v1/openai/v1"))
	assert.True(t, isUnsupportedParameter([]byte("Unsupported parameter: foo")))
	assert.False(t, isUnsupportedParameter([]byte("other")))
}

func TestParseResponsesSSEVariants(t *testing.T) {
	sse := "" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"done\",\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"Hello\"},{\"type\":\"text\",\"text\":\" world\"}]}],\"usage\":{\"input_tokens\":2,\"output_tokens\":3}}}\n\n"
	content, in, out, reason, err := parseResponsesSSE([]byte(sse))
	require.NoError(t, err)
	assert.Equal(t, "Hello world", content)
	assert.Equal(t, 2, in)
	assert.Equal(t, 3, out)
	assert.Equal(t, "done", reason)

	_, _, _, _, err = parseResponsesSSE([]byte("data: {\"error\":{\"message\":\"bad\"}}\n\n"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "bad")

	_, _, _, _, err = parseResponsesSSE([]byte("data: {not-json}\n\n"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "empty responses output")
}

func TestLiteLLMDoJSONPostAndCompleteBranches(t *testing.T) {
	client := NewLiteLLMClient("http://example.com", "key", "m")
	_, _, err := client.doJSONPost(context.Background(), "http://example.com", map[string]interface{}{"bad": math.NaN()})
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to marshal request")

	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       errReadCloser{},
			Header:     make(http.Header),
		}, nil
	})}
	_, _, err = client.doJSONPost(context.Background(), "http://example.com", map[string]interface{}{"ok": true})
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to read response")

	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	_, err = client.Complete(context.Background(), &CompletionRequest{Model: "plain-model", MaxTokens: 1})
	require.Error(t, err)
	assert.ErrorContains(t, err, "litellm request failed")
}

func TestLiteLLMCompleteAdditionalBranches(t *testing.T) {
	t.Run("chat direct success with debug", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2},"model":"plain-model"}`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "plain-model")
		client.client = server.Client()

		stderr := os.Stderr
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stderr = w

		resp, err := client.Complete(context.Background(), &CompletionRequest{
			Model:       "plain-model",
			Debug:       true,
			Messages:    []Message{{Role: "user", Content: "hi"}},
			MaxTokens:   4,
			Temperature: 0.1,
		})
		require.NoError(t, err)
		require.NoError(t, w.Close())
		os.Stderr = stderr
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		assert.Equal(t, "ok", resp.Content)
		assert.Contains(t, buf.String(), "[debug] litellm request")
		assert.Contains(t, buf.String(), "[debug] litellm response status=200")
	})

	t.Run("responses api hard error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`denied`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "gpt-5-codex")
		client.client = server.Client()

		_, err := client.Complete(context.Background(), &CompletionRequest{Model: "gpt-5-codex", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "litellm API error (401): denied")
	})

	t.Run("chat api hard error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`denied`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "plain-model")
		client.client = server.Client()

		_, err := client.Complete(context.Background(), &CompletionRequest{Model: "plain-model", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "litellm API error (401): denied")
	})
}

func TestLiteLLMNameAndMoreSSEBranches(t *testing.T) {
	client := NewLiteLLMClient("http://example.com", "k", "m")
	assert.Equal(t, "litellm", client.Name())

	longLine := "data: " + string(bytes.Repeat([]byte("a"), 70000)) + "\n"
	_, _, _, _, err := parseResponsesSSE([]byte(longLine))
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed reading SSE stream")
}

func TestLiteLLMMethodLevelBranchCoverage(t *testing.T) {
	t.Run("chat completions parse error after 200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "plain-model")
		client.client = server.Client()
		_, err := client.completeChatCompletions(context.Background(), &CompletionRequest{Model: "plain-model", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to parse litellm response")
	})

	t.Run("chat completions not found exhausts variants", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`missing`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "plain-model")
		client.client = server.Client()
		_, err := client.completeChatCompletions(context.Background(), &CompletionRequest{Model: "plain-model", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "litellm API error (404): missing")
	})

	t.Run("chat completions server error exhausts variants", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "plain-model")
		client.client = server.Client()
		_, err := client.completeChatCompletions(context.Background(), &CompletionRequest{Model: "plain-model", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "litellm API error (500): boom")
	})

	t.Run("chat completions malformed url", func(t *testing.T) {
		client := NewLiteLLMClient("://bad", "test-key", "plain-model")
		_, err := client.completeChatCompletions(context.Background(), &CompletionRequest{Model: "plain-model", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "litellm request failed")
	})

	t.Run("chat completions unsupported parameter retries next token field", func(t *testing.T) {
		var tokenFieldNames []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(body, &payload))
			if _, ok := payload["max_tokens"]; ok {
				tokenFieldNames = append(tokenFieldNames, "max_tokens")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`unsupported parameter`))
				return
			}
			if _, ok := payload["max_completion_tokens"]; ok {
				tokenFieldNames = append(tokenFieldNames, "max_completion_tokens")
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2},"model":"plain-model"}`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "plain-model")
		client.client = server.Client()
		resp, err := client.completeChatCompletions(context.Background(), &CompletionRequest{Model: "plain-model", MaxTokens: 4})
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Content)
		assert.Equal(t, []string{"max_tokens", "max_completion_tokens"}, tokenFieldNames)
	})

	t.Run("chat completions not found breaks to next url variant", func(t *testing.T) {
		var paths []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			if len(paths) == 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`missing`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2},"model":"plain-model"}`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL+"/v1", "test-key", "plain-model")
		client.client = server.Client()
		resp, err := client.completeChatCompletions(context.Background(), &CompletionRequest{Model: "plain-model", MaxTokens: 4})
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Content)
		assert.GreaterOrEqual(t, len(paths), 2)
	})

	t.Run("responses parse error after 200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`data: {` + "\n\n"))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "gpt-5-codex")
		client.client = server.Client()
		_, err := client.completeResponses(context.Background(), &CompletionRequest{Model: "gpt-5-codex", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to parse litellm responses stream")
	})

	t.Run("responses not found exhausts variants", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`missing`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "gpt-5-codex")
		client.client = server.Client()
		_, err := client.completeResponses(context.Background(), &CompletionRequest{Model: "gpt-5-codex", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "litellm responses API error (404): missing")
	})

	t.Run("responses server error exhausts variants", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "gpt-5-codex")
		client.client = server.Client()
		_, err := client.completeResponses(context.Background(), &CompletionRequest{Model: "gpt-5-codex", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "litellm responses API error (500): boom")
	})

	t.Run("responses malformed url", func(t *testing.T) {
		client := NewLiteLLMClient("://bad", "test-key", "gpt-5-codex")
		_, err := client.completeResponses(context.Background(), &CompletionRequest{Model: "gpt-5-codex", MaxTokens: 4})
		require.Error(t, err)
		assert.ErrorContains(t, err, "litellm responses request failed")
	})

	t.Run("responses unsupported parameter retries variants", func(t *testing.T) {
		var attempts int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			text := string(body)
			if strings.Contains(text, "max_output_tokens") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`unsupported parameter`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"ok\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "gpt-5-codex")
		client.client = server.Client()
		resp, err := client.completeResponses(context.Background(), &CompletionRequest{Model: "gpt-5-codex", MaxTokens: 4})
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Content)
		assert.GreaterOrEqual(t, attempts, 2)
	})

	t.Run("responses include temperature fallback branch", func(t *testing.T) {
		var payloads []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			payloads = append(payloads, string(body))
			if strings.Contains(string(body), "\"temperature\"") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`unsupported parameter`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"ok\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "gpt-5-codex")
		client.client = server.Client()
		resp, err := client.completeResponses(context.Background(), &CompletionRequest{Model: "gpt-5-codex", MaxTokens: 4, Temperature: 0.7})
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Content)
		require.Len(t, payloads, 2)
		assert.Contains(t, payloads[0], "\"temperature\"")
		assert.NotContains(t, payloads[1], "\"temperature\"")
	})

	t.Run("responses token field none branch", func(t *testing.T) {
		var payloads []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			payloads = append(payloads, string(body))
			if len(payloads) < 4 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`unsupported parameter`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"ok\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n"))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "test-key", "gpt-5-codex")
		client.client = server.Client()
		resp, err := client.completeResponses(context.Background(), &CompletionRequest{Model: "gpt-5-codex", MaxTokens: 4})
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Content)
		require.Len(t, payloads, 4)
		assert.NotContains(t, payloads[3], "max_output_tokens")
		assert.NotContains(t, payloads[3], "max_tokens")
		assert.NotContains(t, payloads[3], "max_completion_tokens")
	})

	t.Run("do json post malformed url", func(t *testing.T) {
		client := NewLiteLLMClient("http://example.com", "key", "m")
		_, _, err := client.doJSONPost(context.Background(), "://bad", map[string]interface{}{"ok": true})
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to create request")
	})
}

func TestParseResponsesSSEAdditionalBranches(t *testing.T) {
	sse := "" +
		"event: ignored\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n" +
		"data: {\"usage\":{\"input_tokens\":3,\"output_tokens\":4}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"\",\"output_text\":\" world\"}}\n\n"

	content, in, out, reason, err := parseResponsesSSE([]byte(sse))
	require.NoError(t, err)
	assert.Equal(t, "Hello", content)
	assert.Equal(t, 3, in)
	assert.Equal(t, 4, out)
	assert.Equal(t, "completed", reason)
}

func TestParseResponsesSSETrailingDataWithoutBlankLine(t *testing.T) {
	content, _, _, _, err := parseResponsesSSE([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"ok\"}}"))
	require.NoError(t, err)
	assert.Equal(t, "ok", content)

	_, _, _, _, err = parseResponsesSSE([]byte("data: {\"error\":{\"message\":\"bad\"}}"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "bad")
}

func TestLiteLLMDebugAndSystemBranches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "\"role\":\"system\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2},"model":"plain-model"}`))
	}))
	defer server.Close()

	client := NewLiteLLMClient(server.URL, "test-key", "plain-model")
	client.client = server.Client()

	stderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	_, err = client.completeChatCompletions(context.Background(), &CompletionRequest{
		Model:     "plain-model",
		System:    "system prompt",
		Debug:     true,
		MaxTokens: 4,
	})
	require.NoError(t, err)
	require.NoError(t, w.Close())
	os.Stderr = stderr

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Contains(t, buf.String(), "[debug] litellm request")
}

func TestLiteLLMGenericReturnBranchesAndDebugFallback(t *testing.T) {
	client := NewLiteLLMClient("http://example.com", "k", "plain-model")
	client.chatCompletionURLCandidates = func(base string) []string { return nil }
	_, err := client.completeChatCompletions(context.Background(), &CompletionRequest{Model: "plain-model"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "litellm API request failed on all chat/completions URL variants")

	client = NewLiteLLMClient("http://example.com", "k", "gpt-5-codex")
	client.responsesURLCandidates = func(base string) []string { return nil }
	_, err = client.completeResponses(context.Background(), &CompletionRequest{Model: "gpt-5-codex"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "litellm responses request failed on all URL and token-field variants")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "responses") {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`boom`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2},"model":"gpt-5-codex"}`))
	}))
	defer server.Close()

	client = NewLiteLLMClient(server.URL, "k", "gpt-5-codex")
	client.client = server.Client()

	stderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	_, err = client.Complete(context.Background(), &CompletionRequest{Model: "gpt-5-codex", Debug: true})
	require.NoError(t, err)
	require.NoError(t, w.Close())
	os.Stderr = stderr
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Contains(t, buf.String(), "responses failed; retrying with chat/completions fallback")
}

func TestLiteLLMDebugStatusBranches(t *testing.T) {
	t.Run("chat completions debug error body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`denied`))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "k", "plain-model")
		client.client = server.Client()

		stderr := os.Stderr
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stderr = w
		_, err = client.completeChatCompletions(context.Background(), &CompletionRequest{Model: "plain-model", Debug: true, MaxTokens: 1})
		require.Error(t, err)
		require.NoError(t, w.Close())
		os.Stderr = stderr
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		assert.Contains(t, buf.String(), "[debug] litellm response status=400 body=denied")
	})

	t.Run("responses debug success line", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"ok\"}}\n\n"))
		}))
		defer server.Close()

		client := NewLiteLLMClient(server.URL, "k", "gpt-5-codex")
		client.client = server.Client()

		stderr := os.Stderr
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stderr = w
		_, err = client.completeResponses(context.Background(), &CompletionRequest{Model: "gpt-5-codex", Debug: true, MaxTokens: 1})
		require.NoError(t, err)
		require.NoError(t, w.Close())
		os.Stderr = stderr
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		assert.Contains(t, buf.String(), "[debug] litellm responses status=200")
	})
}
