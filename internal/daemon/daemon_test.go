package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/rooneyouma/sterm/internal/model"
	"github.com/rooneyouma/sterm/internal/store"
)

type harness struct {
	sock string
	st   *store.Store
	done chan struct{}
	err  error
}

func start(t *testing.T, seed ...model.Command) *harness {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range seed {
		if err := st.Insert(c); err != nil {
			t.Fatal(err)
		}
	}

	h := &harness{sock: filepath.Join(dir, "s.sock"), st: st, done: make(chan struct{})}
	srv := New(st, h.sock)
	srv.PollEvery = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		h.err = srv.Run(ctx)
		close(h.done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(2 * time.Second):
			t.Error("daemon did not stop")
		}
		st.Close()
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := Do(h.sock, Request{Op: "ping"}); err == nil {
			return h
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("daemon did not become ready")
	return nil
}

func TestQuery(t *testing.T) {
	now := time.Now()
	h := start(t,
		model.Command{Cmd: "git status", Time: now.Add(-1 * time.Hour)},
		model.Command{Cmd: "git status", Time: now.Add(-2 * time.Hour)},
		model.Command{Cmd: "git push", Time: now.Add(-3 * time.Hour)},
	)

	resp, err := Do(h.sock, Request{Op: "query", Buffer: "git ", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"git status", "git push"}; !reflect.DeepEqual(resp.Suggestions, want) {
		t.Errorf("suggestions = %v, want %v", resp.Suggestions, want)
	}

	resp, err = Do(h.sock, Request{Op: "query", Buffer: "zzz", Limit: 1})
	if err != nil || len(resp.Suggestions) != 0 {
		t.Errorf("no match should give an empty reply, got %+v, %v", resp, err)
	}
}

func TestPicksUpNewRows(t *testing.T) {
	h := start(t)

	if err := h.st.Insert(model.Command{Cmd: "docker compose up", Time: time.Now()}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := Do(h.sock, Request{Op: "query", Buffer: "docker ", Limit: 1})
		if err == nil && len(resp.Suggestions) == 1 && resp.Suggestions[0] == "docker compose up" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Error("a command recorded after startup never became a suggestion")
}

func TestSecondDaemonRefuses(t *testing.T) {
	h := start(t)

	err := New(h.st, h.sock).Run(context.Background())
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("second daemon returned %v, want ErrAlreadyRunning", err)
	}
	if _, err := Do(h.sock, Request{Op: "ping"}); err != nil {
		t.Errorf("first daemon should still answer: %v", err)
	}
}

func TestStop(t *testing.T) {
	h := start(t)

	if _, err := Do(h.sock, Request{Op: "stop"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.done:
		if h.err != nil {
			t.Errorf("Run returned %v after stop", h.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
	if _, err := Do(h.sock, Request{Op: "ping"}); err == nil {
		t.Error("daemon still answers after stop")
	}
}

func TestClientWithoutDaemon(t *testing.T) {
	_, err := Do(filepath.Join(t.TempDir(), "none.sock"), Request{Op: "ping"})
	if err == nil {
		t.Error("expected an error when no daemon is running")
	}
}
