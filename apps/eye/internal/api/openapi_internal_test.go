package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	observation "github.com/FullFran/eye/internal/observation/domain"
)

// emptyStore is the smallest thing that satisfies the port.
type emptyStore struct{}

func (emptyStore) Query(context.Context, observation.Filter) ([]observation.Record, error) {
	return nil, nil
}

func (emptyStore) Entities(context.Context, observation.Filter) ([]observation.Entity, error) {
	return nil, nil
}

func (emptyStore) Counts(context.Context) (int, int, error) { return 0, 0, nil }

func (emptyStore) States(context.Context) ([]SourceState, error) { return nil, nil }

// A generated document that does not describe the routes the server actually
// registers is worse than no document: it is a contract nobody checked.
func TestOpenAPIDescribesEveryRegisteredRoute(t *testing.T) {
	t.Parallel()

	srv := New(emptyStore{}, nil, nil)
	doc := srv.openAPI()

	for _, rt := range srv.routes() {
		item, ok := doc.Paths[rt.Path]
		if !ok {
			t.Errorf("route %s %s is registered but absent from the document", rt.Method, rt.Path)
			continue
		}
		if item.Get == nil {
			t.Errorf("path %s has no GET operation", rt.Path)
			continue
		}
		if item.Get.Summary != rt.Summary {
			t.Errorf("path %s summary = %q, want %q", rt.Path, item.Get.Summary, rt.Summary)
		}
		if len(item.Get.Parameters) != len(rt.Params) {
			t.Errorf("path %s declares %d parameters, the route reads %d",
				rt.Path, len(item.Get.Parameters), len(rt.Params))
		}
	}

	if len(doc.Paths) != len(srv.routes()) {
		t.Errorf("document has %d paths for %d routes", len(doc.Paths), len(srv.routes()))
	}
}

// Every path in the document has to answer. A route table that drifts from the
// mux would still generate a document; this is what catches that.
func TestEveryDocumentedPathIsRouted(t *testing.T) {
	t.Parallel()

	server := New(emptyStore{}, nil, nil)
	handler := server.Handler()

	for path := range server.openAPI().Paths {
		w := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		handler.ServeHTTP(w, r)

		if w.Code == http.StatusNotFound {
			t.Errorf("documented path %s is not routed", path)
		}
	}
}

func TestOpenAPIIsServedAndWellFormed(t *testing.T) {
	t.Parallel()

	handler := New(emptyStore{}, nil, nil).Handler()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/openapi.json", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the document is not JSON: %v", err)
	}
	if version, _ := doc["openapi"].(string); !strings.HasPrefix(version, "3.1") {
		t.Errorf("openapi = %v, want 3.1", doc["openapi"])
	}
	if doc["info"] == nil || doc["paths"] == nil {
		t.Errorf("document = %v", doc)
	}
}

// A guarded deployment has to say so in its own description, or a client has
// no way to learn it needs a token except by being refused.
func TestOpenAPIDeclaresTheBearerSchemeWhenGuarded(t *testing.T) {
	t.Parallel()

	doc := New(emptyStore{}, nil, nil, WithToken("s3cret")).openAPI()

	if doc.Components.SecuritySchemes["bearerAuth"].Scheme != "bearer" {
		t.Fatalf("security schemes = %+v", doc.Components.SecuritySchemes)
	}

	guardedPath := doc.Paths["/v1/records"]
	if guardedPath.Get == nil || len(guardedPath.Get.Security) == 0 {
		t.Error("/v1/records is guarded but the document does not say so")
	}
	if open := doc.Paths["/health"]; open.Get != nil && len(open.Get.Security) != 0 {
		t.Error("/health is open but the document requires a token for it")
	}
}
