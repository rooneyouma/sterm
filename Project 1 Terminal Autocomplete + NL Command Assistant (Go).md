# Project 1: Terminal Autocomplete + NL Command Assistant (Go)

**Type:** Side project, week 1\
**Language:** Go\
**Target shell:** zsh (macOS/Linux), with fish and bash as later options\
**Status:** Planning

## 1. Overview

A fast, local-first command-line tool that watches your shell history and shows a faint "ghost" suggestion as you type, accepted with the right arrow. It also lets you describe what you want in plain English (for example `# create a next project`) and replaces the buffer with the generated command, ready for you to review and run.

The project doubles as a way to learn Go: structs, interfaces, `database/sql`, testing, benchmarks, goroutines, channels, and Unix sockets.

## 2. Goals

- Ghost-text suggestions that feel instant (under 20ms per keystroke).
- Smart ranking using frequency, recency, directory, git repo, and exit code.
- Natural-language to command via a swappable LLM backend (local Ollama first, cloud optional).
- Single static binary, no cgo, easy to install and cross-compile.
- Privacy by default: nothing leaves the machine unless a cloud backend is explicitly configured.

## 3. Non-goals (for now)

- Windows/PowerShell support.
- Cross-machine sync.
- Auto-executing generated commands (never).
- A GUI.

## 4. Architecture

### 4.1 Components

1. **Shell integration (`shell/suggest.zsh`)**: `preexec`/`precmd` hooks capture commands. A ZLE widget renders ghost text via `POSTDISPLAY` and `region_highlight`, and binds the right arrow and End to accept it.
2. **CLI (`cmd/suggest`)**: subcommands `record`, `query`, `import`, `ask`, `stats`, `explain`.
3. **Store (`internal/store`)**: SQLite in WAL mode via `modernc.org/sqlite` (pure Go).
4. **Ranker (`internal/rank`)**: pure scoring functions, easy to unit test and benchmark.
5. **Generator (`internal/llm`)**: an interface with Ollama and cloud implementations.
6. **Daemon (`internal/daemon`)**: long-running process holding an in-memory index, reached over a Unix socket (stage 2).

### 4.2 Data flow

- **Record:** shell hook calls `suggest record` in the background (`&!`) with command, cwd, exit code, timestamp, and repo.
- **Suggest:** on each keystroke the widget asks `suggest query --buffer ... --cwd ...`; the ranker returns the best prefix match.
- **Ask:** a hotkey or `#` prefix sends the description plus minimal context to `suggest ask`; history is searched first, the LLM is called only on a miss.

### 4.3 Data model

```sql
CREATE TABLE history (
  id        INTEGER PRIMARY KEY,
  cmd       TEXT NOT NULL,
  cwd       TEXT,
  repo      TEXT,
  exit_code INTEGER,
  ts        INTEGER NOT NULL
);
CREATE INDEX idx_history_cmd ON history(cmd);
CREATE INDEX idx_history_ts  ON history(ts);

CREATE TABLE ask_cache (
  key      TEXT PRIMARY KEY,   -- hash of prompt + context
  command  TEXT NOT NULL,
  ts       INTEGER NOT NULL
);

-- later
CREATE TABLE transitions (
  prev_cmd TEXT, next_cmd TEXT, count INTEGER,
  PRIMARY KEY (prev_cmd, next_cmd)
);
```

## 5. Ranking (v1)

Score each prefix match as:

`score = freq_weight * log(1 + count) + recency_decay(ts) + same_dir_boost + same_repo_boost - failed_penalty`

Tunable weights live in config. Start simple, then tune against your own history using a small table-driven test set.

## 6. Natural-language mode

### 6.1 Triggers

- `suggest ask "create a next project"` prints the command.
- Hotkey (Ctrl-G): replaces the buffer with the generated command.
- `#` prefix: a line starting with `#` is intercepted and replaced with the command (stage 2).

### 6.2 Backend interface

```go
type Generator interface {
    Generate(ctx context.Context, p Prompt) (string, error)
}
```

- **Ollama (default):** HTTP POST to `localhost:11434/api/chat`, model such as `qwen2.5-coder` or `llama3.2`.
- **Cloud (optional):** API key read from an environment variable, never stored in the DB.
- Config in `~/.config/suggest/config.toml` sets the backend and fallback order.

### 6.3 Behaviour rules

- Search history first; call the LLM only on a miss.
- Send only minimal context: OS, shell, cwd, git repo flag, detected project type, last few commands.
- System prompt demands the command only; strip markdown fences in Go regardless.
- `context.WithTimeout` on every call so the shell never hangs.
- Cache answers by prompt plus context.

## 7. Safety and privacy

- Never auto-execute generated commands; always place them in the buffer.
- Warn on dangerous patterns: `rm -rf`, `sudo`, `dd`, `curl | sh`, `chmod -R`.
- Skip recording commands that start with a space.
- Skip commands matching `password|token|secret|AWS_` and similar patterns.
- Never send full history to a cloud model; apply the same secret filter to any context sent.

## 8. Performance budget

- Query latency target: under 20ms end to end.
- Stage 1 (process per query): acceptable for MVP at roughly 5-15ms.
- Stage 2 (daemon with in-memory index over a Unix socket): target under 5ms.
- Enforce with `go test -bench` on the ranker and a simple latency check in CI.

## 9. Repo layout

```
suggest/
├── cmd/suggest/main.go
├── internal/
│   ├── store/
│   ├── rank/
│   ├── model/
│   ├── llm/
│   └── daemon/
├── shell/suggest.zsh
├── testdata/
├── go.mod
└── README.md
```

## 10. Milestones

### M1: Record and import

- [ ] `go mod init`, repo scaffolding
- [ ] SQLite schema and migrations
- [ ] `suggest record`
- [ ] `suggest import` for `~/.zsh_history`

* **Go concepts:** structs, packages, `database/sql`, error handling

### M2: Query by prefix

- [ ] `suggest query --buffer ... --cwd ...`
- [ ] Table-driven tests with a seeded test DB

* **Go concepts:** testing, `testdata`, test helpers

### M3: Ranking

- [ ] Frecency scoring in `internal/rank`
- [ ] Directory/repo boost, failure penalty
- [ ] Benchmarks against the 20ms budget

* **Go concepts:** benchmarks, pure functions, small interfaces

### M4: zsh widget

- [ ] Hooks (`preexec`, `precmd`) with background recording
- [ ] Ghost text via `POSTDISPLAY` and `region_highlight`
- [ ] Accept with right arrow / End

### M5: Daemon

- [ ] Unix socket server with in-memory index
- [ ] Graceful startup/shutdown, auto-start from shell

* **Go concepts:** goroutines, channels, `sync.RWMutex`, `net`, `context`

### M6: Natural-language mode

- [ ] `Generator` interface and Ollama client
- [ ] `suggest ask` plus Ctrl-G widget
- [ ] History-first lookup, caching, timeouts
- [ ] Dangerous-command warnings

* **Go concepts:** interfaces, `net/http`, `encoding/json`, timeouts

## 11. Stretch ideas

- Next-command prediction from a bigram table (what usually follows your last command).
- Learn from accepted vs ignored suggestions.
- Flag and subcommand completion from `--help` or zsh completion specs.
- Git-branch and path awareness.
- Fuzzy popup picker (Ctrl-R replacement) with Bubble Tea.
- `suggest explain` and "fix my last error" (thefuck-style).
- `suggest stats` and alias-candidate report.
- Streaming LLM output via channels.
- Encrypted multi-machine sync.

## 12. Tech stack and references

- Go 1.22+, `modernc.org/sqlite`, `cobra` (or stdlib `flag` first), `pelletier/go-toml`, Bubble Tea (stretch)
- Ollama for local models
- Reference projects: zsh-autosuggestions, atuin, fzf, thefuck

## 13. Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Per-keystroke latency | Benchmarks early; move to daemon in M5 |
| ZLE widget quirks | Prototype widget with a hardcoded suggestion first |
| Secrets stored in history | Filter on record; skip space-prefixed commands |
| Bad or unsafe LLM output | Never auto-run; pattern warnings; review in buffer |
| Slow or missing Ollama | Timeouts, history-first, clear error message |
| Scope creep | Ship M1-M4 as v0.1 before touching the daemon or LLM |

## 14. Definition of done

**v0.1 (MVP):** history recorded and imported, prefix suggestions with ranking, ghost text accepted with the right arrow, tests passing, README with install steps.

**v0.2:** daemon, sub-5ms suggestions.

**v0.3:** natural-language mode with Ollama, history-first lookup, safety warnings.

## 15. Suggested week plan

- **Day 1:** repo setup, schema, `record`, `import` (M1)
- **Day 2:** `query` and tests (M2)
- **Day 3:** ranking and benchmarks (M3)
- **Day 4:** zsh widget and ghost text (M4)
- **Day 5:** polish, README, tag v0.1
- **Weekend:** daemon (M5) or natural-language mode (M6), whichever is more fun

## 16. Repo name ideas

- **ghostline**: the ghost-text suggestion, short and memorable
- **nudge**: gentle suggestions as you type
- **hindsight**: it learns from your history
- **gosuggest**: plain and descriptive, signals Go
- **shellmate**: a companion for your shell
- **tabula**: a nod to tab completion and a blank slate
- **promptpal**: covers both autocomplete and natural-language prompts

## 17. Open questions

- Exact OS and zsh version for hook testing?
- Ollama only, or also a cloud backend at launch?
- Install method: `go install`, Homebrew tap, or release binaries?
