package api

import "net/http"

// Route is one endpoint of the API.
//
// The table below is the single description of what eye serves: the mux is
// registered from it and the OpenAPI document is generated from it, so a route
// cannot exist in one and be missing from the other. That is the whole reason
// the table exists rather than a list of mux.HandleFunc calls.
type Route struct {
	// Method is the HTTP verb. Everything eye serves is a GET: this is a
	// read API over public observations.
	Method string
	// Path is the mux pattern, which is also the OpenAPI path.
	Path string
	// Summary is one line describing what the endpoint answers.
	Summary string
	// Params are the query parameters the endpoint reads.
	Params []Param
	// Open marks a route reachable without a token. Probes and the
	// self-description need to be, or a health check reports the deployment
	// down the moment the token rotates.
	Open bool

	handler http.HandlerFunc
}

// Param is one query parameter of a route.
type Param struct {
	Name        string
	Description string
	Required    bool
}

// filterParams are the query parameters shared by every endpoint that selects
// a slice of what eye knows.
func filterParams() []Param {
	return []Param{
		{Name: "topic", Description: "comma-separated topics"},
		{Name: "source", Description: "comma-separated source ids"},
		{Name: "kind", Description: "comma-separated record kinds"},
		{Name: "since", Description: "RFC 3339 timestamp, or a duration such as 2h meaning ago"},
		{Name: "until", Description: "RFC 3339 timestamp, or a duration meaning ago"},
		{Name: "bbox", Description: "west,south,east,north in WGS84"},
		{Name: "near", Description: "lat,lon in WGS84"},
		{Name: "radius_km", Description: "radius around near, in kilometres"},
		{Name: "text", Description: "free text over title and description, accent folded"},
		{Name: "min_severity", Description: "0..5"},
		{Name: "limit", Description: "maximum items, capped at 5000"},
	}
}

// routes is the route table.
func (s *Server) routes() []Route {
	return []Route{
		{
			Method: http.MethodGet, Path: "/health", Open: true,
			Summary: "Liveness, uptime and what the store holds",
			handler: s.handleHealth,
		},
		{
			Method: http.MethodGet, Path: "/", Open: true,
			Summary: "The web console, or the service description when no console is built in",
			handler: s.handleRoot,
		},
		{
			Method: http.MethodGet, Path: "/openapi.json", Open: true,
			Summary: "This API, as an OpenAPI 3.1 document",
			handler: s.handleOpenAPI,
		},
		{
			Method: http.MethodGet, Path: "/v1",
			Summary: "The versioned index: every endpoint and what it answers",
			handler: s.handleV1Index,
		},
		{
			Method: http.MethodGet, Path: "/v1/records",
			Summary: "Observations, newest first, with their provenance",
			Params:  filterParams(),
			handler: s.handleRecords,
		},
		{
			Method: http.MethodGet, Path: "/v1/entities",
			Summary: "Inventory: the things observations are about",
			Params:  filterParams(),
			handler: s.handleEntities,
		},
		{
			Method: http.MethodGet, Path: "/v1/sources",
			Summary: "The registry: what eye may read, and how each source is doing",
			handler: s.handleSources,
		},
		{
			Method: http.MethodGet, Path: "/v1/stats",
			Summary: "Aggregate counts by topic, source and kind, and the span held",
			handler: s.handleStats,
		},
		{
			Method: http.MethodGet, Path: "/v1/changes",
			Summary: "What appeared, changed or stopped being published",
			Params: []Param{
				{Name: "since", Description: "RFC 3339 timestamp, or a duration such as 24h"},
				{Name: "source", Description: "comma-separated source ids"},
				{Name: "topic", Description: "comma-separated topics"},
				{Name: "limit", Description: "maximum items, capped at 5000"},
			},
			handler: s.handleChanges,
		},
		{
			Method: http.MethodGet, Path: "/v1/geojson",
			Summary: "Records with a location, as an RFC 7946 FeatureCollection",
			Params:  filterParams(),
			handler: s.handleGeoJSON,
		},
		{
			Method: http.MethodGet, Path: "/v1/entities.geojson",
			Summary: "Entities with a location, as an RFC 7946 FeatureCollection",
			Params:  filterParams(),
			handler: s.handleEntitiesGeoJSON,
		},
		{
			Method: http.MethodGet, Path: "/v1/transit/lines",
			Summary: "Cordoba's bus lines, with their service hours and stop count",
			Params: []Param{
				{Name: "text", Description: "match a line code or name"},
				{Name: "limit", Description: "maximum items"},
			},
			handler: s.handleTransitLines,
		},
		{
			Method: http.MethodGet, Path: "/v1/transit/stops",
			Summary: "Bus stops held in the store",
			Params: []Param{
				{Name: "text", Description: "match a stop name, accent folded"},
				{Name: "near", Description: "lat,lon in WGS84"},
				{Name: "radius_km", Description: "radius around near, in kilometres"},
				{Name: "bbox", Description: "west,south,east,north in WGS84"},
				{Name: "limit", Description: "maximum items"},
			},
			handler: s.handleTransitStops,
		},
		{
			Method: http.MethodGet, Path: "/v1/transit/arrivals",
			Summary: "Live arrival estimates at a stop. Personal source: never redistributed",
			Params: []Param{
				{Name: "stop", Description: "comma-separated stop numbers as printed on the pole"},
				{Name: "text", Description: "stop name to resolve instead, at most 5 matches"},
			},
			handler: s.handleTransitArrivals,
		},
		{
			Method: http.MethodGet, Path: "/v1/transit/departures",
			Summary: "Published train departures from a station",
			Params: []Param{
				{Name: "station", Description: "station name, default CORDOBA"},
				{Name: "limit", Description: "maximum departures"},
			},
			handler: s.handleTransitDepartures,
		},
	}
}
