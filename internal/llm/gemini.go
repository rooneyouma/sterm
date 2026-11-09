package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Gemini struct {
	BaseURL string
	Model   string
	APIKey  string
	Client  *http.Client
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	SystemInstruction geminiContent    `json:"systemInstruction"`
	Contents          []geminiContent  `json:"contents"`
	Tools             []map[string]any `json:"tools,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content           geminiContent `json:"content"`
		GroundingMetadata struct {
			GroundingChunks []struct {
				Web struct {
					URI   string `json:"uri"`
					Title string `json:"title"`
				} `json:"web"`
			} `json:"groundingChunks"`
		} `json:"groundingMetadata"`
	} `json:"candidates"`
}

func (g *Gemini) Name() string { return "gemini/" + g.Model }

func (g *Gemini) Generate(ctx context.Context, p Prompt) (string, error) {
	return g.generate(ctx, p, false)
}

// Research answers factual questions with Gemini's Google Search grounding.
func (g *Gemini) Research(ctx context.Context, p Prompt) (string, error) {
	return g.generate(ctx, p, true)
}

func (g *Gemini) generate(ctx context.Context, p Prompt, research bool) (string, error) {
	if g.APIKey == "" {
		return "", errors.New("no API key set")
	}
	base := strings.TrimRight(g.BaseURL, "/")
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	model := strings.TrimPrefix(g.Model, "models/")
	endpoint := base + "/models/" + url.PathEscape(model) + ":generateContent"

	system := SystemPrompt(p)
	req := geminiRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: system}}},
		Contents:          []geminiContent{{Role: "user", Parts: []geminiPart{{Text: p.Request}}}},
	}
	if research {
		req.SystemInstruction.Parts[0].Text = "Answer the user's factual question using Google Search. Be concise, distinguish verified facts from uncertainty, and do not provide shell commands. Use inline citations where possible and list the supplied source titles and URLs."
		req.Tools = []map[string]any{{"google_search": map[string]any{}}}
	}
	headers := map[string]string{"x-goog-api-key": g.APIKey}

	var resp geminiResponse
	if err := postJSON(ctx, g.Client, endpoint, headers, req, &resp); err != nil {
		return "", providerError(ctx, "gemini", g.Model, g.APIKey, err)
	}
	if len(resp.Candidates) == 0 {
		return "", errors.New("gemini returned no answer (the request may have been blocked)")
	}

	var out strings.Builder
	candidate := resp.Candidates[0]
	for _, part := range candidate.Content.Parts {
		out.WriteString(part.Text)
	}
	if out.Len() == 0 {
		return "", errors.New("gemini returned an empty answer")
	}
	if research {
		seen := map[string]bool{}
		out.WriteString("\n\nSources:")
		for _, chunk := range candidate.GroundingMetadata.GroundingChunks {
			if chunk.Web.URI == "" || seen[chunk.Web.URI] {
				continue
			}
			seen[chunk.Web.URI] = true
			if chunk.Web.Title != "" {
				fmt.Fprintf(&out, "\n- %s: %s", chunk.Web.Title, chunk.Web.URI)
			} else {
				fmt.Fprintf(&out, "\n- %s", chunk.Web.URI)
			}
		}
		if len(seen) == 0 {
			out.WriteString("\n- No web sources were returned.")
		}
	}
	return out.String(), nil
}
