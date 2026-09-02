package api

import (
	"net/http"

	"github.com/FullFran/eye/internal/version"
)

// openAPIVersion is the specification this document claims to follow.
const openAPIVersion = "3.1.0"

// Document is an OpenAPI 3.1 description of this API.
//
// It is a Go value rather than a string blob so that it is generated from the
// same route table the mux is registered from. A hand-written document is a
// contract nobody checks, and it starts lying on the first pull request that
// adds an endpoint.
type Document struct {
	OpenAPI    string              `json:"openapi"`
	Info       Info                `json:"info"`
	Servers    []ServerURL         `json:"servers,omitempty"`
	Paths      map[string]PathItem `json:"paths"`
	Components Components          `json:"components"`
}

// Info is the document's metadata.
type Info struct {
	Title       string   `json:"title"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	License     *License `json:"license,omitempty"`
}

// License names the terms the API itself is offered under. The data carries
// its own licence, per record.
type License struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// ServerURL is one base URL the API is served from.
type ServerURL struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// PathItem holds the operations available on one path.
type PathItem struct {
	Get *Operation `json:"get,omitempty"`
}

// Operation is one endpoint.
type Operation struct {
	OperationID string                `json:"operationId"`
	Summary     string                `json:"summary"`
	Tags        []string              `json:"tags,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	Responses   map[string]Response   `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
}

// Parameter is one query parameter.
type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Schema      Schema `json:"schema"`
}

// Schema is the minimal type description this document needs.
type Schema struct {
	Type string `json:"type"`
}

// Response is one documented outcome.
type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// MediaType is the body shape of a response.
type MediaType struct {
	Schema Schema `json:"schema"`
}

// Components holds the reusable pieces of the document.
type Components struct {
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecurityScheme describes how a caller authenticates.
type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	Description  string `json:"description,omitempty"`
}

// handleOpenAPI serves the document.
func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.openAPI())
}

// openAPI generates the document from the route table.
func (s *Server) openAPI() Document {
	doc := Document{
		OpenAPI: openAPIVersion,
		Info: Info{
			Title:   "eye",
			Version: version.Version,
			Description: "A live, source-verifiable model of Córdoba, from public sources only. " +
				"Every record carries its publisher, its licence and both timestamps: when the " +
				"phenomenon was observed and when eye fetched it. Camera images are never served, " +
				"and records from undocumented personal sources are never redistributed.",
			License: &License{Name: "See the license field on every record"},
		},
		Paths: map[string]PathItem{},
		Components: Components{
			SecuritySchemes: map[string]SecurityScheme{
				"bearerAuth": {
					Type: "http", Scheme: "bearer", BearerFormat: "opaque",
					Description: "The value of EYE_API_TOKEN. Required on /v1 when the " +
						"deployment configures one; absent means the API is open, which " +
						"eye serve only permits on a loopback bind.",
				},
			},
		},
	}

	for _, rt := range s.routes() {
		operation := &Operation{
			OperationID: operationID(rt),
			Summary:     rt.Summary,
			Parameters:  parametersOf(rt),
			Responses:   responsesFor(rt),
		}
		// Only a guarded deployment declares the requirement. An open one
		// that claimed to need a token would be describing a different
		// service from the one it is.
		if !rt.Open && s.token != "" {
			operation.Security = []map[string][]string{{"bearerAuth": {}}}
		}
		doc.Paths[rt.Path] = PathItem{Get: operation}
	}

	return doc
}

// parametersOf renders a route's query parameters.
func parametersOf(rt Route) []Parameter {
	if len(rt.Params) == 0 {
		return nil
	}

	out := make([]Parameter, 0, len(rt.Params))
	for _, p := range rt.Params {
		out = append(out, Parameter{
			Name: p.Name, In: "query", Description: p.Description,
			Required: p.Required, Schema: Schema{Type: "string"},
		})
	}
	return out
}

// responsesFor documents the outcomes a route actually produces.
func responsesFor(rt Route) map[string]Response {
	jsonBody := map[string]MediaType{"application/json": {Schema: Schema{Type: "object"}}}

	responses := map[string]Response{
		"200": {Description: "The answer", Content: jsonBody},
	}
	if len(rt.Params) > 0 {
		responses["400"] = Response{Description: "A parameter naming what is wrong with it", Content: jsonBody}
	}
	if !rt.Open {
		responses["401"] = Response{Description: "Missing or wrong bearer token", Content: jsonBody}
	}
	if rt.Path == "/v1/transit/arrivals" {
		responses["403"] = Response{
			Description: "A personal source this deployment may not redistribute (ADR-0008)",
			Content:     jsonBody,
		}
		responses["502"] = Response{Description: "The operator's endpoint did not answer", Content: jsonBody}
	}
	if rt.Path == "/v1/transit/departures" {
		responses["502"] = Response{Description: "The reader did not answer", Content: jsonBody}
	}
	if rt.Path == "/v1/transit/arrivals" || rt.Path == "/v1/transit/departures" {
		responses["503"] = Response{Description: "No transit reader is wired into this build", Content: jsonBody}
	}
	if rt.Path == "/v1/stats" {
		responses["501"] = Response{Description: "This store cannot aggregate", Content: jsonBody}
	}
	return responses
}

// operationID renders a stable identifier for a route, which is what a
// generated client names its method after.
func operationID(rt Route) string {
	switch rt.Path {
	case "/":
		return "getIndex"
	case "/health":
		return "getHealth"
	case "/openapi.json":
		return "getOpenAPI"
	}

	id := "get"
	for _, part := range splitPath(rt.Path) {
		id += title(part)
	}
	return id
}

// splitPath breaks a path into its identifier-safe parts.
func splitPath(path string) []string {
	var (
		out     []string
		current []rune
	)
	for _, r := range path {
		if r == '/' || r == '.' || r == '-' || r == '_' {
			if len(current) > 0 {
				out = append(out, string(current))
				current = nil
			}
			continue
		}
		current = append(current, r)
	}
	if len(current) > 0 {
		out = append(out, string(current))
	}
	return out
}

// title upper-cases the first letter of a word.
func title(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	if runes[0] >= 'a' && runes[0] <= 'z' {
		runes[0] -= 'a' - 'A'
	}
	return string(runes)
}
