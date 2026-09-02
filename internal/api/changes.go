package api

import (
	"encoding/json"
	"net/http"

	observation "github.com/FullFran/eye/internal/observation/domain"
)

// changeKindNames maps eye's internal change vocabulary onto the one this API
// publishes. The wire names are part of the contract; the internal ones are
// free to be renamed without breaking a consumer.
var changeKindNames = map[string]string{
	observation.ChangeAppeared:    "new",
	observation.ChangeUpdated:     "changed",
	observation.ChangeDisappeared: "gone",
}

// changeView is one change as this API serves it.
type changeView struct {
	// Kind is one of new, changed, gone.
	Kind string `json:"kind"`
	// Record is the observation the change was noticed on. It carries the
	// subject's title, topic, position and provenance.
	Record observation.Record `json:"record"`
	// Previous holds the former value of each field that moved. eye stores
	// the diff rather than the whole prior observation, so this is a
	// partial projection and says only what actually changed.
	Previous map[string]string `json:"previous,omitempty"`
	// Fields is the same diff in full, with both sides of each move.
	Fields []observation.FieldChange `json:"fields,omitempty"`
	// Identity is the key the two observations were paired on, so a reader
	// can see why eye considered them the same thing.
	Identity string `json:"identity,omitempty"`
}

// handleChanges serves what appeared, changed or stopped being published.
//
// This is the difference between a reader and a watcher. Everything else here
// answers "what is there"; this answers "what is different", which is the
// question somebody who already looked an hour ago actually has.
func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
	filter, err := filterFrom(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// A change is itself an observation, stored as a record of this kind.
	// There is no second timeline — see ADR-0009.
	filter.Kinds = []string{observation.ChangeKindRecord}

	records, err := s.store.Query(r.Context(), filter)
	if err != nil {
		s.log.Error("change query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	records = s.redistributable(records)
	changes := make([]changeView, 0, len(records))

	for _, rec := range records {
		var change observation.Change
		if err := json.Unmarshal(rec.Payload, &change); err != nil {
			// A change eye can no longer read is not a change eye can
			// report. Skipping beats serving a blank entry.
			continue
		}
		kind, known := changeKindNames[change.Kind]
		if !known {
			continue
		}

		changes = append(changes, changeView{
			Kind: kind, Record: rec, Fields: change.Fields,
			Previous: previousValues(change.Fields), Identity: change.Identity,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"count": len(changes), "changes": changes})
}

// previousValues projects a field diff onto the values the record held before.
func previousValues(fields []observation.FieldChange) map[string]string {
	if len(fields) == 0 {
		return nil
	}

	out := make(map[string]string, len(fields))
	for _, f := range fields {
		out[f.Field] = f.From
	}
	return out
}
