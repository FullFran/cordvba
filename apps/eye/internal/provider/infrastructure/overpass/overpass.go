// Package overpass reads OpenStreetMap features through the Overpass API.
//
// It exists because of a gap nothing else in the registry fills. AUCORSA
// publishes 690 bus stops with names and pole numbers and no coordinates, and
// the municipal cartography layer publishes route geometry with barely a
// handful of named stops. Neither can put a bus stop on a map. OSM can, under a
// documented API and a real licence.
//
// What this adapter deliberately does NOT do is match one to the other. A stop
// called "Avda. Brillante (H.S.Juan de Dios)" in one dataset and "Avenida del
// Brillante" in the other are probably the same pole, and "probably" is not a
// coordinate eye is willing to publish under somebody else's name. The two
// inventories are stored side by side, each with its own provenance.
package overpass

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// ErrOverpass wraps every failure this adapter reports.
var ErrOverpass = errors.New("overpass")

// ErrNoQuery is returned when the registry entry declares no query.
//
// It is a registry error rather than a default, on purpose: an adapter that
// ships its own query decides for itself what to take from somebody else's
// server, and the whole point of the registry is that nothing is fetched that
// it does not name.
var ErrNoQuery = fmt.Errorf("%w: the registry entry declares no `query` option", ErrOverpass)

// bboxPlaceholder is what a registry query writes where the bounding box goes.
const bboxPlaceholder = "{{bbox}}"

// queryTimeout is the server-side budget sent in the Overpass header. It is the
// public instance's own limit that matters here, not ours: a query that runs
// long is a query somebody else pays for.
const queryTimeout = 60

// Provider reads one Overpass query and turns its elements into inventory.
type Provider struct {
	src    source.Source
	client *httpx.Client
}

// New builds a provider for a registry entry.
func New(src source.Source, client *httpx.Client) *Provider {
	return &Provider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// element is one OSM node, way or relation as Overpass serialises it.
//
// Ways and relations carry a `center` rather than a position of their own, and
// a bus stop mapped as a platform way is exactly as real as one mapped as a
// node — so both are read.
type element struct {
	Type   string  `json:"type"`
	ID     int64   `json:"id"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Center *struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"center"`
	Tags map[string]string `json:"tags"`
}

// position returns where the element is, preferring its own coordinates.
func (e element) position() (observation.Point, bool) {
	point := observation.Point{Lat: e.Lat, Lon: e.Lon}
	if e.Center != nil && e.Lat == 0 && e.Lon == 0 {
		point = observation.Point{Lat: e.Center.Lat, Lon: e.Center.Lon}
	}
	if !point.Valid() || (point.Lat == 0 && point.Lon == 0) {
		return observation.Point{}, false
	}
	return point, true
}

// answer is the Overpass JSON envelope.
type answer struct {
	Elements []element `json:"elements"`
}

// Poll reports the inventory refresh as a single record.
//
// One record, not one per stop: a bus stop is a persistent thing and its
// position does not change every poll. The stops themselves are entities, which
// is what Entities returns.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	entities, _, err := p.fetch(ctx)
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, nil
	}

	fetchedAt := entities[0].LastSeen
	payload, _ := json.Marshal(map[string]any{
		"kind":  p.kind(),
		"count": len(entities),
		"query": p.query(),
	})

	rec := observation.Record{
		ID:         p.src.ID + ":inventory",
		Source:     p.src.ID,
		Kind:       "inventory_refresh",
		Topic:      p.src.Topic,
		ObservedAt: fetchedAt,
		FetchedAt:  fetchedAt,
		Title:      fmt.Sprintf("%d %s features in OpenStreetMap", len(entities), p.kind()),
		Severity:   observation.SeverityInfo,
		Confidence: 1,
		// OSM is a survey by volunteers, not a declaration by the authority
		// that runs the buses. It is good data and it is not official data,
		// and flattening that difference is how a map starts lying quietly.
		Quality:    observation.QualityValidated,
		LocalKey:   p.src.ID + ":inventory",
		DedupeKey:  p.src.ID + ":inventory:" + fetchedAt.UTC().Format(time.RFC3339),
		Payload:    payload,
		Provenance: entities[0].Provenance,
	}
	if err := rec.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOverpass, err)
	}
	return []observation.Record{rec}, nil
}

// Entities fetches the inventory.
func (p *Provider) Entities(ctx context.Context) ([]observation.Entity, error) {
	entities, _, err := p.fetch(ctx)
	return entities, err
}

// fetch runs the query and maps the elements, returning how many were skipped.
func (p *Provider) fetch(ctx context.Context) ([]observation.Entity, int, error) {
	query := p.query()
	if query == "" {
		return nil, 0, ErrNoQuery
	}

	endpoint := p.src.URL + "?data=" + url.QueryEscape(query)
	resp, err := p.client.Get(ctx, endpoint, httpx.Validators{})
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrOverpass, err)
	}

	var doc answer
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		return nil, 0, fmt.Errorf("%w: parse: %w", ErrOverpass, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		// The query is the source URL. Without it, nobody reading a stored
		// entity can tell which of the many things in OSM this came from.
		SourceURL: endpoint,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	entities := make([]observation.Entity, 0, len(doc.Elements))
	skipped := 0
	for _, el := range doc.Elements {
		ent, ok := p.toEntity(el, resp.FetchedAt, prov)
		if !ok {
			skipped++
			continue
		}
		entities = append(entities, ent)
	}
	return entities, skipped, nil
}

// toEntity maps one element, skipping anything eye cannot place or name.
//
// Both skips are deliberate. An element with no position is useless to the only
// job this source has, and an element with no name gets no invented one: an
// unnamed OSM node is a real stop, but "bus_stop 914978713" is a label eye made
// up, and a person searching for their stop will never type it.
func (p *Provider) toEntity(el element, seenAt time.Time, prov observation.Provenance) (observation.Entity, bool) {
	point, ok := el.position()
	if !ok {
		return observation.Entity{}, false
	}

	name := strings.TrimSpace(el.Tags["name"])
	if name == "" {
		return observation.Entity{}, false
	}

	payload, _ := json.Marshal(el.Tags)

	ent := observation.Entity{
		// The OSM element type belongs in the id. A node and a way can
		// share an id number, and they are different objects.
		ID:         fmt.Sprintf("%s:%s/%d", p.src.ID, el.Type, el.ID),
		Source:     p.src.ID,
		Kind:       p.kind(),
		Topic:      p.src.Topic,
		Title:      name,
		Position:   &point,
		FirstSeen:  seenAt,
		LastSeen:   seenAt,
		Payload:    payload,
		Provenance: prov,
	}
	if err := ent.Validate(); err != nil {
		return observation.Entity{}, false
	}
	return ent, true
}

// query builds the Overpass QL to send, substituting the registry bounding box.
func (p *Provider) query() string {
	body := strings.TrimSpace(p.src.Options["query"])
	if body == "" {
		return ""
	}
	if bbox := strings.TrimSpace(p.src.Options["bbox"]); bbox != "" {
		body = strings.ReplaceAll(body, bboxPlaceholder, bbox)
	}

	// The header is ours rather than the registry's. JSON output is what
	// this adapter parses, and the timeout is a promise to the public
	// instance about how long we are willing to make it work.
	return fmt.Sprintf("[out:json][timeout:%d];%s\nout body center;", queryTimeout, body)
}

// kind is the entity kind the registry asked for.
func (p *Provider) kind() string {
	if k := strings.TrimSpace(p.src.Options["kind"]); k != "" {
		return k
	}
	return "osm_feature"
}

// Interface checks.
var (
	_ provider.Provider       = (*Provider)(nil)
	_ provider.EntityProvider = (*Provider)(nil)
)
