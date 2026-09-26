package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	"github.com/FullFran/eye/internal/render"
)

// maxFrameBytes caps a camera frame. A traffic still is tens of kilobytes; a
// response far larger than that is not a frame and is not worth decoding.
const maxFrameBytes = 8 << 20

// staleFrameAfter is when a frame stops being a useful statement about a road.
//
// DGT cameras refresh every five to twenty minutes, and some stop refreshing
// for weeks: one sampled camera had not updated in 53 days. Rendering that as
// the current state of a road is the exact lie this project exists not to tell,
// so age is always shown and anything past this threshold is called out.
const staleFrameAfter = 30 * time.Minute

// cameraCommand renders one public camera frame in the terminal.
//
// The frame lives in memory for as long as it takes to draw it and is then
// gone. There is no code path here that writes an image to disk, and none is to
// be added — see ADR-0007.
func cameraCommand() Command {
	return Command{
		Name:    "camera",
		Summary: "Show a public traffic camera frame, with the age of the image",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("camera", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
				dataDir   = fs.String("data-dir", "", "override where the store and raw cache live")
				width     = fs.Int("width", 0, "output width in characters (default: terminal width)")
				list      = fs.Bool("list", false, "list matching cameras instead of drawing one")
				nearest   = fs.String("near", "", "pick the camera nearest to lat,lon")
				open      = fs.Bool("open", false, "show the frame in the system image viewer instead of the terminal")
				window    = fs.Bool("window", false, "open a graphical terminal outside tmux and draw the frame there")
				hold      = fs.Bool("hold", false, "wait for a keypress before exiting (used by --window)")
				pick      = fs.Bool("pick", false, "browse and choose a camera interactively")
			)
			positional, err := parseInterspersed(fs, args)
			if err != nil {
				return err
			}

			query := strings.TrimSpace(strings.Join(positional, " "))

			rt, err := newRuntime(runtimeOptions{registry: *registryP, dataDir: *dataDir})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			cams, err := findCameras(ctx, rt, query, *nearest)
			if err != nil {
				return err
			}
			if len(cams) == 0 {
				if query == "" {
					return errors.New("no cameras in the inventory — run `eye daemon --once` first")
				}
				return fmt.Errorf("no camera matches %q — try a road like A-4, or run `eye camera` with no arguments to browse", query)
			}

			opts := showOptions{list: *list, window: *window, open: *open, hold: *hold, width: *width}

			// With no search and no coordinates, browse rather than
			// guess. Picking one of 1980 cameras from memory is not a
			// reasonable thing to ask of anybody.
			if *pick || (query == "" && *nearest == "" && !opts.list) {
				if !*pick && !isInteractive() {
					return errors.New("say which camera: eye camera <road, province or id>, --near lat,lon, or run it in a terminal to browse")
				}

				centre := cordobaCentre
				chosen, pickErr := pickCamera(cams, centre, os.Stdin, stdout)
				if pickErr != nil {
					return pickErr
				}
				cams, opts.list = []observation.Entity{chosen}, false
			}

			return showCamera(ctx, cams, query, opts, stdout, stderr)
		},
	}
}

// showOptions are the output choices for a camera.
type showOptions struct {
	list   bool
	window bool
	open   bool
	hold   bool
	width  int
}

// showCamera decides how to present the matches.
func showCamera(ctx context.Context, cams []observation.Entity, query string, opts showOptions, stdout, stderr io.Writer) error {
	// Too many matches to guess between: list them rather than pick one.
	if opts.list || (query != "" && len(cams) > 12) {
		return listCameras(stdout, cams)
	}

	cam := cams[0]

	if opts.window {
		ref := query
		if ref == "" {
			ref = cam.Title
		}
		return openInTerminal(ctx, ref, nil, stderr)
	}

	if err := drawCamera(ctx, stdout, cam, opts.width, opts.open, stderr); err != nil {
		return err
	}
	if opts.hold {
		waitForKey(stdout)
	}
	return nil
}

// findCameras selects cameras from the inventory by text or by proximity.
func findCameras(ctx context.Context, rt *runtime, query, near string) ([]observation.Entity, error) {
	filter := observation.Filter{Kinds: []string{"camera"}, Text: query}

	if near != "" {
		point, err := parsePoint(near)
		if err != nil {
			return nil, err
		}
		filter.Text = ""
		filter.Near = &point
		filter.RadiusKm = 50
	}

	cams, err := rt.store.Entities(ctx, filter)
	if err != nil {
		return nil, err
	}

	if filter.Near != nil {
		centre := *filter.Near
		sort.Slice(cams, func(i, j int) bool {
			return distanceOf(centre, cams[i]) < distanceOf(centre, cams[j])
		})
	}
	return cams, nil
}

// distanceOf is a camera's distance from a point, or +Inf when it has none.
func distanceOf(centre observation.Point, e observation.Entity) float64 {
	if e.Position == nil {
		return 1e18
	}
	return centre.DistanceKm(*e.Position)
}

// parsePoint reads a "lat,lon" flag value.
func parsePoint(s string) (observation.Point, error) {
	lat, lon, ok := strings.Cut(s, ",")
	if !ok {
		return observation.Point{}, fmt.Errorf("--near takes lat,lon, got %q", s)
	}

	latVal, err1 := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	lonVal, err2 := strconv.ParseFloat(strings.TrimSpace(lon), 64)
	if err1 != nil || err2 != nil {
		return observation.Point{}, fmt.Errorf("--near takes numeric lat,lon, got %q", s)
	}

	p := observation.Point{Lat: latVal, Lon: lonVal}
	if !p.Valid() {
		return observation.Point{}, fmt.Errorf("--near %q is outside WGS84", s)
	}
	return p, nil
}

// listCameras prints the matches so the user can narrow down.
func listCameras(w io.Writer, cams []observation.Entity) error {
	for _, c := range cams[:min(len(cams), 40)] {
		id := c.ID
		if _, rest, ok := strings.Cut(id, ":"); ok {
			id = rest
		}
		_, _ = fmt.Fprintf(w, "  %-10s %s\n", id, ellipsis(c.Title, 60))
	}
	if len(cams) > 40 {
		_, _ = fmt.Fprintf(w, "  … and %d more\n", len(cams)-40)
	}
	_, _ = fmt.Fprintf(w, "\n%s. Narrow the search, or pass an id.\n", plural(len(cams), "match", "matches"))
	return nil
}

// drawCamera fetches one frame and renders it with its age.
//
// Three ways of showing it, in order of fidelity: the system image viewer, the
// Kitty graphics protocol, and coloured half-blocks. The caption is identical
// in all three, because the age is not decoration.
func drawCamera(ctx context.Context, w io.Writer, cam observation.Entity, width int, useViewer bool, stderr io.Writer) error {
	imageURL := cameraImageURL(cam)
	if imageURL == "" {
		return fmt.Errorf("%s publishes no image, only a position", cam.Title)
	}

	frame, err := fetchFrame(ctx, imageURL)
	if err != nil {
		return err
	}

	if useViewer {
		if err := writeFrameCaption(w, cam, frame); err != nil {
			return err
		}
		return openInViewer(ctx, frame.body, frameName(cam), stderr)
	}

	img, err := render.Decode(bytes.NewReader(frame.body))
	if err != nil {
		return err
	}

	if width <= 0 {
		width = terminalWidth()
	}

	if render.SupportsKitty() {
		if drawn, err := render.Kitty(img, width); err == nil {
			_, _ = fmt.Fprint(w, drawn)
			return writeFrameCaption(w, cam, frame)
		}
		// Fall through to half-blocks: a terminal that advertises the
		// protocol but cannot encode is still owed a picture.
	}

	_, _ = fmt.Fprint(w, render.HalfBlocks(img, render.Bounds{Cols: width, Rows: width / 3}))
	return writeFrameCaption(w, cam, frame)
}

// frameName is a filesystem-safe name for a frame handed to a viewer.
func frameName(cam observation.Entity) string {
	id := cam.ID
	if _, rest, ok := strings.Cut(id, ":"); ok {
		id = rest
	}
	return "camera-" + strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, id)
}

// waitForKey holds a spawned window open until the user is done looking.
func waitForKey(w io.Writer) {
	_, _ = fmt.Fprint(w, "\nPress Enter to close. ")
	var discard [1]byte
	_, _ = os.Stdin.Read(discard[:])
}

// frame is one fetched image, held in memory only.
type frame struct {
	body         []byte
	lastModified time.Time
	fetchedAt    time.Time
	contentType  string
}

// age is how old the image was when the publisher last wrote it.
func (f frame) age(now time.Time) (time.Duration, bool) {
	if f.lastModified.IsZero() {
		return 0, false
	}
	return now.Sub(f.lastModified), true
}

// fetchFrame retrieves a camera image.
//
// It uses its own HTTP client with NO recorder attached. The shared client
// writes every payload into the raw cache, which persists by design — exactly
// the wrong behaviour for an image of a public road. This is the mechanism
// behind ADR-0007's "RAM only", not a comment asking someone to be careful.
func fetchFrame(ctx context.Context, url string) (frame, error) {
	client := httpx.New(httpx.WithMaxBytes(maxFrameBytes))

	resp, err := client.Get(ctx, url, httpx.Validators{})
	if err != nil {
		return frame{}, fmt.Errorf("fetch frame: %w", err)
	}

	if ct := resp.ContentType; ct != "" && !strings.HasPrefix(ct, "image/") {
		return frame{}, fmt.Errorf("%s returned %s, not an image", url, ct)
	}

	f := frame{body: resp.Body, fetchedAt: resp.FetchedAt, contentType: resp.ContentType}
	if lm := resp.Validators.LastModified; lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			f.lastModified = t.UTC()
		}
	}
	return f, nil
}

// writeFrameCaption prints what the image is, how old it is, and who owns it.
func writeFrameCaption(w io.Writer, cam observation.Entity, f frame) error {
	now := time.Now().UTC()

	_, _ = fmt.Fprintf(w, "\n%s\n", cam.Title)
	if cam.Position != nil {
		_, _ = fmt.Fprintf(w, "%.4f, %.4f\n", cam.Position.Lat, cam.Position.Lon)
	}

	switch age, ok := f.age(now); {
	case !ok:
		_, _ = fmt.Fprintf(w, "\nIMAGE AGE   unknown — the publisher sent no timestamp\n")
	case age > staleFrameAfter:
		// Loud on purpose. Some cameras stop refreshing for weeks, and a
		// stale frame shown as current is worse than no frame at all.
		_, _ = fmt.Fprintf(w, "\nIMAGE AGE   %s — STALE. This is not the current state of the road.\n",
			ageOf(f.lastModified, now))
	default:
		_, _ = fmt.Fprintf(w, "\nIMAGE AGE   %s (captured %s)\n",
			ageOf(f.lastModified, now), f.lastModified.Local().Format("15:04:05"))
	}

	_, _ = fmt.Fprintf(w, "SOURCE      %s · %s\n", cam.Provenance.Publisher, cam.Provenance.License)
	_, _ = fmt.Fprintf(w, "IMAGE       %s\n", cameraImageURL(cam))
	_, _ = fmt.Fprintln(w, "\nHeld in memory only. eye does not store camera images.")
	return nil
}

// cameraImageURL reads the still-image endpoint out of an entity's payload.
func cameraImageURL(e observation.Entity) string {
	var payload struct {
		ImageURL string `json:"image_url"`
	}
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return ""
	}
	return payload.ImageURL
}

// terminalWidth reports the usable width, falling back to a conservative
// default when stdout is not a terminal or the environment says nothing.
func terminalWidth() int {
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 20 {
			return min(n, 160)
		}
	}
	return render.DefaultBounds.Cols
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
