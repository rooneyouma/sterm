package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/pelletier/go-toml/v2"
)

var Backends = []string{"anthropic", "openai", "gemini", "openrouter", "ollama"}

type Provider struct {
	Model          string   `toml:"model"`
	FallbackModels []string `toml:"fallback_models"`
	APIKeyEnv      string   `toml:"api_key_env"`
}

type Ollama struct {
	URL   string `toml:"url"`
	Model string `toml:"model"`
}

type Config struct {
	Backend        string   `toml:"backend"`
	Fallbacks      []string `toml:"fallbacks"`
	TimeoutSeconds int      `toml:"timeout_seconds"`

	Anthropic  Provider `toml:"anthropic"`
	OpenAI     Provider `toml:"openai"`
	Gemini     Provider `toml:"gemini"`
	OpenRouter Provider `toml:"openrouter"`
	Ollama     Ollama   `toml:"ollama"`
}

func Default() Config {
	return Config{
		Backend:        "ollama",
		Fallbacks:      []string{"gemini", "openrouter"},
		TimeoutSeconds: 30,
		Anthropic:      Provider{Model: "claude-haiku-5-5", APIKeyEnv: "ANTHROPIC_API_KEY"},
		OpenAI:         Provider{Model: "gpt-4o-mini", APIKeyEnv: "OPENAI_API_KEY"},
		Gemini: Provider{
			Model:          "gemini-3.5-flash-lite",
			FallbackModels: []string{"gemini-3.1-flash-lite"},
			APIKeyEnv:      "GEMINI_API_KEY",
		},
		OpenRouter: Provider{Model: "openrouter/free", APIKeyEnv: "OPENROUTER_API_KEY"},
		Ollama:     Ollama{URL: "http://localhost:11434", Model: "qwen2.5-coder:1.5b"},
	}
}

func (c Config) Cloud(name string) (Provider, bool) {
	switch name {
	case "anthropic":
		return c.Anthropic, true
	case "openai":
		return c.OpenAI, true
	case "gemini":
		return c.Gemini, true
	case "openrouter":
		return c.OpenRouter, true
	}
	return Provider{}, false
}

func (c Config) Timeout() time.Duration {
	return time.Duration(c.TimeoutSeconds) * time.Second
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sterm", "config.toml"), nil
}

func fill(p *Provider, d Provider) {
	if p.Model == "" {
		p.Model = d.Model
	}
	if p.APIKeyEnv == "" {
		p.APIKeyEnv = d.APIKeyEnv
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}

	d := Default()
	if cfg.Backend == "" {
		cfg.Backend = d.Backend
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = d.TimeoutSeconds
	}
	fill(&cfg.Anthropic, d.Anthropic)
	fill(&cfg.OpenAI, d.OpenAI)
	fill(&cfg.Gemini, d.Gemini)
	fill(&cfg.OpenRouter, d.OpenRouter)
	if cfg.Ollama.URL == "" {
		cfg.Ollama.URL = d.Ollama.URL
	}
	if cfg.Ollama.Model == "" {
		cfg.Ollama.Model = d.Ollama.Model
	}

	if err := cfg.validate(); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func (c Config) validate() error {
	if !slices.Contains(Backends, c.Backend) {
		return fmt.Errorf("backend must be one of %v, got %q", Backends, c.Backend)
	}
	for _, f := range c.Fallbacks {
		if !slices.Contains(Backends, f) {
			return fmt.Errorf("fallbacks must each be one of %v, got %q", Backends, f)
		}
	}
	if c.TimeoutSeconds < 1 {
		return errors.New("timeout_seconds must be at least 1")
	}
	return nil
}
