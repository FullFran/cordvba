package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
)

// changeRecord builds a stored change of the given kind.
func changeRecord(id, kind string, fields ...observation.FieldChange) observation.Record {
	r := record(id, "traffic")
	r.Kind = observation.ChangeKindRecord
	r.Quality = observation.QualityInferred
	payload, _ := json.Marshal(observation.Change{
		Kind: kind, RecordID: "dgt-incidents:" + id, Identity: "identity:" + id, Fields: fields,
	})
	r.Payload = payload
	return r
}

// The three kinds are the vocabulary the contract names, and a consumer
// switching on them must never meet a fourth spelling.
func TestChangesRenderTheContractKinds(t *testing.T) {
	t.Parallel()

	store := &fakeStore{records: []observation.Record{
		changeRecord("1", observation.ChangeAppeared),
		changeRecord("2", observation.ChangeUpdated,
			observation.FieldChange{Field: "severity", From: "2", To: "4"}),
		changeRecord("3", observation.ChangeDisappeared),
	}}

	srv := serve(t, store)
	status, body := get(t, srv, "/v1/changes")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}

	changes, _ := body["changes"].([]any)
	if len(changes) != 3 {
		t.Fatalf("changes = %v", body)
	}

	seen := map[string]bool{}
	for _, c := range changes {
		entry, _ := c.(map[string]any)
		kind, _ := entry["kind"].(string)
		seen[kind] = true

		if entry["record"] == nil {
			t.Errorf("change %v carries no record", entry)
		}
	}
	for _, want := range []string{"new", "changed", "gone"} {
		if !seen[want] {
			t.Errorf("kind %q is missing from %v", want, seen)
		}
	}
}

// Only change records: the endpoint asks the store for them by kind rather
// than fetching everything and sorting it out afterwards.
func TestChangesAsksTheStoreForChangeRecords(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	srv := serve(t, store)

	get(t, srv, "/v1/changes?since=6h&source=dgt-incidents&topic=traffic&limit=9") //nolint:dogsled // asserting on the filter

	f := store.lastFilter
	if len(f.Kinds) != 1 || f.Kinds[0] != observation.ChangeKindRecord {
		t.Errorf("kinds = %v, want only change records", f.Kinds)
	}
	if f.Since == nil {
		t.Error("since was dropped")
	}
	if len(f.Sources) != 1 || f.Sources[0] != "dgt-incidents" {
		t.Errorf("sources = %v", f.Sources)
	}
	if len(f.Topics) != 1 || f.Topics[0] != "traffic" {
		t.Errorf("topics = %v", f.Topics)
	}
	if f.Limit != 9 {
		t.Errorf("limit = %d", f.Limit)
	}
}

// A change is only worth reading if you can see what moved.
func TestChangesCarryThePreviousValues(t *testing.T) {
	t.Parallel()

	store := &fakeStore{records: []observation.Record{
		changeRecord("2", observation.ChangeUpdated,
			observation.FieldChange{Field: "severity", From: "2", To: "4"}),
	}}

	srv := serve(t, store)
	_, body := get(t, srv, "/v1/changes")

	changes, _ := body["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("changes = %v", body)
	}
	entry, _ := changes[0].(map[string]any)

	previous, _ := entry["previous"].(map[string]any)
	if previous["severity"] != "2" {
		t.Errorf("previous = %v, want the value the field held before", previous)
	}
	fields, _ := entry["fields"].([]any)
	if len(fields) != 1 {
		t.Errorf("fields = %v", entry["fields"])
	}
}

// A change derived from a record eye may not redistribute is still that
// source's data. Deriving does not launder it.
func TestChangesNeverServeAPersonalSource(t *testing.T) {
	t.Parallel()

	personal := changeRecord("secret", observation.ChangeAppeared)
	personal.Source = "aucorsa-arrivals"

	srv := serveWithPersonalSource(t, &fakeStore{records: []observation.Record{
		changeRecord("public", observation.ChangeAppeared), personal,
	}})

	_, body := get(t, srv, "/v1/changes")
	changes, _ := body["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want only the redistributable one", len(changes))
	}
}

// A change whose payload can no longer be read is not a change eye can
// report. Skipping it beats serving a blank entry.
func TestChangesSkipUnreadablePayloads(t *testing.T) {
	t.Parallel()

	broken := record("broken", "traffic")
	broken.Kind = observation.ChangeKindRecord
	broken.Payload = json.RawMessage(`not json`)

	srv := serve(t, &fakeStore{records: []observation.Record{broken}})
	status, body := get(t, srv, "/v1/changes")

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if body["count"] != float64(0) {
		t.Errorf("count = %v, want the unreadable change skipped", body["count"])
	}
}
