package httpx_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
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

// recordingSpy captures what the client hands to the raw cache.
type recordingSpy struct {
	bodies [][]byte
	err    error
}

func (r *recordingSpy) Put(body []byte) (string, error) {
	r.bodies = append(r.bodies, append([]byte(nil), body...))
	return "hash", r.err
}

func TestGetRecordsThePayload(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<rss>evidence</rss>"))
	}))
	defer srv.Close()

	spy := &recordingSpy{}
	resp, err := httpx.New(httpx.WithRecorder(spy)).Get(context.Background(), srv.URL, httpx.Validators{})
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}

	if len(spy.bodies) != 1 {
		t.Fatalf("recorder saw %d payloads, want 1", len(spy.bodies))
	}
	// The bytes recorded must be the bytes parsed, or the evidence chain
	// points at something the code never read.
	if string(spy.bodies[0]) != string(resp.Body) {
		t.Errorf("recorded %q, parsed %q", spy.bodies[0], resp.Body)
	}
	if resp.RecordError != nil {
		t.Errorf("RecordError = %v", resp.RecordError)
	}
}

// Losing the evidence copy is bad. Losing the observation as well is worse.
func TestGetSurvivesARecorderFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	spy := &recordingSpy{err: errors.New("disk full")}
	resp, err := httpx.New(httpx.WithRecorder(spy)).Get(context.Background(), srv.URL, httpx.Validators{})
	if err != nil {
		t.Fatalf("Get() = %v, want the response despite the cache failure", err)
	}
	if string(resp.Body) != "payload" {
		t.Errorf("body = %q", resp.Body)
	}
	if resp.RecordError == nil {
		t.Error("RecordError is nil; the cache failure was swallowed silently")
	}
}

// A credential belongs in a header, not in a URL: URLs end up in error
// messages, in the health table and in the operator's terminal.
func TestDoSendsRequestHeaders(t *testing.T) {
	t.Parallel()

	var gotKey, gotAccept, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("api_key")
		gotAccept = r.Header.Get("Accept")
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"estado":200}`))
	}))
	defer srv.Close()

	resp, err := httpx.New().Do(context.Background(), httpx.Request{
		URL:     srv.URL,
		Headers: map[string]string{"api_key": "a.b.c", "Accept": "application/json"},
	})
	if err != nil {
		t.Fatalf("Do() = %v", err)
	}
	if gotKey != "a.b.c" {
		t.Errorf("api_key = %q, want the caller's header", gotKey)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if !strings.Contains(gotUA, "eye") {
		t.Errorf("User-Agent = %q, want eye to still identify itself", gotUA)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

// The status code is part of the answer. AEMET says "no data" with a 404 and
// "wrong key" with a 401, and telling those apart is the difference between an
// empty poll and a misconfiguration.
func TestDoReportsStatusCodeOnFailure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		code int
	}{
		{name: "unauthorized", code: http.StatusUnauthorized},
		{name: "no data", code: http.StatusNotFound},
		{name: "rate limited", code: http.StatusTooManyRequests},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
			}))
			defer srv.Close()

			resp, err := httpx.New().Get(context.Background(), srv.URL, httpx.Validators{})
			if !errors.Is(err, httpx.ErrStatus) {
				t.Fatalf("Get() = %v, want ErrStatus", err)
			}
			if resp.StatusCode != tc.code {
				t.Errorf("StatusCode = %d, want %d", resp.StatusCode, tc.code)
			}
		})
	}
}

// writePEM saves a certificate as PEM and returns the path.
func writePEM(t *testing.T, cert *x509.Certificate) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "extra-ca.pem")
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write pem: %v", err)
	}
	return path
}

// A server whose certificate is not in the system pool must be refused. This is
// the control for the test below it: without it, WithExtraCAFile could be doing
// nothing and both tests would still pass.
func TestGetRefusesAnUntrustedCertificate(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret"))
	}))
	t.Cleanup(srv.Close)

	_, err := httpx.New().Get(t.Context(), srv.URL, httpx.Validators{})
	if err == nil {
		t.Fatal("an unknown certificate authority was accepted")
	}
}

// TestWithExtraCAFileCompletesAChain is why this option exists.
//
// Several Spanish public-sector servers — MITECO's air-quality host among them —
// send an INCOMPLETE certificate chain, omitting the FNMT-RCM intermediate. The
// root is publicly trusted and the intermediate is published; Go simply does not
// fetch it, because it does no AIA chasing. Supplying it is completing a chain,
// not disabling verification, and the difference is the whole point.
func TestWithExtraCAFileCompletesAChain(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("data"))
	}))
	t.Cleanup(srv.Close)

	client, err := httpx.NewWithOptions(httpx.WithExtraCAFile(writePEM(t, srv.Certificate())))
	if err != nil {
		t.Fatalf("httpx.NewWithOptions() = %v", err)
	}

	resp, err := client.Get(t.Context(), srv.URL, httpx.Validators{})
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if got := string(resp.Body); got != "data" {
		t.Errorf("body = %q, want %q", got, "data")
	}
}

// A missing or unreadable CA file is an error the operator must see. Silently
// carrying on with the system pool would turn a typo in a path into a source
// that fails for a reason nobody can find.
func TestWithExtraCAFileReportsAMissingFile(t *testing.T) {
	t.Parallel()

	_, err := httpx.NewWithOptions(httpx.WithExtraCAFile(filepath.Join(t.TempDir(), "nope.pem")))
	if err == nil {
		t.Fatal("a missing CA file was accepted")
	}
}

// A file that exists but holds no certificate is the same class of mistake, and
// is worse to miss: it looks configured.
func TestWithExtraCAFileRejectsAFileWithNoCertificate(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(path, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := httpx.NewWithOptions(httpx.WithExtraCAFile(path))
	if err == nil {
		t.Fatal("a file with no certificate in it was accepted")
	}
}

// An empty path means "not configured" and must build a normal client.
func TestWithExtraCAFileIgnoresAnEmptyPath(t *testing.T) {
	t.Parallel()

	if _, err := httpx.NewWithOptions(httpx.WithExtraCAFile("  ")); err != nil {
		t.Fatalf("an unset EYE_EXTRA_CA_FILE broke the client: %v", err)
	}
}

// selfSignedServer starts a TLS server on a certificate generated here.
//
// httptest.NewTLSServer reuses one built-in certificate for every server it
// creates, so two of those are not two authorities — trusting one trusts the
// other, and a test built on them proves nothing about the size of the pool.
func selfSignedServer(t *testing.T, commonName string) *httptest.Server {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// The serial and the validity window are fixed rather than derived from
	// the clock: nothing here depends on the date, and a test that does is a
	// test that fails on a Tuesday in a year's time.
	template := x509.Certificate{
		SerialNumber:          big.NewInt(4242),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(commonName))
	}))
	srv.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// TestWithExtraCAFileWidensTheSystemPoolRatherThanReplacingIt guards the
// invariant the option is named after.
//
// The failure it prevents is subtle and nasty: build the pool from the extra
// file alone and eye trusts that authority and NOTHING else, so every other
// source in the registry starts failing TLS against certificates that are
// perfectly valid. That reads as "the internet broke", not as "the CA option is
// wrong", which is exactly the kind of error this option exists to avoid
// creating.
func TestWithExtraCAFileWidensTheSystemPoolRatherThanReplacingIt(t *testing.T) {
	t.Parallel()

	trusted := selfSignedServer(t, "trusted.example")
	stranger := selfSignedServer(t, "stranger.example")

	client, err := httpx.NewWithOptions(httpx.WithExtraCAFile(writePEM(t, trusted.Certificate())))
	if err != nil {
		t.Fatalf("NewWithOptions() = %v", err)
	}

	if _, err := client.Get(t.Context(), trusted.URL, httpx.Validators{}); err != nil {
		t.Errorf("the authority in the extra file was not trusted: %v", err)
	}
	if _, err := client.Get(t.Context(), stranger.URL, httpx.Validators{}); err == nil {
		t.Error("an unrelated certificate authority was accepted; the option widened trust too far")
	}
}
