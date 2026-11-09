package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var ErrOffline = errors.New("no internet connection: connect to the internet and try again")

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var defaultClient = &http.Client{
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 4 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
	},
}

func postJSON(ctx context.Context, c *http.Client, url string, headers map[string]string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if c == nil {
		c = defaultClient
	}

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{Status: resp.StatusCode, Body: truncate(strings.TrimSpace(string(data)), 300)}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("unreadable reply: %w", err)
	}
	return nil
}

func providerError(ctx context.Context, provider, model, key string, err error) error {
	var he *HTTPError
	var dnsErr *net.DNSError
	var opErr *net.OpError

	switch {
	case errors.As(err, &he):
		switch {
		case he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden:
			return redact(errors.New(fmt.Sprintf("%s rejected the key (HTTP %d); check the key and that billing is set up", provider, he.Status)), key)
		case he.Status == http.StatusNotFound:
			return redact(errors.New(fmt.Sprintf("%s: model %q was not found; set a valid model in your config (%s)", provider, model, he.Body)), key)
		case he.Status == http.StatusTooManyRequests:
			return redact(errors.New(fmt.Sprintf("%s: rate limited or out of credit; try again later", provider)), key)
		case he.Status >= 500:
			return redact(errors.New(fmt.Sprintf("%s is busy or down (HTTP %d); try again in a moment", provider, he.Status)), key)
		default:
			return redact(errors.New(fmt.Sprintf("%s rejected the request (HTTP %d): %s", provider, he.Status, he.Body)), key)
		}
	case ctx.Err() != nil:
		return fmt.Errorf("%s: timed out waiting for the API", provider)
	case errors.As(err, &dnsErr), errors.As(err, &opErr):
		return ErrOffline
	default:
		return redact(err, key)
	}
}

func redact(err error, key string) error {
	if err == nil || key == "" {
		return err
	}
	if msg := err.Error(); strings.Contains(msg, key) {
		return errors.New(strings.ReplaceAll(msg, key, "[redacted]"))
	}
	return err
}
