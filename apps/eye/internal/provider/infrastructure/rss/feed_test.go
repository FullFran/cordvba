package rss_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/rss"
)

// loadFixture reads a recorded feed and decodes it the way the provider will.
func loadFixture(t *testing.T, name, contentType string) string {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/rss/" + name) // #nosec G304 -- fixture path built from a test literal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return (&httpx.Response{Body: body, ContentType: contentType}).DecodeUTF8()
}

func TestParseRSS2WithCDATA(t *testing.T) {
	t.Parallel()

	feed, err := rss.Parse(loadFixture(t, "diariocordoba.xml", "application/rss+xml; charset=utf-8"))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	if !strings.Contains(feed.Title, "Diario Córdoba") {
		t.Errorf("feed title = %q, want it to name the outlet", feed.Title)
	}
	if len(feed.Items) == 0 {
		t.Fatal("expected items")
	}

	first := feed.Items[0]
	if first.Title == "" {
		t.Error("item title is empty; CDATA was probably not unwrapped")
	}
	if !strings.HasPrefix(first.Link, "https://") {
		t.Errorf("item link = %q, want an absolute URL", first.Link)
	}
	if first.Published.IsZero() {
		t.Error("item has no parsed publication date")
	}
}

func TestParseRSS091(t *testing.T) {
	t.Parallel()

	// The UCO events feed is RSS 0.91 and uses pubDate for the event start.
	feed, err := rss.Parse(loadFixture(t, "uco-eventos.rss", "application/rss+xml; charset=utf-8"))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if len(feed.Items) == 0 {
		t.Fatal("expected items")
	}

	for _, it := range feed.Items {
		if it.Title == "" {
			t.Error("event with no title")
		}
		if it.Link == "" {
			t.Error("event with no link")
		}
	}
}

func TestParseISO8859Feed(t *testing.T) {
	t.Parallel()

	// The BOE serves ISO-8859-1. If the charset is mishandled, every
	// accented word becomes a replacement character.
	feed, err := rss.Parse(loadFixture(t, "boe.xml", "text/xml; charset=ISO-8859-1"))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	if !strings.Contains(feed.Title, "Boletín") {
		t.Errorf("feed title = %q, want the accented form decoded", feed.Title)
	}
	if strings.ContainsRune(feed.Title, '�') {
		t.Errorf("feed title = %q contains a replacement character", feed.Title)
	}
	if len(feed.Items) == 0 {
		t.Fatal("expected items")
	}
}

func TestParseAtom(t *testing.T) {
	t.Parallel()

	body := `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Ayuntamiento de Córdoba</title>
  <link href="https://www.cordoba.es/" rel="alternate"/>
  <entry>
    <title>Corte de tráfico en la Avenida del Gran Capitán</title>
    <link href="https://www.cordoba.es/aviso/1" rel="alternate"/>
    <id>urn:cordoba:aviso:1</id>
    <published>2026-08-28T09:30:00+02:00</published>
    <summary>Obras de reasfaltado.</summary>
    <author><name>Movilidad</name></author>
    <category term="movilidad"/>
  </entry>
</feed>`

	feed, err := rss.Parse(body)
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	if len(feed.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(feed.Items))
	}

	it := feed.Items[0]
	if it.Link != "https://www.cordoba.es/aviso/1" {
		t.Errorf("link = %q", it.Link)
	}
	if it.Author != "Movilidad" {
		t.Errorf("author = %q", it.Author)
	}
	if want := time.Date(2026, time.August, 28, 7, 30, 0, 0, time.UTC); !it.Published.Equal(want) {
		t.Errorf("published = %v, want %v", it.Published, want)
	}
	if len(it.Categories) != 1 || it.Categories[0] != "movilidad" {
		t.Errorf("categories = %v", it.Categories)
	}
}

func TestParseRejectsNonFeed(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		"",
		"not xml at all",
		`<html><body><h1>404</h1></body></html>`,
		`<?xml version="1.0"?><root><thing/></root>`,
	} {
		if _, err := rss.Parse(body); err == nil {
			t.Errorf("Parse(%q) = nil error, want a failure", body)
		}
	}
}

func TestSummarizeStripsHTML(t *testing.T) {
	t.Parallel()

	body := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"><channel><title>t</title><item>
  <title>Velá de la Fuensanta</title>
  <link>https://example.org/1</link>
  <description><![CDATA[<p>Plaza de la <strong>Fuensanta</strong></p>

  <p>Del 4 al 8.</p>]]></description>
</item></channel></rss>`

	feed, err := rss.Parse(body)
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	got := feed.Items[0].Description
	if want := "Plaza de la Fuensanta Del 4 al 8."; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
}

// IGN places each earthquake with the W3C basic geo vocabulary. Dropping those
// tags would leave every seismic record without a position, invisible to any
// spatial query.
func TestParseReadsGeoTags(t *testing.T) {
	t.Parallel()

	feed, err := rss.Parse(loadFixture(t, "ign-sismologia.xml", "application/xml; charset=utf-8"))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if len(feed.Items) == 0 {
		t.Fatal("expected items")
	}

	var located int
	for _, it := range feed.Items {
		if it.Lat == nil || it.Lon == nil {
			continue
		}
		located++
		// Peninsular Spain, generously bounded.
		if *it.Lat < 27 || *it.Lat > 44 || *it.Lon < -19 || *it.Lon > 5 {
			t.Errorf("%q placed at %.4f, %.4f — outside Spain", it.Title, *it.Lat, *it.Lon)
		}
	}
	if located == 0 {
		t.Error("no item carried a position; the geo tags were dropped")
	}
}

func TestParseGeoEdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		item    string
		wantPos bool
	}{
		{
			name:    "separate lat and long",
			item:    `<geo:lat>37.8882</geo:lat><geo:long>-4.7794</geo:long>`,
			wantPos: true,
		},
		{
			name:    "combined point",
			item:    `<geo:point>37.8882 -4.7794</geo:point>`,
			wantPos: true,
		},
		{
			name:    "no geo at all",
			item:    ``,
			wantPos: false,
		},
		{
			// Null island is a real place. An unparsable coordinate is
			// absent, never defaulted to zero.
			name:    "unparsable latitude is absent, not zero",
			item:    `<geo:lat>norte</geo:lat><geo:long>-4.7794</geo:long>`,
			wantPos: false,
		},
		{
			name:    "latitude without longitude",
			item:    `<geo:lat>37.8882</geo:lat>`,
			wantPos: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:geo="http://www.w3.org/2003/01/geo/wgs84_pos#"><channel><title>t</title>
<item><title>Terremoto</title><link>https://example.org/1</link>` + tc.item + `</item>
</channel></rss>`

			feed, err := rss.Parse(body)
			if err != nil {
				t.Fatalf("Parse() = %v", err)
			}

			got := feed.Items[0].Lat != nil && feed.Items[0].Lon != nil
			if got != tc.wantPos {
				t.Errorf("position present = %v, want %v", got, tc.wantPos)
			}
		})
	}
}
