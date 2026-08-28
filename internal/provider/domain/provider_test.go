package domain_test

import (
	"testing"
	"time"

	domain "github.com/FullFran/eye/internal/provider/domain"
)

func TestHealthStale(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 28, 21, 38, 0, 0, time.UTC)

	cases := []struct {
		name      string
		health    domain.Health
		tolerance time.Duration
		want      bool
	}{
		{
			name:      "never succeeded is stale",
			health:    domain.Health{SourceID: "saih"},
			tolerance: time.Hour,
			want:      true,
		},
		{
			name:      "fresh within tolerance",
			health:    domain.Health{SourceID: "dgt", LastSuccess: now.Add(-30 * time.Second)},
			tolerance: time.Minute,
			want:      false,
		},
		{
			name:      "exactly at tolerance is not yet stale",
			health:    domain.Health{SourceID: "dgt", LastSuccess: now.Add(-time.Minute)},
			tolerance: time.Minute,
			want:      false,
		},
		{
			name:      "past tolerance is stale",
			health:    domain.Health{SourceID: "aemet", LastSuccess: now.Add(-2 * time.Hour)},
			tolerance: time.Hour,
			want:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.health.Stale(now, tc.tolerance); got != tc.want {
				t.Errorf("Stale() = %v, want %v", got, tc.want)
			}
		})
	}
}
