package domain_test

import (
	"errors"
	"testing"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
)

// validProvenance returns the minimum evidence a record needs to be storable.
func validProvenance(fetchedAt time.Time) domain.Provenance {
	return domain.Provenance{
		Publisher: "DGT",
		SourceURL: "https://nap.dgt.es/es/dataset/incidencias-dgt-datex2-v3-7",
		License:   "CC-BY-4.0",
		FetchedAt: fetchedAt,
		RawHash:   "3f786850e387550fdab836ed7e6dc881de23001b",
	}
}

// validRecord returns a record that passes Validate, for tests to mutate.
func validRecord() domain.Record {
	observed := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	fetched := observed.Add(13 * time.Minute)

	return domain.Record{
		ID:         "dgt:incident:42",
		Source:     "dgt-incidents",
		Kind:       "road_incident",
		Topic:      "transport",
		ObservedAt: observed,
		FetchedAt:  fetched,
		Position:   &domain.Point{Lat: 37.8882, Lon: -4.7794},
		Title:      "A-4 lane closure",
		Severity:   domain.SeverityModerate,
		Confidence: 0.8,
		Quality:    domain.QualityOfficial,
		Provenance: validProvenance(fetched),
	}
}

func TestRecordLatency(t *testing.T) {
	t.Parallel()

	r := validRecord()
	if got, want := r.Latency(), 13*time.Minute; got != want {
		t.Errorf("Latency() = %v, want %v", got, want)
	}
}

func TestRecordLatencyNegativeWhenSourceDatesInTheFuture(t *testing.T) {
	t.Parallel()

	// A source that timestamps ahead of us is a data problem worth seeing,
	// not something to clamp to zero.
	r := validRecord()
	r.ObservedAt = r.FetchedAt.Add(time.Hour)

	if got := r.Latency(); got >= 0 {
		t.Errorf("Latency() = %v, want a negative duration", got)
	}
}

func TestRecordValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*domain.Record)
		wantErr error
	}{
		{name: "valid record", mutate: func(*domain.Record) {}},
		{name: "empty id", mutate: func(r *domain.Record) { r.ID = "" }, wantErr: domain.ErrInvalidRecord},
		{name: "empty source", mutate: func(r *domain.Record) { r.Source = "" }, wantErr: domain.ErrInvalidRecord},
		{name: "empty topic", mutate: func(r *domain.Record) { r.Topic = "" }, wantErr: domain.ErrInvalidRecord},
		{name: "zero observed_at", mutate: func(r *domain.Record) { r.ObservedAt = time.Time{} }, wantErr: domain.ErrInvalidRecord},
		{name: "zero fetched_at", mutate: func(r *domain.Record) { r.FetchedAt = time.Time{} }, wantErr: domain.ErrInvalidRecord},
		{name: "severity above scale", mutate: func(r *domain.Record) { r.Severity = 6 }, wantErr: domain.ErrInvalidRecord},
		{name: "severity below scale", mutate: func(r *domain.Record) { r.Severity = -1 }, wantErr: domain.ErrInvalidRecord},
		{name: "confidence above one", mutate: func(r *domain.Record) { r.Confidence = 1.5 }, wantErr: domain.ErrInvalidRecord},
		{name: "confidence below zero", mutate: func(r *domain.Record) { r.Confidence = -0.1 }, wantErr: domain.ErrInvalidRecord},
		{name: "unknown quality", mutate: func(r *domain.Record) { r.Quality = "vibes" }, wantErr: domain.ErrInvalidRecord},
		{name: "position outside WGS84", mutate: func(r *domain.Record) { r.Position = &domain.Point{Lat: 91} }, wantErr: domain.ErrInvalidRecord},
		{name: "missing publisher", mutate: func(r *domain.Record) { r.Provenance.Publisher = "" }, wantErr: domain.ErrMissingProvenance},
		{name: "missing source url", mutate: func(r *domain.Record) { r.Provenance.SourceURL = "" }, wantErr: domain.ErrMissingProvenance},
		{name: "missing license", mutate: func(r *domain.Record) { r.Provenance.License = "" }, wantErr: domain.ErrMissingProvenance},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := validRecord()
			tc.mutate(&r)
			err := r.Validate()

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() = %v, want error matching %v", err, tc.wantErr)
			}
		})
	}
}

func TestRecordAcceptsUnspecifiedLicenceButNotAnEmptyOne(t *testing.T) {
	t.Parallel()

	// An undeclared licence is a fact eye records; a blank field is a bug.
	r := validRecord()
	r.Provenance.License = domain.LicenseUnspecified

	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() with unspecified licence = %v, want nil", err)
	}
}

func TestQualityValid(t *testing.T) {
	t.Parallel()

	for _, q := range []domain.Quality{
		domain.QualityOfficial,
		domain.QualityValidated,
		domain.QualityPreliminary,
		domain.QualityInferred,
	} {
		if !q.Valid() {
			t.Errorf("Quality(%q).Valid() = false, want true", q)
		}
	}

	if domain.Quality("official-ish").Valid() {
		t.Error("unknown quality should not validate")
	}
}
