package model

import "time"

// Match is one distinct command found by a search, with usage stats.
type Match struct {
	Cmd   string
	Count int
	Last  time.Time
}
