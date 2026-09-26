package rss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// Provider polls one syndication feed and normalizes it into records.
type Provider struct {
	src    source.Source
	client *httpx.Client

	// validators carry the previous poll's cache headers so the next
	// request can be conditional.
	validators httpx.Validators
}

// New builds a provider for a registry entry.
func New(src source.Source, client *httpx.Client) *Provider {
	return &Provider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// Poll fetches the feed and returns one record per entry.
//
// A 304 is a successful, empty poll: the publisher told us nothing changed.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	resp, err := p.client.Get(ctx, p.src.URL, p.validators)
	switch {
	case errors.Is(err, httpx.ErrNotModified):
		p.validators = resp.Validators
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("poll %s: %w", p.src.ID, err)
	}
	p.validators = resp.Validators

	feed, err := Parse(resp.DecodeUTF8())
	if err != nil {
		return nil, fmt.Errorf("poll %s: %w", p.src.ID, err)
	}

	rawHash := sha256.Sum256(resp.Body)
	hash := hex.EncodeToString(rawHash[:])

	records := make([]observation.Record, 0, len(feed.Items))
	for _, item := range feed.Items {
		rec, err := p.toRecord(item, resp.FetchedAt, hash)
		if err != nil {
			// One malformed entry must not discard the rest of the
			// feed; publishers ship broken items regularly.
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// isEventFeed reports whether this source publishes things that are going to
// happen rather than things that have been published.
//
// The distinction changes what the feed's timestamp means: for the UCO events
// feed, pubDate is the event start, not the moment of publication.
func (p *Provider) isEventFeed() bool { return p.src.Topic == "events" }

// toRecord normalizes one entry.
func (p *Provider) toRecord(item Item, fetchedAt time.Time, rawHash string) (observation.Record, error) {
	if item.Title == "" {
		return observation.Record{}, errors.New("rss: entry with no title")
	}

	link := firstNonEmpty(item.Link, item.GUID)
	dedupe := dedupeKey(p.src.ID, link, item.Title)

	rec := observation.Record{
		ID:          p.src.ID + ":" + dedupe,
		Source:      p.src.ID,
		Kind:        p.kind(),
		Topic:       p.src.Topic,
		FetchedAt:   fetchedAt,
		Title:       item.Title,
		Description: truncate(item.Description, 400),
		Severity:    observation.SeverityInfo,
		Confidence:  1, // The publisher said it; that much is certain.
		Quality:     observation.QualityOfficial,
		// The link is the article. Its title and description are what a
		// newsroom edits after publishing, so identity taken from those
		// would report an edit as a different story.
		LocalKey:  link,
		DedupeKey: dedupe,
		Provenance: observation.Provenance{
			Publisher: p.src.Authority,
			SourceURL: firstNonEmpty(link, p.src.URL),
			License:   p.src.License,
			FetchedAt: fetchedAt,
			RawHash:   rawHash,
		},
	}

	if item.Lat != nil && item.Lon != nil {
		pos := observation.Point{Lat: *item.Lat, Lon: *item.Lon}
		if pos.Valid() {
			rec.Position = &pos
		}
	}

	if p.isEventFeed() {
		// We learned about the event now; the event itself is later.
		rec.ObservedAt = fetchedAt
		if !item.Published.IsZero() {
			starts := item.Published
			rec.ValidFrom = &starts
		}
	} else {
		rec.ObservedAt = item.Published
		if rec.ObservedAt.IsZero() {
			// A feed entry with no date is dated at fetch time, and
			// the payload records that it was missing rather than
			// letting the gap disappear.
			rec.ObservedAt = fetchedAt
		}
	}

	payload := map[string]any{"link": link}
	if item.Author != "" {
		payload["author"] = item.Author
	}
	if len(item.Categories) > 0 {
		payload["categories"] = item.Categories
	}
	if item.Published.IsZero() {
		payload["published_missing"] = true
	}
	if magnitude, ok := p.magnitude(item); ok {
		payload["magnitude"] = magnitude
		rec.Severity = severityForMagnitude(magnitude)
	}
	if raw, err := json.Marshal(payload); err == nil {
		rec.Payload = raw
	}

	if err := rec.Validate(); err != nil {
		return observation.Record{}, err
	}
	return rec, nil
}

// kind labels the record by what the feed publishes.
//
// The registry wins when it says something. A syndication feed is a transport,
// not a subject: IGN publishes earthquakes over RSS, and calling those
// "news_item" because they arrived as RSS makes them unfindable by anything
// looking for seismic events. Everything else falls back to the topic, so a
// feed that says nothing keeps the behaviour it has always had.
func (p *Provider) kind() string {
	if declared := strings.TrimSpace(p.src.Options["kind"]); declared != "" {
		return declared
	}

	switch p.src.Topic {
	case "events":
		return "event_listing"
	case "civic":
		return "official_publication"
	default:
		return "news_item"
	}
}

// magnitudePattern reads the magnitude out of the sentence IGN writes for every
// earthquake: "Se ha producido un terremoto de magnitud 3.1 en NE CEHEGÍN.MU".
//
// It is anchored on the word rather than on any number in the text, because a
// headline is full of numbers and none of the others is a magnitude.
var magnitudePattern = regexp.MustCompile(`(?i)magnitud[a-z]*\s*:?\s*([0-9]+(?:[.,][0-9]+)?)`)

// magnitude extracts a stated magnitude, and only for a source that asked.
//
// It is opt-in per registry entry (`options: {severity: magnitude}`) rather
// than applied everywhere, because guessing severity from a number found in a
// press headline would be exactly the kind of invention this project does not
// do. A feed that states a magnitude may be read this way; one that does not,
// may not.
func (p *Provider) magnitude(item Item) (float64, bool) {
	if !strings.EqualFold(strings.TrimSpace(p.src.Options["severity"]), "magnitude") {
		return 0, false
	}

	match := magnitudePattern.FindStringSubmatch(item.Title + " " + item.Description)
	if match == nil {
		return 0, false
	}

	value, err := strconv.ParseFloat(strings.Replace(match[1], ",", ".", 1), 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

// severityForMagnitude maps the Richter-like scale the publisher states onto
// eye's cross-source severity scale.
//
// The boundaries follow the usual reading of the moment magnitude scale: below
// 2 is only instrumentally detectable, 4 is where damage becomes possible, and
// 5 is where it becomes likely. They are coarse on purpose — the point is that
// a magnitude 5 does not sit in the same rank as a press headline, not that eye
// has an opinion about seismology.
func severityForMagnitude(magnitude float64) observation.Severity {
	switch {
	case magnitude >= 5:
		return observation.SeverityCritical
	case magnitude >= 4:
		return observation.SeverityHigh
	case magnitude >= 3:
		return observation.SeverityModerate
	case magnitude >= 2:
		return observation.SeverityLow
	default:
		return observation.SeverityInfo
	}
}

// dedupeKey is a stable fingerprint for an entry, so the same article arriving
// on every poll produces the same record id.
func dedupeKey(sourceID, link, title string) string {
	basis := sourceID + "|" + strings.ToLower(strings.TrimSpace(firstNonEmpty(link, title)))
	sum := sha256.Sum256([]byte(basis))
	return hex.EncodeToString(sum[:8])
}

// truncate shortens a description on a rune boundary.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}

// Compile-time proof that the adapter satisfies the port.
var _ provider.Provider = (*Provider)(nil)
