package llm

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Prompt struct {
	Request     string
	OS          string
	Shell       string
	ProjectType string
	InGit       bool
}

type Generator interface {
	Name() string
	Generate(ctx context.Context, p Prompt) (string, error)
}

type ResearchGenerator interface {
	Generator
	Research(ctx context.Context, p Prompt) (string, error)
}

func IsResearchRequest(request string) bool {
	r := strings.ToLower(strings.TrimSpace(request))
	for _, marker := range []string{"## ", "# "} {
		if strings.HasPrefix(r, marker) {
			r = strings.TrimSpace(strings.TrimPrefix(r, marker))
		}
	}
	if r == "" {
		return false
	}
	if strings.HasPrefix(r, "what command") || strings.HasPrefix(r, "which command") ||
		strings.Contains(r, "shell command") || strings.Contains(r, "command to ") ||
		strings.Contains(r, "how do i ") || strings.Contains(r, "how can i ") ||
		strings.Contains(r, "show me the command") || strings.Contains(r, "give me the command") ||
		strings.Contains(r, " in bash") || strings.Contains(r, " in zsh") || strings.Contains(r, " in terminal") {
		return false
	}
	for _, hint := range []string{"command", "shell", "terminal", "bash", "zsh", "powershell", "script", "one-liner", "run ", "install ", "setup ", "how to "} {
		if strings.Contains(r, hint) {
			return false
		}
	}
	if !strings.Contains(r, "?") && !strings.HasPrefix(r, "tell me ") && !strings.HasPrefix(r, "find out ") && !strings.HasPrefix(r, "explain ") && !strings.HasPrefix(r, "summarize ") {
		return false
	}
	for _, prefix := range []string{
		"who ", "what ", "when ", "where ", "which ", "why ", "how many ", "how much ", "how old ",
		"is ", "are ", "was ", "were ", "do ", "does ", "did ", "can ", "could ", "should ", "will ", "would ",
		"tell me ", "find out ", "explain ", "summarize ",
	} {
		if strings.HasPrefix(r, prefix) {
			return true
		}
	}
	return false
}

func SystemPrompt(p Prompt) string {
	var b strings.Builder
	b.WriteString("You translate a request into one shell command.\n")
	fmt.Fprintf(&b, "Target shell: %s. Operating system: %s.\n", p.Shell, p.OS)
	if p.ProjectType != "" {
		fmt.Fprintf(&b, "The current folder looks like a %s project.\n", p.ProjectType)
	}
	if p.InGit {
		b.WriteString("The current folder is inside a git repository.\n")
	}
	b.WriteString("Rules:\n")
	b.WriteString("- Reply with ONLY the command, on a single line. No explanation, no markdown, no backticks.\n")
	b.WriteString("- Prefer common, standard tools. Chain steps with && when needed.\n")
	b.WriteString("- Do not use sudo unless the request needs it.\n")
	b.WriteString("- If the request cannot be done as one command line, reply with: # cannot: <short reason>\n")
	return b.String()
}

var fenceRe = regexp.MustCompile("(?s)```[a-zA-Z]*\\n?(.*?)```")

func Clean(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if m := fenceRe.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}
	s = strings.TrimSpace(strings.Trim(s, "`"))

	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	switch len(lines) {
	case 0:
		return "", errors.New("the model returned an empty answer")
	case 1:
	default:
		return "", errors.New("the model returned several lines instead of one command; try rephrasing")
	}

	cmd := strings.TrimPrefix(lines[0], "$ ")
	if strings.HasPrefix(cmd, "#") {
		msg := strings.TrimSpace(strings.TrimPrefix(cmd, "#"))
		msg = strings.TrimSpace(strings.TrimPrefix(msg, "cannot:"))
		return "", fmt.Errorf("the model declined: %s", msg)
	}
	return cmd, nil
}

func Ask(ctx context.Context, gens []Generator, p Prompt) (cmd, used string, err error) {
	return AskWithTimeout(ctx, gens, p, 0)
}

// AskWithTimeout gives each generator its own attempt timeout, allowing later
// fallbacks to run when an earlier provider is slow.
func AskWithTimeout(ctx context.Context, gens []Generator, p Prompt, timeout time.Duration) (cmd, used string, err error) {
	var failures []string
	for _, g := range gens {
		attemptCtx := ctx
		cancel := func() {}
		if timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		raw, e := g.Generate(attemptCtx, p)
		cancel()
		if e == nil {
			var c string
			if c, e = Clean(raw); e == nil {
				return c, g.Name(), nil
			}
		}
		if errors.Is(e, ErrOffline) {
			return "", "", ErrOffline
		}
		failures = append(failures, g.Name()+": "+e.Error())
		if ctx.Err() != nil {
			break
		}
	}
	return "", "", errors.New(strings.Join(failures, "; "))
}

func ResearchWithTimeout(ctx context.Context, gens []Generator, p Prompt, timeout time.Duration) (answer, used string, err error) {
	var failures []string
	for _, g := range gens {
		researcher, ok := g.(ResearchGenerator)
		if !ok {
			continue
		}
		attemptCtx := ctx
		cancel := func() {}
		if timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		text, attemptErr := researcher.Research(attemptCtx, p)
		cancel()
		if attemptErr == nil && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), g.Name(), nil
		}
		if attemptErr == nil {
			attemptErr = errors.New("empty research answer")
		}
		if errors.Is(attemptErr, ErrOffline) {
			return "", "", ErrOffline
		}
		failures = append(failures, g.Name()+": "+attemptErr.Error())
		if ctx.Err() != nil {
			break
		}
	}
	if len(failures) == 0 {
		return "", "", errors.New("web research is currently available through Gemini; select --backend=gemini and configure its API key")
	}
	return "", "", errors.New(strings.Join(failures, "; "))
}
