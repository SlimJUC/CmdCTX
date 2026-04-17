package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaConfig holds settings for the Ollama local provider.
type OllamaConfig struct {
	BaseURL string // default: http://localhost:11434
	Model   string // e.g. "llama3.2", "mistral", "codellama"
}

type ollamaProvider struct {
	cfg    OllamaConfig
	client *http.Client
}

// NewOllama creates a provider for a locally running Ollama instance.
func NewOllama(cfg OllamaConfig) Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	if cfg.Model == "" {
		cfg.Model = "llama3.2"
	}
	return &ollamaProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 120 * time.Second}, // local models can be slow
	}
}

func (p *ollamaProvider) Name() string      { return "ollama" }
func (p *ollamaProvider) ModelName() string { return p.cfg.Model }
func (p *ollamaProvider) IsLocal() bool     { return true }

// ollamaChatRequest uses the Ollama /api/chat endpoint for structured dialogue.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	Temperature float32 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Model string `json:"model"`
}

func (p *ollamaProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	messages := make([]ollamaMessage, 0, 2)
	if req.SystemPrompt != "" {
		messages = append(messages, ollamaMessage{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, ollamaMessage{Role: "user", Content: req.UserPrompt})

	body := ollamaChatRequest{
		Model:    p.cfg.Model,
		Messages: messages,
		Stream:   false,
		Options: ollamaOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshalling ollama request: %w", err)
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed (is Ollama running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(body))
	}

	respData, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading ollama response: %w", err)
	}

	var apiResp ollamaChatResponse
	if err := json.Unmarshal(respData, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing ollama response: %w", err)
	}

	return &CompletionResponse{
		Content:  apiResp.Message.Content,
		Model:    p.cfg.Model,
		Provider: "ollama",
	}, nil
}
