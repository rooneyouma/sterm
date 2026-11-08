package importer

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/rooneyouma/sterm/internal/model"
)

// ParseZsh reads a zsh history file. Commands without a timestamp get
// made-up ones, one second apart, ending just before `fallback`.
func ParseZsh(r io.Reader, fallback time.Time) ([]model.Command, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024) // allow very long lines

	var out []model.Command
	var pending strings.Builder
	var ts time.Time
	inMulti := false

	for sc.Scan() {
		line := sc.Text()

		if !inMulti {
			ts = time.Time{} // zero means "no timestamp"
			if t, rest, ok := parseExtended(line); ok {
				ts, line = t, rest
			}
		}

		// A trailing backslash means the command continues on the next line.
		if strings.HasSuffix(line, `\`) {
			pending.WriteString(strings.TrimSuffix(line, `\`))
			pending.WriteString("\n")
			inMulti = true
			continue
		}

		pending.WriteString(line)
		cmd := strings.TrimRight(pending.String(), " \t\n")
		pending.Reset()
		inMulti = false

		if strings.TrimSpace(cmd) == "" {
			continue
		}
		out = append(out, model.Command{Cmd: cmd, Time: ts})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	// Give untimed commands increasing timestamps that end at `fallback`.
	untimed := 0
	for _, c := range out {
		if c.Time.IsZero() {
			untimed++
		}
	}
	n := 0
	for i := range out {
		if out[i].Time.IsZero() {
			out[i].Time = fallback.Add(-time.Duration(untimed-n) * time.Second)
			n++
		}
	}
	return out, nil
}

// parseExtended splits ": 1700000000:0;command" into its timestamp and command.
func parseExtended(line string) (time.Time, string, bool) {
	if !strings.HasPrefix(line, ": ") {
		return time.Time{}, "", false
	}
	rest := line[2:]
	semi := strings.Index(rest, ";")
	if semi < 0 {
		return time.Time{}, "", false
	}
	secs, _, _ := strings.Cut(rest[:semi], ":")
	sec, err := strconv.ParseInt(secs, 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}
	return time.Unix(sec, 0), rest[semi+1:], true
}
