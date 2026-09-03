package aemet

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The AEMET-Meteoalerta extension names, as fixed by annex 3 of the Plan
// Meteoalerta. They are the only part of the CAP profile that is not plain
// OASIS CAP 1.2.
const (
	paramLevel       = "AEMET-Meteoalerta nivel"
	paramParameter   = "AEMET-Meteoalerta parametro"
	paramProbability = "AEMET-Meteoalerta probabilidad"
	codePhenomenon   = "AEMET-Meteoalerta fenomeno"
	geocodeZone      = "AEMET-Meteoalerta zona"
)

// Alert is one CAP 1.2 message as AEMET emits it under the Meteoalerta plan.
//
// Only the elements eye normalizes are named. The rest of CAP stays in the raw
// archive: an adapter that invents domain fields out of a wire format is how a
// normalizer starts leaking DATEX II and CAP into the rest of the program.
type Alert struct {
	XMLName    xml.Name `xml:"alert"`
	Identifier string   `xml:"identifier"`
	Sender     string   `xml:"sender"`
	Sent       string   `xml:"sent"`
	Status     string   `xml:"status"`
	MsgType    string   `xml:"msgType"`
	Scope      string   `xml:"scope"`
	References string   `xml:"references"`
	Infos      []Info   `xml:"info"`
}

// Info is one language block of an alert. AEMET publishes es-ES and en-GB.
type Info struct {
	Language     string       `xml:"language"`
	Category     string       `xml:"category"`
	Event        string       `xml:"event"`
	ResponseType string       `xml:"responseType"`
	Urgency      string       `xml:"urgency"`
	Severity     string       `xml:"severity"`
	Certainty    string       `xml:"certainty"`
	EventCodes   []NamedValue `xml:"eventCode"`
	Effective    string       `xml:"effective"`
	Onset        string       `xml:"onset"`
	Expires      string       `xml:"expires"`
	SenderName   string       `xml:"senderName"`
	Headline     string       `xml:"headline"`
	Description  string       `xml:"description"`
	Instruction  string       `xml:"instruction"`
	Web          string       `xml:"web"`
	Contact      string       `xml:"contact"`
	Parameters   []NamedValue `xml:"parameter"`
	Areas        []Area       `xml:"area"`
}

// NamedValue is CAP's key/value pair, used by parameter, eventCode and geocode.
type NamedValue struct {
	ValueName string `xml:"valueName"`
	Value     string `xml:"value"`
}

// Area is one warning zone: a name, its boundary and its Meteoalerta code.
type Area struct {
	Desc     string       `xml:"areaDesc"`
	Polygons []string     `xml:"polygon"`
	Geocodes []NamedValue `xml:"geocode"`
}

// ParseCAP reads one CAP 1.2 message.
func ParseCAP(body []byte) (Alert, error) {
	var a Alert
	if err := xml.Unmarshal(body, &a); err != nil {
		return Alert{}, fmt.Errorf("%w: cap: %w", ErrAEMET, err)
	}
	if a.Identifier == "" || len(a.Infos) == 0 {
		return Alert{}, fmt.Errorf("%w: cap: message has no identifier or no info block", ErrAEMET)
	}
	return a, nil
}

// Localized returns the info block for a language, falling back to the same
// language family and then to the first block published.
func (a Alert) Localized(language string) (Info, bool) {
	if len(a.Infos) == 0 {
		return Info{}, false
	}
	want := strings.ToLower(strings.TrimSpace(language))
	family, _, _ := strings.Cut(want, "-")

	for _, info := range a.Infos {
		if strings.EqualFold(strings.TrimSpace(info.Language), want) {
			return info, true
		}
	}
	for _, info := range a.Infos {
		if prefix, _, _ := strings.Cut(strings.ToLower(info.Language), "-"); prefix == family && family != "" {
			return info, true
		}
	}
	return a.Infos[0], true
}

// Parameter returns the value of a named CAP parameter.
func (i Info) Parameter(name string) string {
	for _, p := range i.Parameters {
		if strings.EqualFold(strings.TrimSpace(p.ValueName), name) {
			return strings.TrimSpace(p.Value)
		}
	}
	return ""
}

// Level is the AEMET warning colour: verde, amarillo, naranja or rojo.
func (i Info) Level() string { return strings.ToLower(i.Parameter(paramLevel)) }

// Phenomenon splits the Meteoalerta event code into its two-letter code and the
// phenomenon name beside it, for example "AT" and the Spanish for maximum
// temperatures.
func (i Info) Phenomenon() (code, name string) {
	for _, c := range i.EventCodes {
		if strings.EqualFold(strings.TrimSpace(c.ValueName), codePhenomenon) {
			code, name, _ = strings.Cut(strings.TrimSpace(c.Value), ";")
			return strings.TrimSpace(code), strings.TrimSpace(name)
		}
	}
	return "", ""
}

// Zone is the Meteoalerta zone code of an area.
func (a Area) Zone() string {
	for _, g := range a.Geocodes {
		if strings.EqualFold(strings.TrimSpace(g.ValueName), geocodeZone) {
			return strings.TrimSpace(g.Value)
		}
	}
	return ""
}

// capLayouts are the timestamp forms AEMET emits. The plan fixes an offset on
// every one of them, including "-00:00" for UTC.
var capLayouts = []string{time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05"}

// parseCAPTime reads a CAP timestamp, returning the zero time when it is absent
// or unreadable rather than guessing a moment.
func parseCAPTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range capLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// polygonGeoJSON converts CAP polygons into GeoJSON and returns the centroid of
// the first ring.
//
// CAP writes "lat,lon" pairs; GeoJSON wants [lon, lat]. Getting that backwards
// puts every warning for Cordoba somewhere in the Indian Ocean.
func polygonGeoJSON(polygons []string) (geometry json.RawMessage, lat, lon float64, ok bool) {
	rings := make([][][2]float64, 0, len(polygons))
	for _, raw := range polygons {
		if ring := parseRing(raw); len(ring) >= 4 {
			rings = append(rings, ring)
		}
	}
	if len(rings) == 0 {
		return nil, 0, 0, false
	}

	var shape any
	if len(rings) == 1 {
		shape = map[string]any{"type": "Polygon", "coordinates": [][][2]float64{rings[0]}}
	} else {
		multi := make([][][][2]float64, 0, len(rings))
		for _, r := range rings {
			multi = append(multi, [][][2]float64{r})
		}
		shape = map[string]any{"type": "MultiPolygon", "coordinates": multi}
	}

	encoded, err := json.Marshal(shape)
	if err != nil {
		return nil, 0, 0, false
	}

	lon, lat = centroid(rings[0])
	return encoded, lat, lon, true
}

// parseRing reads one CAP polygon into a closed GeoJSON ring.
func parseRing(raw string) [][2]float64 {
	fields := strings.Fields(raw)
	ring := make([][2]float64, 0, len(fields)+1)

	for _, pair := range fields {
		latText, lonText, found := strings.Cut(pair, ",")
		if !found {
			continue
		}
		lat, errLat := strconv.ParseFloat(strings.TrimSpace(latText), 64)
		lon, errLon := strconv.ParseFloat(strings.TrimSpace(lonText), 64)
		if errLat != nil || errLon != nil {
			continue
		}
		ring = append(ring, [2]float64{lon, lat})
	}

	// GeoJSON requires the ring to close; CAP usually does it already.
	if len(ring) >= 3 && ring[0] != ring[len(ring)-1] {
		ring = append(ring, ring[0])
	}
	return ring
}

// centroid averages a ring's vertices, ignoring the repeated closing point.
func centroid(ring [][2]float64) (lon, lat float64) {
	points := ring
	if len(points) > 1 && points[0] == points[len(points)-1] {
		points = points[:len(points)-1]
	}
	if len(points) == 0 {
		return 0, 0
	}
	for _, p := range points {
		lon += p[0]
		lat += p[1]
	}
	n := float64(len(points))
	return lon / n, lat / n
}
