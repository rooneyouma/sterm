package safety

import "regexp"

type rule struct {
	re     *regexp.Regexp
	reason string
}

var rules = []rule{
	{regexp.MustCompile(`\brm\b[^|;&]*\s(-[a-zA-Z]*[rR][a-zA-Z]*|--recursive)\b`), "rm -r deletes folders recursively"},
	{regexp.MustCompile(`\bsudo\b`), "runs with root privileges (sudo)"},
	{regexp.MustCompile(`\bdd\b[^|;&]*\bof=`), "dd writes raw data to a file or device"},
	{regexp.MustCompile(`\bmkfs(\.[a-z0-9]+)?\b`), "formats a filesystem"},
	{regexp.MustCompile(`\b(curl|wget)\b[^;&]*\|\s*(sudo\s+)?(ba|z|da)?sh\b`), "pipes a download straight into a shell"},
	{regexp.MustCompile(`\b(chmod|chown)\b[^|;&]*\s-[a-zA-Z]*R`), "changes permissions or ownership recursively"},
	{regexp.MustCompile(`>\s*/dev/(sd|nvme|hd|mmcblk)`), "writes directly to a disk device"},
	{regexp.MustCompile(`:\(\)\s*\{`), "looks like a fork bomb"},
	{regexp.MustCompile(`\bgit\s+push\b[^|;&]*(--force\b|\s-f\b)`), "force push can overwrite remote history"},
	{regexp.MustCompile(`\bgit\s+reset\s+--hard`), "git reset --hard discards uncommitted work"},
	{regexp.MustCompile(`\bgit\s+clean\s+-[a-zA-Z]*f`), "git clean -f deletes untracked files"},
	{regexp.MustCompile(`(?i)\bdrop\s+(table|database)\b`), "drops a database table or database"},
	{regexp.MustCompile(`\b(shutdown|reboot|poweroff)\b`), "shuts down or restarts the machine"},
}

func Check(cmd string) []string {
	var reasons []string
	for _, r := range rules {
		if r.re.MatchString(cmd) {
			reasons = append(reasons, r.reason)
		}
	}
	return reasons
}
