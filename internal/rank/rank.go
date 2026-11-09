package rank

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/rooneyouma/sterm/internal/model"
)

// Weights controls how much each signal matters.
type Weights struct {
	Freq         float64
	Recency      float64
	HalfLifeDays float64
	Dir          float64
	Repo         float64
	Fail         float64
}

var DefaultWeights = Weights{
	Freq:         1.0,
	Recency:      1.5,
	HalfLifeDays: 14,
	Dir:          1.5,
	Repo:         1.0,
	Fail:         3.0,
}

// Score rates one match. Higher is better.
func Score(m model.Match, now time.Time, w Weights) float64 {
	age := now.Sub(m.Last)
	if age < 0 {
		age = 0
	}
	days := age.Hours() / 24

	s := w.Freq * math.Log1p(float64(m.Count))
	s += w.Recency * math.Pow(0.5, days/w.HalfLifeDays)
	s += w.Dir * math.Log1p(float64(m.DirCount))
	s += w.Repo * math.Log1p(float64(m.RepoCount))
	if m.Count > 0 {
		s -= w.Fail * float64(m.Failures) / float64(m.Count)
	}
	return s
}

// Rank sorts matches in place, best first.
func Rank(matches []model.Match, now time.Time, w Weights) {
	type scored struct {
		m model.Match
		s float64
	}
	items := make([]scored, len(matches))
	for i, m := range matches {
		items[i] = scored{m, Score(m, now, w)}
	}

	slices.SortStableFunc(items, func(a, b scored) int {
		if c := cmp.Compare(b.s, a.s); c != 0 {
			return c
		}
		if c := b.m.Last.Compare(a.m.Last); c != 0 {
			return c
		}
		return strings.Compare(a.m.Cmd, b.m.Cmd)
	})

	for i := range items {
		matches[i] = items[i].m
	}
}
