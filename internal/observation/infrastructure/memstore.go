// Package infrastructure holds the storage adapters for observations.
//
// MemStore is the in-process adapter used by the query commands, which poll
// live and answer from what they just fetched. The durable SQLite adapter
// implements the same ports and lands with the daemon.
package infrastructure

import (
	"context"
	"sync"

	domain "github.com/FullFran/eye/internal/observation/domain"
)

// MemStore is a concurrency-safe in-memory implementation of both stores.
type MemStore struct {
	mu       sync.RWMutex
	records  map[string]domain.Record
	entities map[string]domain.Entity
}

// NewMemStore builds an empty store.
func NewMemStore() *MemStore {
	return &MemStore{
		records:  make(map[string]domain.Record),
		entities: make(map[string]domain.Entity),
	}
}

// Append stores records, skipping ones already present, and reports how many
// were new.
func (s *MemStore) Append(_ context.Context, records []domain.Record) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var added int
	for _, r := range records {
		if _, exists := s.records[r.ID]; exists {
			continue
		}
		s.records[r.ID] = r
		added++
	}
	return added, nil
}

// Query returns matching records, newest first.
func (s *MemStore) Query(_ context.Context, f domain.Filter) ([]domain.Record, error) {
	s.mu.RLock()
	out := make([]domain.Record, 0, len(s.records))
	for _, r := range s.records {
		if f.MatchRecord(r) {
			out = append(out, r)
		}
	}
	s.mu.RUnlock()

	domain.SortRecordsNewestFirst(out)
	return domain.ApplyLimit(out, f.Limit), nil
}

// Upsert stores entities, refreshing LastSeen and keeping the original
// FirstSeen for ones already known.
func (s *MemStore) Upsert(_ context.Context, entities []domain.Entity) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var added int
	for _, e := range entities {
		if existing, ok := s.entities[e.ID]; ok {
			e.FirstSeen = existing.FirstSeen
		} else {
			added++
		}
		s.entities[e.ID] = e
	}
	return added, nil
}

// Entities returns matching inventory.
func (s *MemStore) Entities(_ context.Context, f domain.Filter) ([]domain.Entity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.Entity, 0, len(s.entities))
	for _, e := range s.entities {
		if f.MatchEntity(e) {
			out = append(out, e)
		}
	}
	return domain.ApplyLimit(out, f.Limit), nil
}

// Len reports how many records and entities are held.
func (s *MemStore) Len() (records, entities int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records), len(s.entities)
}

var (
	_ domain.RecordStore = (*MemStore)(nil)
	_ domain.EntityStore = (*MemStore)(nil)
)
