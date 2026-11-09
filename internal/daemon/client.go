package daemon

import (
	"encoding/json"
	"errors"
	"net"
	"time"
)

// Do sends one request to the daemon and returns its reply.
// It fails fast if the daemon isn't running, so callers can fall back.
func Do(sock string, req Request) (Response, error) {
	conn, err := net.DialTimeout("unix", sock, 100*time.Millisecond)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, err
	}
	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
