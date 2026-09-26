// Package domain holds the observation core of eye: the geo-temporal types
// every provider normalizes into, and the provenance that makes each one
// auditable. It imports nothing outside the standard library.
package domain

import "math"

// earthRadiusKm is the mean radius used for great-circle distance.
const earthRadiusKm = 6371.0

// Point is a WGS84 coordinate.
type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Valid reports whether the point falls inside the WGS84 domain.
func (p Point) Valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
}

// DistanceKm returns the great-circle distance between two points using the
// haversine formula. It is accurate enough for the city-scale radii eye works
// with and needs no external dependency.
func (p Point) DistanceKm(q Point) float64 {
	lat1, lat2 := rad(p.Lat), rad(q.Lat)
	dLat, dLon := rad(q.Lat-p.Lat), rad(q.Lon-p.Lon)

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)

	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(h)))
}

// BBox is an axis-aligned bounding box in WGS84, expressed the way the source
// APIs expect it: west, south, east, north.
type BBox struct {
	West  float64 `json:"west"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	North float64 `json:"north"`
}

// Valid reports whether the box is well formed and inside the WGS84 domain.
func (b BBox) Valid() bool {
	if b.West > b.East || b.South > b.North {
		return false
	}
	return Point{Lat: b.South, Lon: b.West}.Valid() &&
		Point{Lat: b.North, Lon: b.East}.Valid()
}

// Contains reports whether the point falls inside the box, edges included.
func (b BBox) Contains(p Point) bool {
	return p.Lat >= b.South && p.Lat <= b.North &&
		p.Lon >= b.West && p.Lon <= b.East
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }
