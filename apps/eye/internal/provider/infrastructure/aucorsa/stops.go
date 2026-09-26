package aucorsa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
)

// Stop is one bus stop as the operator's own directory names it.
//
// The ID is the number printed on the pole, which is also what the arrivals
// endpoint expects. That is worth stating because the operator's web pages
// carry a second, unrelated number — an internal page identifier — and using
// the wrong one silently returns no arrivals rather than an error.
type Stop struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// suggestion is the directory's own shape.
type suggestion struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Link  string `json:"link"`
}

// trailingID matches the number the directory appends to every label, which is
// the stop's own ID repeated and so is noise once the ID is a field of its own.
var trailingID = regexp.MustCompile(`\s*\(\d+\)\s*$`)

// LookupStops resolves a search term against the operator's stop directory.
//
// This is what makes the arrivals reader usable: stops are searched by the name
// on the pole rather than configured as opaque numbers nobody can verify.
func (p *RealtimeProvider) LookupStops(ctx context.Context, term string) ([]Stop, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("%w: no stop name to look up", ErrAucorsa)
	}

	token, err := p.currentToken(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/autocompletion/stop?term=%s&_wpnonce=%s",
		strings.TrimSuffix(p.src.URL, "/"), url.QueryEscape(term), token)

	resp, err := p.client.Get(ctx, endpoint, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: stop directory: %w", ErrAucorsa, err)
	}

	// The directory declines the same way the arrivals endpoint does: HTTP 200
	// with a body that is not data.
	body := strings.TrimSpace(string(resp.Body))
	if body == "-1" || body == "false" {
		p.invalidateToken()
		return nil, fmt.Errorf("%w: the stop directory declined the request", ErrAucorsa)
	}

	var found []suggestion
	if err := json.Unmarshal(resp.Body, &found); err != nil {
		return nil, fmt.Errorf("%w: the stop directory no longer answers in the shape this reader expects: %w",
			ErrAucorsa, err)
	}

	out := make([]Stop, 0, len(found))
	for _, s := range found {
		if s.ID == "" {
			continue
		}
		out = append(out, Stop{
			ID:   s.ID,
			Name: strings.TrimSpace(trailingID.ReplaceAllString(clean(s.Label), "")),
			URL:  s.Link,
		})
	}
	return out, nil
}
