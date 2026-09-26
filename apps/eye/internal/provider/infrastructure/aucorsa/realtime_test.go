package aucorsa

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// fragment loads the recorded response and unwraps the JSON string the
// endpoint answers with.
func fragment(t *testing.T) string {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/aucorsa/estimations.json") // #nosec G304 -- fixture path is a literal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var inner string
	if err := json.Unmarshal(body, &inner); err != nil {
		t.Fatalf("the fixture is not a JSON-wrapped fragment: %v", err)
	}
	return inner
}

// The endpoint answers with a rendered popup rather than data, so the parser
// has to hold up against the real markup. This fixture is a live capture.
func TestParseEstimatesReadsTheRealFragment(t *testing.T) {
	t.Parallel()

	stopID, stopName, estimates := parseEstimates(fragment(t))

	if stopID != "401" {
		t.Errorf("stop id = %q, want 401", stopID)
	}
	if !strings.Contains(stopName, "Trassierra") {
		t.Errorf("stop name = %q, want the street", stopName)
	}
	if len(estimates) == 0 {
		t.Fatal("no estimates parsed from a fragment that contains them")
	}

	for _, e := range estimates {
		if e.Line == "" {
			t.Error("an estimate has no line")
		}
		if e.Minutes < 0 || e.Minutes > 240 {
			t.Errorf("line %s: %d minutes is not a plausible wait", e.Line, e.Minutes)
		}
	}
}

// The next bus and the one behind it are different observations, and the
// second must not be dropped or merged into the first.
func TestParseEstimatesKeepsTheQueueInOrder(t *testing.T) {
	t.Parallel()

	_, _, estimates := parseEstimates(fragment(t))

	byLine := map[string][]estimate{}
	for _, e := range estimates {
		byLine[e.Line] = append(byLine[e.Line], e)
	}

	for line, queue := range byLine {
		for i, e := range queue {
			if e.Position != i {
				t.Errorf("line %s: estimate %d is marked position %d", line, i, e.Position)
			}
		}
		// The operator reports occupancy for the imminent bus only, so
		// claiming it for the ones behind would be inventing data.
		for _, e := range queue[1:] {
			if e.Occupancy != "" {
				t.Errorf("line %s: a later bus carries an occupancy reading", line)
			}
		}
	}
}

func TestParseEstimatesHandlesSeveralLinesAtOneStop(t *testing.T) {
	t.Parallel()

	// Two lines calling at one stop, the second with no occupancy icon.
	f := `<div class="ppp-content"><div class="ppp-stop-label">Parada 401: Plaza de las Tendillas</div>` +
		`<div class="ppp-container"><div class="ppp-label">` +
		`<div class="ppp-line-number" style="x">3</div>` +
		`<div class="ppp-line-route">ALBAIDA - RENFE</div></div>` +
		`<div class="ppp-estimations"><div class="ppp-estimation"><span>Pr&oacute;ximo autob&uacute;s: <strong>2 minutos</strong></span>` +
		`<img class="imgocupationbus" alt="Ocupaci&oacute;n Baja"></div>` +
		`<div class="ppp-estimation"><span>Siguiente: <strong>22 minutos</strong></span></div></div></div>` +
		`<div class="ppp-container"><div class="ppp-label">` +
		`<div class="ppp-line-number" style="y">T</div>` +
		`<div class="ppp-line-route">C&Oacute;RDOBA - TRASIERRA</div></div>` +
		`<div class="ppp-estimations"><div class="ppp-estimation"><span>Pr&oacute;ximo: <strong>33 minutos</strong></span></div></div></div>` +
		`<div class="ppp-favorited" data-stop-id="401">favoritas</div></div>`

	stopID, stopName, estimates := parseEstimates(f)

	if stopID != "401" || stopName != "Plaza de las Tendillas" {
		t.Errorf("stop = %q / %q", stopID, stopName)
	}
	if len(estimates) != 3 {
		t.Fatalf("estimates = %d, want 3 across two lines", len(estimates))
	}

	// The last container must be captured too: it has no following block to
	// delimit it, which is the case a naive split loses.
	var sawT bool
	for _, e := range estimates {
		if e.Line == "T" {
			sawT = true
			if e.Minutes != 33 {
				t.Errorf("line T = %d minutes, want 33", e.Minutes)
			}
			// Entities must survive the markup, not arrive as "CÓRDOBA".
			if !strings.Contains(e.Route, "CÓRDOBA") {
				t.Errorf("route = %q, want the entity decoded", e.Route)
			}
		}
	}
	if !sawT {
		t.Error("the final line block was dropped")
	}
}

// "Sin estimaciones" is a real answer, not a parse failure, and it must yield
// nothing rather than a fabricated wait.
func TestParseEstimatesOnNoService(t *testing.T) {
	t.Parallel()

	_, _, estimates := parseEstimates(
		`<div class="ppp-content"><div class="ppp-no-estimations">Sin estimaciones</div></div>`)

	if len(estimates) != 0 {
		t.Errorf("estimates = %d, want none", len(estimates))
	}
}

func TestParseEstimatesOnRubbish(t *testing.T) {
	t.Parallel()

	for _, f := range []string{"", "-1", "<html><body>404</body></html>"} {
		if _, _, estimates := parseEstimates(f); len(estimates) != 0 {
			t.Errorf("parseEstimates(%q) invented %d estimates", f, len(estimates))
		}
	}
}
