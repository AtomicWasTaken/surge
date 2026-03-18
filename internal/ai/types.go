package ai

import "context"

// CompletionRequest represents a request to the AI model.
type CompletionRequest struct {
	Model       string
	Messages    []Message
	MaxTokens   int
	Temperature float64
	System      string
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionResponse represents a response from the AI model.
type CompletionResponse struct {
	Content    string `json:"content"`
	Model      string `json:"model"`
	TokensIn   int    `json:"tokensIn"`
	TokensOut  int    `json:"tokensOut"`
	FinishReason string `json:"finishReason"`
}

// AIClient is the interface for AI model interactions.
type AIClient interface {
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
	Name() string
}
