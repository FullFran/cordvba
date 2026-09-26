package datex

import (
	"encoding/xml"
	"io"
	"strings"
)

// newDecoder builds an XML decoder for a body that httpx has already
// transcoded to UTF-8.
//
// The CharsetReader is a pass-through: encoding/xml refuses a document that
// declares a non-UTF-8 encoding unless one is supplied, and the declaration in
// these feeds is stale by the time we parse them. Strict is off because DGT's
// profile carries namespace prefixes we do not bind.
func newDecoder(body string) *xml.Decoder {
	dec := xml.NewDecoder(strings.NewReader(body))
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	dec.Strict = false
	return dec
}
