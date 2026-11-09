package main

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rooneyouma/sterm/internal/llm"

	"github.com/rooneyouma/sterm/internal/config"
)

func TestGeminiModelFallbackOrder(t *testing.T) {
	cfg := config.Default()
	got := backendModels("gemini", cfg)
	want := []string{
		"gemini-3.5-flash-lite",
		"gemini-3.1-flash-lite",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backendModels(gemini) = %v, want %v", got, want)
	}
}

func TestCacheKeySeparatesResearchFromCommands(t *testing.T) {
	p := llm.Prompt{Request: "who founded Example Inc?", OS: "linux", Shell: "zsh"}
	commandKey := cacheKey(p, "gemini/model", "command")
	researchKey := cacheKey(p, "gemini/model", "research")
	if commandKey == researchKey {
		t.Fatal("research and command responses must not share a cache entry")
	}
}

func TestStripRequestMarker(t *testing.T) {
	for input, want := range map[string]string{
		"## list files": "list files",
		"# list files":  "list files",
		"list files":    "list files",
	} {
		if got := stripRequestMarker(input); got != want {
			t.Errorf("stripRequestMarker(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestConfirmAndRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := confirmAndRun("printf 'file output\\n'", "sh", "", strings.NewReader("y\n"), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "file output\n" {
		t.Fatalf("command output = %q, want visible command output", got)
	}
}

func TestConfirmAndRunDeclines(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := confirmAndRun("printf 'should not run'", "sh", "", strings.NewReader("n\n"), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("declined command produced output: %q", got)
	}
	if !strings.Contains(stderr.String(), "Not run.") {
		t.Fatalf("decline confirmation not reported: %q", stderr.String())
	}
}

func TestConfirmAndRunReturnsWithTerminalOpen(t *testing.T) {
	input, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer output.Close()
	if _, err := output.WriteString("y\n"); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- confirmAndRun("true", "sh", "", input, &bytes.Buffer{}, &bytes.Buffer{})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("command did not return while terminal input remained open")
	}
}
