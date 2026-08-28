package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

// Identity in eye answers two different questions, and conflating them is what
// made the original DedupeKey useless — see
// docs/adr/0009-identity-before-fusion.md.
//
//   - LocalKey:    is this the same thing this source told me about last time?
//   - Fingerprint: is this the same thing another source is telling me about?
//
// The first is what makes change detection possible. The second is what makes
// cross-source deduplication possible. This file computes the second, in the
// domain, because twelve adapters each inventing an identity scheme is how eye
// ended up with twelve keys that could only match a record against itself.

// titleBudget caps how much of a normalized title takes part in identity.
// Publishers append arbitrary tails; the opening words are what they agree on.
const titleBudget = 80

// geoPrecision is the number of decimal places kept from a coordinate, which is
// roughly a hundred metres. Two publishers rarely place the same incident on
// the same metre, and rarely disagree by a street.
const geoPrecision = 3

// Normalize reduces a title to the part two publishers would write the same
// way: folded, stripped of punctuation, and with its spacing collapsed.
//
// It matches wording, not meaning. Two publishers who chose different words for
// one happening stay apart, and recognising those is the fuzzy layer's job,
// with a threshold somebody can inspect.
func Normalize(s string) string {
	folded := Fold(s)

	var sb strings.Builder
	sb.Grow(len(folded))
	for _, r := range folded {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			sb.WriteRune(r)
		default:
			// Punctuation becomes a separator rather than vanishing, so
			// "A-4" stays two tokens and cannot fuse into "a4".
			sb.WriteRune(' ')
		}
	}

	return strings.Join(strings.Fields(sb.String()), " ")
}

// Fingerprint is the cross-source identity of what a record describes.
//
// It is built only from what does not change: the kind of thing, the words
// naming it, roughly where it is and roughly when. Status, severity, start time
// and counts are all deliberately absent, because **a field inside the identity
// is a field whose change can never be detected** — a changed value would
// produce a different identity, and therefore a different thing.
//
// The source takes no part. A fingerprint carrying it could only ever match a
// record against itself.
//
// A record eye cannot identify returns an empty fingerprint rather than a
// plausible one, so it stays ungrouped instead of joining something unrelated.
func (r Record) Fingerprint() string {
	title := Normalize(r.Title)
	if title == "" {
		return ""
	}
	if len(title) > titleBudget {
		title = title[:titleBudget]
	}

	return strings.Join([]string{r.Kind, title, r.coarsePosition(), r.identityDate()}, "|")
}

// coarsePosition rounds a position to about a hundred metres, so two
// publishers' coordinates for one incident land on the same key. A record with
// no position contributes nothing rather than a default that would group every
// positionless record together.
func (r Record) coarsePosition() string {
	if r.Position == nil {
		return ""
	}
	return fmt.Sprintf("%.*f,%.*f", geoPrecision, r.Position.Lat, geoPrecision, r.Position.Lon)
}

// identityDate is the day the record is about.
//
// ValidFrom when the source published one, because for a scheduled thing the
// interesting date is its own, not the day we happened to read it. The day
// rather than the hour, so an event moved half an hour keeps its identity and
// the move reads as a change.
//
// The cost is stated in ADR-0009: a concert postponed to another month reads as
// a disappearance and an appearance. A wider bucket would hide that, at the
// price of collapsing genuinely different events.
func (r Record) identityDate() string {
	when := r.ObservedAt
	if r.ValidFrom != nil {
		when = *r.ValidFrom
	}
	return when.UTC().Format("2006-01-02")
}

// HashHex is a short, stable digest of a string, used where an identity has to
// become a fixed-width identifier. Truncated to 16 hex characters: this names
// records in one person's store, not the contents of a security boundary.
func HashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}
