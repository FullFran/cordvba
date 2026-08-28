// Package httpx is the shared outbound HTTP adapter. Every request eye makes to
// a public source goes through it, so the timeouts, size caps, conditional
// requests and rate-limit courtesy are decided once instead of per provider.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Errors returned by Client.
var (
	// ErrNotModified reports a 304. It is a healthy outcome, not a failure:
	// the source simply had nothing new.
	ErrNotModified = errors.New("httpx: not modified")
	// ErrTooLarge reports a body over the configured cap.
	ErrTooLarge = errors.New("httpx: response body exceeds limit")
	// ErrStatus reports an unexpected HTTP status.
	ErrStatus = errors.New("httpx: unexpected status")
)

// Defaults applied when the caller does not override them.
const (
	DefaultTimeout   = 30 * time.Second
	DefaultMaxBytes  = 32 << 20 // 32 MiB — GTFS feeds are large; HTML is not
	DefaultUserAgent = "eye/0.1 (+https://github.com/FullFran/eye)"
)

// Client wraps net/http with the policies every eye request needs.
type Client struct {
	http      *http.Client
	userAgent string
	maxBytes  int64
}

// Option customises a Client.
type Option func(*Client)

// WithTimeout overrides the per-request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.http.Timeout = d }
}

// WithMaxBytes overrides the response body cap.
func WithMaxBytes(n int64) Option {
	return func(c *Client) { c.maxBytes = n }
}

// WithUserAgent overrides the User-Agent header. eye always identifies itself:
// an operator seeing our traffic should be able to tell who we are.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithHTTPClient injects an underlying client, mainly for tests.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// New builds a Client. The zero-option form is the one providers should use.
func New(opts ...Option) *Client {
	c := &Client{
		http:      &http.Client{Timeout: DefaultTimeout},
		userAgent: DefaultUserAgent,
		maxBytes:  DefaultMaxBytes,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Validators carries the cache validators a source returned last time, so the
// next request can ask "has this changed?" instead of downloading it again.
type Validators struct {
	ETag         string
	LastModified string
}

// Response is a fetched body plus the metadata needed to poll politely next
// time.
type Response struct {
	Body       []byte
	Validators Validators
	// ContentType is the raw header value, charset parameter included. RSS
	// feeds in the wild are not all UTF-8.
	ContentType string
	// FetchedAt is when the response completed.
	FetchedAt time.Time
	// RetryAfter is the publisher's own instruction on when to come back.
	RetryAfter time.Duration
}

// Get fetches a URL, sending conditional headers when validators are supplied.
//
// A 304 returns ErrNotModified with a Response carrying the retained
// validators, so the caller can record a successful, empty poll.
func (c *Client) Get(ctx context.Context, url string, v Validators) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)
	// Accept-Encoding is deliberately NOT set: net/http negotiates gzip on
	// its own and decompresses transparently, but only while the caller has
	// not touched the header. Setting it by hand hands us raw gzip bytes,
	// which then fail to parse as XML in a way that looks like a broken feed.
	if v.ETag != "" {
		req.Header.Set("If-None-Match", v.ETag)
	}
	if v.LastModified != "" {
		req.Header.Set("If-Modified-Since", v.LastModified)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	out := &Response{
		ContentType: resp.Header.Get("Content-Type"),
		FetchedAt:   time.Now().UTC(),
		RetryAfter:  parseRetryAfter(resp.Header.Get("Retry-After")),
		Validators: Validators{
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
		},
	}

	switch {
	case resp.StatusCode == http.StatusNotModified:
		// Retain the validators we sent; a 304 need not echo them.
		if out.Validators.ETag == "" {
			out.Validators = v
		}
		return out, ErrNotModified
	case resp.StatusCode != http.StatusOK:
		return out, fmt.Errorf("%w: %s from %s", ErrStatus, resp.Status, url)
	}

	body, err := readCapped(resp.Body, c.maxBytes)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", url, err)
	}
	out.Body = body

	return out, nil
}

// readCapped reads at most maxBytes, and reports an error rather than silently
// truncating: a half-read feed parsed as if complete is worse than a failure.
func readCapped(r io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%w (%d bytes)", ErrTooLarge, maxBytes)
	}
	return body, nil
}

// parseRetryAfter accepts both forms of the header: delay-seconds and an
// HTTP date.
func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
