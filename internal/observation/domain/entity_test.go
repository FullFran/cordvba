package domain_test

import (
	"errors"
	"testing"
	"time"

	domain "github.com/FullFran/eye/internal/observation/domain"
)

// validEntity returns an entity that passes Validate, for tests to mutate.
func validEntity() domain.Entity {
	first := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)

	return domain.Entity{
		ID:        "cordoba:camera:17",
		Source:    "cordoba-ckan-cameras",
		Kind:      "camera",
		Topic:     "transport",
		Title:     "Avenida del Aeropuerto",
		Position:  &domain.Point{Lat: 37.8720, Lon: -4.8010},
		FirstSeen: first,
		LastSeen:  first.Add(27 * 24 * time.Hour),
		Provenance: domain.Provenance{
			Publisher: "Ayuntamiento de Cordoba",
			SourceURL: "https://datosabiertos.cordoba.es/ckan/dataset/camaras-de-trafico",
			License:   domain.LicenseUnspecified,
			FetchedAt: first,
		},
	}
}

func TestEntityValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*domain.Entity)
		wantErr error
	}{
		{name: "valid entity", mutate: func(*domain.Entity) {}},
		{name: "empty id", mutate: func(e *domain.Entity) { e.ID = "" }, wantErr: domain.ErrInvalidEntity},
		{name: "empty source", mutate: func(e *domain.Entity) { e.Source = "" }, wantErr: domain.ErrInvalidEntity},
		{name: "empty kind", mutate: func(e *domain.Entity) { e.Kind = "" }, wantErr: domain.ErrInvalidEntity},
		{name: "empty topic", mutate: func(e *domain.Entity) { e.Topic = "" }, wantErr: domain.ErrInvalidEntity},
		{
			name:    "position outside WGS84",
			mutate:  func(e *domain.Entity) { e.Position = &domain.Point{Lon: -181} },
			wantErr: domain.ErrInvalidEntity,
		},
		{
			name:    "last seen before first seen",
			mutate:  func(e *domain.Entity) { e.LastSeen = e.FirstSeen.Add(-time.Hour) },
			wantErr: domain.ErrInvalidEntity,
		},
		{
			name:    "missing provenance",
			mutate:  func(e *domain.Entity) { e.Provenance = domain.Provenance{} },
			wantErr: domain.ErrMissingProvenance,
		},
		{
			name:   "never seen again is fine",
			mutate: func(e *domain.Entity) { e.LastSeen = time.Time{} },
		},
		{
			name:   "entity without position is fine",
			mutate: func(e *domain.Entity) { e.Position = nil },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := validEntity()
			tc.mutate(&e)
			err := e.Validate()

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

func TestSeverityValid(t *testing.T) {
	t.Parallel()

	for s := domain.SeverityNone; s <= domain.SeverityCritical; s++ {
		if !s.Valid() {
			t.Errorf("Severity(%d).Valid() = false, want true", s)
		}
	}
	for _, s := range []domain.Severity{-1, 6, 100} {
		if s.Valid() {
			t.Errorf("Severity(%d).Valid() = true, want false", s)
		}
	}
}
