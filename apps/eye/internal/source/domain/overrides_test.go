package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// overrideRegistry is a small registry the override tests validate against.
func overrideRegistry() []domain.Source {
	enabled := validSource()
	enabled.ID = "metar-cordoba"
	enabled.Topic = "weather"

	held := validSource()
	held.ID = "saih-guadalquivir"
	held.Topic = "hydrology"
	held.Automation = domain.AutomationReviewTerms
	held.Interval = 0

	personal := validSource()
	personal.ID = "aucorsa-arrivals"
	personal.Topic = "transport"
	personal.Access = domain.AccessUndocumentedPersonal
	personal.Notes = "Undocumented endpoint the operator reads for themselves."

	return []domain.Source{enabled, held, personal}
}

func TestOverridesValidateRejectsUnknownIDs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		overrides domain.Overrides
		want      string
	}{
		{
			name:      "unknown id in only",
			overrides: domain.Overrides{Sources: domain.SourceOverrides{Only: []string{"does-not-exist"}}},
			want:      "unknown source id",
		},
		{
			name:      "unknown id in disable",
			overrides: domain.Overrides{Sources: domain.SourceOverrides{Disable: []string{"does-not-exist"}}},
			want:      "unknown source id",
		},
		{
			name:      "unknown topic in topics_only",
			overrides: domain.Overrides{Sources: domain.SourceOverrides{TopicsOnly: []string{"aviation"}}},
			want:      "unknown topic",
		},
		{
			name:      "unknown id in retention",
			overrides: domain.Overrides{Retention: map[string]domain.RetentionOverride{"does-not-exist": {Kind: domain.RetentionHistorical}}},
			want:      "unknown source id",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.overrides.Validate(overrideRegistry())
			if !errors.Is(err, domain.ErrInvalidOverrides) {
				t.Fatalf("Validate() = %v, want ErrInvalidOverrides", err)
			}
			if got := err.Error(); !contains(got, tc.want) {
				t.Errorf("error = %q, want it to mention %q", got, tc.want)
			}
		})
	}
}

// Naming the same id in both only and disable is a contradiction the operator
// must resolve, not a case for eye to guess at.
func TestOverridesValidateRejectsSameIDInOnlyAndDisable(t *testing.T) {
	t.Parallel()

	o := domain.Overrides{Sources: domain.SourceOverrides{
		Only:    []string{"metar-cordoba"},
		Disable: []string{"metar-cordoba"},
	}}

	err := o.Validate(overrideRegistry())
	if !errors.Is(err, domain.ErrInvalidOverrides) {
		t.Fatalf("Validate() = %v, want ErrInvalidOverrides", err)
	}
	if got := err.Error(); !contains(got, "metar-cordoba") || !contains(got, "both") {
		t.Errorf("error = %q, want it to name the conflicting entry", got)
	}
}

func TestOverridesValidateAcceptsAWellFormedFile(t *testing.T) {
	t.Parallel()

	o := domain.Overrides{
		Sources: domain.SourceOverrides{Only: []string{"metar-cordoba"}},
		Retention: map[string]domain.RetentionOverride{
			"metar-cordoba": {Kind: domain.RetentionHistorical},
		},
	}
	if err := o.Validate(overrideRegistry()); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

// Overrides restrict; they never grant. A held source stays held no matter how
// the operator's "only" list reads, because that decision belongs to the
// registry (and, for personal sources, to the machine), never to this file.
func TestOverridesCannotEnableAHeldSource(t *testing.T) {
	t.Parallel()

	sources := overrideRegistry()
	o := domain.Overrides{Sources: domain.SourceOverrides{Only: []string{"saih-guadalquivir"}}}
	if err := o.Validate(sources); err != nil {
		t.Fatalf("Validate() = %v, want nil (unknown-id checks pass)", err)
	}

	for _, s := range sources {
		if s.ID != "saih-guadalquivir" {
			continue
		}
		if o.Schedulable(s, false) {
			t.Error("Schedulable() = true for a source only the operator's list named, which the registry itself holds")
		}
	}
}

// Same rule, for the other kind of source ADR-0008 gates: an undocumented
// personal source without the machine's own opt-in.
func TestOverridesCannotEnableAPersonalSourceWithoutItsOptIn(t *testing.T) {
	t.Parallel()

	sources := overrideRegistry()
	o := domain.Overrides{Sources: domain.SourceOverrides{Only: []string{"aucorsa-arrivals"}}}

	var personal domain.Source
	for _, s := range sources {
		if s.ID == "aucorsa-arrivals" {
			personal = s
		}
	}

	// Permits() answers the overrides' own question; it is Overrides that can
	// only restrict, so the personal opt-in gate is composed by the caller
	// (see DisabledByOperator) — Permits alone must not smuggle it back in.
	if !o.Permits(personal) {
		t.Fatal("Permits() = false, want true: the only list names this id")
	}
	if o.DisabledByOperator(personal, false) {
		t.Error("DisabledByOperator() = true for a source the machine never opted into; that is not the operator's override at work")
	}
	if o.DisabledByOperator(personal, true) {
		t.Error("DisabledByOperator() = true once the only list names it and the machine opted in")
	}
	if o.Schedulable(personal, false) {
		t.Error("Schedulable() = true for an undocumented_personal source without the machine's opt-in, even though it is in the only list")
	}
	if !o.Schedulable(personal, true) {
		t.Error("Schedulable() = false once the machine opts in AND the only list names it")
	}
}

func TestOverridesPermitsOnly(t *testing.T) {
	t.Parallel()

	o := domain.Overrides{Sources: domain.SourceOverrides{Only: []string{"metar-cordoba"}}}
	sources := overrideRegistry()

	for _, s := range sources {
		want := s.ID == "metar-cordoba"
		if got := o.Permits(s); got != want {
			t.Errorf("Permits(%s) = %v, want %v", s.ID, got, want)
		}
	}
}

func TestOverridesPermitsDisable(t *testing.T) {
	t.Parallel()

	o := domain.Overrides{Sources: domain.SourceOverrides{Disable: []string{"metar-cordoba"}}}
	sources := overrideRegistry()

	for _, s := range sources {
		want := s.ID != "metar-cordoba"
		if got := o.Permits(s); got != want {
			t.Errorf("Permits(%s) = %v, want %v", s.ID, got, want)
		}
	}
}

func TestOverridesPermitsTopicsOnly(t *testing.T) {
	t.Parallel()

	o := domain.Overrides{Sources: domain.SourceOverrides{TopicsOnly: []string{"weather"}}}
	sources := overrideRegistry()

	for _, s := range sources {
		want := s.Topic == "weather"
		if got := o.Permits(s); got != want {
			t.Errorf("Permits(%s) = %v, want %v", s.ID, got, want)
		}
	}
}

func TestOverridesZeroValuePermitsEverything(t *testing.T) {
	t.Parallel()

	var o domain.Overrides
	for _, s := range overrideRegistry() {
		if !o.Permits(s) {
			t.Errorf("Permits(%s) = false, want true: no overrides means today's behaviour", s.ID)
		}
	}
}

func TestOverridesDisabledByOperator(t *testing.T) {
	t.Parallel()

	sources := overrideRegistry()
	var enabled, held domain.Source
	for _, s := range sources {
		switch s.ID {
		case "metar-cordoba":
			enabled = s
		case "saih-guadalquivir":
			held = s
		}
	}

	o := domain.Overrides{Sources: domain.SourceOverrides{Disable: []string{"metar-cordoba"}}}

	if !o.DisabledByOperator(enabled, false) {
		t.Error("DisabledByOperator() = false for a source the disable list names")
	}
	// A source the registry already holds is not "disabled by operator": that
	// label belongs only to a source the registry would otherwise schedule.
	if o.DisabledByOperator(held, false) {
		t.Error("DisabledByOperator() = true for a source the registry itself holds")
	}
}

func TestOverridesEffectiveRetentionWithoutOverride(t *testing.T) {
	t.Parallel()

	var o domain.Overrides
	s := overrideRegistry()[0]
	s.Retention = domain.RetentionHistorical

	kind, ttl, overridden := o.EffectiveRetention(s)
	if overridden {
		t.Fatal("overridden = true, want false: no override for this id")
	}
	if kind != domain.RetentionHistorical {
		t.Errorf("kind = %v, want the registry's own retention", kind)
	}
	if ttl != 0 {
		t.Errorf("ttl = %v, want zero", ttl)
	}
}

func TestOverridesEffectiveRetentionWithOverride(t *testing.T) {
	t.Parallel()

	s := overrideRegistry()[0]
	o := domain.Overrides{Retention: map[string]domain.RetentionOverride{
		s.ID: {Kind: domain.RetentionEphemeral, TTL: 24 * time.Hour},
	}}

	kind, ttl, overridden := o.EffectiveRetention(s)
	if !overridden {
		t.Fatal("overridden = false, want true")
	}
	if kind != domain.RetentionEphemeral {
		t.Errorf("kind = %v, want ephemeral", kind)
	}
	if ttl != 24*time.Hour {
		t.Errorf("ttl = %v, want 24h", ttl)
	}
}

// contains keeps the assertions above readable without repeating the package
// qualifier at every call site.
func contains(s, substr string) bool { return strings.Contains(s, substr) }
