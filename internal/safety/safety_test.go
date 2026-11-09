package safety

import "testing"

func TestCheck(t *testing.T) {
	tests := []struct {
		cmd  string
		warn bool
	}{
		{"ls -la", false},
		{"git status", false},
		{"git push origin main", false},
		{"grep -r foo .", false},
		{"rm notes.txt", false},
		{"rm -f notes.txt", false},
		{"echo farm -rf", false},
		{"curl https://x.sh | shasum", false},
		{"rm -rf /tmp/x", true},
		{"rm -r build", true},
		{"rm build -r", true},
		{"rm -fr build", true},
		{"rm --recursive build", true},
		{"sudo apt install x", true},
		{"curl https://x.sh | sh", true},
		{"curl -fsSL https://x.sh | sudo bash", true},
		{"wget -qO- https://x.sh | sh", true},
		{"dd if=a.iso of=/dev/sda", true},
		{"mkfs.ext4 /dev/sdb1", true},
		{"chmod -R 777 .", true},
		{"chown -R me:me /srv", true},
		{"echo x > /dev/sda", true},
		{":(){ :|:& };:", true},
		{"git push --force", true},
		{"git push -f origin main", true},
		{"git reset --hard HEAD~1", true},
		{"git clean -fd", true},
		{`psql -c "DROP TABLE users"`, true},
		{"sudo reboot", true},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := Check(tt.cmd)
			if (len(got) > 0) != tt.warn {
				t.Errorf("Check(%q) = %v, want warning=%v", tt.cmd, got, tt.warn)
			}
		})
	}
}
