// Package aemet adapts AEMET OpenData, the Spanish meteorological agency's
// public API.
//
// Every AEMET product is fetched in two steps. The documented endpoint answers
// with a small envelope — descripcion, estado, datos, metadatos — and the
// payload itself lives at the one-shot URL in `datos`, which expires. Both
// steps are part of one poll, and neither is useful alone.
//
// The key travels in the `api_key` header, as AEMET's own OpenAPI document
// specifies. The query-string form also works, and is not used: a key in a URL
// ends up in error strings, health rows and the operator's terminal.
package aemet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	source "github.com/FullFran/eye/internal/source/domain"
)

// Errors returned by the AEMET adapters.
var (
	// ErrAEMET wraps anything AEMET answered that eye cannot use.
	ErrAEMET = errors.New("aemet")

	// ErrMissingAPIKey reports that this build has no credential for AEMET.
	// It names the environment variable on purpose: a source that silently
	// polls nothing is worse than one that says why it cannot.
	ErrMissingAPIKey = errors.New("aemet: no API key; set AEMET_API_KEY (free at https://opendata.aemet.es/centrodedescargas/altaUsuario)")

	// ErrUnauthorized reports estado 401 or 403: the key was rejected.
	ErrUnauthorized = errors.New("aemet: the API key was rejected")

	// ErrRateLimited reports estado 429. AEMET publishes a request budget
	// and this is it being spent.
	ErrRateLimited = errors.New("aemet: request limit reached")

	// ErrNoData reports estado 404, which AEMET uses both for "peticion sin
	// datos" and for a `datos` URL that has expired. It is a normal, empty
	// outcome rather than a failure, and the adapters treat it as one.
	ErrNoData = errors.New("aemet: no data for this request")
)

// defaultBase is the API root when the registry entry points at the portal.
const defaultBase = "https://opendata.aemet.es/opendata"

// envelope is the first-step response every AEMET product shares. The same
// shape carries the failures: estado is the status, descripcion the reason.
type envelope struct {
	Descripcion string `json:"descripcion"`
	Estado      int    `json:"estado"`
	Datos       string `json:"datos"`
	Metadatos   string `json:"metadatos"`
}

// client performs the two-step fetch for one registry entry.
type client struct {
	src    source.Source
	http   *httpx.Client
	apiKey string
}

// payload is the result of a completed two-step fetch: the bytes AEMET served,
// the response they arrived in, and where the field dictionary lives.
type payload struct {
	body        []byte
	contentType string
	fetchedAt   time.Time
	rawHash     string
	endpoint    string
	metadatos   string
}

// decodeUTF8 returns the payload as valid UTF-8. Several AEMET products are
// served as ISO-8859-15, and reading those as UTF-8 turns every accented
// Spanish place name into a replacement character.
func (p payload) decodeUTF8() string {
	return (&httpx.Response{Body: p.body, ContentType: p.contentType}).DecodeUTF8()
}

// fetch performs both steps of one AEMET request.
//
// The second step is a plain GET: the `datos` URL is a one-shot public link and
// carries no credential. It can also answer 200 with an envelope saying the
// link expired, which is why the body is inspected rather than trusted.
func (c *client) fetch(ctx context.Context, path string) (payload, error) {
	key := strings.TrimSpace(c.apiKey)
	if key == "" {
		return payload{}, fmt.Errorf("poll %s: %w", c.src.ID, ErrMissingAPIKey)
	}

	endpoint := c.base() + path
	resp, err := c.http.Do(ctx, httpx.Request{
		URL: endpoint,
		Headers: map[string]string{
			"api_key": key,
			"Accept":  "application/json",
		},
	})
	if err != nil {
		if resp != nil && resp.StatusCode != 0 {
			return payload{}, fmt.Errorf("poll %s: %w",
				c.src.ID, classify(resp.StatusCode, http.StatusText(resp.StatusCode)))
		}
		return payload{}, fmt.Errorf("%w: %s: %w", ErrAEMET, c.src.ID, err)
	}

	// AEMET answers a request with no credential at all with a 200 and an
	// empty body. Read as a successful empty poll that is indistinguishable
	// from a calm day, which is exactly the failure eye must never make.
	if len(bytes.TrimSpace(resp.Body)) == 0 {
		return payload{}, fmt.Errorf("%w: %s: the first step returned %d with an empty body",
			ErrAEMET, c.src.ID, resp.StatusCode)
	}

	var env envelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return payload{}, fmt.Errorf("%w: %s: the first step is not an envelope: %w", ErrAEMET, c.src.ID, err)
	}
	if err := classify(env.Estado, env.Descripcion); err != nil {
		return payload{}, fmt.Errorf("poll %s: %w", c.src.ID, err)
	}
	if env.Datos == "" {
		return payload{}, fmt.Errorf("%w: %s: the envelope carries no datos url", ErrAEMET, c.src.ID)
	}

	data, err := c.http.Get(ctx, env.Datos, httpx.Validators{})
	if err != nil {
		if data != nil && data.StatusCode != 0 {
			return payload{}, fmt.Errorf("poll %s datos: %w",
				c.src.ID, classify(data.StatusCode, http.StatusText(data.StatusCode)))
		}
		return payload{}, fmt.Errorf("%w: %s datos: %w", ErrAEMET, c.src.ID, err)
	}

	// An expired link answers 200 with the same envelope shape. Any other
	// product is either an archive or a JSON array, so a decodable envelope
	// carrying a status of its own is the link telling us it is gone.
	var second envelope
	if json.Unmarshal(data.Body, &second) == nil && second.Estado != 0 && second.Estado != http.StatusOK {
		return payload{}, fmt.Errorf("poll %s datos: %w",
			c.src.ID, classify(second.Estado, second.Descripcion))
	}

	sum := sha256.Sum256(data.Body)
	return payload{
		body:        data.Body,
		contentType: data.ContentType,
		fetchedAt:   data.FetchedAt,
		rawHash:     hex.EncodeToString(sum[:]),
		endpoint:    endpoint,
		metadatos:   env.Metadatos,
	}, nil
}

// base resolves the API root from the registry entry. The registry points at
// the portal; the API lives one path segment below it.
func (c *client) base() string {
	u := strings.TrimSuffix(c.src.Option("base", c.src.URL), "/")
	if u == "" {
		return defaultBase
	}
	if !strings.HasSuffix(u, "/opendata") {
		u += "/opendata"
	}
	return u
}

// ttl resolves the retention window for a sensor sample.
func (c *client) ttl(fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(c.src.Option("ttl", "")); err == nil && d > 0 {
		return d
	}
	return fallback
}

// classify maps an AEMET status to the sentinel that describes it.
func classify(status int, description string) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrUnauthorized, description)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNoData, description)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrRateLimited, description)
	default:
		return fmt.Errorf("%w: estado %d: %s", ErrAEMET, status, description)
	}
}

// truncate shortens text on a rune boundary.
func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}

// firstNonEmpty returns the first value that is not blank.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
