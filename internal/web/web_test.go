package web

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
)

// expectedAssets is the manifest of everything the UI ships. It is written by
// hand so that adding a file without deciding its content type is a test
// failure rather than a silent octet-stream.
var expectedAssets = []struct {
	path        string
	contentType string
}{
	{"index.html", "text/html; charset=utf-8"},
	{"favicon.svg", "image/svg+xml"},
	{"css/app.css", "text/css; charset=utf-8"},
	{"js/main.js", "text/javascript; charset=utf-8"},
	{"js/api.js", "text/javascript; charset=utf-8"},
	{"js/dom.js", "text/javascript; charset=utf-8"},
	{"js/format.js", "text/javascript; charset=utf-8"},
	{"js/highlight.js", "text/javascript; charset=utf-8"},
	{"js/basemap.js", "text/javascript; charset=utf-8"},
	{"js/views/dashboard.js", "text/javascript; charset=utf-8"},
	{"js/views/map.js", "text/javascript; charset=utf-8"},
	{"js/views/transit.js", "text/javascript; charset=utf-8"},
	{"js/views/records.js", "text/javascript; charset=utf-8"},
	{"js/views/sources.js", "text/javascript; charset=utf-8"},
}

func get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	return w
}

func TestHandler_servesIndexAtRoot(t *testing.T) {
	t.Parallel()

	w := get(t, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("got content type %q, want text/html; charset=utf-8", ct)
	}

	body := w.Body.String()
	for _, want := range []string{"<title>", `id="app"`, `js/main.js`} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html does not contain %q", want)
		}
	}
}

func TestHandler_servesEveryAsset(t *testing.T) {
	t.Parallel()

	for _, tc := range expectedAssets {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			w := get(t, "/"+tc.path)
			if w.Code != http.StatusOK {
				t.Fatalf("got status %d, want 200", w.Code)
			}
			if ct := w.Header().Get("Content-Type"); ct != tc.contentType {
				t.Errorf("got content type %q, want %q", ct, tc.contentType)
			}
			if w.Body.Len() == 0 {
				t.Error("asset served an empty body")
			}
			if w.Header().Get("ETag") == "" {
				t.Error("asset served without an ETag")
			}
		})
	}
}

func TestHandler_notFound(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		target string
	}{
		{name: "unknown page", target: "/nope"},
		{name: "unknown asset", target: "/js/does-not-exist.js"},
		{name: "the embed prefix is not a route", target: "/assets/index.html"},
		{name: "directory listing", target: "/js/"},
		{name: "traversal attempt", target: "/../go.mod"},
		{name: "encoded traversal attempt", target: "/%2e%2e/go.mod"},
		{name: "api paths belong to the api", target: "/v1/records"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := get(t, tc.target)
			if w.Code != http.StatusNotFound {
				t.Fatalf("got status %d for %q, want 404", w.Code, tc.target)
			}
		})
	}
}

func TestHandler_rejectsWritingMethods(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			Handler().ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), method, "/", nil))
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("got status %d, want 405", w.Code)
			}
			if allow := w.Header().Get("Allow"); allow != "GET, HEAD" {
				t.Errorf("got Allow %q, want %q", allow, "GET, HEAD")
			}
		})
	}
}

func TestHandler_headServesHeadersWithoutBody(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodHead, "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("HEAD returned %d body bytes, want 0", w.Body.Len())
	}
	if w.Header().Get("Content-Length") == "" {
		t.Error("HEAD did not report a Content-Length")
	}
}

func TestHandler_securityHeaders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		want   string
	}{
		{name: "csp", header: "Content-Security-Policy", want: contentSecurityPolicy},
		{name: "no sniffing", header: "X-Content-Type-Options", want: "nosniff"},
		{name: "no framing", header: "X-Frame-Options", want: "DENY"},
		{name: "no referrer leak", header: "Referrer-Policy", want: "no-referrer"},
		{name: "revalidate", header: "Cache-Control", want: "no-cache"},
	}

	w := get(t, "/")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := w.Header().Get(tc.header); got != tc.want {
				t.Errorf("got %s = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestHandler_conditionalGetRevalidates(t *testing.T) {
	t.Parallel()

	first := get(t, "/css/app.css")
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the first response")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/css/app.css", nil)
	r.Header.Set("If-None-Match", etag)
	Handler().ServeHTTP(w, r)

	if w.Code != http.StatusNotModified {
		t.Fatalf("got status %d for a matching ETag, want 304", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("304 carried %d body bytes, want 0", w.Body.Len())
	}
}

func TestEmbeddedAssets_matchTheManifestAndAreNotEmpty(t *testing.T) {
	t.Parallel()

	declared := map[string]bool{}
	for _, a := range expectedAssets {
		declared[a.path] = true
	}

	found := map[string]bool{}
	err := fs.WalkDir(assets, assetRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel := strings.TrimPrefix(p, assetRoot+"/")
		found[rel] = true

		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() == 0 {
			t.Errorf("embedded asset %q is empty", rel)
		}
		if _, ok := contentTypes[path.Ext(rel)]; !ok {
			t.Errorf("embedded asset %q has extension %q with no declared content type", rel, path.Ext(rel))
		}
		if !declared[rel] {
			t.Errorf("embedded asset %q is not in the test manifest", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the embedded assets: %v", err)
	}

	for p := range declared {
		if !found[p] {
			t.Errorf("manifest lists %q but it is not embedded", p)
		}
	}
}

// originPattern finds every https origin mentioned anywhere in the UI. The
// character class stops at the first path separator, so a match is the origin.
var originPattern = regexp.MustCompile(`https://[A-Za-z0-9{}*._-]+`)

// navigationOrigins are linked with an anchor and never fetched, so they do not
// belong in the policy. Leaflet's tile attribution requires both of them.
var navigationOrigins = map[string]bool{
	"https://www.openstreetmap.org": true,
	"https://carto.com":             true,
	"https://leafletjs.com":         true,
}

// TestContentSecurityPolicy_coversWhatThePageActuallyLoads is the point of the
// whole exercise: a policy that does not match the page is worse than none,
// because it looks like protection while the browser silently drops requests.
func TestContentSecurityPolicy_coversWhatThePageActuallyLoads(t *testing.T) {
	t.Parallel()

	for _, origin := range fetchOrigins {
		if !strings.Contains(contentSecurityPolicy, origin) {
			t.Errorf("policy does not name the fetched origin %q", origin)
		}
	}

	allowed := map[string]bool{}
	for _, o := range fetchOrigins {
		allowed[o] = true
	}

	err := fs.WalkDir(assets, assetRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := assets.ReadFile(p)
		if err != nil {
			return err
		}

		text := string(body)
		for _, loc := range originPattern.FindAllStringIndex(text, -1) {
			origin := normaliseOrigin(text[loc[0]:loc[1]])

			switch {
			case allowed[origin]:
				continue
			case navigationOrigins[origin]:
				// A link is a navigation, not a subresource. Prove it is
				// really a link rather than a forgotten fetch.
				if !strings.HasSuffix(text[:loc[0]], `<a href="`) {
					t.Errorf("%s: %q is treated as a link-only origin but is not inside an anchor href", p, origin)
				}
			default:
				t.Errorf("%s references %q, which the Content-Security-Policy does not allow", p, origin)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the embedded assets: %v", err)
	}
}

// normaliseOrigin turns a Leaflet subdomain template into the wildcard a policy
// would use, so a `{s}.tiles.example.com` URL matches `*.tiles.example.com`.
//
// No current tile URL uses one — OpenStreetMap serves from a single host — but
// the substitution stays because the next basemap somebody tries will, and a
// CSP check that silently stops matching is worse than no check.
func normaliseOrigin(origin string) string {
	return strings.Replace(origin, "https://{s}.", "https://*.", 1)
}

func TestContentSecurityPolicy_locksDownEverythingElse(t *testing.T) {
	t.Parallel()

	for _, want := range []string{
		"default-src 'none'",
		"connect-src 'self'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
		"object-src 'none'",
	} {
		if !strings.Contains(contentSecurityPolicy, want) {
			t.Errorf("policy is missing %q", want)
		}
	}

	// An inline script or style would need one of these, and the UI has
	// neither. If that ever changes the policy must change deliberately.
	for _, forbidden := range []string{"'unsafe-inline'", "'unsafe-eval'"} {
		if strings.Contains(contentSecurityPolicy, forbidden) {
			t.Errorf("policy contains %s", forbidden)
		}
	}
}

func TestIndex_hasNoInlineScriptOrStyle(t *testing.T) {
	t.Parallel()

	body, err := assets.ReadFile(assetRoot + "/index.html")
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}

	html := string(body)
	for _, forbidden := range []string{"<script>", "<style>", ` style="`, "onclick=", "onload="} {
		if strings.Contains(html, forbidden) {
			t.Errorf("index.html contains %q, which the Content-Security-Policy forbids", forbidden)
		}
	}
}

func TestHandler_isReusableAcrossRequests(t *testing.T) {
	t.Parallel()

	h := Handler()
	for i := range 3 {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/js/main.js", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: got status %d, want 200", i, w.Code)
		}
		if _, err := io.ReadAll(w.Body); err != nil {
			t.Fatalf("request %d: reading body: %v", i, err)
		}
	}
}
