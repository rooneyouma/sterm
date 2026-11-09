package model

import "time"

// Match is one distinct command found by a search, with usage stats.
type Match struct {
	Cmd       string
	Count     int       // times it was run in total
	Last      time.Time // the most recent run
	DirCount  int       // times it was run in the current directory
	RepoCount int       // times it was run in the current git repo
	Failures  int       // times it exited with a non-zero code
}
