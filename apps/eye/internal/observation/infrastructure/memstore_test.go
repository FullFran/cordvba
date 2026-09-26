package infrastructure_test

import (
	"context"
	"testing"
	"time"

	domain "github.com/FullFran/eye/internal/observation/domain"
	store "github.com/FullFran/eye/internal/observation/infrastructure"
)

// record builds a valid record with the given id and time.
func record(id string, at time.Time) domain.Record {
	return domain.Record{
		ID:         id,
		Source:     "test",
		Kind:       "news_item",
		Topic:      "press",
		ObservedAt: at,
		FetchedAt:  at,
		Title:      "Title " + id,
		Quality:    domain.QualityOfficial,
		Confidence: 1,
		Provenance: domain.Provenance{
			Publisher: "Test", SourceURL: "https://example.org", License: "unspecified", FetchedAt: at,
		},
	}
}

func TestAppendIgnoresDuplicates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := store.NewMemStore()
	now := time.Now().UTC()

	added, err := s.Append(ctx, []domain.Record{record("a", now), record("b", now)})
	if err != nil || added != 2 {
		t.Fatalf("Append() = %d, %v; want 2, nil", added, err)
	}

	// Polling again returns the same feed entries; they must not pile up.
	added, err = s.Append(ctx, []domain.Record{record("a", now), record("c", now)})
	if err != nil || added != 1 {
		t.Fatalf("second Append() = %d, %v; want 1, nil", added, err)
	}

	records, _ := s.Len()
	if records != 3 {
		t.Errorf("stored %d records, want 3", records)
	}
}

func TestQueryReturnsNewestFirstWithLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := store.NewMemStore()
	now := time.Now().UTC()

	_, _ = s.Append(ctx, []domain.Record{
		record("old", now.Add(-3*time.Hour)),
		record("newest", now),
		record("middle", now.Add(-time.Hour)),
	})

	got, err := s.Query(ctx, domain.Filter{Limit: 2})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	if got[0].ID != "newest" || got[1].ID != "middle" {
		t.Errorf("order = %s, %s; want newest, middle", got[0].ID, got[1].ID)
	}
}

func TestUpsertKeepsFirstSeen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := store.NewMemStore()

	first := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	later := first.Add(200 * 24 * time.Hour)

	entity := func(seen time.Time) domain.Entity {
		return domain.Entity{
			ID: "cam-1", Source: "test", Kind: "camera", Topic: "transport",
			Title: "Puente de Andalucía", FirstSeen: seen, LastSeen: seen,
			Provenance: domain.Provenance{
				Publisher: "Test", SourceURL: "https://example.org", License: "unspecified", FetchedAt: seen,
			},
		}
	}

	if added, err := s.Upsert(ctx, []domain.Entity{entity(first)}); err != nil || added != 1 {
		t.Fatalf("Upsert() = %d, %v", added, err)
	}
	if added, err := s.Upsert(ctx, []domain.Entity{entity(later)}); err != nil || added != 0 {
		t.Fatalf("second Upsert() = %d, %v; want 0 new", added, err)
	}

	got, err := s.Entities(ctx, domain.Filter{})
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entities = %d, want 1", len(got))
	}
	if !got[0].FirstSeen.Equal(first) {
		t.Errorf("FirstSeen = %v, want the original %v", got[0].FirstSeen, first)
	}
	if !got[0].LastSeen.Equal(later) {
		t.Errorf("LastSeen = %v, want the refreshed %v", got[0].LastSeen, later)
	}
}

func TestQueryAppliesFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := store.NewMemStore()
	now := time.Now().UTC()

	press := record("p", now)
	civic := record("c", now)
	civic.Topic = "civic"
	_, _ = s.Append(ctx, []domain.Record{press, civic})

	got, _ := s.Query(ctx, domain.Filter{Topics: []string{"civic"}})
	if len(got) != 1 || got[0].ID != "c" {
		t.Errorf("filtered query = %v, want only the civic record", got)
	}
}
