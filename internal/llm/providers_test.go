package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testKey = "sk-test-SECRET-123"

func builders() map[string]func(url string) Generator {
	return map[string]func(string) Generator{
		"anthropic": func(u string) Generator { return &Anthropic{BaseURL: u, Model: "m", APIKey: testKey} },
		"openai": func(u string) Generator {
			return &OpenAICompat{Provider: "openai", BaseURL: u, Model: "m", APIKey: testKey}
		},
		"openrouter": func(u string) Generator {
			return &OpenAICompat{Provider: "openrouter", BaseURL: u, Model: "m", APIKey: testKey}
		},
		"gemini": func(u string) Generator { return &Gemini{BaseURL: u, Model: "m", APIKey: testKey} },
	}
}

func TestOpenAICompat(t *testing.T) {
	var gotAuth, gotPath, gotModel string
	var msgs []chatMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		var req struct {
			Model    string        `json:"model"`
			Messages []chatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		gotModel, msgs = req.Model, req.Messages
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ls -la"}}]}`))
	}))
	defer srv.Close()

	o := &OpenAICompat{Provider: "openrouter", BaseURL: srv.URL + "/api/v1", Model: "some/model", APIKey: "k1"}
	raw, err := o.Generate(context.Background(), Prompt{Request: "list files", Shell: "zsh", OS: "Ubuntu"})
	if err != nil || raw != "ls -la" {
		t.Fatalf("got %q, %v", raw, err)
	}
	if gotPath != "/api/v1/chat/completions" || gotAuth != "Bearer k1" || gotModel != "some/model" {
		t.Errorf("path=%q auth=%q model=%q", gotPath, gotAuth, gotModel)
	}
	if len(msgs) != 2 || msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "zsh") || msgs[1].Content != "list files" {
		t.Errorf("messages wrong: %+v", msgs)
	}
	if o.Name() != "openrouter/some/model" {
		t.Errorf("Name() = %q", o.Name())
	}
}

func TestOpenAICompatDefaults(t *testing.T) {
	if got := (&OpenAICompat{Provider: "openai"}).baseURL(); got != "https://api.openai.com/v1" {
		t.Errorf("openai default base = %q", got)
	}
	if got := (&OpenAICompat{Provider: "openrouter"}).baseURL(); got != "https://openrouter.ai/api/v1" {
		t.Errorf("openrouter default base = %q", got)
	}
}

func TestOpenAICompatNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	_, err := (&OpenAICompat{Provider: "openai", BaseURL: srv.URL, Model: "m", APIKey: "k"}).Generate(context.Background(), Prompt{})
	if err == nil || !strings.Contains(err.Error(), "no answer") {
		t.Errorf("an empty choices list should be an error, got %v", err)
	}
}

func TestGemini(t *testing.T) {
	var gotPath, gotQuery, gotKey string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotKey = r.URL.Path, r.URL.RawQuery, r.Header.Get("x-goog-api-key")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"git "},{"text":"status"}]}}]}`))
	}))
	defer srv.Close()

	g := &Gemini{BaseURL: srv.URL + "/v1beta", Model: "models/gemini-test", APIKey: "gk"}
	raw, err := g.Generate(context.Background(), Prompt{Request: "show status", Shell: "zsh", OS: "Ubuntu"})
	if err != nil || raw != "git status" {
		t.Fatalf("got %q, %v", raw, err)
	}
	if gotPath != "/v1beta/models/gemini-test:generateContent" {
		t.Errorf("path = %q", gotPath)
	}
	if gotKey != "gk" {
		t.Errorf("x-goog-api-key = %q", gotKey)
	}
	if gotQuery != "" {
		t.Errorf("the key must not travel in the URL, but the query was %q", gotQuery)
	}
	sys, _ := json.Marshal(body["systemInstruction"])
	if !strings.Contains(string(sys), "zsh") {
		t.Errorf("system instruction missing context: %s", sys)
	}
}

func TestGeminiResearchUsesGoogleSearchAndReturnsSources(t *testing.T) {
	var gotTools []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tools []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		gotTools = req.Tools
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Example Inc was founded by Ada Example."}]},"groundingMetadata":{"groundingChunks":[{"web":{"uri":"https://example.com/about","title":"Example Inc About"}}]}}]}`))
	}))
	defer srv.Close()

	g := &Gemini{BaseURL: srv.URL, Model: "m", APIKey: "test"}
	answer, err := g.Research(context.Background(), Prompt{Request: "who founded Example Inc?"})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotTools) != 1 || gotTools[0]["google_search"] == nil {
		t.Fatalf("Gemini tools = %#v, want google_search", gotTools)
	}
	for _, want := range []string{"Ada Example", "Sources:", "Example Inc About", "https://example.com/about"} {
		if !strings.Contains(answer, want) {
			t.Errorf("research answer %q missing %q", answer, want)
		}
	}
}

func TestGeminiBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"candidates":[]}`))
	}))
	defer srv.Close()
	_, err := (&Gemini{BaseURL: srv.URL, Model: "m", APIKey: "k"}).Generate(context.Background(), Prompt{})
	if err == nil || !strings.Contains(err.Error(), "no answer") {
		t.Errorf("no candidates should be an error, got %v", err)
	}
}

func TestProviderHTTPErrors(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{401, "rejected the key"},
		{403, "rejected the key"},
		{404, "was not found"},
		{429, "rate limited"},
		{503, "busy or down"},
		{400, "rejected the request"},
	}
	for name, build := range builders() {
		for _, tt := range cases {
			t.Run(fmt.Sprintf("%s/%d", name, tt.status), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "problem with key "+testKey, tt.status)
				}))
				defer srv.Close()

				_, err := build(srv.URL).Generate(context.Background(), Prompt{})
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("want error containing %q, got %v", tt.want, err)
				}
				if errors.Is(err, ErrOffline) {
					t.Error("an HTTP error is not 'offline'")
				}
				if strings.Contains(err.Error(), testKey) {
					t.Errorf("the API key leaked into an error message: %v", err)
				}
			})
		}
	}
}

func TestProvidersOffline(t *testing.T) {
	for name, build := range builders() {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			u := srv.URL
			srv.Close()

			_, err := build(u).Generate(context.Background(), Prompt{})
			if !errors.Is(err, ErrOffline) {
				t.Fatalf("want ErrOffline, got %v", err)
			}
			if !strings.Contains(err.Error(), "connect to the internet") {
				t.Errorf("message should say what to do, got %q", err)
			}
		})
	}
}

func TestProvidersTimeout(t *testing.T) {
	for name, build := range builders() {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-time.After(3 * time.Second):
				}
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			_, err := build(srv.URL).Generate(ctx, Prompt{})
			if err == nil || !strings.Contains(err.Error(), "timed out") {
				t.Errorf("a slow server should hit the timeout, got %v", err)
			}
			if errors.Is(err, ErrOffline) {
				t.Error("a timeout is not 'offline'")
			}
		})
	}
}

func TestProvidersNeedAKey(t *testing.T) {
	noKey := []Generator{
		&Anthropic{Model: "m"},
		&OpenAICompat{Provider: "openai", Model: "m"},
		&Gemini{Model: "m"},
	}
	for _, g := range noKey {
		if _, err := g.Generate(context.Background(), Prompt{}); err == nil {
			t.Errorf("%s: an empty key should be rejected before any request", g.Name())
		}
	}
}
