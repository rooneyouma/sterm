package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMissingFileGivesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend != "ollama" || cfg.TimeoutSeconds != 30 || len(cfg.Fallbacks) != 2 || cfg.Fallbacks[0] != "gemini" || cfg.Fallbacks[1] != "openrouter" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	for _, name := range []string{"anthropic", "openai", "gemini", "openrouter"} {
		p, ok := cfg.Cloud(name)
		if !ok || p.Model == "" || p.APIKeyEnv == "" {
			t.Errorf("%s should have a default model and key variable: %+v", name, p)
		}
	}
	if cfg.Gemini.Model != "gemini-3.5-flash-lite" {
		t.Errorf("unexpected Gemini primary model: %q", cfg.Gemini.Model)
	}
	if len(cfg.Gemini.FallbackModels) != 1 || cfg.Gemini.FallbackModels[0] != "gemini-3.1-flash-lite" {
		t.Errorf("unexpected Gemini fallback models: %v", cfg.Gemini.FallbackModels)
	}
	if cfg.OpenRouter.Model != "openrouter/free" {
		t.Errorf("unexpected OpenRouter default model: %q", cfg.OpenRouter.Model)
	}
}

func TestOverridesKeepOtherDefaults(t *testing.T) {
	cfg, err := Load(write(t, `
backend = "openai"
fallbacks = ["gemini", "anthropic"]

[openai]
model = "my-model"

[gemini]
api_key_env = "MY_GEMINI_KEY"
fallback_models = ["gemini-stable-a", "gemini-stable-b"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend != "openai" || len(cfg.Fallbacks) != 2 || cfg.Fallbacks[0] != "gemini" {
		t.Errorf("order not applied: %+v", cfg)
	}
	if cfg.OpenAI.Model != "my-model" || cfg.OpenAI.APIKeyEnv != "OPENAI_API_KEY" {
		t.Errorf("openai: %+v", cfg.OpenAI)
	}
	if cfg.Gemini.APIKeyEnv != "MY_GEMINI_KEY" || cfg.Gemini.Model == "" {
		t.Errorf("gemini: %+v", cfg.Gemini)
	}
	if len(cfg.Gemini.FallbackModels) != 2 || cfg.Gemini.FallbackModels[1] != "gemini-stable-b" {
		t.Errorf("Gemini fallback models: %v", cfg.Gemini.FallbackModels)
	}
}

func TestInvalidConfigs(t *testing.T) {
	for name, content := range map[string]string{
		"bad backend":  `backend = "banana"`,
		"bad fallback": `fallbacks = ["openai", "banana"]`,
		"bad timeout":  `timeout_seconds = -5`,
		"broken toml":  `backend = `,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(write(t, content)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
