package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rooneyouma/sterm/internal/daemon"
	"github.com/rooneyouma/sterm/internal/store"
)

// runDaemon handles: sterm daemon [run|start|stop|status]
func runDaemon(args []string) error {
	sub := "run"
	if len(args) > 0 {
		sub = args[0]
	}

	dbPath, err := store.DefaultPath()
	if err != nil {
		return err
	}
	sock := daemon.SocketPath(dbPath)

	switch sub {
	case "run":
		return serveDaemon(dbPath, sock)
	case "start":
		return startDaemon(sock)
	case "stop":
		if _, err := daemon.Do(sock, daemon.Request{Op: "stop"}); err != nil {
			fmt.Println("daemon is not running")
			return nil
		}
		fmt.Println("daemon stopped")
		return nil
	case "status":
		resp, err := daemon.Do(sock, daemon.Request{Op: "ping"})
		if err != nil {
			fmt.Println("daemon is not running")
			return nil
		}
		fmt.Printf("daemon is running, %d distinct commands indexed\n", resp.Commands)
		return nil
	default:
		return fmt.Errorf("unknown daemon command %q (use run, start, stop, status)", sub)
	}
}

// serveDaemon runs the server in the foreground until Ctrl-C or SIGTERM.
func serveDaemon(dbPath, sock string) error {
	s, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer s.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = daemon.New(s, sock).Run(ctx)
	if errors.Is(err, daemon.ErrAlreadyRunning) {
		fmt.Println("daemon already running")
		return nil
	}
	return err
}

// startDaemon launches the daemon in the background, detached from this terminal,
// and returns once it answers (or after about 2 seconds).
func startDaemon(sock string) error {
	if _, err := daemon.Do(sock, daemon.Request{Op: "ping"}); err == nil {
		fmt.Println("daemon already running")
		return nil
	}

	dir := filepath.Dir(sock)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := filepath.Join(dir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "daemon", "run")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	_ = cmd.Process.Release()

	for i := 0; i < 100; i++ {
		if _, err := daemon.Do(sock, daemon.Request{Op: "ping"}); err == nil {
			fmt.Println("daemon started")
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start in time; see %s", logPath)
}
