package aemet

import (
	"context"
	"errors"
	"fmt"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// ErrUnspecifiedProduct reports a registry entry that names AEMET's transport
// but not what to ask it for.
//
// "Two steps and a JSON envelope" describes warnings, station readings, tide
// tables and satellite imagery alike. Guessing between them would put one
// product's data under another product's source id, which is worse than an
// entry that plainly refuses to poll.
var ErrUnspecifiedProduct = errors.New(`aemet: the registry does not say which product this source is: set format to "aemet-warnings" or "aemet-observation", or options.product to "warnings" or "observation"`)

// NewProduct builds the adapter a two-step registry entry asks for.
func NewProduct(src source.Source, c *httpx.Client, apiKey string) provider.Provider {
	switch src.Option("product", "") {
	case "warnings", "avisos", "avisos_cap":
		return NewWarnings(src, c, apiKey)
	case "observation", "observacion":
		return NewObservation(src, c, apiKey)
	default:
		return &unspecified{src: src}
	}
}

// unspecified stands in for a source whose product the registry never named. It
// fails closed on every poll and says exactly what to write in the registry.
type unspecified struct {
	src source.Source
}

// Info implements provider.Provider.
func (p *unspecified) Info() source.Source { return p.src }

// Poll always fails, naming the registry change that would fix it.
func (p *unspecified) Poll(_ context.Context) ([]observation.Record, error) {
	return nil, fmt.Errorf("poll %s: %w", p.src.ID, ErrUnspecifiedProduct)
}

var _ provider.Provider = (*unspecified)(nil)
