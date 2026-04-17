// Package ai provides a clean abstraction over AI providers.
// The design enforces that AI is used only for structured intent extraction,
// never for generating raw shell commands directly.
package ai

import (
	"context"
	"fmt"

	"github.com/slim/cmdctx/internal/config"
)

// CompletionRequest is the structured input sent to an AI provider.
type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	// MaxTokens limits the response length. Providers should respect this.
	MaxTokens int
	// Temperature controls determinism. Lower = more deterministic.
	Temperature float32
}

// CompletionResponse holds the raw text response from the AI.
type CompletionResponse struct {
	Content    string
	TokensUsed int
	Model      string
	Provider   string
}

// Provider is the interface all AI backends must implement.
// It is intentionally minimal: we only need structured text completion.
type Provider interface {
	// Complete sends a prompt and returns a structured response.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	// Name returns the provider's identifier string.
	Name() string
	// ModelName returns the model being used.
	ModelName() string
	// IsLocal returns true if no data leaves the machine.
	IsLocal() bool
}

// NewFromConfig constructs a Provider from the active provider config.
// Returns an error if the provider type is unknown or misconfigured.
func NewFromConfig(cfg *config.Config) (Provider, error) {
	p := cfg.ActiveProviderConfig()
	if p == nil {
		// Default to Ollama if nothing is configured.
		return NewOllama(OllamaConfig{
			BaseURL: "http://localhost:11434",
			Model:   "llama3.2",
		}), nil
	}

	switch p.Type {
	case "ollama":
		return NewOllama(OllamaConfig{
			BaseURL: p.BaseURL,
			Model:   p.Model,
		}), nil
	case "openai":
		if p.APIKey == "" {
			return nil, fmt.Errorf("openai provider requires api_key")
		}
		return NewOpenAI(OpenAIConfig{
			BaseURL: p.BaseURL,
			APIKey:  p.APIKey,
			Model:   p.Model,
		}), nil
	case "anthropic":
		if p.APIKey == "" {
			return nil, fmt.Errorf("anthropic provider requires api_key")
		}
		return NewAnthropic(AnthropicConfig{
			APIKey: p.APIKey,
			Model:  p.Model,
		}), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %q (supported: ollama, openai, anthropic)", p.Type)
	}
}

// NullProvider is used in tests and when no AI provider is configured.
// It returns a predictable empty response that callers can detect.
type NullProvider struct{}

func (n *NullProvider) Complete(_ context.Context, _ CompletionRequest) (*CompletionResponse, error) {
	return nil, fmt.Errorf("no AI provider configured — run 'cmdctx providers' to set one up")
}
func (n *NullProvider) Name() string      { return "none" }
func (n *NullProvider) ModelName() string { return "" }
func (n *NullProvider) IsLocal() bool     { return true }
