package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rooneyouma/sterm/internal/index"
	"github.com/rooneyouma/sterm/internal/rank"
	"github.com/rooneyouma/sterm/internal/store"
)

var ErrAlreadyRunning = errors.New("daemon already running")

// Request is one message from a client. Op is "query", "ping" or "stop".
type Request struct {
	Op     string `json:"op"`
	Buffer string `json:"buffer,omitempty"`
	Cwd    string `json:"cwd,omitempty"`
	Repo   string `json:"repo,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// Response is the daemon's reply.
type Response struct {
	Suggestions []string `json:"suggestions,omitempty"`
	Commands    int      `json:"commands"`
	Error       string   `json:"error,omitempty"`
}

// SocketPath puts the socket next to the database, in the private data folder.
func SocketPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "sterm.sock")
}

type Server struct {
	PollEvery time.Duration

	st     *store.Store
	sock   string
	ix     *index.Index
	lastID int64
	stop   context.CancelFunc
}

func New(st *store.Store, sock string) *Server {
	return &Server{PollEvery: time.Second, st: st, sock: sock, ix: index.New()}
}

// Run serves until ctx is cancelled or a client sends "stop".
func (s *Server) Run(ctx context.Context) error {
	lock, err := acquireLock(s.sock + ".lock")
	if err != nil {
		return err
	}
	defer lock.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.stop = cancel

	started := time.Now()
	if err := s.sync(); err != nil {
		return fmt.Errorf("load history: %w", err)
	}
	log.Printf("loaded %d distinct commands in %v", s.ix.Len(), time.Since(started).Round(time.Millisecond))

	_ = os.Remove(s.sock)
	ln, err := net.Listen("unix", s.sock)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.sock, 0o600); err != nil {
		ln.Close()
		return err
	}
	log.Printf("listening on %s", s.sock)

	go s.poll(ctx)
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("shutting down")
				return nil
			}
			log.Printf("accept: %v", err)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		go s.handle(conn)
	}
}

// acquireLock takes an exclusive lock on a file, or reports another daemon is running.
func acquireLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, ErrAlreadyRunning
	}
	return f, nil
}

// poll feeds newly recorded commands into the index until ctx is cancelled.
func (s *Server) poll(ctx context.Context) {
	t := time.NewTicker(s.PollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.sync(); err != nil {
				log.Printf("sync: %v", err)
			}
		}
	}
}

// sync pulls rows we haven't seen yet and adds them to the index.
func (s *Server) sync() error {
	cmds, err := s.st.Since(s.lastID)
	if err != nil {
		return err
	}
	for _, c := range cmds {
		s.ix.Add(c)
		s.lastID = c.ID
	}
	return nil
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}
	resp := s.respond(req)
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		return
	}
	if req.Op == "stop" {
		s.stop()
	}
}

func (s *Server) respond(req Request) Response {
	switch req.Op {
	case "ping", "stop":
		return Response{Commands: s.ix.Len()}
	case "query":
		return s.query(req)
	default:
		return Response{Error: fmt.Sprintf("unknown op %q", req.Op)}
	}
}

func (s *Server) query(req Request) Response {
	limit := req.Limit
	if limit < 1 {
		limit = 1
	}

	ms := s.ix.Matches(req.Buffer, req.Cwd, req.Repo)
	rank.Rank(ms, time.Now(), rank.DefaultWeights)
	if len(ms) > limit {
		ms = ms[:limit]
	}

	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Cmd
	}
	return Response{Suggestions: out, Commands: s.ix.Len()}
}
