package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// mutablePortal serves one RSS feed whose contents the test controls, so the
// whole change-detection path can be exercised end to end rather than trusted.
type mutablePortal struct {
	srv   *httptest.Server
	items atomic.Pointer[[]string]
}

func newMutablePortal(t *testing.T, items ...string) *mutablePortal {
	t.Helper()

	p := &mutablePortal{}
	p.set(items...)

	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		var sb strings.Builder
		sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Prueba</title>`)
		for _, item := range *p.items.Load() {
			sb.WriteString(item)
		}
		sb.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(sb.String()))
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *mutablePortal) set(items ...string) {
	list := items
	p.items.Store(&list)
}

// article renders one feed entry.
func article(title, description string) string {
	return fmt.Sprintf(
		`<item><title>%s</title><link>https://example.test/%s</link><description>%s</description></item>`,
		title, title, description)
}

// changesRegistry points a single press source at the mutable portal.
func changesRegistry(t *testing.T, portal string) string {
	t.Helper()

	return writeRegistryBody(t, fmt.Sprintf(`
sources:
  - id: test-press
    authority: Diario de Prueba
    topic: press
    url: %s
    format: rss
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 30m
`, portal))
}

// The first look has no baseline, and saying "nothing changed" then would be
// claiming knowledge eye does not have.
func TestChangesSaysSoOnTheFirstLook(t *testing.T) {
	t.Parallel()

	portal := newMutablePortal(t, article("Incendio en la Ribera", "Un incendio"))
	registry := changesRegistry(t, portal.srv.URL)

	code, stdout, stderr := runCmd(t, registry, "changes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "first time eye has looked") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestChangesDetectsAnArticleAppearing(t *testing.T) {
	t.Parallel()

	portal := newMutablePortal(t, article("Incendio en la Ribera", "Un incendio"))
	registry := changesRegistry(t, portal.srv.URL)
	dataDir := t.TempDir()

	// Baseline.
	runCmdIn(t, registry, dataDir, "changes")

	portal.set(
		article("Incendio en la Ribera", "Un incendio"),
		article("Corte de agua en Ciudad Jardin", "Corte programado"),
	)

	code, stdout, stderr := runCmdIn(t, registry, dataDir, "changes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "NEW") || !strings.Contains(stdout, "Ciudad Jardin") {
		t.Errorf("the new article was not reported:\n%s", stdout)
	}
	// The one that did not move must not be reported alongside it.
	if strings.Count(stdout, "Incendio en la Ribera") != 0 {
		t.Errorf("an unchanged article was reported as a change:\n%s", stdout)
	}
}

func TestChangesDetectsAFieldMoving(t *testing.T) {
	t.Parallel()

	portal := newMutablePortal(t, article("Incendio en la Ribera", "Un incendio"))
	registry := changesRegistry(t, portal.srv.URL)
	dataDir := t.TempDir()

	runCmdIn(t, registry, dataDir, "changes")
	portal.set(article("Incendio en la Ribera", "Controlado a las 14:00"))

	code, stdout, stderr := runCmdIn(t, registry, dataDir, "changes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "CHANGED") {
		t.Errorf("the edit was not reported:\n%s", stdout)
	}
	// The diff has to show both sides, or it says nothing useful.
	if !strings.Contains(stdout, "description") || !strings.Contains(stdout, "Controlado") {
		t.Errorf("the field diff is missing:\n%s", stdout)
	}
}

func TestChangesDetectsAnArticleDropping(t *testing.T) {
	t.Parallel()

	portal := newMutablePortal(t,
		article("Incendio en la Ribera", "Un incendio"),
		article("Corte de agua", "Corte programado"),
	)
	registry := changesRegistry(t, portal.srv.URL)
	dataDir := t.TempDir()

	runCmdIn(t, registry, dataDir, "changes")
	portal.set(article("Incendio en la Ribera", "Un incendio"))

	code, stdout, stderr := runCmdIn(t, registry, dataDir, "changes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "GONE") || !strings.Contains(stdout, "Corte de agua") {
		t.Errorf("the dropped article was not reported:\n%s", stdout)
	}
}

// The whole discipline of the feature: a source that breaks says nothing.
func TestChangesTreatsABrokenSourceAsSilenceNotAnEnding(t *testing.T) {
	t.Parallel()

	portal := newMutablePortal(t, article("Incendio en la Ribera", "Un incendio"))
	registry := changesRegistry(t, portal.srv.URL)
	dataDir := t.TempDir()

	runCmdIn(t, registry, dataDir, "changes")

	// The feed breaks entirely.
	dead := changesRegistry(t, "http://127.0.0.1:1/rss")
	_, stdout, _ := runCmdIn(t, dead, dataDir, "changes")

	if strings.Contains(stdout, "GONE") {
		t.Errorf("a broken source was reported as things ending:\n%s", stdout)
	}
}

func TestChangesFiltersByKind(t *testing.T) {
	t.Parallel()

	portal := newMutablePortal(t, article("Incendio en la Ribera", "Un incendio"))
	registry := changesRegistry(t, portal.srv.URL)
	dataDir := t.TempDir()

	runCmdIn(t, registry, dataDir, "changes")
	portal.set(
		article("Incendio en la Ribera", "Controlado"),
		article("Corte de agua", "Corte programado"),
	)

	_, stdout, _ := runCmdIn(t, registry, dataDir, "changes", "--new")
	if !strings.Contains(stdout, "Corte de agua") {
		t.Errorf("--new dropped the new article:\n%s", stdout)
	}
	if strings.Contains(stdout, "CHANGED") {
		t.Errorf("--new showed an edit:\n%s", stdout)
	}
}

func TestChangesAsJSON(t *testing.T) {
	t.Parallel()

	portal := newMutablePortal(t, article("Incendio en la Ribera", "Un incendio"))
	registry := changesRegistry(t, portal.srv.URL)
	dataDir := t.TempDir()

	runCmdIn(t, registry, dataDir, "changes")
	portal.set(article("Incendio en la Ribera", "Un incendio"), article("Corte de agua", "Corte"))

	code, stdout, stderr := runCmdIn(t, registry, dataDir, "changes", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	for _, want := range []string{`"kind"`, `"appeared"`, `"identity"`, `"publisher"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("JSON is missing %s:\n%s", want, stdout)
		}
	}
}
