package model 

import "time"


type Command struct {
	ID       int64
	Cmd      string
	Cwd      string
	Repo     string
	ExitCode int
	Time     time.Time
}

