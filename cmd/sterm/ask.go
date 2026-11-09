package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/rooneyouma/sterm/internal/config"
	"github.com/rooneyouma/sterm/internal/filter"
	"github.com/rooneyouma/sterm/internal/llm"
	"github.com/rooneyouma/sterm/internal/safety"
	"github.com/rooneyouma/sterm/internal/store"
)

func runAsk(args []string) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	cwdFlag := fs.String("cwd", "", "current directory (default: actual current directory)")
	shell := fs.String("shell", "", "shell to target (default: detected from $SHELL)")
	backend := fs.String("backend", "", "use only this backend: anthropic, openai, gemini, openrouter or ollama")
	fresh := fs.Bool("fresh", false, "ignore the cached answer and ask the model again")
	list := fs.Bool("list", false, "show which backends are configured, then exit")
	run := fs.Bool("run", false, "ask before running the generated command")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfgPath, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if *list {
		printBackends(cfg)
		return nil
	}

	request := stripRequestMarker(strings.Join(fs.Args(), " "))
	if request == "" {
		return errors.New(`usage: sterm ask "what you want to do"`)
	}

	cwd, gitRoot := resolveContext(*cwdFlag, "")
	p := llm.Prompt{
		Request:     request,
		OS:          llm.DetectOS(),
		Shell:       *shell,
		ProjectType: llm.DetectProject(cwd),
		InGit:       gitRoot != "",
	}
	if p.Shell == "" {
		p.Shell = llm.DetectShell()
	}

	order := append([]string{cfg.Backend}, cfg.Fallbacks...)
	if *backend != "" {
		order = []string{*backend}
	}
	var gens []llm.Generator
	var skipped []string
	seen := map[string]bool{}
	for _, name := range order {
		for _, model := range backendModels(name, cfg) {
			id := name + "/" + model
			if seen[id] {
				continue
			}
			seen[id] = true
			g, err := newGenerator(name, model, cfg)
			if err != nil {
				skipped = append(skipped, err.Error())
				break
			}
			gens = append(gens, g)
		}
	}
	if len(gens) == 0 {
		return errors.New(strings.Join(skipped, "; "))
	}
	for _, msg := range skipped {
		fmt.Fprintln(os.Stderr, "note: skipped "+msg)
	}

	var st *store.Store
	if s, err := openStore(); err == nil {
		st = s
		defer st.Close()
	}
	mode := "command"
	if llm.IsResearchRequest(p.Request) {
		mode = "research"
	}
	key := cacheKey(p, gens[0].Name(), mode)

	if st != nil && !*fresh {
		if cmd, ok, err := st.GetAsk(key); err == nil && ok {
			if llm.IsResearchRequest(p.Request) {
				fmt.Println(cmd)
				return nil
			}
			return showAnswer(cmd, p.Shell, cwd, *run, os.Stdin, os.Stdout, os.Stderr)
		}
	}

	ctx := context.Background()
	if llm.IsResearchRequest(p.Request) {
		answer, _, err := llm.ResearchWithTimeout(ctx, gens, p, cfg.Timeout())
		if err != nil {
			return err
		}
		if st != nil && !filter.ShouldSkip(answer) {
			_ = st.PutAsk(key, answer)
		}
		fmt.Println(answer)
		return nil
	}

	cmd, _, err := llm.AskWithTimeout(ctx, gens, p, cfg.Timeout())
	if err != nil {
		return err
	}
	if st != nil && !filter.ShouldSkip(cmd) {
		_ = st.PutAsk(key, cmd)
	}
	return showAnswer(cmd, p.Shell, cwd, *run, os.Stdin, os.Stdout, os.Stderr)
}

func backendModels(name string, cfg config.Config) []string {
	if name == "gemini" {
		return append([]string{cfg.Gemini.Model}, cfg.Gemini.FallbackModels...)
	}
	if provider, ok := cfg.Cloud(name); ok {
		return []string{provider.Model}
	}
	if name == "ollama" {
		return []string{cfg.Ollama.Model}
	}
	return []string{""}
}

func stripRequestMarker(request string) string {
	request = strings.TrimSpace(request)
	for _, marker := range []string{"## ", "# "} {
		if strings.HasPrefix(request, marker) {
			return strings.TrimSpace(strings.TrimPrefix(request, marker))
		}
	}
	return request
}

func newGenerator(name, model string, cfg config.Config) (llm.Generator, error) {
	switch name {
	case "ollama":
		return &llm.Ollama{BaseURL: cfg.Ollama.URL, Model: model}, nil
	case "anthropic", "openai", "gemini", "openrouter":
		pc, _ := cfg.Cloud(name)
		key := os.Getenv(pc.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("%s: environment variable %s is not set", name, pc.APIKeyEnv)
		}
		switch name {
		case "anthropic":
			return &llm.Anthropic{Model: model, APIKey: key}, nil
		case "gemini":
			return &llm.Gemini{Model: model, APIKey: key}, nil
		default:
			return &llm.OpenAICompat{Provider: name, Model: model, APIKey: key}, nil
		}
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}

func printBackends(cfg config.Config) {
	fmt.Printf("primary:   %s\n", cfg.Backend)
	if len(cfg.Fallbacks) > 0 {
		fmt.Printf("fallbacks: %s\n", strings.Join(cfg.Fallbacks, ", "))
	}
	fmt.Println()
	for _, name := range config.Backends {
		if name == "ollama" {
			state := "local model (available when selected)"
			if cfg.Backend == "ollama" {
				state = "primary local model"
			}
			fmt.Printf("%-11s model=%-28s %s\n", name, cfg.Ollama.Model, state)
			continue
		}
		pc, _ := cfg.Cloud(name)
		state := "ready"
		if os.Getenv(pc.APIKeyEnv) == "" {
			state = "missing " + pc.APIKeyEnv
		}
		fmt.Printf("%-11s model=%-28s %s\n", name, pc.Model, state)
		if name == "gemini" && len(pc.FallbackModels) > 0 {
			fmt.Printf("%-11s fallback models: %s\n", "", strings.Join(pc.FallbackModels, ", "))
		}
	}
}

func cacheKey(p llm.Prompt, backendName, mode string) string {
	norm := strings.ToLower(strings.Join(strings.Fields(p.Request), " "))
	sum := sha256.Sum256([]byte(strings.Join([]string{
		norm, p.OS, p.Shell, p.ProjectType, fmt.Sprint(p.InGit), backendName, mode,
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func emit(cmd string) {
	fmt.Println(cmd)
	for _, reason := range safety.Check(cmd) {
		fmt.Fprintln(os.Stderr, "warning: "+reason)
	}
}

func showAnswer(cmd, shell, cwd string, run bool, stdin io.Reader, stdout, stderr io.Writer) error {
	emitTo(cmd, stdout, stderr)
	if !run {
		return nil
	}
	return confirmAndRun(cmd, shell, cwd, stdin, stdout, stderr)
}

func emitTo(cmd string, stdout, stderr io.Writer) {
	fmt.Fprintln(stdout, cmd)
	for _, reason := range safety.Check(cmd) {
		fmt.Fprintln(stderr, "warning: "+reason)
	}
}

func confirmAndRun(cmd, shell, cwd string, stdin io.Reader, stdout, stderr io.Writer) error {
	fmt.Fprint(stderr, "Run this command? [y/N] ")
	answer, err := readConfirmation(stdin)
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if answer != "y" && answer != "Y" && !strings.EqualFold(answer, "yes") {
		fmt.Fprintln(stderr, "Not run.")
		return nil
	}

	command := exec.Command(shell, "-c", cmd)
	command.Dir = cwd
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

func readConfirmation(stdin io.Reader) (string, error) {
	var answer strings.Builder
	var one [1]byte
	for {
		_, err := stdin.Read(one[:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				return strings.TrimSpace(answer.String()), nil
			}
			return "", err
		}
		if one[0] == '\n' {
			return strings.TrimSpace(answer.String()), nil
		}
		if one[0] != '\r' {
			answer.WriteByte(one[0])
		}
	}
}
