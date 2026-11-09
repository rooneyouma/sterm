package index

import (
	"strings"
	"sync"
	"time"

	"github.com/rooneyouma/sterm/internal/model"
)

// entry is everything we know about one distinct command.
type entry struct {
	count    int
	last     time.Time
	failures int
	dirs     map[string]int
	repos    map[string]int
}

// Index holds the whole history in memory, grouped by command.
// It is safe for concurrent use: many queries can read at once, writes are exclusive.
type Index struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

func New() *Index {
	return &Index{entries: make(map[string]*entry)}
}

// Len returns the number of distinct commands.
func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.entries)
}

// Add records one run of a command.
func (ix *Index) Add(c model.Command) {
	cmd := strings.TrimRight(c.Cmd, " \t\r\n")
	if cmd == "" {
		return
	}

	ix.mu.Lock()
	defer ix.mu.Unlock()

	e := ix.entries[cmd]
	if e == nil {
		e = &entry{}
		ix.entries[cmd] = e
	}
	e.count++
	if c.Time.After(e.last) {
		e.last = c.Time
	}
	if c.ExitCode != 0 {
		e.failures++
	}
	if c.Cwd != "" {
		if e.dirs == nil {
			e.dirs = make(map[string]int)
		}
		e.dirs[c.Cwd]++
	}
	if c.Repo != "" {
		if e.repos == nil {
			e.repos = make(map[string]int)
		}
		e.repos[c.Repo]++
	}
}

// Matches returns every command that starts with prefix (excluding prefix itself),
// with stats measured against cwd and repo. The caller ranks them.
func (ix *Index) Matches(prefix, cwd, repo string) []model.Match {
	if prefix == "" {
		return nil
	}

	ix.mu.RLock()
	defer ix.mu.RUnlock()

	var out []model.Match
	for cmd, e := range ix.entries {
		if len(cmd) <= len(prefix) || !strings.HasPrefix(cmd, prefix) {
			continue
		}
		m := model.Match{Cmd: cmd, Count: e.count, Last: e.last, Failures: e.failures}
		if cwd != "" {
			m.DirCount = e.dirs[cwd]
		}
		if repo != "" {
			m.RepoCount = e.repos[repo]
		}
		out = append(out, m)
	}
	return out
}
