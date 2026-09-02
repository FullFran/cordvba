package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// eye reads sources under licences that in several cases permit personal use
// only, and the store holds an aircraft position history. Exposing that to a
// network has to be a decision somebody made, not a default.
func TestCheckBindRefusesNonLoopbackByDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		addr    string
		public  bool
		wantErr bool
	}{
		{name: "loopback ipv4", addr: "127.0.0.1:8787"},
		{name: "loopback ipv6", addr: "[::1]:8787"},
		{name: "localhost", addr: "localhost:8787"},
		{name: "all interfaces", addr: "0.0.0.0:8787", wantErr: true},
		{name: "all interfaces ipv6", addr: "[::]:8787", wantErr: true},
		{name: "empty host means all interfaces", addr: ":8787", wantErr: true},
		{name: "a real address", addr: "192.168.1.20:8787", wantErr: true},
		{name: "a hostname", addr: "eye.local:8787", wantErr: true},
		{name: "all interfaces with --public", addr: "0.0.0.0:8787", public: true},
		{name: "a real address with --public", addr: "192.168.1.20:8787", public: true},
		{name: "malformed", addr: "not-an-address", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkBind(tc.addr, tc.public)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkBind(%q, %v) = nil, want a refusal", tc.addr, tc.public)
				}
				// The refusal has to say how to proceed on purpose.
				if !strings.Contains(err.Error(), "--public") && !strings.Contains(err.Error(), "addr") {
					t.Errorf("error = %q, want it to name the way through", err)
				}
				return
			}
			if err != nil {
				t.Errorf("checkBind(%q, %v) = %v", tc.addr, tc.public, err)
			}
		})
	}
}

// Binding to the world with no token is a mistake, not a choice. A refusal
// here is the only thing between a private store and an open one.
func TestPublicBindNeedsAToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		public  bool
		token   string
		wantErr bool
	}{
		{name: "loopback with no token", public: false},
		{name: "loopback with a token", token: "s3cret"},
		{name: "public with a token", public: true, token: "s3cret"},
		{name: "public with no token", public: true, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkPublicAuth(tc.public, tc.token)
			if tc.wantErr {
				if err == nil {
					t.Fatal("a public bind with no token was allowed")
				}
				// The refusal has to name the way through.
				if !strings.Contains(err.Error(), "EYE_API_TOKEN") ||
					!strings.Contains(err.Error(), "--token-file") {
					t.Errorf("error = %q, want it to name both ways to set a token", err)
				}
				return
			}
			if err != nil {
				t.Errorf("checkPublicAuth(%v, %q) = %v", tc.public, tc.token, err)
			}
		})
	}
}

// A token in a file is the form a container or a systemd unit can actually
// deliver without putting the secret in a process listing.
func TestResolveTokenReadsAFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "token")
	// A trailing newline is what every editor and every `echo` writes, and
	// a token with one fails in a way nobody can see.
	if err := os.WriteFile(path, []byte("  file-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	got, err := resolveToken(path, "env-token")
	if err != nil {
		t.Fatalf("resolveToken = %v", err)
	}
	if got != "file-token" {
		t.Errorf("token = %q, want the file to win, trimmed", got)
	}
}

func TestResolveTokenFallsBackToTheEnvironment(t *testing.T) {
	t.Parallel()

	got, err := resolveToken("", "env-token")
	if err != nil {
		t.Fatalf("resolveToken = %v", err)
	}
	if got != "env-token" {
		t.Errorf("token = %q", got)
	}
}

func TestResolveTokenRefusesAnUnusableFile(t *testing.T) {
	t.Parallel()

	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	cases := []struct{ name, path string }{
		{name: "missing", path: filepath.Join(t.TempDir(), "nope")},
		{name: "empty", path: empty},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := resolveToken(tc.path, "env-token"); err == nil {
				t.Fatal("an unusable token file was accepted")
			}
		})
	}
}

// The refusal has to happen before anything is opened or bound. A serve that
// fails after it is already listening has already made the mistake.
func TestServeRefusesAPublicBindBeforeListening(t *testing.T) {
	t.Setenv("EYE_API_TOKEN", "")
	t.Setenv("EYE_DATA_DIR", t.TempDir())

	var out, errOut bytes.Buffer
	err := serveCommand().Run(t.Context(),
		[]string{"--public", "--addr", "127.0.0.1:0", "--data-dir", t.TempDir()},
		&out, &errOut)

	if err == nil {
		t.Fatal("eye serve --public started with no token")
	}
	if !strings.Contains(err.Error(), "EYE_API_TOKEN") {
		t.Errorf("error = %q", err)
	}
	if out.Len() != 0 {
		t.Errorf("it announced itself before refusing: %q", out.String())
	}
}

// A distroless image has no shell and no curl, so the probe a container health
// check runs is eye itself asking an eye server whether it is answering.
func TestProbeReportsWhatTheServerAnswered(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "healthy", status: http.StatusOK},
		{name: "unhealthy", status: http.StatusInternalServerError, wantErr: true},
		{name: "unauthorized", status: http.StatusUnauthorized, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}))
			defer srv.Close()

			var out bytes.Buffer
			err := probe(t.Context(), srv.URL+"/health", &out)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("probe of a %d response reported healthy", tc.status)
				}
				return
			}
			if err != nil {
				t.Fatalf("probe = %v", err)
			}
			if out.Len() == 0 {
				t.Error("the probe said nothing about what it found")
			}
		})
	}
}

func TestProbeFailsWhenNothingIsListening(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	// Port 1 on loopback: nothing an ordinary user can bind.
	if err := probe(t.Context(), "http://127.0.0.1:1/health", &out); err == nil {
		t.Fatal("a probe of a closed port reported healthy")
	}
}

// The probe must not open the store or bind anything: a health check that
// contends for the database is a health check that causes outages.
func TestProbeShortCircuitsServe(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	// No --data-dir, and a data dir that could not be created: reaching the
	// runtime at all would fail.
	if err := serveCommand().Run(t.Context(),
		[]string{"--probe", srv.URL + "/health", "--addr", "0.0.0.0:8787"},
		&out, &errOut); err != nil {
		t.Fatalf("probe through the command = %v", err)
	}
}
