// Package rss is a generic syndication adapter. One parser serves every RSS
// 0.91, RSS 2.0 and Atom feed eye reads: local press, the BOE, and the
// University of Cordoba event listing.
package rss

import (
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrNotAFeed is returned when the payload parses as XML but is not syndication.
var ErrNotAFeed = errors.New("rss: not an RSS or Atom feed")

// Item is one entry, normalized across the three formats.
type Item struct {
	// Lat and Lon come from the W3C basic geo vocabulary, which IGN uses to
	// place each earthquake. Without them a seismic record has no position
	// and no spatial query can see it.
	Lat *float64
	Lon *float64

	Title       string
	Link        string
	Description string
	Author      string
	Categories  []string
	// GUID is the publisher's own identifier, when it provides one.
	GUID string
	// Published is the item's timestamp. For a news feed it is the
	// publication moment; for an event feed it is the event start.
	Published time.Time
}

// Feed is a parsed syndication document.
type Feed struct {
	Title string
	Link  string
	Items []Item
}

// rssDocument covers RSS 0.91 through 2.0. Unknown elements are ignored, which
// is what keeps a publisher adding a namespace from taking the provider down.
type rssDocument struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title string `xml:"title"`
		Link  string `xml:"link"`
		Items []struct {
			Title       string   `xml:"title"`
			Link        string   `xml:"link"`
			Description string   `xml:"description"`
			Creator     string   `xml:"creator"`
			Author      string   `xml:"author"`
			Categories  []string `xml:"category"`
			GUID        string   `xml:"guid"`
			PubDate     string   `xml:"pubDate"`
			Date        string   `xml:"date"`
			Lat         string   `xml:"lat"`
			Long        string   `xml:"long"`
			GeoPoint    string   `xml:"point"`
		} `xml:"item"`
	} `xml:"channel"`
}

// atomDocument covers Atom 1.0.
type atomDocument struct {
	XMLName xml.Name `xml:"feed"`
	Title   string   `xml:"title"`
	Links   []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
	} `xml:"link"`
	Entries []struct {
		Title string `xml:"title"`
		Links []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
		Summary   string `xml:"summary"`
		Content   string `xml:"content"`
		ID        string `xml:"id"`
		Updated   string `xml:"updated"`
		Published string `xml:"published"`
		Author    struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Categories []struct {
			Term string `xml:"term,attr"`
		} `xml:"category"`
	} `xml:"entry"`
}

// Parse reads a syndication document, trying RSS first and Atom second.
func Parse(body string) (*Feed, error) {
	if feed, err := parseRSS(body); err == nil {
		return feed, nil
	}
	if feed, err := parseAtom(body); err == nil {
		return feed, nil
	}
	return nil, ErrNotAFeed
}

// decodeXML unmarshals a document whose bytes are already UTF-8.
//
// encoding/xml refuses a document declaring a non-UTF-8 encoding unless a
// CharsetReader is supplied. httpx has already transcoded the body, so the
// declaration is stale and the reader is a pass-through — without it, every
// ISO-8859-1 feed (the BOE among them) fails to parse.
func decodeXML(body string, v any) error {
	dec := xml.NewDecoder(strings.NewReader(body))
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	dec.Strict = false
	return dec.Decode(v)
}

// parseRSS decodes an RSS document of any version.
func parseRSS(body string) (*Feed, error) {
	var doc rssDocument
	if err := decodeXML(body, &doc); err != nil {
		return nil, fmt.Errorf("rss: %w", err)
	}
	if len(doc.Channel.Items) == 0 && doc.Channel.Title == "" {
		return nil, ErrNotAFeed
	}

	feed := &Feed{
		Title: clean(doc.Channel.Title),
		Link:  clean(doc.Channel.Link),
		Items: make([]Item, 0, len(doc.Channel.Items)),
	}

	for _, it := range doc.Channel.Items {
		item := Item{
			Title:       clean(it.Title),
			Link:        clean(it.Link),
			Description: summarize(it.Description),
			Author:      clean(firstNonEmpty(it.Creator, it.Author)),
			Categories:  cleanAll(it.Categories),
			GUID:        clean(it.GUID),
			Published:   parseTime(firstNonEmpty(it.PubDate, it.Date)),
		}
		item.Lat, item.Lon = parseGeo(it.Lat, it.Long, it.GeoPoint)
		feed.Items = append(feed.Items, item)
	}
	return feed, nil
}

// parseAtom decodes an Atom 1.0 document.
func parseAtom(body string) (*Feed, error) {
	var doc atomDocument
	if err := decodeXML(body, &doc); err != nil {
		return nil, fmt.Errorf("atom: %w", err)
	}
	if len(doc.Entries) == 0 && doc.Title == "" {
		return nil, ErrNotAFeed
	}

	feed := &Feed{Title: clean(doc.Title), Items: make([]Item, 0, len(doc.Entries))}
	for _, l := range doc.Links {
		if l.Rel == "" || l.Rel == "alternate" {
			feed.Link = l.Href
			break
		}
	}

	for _, e := range doc.Entries {
		item := Item{
			Title:       clean(e.Title),
			Description: summarize(firstNonEmpty(e.Summary, e.Content)),
			Author:      clean(e.Author.Name),
			GUID:        clean(e.ID),
			Published:   parseTime(firstNonEmpty(e.Published, e.Updated)),
		}
		for _, l := range e.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				item.Link = l.Href
				break
			}
		}
		for _, c := range e.Categories {
			if c.Term != "" {
				item.Categories = append(item.Categories, c.Term)
			}
		}
		feed.Items = append(feed.Items, item)
	}
	return feed, nil
}

// parseGeo reads a position from the W3C basic geo vocabulary, accepting both
// the separate lat/long elements and the combined "lat lon" point form.
//
// A coordinate that does not parse is left absent rather than defaulted to
// zero: null island is a real place, and eye does not put earthquakes there.
func parseGeo(lat, lon, point string) (*float64, *float64) {
	if strings.TrimSpace(lat) == "" && strings.TrimSpace(point) != "" {
		if fields := strings.Fields(point); len(fields) == 2 {
			lat, lon = fields[0], fields[1]
		}
	}

	latVal, latErr := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	lonVal, lonErr := strconv.ParseFloat(strings.TrimSpace(lon), 64)
	if latErr != nil || lonErr != nil {
		return nil, nil
	}
	return &latVal, &lonVal
}

// timeLayouts covers what publishers actually emit, which is a superset of what
// the specifications require.
var timeLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	time.RFC3339,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// parseTime returns the zero time when no layout matches. A missing date is
// recorded as missing rather than guessed at.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// tagPattern strips the HTML that publishers put inside description elements.
var tagPattern = regexp.MustCompile(`<[^>]*>`)

// whitespacePattern collapses the runs of whitespace that stripping leaves.
var whitespacePattern = regexp.MustCompile(`\s+`)

// summarize turns an HTML description into a single line of plain text.
func summarize(s string) string {
	s = tagPattern.ReplaceAllString(s, " ")
	return clean(s)
}

// clean unescapes entities and collapses whitespace.
func clean(s string) string {
	s = html.UnescapeString(s)
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(s, " "))
}

// cleanAll applies clean to a slice, dropping entries that end up empty.
func cleanAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if c := clean(s); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// firstNonEmpty returns the first value that is not blank.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
