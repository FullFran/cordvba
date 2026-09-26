package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
)

// pickerPageSize is how many options are shown at once. More than this and the
// list stops being something you can scan.
const pickerPageSize = 15

// cordobaCentre is the default point cameras are ranked from.
var cordobaCentre = observation.Point{Lat: 37.8882, Lon: -4.7794}

// pickCamera runs an interactive chooser over the inventory.
//
// It is a numbered list rather than an arrow-key interface, and deliberately
// so: raw terminal mode needs a dependency this project has not decided to
// spend yet (see ADR-0004), and a numbered list works over ssh, inside tmux,
// and in a terminal that does nothing clever. Typing a filter narrows; typing a
// number chooses.
func pickCamera(cams []observation.Entity, centre observation.Point, stdin io.Reader, stdout io.Writer) (observation.Entity, error) {
	if len(cams) == 0 {
		return observation.Entity{}, fmt.Errorf("no cameras in the inventory — run `eye daemon --once` first")
	}

	sortByDistance(cams, centre)

	// The picker exists to choose something to look at, so it opens on the
	// cameras that publish an image. The municipal ones are nearer but show
	// nothing, and leading with a dozen dead ends is not helpful. A filter
	// still reaches every camera in the inventory.
	viewable := withImage(cams)
	opening := viewable
	if len(opening) == 0 {
		opening = cams
	}
	visible := opening

	reader := bufio.NewReader(stdin)

	for {
		renderPicker(stdout, visible, centre, len(cams), len(cams)-len(viewable))

		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return observation.Entity{}, fmt.Errorf("nothing selected")
		}

		answer := strings.TrimSpace(line)
		switch {
		case answer == "" && len(visible) > 0:
			// Enter with no input takes the first, which is the nearest.
			return visible[0], nil
		case answer == "q", answer == "quit":
			return observation.Entity{}, fmt.Errorf("nothing selected")
		}

		if n, convErr := strconv.Atoi(answer); convErr == nil {
			if n >= 1 && n <= len(visible) && n <= pickerPageSize {
				return visible[n-1], nil
			}
			_, _ = fmt.Fprintf(stdout, "\nThere is no %d in the list. Type a number from the list, or a filter.\n", n)
			continue
		}

		// Anything else is a filter over the full inventory, not over what
		// is currently shown: narrowing should never be a dead end.
		filtered := filterCameras(cams, answer)
		if len(filtered) == 0 {
			_, _ = fmt.Fprintf(stdout, "\nNothing matches %q. Back to the start.\n", answer)
			visible = opening
			continue
		}
		visible = filtered
	}
}

// renderPicker draws the current page of choices.
func renderPicker(w io.Writer, visible []observation.Entity, centre observation.Point, total, imageless int) {
	shown := visible
	if len(shown) > pickerPageSize {
		shown = shown[:pickerPageSize]
	}

	_, _ = fmt.Fprintln(w)
	for i, cam := range shown {
		distance := "     "
		if cam.Position != nil {
			distance = fmt.Sprintf("%5.1f", centre.DistanceKm(*cam.Position))
		}

		marker := " "
		if cameraImageURL(cam) == "" {
			// A municipal camera publishes a position and no image. Saying
			// so here saves the user choosing one and getting an error.
			marker = "·"
		}

		_, _ = fmt.Fprintf(w, " %2d%s %s km  %s\n", i+1, marker, distance, ellipsis(cam.Title, 52))
	}

	if len(visible) > pickerPageSize {
		_, _ = fmt.Fprintf(w, "    … %d more match. Type a filter to narrow.\n", len(visible)-pickerPageSize)
	}
	if len(visible) < total {
		_, _ = fmt.Fprintf(w, "    (%d of %d cameras", len(visible), total)
		if imageless > 0 {
			_, _ = fmt.Fprintf(w, "; %d publish a position but no image", imageless)
		}
		_, _ = fmt.Fprintln(w, ")")
	}
	if hasImagelessCamera(shown) {
		_, _ = fmt.Fprintln(w, "    · = position only, no image published")
	}

	_, _ = fmt.Fprint(w, "\nNumber to view, text to filter, Enter for the nearest, q to quit: ")
}

// hasImagelessCamera reports whether any shown camera lacks an image endpoint.
func hasImagelessCamera(cams []observation.Entity) bool {
	for _, c := range cams {
		if cameraImageURL(c) == "" {
			return true
		}
	}
	return false
}

// withImage keeps only the cameras that publish a still-image endpoint.
func withImage(cams []observation.Entity) []observation.Entity {
	out := make([]observation.Entity, 0, len(cams))
	for _, c := range cams {
		if cameraImageURL(c) != "" {
			out = append(out, c)
		}
	}
	return out
}

// filterCameras narrows the inventory by the same accent-folding, word-order-free
// rules the query flags use.
func filterCameras(cams []observation.Entity, text string) []observation.Entity {
	filter := observation.Filter{Text: text}

	out := make([]observation.Entity, 0, len(cams))
	for _, c := range cams {
		if filter.MatchEntity(c) {
			out = append(out, c)
		}
	}
	return out
}

// sortByDistance orders cameras nearest-first, putting ones that publish an
// image ahead of ones that only publish a position at the same distance.
func sortByDistance(cams []observation.Entity, centre observation.Point) {
	sort.SliceStable(cams, func(i, j int) bool {
		di, dj := distanceOf(centre, cams[i]), distanceOf(centre, cams[j])
		if di != dj {
			return di < dj
		}
		return cameraImageURL(cams[i]) != "" && cameraImageURL(cams[j]) == ""
	})
}

// isInteractive reports whether stdin is a terminal a person is typing into.
//
// The character-device check alone is not enough: /dev/null is a character
// device, and `go test` hands it to the process, so a naive check reports a
// terminal where there is nobody at all.
func isInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}

	devNull, err := os.Stat(os.DevNull)
	if err == nil && os.SameFile(info, devNull) {
		return false
	}
	return true
}
