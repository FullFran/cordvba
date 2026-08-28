package cli

import (
	"strings"
	"testing"
	"time"

	observation "github.com/FullFran/eye/internal/observation/domain"
)

// observationEntityWithID builds a bare entity for naming tests.
func observationEntityWithID(id string) observation.Entity {
	return observation.Entity{ID: id}
}

func TestAgeOf(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 28, 21, 38, 0, 0, time.UTC)

	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "zero time", at: time.Time{}, want: "—"},
		{name: "seconds", at: now.Add(-8 * time.Second), want: "8s"},
		{name: "minutes", at: now.Add(-14 * time.Minute), want: "14m"},
		{name: "hours", at: now.Add(-5 * time.Hour), want: "5h"},
		{name: "days", at: now.Add(-72 * time.Hour), want: "3d"},
		{name: "future event", at: now.Add(48 * time.Hour), want: "in 2d"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ageOf(tc.at, now); got != tc.want {
				t.Errorf("ageOf() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEllipsis(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{name: "fits", in: "Velá", width: 10, want: "Velá"},
		{name: "collapses whitespace", in: "Velá   de  la\nFuensanta", width: 40, want: "Velá de la Fuensanta"},
		{name: "truncates on rune boundary", in: "Diputación de Córdoba", width: 10, want: "Diputació…"},
		{name: "width one", in: "abc", width: 1, want: "a"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ellipsis(tc.in, tc.width); got != tc.want {
				t.Errorf("ellipsis() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPlural(t *testing.T) {
	t.Parallel()

	if got := plural(1, "source", "sources"); got != "1 source" {
		t.Errorf("plural(1) = %q", got)
	}
	if got := plural(3, "source", "sources"); got != "3 sources" {
		t.Errorf("plural(3) = %q", got)
	}
	if got := plural(0, "source", "sources"); got != "0 sources" {
		t.Errorf("plural(0) = %q", got)
	}
}

func TestFrameName(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, id, want string }{
		{name: "dgt id", id: "dgt-cameras:421", want: "camera-421"},
		{name: "cordoba id", id: "cordoba-cameras:num.7", want: "camera-num-7"},
		{name: "no prefix", id: "421", want: "camera-421"},
		{name: "path characters are stripped", id: "s:../../etc/passwd", want: "camera-------etc-passwd"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := frameName(observationEntityWithID(tc.id))
			if got != tc.want {
				t.Errorf("frameName(%q) = %q, want %q", tc.id, got, tc.want)
			}
			// Whatever the id contains, the result must not be able to
			// escape the directory it is joined to.
			if strings.ContainsAny(got, "/\\") {
				t.Errorf("frameName(%q) = %q contains a path separator", tc.id, got)
			}
		})
	}
}
