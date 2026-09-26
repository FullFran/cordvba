// Package firms adapts NASA FIRMS active-fire detections over a bounding box.
//
// FIRMS is orbital: the latency between a fire burning and a pixel being
// published is the satellite's, not the network's. Polling faster than the
// overpass buys rate limits and nothing else.
//
// The product's own `confidence` column is metadata about the detection
// algorithm — "low", "nominal", "high" — and it stays in the payload. Copying
// it into Record.Confidence would relabel eye's confidence in an observation as
// NASA's confidence in a pixel.
package firms

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// Errors returned by the adapter.
var (
	// ErrFIRMS wraps anything FIRMS answered that eye cannot use.
	ErrFIRMS = errors.New("firms")

	// ErrMissingMapKey reports that this build has no FIRMS credential. It
	// names the environment variable: a source that silently returns no
	// detections is indistinguishable from a province that is not burning.
	ErrMissingMapKey = errors.New("firms: no map key; set FIRMS_MAP_KEY (free at https://firms.modaps.eosdis.nasa.gov/api/area/)")

	// ErrRejectedMapKey reports a key FIRMS would not accept. It answers
	// this as the plain text "Invalid MAP_KEY.", sometimes with a 400 and
	// sometimes with a 200.
	ErrRejectedMapKey = errors.New("firms: the map key was rejected")
)

// Defaults for a registry entry that names only a viewport.
const (
	defaultProduct = "VIIRS_SNPP_NRT"
	defaultDays    = "1"
	// defaultTTL is how long a detection is kept. Near-real-time pixels are
	// superseded by standard processing; eye keeps a fire season, not an
	// archive.
	defaultTTL = 30 * 24 * time.Hour
)

// invalidKeyMarker is what FIRMS writes instead of a CSV when the key is wrong.
const invalidKeyMarker = "Invalid MAP_KEY"

// Provider reads active-fire detections for one area.
type Provider struct {
	src    source.Source
	client *httpx.Client
	mapKey string
}

// New builds the adapter for a registry entry.
func New(src source.Source, c *httpx.Client, mapKey string) *Provider {
	return &Provider{src: src, client: c, mapKey: mapKey}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// Poll fetches the current detections and returns one record per pixel.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	endpoint, err := p.endpoint()
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Get(ctx, endpoint, httpx.Validators{})
	if err != nil {
		if resp != nil && (bytes.Contains(resp.Body, []byte(invalidKeyMarker)) ||
			resp.StatusCode == http.StatusBadRequest) {
			// FIRMS rejects a key with "Invalid MAP_KEY.", under a
			// 400 or, sometimes, under a 200.
			return nil, fmt.Errorf("poll %s: %w", p.src.ID, ErrRejectedMapKey)
		}
		// The key is a path segment of the URL, and the transport quotes
		// the URL back in its errors. Unredacted, that error would be
		// written into the store as this source's last known failure.
		return nil, fmt.Errorf("%w: %s: %w", ErrFIRMS, p.src.ID, redacted{err: err, secret: strings.TrimSpace(p.mapKey)})
	}

	if bytes.Contains(resp.Body, []byte(invalidKeyMarker)) {
		return nil, fmt.Errorf("poll %s: %w", p.src.ID, ErrRejectedMapKey)
	}

	rows, err := parseCSV(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("poll %s: %w", p.src.ID, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		// The public endpoint without the credential. A source URL that
		// carries a key is a key in the store, in the API and on screen.
		SourceURL: p.publicURL(),
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	ttl := p.ttl()
	records := make([]observation.Record, 0, len(rows))
	for _, row := range rows {
		if rec, ok := p.toRecord(row, resp.FetchedAt, ttl, prov); ok {
			records = append(records, rec)
		}
	}
	return records, nil
}

// withinDeclaredBox keeps a detection only if it falls inside the bounding box
// the registry declared.
//
// The area API filters server-side, so this is a no-op there. The NRT archive
// does not: it is one continental file, and without this a source declaring
// Cordoba would quietly fill the store with fires in Poland. The filter runs in
// both modes on purpose — a source that says it watches a box must never hold a
// record outside it, whichever endpoint it happened to read.
//
// A malformed or absent box keeps everything rather than dropping everything.
// Poll already refuses a source with no bbox, so reaching here without one means
// the box was unparseable, and silently discarding every detection is the worse
// of the two failures.
func (p *Provider) withinDeclaredBox(pos observation.Point) bool {
	parts := strings.Split(strings.TrimSpace(p.src.Option("bbox", "")), ",")
	if len(parts) != 4 {
		return true
	}

	west, okW := parseFloat(parts[0])
	south, okS := parseFloat(parts[1])
	east, okE := parseFloat(parts[2])
	north, okN := parseFloat(parts[3])
	if !okW || !okS || !okE || !okN {
		return true
	}
	return pos.Lon >= west && pos.Lon <= east && pos.Lat >= south && pos.Lat <= north
}

// toRecord normalizes one detection. A row eye cannot place in space or time is
// dropped rather than stored at a made-up position.
func (p *Provider) toRecord(row map[string]string, fetchedAt time.Time, ttl time.Duration,
	prov observation.Provenance,
) (observation.Record, bool) {
	lat, okLat := parseFloat(row["latitude"])
	lon, okLon := parseFloat(row["longitude"])
	if !okLat || !okLon {
		return observation.Record{}, false
	}
	pos := observation.Point{Lat: lat, Lon: lon}
	if !pos.Valid() {
		return observation.Record{}, false
	}
	if !p.withinDeclaredBox(pos) {
		return observation.Record{}, false
	}

	observedAt, ok := parseAcquisition(row["acq_date"], row["acq_time"])
	if !ok {
		return observation.Record{}, false
	}

	frp, hasFRP := parseFloat(row["frp"])
	expires := observedAt.Add(ttl)

	// The pixel is the observation; there is no persistent thing behind it
	// to key to, so identity is the pixel's own place and moment.
	local := fmt.Sprintf("%s:%s:%s:%s", row["latitude"], row["longitude"], row["acq_date"], row["acq_time"])

	payload, err := json.Marshal(row)
	if err != nil {
		return observation.Record{}, false
	}

	rec := observation.Record{
		ID:         p.src.ID + ":" + local,
		Source:     p.src.ID,
		Kind:       "fire_detection",
		Topic:      p.src.Topic,
		ObservedAt: observedAt,
		FetchedAt:  fetchedAt,
		Position:   &pos,
		Title:      detectionTitle(p.product(), frp, hasFRP),
		Severity:   severityOf(frp, hasFRP),
		// eye's own confidence that FIRMS reported this pixel, which is
		// total. The algorithm's grade is in the payload as `confidence`.
		Confidence: 1,
		Quality:    observation.QualityPreliminary,
		LocalKey:   local,
		DedupeKey:  p.src.ID + ":" + local,
		ExpiresAt:  &expires,
		Payload:    payload,
		Provenance: prov,
	}

	if err := rec.Validate(); err != nil {
		return observation.Record{}, false
	}
	return rec, true
}

// archive is the keyless NRT file the registry named, if it named one.
//
// NASA publishes the same detections twice. The area API filters server-side
// and needs a credential; the NRT archive is a continental CSV that needs none.
// eye prefers the archive when the registry declares it, because a source a
// second person cannot reproduce without their own key is a source this project
// cannot honestly say it reads — see the reproducibility argument in ADR-0008.
func (p *Provider) archive() string {
	return strings.TrimSpace(p.src.Option("archive", ""))
}

// endpoint builds the request, from the archive when one is declared and from
// the documented area API otherwise.
func (p *Provider) endpoint() (string, error) {
	// Order matters here, and it is about which error the operator reads
	// first. On the area path a missing key is the actionable problem, so it
	// is reported before anything else; the archive path has no key to be
	// missing, and its only requirement is the box.
	if archive := p.archive(); archive != "" {
		if err := p.requireBBox(); err != nil {
			return "", err
		}
		return archive, nil
	}

	key := strings.TrimSpace(p.mapKey)
	if key == "" {
		return "", fmt.Errorf("poll %s: %w", p.src.ID, ErrMissingMapKey)
	}
	if err := p.requireBBox(); err != nil {
		return "", err
	}
	return p.base() + "csv/" + key + "/" + p.product() + "/" + p.src.Option("bbox", "") + "/" + p.days(), nil
}

// requireBBox refuses a source that names no bounding box.
//
// It is required rather than defaulted in both modes: the area API would
// otherwise be asked for the planet, and the archive already contains a
// continent that has to be narrowed here.
func (p *Provider) requireBBox() error {
	if strings.TrimSpace(p.src.Option("bbox", "")) == "" {
		return fmt.Errorf("%w: source %s needs a \"bbox\" option (west,south,east,north)",
			ErrFIRMS, p.src.ID)
	}
	return nil
}

// publicURL is the same request with the credential replaced by its placeholder,
// so provenance can be read and replayed without leaking the key.
func (p *Provider) publicURL() string {
	// The archive carries no credential, so it IS the public URL. Writing a
	// [MAP_KEY] placeholder here would describe a request eye never made.
	if archive := p.archive(); archive != "" {
		return archive
	}
	return p.base() + "csv/[MAP_KEY]/" + p.product() + "/" +
		strings.TrimSpace(p.src.Option("bbox", "")) + "/" + p.days()
}

// base is the area API root, ending in a slash.
func (p *Provider) base() string {
	u := strings.TrimSuffix(strings.TrimSpace(p.src.URL), "/")
	if !strings.HasSuffix(u, "/api/area") {
		u += "/api/area"
	}
	return u + "/"
}

// product is the FIRMS source to read.
func (p *Provider) product() string { return p.src.Option("source", defaultProduct) }

// days is the day range to request, 1 to 5.
func (p *Provider) days() string { return p.src.Option("days", defaultDays) }

// ttl resolves the retention window for a detection.
func (p *Provider) ttl() time.Duration {
	if d, err := time.ParseDuration(p.src.Option("ttl", "")); err == nil && d > 0 {
		return d
	}
	return defaultTTL
}

// parseCSV reads the response by column name.
//
// The area API carries an `instrument` column the public archive does not, and
// FIRMS has added columns before. Reading by position would turn the next
// addition into silently mislabelled data.
func parseCSV(body []byte) ([]map[string]string, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("%w: the response was empty", ErrFIRMS)
	}

	r := csv.NewReader(bytes.NewReader(body))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: unreadable header: %w", ErrFIRMS, err)
	}
	for i := range header {
		header[i] = strings.ToLower(strings.TrimSpace(header[i]))
	}
	if !contains(header, "latitude") || !contains(header, "longitude") {
		return nil, fmt.Errorf("%w: the response is not a detection table (columns: %v)",
			ErrFIRMS, header)
	}

	var rows []map[string]string
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// One ragged line must not discard the rest of an
			// overpass.
			continue
		}
		row := make(map[string]string, len(header))
		for i, name := range header {
			if i < len(record) {
				row[name] = strings.TrimSpace(record[i])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// severityOf ranks a detection by its fire radiative power in megawatts, which
// is the only quantity in the product that says how much is burning.
//
// A missing value is not treated as nothing: the pixel still tripped the
// detector, so it stays on the scale at its lowest step.
func severityOf(frp float64, ok bool) observation.Severity {
	switch {
	case !ok:
		return observation.SeverityLow
	case frp >= 500:
		return observation.SeverityCritical
	case frp >= 100:
		return observation.SeverityHigh
	case frp >= 10:
		return observation.SeverityModerate
	default:
		return observation.SeverityLow
	}
}

// detectionTitle names the detection without pretending it is a confirmed fire.
func detectionTitle(product string, frp float64, ok bool) string {
	if !ok {
		return "Thermal anomaly (" + product + ")"
	}
	return fmt.Sprintf("Thermal anomaly, %.1f MW (%s)", frp, product)
}

// parseAcquisition combines the two columns FIRMS splits the overpass across:
// acq_date is a date and acq_time is HHMM in UTC.
func parseAcquisition(date, clock string) (time.Time, bool) {
	if date == "" || clock == "" {
		return time.Time{}, false
	}
	for len(clock) < 4 {
		clock = "0" + clock
	}
	t, err := time.Parse("2006-01-02 1504", date+" "+clock)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// parseFloat reads a numeric column, reporting whether it held one.
func parseFloat(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// contains reports whether a column name is present.
func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// redacted hides a credential that a lower layer quoted back inside its own
// error message, while keeping the error chain intact for errors.Is.
type redacted struct {
	err    error
	secret string
}

// Error returns the underlying message with the credential removed.
func (r redacted) Error() string {
	if r.secret == "" {
		return r.err.Error()
	}
	return strings.ReplaceAll(r.err.Error(), r.secret, "[MAP_KEY]")
}

// Unwrap keeps errors.Is and errors.As working through the redaction.
func (r redacted) Unwrap() error { return r.err }

var _ provider.Provider = (*Provider)(nil)
