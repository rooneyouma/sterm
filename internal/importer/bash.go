package importer

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/rooneyouma/sterm/internal/model"
)

// ParseBash reads a bash history file. A line like "#1700000000" sets the
// timestamp for the command on the next line.
func ParseBash(r io.Reader, fallback time.Time) ([]model.Command, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)

	var out []model.Command
	var ts time.Time

	for sc.Scan() {
		line := sc.Text()

		if strings.HasPrefix(line, "#") {
			if sec, err := strconv.ParseInt(line[1:], 10, 64); err == nil {
				ts = time.Unix(sec, 0)
				continue
			}
		}
		if strings.TrimSpace(line) == "" {
			continue
		}

		out = append(out, model.Command{Cmd: line, Time: ts})
		ts = time.Time{} // a timestamp only applies to one command
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	fillTimes(out, fallback)
	return out, nil
}
