# sterm

`sterm` learns from your shell history and suggests matching commands as you type. It also turns a natural-language request into a command for you to review before running.

## Install

Requirements: Go 1.26 or newer and zsh for the shell integration.

From the project directory:

```sh
go install ./cmd/sterm
```

Make sure Go's binary directory is on your `PATH`:

```sh
export PATH="$HOME/go/bin:$PATH"
```

Add this line to `~/.zshrc`, adjusting the path if you cloned the project elsewhere:

```sh
source "$HOME/projects/sterm/shell/sterm.zsh"
```

Restart zsh or run `source ~/.zshrc`.

## Use

- Type part of a command to see a history-based suggestion in dim text. Press **Right Arrow** or **Ctrl-E** to accept it.
- Press **Ctrl-G**, or enter a line beginning with `# ` or `## `, to ask for a command. For example:

  ```text
  ## create a new Next.js project
  ```

  Review the generated command in the shell buffer, edit if needed, then press **Enter** to run it.
- Use `sterm recent` to see recent recorded commands.
- Use `sterm import --format zsh` or `sterm import --format bash` to import shell history.

## Configure AI

AI-generated commands require a configured provider. The default configuration uses Ollama first, then Gemini and OpenRouter as fallbacks. Edit `~/.config/sterm/config.toml` to choose a provider and models. For Gemini, set `GEMINI_API_KEY` in your shell and configure, for example:

```toml
backend = "gemini"
fallbacks = []

[gemini]
model = "gemini-3.5-flash-lite"
fallback_models = ["gemini-3.1-flash-lite"]
api_key_env = "GEMINI_API_KEY"
```

Model access and free-tier limits depend on your Google AI Studio account. Other providers need their corresponding API key environment variable. Use `sterm ask --list` to see configured providers and missing keys.

## How it works

The zsh integration records commands after they run, along with their working directory and exit status. It filters private commands before storing them in a local SQLite database. Suggestions are ranked using command history and context such as directory, repository, frequency, and success.

A background daemon keeps a query index for fast suggestions. Natural-language requests are sent to the configured model; `sterm` places the result in the shell buffer for review and does not execute it automatically.
