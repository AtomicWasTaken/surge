package ai

import (
	"bytes"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// LiteLLMClient implements AIClient for litellm proxies (OpenAI-compatible API).
type LiteLLMClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewLiteLLMClient creates a new litellm client.
func NewLiteLLMClient(baseURL, apiKey, model string) *LiteLLMClient {
	return &LiteLLMClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 120 * 1e9}, // 120 seconds
	}
}

// Complete sends a completion request to the litellm proxy.
func (c *LiteLLMClient) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if useResponsesAPI(req.Model) {
		resp, err := c.completeResponses(ctx, req)
		if err == nil {
			return resp, nil
		}
		if req.Debug {
			fmt.Fprintf(os.Stderr, "[debug] litellm responses failed; retrying with chat/completions fallback: %v\n", err)
		}
		return c.completeChatCompletions(ctx, req)
	}

	return c.completeChatCompletions(ctx, req)
}

func (c *LiteLLMClient) completeChatCompletions(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Convert messages to OpenAI format
	messages := make([]map[string]string, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
	}

	urls := candidateChatCompletionURLs(c.baseURL)
	tokenFields := []string{"max_tokens", "max_completion_tokens"}
	var lastErr error

	for _, url := range urls {
		for _, tokenField := range tokenFields {
			if req.Debug {
				fmt.Fprintf(os.Stderr, "[debug] litellm request url=%s model=%s token_field=%s max_tokens=%d temperature=%.2f messages=%d\n",
					url, req.Model, tokenField, req.MaxTokens, req.Temperature, len(req.Messages))
			}

			payload := map[string]interface{}{
				"model":       req.Model,
				"messages":    messages,
				tokenField:    req.MaxTokens,
				"temperature": req.Temperature,
			}

			respBody, status, err := c.doJSONPost(ctx, url, payload)
			if err != nil {
				lastErr = fmt.Errorf("litellm request failed: %w", err)
				continue
			}
			if status == http.StatusOK {
				if req.Debug {
					fmt.Fprintf(os.Stderr, "[debug] litellm response status=%d\n", status)
				}
				return parseChatCompletionsResponse(respBody)
			}

			if req.Debug {
				fmt.Fprintf(os.Stderr, "[debug] litellm response status=%d body=%s\n", status, strings.TrimSpace(string(respBody)))
			}
			if status == http.StatusNotFound {
				lastErr = fmt.Errorf("litellm API error (%d): %s", status, string(respBody))
				break // try next URL
			}
			if status == http.StatusBadRequest && isUnsupportedParameter(respBody) {
				lastErr = fmt.Errorf("litellm API error (%d): %s", status, string(respBody))
				continue // try next token field
			}
			if status >= http.StatusInternalServerError {
				lastErr = fmt.Errorf("litellm API error (%d): %s", status, string(respBody))
				break // try next URL variant
			}
			return nil, fmt.Errorf("litellm API error (%d): %s", status, string(respBody))
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("litellm API request failed on all chat/completions URL variants")
}

func parseChatCompletionsResponse(respBody []byte) (*CompletionResponse, error) {
	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens     int `json:"total_tokens"`
		} `json:"usage"`
		Model string `json:"model"`
	}

	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to parse litellm response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from litellm")
	}

	return &CompletionResponse{
		Content:      openAIResp.Choices[0].Message.Content,
		Model:        openAIResp.Model,
		TokensIn:     openAIResp.Usage.PromptTokens,
		TokensOut:    openAIResp.Usage.CompletionTokens,
		FinishReason: openAIResp.Choices[0].FinishReason,
	}, nil
}

func (c *LiteLLMClient) completeResponses(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	urls := candidateResponsesURLs(c.baseURL)
	tokenFields := []string{"max_output_tokens", "max_tokens", "max_completion_tokens", ""}
	var lastErr error

	for _, url := range urls {
		for _, tokenField := range tokenFields {
			includeTemperatureOptions := []bool{true}
			if req.Temperature > 0 {
				includeTemperatureOptions = []bool{true, false}
			}
			for _, includeTemperature := range includeTemperatureOptions {
			debugTokenField := tokenField
			if debugTokenField == "" {
				debugTokenField = "<none>"
			}
			if req.Debug {
				tempLabel := "<none>"
				if includeTemperature {
					tempLabel = fmt.Sprintf("%.2f", req.Temperature)
				}
				fmt.Fprintf(os.Stderr, "[debug] litellm responses request url=%s model=%s token_field=%s max_tokens=%d temperature=%s\n",
					url, req.Model, debugTokenField, req.MaxTokens, tempLabel)
			}

			payload := map[string]interface{}{
				"model":        req.Model,
				"instructions": req.System,
				"input":        responsesInputFromMessages(req.Messages),
				"stream":       true,
			}
			if tokenField != "" {
				payload[tokenField] = req.MaxTokens
			}
			if includeTemperature && req.Temperature > 0 {
				payload["temperature"] = req.Temperature
			}

			respBody, status, err := c.doJSONPost(ctx, url, payload)
			if err != nil {
				lastErr = fmt.Errorf("litellm responses request failed: %w", err)
				continue
			}
			if status == http.StatusOK {
				if req.Debug {
					fmt.Fprintf(os.Stderr, "[debug] litellm responses status=%d\n", status)
				}
				content, tokensIn, tokensOut, finishReason, err := parseResponsesSSE(respBody)
				if err != nil {
					return nil, fmt.Errorf("failed to parse litellm responses stream: %w", err)
				}
				return &CompletionResponse{
					Content:      content,
					Model:        req.Model,
					TokensIn:     tokensIn,
					TokensOut:    tokensOut,
					FinishReason: finishReason,
				}, nil
			}

			if req.Debug {
				fmt.Fprintf(os.Stderr, "[debug] litellm responses status=%d body=%s\n", status, strings.TrimSpace(string(respBody)))
			}
			if status == http.StatusNotFound {
				lastErr = fmt.Errorf("litellm responses API error (%d): %s", status, string(respBody))
				break // try next URL variant
			}
			if status == http.StatusBadRequest && isUnsupportedParameter(respBody) {
				lastErr = fmt.Errorf("litellm responses API error (%d): %s", status, string(respBody))
				continue // try next payload variant
			}
			if status >= http.StatusInternalServerError {
				lastErr = fmt.Errorf("litellm responses API error (%d): %s", status, string(respBody))
				break // try next URL variant
			}
			return nil, fmt.Errorf("litellm responses API error (%d): %s", status, string(respBody))
		}
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("litellm responses request failed on all URL and token-field variants")
}

func (c *LiteLLMClient) doJSONPost(ctx context.Context, url string, payload map[string]interface{}) ([]byte, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

func candidateResponsesURLs(base string) []string {
	var candidates []string
	for _, b := range normalizedBaseCandidates(base) {
		candidates = append(candidates,
			b+"/v1/responses",
			b+"/v1/openai/v1/responses",
			b+"/responses",
		)
	}
	return uniqueStrings(candidates)
}

func candidateChatCompletionURLs(base string) []string {
	var candidates []string
	for _, b := range normalizedBaseCandidates(base) {
		candidates = append(candidates,
			b+"/v1/chat/completions",
			b+"/v1/openai/v1/chat/completions",
			b+"/chat/completions",
		)
	}
	return uniqueStrings(candidates)
}

func normalizedBaseCandidates(base string) []string {
	base = strings.TrimSuffix(base, "/")
	candidates := []string{
		base,
		strings.TrimSuffix(base, "/v1"),
		strings.TrimSuffix(base, "/openai/v1"),
		strings.TrimSuffix(base, "/v1/openai/v1"),
	}
	return uniqueStrings(candidates)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

func isUnsupportedParameter(body []byte) bool {
	lowerBody := strings.ToLower(string(body))
	return strings.Contains(lowerBody, "unsupported parameter")
}

func useResponsesAPI(model string) bool {
	return strings.Contains(strings.ToLower(model), "codex")
}

func responsesInputFromMessages(messages []Message) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		items = append(items, map[string]interface{}{
			"role": m.Role,
			"content": []map[string]string{
				{
					"type": "input_text",
					"text": m.Content,
				},
			},
		})
	}
	return items
}

func parseResponsesSSE(body []byte) (string, int, int, string, error) {
	type event struct {
		Type     string `json:"type"`
		Delta    string `json:"delta"`
		Error    struct {
			Message string `json:"message"`
		} `json:"error"`
		Response struct {
			Status     string `json:"status"`
			OutputText string `json:"output_text"`
			Usage      struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Output []struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		} `json:"response"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	var content strings.Builder
	tokensIn := 0
	tokensOut := 0
	finishReason := "completed"

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	var dataLines []string

	processData := func(data string) error {
		if data == "" || data == "[DONE]" {
			return nil
		}
		var ev event
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return nil
		}
		if ev.Error.Message != "" {
			return fmt.Errorf(ev.Error.Message)
		}
		switch ev.Type {
		case "response.output_text.delta":
			content.WriteString(ev.Delta)
		case "response.completed":
			if content.Len() == 0 {
				if ev.Response.OutputText != "" {
					content.WriteString(ev.Response.OutputText)
				} else {
					for _, out := range ev.Response.Output {
						for _, c := range out.Content {
							if c.Type == "output_text" || c.Type == "text" {
								content.WriteString(c.Text)
							}
						}
					}
				}
			}
			if ev.Response.Usage.InputTokens > 0 {
				tokensIn = ev.Response.Usage.InputTokens
			}
			if ev.Response.Usage.OutputTokens > 0 {
				tokensOut = ev.Response.Usage.OutputTokens
			}
			if ev.Response.Status != "" {
				finishReason = ev.Response.Status
			}
		}
		if ev.Usage.InputTokens > 0 {
			tokensIn = ev.Usage.InputTokens
		}
		if ev.Usage.OutputTokens > 0 {
			tokensOut = ev.Usage.OutputTokens
		}
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if len(dataLines) > 0 {
				if err := processData(strings.Join(dataLines, "\n")); err != nil {
					return "", 0, 0, "", err
				}
				dataLines = dataLines[:0]
			}
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	if len(dataLines) > 0 {
		if err := processData(strings.Join(dataLines, "\n")); err != nil {
			return "", 0, 0, "", err
		}
	}
	if err := scanner.Err(); err != nil {
		return "", 0, 0, "", fmt.Errorf("failed reading SSE stream: %w", err)
	}
	if strings.TrimSpace(content.String()) == "" {
		return "", 0, 0, "", fmt.Errorf("empty responses output")
	}

	return content.String(), tokensIn, tokensOut, finishReason, nil
}

// Name returns the provider name.
func (c *LiteLLMClient) Name() string {
	return "litellm"
}
