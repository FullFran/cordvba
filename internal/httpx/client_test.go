package httpx_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/httpx"
)

func TestGetSuccess(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "eye") {
			t.Errorf("User-Agent = %q, want eye to identify itself", ua)
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write([]byte("<rss/>"))
	}))
	defer srv.Close()

	resp, err := httpx.New().Get(context.Background(), srv.URL, httpx.Validators{})
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if string(resp.Body) != "<rss/>" {
		t.Errorf("body = %q", resp.Body)
	}
	if resp.Validators.ETag != `"v1"` {
		t.Errorf("etag = %q", resp.Validators.ETag)
	}
	if resp.FetchedAt.IsZero() {
		t.Error("FetchedAt not set")
	}
}

func TestGetSendsConditionalHeaders(t *testing.T) {
	t.Parallel()

	var gotMatch, gotSince string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMatch = r.Header.Get("If-None-Match")
		gotSince = r.Header.Get("If-Modified-Since")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	sent := httpx.Validators{ETag: `"v1"`, LastModified: "Wed, 27 Aug 2026 10:00:00 GMT"}
	resp, err := httpx.New().Get(context.Background(), srv.URL, sent)

	if !errors.Is(err, httpx.ErrNotModified) {
		t.Fatalf("Get() = %v, want ErrNotModified", err)
	}
	if gotMatch != sent.ETag {
		t.Errorf("If-None-Match = %q, want %q", gotMatch, sent.ETag)
	}
	if gotSince != sent.LastModified {
		t.Errorf("If-Modified-Since = %q, want %q", gotSince, sent.LastModified)
	}
	// A 304 need not echo the validators, so they must be retained.
	if resp.Validators.ETag != sent.ETag {
		t.Errorf("validators = %+v, want the sent ones retained", resp.Validators)
	}
}

func TestGetRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer srv.Close()

	// Truncating silently would let a half-read feed be parsed as complete.
	_, err := httpx.New(httpx.WithMaxBytes(1024)).Get(context.Background(), srv.URL, httpx.Validators{})
	if !errors.Is(err, httpx.ErrTooLarge) {
		t.Fatalf("Get() = %v, want ErrTooLarge", err)
	}
}

func TestGetReportsStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	resp, err := httpx.New().Get(context.Background(), srv.URL, httpx.Validators{})
	if !errors.Is(err, httpx.ErrStatus) {
		t.Fatalf("Get() = %v, want ErrStatus", err)
	}
	if resp.RetryAfter != 2*time.Minute {
		t.Errorf("RetryAfter = %v, want 2m", resp.RetryAfter)
	}
}

func TestGetHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := httpx.New().Get(ctx, srv.URL, httpx.Validators{}); err == nil {
		t.Fatal("Get() with a cancelled context = nil error")
	}
}

func TestGetRejectsBadURL(t *testing.T) {
	t.Parallel()

	if _, err := httpx.New().Get(context.Background(), "://not a url", httpx.Validators{}); err == nil {
		t.Fatal("Get() with a malformed URL = nil error")
	}
}

func TestOptions(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != "custom/1" {
			t.Errorf("User-Agent = %q", ua)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := httpx.New(
		httpx.WithUserAgent("custom/1"),
		httpx.WithTimeout(5*time.Second),
		httpx.WithHTTPClient(&http.Client{Timeout: 5 * time.Second}),
	)
	if _, err := c.Get(context.Background(), srv.URL, httpx.Validators{}); err != nil {
		t.Fatalf("Get() = %v", err)
	}
}

func TestRetryAfterHTTPDate(t *testing.T) {
	t.Parallel()

	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", future)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	resp, _ := httpx.New().Get(context.Background(), srv.URL, httpx.Validators{})
	if resp.RetryAfter <= 0 || resp.RetryAfter > 2*time.Minute {
		t.Errorf("RetryAfter = %v, want roughly 90s", resp.RetryAfter)
	}
}
