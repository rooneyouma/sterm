package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/rooneyouma/sterm/internal/model"
)

// newTestStore opens a fresh database in a temp folder that Go deletes afterwards.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seed inserts commands, each one second apart so they have distinct times.
func seed(t *testing.T, s *Store, cmds ...string) {
	t.Helper()
	base := time.Unix(1_700_000_000, 0)
	for i, c := range cmds {
		err := s.Insert(model.Command{Cmd: c, Time: base.Add(time.Duration(i) * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPrefixMatches(t *testing.T) {
	s := newTestStore(t)
	seed(t, s,
		"git status",
		"git status",
		"git status", // ran 3 times
		"git checkout main",
		"git checkout main", // ran 2 times
		"git commit -m wip",
		"ls -la",
		"Git Shout", // different case: must NOT match "git"
	)

	tests := []struct {
		name   string
		prefix string
		want   []string // expected commands in order
	}{
		{"most used first", "git ", []string{"git status", "git checkout main", "git commit -m wip"}},
		{"narrower prefix", "git ch", []string{"git checkout main"}},
		{"exact match excluded", "git status", nil},
		{"no match", "zzz", nil},
		{"empty prefix", "", nil},
		{"case sensitive", "git S", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.PrefixMatches(tt.prefix, "", "", 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d matches, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, m := range got {
				if m.Cmd != tt.want[i] {
					t.Errorf("match %d = %q, want %q", i, m.Cmd, tt.want[i])
				}
			}
		})
	}
}

func TestPrefixMatchesWildcardsAreLiteral(t *testing.T) {
	s := newTestStore(t)
	seed(t, s, "ls _x", "ls abc")

	got, err := s.PrefixMatches("ls _", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Cmd != "ls _x" {
		t.Errorf("underscore should match literally, got %+v", got)
	}
}

func TestTrailingWhitespaceIsTrimmed(t *testing.T) {
	s := newTestStore(t)
	seed(t, s, "git status ", "git status", "git status\t")

	got, err := s.PrefixMatches("git ", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Cmd != "git status" || got[0].Count != 3 {
		t.Errorf("want one 'git status' x3, got %+v", got)
	}
}

func TestPrefixMatchesStats(t *testing.T) {
	s := newTestStore(t)
	base := time.Unix(1_700_000_000, 0)
	rows := []model.Command{
		{Cmd: "make test", Cwd: "/a", Repo: "/a", ExitCode: 0, Time: base},
		{Cmd: "make test", Cwd: "/a", Repo: "/a", ExitCode: 2, Time: base.Add(1 * time.Second)},
		{Cmd: "make test", Cwd: "/b", Repo: "/b", ExitCode: 0, Time: base.Add(2 * time.Second)},
	}
	for _, c := range rows {
		if err := s.Insert(c); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.PrefixMatches("make ", "/a", "/a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	m := got[0]
	if m.Count != 3 || m.DirCount != 2 || m.RepoCount != 2 || m.Failures != 1 {
		t.Errorf("stats wrong: %+v", m)
	}

	got, err = s.PrefixMatches("make ", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].DirCount != 0 || got[0].RepoCount != 0 {
		t.Errorf("empty context should give zero boosts, got %+v", got[0])
	}
}

func TestAskCache(t *testing.T) {
	s := newTestStore(t)

	if _, ok, err := s.GetAsk("k"); err != nil || ok {
		t.Fatalf("empty cache: ok=%v err=%v", ok, err)
	}
	if err := s.PutAsk("k", "ls -la"); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := s.GetAsk("k"); err != nil || !ok || got != "ls -la" {
		t.Errorf("GetAsk = %q, %v, %v", got, ok, err)
	}
	if err := s.PutAsk("k", "pwd"); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.GetAsk("k"); got != "pwd" {
		t.Errorf("Put should replace the old answer, got %q", got)
	}
}
