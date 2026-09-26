// Package adsblol adapts the adsb.lol public API into aircraft observations
// over a configured viewport.
//
// Aircraft positions are the part of eye that could quietly become a movement
// archive. They do not: every record carries ExpiresAt, and the default is the
// short end of the retention window.
package adsblol

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

// ErrADSB is returned when the API answers something eye cannot use.
var ErrADSB = errors.New("adsblol")

// defaultTTL is how long a raw position is kept. Short by design.
const defaultTTL = 72 * time.Hour

// apiResponse is the subset of the v2 payload eye reads. The API returns more;
// the rest stays in the payload rather than being invented into the domain.
type apiResponse struct {
	Aircraft []aircraft `json:"ac"`
	Now      float64    `json:"now"`
}

// aircraft is one state vector.
type aircraft struct {
	Hex       string          `json:"hex"`
	Type      string          `json:"type"`
	Flight    string          `json:"flight"`
	Reg       string          `json:"r"`
	AcType    string          `json:"t"`
	AltBaro   json.RawMessage `json:"alt_baro"`
	GroundSp  float64         `json:"gs"`
	Track     float64         `json:"track"`
	Lat       float64         `json:"lat"`
	Lon       float64         `json:"lon"`
	Seen      float64         `json:"seen"`
	SeenPos   float64         `json:"seen_pos"`
	Emergency string          `json:"emergency"`
}

// Provider polls a circular viewport around a configured centre.
type Provider struct {
	src    source.Source
	client *httpx.Client
}

// New builds a provider from a registry entry.
func New(src source.Source, client *httpx.Client) *Provider {
	return &Provider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// Poll fetches the current aircraft in the viewport.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	lat, err := floatOption(p.src, "lat")
	if err != nil {
		return nil, err
	}
	lon, err := floatOption(p.src, "lon")
	if err != nil {
		return nil, err
	}
	radius := p.src.Option("radius_nm", "60")

	endpoint := fmt.Sprintf("%s/v2/lat/%s/lon/%s/dist/%s",
		strings.TrimSuffix(p.src.URL, "/"),
		strconv.FormatFloat(lat, 'f', -1, 64),
		strconv.FormatFloat(lon, 'f', -1, 64),
		radius,
	)

	resp, err := p.client.Get(ctx, endpoint, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrADSB, err)
	}

	var payload apiResponse
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return nil, fmt.Errorf("%w: parse: %w", ErrADSB, err)
	}

	sum := sha256.Sum256(resp.Body)
	rawHash := hex.EncodeToString(sum[:])
	ttl := p.ttl()

	records := make([]observation.Record, 0, len(payload.Aircraft))
	for _, ac := range payload.Aircraft {
		rec, ok := p.toRecord(ac, resp.FetchedAt, rawHash, ttl)
		if !ok {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// toRecord normalizes one state vector.
//
// The API returns a positional-free JSON object, but the field names are terse
// enough to be their own trap. They are mapped to named fields here so that
// nobody has to wonder in six months whether "t" was type or track.
func (p *Provider) toRecord(ac aircraft, fetchedAt time.Time, rawHash string, ttl time.Duration) (observation.Record, bool) {
	pos := observation.Point{Lat: ac.Lat, Lon: ac.Lon}
	if (ac.Lat == 0 && ac.Lon == 0) || !pos.Valid() {
		return observation.Record{}, false
	}

	// seen_pos is how many seconds ago the position was received. That is
	// the observation time; our fetch time is not.
	observedAt := fetchedAt.Add(-time.Duration(ac.SeenPos * float64(time.Second)))
	expires := fetchedAt.Add(ttl)

	callsign := strings.TrimSpace(ac.Flight)
	label := firstNonEmpty(callsign, ac.Reg, ac.Hex)

	severity := observation.SeverityNone
	if ac.Emergency != "" && ac.Emergency != "none" {
		severity = observation.SeverityHigh
	}

	payload, _ := json.Marshal(map[string]any{
		"hex":            ac.Hex,
		"callsign":       callsign,
		"registration":   ac.Reg,
		"aircraft_type":  ac.AcType,
		"altitude_baro":  string(ac.AltBaro),
		"ground_speed":   ac.GroundSp,
		"track":          ac.Track,
		"emergency":      ac.Emergency,
		"position_age_s": ac.SeenPos,
	})

	rec := observation.Record{
		ID:         p.src.ID + ":" + ac.Hex,
		Source:     p.src.ID,
		Kind:       "aircraft_position",
		Topic:      p.src.Topic,
		ObservedAt: observedAt,
		FetchedAt:  fetchedAt,
		Position:   &pos,
		Title:      label,
		Severity:   severity,
		Confidence: 1,
		Quality:    observation.QualityPreliminary,
		// The hex is the aircraft. Its position is what changes, so
		// identity built from position would report one aircraft
		// vanishing and another appearing on every single poll.
		LocalKey:  ac.Hex,
		DedupeKey: p.src.ID + ":" + ac.Hex + ":" + observedAt.UTC().Format(time.RFC3339),
		ExpiresAt: &expires,
		Payload:   payload,
		Provenance: observation.Provenance{
			Publisher: p.src.Authority,
			SourceURL: p.src.URL,
			License:   p.src.License,
			FetchedAt: fetchedAt,
			RawHash:   rawHash,
		},
	}

	if err := rec.Validate(); err != nil {
		return observation.Record{}, false
	}
	return rec, true
}

// ttl resolves the retention window for raw positions.
func (p *Provider) ttl() time.Duration {
	if d, err := time.ParseDuration(p.src.Option("ttl", "")); err == nil && d > 0 {
		return d
	}
	return defaultTTL
}

// floatOption reads a required numeric option.
func floatOption(src source.Source, key string) (float64, error) {
	v, err := strconv.ParseFloat(src.Option(key, ""), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: source %s needs a numeric %q option", ErrADSB, src.ID, key)
	}
	return v, nil
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

var _ provider.Provider = (*Provider)(nil)
