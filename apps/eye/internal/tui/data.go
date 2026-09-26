package tui

import (
	"encoding/json"
	"strings"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
)

// Data is one read of everything the cockpit shows.
//
// The model is handed a Data rather than a store handle on purpose: reading is
// I/O, rendering is not, and keeping them apart is what makes a full-screen
// view testable with a bytes.Buffer.
type Data struct {
	// ReadAt is when this picture was taken.
	ReadAt time.Time
	// Registry says which sources.yaml is in force.
	Registry string

	Records  []observation.Record
	Entities []observation.Entity
	Sources  []SourceRow

	// RecordCount and EntityCount are the whole store, which is more than
	// the slices above: those are capped so a tick stays cheap.
	RecordCount int
	EntityCount int

	// Err is a read that failed. A cockpit that silently shows the previous
	// frame when the store is unreadable is a cockpit that lies.
	Err error
}

// SourceRow is one registry entry together with what earlier polls recorded
// about it.
type SourceRow struct {
	ID         string
	Authority  string
	Topic      string
	Format     string
	License    string
	Access     string
	Automation string

	// Pollable is the registry's permission and the machine's opt-in taken
	// together, not the registry's alone.
	Pollable   bool
	HasAdapter bool
	Interval   time.Duration

	LastSuccess time.Time
	LastAttempt time.Time
	LastError   string
	Records     int
	Stale       bool
}

// State describes what eye will actually do with a source, in the words the
// registry uses.
//
// "held", "failing" and "no adapter" are three different problems and the board
// must not flatten them into one: the first is a licence question, the second
// is somebody's server, the third is work eye has not done yet.
func (s SourceRow) State() string {
	switch {
	case s.Automation != "enabled":
		return s.Automation
	case !s.HasAdapter:
		return "no adapter"
	case !s.Pollable:
		return "held"
	case s.LastSuccess.IsZero() && s.LastError != "":
		return "failing"
	case s.LastSuccess.IsZero():
		return "never polled"
	case s.Stale:
		return "stale"
	default:
		return "live"
	}
}

// indicator maps a state onto the theme's status dots.
func (s SourceRow) indicator() string {
	switch s.State() {
	case "live":
		return "live"
	case "failing":
		return "failing"
	case "stale", "held", "review_terms", "manual_link":
		return "stale"
	default:
		return "none"
	}
}

// Arrival is one live bus estimate as the cockpit shows it.
//
// ReadAt is carried per arrival rather than per panel because it is the field
// that decides whether the number may still be called live.
type Arrival struct {
	StopID    string
	StopName  string
	Line      string
	Route     string
	Minutes   int
	Occupancy string
	ReadAt    time.Time
}

// arrivalCadence is how long an operator estimate may be shown as live.
//
// AUCORSA recomputes about once a minute. Past that the number on screen is a
// memory of a prediction, and presenting it as live would be exactly the kind
// of quiet lie eye exists not to tell.
const arrivalCadence = 60 * time.Second

// Stale reports whether the estimate has outlived its own cadence.
func (a Arrival) Stale(now time.Time) bool {
	return a.ReadAt.IsZero() || now.Sub(a.ReadAt) > arrivalCadence
}

// ETA is how long is left until the bus is due, counted down from when the
// estimate was read.
func (a Arrival) ETA(now time.Time) time.Duration {
	due := a.ReadAt.Add(time.Duration(a.Minutes) * time.Minute)
	return due.Sub(now)
}

// Stop is a bus stop the cockpit can ask about.
type Stop struct {
	// ID is the number printed on the pole, which is what the arrivals
	// endpoint expects — never the operator's internal page number.
	ID       string
	Name     string
	Position *observation.Point
}

// stopPayload is the shape the bus-stop entities carry.
type stopPayload struct {
	StopID string `json:"stop_id"`
}

// stopsFrom projects the bus-stop inventory out of the entity list.
func stopsFrom(entities []observation.Entity) []Stop {
	out := make([]Stop, 0, len(entities))
	for _, e := range entities {
		if e.Kind != "bus_stop" {
			continue
		}

		id := ""
		var p stopPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			id = p.StopID
		}
		if id == "" {
			// Fall back to the tail of the entity id, which is how the
			// adapter composes it.
			if i := strings.LastIndex(e.ID, ":"); i >= 0 {
				id = e.ID[i+1:]
			}
		}
		if id == "" {
			continue
		}
		out = append(out, Stop{ID: id, Name: e.Title, Position: e.Position})
	}
	return out
}

// departureKind is the record kind the GTFS reader emits for a scheduled call.
const departureKind = "scheduled_departure"

// departuresFrom returns the next scheduled train departures, soonest first.
func departuresFrom(records []observation.Record, now time.Time) []observation.Record {
	out := make([]observation.Record, 0, 8)
	for _, r := range records {
		if r.Kind != departureKind || r.ValidFrom == nil || r.ValidFrom.Before(now) {
			continue
		}
		out = append(out, r)
	}

	// Insertion sort: this list is a handful of departures, and a sort
	// import for eight items is not worth the line.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].ValidFrom.Before(*out[j-1].ValidFrom); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
