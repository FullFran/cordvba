package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FullFran/cordvba/apps/eye/internal/api"
	"github.com/FullFran/cordvba/apps/eye/internal/logging"
)

// guarded builds a server whose /v1 endpoints need a token.
func guarded(t *testing.T, token string) *httptest.Server {
	t.Helper()

	handler := api.New(&fakeStore{}, nil, logging.Discard(), api.WithToken(token)).Handler()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// reply is what a test needs from a response. The body is read and closed
// inside do, so no test can leak a connection and none has to remember not to.
type reply struct {
	Status int
	Header http.Header
	Body   map[string]any
}

// do performs a request with optional headers.
func do(t *testing.T, srv *httptest.Server, method, path string, headers map[string]string) reply {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	out := reply{Status: resp.StatusCode, Header: resp.Header}
	// A 204 has no body by definition, and decoding one is an error about
	// nothing.
	if resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(&out.Body); err != nil {
			t.Fatalf("decode %s %s: %v", method, path, err)
		}
	}
	return out
}

// A deployment with a token is a private one. Every /v1 endpoint has to be
// behind it, not just the ones somebody remembered.
func TestTokenGuardsEveryV1Endpoint(t *testing.T) {
	t.Parallel()

	srv := guarded(t, "s3cret")

	for _, path := range []string{
		"/v1", "/v1/records", "/v1/entities", "/v1/sources", "/v1/stats",
		"/v1/changes", "/v1/geojson", "/v1/entities.geojson",
		"/v1/transit/lines", "/v1/transit/stops",
		"/v1/transit/arrivals?stop=1", "/v1/transit/departures",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			resp := do(t, srv, http.MethodGet, path, nil)
			if resp.Status != http.StatusUnauthorized {
				t.Fatalf("GET %s without a token = %d, want 401", path, resp.Status)
			}
		})
	}
}

func TestTokenAcceptsTheRightBearerAndRefusesTheWrongOne(t *testing.T) {
	t.Parallel()

	srv := guarded(t, "s3cret")

	cases := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{name: "correct", header: "Bearer s3cret", wantStatus: http.StatusOK},
		{name: "lowercase scheme", header: "bearer s3cret", wantStatus: http.StatusOK},
		{name: "wrong token", header: "Bearer nope", wantStatus: http.StatusUnauthorized},
		{name: "prefix of the token", header: "Bearer s3c", wantStatus: http.StatusUnauthorized},
		{name: "token with no scheme", header: "s3cret", wantStatus: http.StatusUnauthorized},
		{name: "another scheme", header: "Basic s3cret", wantStatus: http.StatusUnauthorized},
		{name: "empty", header: "", wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := map[string]string{}
			if tc.header != "" {
				headers["Authorization"] = tc.header
			}

			resp := do(t, srv, http.MethodGet, "/v1/records", headers)
			if resp.Status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.Status, tc.wantStatus)
			}
		})
	}
}

// Probes have no credentials, and a health check that needs one is a health
// check that reports the deployment down the moment the token rotates.
func TestHealthAndIndexStayOpen(t *testing.T) {
	t.Parallel()

	srv := guarded(t, "s3cret")

	for _, path := range []string{"/health", "/", "/openapi.json"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			resp := do(t, srv, http.MethodGet, path, nil)
			if resp.Status != http.StatusOK {
				t.Fatalf("GET %s = %d, want it open", path, resp.Status)
			}
		})
	}
}

// A browser sends the preflight before it sends the token, so a guarded
// preflight makes every cross-origin call fail before it is made.
func TestPreflightIsAnsweredWithoutAToken(t *testing.T) {
	t.Parallel()

	srv := guarded(t, "s3cret")
	resp := do(t, srv, http.MethodOptions, "/v1/records", map[string]string{
		"Origin":                         "https://map.example.org",
		"Access-Control-Request-Method":  "GET",
		"Access-Control-Request-Headers": "authorization",
	})

	if resp.Status != http.StatusNoContent {
		t.Fatalf("preflight = %d, want 204", resp.Status)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); !contains(got, "Authorization") {
		t.Errorf("allow-headers = %q, want it to allow Authorization", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); !contains(got, "Content-Type") {
		t.Errorf("allow-headers = %q, want it to allow Content-Type", got)
	}
	if resp.Header.Get("Access-Control-Max-Age") == "" {
		t.Error("no max-age; every request pays for a preflight")
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); !contains(got, "GET") {
		t.Errorf("allow-methods = %q", got)
	}
}

func TestCORSOriginIsConfigurable(t *testing.T) {
	t.Parallel()

	handler := api.New(&fakeStore{}, nil, logging.Discard(),
		api.WithCORSOrigin("https://map.example.org")).Handler()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp := do(t, srv, http.MethodGet, "/health", nil)
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://map.example.org" {
		t.Errorf("allow-origin = %q", got)
	}
	// A response that varies by origin must say so, or a shared cache will
	// hand one origin's response to another.
	if got := resp.Header.Get("Vary"); !contains(got, "Origin") {
		t.Errorf("vary = %q, want it to name Origin", got)
	}
}

// On a loopback bind on somebody's own laptop, a token is ceremony. The API is
// open when none is configured, and that is a choice, not an oversight.
func TestWithoutATokenTheAPIIsOpen(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{})
	resp := do(t, srv, http.MethodGet, "/v1/records", nil)

	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want an open API when no token is configured", resp.Status)
	}
}

// The refusal has to be readable by a machine, in the same shape as every
// other error this API returns.
func TestUnauthorizedUsesTheCommonErrorShape(t *testing.T) {
	t.Parallel()

	srv := guarded(t, "s3cret")
	status, body := get(t, srv, "/v1/records")

	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d", status)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("body = %v, want {\"error\":\"unauthorized\"}", body)
	}
}
