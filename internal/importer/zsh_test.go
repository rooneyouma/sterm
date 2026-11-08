package importer

import (
	"strings"
	"testing"
	"time"
)

func TestParseZsh(t *testing.T) {
	input := ": 1700000000:0;git status\n" +
		": 1700000100:0;echo one \\\n" +
		"two\n" +
		"plain command\n"

	cmds, err := ParseZsh(strings.NewReader(input), time.Unix(1800000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 3 {
		t.Fatalf("got %d commands, want 3", len(cmds))
	}
	if cmds[0].Cmd != "git status" || cmds[0].Time.Unix() != 1700000000 {
		t.Errorf("first command wrong: %+v", cmds[0])
	}
	if !strings.Contains(cmds[1].Cmd, "echo one") || !strings.Contains(cmds[1].Cmd, "two") {
		t.Errorf("multi-line command wrong: %q", cmds[1].Cmd)
	}
	if cmds[2].Cmd != "plain command" || cmds[2].Time.IsZero() {
		t.Errorf("plain command wrong: %+v", cmds[2])
	}
}
