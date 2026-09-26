// Package datex reads the DGT's DATEX II 3.7 publications.
//
// The parser deliberately ignores unknown elements. DGT extends its profile
// from time to time, and a schema addition must not take the provider down.
package datex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// ErrDatex is returned when a publication cannot be used.
var ErrDatex = errors.New("datex")

// devicePublication is the DevicePublication payload: the camera inventory.
//
// `payload` is the ROOT element (<d2:payload xsi:type="ns2:DevicePublication">),
// not a child of one, so the devices are addressed directly rather than through
// a wrapper path.
type devicePublication struct {
	XMLName xml.Name `xml:"payload"`
	Devices []struct {
		ID      string `xml:"id,attr"`
		Type    string `xml:"typeOfDevice"`
		Updated string `xml:"lastUpdateOfDeviceInformation"`

		Location struct {
			Road struct {
				Destination string `xml:"roadDestination"`
				Name        string `xml:"roadName"`
			} `xml:"supplementaryPositionalDescription>roadInformation"`

			Coordinates struct {
				Latitude  float64 `xml:"latitude"`
				Longitude float64 `xml:"longitude"`
			} `xml:"tpegPointLocation>point>pointCoordinates"`

			KilometerPoint string `xml:"tpegPointLocation>point>_tpegNonJunctionPointExtension>extendedTpegNonJunctionPoint>kilometerPoint"`
			Province       string `xml:"tpegPointLocation>point>_tpegNonJunctionPointExtension>extendedTpegNonJunctionPoint>province"`
		} `xml:"pointLocation"`

		// DeviceURL is the still-image endpoint. Every DGT camera has one;
		// none of them has a video endpoint of any kind.
		DeviceURL string `xml:"deviceUrl"`
	} `xml:"device"`
}

// DeviceProvider turns the DGT camera inventory into entities.
//
// Inventory only. The image URL is recorded as a field so a viewer can fetch a
// frame on demand; eye never fetches or stores images as part of polling.
type DeviceProvider struct {
	src    source.Source
	client *httpx.Client

	validators httpx.Validators
}

// NewDevices builds a camera inventory provider.
func NewDevices(src source.Source, client *httpx.Client) *DeviceProvider {
	return &DeviceProvider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *DeviceProvider) Info() source.Source { return p.src }

// Poll reports the inventory refresh as a single record. The inventory itself
// is the entity set, not a stream of observations.
func (p *DeviceProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	entities, err := p.Entities(ctx)
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{"kind": "camera", "count": len(entities)})

	rec := observation.Record{
		ID:         p.src.ID + ":inventory",
		Source:     p.src.ID,
		Kind:       "inventory_refresh",
		Topic:      p.src.Topic,
		ObservedAt: now,
		FetchedAt:  now,
		Title:      fmt.Sprintf("%d traffic cameras in the national inventory", len(entities)),
		Severity:   observation.SeverityNone,
		Confidence: 1,
		Quality:    observation.QualityOfficial,
		LocalKey:   "inventory",
		DedupeKey:  fmt.Sprintf("%s:inventory:%d", p.src.ID, len(entities)),
		Payload:    payload,
		Provenance: entities[0].Provenance,
	}
	if err := rec.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDatex, err)
	}
	return []observation.Record{rec}, nil
}

// Entities fetches and normalizes the camera inventory.
func (p *DeviceProvider) Entities(ctx context.Context) ([]observation.Entity, error) {
	resp, err := p.client.Get(ctx, p.src.URL, p.validators)
	switch {
	case errors.Is(err, httpx.ErrNotModified):
		p.validators = resp.Validators
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrDatex, err)
	}
	p.validators = resp.Validators

	var doc devicePublication
	if err := decodeXML(resp.DecodeUTF8(), &doc); err != nil {
		return nil, fmt.Errorf("%w: parse devices: %w", ErrDatex, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: p.src.URL,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	entities := make([]observation.Entity, 0, len(doc.Devices))
	for _, d := range doc.Devices {
		if d.ID == "" || !strings.EqualFold(d.Type, "camera") {
			continue
		}

		pos := observation.Point{Lat: d.Location.Coordinates.Latitude, Lon: d.Location.Coordinates.Longitude}
		if !pos.Valid() || (pos.Lat == 0 && pos.Lon == 0) {
			continue
		}

		firstSeen := parseTime(d.Updated)
		if firstSeen.IsZero() {
			firstSeen = resp.FetchedAt
		}

		payload, _ := json.Marshal(map[string]any{
			"road":            d.Location.Road.Name,
			"destination":     d.Location.Road.Destination,
			"kilometer_point": d.Location.KilometerPoint,
			"province":        d.Location.Province,
			// image_url is a pointer, not content. eye stores where a
			// frame can be fetched, never a frame.
			"image_url":          d.DeviceURL,
			"device_last_update": d.Updated,
		})

		ent := observation.Entity{
			ID:         p.src.ID + ":" + d.ID,
			Source:     p.src.ID,
			Kind:       "camera",
			Topic:      p.src.Topic,
			Title:      deviceTitle(d.Location.Road.Name, d.Location.KilometerPoint, d.Location.Province),
			Position:   &pos,
			FirstSeen:  firstSeen,
			LastSeen:   resp.FetchedAt,
			Payload:    payload,
			Provenance: prov,
		}
		if err := ent.Validate(); err != nil {
			continue
		}
		entities = append(entities, ent)
	}

	if len(entities) == 0 && len(doc.Devices) > 0 {
		return nil, fmt.Errorf("%w: %d devices parsed but none usable", ErrDatex, len(doc.Devices))
	}
	return entities, nil
}

// deviceTitle builds a human label from the road, kilometre point and province.
func deviceTitle(road, km, province string) string {
	parts := make([]string, 0, 3)
	if road != "" {
		if km != "" {
			parts = append(parts, road+" km "+km)
		} else {
			parts = append(parts, road)
		}
	}
	if province != "" {
		parts = append(parts, province)
	}
	if len(parts) == 0 {
		return "camera"
	}
	return strings.Join(parts, " · ")
}

// decodeXML unmarshals a document whose bytes are already UTF-8, tolerating a
// stale encoding declaration and unknown elements.
func decodeXML(body string, v any) error {
	dec := newDecoder(body)
	return dec.Decode(v)
}

// parseTime reads the timestamps DATEX II emits.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// Compile-time proof of the ports this adapter satisfies.
var (
	_ provider.Provider       = (*DeviceProvider)(nil)
	_ provider.EntityProvider = (*DeviceProvider)(nil)
	_                         = io.Discard
)
