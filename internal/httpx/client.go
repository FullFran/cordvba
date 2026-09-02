// Package httpx is the shared outbound HTTP adapter. Every request eye makes to
// a public source goes through it, so the timeouts, size caps, conditional
// requests and rate-limit courtesy are decided once instead of per provider.
package httpx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// Recorder stores a raw payload and returns its content hash. The raw cache
// implements it.
type Recorder interface {
	Put(body []byte) (string, error)
}

// Client wraps net/http with the policies every eye request needs.
type Client struct {
	http      *http.Client
	userAgent string
	maxBytes  int64
	recorder  Recorder

	// extraCAFile is a PEM file of additional certificate authorities to
	// trust, applied by NewWithOptions. It is a path rather than a pool so
	// that a bad path is reported as the configuration error it is.
	extraCAFile string
	// buildErr carries a failure an Option could not return, so
	// NewWithOptions can surface it instead of New silently ignoring it.
	buildErr error
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

// WithExtraCAFile trusts the certificate authorities in a PEM file, in addition
// to the system pool.
//
// It exists for one specific and unglamorous reason. Several Spanish
// public-sector servers send an INCOMPLETE certificate chain: MITECO's
// air-quality host, for instance, omits the FNMT-RCM intermediate that links
// its certificate to a root the system already trusts. Browsers paper over this
// by fetching the missing certificate from the address in the AIA extension.
// Go does not, by design, so the handshake fails with "certificate signed by
// unknown authority" against a server whose certificate is perfectly valid.
//
// Supplying the missing intermediate COMPLETES a chain. It is the opposite of
// InsecureSkipVerify, which eye does not have and will not grow: every
// certificate is still verified, against a pool the operator widened on
// purpose, one named file at a time.
//
// An empty path means "not configured" and changes nothing. Anything else that
// cannot be read, or that contains no certificate, is an error — a typo in this
// path must not degrade into a source that fails for a reason nobody can find.
func WithExtraCAFile(path string) Option {
	return func(c *Client) { c.extraCAFile = strings.TrimSpace(path) }
}

// WithRecorder stores every successful payload as evidence.
//
// Recording here rather than in each provider means the bytes kept are exactly
// the bytes parsed, and the hash the cache files them under is the same
// SHA-256 the provider writes into Provenance.RawHash.
func WithRecorder(r Recorder) Option {
	return func(c *Client) { c.recorder = r }
}

// New builds a Client. The zero-option form is the one providers should use.
//
// Options that can fail are ignored here. Use NewWithOptions when any of them
// can — today that is WithExtraCAFile — so the failure is returned rather than
// swallowed.
func New(opts ...Option) *Client {
	c, _ := build(opts)
	return c
}

// NewWithOptions builds a Client and reports an option that could not be
// applied.
func NewWithOptions(opts ...Option) (*Client, error) {
	c, err := build(opts)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// build applies the options and wires anything that needs assembling after
// them, such as the TLS trust pool.
func build(opts []Option) (*Client, error) {
	c := &Client{
		http:      &http.Client{Timeout: DefaultTimeout},
		userAgent: DefaultUserAgent,
		maxBytes:  DefaultMaxBytes,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.buildErr != nil {
		return nil, c.buildErr
	}
	if err := c.applyExtraCAs(); err != nil {
		return nil, err
	}
	return c, nil
}

// applyExtraCAs widens the client's trust pool with the operator's PEM file.
func (c *Client) applyExtraCAs() error {
	if c.extraCAFile == "" {
		return nil
	}

	pem, err := os.ReadFile(c.extraCAFile) // #nosec G304 -- operator-supplied CA path, never request input
	if err != nil {
		return fmt.Errorf("read extra CA file %s: %w", c.extraCAFile, err)
	}

	// Start from the system pool rather than replacing it. The point is to
	// ADD a missing intermediate, not to narrow eye down to trusting one
	// authority and nothing else.
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return fmt.Errorf("extra CA file %s contains no certificate", c.extraCAFile)
	}

	transport, ok := c.http.Transport.(*http.Transport)
	if !ok || transport == nil {
		base, _ := http.DefaultTransport.(*http.Transport)
		transport = base.Clone()
	} else {
		transport = transport.Clone()
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	transport.TLSClientConfig.RootCAs = pool
	c.http.Transport = transport
	return nil
}

// Validators carries the cache validators a source returned last time, so the
// next request can ask "has this changed?" instead of downloading it again.
type Validators struct {
	ETag         string
	LastModified string
}

// Request describes one outbound fetch: the URL, the cache validators from the
// previous poll, and any extra headers the source's contract requires.
//
// Headers exist for credentials. AEMET takes its key in an `api_key` header,
// and putting a key in a query string would leak it into every error message,
// log line and health row that quotes the URL.
type Request struct {
	URL        string
	Validators Validators
	// Headers are set before the conditional ones, so a caller can never
	// silently disable If-None-Match by supplying it here.
	Headers map[string]string
}

// Response is a fetched body plus the metadata needed to poll politely next
// time.
type Response struct {
	Body []byte
	// StatusCode is the status the source answered with, set for every
	// outcome including the failures. A source that says "no data" with a
	// 404 and "wrong key" with a 401 is telling the caller two very
	// different things, and ErrStatus alone cannot carry the difference.
	StatusCode int
	// Status is the full status line, for messages meant for a human.
	Status     string
	Validators Validators
	// ContentType is the raw header value, charset parameter included. RSS
	// feeds in the wild are not all UTF-8.
	ContentType string
	// FetchedAt is when the response completed.
	FetchedAt time.Time
	// RetryAfter is the publisher's own instruction on when to come back.
	RetryAfter time.Duration
	// RecordError is set when the payload could not be written to the raw
	// cache. The response itself is still usable.
	RecordError error
}

// Get fetches a URL, sending conditional headers when validators are supplied.
//
// A 304 returns ErrNotModified with a Response carrying the retained
// validators, so the caller can record a successful, empty poll.
func (c *Client) Get(ctx context.Context, url string, v Validators) (*Response, error) {
	return c.Do(ctx, Request{URL: url, Validators: v})
}

// Do fetches one request, applying every policy Get applies plus the caller's
// own headers.
func (c *Client) Do(ctx context.Context, r Request) (*Response, error) {
	url, v := r.URL, r.Validators

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)
	for name, value := range r.Headers {
		if value != "" {
			req.Header.Set(name, value)
		}
	}
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
		StatusCode:  resp.StatusCode,
		Status:      resp.Status,
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

	if c.recorder != nil {
		// A cache failure must not fail the poll: losing the evidence
		// copy is worse than not having it, but losing the observation
		// too is worse still.
		if _, err := c.recorder.Put(body); err != nil {
			out.RecordError = err
		}
	}

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
