package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// ageOf renders how old an observation is, compactly and without pretending to
// a precision the source does not have.
func ageOf(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "—"
	}

	d := now.Sub(t)
	future := d < 0
	if future {
		d = -d
	}

	var s string
	switch {
	case d < time.Minute:
		s = fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		s = fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		s = fmt.Sprintf("%dh", int(d.Hours()))
	default:
		s = fmt.Sprintf("%dd", int(d.Hours()/24))
	}

	if future {
		return "in " + s
	}
	return s
}

// ellipsis shortens a string to a column width on a rune boundary.
func ellipsis(s string, width int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

// writeJSON renders a value as indented JSON.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// plural returns the singular or plural form for a count.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, pluralForm)
}
