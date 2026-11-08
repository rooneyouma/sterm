package filter

import "testing"

func TestShouldSkip(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"normal command", "git status", false},
		{"empty", "", true},
		{"only spaces", "   ", true},
		{"leading space", " ls -la", true},
		{"password flag", "mysql --password=abc", true},
		{"aws variable", "export AWS_SECRET_ACCESS_KEY=xyz", true},
		{"token header", "curl -H 'Authorization: token abc'", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldSkip(tt.cmd); got != tt.want {
				t.Errorf("ShouldSkip(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
