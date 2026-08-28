package infrastructure_test

import (
	"errors"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	providers "github.com/FullFran/eye/internal/provider/infrastructure"
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
		p, err := providers.Build(src("s", format, source.AutomationEnabled), client)
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

	_, err := providers.Build(src("s", "carrier-pigeon", source.AutomationEnabled), httpx.New())
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

	built, skipped := providers.BuildPollable(list, httpx.New())

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
