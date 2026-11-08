package importer

import (
	"strings"
	"testing"
	"time"
)

func TestParseBash(t *testing.T) {
	input := "#1700000000\ngit status\nls -la\n\n"

	cmds, err := ParseBash(strings.NewReader(input), time.Unix(1800000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 {
		t.Fatalf("got %d commands, want 2", len(cmds))
	}
	if cmds[0].Cmd != "git status" || cmds[0].Time.Unix() != 1700000000 {
		t.Errorf("first command wrong: %+v", cmds[0])
	}
	if cmds[1].Cmd != "ls -la" || cmds[1].Time.IsZero() {
		t.Errorf("second command wrong: %+v", cmds[1])
	}
}
