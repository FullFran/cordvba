// Package gtfsrt reads GTFS-Realtime feeds served as JSON.
//
// GTFS-RT is normally protobuf, and reading it that way would cost a
// dependency. RENFE publishes the same feeds as JSON alongside the binary form,
// so eye reads those instead: the same data, the same licence, and nothing
// added to the dependency budget.
package gtfsrt

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

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// ErrRealtime is returned when a feed cannot be used.
var ErrRealtime = errors.New("gtfs-rt")

// defaultTTL is how long a live position is kept. Movement data is short-lived
// by design; the schedule is what persists.
const defaultTTL = 24 * time.Hour

// feed is the GTFS-RT envelope as RENFE serialises it.
//
// Numeric fields arrive as JSON strings in this encoding, which is why the
// timestamps are strings and are parsed rather than assigned.
type feed struct {
	Header struct {
		Timestamp string `json:"timestamp"`
	} `json:"header"`
	Entity []entity `json:"entity"`
}

// entity is one update: a vehicle, a trip, or an alert.
type entity struct {
	ID      string `json:"id"`
	Vehicle *struct {
		Trip struct {
			TripID  string `json:"tripId"`
			RouteID string `json:"routeId"`
		} `json:"trip"`
		Position *struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Bearing   float64 `json:"bearing"`
			Speed     float64 `json:"speed"`
		} `json:"position"`
		CurrentStatus string `json:"currentStatus"`
		Timestamp     string `json:"timestamp"`
		StopID        string `json:"stopId"`
		Vehicle       struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"vehicle"`
	} `json:"vehicle"`
	TripUpdate *struct {
		Trip struct {
			TripID               string `json:"tripId"`
			RouteID              string `json:"routeId"`
			ScheduleRelationship string `json:"scheduleRelationship"`
		} `json:"trip"`
		StopTimeUpdate []struct {
			StopID  string `json:"stopId"`
			Arrival *struct {
				Delay int    `json:"delay"`
				Time  string `json:"time"`
			} `json:"arrival"`
			Departure *struct {
				Delay int    `json:"delay"`
				Time  string `json:"time"`
			} `json:"departure"`
		} `json:"stopTimeUpdate"`
		Timestamp string `json:"timestamp"`
	} `json:"tripUpdate"`
	Alert *struct {
		HeaderText      *translated `json:"headerText"`
		DescriptionText *translated `json:"descriptionText"`
		Cause           string      `json:"cause"`
		Effect          string      `json:"effect"`
		ActivePeriod    []struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"activePeriod"`
	} `json:"alert"`
}

// translated is GTFS-RT's multilingual string.
type translated struct {
	Translation []struct {
		Text     string `json:"text"`
		Language string `json:"language"`
	} `json:"translation"`
}

// text returns the first available translation.
func (t *translated) text() string {
	if t == nil {
		return ""
	}
	for _, tr := range t.Translation {
		if strings.TrimSpace(tr.Text) != "" {
			return strings.TrimSpace(tr.Text)
		}
	}
	return ""
}

// Provider reads one GTFS-RT JSON feed.
type Provider struct {
	src    source.Source
	client *httpx.Client
}

// New builds a provider for a registry entry. The `feed` option selects which
// of the three kinds this entry reads.
func New(src source.Source, client *httpx.Client) *Provider {
	return &Provider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// Poll fetches the feed and normalizes it.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	resp, err := p.client.Get(ctx, p.src.URL, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRealtime, err)
	}

	var doc feed
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		return nil, fmt.Errorf("%w: parse: %w", ErrRealtime, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: p.src.URL,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	records := make([]observation.Record, 0, len(doc.Entity))
	for _, e := range doc.Entity {
		rec, ok := p.toRecord(e, resp.FetchedAt, prov)
		if !ok {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// toRecord normalizes one entity, whichever kind it is.
func (p *Provider) toRecord(e entity, fetchedAt time.Time, prov observation.Provenance) (observation.Record, bool) {
	switch {
	case e.Vehicle != nil && e.Vehicle.Position != nil:
		return p.vehicleRecord(e, fetchedAt, prov)
	case e.TripUpdate != nil:
		return p.delayRecord(e, fetchedAt, prov)
	case e.Alert != nil:
		return p.alertRecord(e, fetchedAt, prov)
	default:
		return observation.Record{}, false
	}
}

// vehicleRecord maps a live train position.
func (p *Provider) vehicleRecord(e entity, fetchedAt time.Time, prov observation.Provenance) (observation.Record, bool) {
	v := e.Vehicle
	pos := observation.Point{Lat: v.Position.Latitude, Lon: v.Position.Longitude}
	if !pos.Valid() || (pos.Lat == 0 && pos.Lon == 0) {
		return observation.Record{}, false
	}

	observedAt := unixTime(v.Timestamp, fetchedAt)
	expires := fetchedAt.Add(p.ttl())

	label := firstNonEmpty(v.Vehicle.Label, v.Vehicle.ID, v.Trip.TripID, e.ID)
	payload, _ := json.Marshal(map[string]any{
		"trip_id": v.Trip.TripID, "route_id": v.Trip.RouteID,
		"status": v.CurrentStatus, "stop_id": v.StopID,
		"bearing": v.Position.Bearing, "speed": v.Position.Speed,
		"vehicle_id": v.Vehicle.ID,
	})

	return validated(observation.Record{
		ID: p.src.ID + ":" + e.ID, Source: p.src.ID, Kind: "vehicle_position", Topic: p.src.Topic,
		ObservedAt: observedAt, FetchedAt: fetchedAt, Position: &pos,
		Title:      "Tren " + label,
		Severity:   observation.SeverityNone,
		Confidence: 1, Quality: observation.QualityOfficial,
		DedupeKey: p.src.ID + ":" + e.ID + ":" + observedAt.UTC().Format(time.RFC3339),
		ExpiresAt: &expires, Payload: payload, Provenance: prov,
	})
}

// delayRecord maps a trip update, which is where a delay actually lives.
func (p *Provider) delayRecord(e entity, fetchedAt time.Time, prov observation.Provenance) (observation.Record, bool) {
	tu := e.TripUpdate

	worst := 0
	for _, stu := range tu.StopTimeUpdate {
		for _, ev := range []*struct {
			Delay int    `json:"delay"`
			Time  string `json:"time"`
		}{stu.Arrival, stu.Departure} {
			if ev != nil && abs(ev.Delay) > abs(worst) {
				worst = ev.Delay
			}
		}
	}

	cancelled := strings.EqualFold(tu.Trip.ScheduleRelationship, "CANCELED") ||
		strings.EqualFold(tu.Trip.ScheduleRelationship, "CANCELLED")

	// An on-time train is not news. Reporting every punctual service would
	// bury the late ones, which are the reason to read this feed at all.
	if !cancelled && abs(worst) < 60 {
		return observation.Record{}, false
	}

	title := fmt.Sprintf("Tren %s · %+d min", tu.Trip.TripID, worst/60)
	severity := observation.SeverityLow
	if cancelled {
		title = "Tren " + tu.Trip.TripID + " · CANCELADO"
		severity = observation.SeverityModerate
	} else if worst >= 900 {
		severity = observation.SeverityModerate
	}

	payload, _ := json.Marshal(map[string]any{
		"trip_id": tu.Trip.TripID, "route_id": tu.Trip.RouteID,
		"delay_seconds": worst, "cancelled": cancelled,
		"schedule_relationship": tu.Trip.ScheduleRelationship,
		"stops_affected":        len(tu.StopTimeUpdate),
	})

	observedAt := unixTime(tu.Timestamp, fetchedAt)
	expires := fetchedAt.Add(p.ttl())

	return validated(observation.Record{
		ID: p.src.ID + ":" + e.ID, Source: p.src.ID, Kind: "trip_update", Topic: p.src.Topic,
		ObservedAt: observedAt, FetchedAt: fetchedAt,
		Title:    title,
		Severity: severity, Confidence: 1, Quality: observation.QualityOfficial,
		DedupeKey: p.src.ID + ":" + e.ID + ":" + strconv.Itoa(worst),
		ExpiresAt: &expires, Payload: payload, Provenance: prov,
	})
}

// alertRecord maps a service alert.
func (p *Provider) alertRecord(e entity, fetchedAt time.Time, prov observation.Provenance) (observation.Record, bool) {
	a := e.Alert

	title := firstNonEmpty(a.HeaderText.text(), a.DescriptionText.text(), "Aviso de servicio")
	payload, _ := json.Marshal(map[string]any{
		"cause": a.Cause, "effect": a.Effect,
		"description": a.DescriptionText.text(),
	})

	rec := observation.Record{
		ID: p.src.ID + ":" + e.ID, Source: p.src.ID, Kind: "service_alert", Topic: p.src.Topic,
		ObservedAt: fetchedAt, FetchedAt: fetchedAt,
		Title:       title,
		Description: a.DescriptionText.text(),
		Severity:    observation.SeverityLow, Confidence: 1, Quality: observation.QualityOfficial,
		DedupeKey: p.src.ID + ":" + e.ID, Payload: payload, Provenance: prov,
	}

	if len(a.ActivePeriod) > 0 {
		if start := unixTime(a.ActivePeriod[0].Start, time.Time{}); !start.IsZero() {
			rec.ValidFrom = &start
			rec.ObservedAt = start
		}
	}
	return validated(rec)
}

// ttl resolves the retention window for movement records.
func (p *Provider) ttl() time.Duration {
	if d, err := time.ParseDuration(p.src.Option("ttl", "")); err == nil && d > 0 {
		return d
	}
	return defaultTTL
}

// validated returns a record only if it satisfies the domain invariants.
func validated(r observation.Record) (observation.Record, bool) {
	if err := r.Validate(); err != nil {
		return observation.Record{}, false
	}
	return r, true
}

// unixTime parses the string-encoded epoch seconds this feed uses.
func unixTime(s string, fallback time.Time) time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Unix(n, 0).UTC()
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

// abs returns the magnitude of an int.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var _ provider.Provider = (*Provider)(nil)
