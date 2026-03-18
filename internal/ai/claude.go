package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ClaudeClient implements AIClient for the Anthropic Claude API.
type ClaudeClient struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewClaudeClient creates a new Claude API client.
func NewClaudeClient(apiKey, model string) *ClaudeClient {
	return &ClaudeClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.anthropic.com/v1",
		client:  &http.Client{Timeout: 120 * 1e9},
	}
}

// Complete sends a completion request to the Claude API.
func (c *ClaudeClient) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	url := c.baseURL + "/messages"

	// Build messages in Anthropic format
	messages := make([]map[string]string, 0, len(req.Messages)+1)
	if req.System != "" {
		// Anthropic uses a system role
		messages = append(messages, map[string]string{"role": "user", "content": req.System})
	}
	for _, m := range req.Messages {
		messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
	}

	payload := map[string]interface{}{
		"model":    c.model,
		"messages":  messages,
		"max_tokens": req.MaxTokens,
	}

	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("anthropic-dangerous-direct-browser-access", "true")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("claude request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("claude API error (%s): %s", resp.Status, errResp.Error.Message)
		}
		return nil, fmt.Errorf("claude API error (%s): %s", resp.Status, string(respBody))
	}

	var claudeResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}

	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to parse claude response: %w", err)
	}

	// Extract text content
	var content strings.Builder
	for _, block := range claudeResp.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}

	return &CompletionResponse{
		Content:      content.String(),
		Model:        c.model,
		TokensIn:     claudeResp.Usage.InputTokens,
		TokensOut:    claudeResp.Usage.OutputTokens,
		FinishReason: claudeResp.StopReason,
	}, nil
}

// Name returns the provider name.
func (c *ClaudeClient) Name() string {
	return "claude"
}
