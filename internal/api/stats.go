package api

import (
	"net/http"
	"time"
)

// handleStats serves aggregate counts over everything the store holds.
//
// The counts come from the store rather than from a page of records: a total
// computed from the first five thousand rows is not a total, and a dashboard
// built on one is confidently wrong.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	aggregator, ok := s.store.(Aggregator)
	if !ok {
		writeError(w, http.StatusNotImplemented, "this store cannot aggregate")
		return
	}

	agg, err := aggregator.Aggregate(r.Context())
	if err != nil {
		s.log.Error("aggregate failed", "error", err)
		writeError(w, http.StatusInternalServerError, "aggregate failed")
		return
	}

	body := map[string]any{
		"records":     agg.Records,
		"entities":    agg.Entities,
		"sources":     len(s.sources),
		"by_topic":    orEmpty(agg.ByTopic),
		"by_source":   orEmpty(agg.BySource),
		"by_kind":     orEmpty(agg.ByKind),
		"oldest":      optionalTime(agg.Oldest),
		"newest":      optionalTime(agg.Newest),
		"store_bytes": s.storeBytes(),
	}

	writeJSON(w, http.StatusOK, body)
}

// storeBytes reports the size of the store on disk, or zero when the store has
// no disk to speak of.
func (s *Server) storeBytes() int64 {
	sizer, ok := s.store.(Sizer)
	if !ok {
		return 0
	}
	size, err := sizer.StoreBytes()
	if err != nil {
		s.log.Warn("store size unavailable", "error", err)
		return 0
	}
	return size
}

// orEmpty renders an absent map as an empty object rather than as null, so a
// client can iterate it without checking.
func orEmpty(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}

// optionalTime renders a zero time as absent. A store that has never held a
// record has no oldest observation, and a zero timestamp is a lie with a date
// on it.
func optionalTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
