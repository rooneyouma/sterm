package importer

import (
	"time"

	"github.com/rooneyouma/sterm/internal/model"
)

// fillTimes gives untimed commands increasing timestamps that end at `fallback`.
func fillTimes(cmds []model.Command, fallback time.Time) {
	untimed := 0
	for _, c := range cmds {
		if c.Time.IsZero() {
			untimed++
		}
	}
	n := 0
	for i := range cmds {
		if cmds[i].Time.IsZero() {
			cmds[i].Time = fallback.Add(-time.Duration(untimed-n) * time.Second)
			n++
		}
	}
}
