package rank

import (
	"fmt"
	"testing"
	"time"

	"github.com/rooneyouma/sterm/internal/model"
)

var now = time.Unix(1_700_000_000, 0)

func order(ms []model.Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Cmd
	}
	return out
}

func TestRank(t *testing.T) {
	longAgo := now.Add(-90 * 24 * time.Hour)

	tests := []struct {
		name string
		in   []model.Match
		want []string
	}{
		{
			name: "more frequent wins",
			in: []model.Match{
				{Cmd: "rare", Count: 2, Last: now},
				{Cmd: "common", Count: 10, Last: now},
			},
			want: []string{"common", "rare"},
		},
		{
			name: "more recent wins",
			in: []model.Match{
				{Cmd: "stale", Count: 3, Last: longAgo},
				{Cmd: "fresh", Count: 3, Last: now},
			},
			want: []string{"fresh", "stale"},
		},
		{
			name: "used in this directory wins",
			in: []model.Match{
				{Cmd: "elsewhere", Count: 3, Last: now},
				{Cmd: "here", Count: 2, Last: now, DirCount: 2},
			},
			want: []string{"here", "elsewhere"},
		},
		{
			name: "used in this repo wins",
			in: []model.Match{
				{Cmd: "other-repo", Count: 3, Last: now},
				{Cmd: "this-repo", Count: 2, Last: now, RepoCount: 2},
			},
			want: []string{"this-repo", "other-repo"},
		},
		{
			name: "commands that always fail lose",
			in: []model.Match{
				{Cmd: "flaky", Count: 5, Failures: 5, Last: now},
				{Cmd: "solid", Count: 5, Last: now},
			},
			want: []string{"solid", "flaky"},
		},
		{
			name: "failing often beats being popular",
			in: []model.Match{
				{Cmd: "popular-but-broken", Count: 10, Failures: 10, Last: now},
				{Cmd: "modest-but-works", Count: 3, Last: now},
			},
			want: []string{"modest-but-works", "popular-but-broken"},
		},
		{
			name: "ties are alphabetical",
			in: []model.Match{
				{Cmd: "b", Count: 1, Last: now},
				{Cmd: "a", Count: 1, Last: now},
			},
			want: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Rank(tt.in, now, DefaultWeights)
			got := order(tt.in)
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("order = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func BenchmarkRank200(b *testing.B) {
	base := make([]model.Match, 200)
	for i := range base {
		base[i] = model.Match{
			Cmd:       fmt.Sprintf("command %d", i),
			Count:     i%17 + 1,
			Last:      now.Add(-time.Duration(i) * time.Hour),
			DirCount:  i % 3,
			RepoCount: i % 5,
			Failures:  i % 2,
		}
	}
	work := make([]model.Match, len(base))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(work, base)
		Rank(work, now, DefaultWeights)
	}
}
