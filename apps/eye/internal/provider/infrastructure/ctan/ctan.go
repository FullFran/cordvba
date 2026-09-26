// Package ctan reads the Red de Consorcios de Transporte de Andalucía API.
//
// It covers the metropolitan buses between Córdoba and its province, which no
// other source in the registry does: AUCORSA runs the city, the consortium runs
// everything that arrives from outside it, and the two are separate networks
// with separate operators and separate data.
//
// Two things make it worth an adapter of its own rather than another GTFS
// entry. Its stops carry coordinates, which AUCORSA's do not. And it publishes
// service notices with start and end dates — a real disruption feed, which the
// city network has no equivalent of.
package ctan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// ErrCTAN wraps every failure this adapter reports.
var ErrCTAN = errors.New("ctan")

// dateLayout is how the consortium writes a date. There is no time and no zone
// on a notice, so a start date means "from the beginning of that day in
// Córdoba" and an end date means "to the end of it" — see validity.
const dateLayout = "2006-01-02"

// madrid is the zone the consortium's dates are written in. Reading them as UTC
// would shift every notice by an hour or two and make a notice that starts
// today look like it started yesterday evening.
var madrid = mustLoadMadrid()

// mustLoadMadrid resolves Europe/Madrid, falling back to a fixed +01:00 on a
// system with no zone database — a container built FROM scratch, for instance.
// An hour of error in a notice's start date is bad; refusing to run is worse.
func mustLoadMadrid() *time.Location {
	if loc, err := time.LoadLocation("Europe/Madrid"); err == nil {
		return loc
	}
	return time.FixedZone("CET", 3600)
}

// Notices reads the consortium's service notices.
type Notices struct {
	src    source.Source
	client *httpx.Client
}

// NewNotices builds a notices provider for a registry entry.
func NewNotices(src source.Source, client *httpx.Client) *Notices {
	return &Notices{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Notices) Info() source.Source { return p.src }

// notice is one entry of /Consorcios/{id}/noticias.
type notice struct {
	ID         string `json:"idNoticia"`
	CategoryID string `json:"idCategoria"`
	Category   string `json:"categoria"`
	Title      string `json:"titulo"`
	Subtitle   string `json:"subTitulo"`
	Summary    string `json:"resumen"`
	StartDate  string `json:"fechaInicio"`
	EndDate    string `json:"fechafin"`
	FixedEnd   string `json:"fechafinFija"`
	Lines      string `json:"lineas"`
	IsNew      string `json:"novedad"`
}

// noticesAnswer is the envelope.
type noticesAnswer struct {
	Notices []notice `json:"noticias"`
}

// Poll fetches the notices and normalizes them.
func (p *Notices) Poll(ctx context.Context) ([]observation.Record, error) {
	resp, err := p.client.Get(ctx, p.src.URL, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCTAN, err)
	}

	var doc noticesAnswer
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		return nil, fmt.Errorf("%w: parse: %w", ErrCTAN, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: p.src.URL,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	records := make([]observation.Record, 0, len(doc.Notices))
	for _, n := range doc.Notices {
		rec, ok := p.toRecord(n, resp.FetchedAt, prov)
		if !ok {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// toRecord maps one notice.
func (p *Notices) toRecord(n notice, fetchedAt time.Time, prov observation.Provenance) (observation.Record, bool) {
	title := strings.TrimSpace(n.Title)
	if title == "" {
		return observation.Record{}, false
	}

	from, until := validity(n)

	// ObservedAt is when the notice takes effect, when it says. It is not
	// the fetch time: a notice published a fortnight ago for next Tuesday
	// is not news from this minute, and dating it now would put it at the
	// top of every timeline for the rest of the day.
	observedAt := fetchedAt
	if from != nil {
		observedAt = *from
	}

	payload, _ := json.Marshal(map[string]any{
		"category":    n.Category,
		"category_id": n.CategoryID,
		"lines":       splitLines(n.Lines),
		"subtitle":    strings.TrimSpace(n.Subtitle),
		"highlighted": n.IsNew == "1",
	})

	rec := observation.Record{
		ID:          p.src.ID + ":" + n.ID,
		Source:      p.src.ID,
		Kind:        "service_notice",
		Topic:       p.src.Topic,
		ObservedAt:  observedAt,
		FetchedAt:   fetchedAt,
		ValidFrom:   from,
		ValidUntil:  until,
		Title:       title,
		Description: strings.TrimSpace(firstNonEmpty(n.Summary, n.Subtitle)),
		Severity:    severityFor(n.CategoryID),
		Confidence:  1,
		// The consortium announcing its own service is the competent
		// authority saying what it is going to do.
		Quality: observation.QualityOfficial,
		// The consortium's own notice id. It is stable across polls, which
		// a fingerprint of edited free text would not be.
		LocalKey:   n.ID,
		DedupeKey:  p.src.ID + ":" + n.ID,
		Payload:    payload,
		Provenance: prov,
	}
	if err := rec.Validate(); err != nil {
		return observation.Record{}, false
	}
	return rec, true
}

// validity reads the period the notice applies to.
//
// The end date is pushed to the end of that day. The consortium writes
// "fechafin: 2026-09-13" meaning the notice still applies on the 13th, and
// reading it as midnight would expire it a day early.
func validity(n notice) (from, until *time.Time) {
	if start, err := time.ParseInLocation(dateLayout, strings.TrimSpace(n.StartDate), madrid); err == nil {
		utc := start.UTC()
		from = &utc
	}
	end := firstNonEmpty(n.EndDate, n.FixedEnd)
	if finish, err := time.ParseInLocation(dateLayout, strings.TrimSpace(end), madrid); err == nil {
		utc := finish.Add(24*time.Hour - time.Second).UTC()
		until = &utc
	}
	return from, until
}

// severityFor ranks a notice by the consortium's own category.
//
// The categories are theirs, and the ranking is eye's reading of what each one
// costs a passenger standing at a stop. A cancelled stop and a diverted route
// change whether the bus comes; a special timetable changes when; a procurement
// announcement changes nothing for anyone waiting.
func severityFor(categoryID string) observation.Severity {
	switch categoryID {
	case "1": // Ruta alternativa
		return observation.SeverityModerate
	case "2": // Parada no operativa
		return observation.SeverityModerate
	case "5": // Horario especial
		return observation.SeverityLow
	case "3": // Concursos
		return observation.SeverityInfo
	default: // 4, General, and anything they add later
		return observation.SeverityInfo
	}
}

// splitLines turns "10, 11, 12, " into a list, dropping the trailing blank the
// API always leaves behind.
func splitLines(s string) []string {
	out := make([]string, 0, 8)
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// Stops reads the consortium's stop inventory, which carries coordinates.
type Stops struct {
	src    source.Source
	client *httpx.Client
}

// NewStops builds a stop-inventory provider for a registry entry.
func NewStops(src source.Source, client *httpx.Client) *Stops {
	return &Stops{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Stops) Info() source.Source { return p.src }

// stop is one entry of /Consorcios/{id}/paradas.
type stop struct {
	ID           string `json:"idParada"`
	Name         string `json:"nombre"`
	Latitude     string `json:"latitud"`
	Longitude    string `json:"longitud"`
	Zone         string `json:"idZona"`
	Municipality string `json:"municipio"`
	Locality     string `json:"nucleo"`
}

// stopsAnswer is the envelope.
type stopsAnswer struct {
	Stops []stop `json:"paradas"`
}

// Poll reports the inventory refresh as a single record.
func (p *Stops) Poll(ctx context.Context) ([]observation.Record, error) {
	entities, err := p.Entities(ctx)
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, nil
	}

	fetchedAt := entities[0].LastSeen
	payload, _ := json.Marshal(map[string]any{"kind": p.kind(), "count": len(entities)})

	rec := observation.Record{
		ID:         p.src.ID + ":inventory",
		Source:     p.src.ID,
		Kind:       "inventory_refresh",
		Topic:      p.src.Topic,
		ObservedAt: fetchedAt,
		FetchedAt:  fetchedAt,
		Title:      fmt.Sprintf("%d metropolitan bus stops in the Cordoba consortium", len(entities)),
		Severity:   observation.SeverityInfo,
		Confidence: 1,
		Quality:    observation.QualityOfficial,
		LocalKey:   p.src.ID + ":inventory",
		DedupeKey:  p.src.ID + ":inventory:" + fetchedAt.UTC().Format(time.RFC3339),
		Payload:    payload,
		Provenance: entities[0].Provenance,
	}
	if err := rec.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCTAN, err)
	}
	return []observation.Record{rec}, nil
}

// Entities fetches the stop inventory.
func (p *Stops) Entities(ctx context.Context) ([]observation.Entity, error) {
	resp, err := p.client.Get(ctx, p.src.URL, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCTAN, err)
	}

	var doc stopsAnswer
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		return nil, fmt.Errorf("%w: parse: %w", ErrCTAN, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: p.src.URL,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	out := make([]observation.Entity, 0, len(doc.Stops))
	for _, s := range doc.Stops {
		lat, latErr := strconv.ParseFloat(strings.TrimSpace(s.Latitude), 64)
		lon, lonErr := strconv.ParseFloat(strings.TrimSpace(s.Longitude), 64)
		if latErr != nil || lonErr != nil {
			continue
		}
		point := observation.Point{Lat: lat, Lon: lon}
		if !point.Valid() {
			continue
		}

		payload, _ := json.Marshal(map[string]any{
			"zone": s.Zone, "municipality": s.Municipality, "locality": s.Locality,
		})

		ent := observation.Entity{
			ID:         p.src.ID + ":" + s.ID,
			Source:     p.src.ID,
			Kind:       p.kind(),
			Topic:      p.src.Topic,
			Title:      strings.TrimSpace(s.Name),
			Position:   &point,
			FirstSeen:  resp.FetchedAt,
			LastSeen:   resp.FetchedAt,
			Payload:    payload,
			Provenance: prov,
		}
		if err := ent.Validate(); err != nil {
			continue
		}
		out = append(out, ent)
	}
	return out, nil
}

// kind is the entity kind the registry asked for.
func (p *Stops) kind() string { return p.src.Option("kind", "bus_stop") }

// firstNonEmpty returns the first value that is not blank.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Interface checks.
var (
	_ provider.Provider       = (*Notices)(nil)
	_ provider.Provider       = (*Stops)(nil)
	_ provider.EntityProvider = (*Stops)(nil)
)
