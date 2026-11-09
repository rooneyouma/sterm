package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClean(t *testing.T) {
	tests := []struct {
		name, in, want string
		wantErr        string
	}{
		{"plain", "ls -la", "ls -la", ""},
		{"surrounding whitespace", "  \n git status \n", "git status", ""},
		{"fence", "```\nls -la\n```", "ls -la", ""},
		{"fence with language", "```bash\nnpx create-next-app@latest\n```", "npx create-next-app@latest", ""},
		{"fence with chatter around it", "Here you go:\n```sh\ndf -h\n```\nHope that helps", "df -h", ""},
		{"inline backticks", "`pwd`", "pwd", ""},
		{"prompt prefix", "$ whoami", "whoami", ""},
		{"empty", "  ", "", "empty"},
		{"several lines", "cd app\nnpm install", "", "several lines"},
		{"declined", "# cannot: that needs a GUI", "", "that needs a GUI"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Clean(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Clean(%q) error = %v, want one containing %q", tt.in, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("Clean(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestSystemPrompt(t *testing.T) {
	s := SystemPrompt(Prompt{Shell: "zsh", OS: "Ubuntu 24.04", ProjectType: "Go", InGit: true})
	for _, want := range []string{"zsh", "Ubuntu 24.04", "Go project", "git repository", "ONLY the command"} {
		if !strings.Contains(s, want) {
			t.Errorf("system prompt is missing %q:\n%s", want, s)
		}
	}
	plain := SystemPrompt(Prompt{Shell: "bash", OS: "linux"})
	if strings.Contains(plain, "project") || strings.Contains(plain, "git repository") {
		t.Errorf("optional lines should be left out:\n%s", plain)
	}
}

func TestIsResearchRequest(t *testing.T) {
	tests := []struct {
		request string
		want    bool
	}{
		{"who is the founder of Example Inc?", true},
		{"what is the official website for Example Inc?", true},
		{"is Example Inc still active in 2026?", true},
		{"can you tell me whether the company is public or private?", true},
		{"where is the headquarters of Example Inc?", true},
		{"list the 5 largest files here", false},
		{"what command lists files?", false},
		{"show me the shell command for disk usage", false},
		{"how do I install Node.js on Ubuntu?", false},
	}
	for _, tt := range tests {
		if got := IsResearchRequest(tt.request); got != tt.want {
			t.Errorf("IsResearchRequest(%q) = %v, want %v", tt.request, got, tt.want)
		}
	}
}

type fake struct {
	name, out string
	err       error
}

func (f fake) Name() string { return f.name }
func (f fake) Generate(context.Context, Prompt) (string, error) {
	return f.out, f.err
}

type counting struct {
	fake
	n *int
}

func (c counting) Generate(ctx context.Context, p Prompt) (string, error) {
	*c.n++
	return c.fake.Generate(ctx, p)
}

func TestAskFallback(t *testing.T) {
	ctx := context.Background()

	cmd, used, err := Ask(ctx, []Generator{
		fake{name: "a", err: errors.New("boom")},
		fake{name: "b", out: "```\nls\n```"},
	}, Prompt{})
	if err != nil || cmd != "ls" || used != "b" {
		t.Errorf("fallback: got %q from %q, err %v", cmd, used, err)
	}

	cmd, used, err = Ask(ctx, []Generator{
		fake{name: "a", out: "one\ntwo"},
		fake{name: "b", out: "pwd"},
	}, Prompt{})
	if err != nil || cmd != "pwd" || used != "b" {
		t.Errorf("unusable answer should fall through: got %q from %q, err %v", cmd, used, err)
	}

	_, _, err = Ask(ctx, []Generator{
		fake{name: "a", err: errors.New("down")},
		fake{name: "b", err: errors.New("also down")},
	}, Prompt{})
	if err == nil || !strings.Contains(err.Error(), "down") || !strings.Contains(err.Error(), "also down") {
		t.Errorf("both failing should report both: %v", err)
	}
}

func TestAskStopsWhenOffline(t *testing.T) {
	calls := 0
	_, _, err := Ask(context.Background(), []Generator{
		fake{name: "a", err: ErrOffline},
		counting{fake: fake{name: "b", out: "ls"}, n: &calls},
	}, Prompt{})
	if err != ErrOffline {
		t.Errorf("want ErrOffline itself, got %v", err)
	}
	if calls != 0 {
		t.Errorf("no further provider should be tried once the network is down, but %d were", calls)
	}
}

type waitsForContext struct{ name string }

func (g waitsForContext) Name() string { return g.name }
func (g waitsForContext) Generate(ctx context.Context, _ Prompt) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestAskTimeoutAllowsFallback(t *testing.T) {
	cmd, used, err := AskWithTimeout(context.Background(), []Generator{
		waitsForContext{name: "slow"},
		fake{name: "fallback", out: "ls"},
	}, Prompt{}, 10*time.Millisecond)
	if err != nil || cmd != "ls" || used != "fallback" {
		t.Fatalf("fallback after timeout: got %q from %q, err %v", cmd, used, err)
	}
}

func TestOllamaDormantLayer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"message":{"role":"assistant","content":"ls -la"}}`))
	}))
	defer srv.Close()

	raw, err := (&Ollama{BaseURL: srv.URL, Model: "tiny"}).Generate(context.Background(), Prompt{Request: "list files"})
	if err != nil || raw != "ls -la" {
		t.Errorf("got %q, %v", raw, err)
	}
}

func TestDetectProject(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := DetectProject(deep); got != "" {
		t.Errorf("no marker anywhere should give empty, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DetectProject(deep); got != "Go" {
		t.Errorf("DetectProject(deep) = %q, want Go (found in a parent folder)", got)
	}
}
