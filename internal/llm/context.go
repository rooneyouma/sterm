package llm

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func DetectOS() string {
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
				return strings.Trim(v, `"`)
			}
		}
	}
	return runtime.GOOS
}

func DetectShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return filepath.Base(s)
	}
	return "sh"
}

var markers = []struct{ file, kind string }{
	{"go.mod", "Go"},
	{"package.json", "Node.js"},
	{"Cargo.toml", "Rust"},
	{"pyproject.toml", "Python"},
	{"requirements.txt", "Python"},
	{"Gemfile", "Ruby"},
	{"pom.xml", "Java (Maven)"},
	{"build.gradle", "Java or Kotlin (Gradle)"},
	{"composer.json", "PHP"},
}

func DetectProject(dir string) string {
	for i := 0; i < 6 && dir != ""; i++ {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m.file)); err == nil {
				return m.kind
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
