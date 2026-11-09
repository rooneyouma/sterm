package index

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rooneyouma/sterm/internal/model"
	"github.com/rooneyouma/sterm/internal/rank"
	"github.com/rooneyouma/sterm/internal/store"
)

var base = time.Unix(1_700_000_000, 0)

func mk(cmd, cwd, repo string, exit, sec int) model.Command {
	return model.Command{
		Cmd:      cmd,
		Cwd:      cwd,
		Repo:     repo,
		ExitCode: exit,
		Time:     base.Add(time.Duration(sec) * time.Second),
	}
}

func TestAddAndMatches(t *testing.T) {
	ix := New()
	for _, c := range []model.Command{
		mk("make test", "/a", "/a", 0, 1),
		mk("make test", "/a", "/a", 2, 2),
		mk("make test", "/b", "/b", 0, 3),
		mk("make build ", "", "", 0, 4),
		mk("Make Test", "", "", 0, 5),
	} {
		ix.Add(c)
	}

	got := ix.Matches("make ", "/a", "/a")
	if len(got) != 2 {
		t.Fatalf("got %d matches, want 2: %+v", len(got), got)
	}
	byCmd := map[string]model.Match{}
	for _, m := range got {
		byCmd[m.Cmd] = m
	}

	m := byCmd["make test"]
	if m.Count != 3 || m.DirCount != 2 || m.RepoCount != 2 || m.Failures != 1 {
		t.Errorf("stats wrong: %+v", m)
	}
	if !m.Last.Equal(base.Add(3 * time.Second)) {
		t.Errorf("last = %v, want newest run", m.Last)
	}
	if _, ok := byCmd["make build"]; !ok {
		t.Errorf("trailing space was not trimmed: %+v", got)
	}

	if m := ix.Matches("make ", "", ""); m[0].DirCount != 0 || m[0].RepoCount != 0 {
		t.Errorf("no context should mean no boosts: %+v", m[0])
	}
	if got := ix.Matches("make test", "", ""); len(got) != 0 {
		t.Errorf("exact prefix should be excluded: %+v", got)
	}
	if got := ix.Matches("", "", ""); got != nil {
		t.Errorf("empty prefix should match nothing: %+v", got)
	}
	if got := ix.Matches("make T", "", ""); len(got) != 0 {
		t.Errorf("matching should be case-sensitive: %+v", got)
	}
}

func TestWildcardsAreLiteral(t *testing.T) {
	ix := New()
	ix.Add(mk("ls _x", "", "", 0, 1))
	ix.Add(mk("ls abc", "", "", 0, 2))
	if got := ix.Matches("ls _", "", ""); len(got) != 1 || got[0].Cmd != "ls _x" {
		t.Errorf("underscore should match literally: %+v", got)
	}
}

func TestParityWithStore(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for _, c := range []model.Command{
		mk("git status", "/w/api", "/w/api", 0, 1),
		mk("git status", "/w/api", "/w/api", 0, 2),
		mk("git status", "/w/web", "/w/web", 0, 3),
		mk("git push", "/w/api", "/w/api", 1, 4),
		mk("git push", "/w/api", "/w/api", 1, 5),
		mk("git checkout main", "/w/web", "/w/web", 0, 6),
		mk("go test ./...", "/w/api", "/w/api", 0, 7),
		mk("go test ./...", "/w/api", "/w/api", 2, 8),
		mk("go build", "", "", 0, 9),
		mk("ls -la", "/tmp", "", 0, 10),
	} {
		if err := s.Insert(c); err != nil {
			t.Fatal(err)
		}
	}

	ix := New()
	all, err := s.Since(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range all {
		ix.Add(c)
	}

	now := base.Add(48 * time.Hour)
	prefixes := []string{"g", "git ", "git p", "go ", "l", "zzz", "git status"}
	contexts := [][2]string{{"", ""}, {"/w/api", "/w/api"}, {"/w/web", "/w/web"}, {"/tmp", ""}}

	for _, p := range prefixes {
		for _, c := range contexts {
			fromStore, err := s.PrefixMatches(p, c[0], c[1], 200)
			if err != nil {
				t.Fatal(err)
			}
			fromIndex := ix.Matches(p, c[0], c[1])
			rank.Rank(fromStore, now, rank.DefaultWeights)
			rank.Rank(fromIndex, now, rank.DefaultWeights)

			if len(fromStore) != len(fromIndex) {
				t.Errorf("prefix %q ctx %v: store gave %d, index gave %d", p, c, len(fromStore), len(fromIndex))
				continue
			}
			for i := range fromStore {
				a, b := fromStore[i], fromIndex[i]
				if a.Cmd != b.Cmd || a.Count != b.Count || a.DirCount != b.DirCount ||
					a.RepoCount != b.RepoCount || a.Failures != b.Failures || !a.Last.Equal(b.Last) {
					t.Errorf("prefix %q ctx %v: store %+v, index %+v", p, c, a, b)
				}
			}
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	ix := New()
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				ix.Add(model.Command{Cmd: fmt.Sprintf("git %d", i%20), Time: time.Unix(int64(i), 0)})
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				ix.Matches("git ", "", "")
			}
		}()
	}
	wg.Wait()
	if ix.Len() != 20 {
		t.Errorf("Len = %d, want 20", ix.Len())
	}
}

func BenchmarkMatchesAndRank(b *testing.B) {
	ix := New()
	for i := 0; i < 20_000; i++ {
		ix.Add(model.Command{
			Cmd:  fmt.Sprintf("tool%d --flag %d", i%500, i),
			Cwd:  "/w",
			Time: base.Add(time.Duration(i) * time.Second),
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ms := ix.Matches("tool1", "/w", "")
		rank.Rank(ms, base, rank.DefaultWeights)
	}
}
