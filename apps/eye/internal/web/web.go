// Package web serves eye's embedded operator console: a single page that reads
// the same /v1 API any other client reads.
//
// It is an infrastructure adapter and holds no privileges. It never proxies the
// API, never sees a token and never talks to the store: the browser calls /v1
// directly on the same origin, so anything the page can show is something a
// curl one-liner could have shown too. Deleting this package would cost eye a
// convenience and nothing else.
//
// The assets are embedded with go:embed, so the binary stays a single file with
// no build step, no bundler and no npm tree behind it (ADR-0004).
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// assetRoot is the directory inside the embedded filesystem. It is not part of
// any URL: "/css/app.css" is served from "assets/css/app.css".
const assetRoot = "assets"

//go:embed assets
var assets embed.FS

// fetchOrigins are the external origins the page actually fetches from, as
// opposed to merely linking to. Every entry here has to appear in the policy
// below, and the tests fail if the assets reference an origin that is missing.
//
//   - tile.openstreetmap.org serves the basemap images, darkened in CSS.
//   - cdnjs serves Leaflet's script and stylesheet, and the stylesheet in turn
//     references its own images from the same origin.
//   - CARTO serves the dark basemap tiles.
var fetchOrigins = []string{
	"https://cdnjs.cloudflare.com",
	"https://tile.openstreetmap.org",
}

// contentSecurityPolicy denies everything and then names exactly what the page
// loads. Writing it by hand is the only way it stays true: a policy copied from
// a template is a policy nobody can audit against the markup.
//
// There is no 'unsafe-inline' because there is no inline script or style. The
// UI sets element styles through the CSSOM, which CSP does not govern, so the
// bar charts and the map still work under a strict policy.
const contentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self' https://cdnjs.cloudflare.com; " +
	"style-src 'self' https://cdnjs.cloudflare.com; " +
	"img-src 'self' data: https://cdnjs.cloudflare.com https://tile.openstreetmap.org; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"base-uri 'none'; " +
	"form-action 'none'; " +
	"frame-ancestors 'none'; " +
	"object-src 'none'"

// contentTypes maps an extension to the type the browser is told. The mapping
// is explicit rather than mime.TypeByExtension because that function reads the
// host's /etc/mime.types, and an asset served as application/javascript on one
// machine and text/javascript on another is a bug waiting for a strange laptop.
var contentTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".svg":  "image/svg+xml",
	".json": "application/json; charset=utf-8",
}

// asset is one embedded file, prepared once so that serving it is a map lookup.
type asset struct {
	body        []byte
	contentType string
	etag        string
}

// handler serves the embedded UI from a fixed table of files.
type handler struct {
	files map[string]asset
}

// Handler serves the embedded UI. It never proxies the API; the browser talks
// to /v1 directly.
func Handler() http.Handler {
	return &handler{files: load()}
}

// ServeHTTP resolves a request against the embedded table. Anything not in the
// table is a 404, including directories: the UI routes in the fragment, so the
// server never has to guess whether an unknown path is a page or a mistake.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		h.fail(w, http.StatusMethodNotAllowed, "the console is read-only")
		return
	}

	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}

	file, ok := h.files[name]
	if !ok {
		h.fail(w, http.StatusNotFound, "no such asset")
		return
	}

	h.headers(w)
	w.Header().Set("Content-Type", file.contentType)
	w.Header().Set("ETag", file.etag)

	// The assets change only when the binary does, so revalidation is cheap
	// and a stale console after an upgrade is not a thing that can happen.
	if match := r.Header.Get("If-None-Match"); match != "" && etagMatches(match, file.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(file.body)))
	w.WriteHeader(http.StatusOK)

	if r.Method == http.MethodHead {
		return
	}
	// A short write to a disconnected client is not an error worth logging;
	// the connection is already gone.
	_, _ = w.Write(file.body)
}

// headers applies the policy every response carries.
func (h *handler) headers(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-cache")
}

// fail answers in the same JSON shape the API uses, so a client that hits the
// wrong port still gets something it can parse.
func (h *handler) fail(w http.ResponseWriter, status int, message string) {
	h.headers(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":` + strconv.Quote(message) + "}\n"))
}

// load reads every embedded asset into the serving table.
//
// It runs per Handler call rather than in an init, because a package that does
// work on import is a package that cannot be left out of a build.
func load() map[string]asset {
	files := map[string]asset{}

	// The tree is embedded at compile time, so a walk over it cannot fail at
	// runtime for any reason a caller could act on.
	_ = fs.WalkDir(assets, assetRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		body, err := assets.ReadFile(p)
		if err != nil {
			return err
		}

		sum := sha256.Sum256(body)
		files[strings.TrimPrefix(p, assetRoot+"/")] = asset{
			body:        body,
			contentType: contentTypeFor(p),
			etag:        `"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`,
		}
		return nil
	})

	return files
}

// contentTypeFor resolves an extension, falling back to an opaque type rather
// than to a guess. The tests refuse any extension that gets that far.
func contentTypeFor(p string) string {
	if ct, ok := contentTypes[path.Ext(p)]; ok {
		return ct
	}
	return "application/octet-stream"
}

// etagMatches reports whether an If-None-Match list contains the tag. The list
// is comma separated and may be the wildcard.
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}
