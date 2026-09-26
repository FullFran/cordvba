package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	provider "github.com/FullFran/eye/internal/provider/domain"
	providers "github.com/FullFran/eye/internal/provider/infrastructure"
	"github.com/FullFran/eye/internal/provider/infrastructure/aemet"
	"github.com/FullFran/eye/internal/provider/infrastructure/firms"
	source "github.com/FullFran/eye/internal/source/domain"
)

// src builds a registry entry with the given format and automation state.
func src(id, format string, automation source.AutomationStatus) source.Source {
	return source.Source{
		ID: id, Authority: "A", Topic: "press", URL: "https://example.org/rss",
		Format: format, License: "unspecified",
		Access: source.AccessDocumentedAPI, Automation: automation, Interval: time.Minute,
	}
}

func TestBuildKnownFormats(t *testing.T) {
	t.Parallel()

	client := httpx.New()
	for _, format := range providers.SupportedFormats() {
		p, err := providers.Build(src("s", format, source.AutomationEnabled), client, providers.Options{})
		if err != nil {
			t.Errorf("Build(%q) = %v", format, err)
			continue
		}
		if p.Info().Format != format {
			t.Errorf("adapter for %q reports format %q", format, p.Info().Format)
		}
	}
}

func TestBuildUnknownFormat(t *testing.T) {
	t.Parallel()

	_, err := providers.Build(src("s", "carrier-pigeon", source.AutomationEnabled), httpx.New(), providers.Options{})
	if !errors.Is(err, providers.ErrNoAdapter) {
		t.Fatalf("Build() = %v, want ErrNoAdapter", err)
	}
}

// Permitted and readable are different questions, and only their intersection
// gets fetched.
func TestBuildPollableAppliesBothGates(t *testing.T) {
	t.Parallel()

	list := []source.Source{
		src("readable-and-permitted", "rss", source.AutomationEnabled),
		src("permitted-but-unreadable", "datex2-3.7-xml", source.AutomationEnabled),
		src("readable-but-held", "rss", source.AutomationReviewTerms),
		src("held-and-unreadable", "wms", source.AutomationManualLink),
	}

	built, skipped := providers.BuildPollable(list, httpx.New(), providers.Options{})

	if len(built) != 1 || built[0].Info().ID != "readable-and-permitted" {
		t.Fatalf("built = %v, want only the readable and permitted source", ids(built))
	}
	if len(skipped) != 1 || skipped[0].ID != "permitted-but-unreadable" {
		t.Errorf("skipped = %v, want only the permitted source with no adapter", skipped)
	}
}

func TestSupported(t *testing.T) {
	t.Parallel()

	if !providers.Supported("rss") {
		t.Error("rss should be supported")
	}
	if providers.Supported("smoke-signals") {
		t.Error("an unknown format should not be supported")
	}
}

// ids extracts source ids for error messages.
func ids[T interface{ Info() source.Source }](items []T) []string {
	out := make([]string, 0, len(items))
	for _, i := range items {
		out = append(out, i.Info().ID)
	}
	return out
}

// An undocumented personal source needs the machine's consent as well as the
// registry's. A registry is committed to a repository; a machine flag is not,
// which is what keeps "I read this on my laptop" from becoming "eye reads this
// for everyone who clones it".
func TestBuildPollableGatesPersonalSourcesOnTheMachineToo(t *testing.T) {
	t.Parallel()

	personal := src("adif-live", "rss", source.AutomationEnabled)
	personal.Access = source.AccessUndocumentedPersonal
	personal.Notes = "Undocumented endpoint the operator reads for themselves."

	list := []source.Source{
		src("documented", "rss", source.AutomationEnabled),
		personal,
	}

	built, skipped := providers.BuildPollable(list, httpx.New(), providers.Options{})
	if len(built) != 1 || built[0].Info().ID != "documented" {
		t.Errorf("without the opt-in, built = %v", ids(built))
	}
	if len(skipped) != 1 || skipped[0].ID != "adif-live" {
		t.Errorf("the personal source was not reported as skipped: %v", skipped)
	}

	built, _ = providers.BuildPollable(list, httpx.New(), providers.Options{AllowPersonal: true})
	if len(built) != 2 {
		t.Errorf("with the opt-in, built = %v, want both", ids(built))
	}
}

// A source nobody can explain is one nobody can review.
func TestPersonalSourceMustExplainItself(t *testing.T) {
	t.Parallel()

	s := src("adif-live", "rss", source.AutomationEnabled)
	s.Access = source.AccessUndocumentedPersonal

	if err := s.Validate(); err == nil {
		t.Fatal("a personal source with no notes was accepted")
	}

	s.Notes = "Undocumented endpoint the operator reads for themselves."
	if err := s.Validate(); err != nil {
		t.Fatalf("a documented personal source was rejected: %v", err)
	}
}

// eye must never serve onward what it was not entitled to redistribute.
func TestPersonalSourcesAreNotRedistributable(t *testing.T) {
	t.Parallel()

	ordinary := src("boe", "rss", source.AutomationEnabled)
	if !ordinary.Redistributable() {
		t.Error("an ordinary source is not redistributable")
	}

	personal := ordinary
	personal.Access = source.AccessUndocumentedPersonal
	if personal.Redistributable() {
		t.Error("a personal source is redistributable")
	}
}

// The five formats this build learned to read. A registry entry marked
// automation: enabled and left without an adapter fetches nothing and says
// nothing, which is the one failure mode eye cannot afford.
func TestCredentialedFormatsAreRegistered(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"rest-json-two-step", "aemet-warnings", "aemet-observation", "rest-csv", "csv-dcat"} {
		if !providers.Supported(format) {
			t.Errorf("format %q still has no adapter", format)
		}
	}
}

// A credential is the machine's, not the registry's. It reaches the adapter
// through Options so that sources.yaml never has to hold a key.
func TestCredentialsReachTheAdapters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		format string
		opts   providers.Options
		want   error
	}{
		{
			name:   "aemet without a key",
			format: "aemet-warnings",
			want:   aemet.ErrMissingAPIKey,
		},
		{
			name:   "firms without a key",
			format: "rest-csv",
			want:   firms.ErrMissingMapKey,
		},
		{
			name:   "a two-step source that does not name its product",
			format: "rest-json-two-step",
			opts:   providers.Options{AEMETAPIKey: "a.b.c"},
			want:   aemet.ErrUnspecifiedProduct,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, err := providers.Build(src("s", tc.format, source.AutomationEnabled), httpx.New(), tc.opts)
			if err != nil {
				t.Fatalf("Build() = %v", err)
			}
			if _, err := p.Poll(context.Background()); !errors.Is(err, tc.want) {
				t.Fatalf("Poll() = %v, want %v", err, tc.want)
			}
		})
	}
}

// With the product named, the same registry format builds two different
// adapters, and the observation one is an inventory of stations as well.
func TestTwoStepFormatResolvesItsProduct(t *testing.T) {
	t.Parallel()

	warnings := src("aemet-warnings", "rest-json-two-step", source.AutomationEnabled)
	warnings.Options = map[string]string{"product": "warnings"}

	observation := src("aemet-observation", "rest-json-two-step", source.AutomationEnabled)
	observation.Options = map[string]string{"product": "observation"}

	opts := providers.Options{AEMETAPIKey: "a.b.c"}

	w, err := providers.Build(warnings, httpx.New(), opts)
	if err != nil {
		t.Fatalf("Build(warnings) = %v", err)
	}
	if _, ok := w.(provider.EntityProvider); ok {
		t.Error("the warnings adapter should not claim to publish an inventory")
	}

	o, err := providers.Build(observation, httpx.New(), opts)
	if err != nil {
		t.Fatalf("Build(observation) = %v", err)
	}
	if _, ok := o.(provider.EntityProvider); !ok {
		t.Error("the observation adapter publishes stations and must implement EntityProvider")
	}
}
