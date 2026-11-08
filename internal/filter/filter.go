package filter

import (
	"regexp"
	"strings"
)

var secretPattern = regexp.MustCompile(`(?i)(password|passwd|token|secret|api[_-]?key|AWS_)`)

// ShouldSkip reports whether a command must NOT be saved.
func ShouldSkip(cmd string) bool {
	if strings.TrimSpace(cmd) == "" {
		return true
	}
	if strings.HasPrefix(cmd, " ") { // a leading space means "keep this private"
		return true
	}
	return secretPattern.MatchString(cmd)
}
