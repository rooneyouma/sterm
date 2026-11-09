package llm

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type Anthropic struct {
	BaseURL string
	Model   string
	APIKey  string
	Client  *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system"`
	Messages  []chatMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (a *Anthropic) Name() string { return "anthropic/" + a.Model }

func (a *Anthropic) Generate(ctx context.Context, p Prompt) (string, error) {
	if a.APIKey == "" {
		return "", errors.New("no API key set")
	}
	base := strings.TrimRight(a.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}

	req := anthropicRequest{
		Model:     a.Model,
		MaxTokens: 300,
		System:    SystemPrompt(p),
		Messages:  []chatMessage{{Role: "user", Content: p.Request}},
	}
	headers := map[string]string{
		"x-api-key":         a.APIKey,
		"anthropic-version": "2023-06-01",
	}

	var resp anthropicResponse
	if err := postJSON(ctx, a.Client, base+"/v1/messages", headers, req, &resp); err != nil {
		return "", providerError(ctx, "anthropic", a.Model, a.APIKey, err)
	}

	var out strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			out.WriteString(block.Text)
		}
	}
	return out.String(), nil
}
