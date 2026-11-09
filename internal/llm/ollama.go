package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Ollama struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

type ollamaRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

type ollamaResponse struct {
	Message chatMessage `json:"message"`
}

func (o *Ollama) Name() string { return "ollama/" + o.Model }

func (o *Ollama) Generate(ctx context.Context, p Prompt) (string, error) {
	base := strings.TrimRight(o.BaseURL, "/")
	if base == "" {
		base = "http://localhost:11434"
	}

	req := ollamaRequest{
		Model: o.Model,
		Messages: []chatMessage{
			{Role: "system", Content: SystemPrompt(p)},
			{Role: "user", Content: p.Request},
		},
		Stream:  false,
		Options: map[string]any{"temperature": 0},
	}

	var resp ollamaResponse
	if err := postJSON(ctx, o.Client, base+"/api/chat", nil, req, &resp); err != nil {
		var he *HTTPError
		var ue *url.Error
		switch {
		case errors.As(err, &he) && he.Status == http.StatusNotFound:
			return "", fmt.Errorf("model %q is not installed; run: ollama pull %s", o.Model, o.Model)
		case ctx.Err() != nil:
			return "", errors.New("timed out waiting for the local model")
		case errors.As(err, &ue):
			return "", fmt.Errorf("cannot reach Ollama at %s (is it running?)", base)
		}
		return "", err
	}
	return resp.Message.Content, nil
}
