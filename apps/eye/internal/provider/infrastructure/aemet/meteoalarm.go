package aemet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// MeteoAlarmProvider reads AEMET's warnings through MeteoAlarm's public feed.
//
// It exists because AEMET's own OpenData API needs a personal API key, obtained
// by email registration, and a source that only works with the operator's own
// credential is one nobody else running eye can reproduce. MeteoAlarm — the
// EUMETNET early-warning service — relays the same warnings with no credential
// at all, and the CAP documents behind the feed name their sender as
// "AEMET. Agencia Estatal de Meteorología". Same authority, same warnings, an
// endpoint anybody can fetch.
//
// The relay is not hidden. Provenance points at MeteoAlarm's URL, because that
// is what eye actually read, and the payload records the issuing authority
// separately from the publisher that carried it.
type MeteoAlarmProvider struct {
	src    source.Source
	client *httpx.Client
}

// NewMeteoAlarm builds a keyless warnings provider for a registry entry.
func NewMeteoAlarm(src source.Source, c *httpx.Client) *MeteoAlarmProvider {
	return &MeteoAlarmProvider{src: src, client: c}
}

// Info implements provider.Provider.
func (p *MeteoAlarmProvider) Info() source.Source { return p.src }

// atomFeed is MeteoAlarm's Atom envelope. Every CAP field eye needs is inline
// on the entry, so one fetch is the whole answer — following the per-entry CAP
// link would be 210 extra requests for data already in hand.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Rights  string      `xml:"rights"`
	Entries []atomEntry `xml:"entry"`
}

// atomEntry is one warning.
type atomEntry struct {
	Title      string `xml:"title"`
	ID         string `xml:"id"`
	Updated    string `xml:"updated"`
	Identifier string `xml:"identifier"`
	AreaDesc   string `xml:"areaDesc"`
	Event      string `xml:"event"`
	Sent       string `xml:"sent"`
	Effective  string `xml:"effective"`
	Onset      string `xml:"onset"`
	Expires    string `xml:"expires"`
	Severity   string `xml:"severity"`
	Certainty  string `xml:"certainty"`
	Urgency    string `xml:"urgency"`
	Status     string `xml:"status"`
	MsgType    string `xml:"message_type"`
	Geocodes   []struct {
		Name  string `xml:"valueName"`
		Value string `xml:"value"`
	} `xml:"geocode"`
}

// zone returns the EMMA_ID the warning applies to, which is the stable
// identifier for a warning area across renamings of its description.
func (e atomEntry) zone() string {
	for _, g := range e.Geocodes {
		if strings.EqualFold(strings.TrimSpace(g.Name), "EMMA_ID") {
			return strings.TrimSpace(g.Value)
		}
	}
	return ""
}

// Poll fetches the feed and normalizes every warning in it.
//
// No geographic filter is applied at ingestion, for the same reason the DGT
// incident feed applies none: the publisher publishes Spain, eye filters at
// query time, and a warning dropped at ingestion is one no later query can
// recover. Missing a red warning because a zone was renamed is not a trade this
// source is willing to make. A `zones` option exists for anyone who wants it.
func (p *MeteoAlarmProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	resp, err := p.client.Get(ctx, p.src.URL, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("poll %s: %w", p.src.ID, err)
	}

	var feed atomFeed
	if err := xml.Unmarshal(resp.Body, &feed); err != nil {
		return nil, fmt.Errorf("poll %s: parse: %w", p.src.ID, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: p.src.URL,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	wanted := p.zones()
	records := make([]observation.Record, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		if !matchesZone(entry, wanted) {
			continue
		}
		if rec, ok := p.toRecord(entry, resp.FetchedAt, prov); ok {
			records = append(records, rec)
		}
	}
	return records, nil
}

// toRecord normalizes one warning.
func (p *MeteoAlarmProvider) toRecord(e atomEntry, fetchedAt time.Time,
	prov observation.Provenance,
) (observation.Record, bool) {
	area := strings.TrimSpace(e.AreaDesc)
	if area == "" {
		return observation.Record{}, false
	}

	// ObservedAt is when the warning was issued, not when eye read it. The
	// difference between the two is the latency this project refuses to hide.
	observedAt := parseCAPTime(firstNonBlank(e.Sent, e.Updated))
	if observedAt.IsZero() {
		observedAt = fetchedAt
	}

	var from, until *time.Time
	// Onset is when the weather starts; effective is when the warning takes
	// effect. A person wants to know when the heat arrives, so onset wins.
	if t := parseCAPTime(firstNonBlank(e.Onset, e.Effective)); !t.IsZero() {
		utc := t.UTC()
		from = &utc
	}
	if t := parseCAPTime(e.Expires); !t.IsZero() {
		utc := t.UTC()
		until = &utc
	}

	payload, _ := json.Marshal(map[string]any{
		// The authority that issued the warning, kept apart from the
		// publisher that relayed it. Both are true and they are not the
		// same fact.
		"issuing_authority": "AEMET. Agencia Estatal de Meteorología",
		"relayed_by":        "meteoalarm.org",
		"cap_identifier":    strings.TrimSpace(e.Identifier),
		"cap_severity":      strings.TrimSpace(e.Severity),
		"cap_certainty":     strings.TrimSpace(e.Certainty),
		"cap_urgency":       strings.TrimSpace(e.Urgency),
		"cap_status":        strings.TrimSpace(e.Status),
		"cap_message_type":  strings.TrimSpace(e.MsgType),
		"event":             strings.TrimSpace(e.Event),
		"area":              area,
		"zone":              e.zone(),
		"cap_document":      strings.TrimSpace(e.ID),
	})

	// The CAP identifier is the publisher's own stable key for a warning, and
	// an update to a warning carries a new one. Falling back to the entry id
	// keeps identity working if a feed ever omits it.
	local := firstNonBlank(strings.TrimSpace(e.Identifier), strings.TrimSpace(e.ID))

	rec := observation.Record{
		ID:          p.src.ID + ":" + local,
		Source:      p.src.ID,
		Kind:        p.src.Option("kind", "weather_warning"),
		Topic:       p.src.Topic,
		ObservedAt:  observedAt,
		FetchedAt:   fetchedAt,
		ValidFrom:   from,
		ValidUntil:  until,
		Title:       firstNonBlank(strings.TrimSpace(e.Title), strings.TrimSpace(e.Event)+" — "+area),
		Description: strings.TrimSpace(e.Event) + " · " + area,
		Severity:    severityOf(Info{Severity: e.Severity}),
		Confidence:  1,
		// A national met service declaring a warning is the competent
		// authority speaking, even when another service carried the message.
		Quality:   QualityForWarning,
		LocalKey:  local,
		DedupeKey: p.src.ID + ":" + local,
		// No ExpiresAt: an expired warning is still the record of what was
		// said. ValidUntil is what says it no longer applies.
		Payload:    payload,
		Provenance: prov,
	}
	if err := rec.Validate(); err != nil {
		return observation.Record{}, false
	}
	return rec, true
}

// QualityForWarning is the quality every warning record carries. It is a named
// constant so the reasoning sits next to the value rather than in a commit.
const QualityForWarning = observation.QualityOfficial

// zones is the optional ingestion filter, empty by default.
func (p *MeteoAlarmProvider) zones() []string {
	raw := strings.TrimSpace(p.src.Option("zones", ""))
	if raw == "" {
		return nil
	}
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// matchesZone reports whether a warning is one the registry asked for.
//
// An empty list means everything, which is the default. A zone may be given as
// an EMMA_ID or as a substring of the area description; both are folded, because
// nobody types "Subbética" with the accent.
func matchesZone(e atomEntry, wanted []string) bool {
	if len(wanted) == 0 {
		return true
	}

	area := observation.Fold(e.AreaDesc)
	zone := strings.ToUpper(strings.TrimSpace(e.zone()))
	for _, w := range wanted {
		if zone != "" && strings.EqualFold(zone, strings.TrimSpace(w)) {
			return true
		}
		if folded := observation.Fold(w); folded != "" && strings.Contains(area, folded) {
			return true
		}
	}
	return false
}

// firstNonBlank returns the first value that is not whitespace.
func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Interface check.
var _ provider.Provider = (*MeteoAlarmProvider)(nil)
