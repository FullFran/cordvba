package domain_test

import (
	"testing"
	"time"

	domain "github.com/FullFran/eye/internal/observation/domain"
)

// recordAt builds a record for filter tests.
func recordAt(topic, source, kind string, at time.Time, pos *domain.Point) domain.Record {
	r := validRecord()
	r.Topic, r.Source, r.Kind, r.ObservedAt, r.Position = topic, source, kind, at, pos
	return r
}

func TestFilterMatchRecord(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	hourAgo := now.Add(-time.Hour)
	cordoba := domain.Point{Lat: 37.8882, Lon: -4.7794}
	madrid := domain.Point{Lat: 40.4168, Lon: -3.7038}

	base := recordAt("press", "diario-cordoba", "news_item", hourAgo, &cordoba)
	base.Title = "Velá de la Fuensanta"

	since := now.Add(-2 * time.Hour)
	tooRecent := now.Add(-time.Minute)
	until := now.Add(-30 * time.Minute)

	cases := []struct {
		name   string
		filter domain.Filter
		record domain.Record
		want   bool
	}{
		{name: "empty filter matches everything", filter: domain.Filter{}, record: base, want: true},
		{name: "topic matches", filter: domain.Filter{Topics: []string{"press"}}, record: base, want: true},
		{name: "topic is case-insensitive", filter: domain.Filter{Topics: []string{"PRESS"}}, record: base, want: true},
		{name: "topic excludes", filter: domain.Filter{Topics: []string{"fire"}}, record: base, want: false},
		{name: "source excludes", filter: domain.Filter{Sources: []string{"boe"}}, record: base, want: false},
		{name: "kind excludes", filter: domain.Filter{Kinds: []string{"aircraft_position"}}, record: base, want: false},
		{name: "since includes", filter: domain.Filter{Since: &since}, record: base, want: true},
		{name: "since excludes older", filter: domain.Filter{Since: &tooRecent}, record: base, want: false},
		{name: "until excludes newer", filter: domain.Filter{Until: &until}, record: base, want: true},
		{name: "min severity excludes", filter: domain.Filter{MinSeverity: domain.SeverityCritical}, record: base, want: false},
		{name: "text matches title", filter: domain.Filter{Text: "fuensanta"}, record: base, want: true},
		{name: "text excludes", filter: domain.Filter{Text: "no such thing"}, record: base, want: false},
		{
			name:   "bbox includes",
			filter: domain.Filter{BBox: &domain.BBox{West: -5.6, South: 37.1, East: -4.0, North: 38.7}},
			record: base, want: true,
		},
		{
			name:   "bbox excludes",
			filter: domain.Filter{BBox: &domain.BBox{West: -5.6, South: 37.1, East: -4.0, North: 38.7}},
			record: recordAt("press", "s", "k", hourAgo, &madrid), want: false,
		},
		{
			name:   "radius includes",
			filter: domain.Filter{Near: &cordoba, RadiusKm: 10},
			record: base, want: true,
		},
		{
			name:   "radius excludes",
			filter: domain.Filter{Near: &cordoba, RadiusKm: 10},
			record: recordAt("press", "s", "k", hourAgo, &madrid), want: false,
		},
		{
			name:   "spatial filter excludes a record with no position",
			filter: domain.Filter{Near: &cordoba, RadiusKm: 10},
			record: recordAt("press", "s", "k", hourAgo, nil), want: false,
		},
		{
			name:   "no spatial filter keeps a record with no position",
			filter: domain.Filter{Topics: []string{"press"}},
			record: recordAt("press", "s", "k", hourAgo, nil), want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.filter.MatchRecord(tc.record); got != tc.want {
				t.Errorf("MatchRecord() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFilterMatchEntity(t *testing.T) {
	t.Parallel()

	e := validEntity()

	if !(domain.Filter{Kinds: []string{"camera"}}).MatchEntity(e) {
		t.Error("kind filter should match")
	}
	if (domain.Filter{Kinds: []string{"gauge"}}).MatchEntity(e) {
		t.Error("kind filter should exclude")
	}
	if !(domain.Filter{Text: "aeropuerto"}).MatchEntity(e) {
		t.Error("text filter should match the title case-insensitively")
	}
	if (domain.Filter{Sources: []string{"other"}}).MatchEntity(e) {
		t.Error("source filter should exclude")
	}
	if (domain.Filter{Near: &domain.Point{Lat: 0, Lon: 0}, RadiusKm: 1}).MatchEntity(e) {
		t.Error("radius filter should exclude a distant entity")
	}
}

func TestSortRecordsNewestFirst(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	records := []domain.Record{
		recordAt("a", "s", "k", now.Add(-3*time.Hour), nil),
		recordAt("b", "s", "k", now, nil),
		recordAt("c", "s", "k", now.Add(-time.Hour), nil),
	}

	domain.SortRecordsNewestFirst(records)

	for i := 1; i < len(records); i++ {
		if records[i].ObservedAt.After(records[i-1].ObservedAt) {
			t.Fatalf("records are not newest-first: %v", records)
		}
	}
}

func TestApplyLimit(t *testing.T) {
	t.Parallel()

	items := []int{1, 2, 3, 4, 5}

	if got := domain.ApplyLimit(items, 0); len(got) != 5 {
		t.Errorf("limit 0 should keep everything, got %d", len(got))
	}
	if got := domain.ApplyLimit(items, 3); len(got) != 3 {
		t.Errorf("limit 3 = %d items", len(got))
	}
	if got := domain.ApplyLimit(items, 99); len(got) != 5 {
		t.Errorf("limit above length = %d items", len(got))
	}
}

func TestFold(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, in, want string }{
		{name: "córdoba", in: "CÓRDOBA", want: "cordoba"},
		{name: "diputación", in: "Diputación", want: "diputacion"},
		{name: "velá", in: "Velá de la Fuensanta", want: "vela de la fuensanta"},
		{name: "españa", in: "España", want: "espana"},
		{name: "jaén", in: "JAÉN", want: "jaen"},
		{name: "already plain", in: "A-4 km 399.1", want: "a-4 km 399.1"},
		{name: "empty", in: "", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.Fold(tc.in); got != tc.want {
				t.Errorf("Fold(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The camera outside Córdoba is titled "A-4 km 399.1 · CÓRDOBA". Nobody types
// the accent, and nobody types the fields in that order.
func TestFilterTextIsAccentInsensitiveAndOrderFree(t *testing.T) {
	t.Parallel()

	rec := recordAt("transport", "dgt-cameras", "camera", time.Now().UTC(), nil)
	rec.Title = "A-4 km 399.1 · CÓRDOBA"
	rec.Description = "Direccion General de Trafico"

	cases := []struct {
		name string
		text string
		want bool
	}{
		{name: "exact", text: "A-4 km 399.1", want: true},
		{name: "no accent", text: "cordoba", want: true},
		{name: "with accent", text: "CÓRDOBA", want: true},
		{name: "words out of order", text: "cordoba a-4", want: true},
		{name: "across title and description", text: "cordoba trafico", want: true},
		{name: "one word missing", text: "cordoba sevilla", want: false},
		{name: "no match at all", text: "burgos", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := (domain.Filter{Text: tc.text}).MatchRecord(rec); got != tc.want {
				t.Errorf("MatchRecord with text %q = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
