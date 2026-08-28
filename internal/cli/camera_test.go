package cli_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/cli"
)

// cameraPortal serves a DATEX II device inventory and the JPEG it points at.
func cameraPortal(t *testing.T) (*httptest.Server, *int) {
	t.Helper()

	var frame bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 160, 90))
	for y := range 90 {
		for x := range 160 {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	if err := jpeg.Encode(&frame, img, nil); err != nil {
		t.Fatalf("encode frame: %v", err)
	}

	var frameRequests int
	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)

	mux.HandleFunc("/devices.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<d2:payload xsi:type="ns2:DevicePublication"
  xmlns:d2="http://levelC/schema/3/d2Payload"
  xmlns:ns2="http://levelC/schema/3/faultAndStatus"
  xmlns:loc="http://levelC/schema/3/locationReferencing"
  xmlns:lse="http://levelC/schema/3/locationExtension"
  xmlns:fse="http://levelC/schema/3/faultAndStatusSpanishExtension"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <ns2:device xsi:type="fse:ExtendedDevice" id="421" version="2">
    <ns2:typeOfDevice>camera</ns2:typeOfDevice>
    <ns2:lastUpdateOfDeviceInformation>2025-10-28T14:19:42.000+01:00</ns2:lastUpdateOfDeviceInformation>
    <ns2:pointLocation>
      <loc:supplementaryPositionalDescription>
        <loc:roadInformation><loc:roadDestination>SEVILLA</loc:roadDestination><loc:roadName>A-4</loc:roadName></loc:roadInformation>
      </loc:supplementaryPositionalDescription>
      <loc:tpegPointLocation xsi:type="loc:TpegSimplePoint">
        <loc:point xsi:type="loc:TpegNonJunctionPoint">
          <loc:pointCoordinates><loc:latitude>37.8900</loc:latitude><loc:longitude>-4.7449</loc:longitude></loc:pointCoordinates>
          <loc:_tpegNonJunctionPointExtension><loc:extendedTpegNonJunctionPoint>
            <lse:kilometerPoint>399.1</lse:kilometerPoint><lse:province>CÓRDOBA</lse:province>
          </loc:extendedTpegNonJunctionPoint></loc:_tpegNonJunctionPointExtension>
        </loc:point>
      </loc:tpegPointLocation>
    </ns2:pointLocation>
    <fse:deviceUrl>%s/frame.jpg</fse:deviceUrl>
  </ns2:device>
</d2:payload>`, srv.URL)
	})

	mux.HandleFunc("/frame.jpg", func(w http.ResponseWriter, _ *http.Request) {
		frameRequests++
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Last-Modified", time.Now().Add(-4*time.Minute).UTC().Format(http.TimeFormat))
		_, _ = w.Write(frame.Bytes())
	})

	srv.Start()
	t.Cleanup(srv.Close)
	return srv, &frameRequests
}

// writeCameraRegistry points a datex2-devices source at the fake portal.
func writeCameraRegistry(t *testing.T, portal string) string {
	t.Helper()

	return writeRegistryBody(t, fmt.Sprintf(`
sources:
  - id: dgt-cameras
    authority: Direccion General de Trafico
    topic: transport
    url: %s/devices.xml
    format: datex2-devices
    license: free-of-charge-nap-terms
    access: documented_api
    automation: enabled
    interval: 1h
`, portal))
}

func TestCameraRendersFrameWithItsAge(t *testing.T) {
	t.Parallel()

	srv, _ := cameraPortal(t)
	registry := writeCameraRegistry(t, srv.URL)
	dataDir := t.TempDir()

	if code, _, stderr := runCmdIn(t, registry, dataDir, "daemon", "--once"); code != 0 {
		t.Fatalf("inventory pass failed: %d %s", code, stderr)
	}

	code, stdout, stderr := runCmdIn(t, registry, dataDir, "camera", "A-4", "--width", "32")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}

	// The image itself.
	if !strings.Contains(stdout, "▀") {
		t.Error("no half-block output; the frame was not drawn")
	}
	// The caption, which is the part that must never be optional.
	for _, want := range []string{"A-4 km 399.1", "CÓRDOBA", "IMAGE AGE", "4m", "Held in memory only"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("caption is missing %q:\n%s", want, captionOf(stdout))
		}
	}
}

// A frame older than the staleness threshold must be called out, not shown as
// if it were the current state of the road. One real DGT camera had not
// refreshed in 53 days.
func TestCameraFlagsAStaleFrame(t *testing.T) {
	t.Parallel()

	var frame bytes.Buffer
	if err := jpeg.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 32, 24)), nil); err != nil {
		t.Fatalf("encode: %v", err)
	}

	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)
	mux.HandleFunc("/devices.xml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><d2:payload xmlns:d2="d" xmlns:ns2="n" xmlns:loc="l" xmlns:fse="f" xmlns:xsi="x">
  <ns2:device id="1"><ns2:typeOfDevice>camera</ns2:typeOfDevice>
    <ns2:pointLocation><loc:tpegPointLocation><loc:point><loc:pointCoordinates>
      <loc:latitude>37.89</loc:latitude><loc:longitude>-4.74</loc:longitude>
    </loc:pointCoordinates></loc:point></loc:tpegPointLocation></ns2:pointLocation>
    <fse:deviceUrl>%s/frame.jpg</fse:deviceUrl></ns2:device></d2:payload>`, srv.URL)
	})
	mux.HandleFunc("/frame.jpg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Last-Modified", time.Now().Add(-53*24*time.Hour).UTC().Format(http.TimeFormat))
		_, _ = w.Write(frame.Bytes())
	})
	srv.Start()
	defer srv.Close()

	registry := writeCameraRegistry(t, srv.URL)
	dataDir := t.TempDir()
	runCmdIn(t, registry, dataDir, "daemon", "--once") //nolint:dogsled // the pass only primes the store

	_, stdout, _ := runCmdIn(t, registry, dataDir, "camera", "camera", "--width", "24")

	if !strings.Contains(stdout, "STALE") {
		t.Errorf("a 53-day-old frame was not flagged as stale:\n%s", captionOf(stdout))
	}
	if !strings.Contains(stdout, "not the current state of the road") {
		t.Errorf("the caption does not say what stale means:\n%s", captionOf(stdout))
	}
}

// ADR-0007 says camera frames live in RAM and never reach disk. This is the
// test that makes that a mechanism rather than a promise: the shared HTTP
// client records every payload into the raw cache, and the camera path must
// deliberately bypass it.
func TestCameraWritesNoImageToDisk(t *testing.T) {
	t.Parallel()

	srv, requests := cameraPortal(t)
	registry := writeCameraRegistry(t, srv.URL)
	dataDir := t.TempDir()

	runCmdIn(t, registry, dataDir, "daemon", "--once") //nolint:dogsled // primes the inventory

	before := filesUnder(t, dataDir)

	if code, _, stderr := runCmdIn(t, registry, dataDir, "camera", "A-4", "--width", "16"); code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	if *requests == 0 {
		t.Fatal("the frame was never fetched; this test would pass vacuously")
	}

	for path, size := range filesUnder(t, dataDir) {
		if _, existed := before[path]; !existed {
			t.Errorf("fetching a frame created %s (%d bytes) — camera images must never reach disk", path, size)
		}
	}

	// And no file anywhere under the data dir may contain the JPEG magic.
	if err := filepath.WalkDir(dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry is not the subject of this test
		}
		body, readErr := os.ReadFile(path) // #nosec G304,G122 -- walking a directory this test created in t.TempDir()
		if readErr != nil {
			return nil
		}
		if bytes.Contains(body, []byte("\xff\xd8\xff")) {
			t.Errorf("%s contains JPEG data — a camera frame was persisted", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestCameraListsMatches(t *testing.T) {
	t.Parallel()

	srv, _ := cameraPortal(t)
	registry := writeCameraRegistry(t, srv.URL)
	dataDir := t.TempDir()
	runCmdIn(t, registry, dataDir, "daemon", "--once") //nolint:dogsled // primes the store

	// Accent-free and out of order, the way a person actually types.
	_, stdout, _ := runCmdIn(t, registry, dataDir, "camera", "cordoba a-4", "--list")
	if !strings.Contains(stdout, "A-4 km 399.1") {
		t.Errorf("accent-free multi-word search did not find the camera:\n%s", stdout)
	}
}

func TestCameraRequiresAQuery(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, _, stderr := runCmd(t, registry, "camera")

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "say which camera") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestCameraReportsNoMatch(t *testing.T) {
	t.Parallel()

	srv, _ := cameraPortal(t)
	registry := writeCameraRegistry(t, srv.URL)
	dataDir := t.TempDir()
	runCmdIn(t, registry, dataDir, "daemon", "--once") //nolint:dogsled // primes the store

	code, _, stderr := runCmdIn(t, registry, dataDir, "camera", "vladivostok")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no camera matches") {
		t.Errorf("stderr = %q", stderr)
	}
}

// filesUnder maps every file under dir to its size.
func filesUnder(t *testing.T, dir string) map[string]int64 {
	t.Helper()

	out := map[string]int64{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // a vanished temp file is not a failure
		}
		out[path] = info.Size()
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

// captionOf returns the text after the image, for readable failure messages.
func captionOf(out string) string {
	if i := strings.LastIndex(out, "\x1b[0m\n"); i >= 0 {
		return out[i+5:]
	}
	return out
}

// ramDir must refuse rather than fall back to /tmp, which is a real directory
// on a real disk on plenty of systems. Refusing is the correct outcome: the
// guarantee is that a camera frame never reaches persistent storage.
func TestOpenRefusesWhenNoMemoryBackedDirectoryExists(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "does-not-exist"))
	t.Setenv("PATH", "")

	srv, _ := cameraPortal(t)
	registry := writeCameraRegistry(t, srv.URL)
	dataDir := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := cli.New().Run(t.Context(),
		[]string{"daemon", "--once", "--registry", registry, "--data-dir", dataDir},
		&stdout, &stderr)
	if code != 0 {
		t.Fatalf("inventory pass failed: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = cli.New().Run(t.Context(),
		[]string{"camera", "A-4", "--open", "--registry", registry, "--data-dir", dataDir},
		&stdout, &stderr)

	// With no viewer on PATH and no tmpfs, the command must fail loudly.
	if code == 0 {
		t.Error("--open succeeded with neither a viewer nor a memory-backed directory")
	}
	if !strings.Contains(stderr.String(), "no image viewer available") {
		t.Errorf("stderr = %q, want it to name the problem", stderr.String())
	}
	// Whatever happened, nothing may have been written next to the store.
	for path := range filesUnder(t, dataDir) {
		if strings.HasSuffix(path, ".jpg") {
			t.Errorf("a frame was written to %s", path)
		}
	}
}
