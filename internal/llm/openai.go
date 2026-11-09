package llm

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type OpenAICompat struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
	Client   *http.Client
}

type openAIRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type openAIResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (o *OpenAICompat) Name() string { return o.Provider + "/" + o.Model }

func (o *OpenAICompat) baseURL() string {
	if b := strings.TrimRight(o.BaseURL, "/"); b != "" {
		return b
	}
	if o.Provider == "openrouter" {
		return "https://openrouter.ai/api/v1"
	}
	return "https://api.openai.com/v1"
}

func (o *OpenAICompat) Generate(ctx context.Context, p Prompt) (string, error) {
	if o.APIKey == "" {
		return "", errors.New("no API key set")
	}

	req := openAIRequest{
		Model: o.Model,
		Messages: []chatMessage{
			{Role: "system", Content: SystemPrompt(p)},
			{Role: "user", Content: p.Request},
		},
	}
	headers := map[string]string{"Authorization": "Bearer " + o.APIKey}

	var resp openAIResponse
	if err := postJSON(ctx, o.Client, o.baseURL()+"/chat/completions", headers, req, &resp); err != nil {
		return "", providerError(ctx, o.Provider, o.Model, o.APIKey, err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New(o.Provider + " returned no answer")
	}
	return resp.Choices[0].Message.Content, nil
}
