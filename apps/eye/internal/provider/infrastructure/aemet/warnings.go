package aemet

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// warningsPath is the documented endpoint for the last warning bulletin of an
// area.
const warningsPath = "/api/avisos_cap/ultimoelaborado/area/"

// defaultArea is Andalucia, the autonomous community Cordoba belongs to.
const defaultArea = "61"

// maxCAPMembers caps how many messages one archive may contain. A bulletin for
// a community holds tens of files; a number far past that is a wire format
// change, not a busy day.
const maxCAPMembers = 2000

// WarningsProvider reads AEMET's adverse-phenomena warnings for one area.
//
// The payload is a gtar archive of CAP 1.2 messages — one per warning zone and
// phenomenon — as AEMET's own product metadata states: "application/x-gtar
// (contiene ficheros CAP v1.2)".
//
// Messages with no warning in force are published too, at level verde, and eye
// keeps them. A source that only ever says something when the news is bad
// cannot tell the difference between calm and silence.
type WarningsProvider struct {
	client client
}

// NewWarnings builds the warnings adapter for a registry entry.
func NewWarnings(src source.Source, c *httpx.Client, apiKey string) *WarningsProvider {
	return &WarningsProvider{client: client{src: src, http: c, apiKey: apiKey}}
}

// Info implements provider.Provider.
func (p *WarningsProvider) Info() source.Source { return p.client.src }

// Poll fetches the current bulletin and returns one record per zone and
// phenomenon.
//
// A 404 is a successful, empty poll: AEMET uses it both for "nothing to report"
// and for a datos link that expired between the two steps.
func (p *WarningsProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	pay, err := p.client.fetch(ctx, warningsPath+p.area())
	switch {
	case errors.Is(err, ErrNoData):
		return nil, nil
	case err != nil:
		return nil, err
	}

	messages, err := unpackCAP(pay.body)
	if err != nil {
		return nil, fmt.Errorf("poll %s: %w", p.client.src.ID, err)
	}

	records := make([]observation.Record, 0, len(messages))
	for _, body := range messages {
		alert, err := ParseCAP(body)
		if err != nil {
			// One unreadable message must not discard the bulletin.
			continue
		}
		records = append(records, p.toRecords(alert, pay)...)
	}
	return records, nil
}

// area is the AEMET area code to ask for.
func (p *WarningsProvider) area() string { return p.client.src.Option("area", defaultArea) }

// language is the CAP info block to normalize from.
func (p *WarningsProvider) language() string { return p.client.src.Option("language", "es-ES") }

// toRecords turns one CAP message into one record per zone it covers.
func (p *WarningsProvider) toRecords(alert Alert, pay payload) []observation.Record {
	info, ok := alert.Localized(p.language())
	if !ok {
		return nil
	}

	observedAt := parseCAPTime(alert.Sent)
	if observedAt.IsZero() {
		observedAt = pay.fetchedAt
	}
	code, name := info.Phenomenon()
	severity := severityOf(info)

	out := make([]observation.Record, 0, len(info.Areas))
	for _, area := range info.Areas {
		zone := area.Zone()
		if zone == "" {
			// Without the zone code there is nothing stable to key the
			// warning to, and a warning that cannot be followed across
			// updates is worse than none.
			continue
		}

		rec := observation.Record{
			ID:          p.client.src.ID + ":" + alert.Identifier + ":" + zone,
			Source:      p.client.src.ID,
			Kind:        "weather_warning",
			Topic:       p.client.src.Topic,
			ObservedAt:  observedAt,
			FetchedAt:   pay.fetchedAt,
			Title:       warningTitle(info, area),
			Description: truncate(info.Description, 400),
			Severity:    severity,
			// AEMET said it; the statement itself is certain. What is
			// probabilistic is the weather, and CAP's own certainty
			// and probability stay in the payload where they belong.
			Confidence: 1,
			Quality:    observation.QualityOfficial,
			// The zone and the phenomenon are the thing being warned
			// about. The message identifier changes on every update,
			// so identity taken from it would report each update as a
			// brand new warning.
			LocalKey:  zone + ":" + code,
			DedupeKey: alert.Identifier + ":" + zone,
			Provenance: observation.Provenance{
				Publisher: p.client.src.Authority,
				SourceURL: pay.endpoint,
				License:   p.client.src.License,
				FetchedAt: pay.fetchedAt,
				RawHash:   pay.rawHash,
			},
		}

		if onset := parseCAPTime(info.Onset); !onset.IsZero() {
			rec.ValidFrom = &onset
		}
		if expires := parseCAPTime(info.Expires); !expires.IsZero() {
			rec.ValidUntil = &expires
		}
		// ExpiresAt is deliberately unset. It drives deletion, and an
		// official warning that has passed is still the record of what
		// was said; ValidUntil is what says it no longer applies.

		if geometry, lat, lon, ok := polygonGeoJSON(area.Polygons); ok {
			pos := observation.Point{Lat: lat, Lon: lon}
			if pos.Valid() {
				rec.Position = &pos
				rec.Geometry = geometry
			}
		}

		rec.Payload = warningPayload(alert, info, area, code, name)

		if err := rec.Validate(); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// warningPayload keeps everything CAP said that the domain does not model.
//
// The colour, the phenomenon, the threshold and the probability are what a
// reader needs to judge the warning, and none of them fit the record's fields
// without flattening one into another.
func warningPayload(alert Alert, info Info, area Area, code, name string) []byte {
	fields := map[string]any{
		"cap_identifier": alert.Identifier,
		"cap_severity":   info.Severity,
		"msg_type":       alert.MsgType,
		"level":          info.Level(),
		"phenomenon":     code,
		"zone":           area.Zone(),
		"zone_name":      area.Desc,
		"parameter":      info.Parameter(paramParameter),
	}
	optional := map[string]string{
		"phenomenon_name": name,
		"probability":     info.Parameter(paramProbability),
		"certainty":       info.Certainty,
		"urgency":         info.Urgency,
		"instruction":     truncate(info.Instruction, 400),
		"web":             info.Web,
		"updates":         alert.References,
	}
	for key, value := range optional {
		if strings.TrimSpace(value) != "" {
			fields[key] = value
		}
	}

	raw, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return raw
}

// warningTitle names the warning and the zone it applies to. AEMET's headline
// already carries both for a real warning; the no-warning bulletins cover
// several zones with one headline, so the zone has to be added back.
func warningTitle(info Info, area Area) string {
	title := strings.TrimSpace(firstNonEmpty(info.Headline, info.Event))
	if area.Desc == "" || strings.Contains(title, area.Desc) {
		return title
	}
	return strings.TrimSuffix(title, ".") + ". " + area.Desc
}

// severityOf maps an AEMET warning level onto eye's cross-source scale.
//
// The Meteoalerta colour is the authority's own ladder and is preferred; CAP's
// severity is the fallback, and the plan defines them as the same four steps:
// verde/Minor, amarillo/Moderate, naranja/Severe, rojo/Extreme.
func severityOf(info Info) observation.Severity {
	switch info.Level() {
	case "rojo":
		return observation.SeverityCritical
	case "naranja":
		return observation.SeverityHigh
	case "amarillo":
		return observation.SeverityModerate
	case "verde":
		return observation.SeverityInfo
	}

	switch strings.ToLower(strings.TrimSpace(info.Severity)) {
	case "extreme":
		return observation.SeverityCritical
	case "severe":
		return observation.SeverityHigh
	case "moderate":
		return observation.SeverityModerate
	default:
		return observation.SeverityInfo
	}
}

// unpackCAP reads the gzipped tar AEMET serves and returns each CAP message.
func unpackCAP(body []byte) ([][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: the warning bulletin is not a gzip archive: %w", ErrAEMET, err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var messages [][]byte

	for len(messages) < maxCAPMembers {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: the warning bulletin is not a tar archive: %w", ErrAEMET, err)
		}
		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(strings.ToLower(header.Name), ".xml") {
			continue
		}
		member, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("%w: reading %s: %w", ErrAEMET, header.Name, err)
		}
		messages = append(messages, member)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: the warning bulletin holds no CAP messages", ErrAEMET)
	}
	return messages, nil
}

var _ provider.Provider = (*WarningsProvider)(nil)
