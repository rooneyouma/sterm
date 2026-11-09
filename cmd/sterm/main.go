package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rooneyouma/sterm/internal/filter"
	"github.com/rooneyouma/sterm/internal/importer"
	"github.com/rooneyouma/sterm/internal/model"
	"github.com/rooneyouma/sterm/internal/rank"
	"github.com/rooneyouma/sterm/internal/repo"
	"github.com/rooneyouma/sterm/internal/store"
)

const version = "0.1.0"

const candidateLimit = 200

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "version":
		fmt.Println("sterm", version)
	case "record":
		err = runRecord(os.Args[2:])
	case "recent":
		err = runRecent()
	case "import":
		err = runImport(os.Args[2:])
	case "query":
		err = runQuery(os.Args[2:])
	case "ask":
		fmt.Println("ask is not implemented yet")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sterm <version|record|recent|import|query|ask>")
}

func openStore() (*store.Store, error) {
	path, err := store.DefaultPath()
	if err != nil {
		return nil, err
	}
	return store.Open(path)
}

func runRecord(args []string) error {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	cmd := fs.String("cmd", "", "the command that was run")
	cwdFlag := fs.String("cwd", "", "directory it was run in (default: current directory)")
	repoFlag := fs.String("repo", "", "git repo root (default: detected from cwd)")
	exit := fs.Int("exit", 0, "exit code")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *cmd == "" {
		return errors.New("--cmd is required")
	}
	if filter.ShouldSkip(*cmd) {
		return nil // silently ignore private commands
	}
	cwd, gitRoot := resolveContext(*cwdFlag, *repoFlag)

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	return s.Insert(model.Command{
		Cmd:      *cmd,
		Cwd:      cwd,
		Repo:     gitRoot,
		ExitCode: *exit,
		Time:     time.Now(),
	})
}

func runRecent() error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	cmds, err := s.Recent(10)
	if err != nil {
		return err
	}
	for _, c := range cmds {
		fmt.Printf("%s  [%d]  %s\n", c.Time.Format("2006-01-02 15:04"), c.ExitCode, c.Cmd)
	}
	return nil
}

func runImport(args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	format := fs.String("format", "zsh", "history format: zsh or bash")
	file := fs.String("file", "", "history file (default depends on format)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *file == "" {
		switch *format {
		case "zsh":
			*file = filepath.Join(home, ".zsh_history")
		case "bash":
			*file = filepath.Join(home, ".bash_history")
		}
	}

	f, err := os.Open(*file)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	var cmds []model.Command
	switch *format {
	case "zsh":
		cmds, err = importer.ParseZsh(f, info.ModTime())
	case "bash":
		cmds, err = importer.ParseBash(f, info.ModTime())
	default:
		return fmt.Errorf("unknown format %q (use zsh or bash)", *format)
	}
	if err != nil {
		return err
	}

	// Keep only commands that pass the privacy filter.
	kept := cmds[:0]
	for _, c := range cmds {
		if !filter.ShouldSkip(c.Cmd) {
			kept = append(kept, c)
		}
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.InsertMany(kept); err != nil {
		return err
	}
	fmt.Printf("imported %d commands (skipped %d private)\n", len(kept), len(cmds)-len(kept))
	return nil
}

func runQuery(args []string) error {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	buffer := fs.String("buffer", "", "what has been typed so far")
	cwdFlag := fs.String("cwd", "", "current directory (default: actual current directory)")
	repoFlag := fs.String("repo", "", "git repo root (default: detected from cwd)")
	limit := fs.Int("limit", 1, "how many suggestions to print")
	explain := fs.Bool("explain", false, "show scores, for debugging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cwd, gitRoot := resolveContext(*cwdFlag, *repoFlag)

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	matches, err := s.PrefixMatches(*buffer, cwd, gitRoot, candidateLimit)
	if err != nil {
		return err
	}

	now := time.Now()
	rank.Rank(matches, now, rank.DefaultWeights)
	if len(matches) > *limit {
		matches = matches[:*limit]
	}

	for _, m := range matches {
		if *explain {
			fmt.Printf("%6.2f  n=%d dir=%d repo=%d fail=%d  %s\n",
				rank.Score(m, now, rank.DefaultWeights),
				m.Count, m.DirCount, m.RepoCount, m.Failures, m.Cmd)
		} else {
			fmt.Println(m.Cmd)
		}
	}
	return nil
}

func resolveContext(cwd, gitRoot string) (string, string) {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if gitRoot == "" {
		gitRoot = repo.Find(cwd)
	}
	return cwd, gitRoot
}
