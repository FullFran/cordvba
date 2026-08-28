package domain_test

import (
	"math"
	"testing"

	domain "github.com/FullFran/eye/internal/observation/domain"
)

// cordoba is the reference point eye centres its default viewport on.
var cordoba = domain.Point{Lat: 37.8882, Lon: -4.7794}

func TestPointValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		point domain.Point
		want  bool
	}{
		{name: "cordoba", point: cordoba, want: true},
		{name: "null island", point: domain.Point{}, want: true},
		{name: "latitude above pole", point: domain.Point{Lat: 90.1}, want: false},
		{name: "latitude below pole", point: domain.Point{Lat: -90.1}, want: false},
		{name: "longitude past antimeridian", point: domain.Point{Lon: 180.1}, want: false},
		{name: "swapped lat lon", point: domain.Point{Lat: -4.7794, Lon: 37.8882}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.point.Valid(); got != tc.want {
				t.Errorf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPointDistanceKm(t *testing.T) {
	t.Parallel()

	// Cordoba to Seville is roughly 120 km in a straight line.
	seville := domain.Point{Lat: 37.3891, Lon: -5.9845}

	cases := []struct {
		name      string
		from, to  domain.Point
		want      float64
		tolerance float64
	}{
		{name: "same point is zero", from: cordoba, to: cordoba, want: 0, tolerance: 0.001},
		{name: "cordoba to seville", from: cordoba, to: seville, want: 118, tolerance: 5},
		{name: "symmetric", from: seville, to: cordoba, want: 118, tolerance: 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.from.DistanceKm(tc.to)
			if math.Abs(got-tc.want) > tc.tolerance {
				t.Errorf("DistanceKm() = %.3f, want %.3f ± %.3f", got, tc.want, tc.tolerance)
			}
		})
	}
}

func TestBBox(t *testing.T) {
	t.Parallel()

	// The bounding box the deep-research report uses for FIRMS queries.
	province := domain.BBox{West: -5.6, South: 37.1, East: -4.0, North: 38.7}

	t.Run("valid box contains cordoba", func(t *testing.T) {
		t.Parallel()
		if !province.Valid() {
			t.Fatal("province box should be valid")
		}
		if !province.Contains(cordoba) {
			t.Error("province box should contain Cordoba")
		}
	})

	t.Run("rejects inverted box", func(t *testing.T) {
		t.Parallel()
		inverted := domain.BBox{West: -4.0, South: 37.1, East: -5.6, North: 38.7}
		if inverted.Valid() {
			t.Error("box with west > east should be invalid")
		}
	})

	t.Run("excludes point outside", func(t *testing.T) {
		t.Parallel()
		madrid := domain.Point{Lat: 40.4168, Lon: -3.7038}
		if province.Contains(madrid) {
			t.Error("province box should not contain Madrid")
		}
	})
}
