package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// corsMaxAge is how long a browser may cache the preflight, in seconds. Ten
// minutes turns a preflight per request into a preflight per session.
const corsMaxAge = "600"

// bearerPrefix is the only authorization scheme this API accepts.
const bearerPrefix = "bearer "

// writeCORS sets the cross-origin headers on every response, preflight or not.
func (s *Server) writeCORS(w http.ResponseWriter) {
	origin := s.corsOrigin
	if origin == "" {
		origin = "*"
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	// Authorization because the token travels in it, Content-Type because a
	// browser client sets it even on a GET.
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Max-Age", corsMaxAge)

	// A response that varies by origin has to say so, or a shared cache
	// hands one origin's response to another.
	if origin != "*" {
		w.Header().Add("Vary", "Origin")
	}
}

// withAuth guards the versioned API with a bearer token.
//
// Only /v1 is guarded. /health is a probe, / describes the service and
// /openapi.json is that description in machine form: a deployment whose health
// check needs a credential is one that reports itself down the moment the
// token rotates.
//
// An empty token leaves everything open, which is the right default for a
// loopback bind on somebody's own machine. `eye serve --public` refuses to
// start without one, so "open" can never mean "open to the network".
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" || !guarded(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if !s.authorized(r.Header.Get("Authorization")) {
			// The header is the way in, and RFC 7235 says a 401 names it.
			w.Header().Set("WWW-Authenticate", `Bearer realm="eye"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// guarded reports whether a path belongs to the versioned API.
//
// The comparison is exact rather than a prefix match, so "/v1nonsense" is a
// 404 from the mux and not an accidentally guarded — or accidentally open —
// endpoint.
func guarded(path string) bool {
	return path == "/v1" || strings.HasPrefix(path, "/v1/")
}

// authorized compares the presented credential against the configured token.
//
// Both sides are hashed first so the comparison runs over two fixed-length
// values: subtle.ConstantTimeCompare returns early when lengths differ, which
// would leak the length of the token one probe at a time.
func (s *Server) authorized(header string) bool {
	if len(header) < len(bearerPrefix) ||
		!strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return false
	}

	presented := sha256.Sum256([]byte(strings.TrimSpace(header[len(bearerPrefix):])))
	expected := sha256.Sum256([]byte(s.token))

	return subtle.ConstantTimeCompare(presented[:], expected[:]) == 1
}
