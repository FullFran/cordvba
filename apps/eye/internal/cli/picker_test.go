package cli

import (
	"strings"
	"testing"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
)

// cam builds a camera entity for picker tests.
func cam(id, title string, lat, lon float64, imageURL string) observation.Entity {
	e := observation.Entity{
		ID: "dgt-cameras:" + id, Source: "dgt-cameras", Kind: "camera", Topic: "transport",
		Title: title, Position: &observation.Point{Lat: lat, Lon: lon},
	}
	if imageURL != "" {
		e.Payload = []byte(`{"image_url":"` + imageURL + `"}`)
	} else {
		e.Payload = []byte(`{"name":"` + title + `"}`)
	}
	return e
}

// inventory mirrors the real shape: municipal cameras are nearer but publish no
// image, and the DGT ones further out are the ones you can actually look at.
func inventory() []observation.Entity {
	return []observation.Entity{
		cam("421", "A-4 km 399.1 · CÓRDOBA", 37.8900, -4.7449, "https://example.org/421.jpg"),
		cam("num.26", "PLAZA DE LAS TENDILLAS", 37.8845, -4.7790, ""),
		cam("160804", "A-4 km 412.1 · CÓRDOBA", 37.8018, -4.8067, "https://example.org/160804.jpg"),
		cam("345", "A-92 km 104.8 · SEVILLA", 37.3891, -5.9845, "https://example.org/345.jpg"),
		cam("num.7", "AVDA. DE LA LIBERTAD", 37.8860, -4.7810, ""),
	}
}

// pick runs the picker over scripted input and returns the choice and output.
func pick(t *testing.T, input string) (observation.Entity, string, error) {
	t.Helper()

	var out strings.Builder
	chosen, err := pickCamera(inventory(), cordobaCentre, strings.NewReader(input), &out)
	return chosen, out.String(), err
}

// The picker exists to choose something to look at, so it must not open on a
// dozen cameras that show nothing just because they are nearer.
func TestPickerOpensOnViewableCamerasNearestFirst(t *testing.T) {
	t.Parallel()

	_, out, _ := pick(t, "q\n")

	if !strings.Contains(out, "  1") || !strings.Contains(out, "A-4 km 399.1") {
		t.Errorf("the nearest viewable camera is not first:\n%s", out)
	}
	if strings.Contains(out, "TENDILLAS") {
		t.Error("a camera with no image was offered on the opening screen")
	}
	if !strings.Contains(out, "publish a position but no image") {
		t.Errorf("the output does not say the imageless cameras exist:\n%s", out)
	}
}

func TestPickerSelectsByNumber(t *testing.T) {
	t.Parallel()

	chosen, _, err := pick(t, "2\n")
	if err != nil {
		t.Fatalf("pickCamera() = %v", err)
	}
	if !strings.Contains(chosen.Title, "412.1") {
		t.Errorf("chose %q, want the second entry", chosen.Title)
	}
}

func TestPickerEnterTakesTheNearest(t *testing.T) {
	t.Parallel()

	chosen, _, err := pick(t, "\n")
	if err != nil {
		t.Fatalf("pickCamera() = %v", err)
	}
	if !strings.Contains(chosen.Title, "399.1") {
		t.Errorf("chose %q, want the nearest", chosen.Title)
	}
}

// Filtering uses the same accent-folding, order-free rules as the flags, and
// reaches cameras the opening screen deliberately hid.
func TestPickerFiltersThenSelects(t *testing.T) {
	t.Parallel()

	chosen, out, err := pick(t, "sevilla\n1\n")
	if err != nil {
		t.Fatalf("pickCamera() = %v", err)
	}
	if !strings.Contains(chosen.Title, "SEVILLA") {
		t.Errorf("chose %q, want the filtered camera", chosen.Title)
	}
	if !strings.Contains(out, "of 5 cameras") {
		t.Errorf("the output does not show the narrowing:\n%s", out)
	}
}

func TestPickerFilterReachesImagelessCameras(t *testing.T) {
	t.Parallel()

	chosen, out, err := pick(t, "tendillas\n1\n")
	if err != nil {
		t.Fatalf("pickCamera() = %v", err)
	}
	if !strings.Contains(chosen.Title, "TENDILLAS") {
		t.Errorf("chose %q, want the filtered municipal camera", chosen.Title)
	}
	// It is offered, but marked, so choosing it is an informed decision.
	if !strings.Contains(out, "position only, no image published") {
		t.Errorf("the imageless camera was not marked:\n%s", out)
	}
}

func TestPickerFoldsAccentsInFilters(t *testing.T) {
	t.Parallel()

	chosen, _, err := pick(t, "cordoba\n1\n")
	if err != nil {
		t.Fatalf("pickCamera() = %v", err)
	}
	if !strings.Contains(chosen.Title, "CÓRDOBA") {
		t.Errorf("chose %q; an accent-free filter did not match", chosen.Title)
	}
}

// A filter that matches nothing must not be a dead end.
func TestPickerRecoversFromAnEmptyFilter(t *testing.T) {
	t.Parallel()

	chosen, out, err := pick(t, "vladivostok\n1\n")
	if err != nil {
		t.Fatalf("pickCamera() = %v", err)
	}
	if !strings.Contains(out, "Nothing matches") {
		t.Errorf("the output does not explain the empty result:\n%s", out)
	}
	if !strings.Contains(chosen.Title, "399.1") {
		t.Errorf("chose %q, want the list to have been restored", chosen.Title)
	}
}

func TestPickerRejectsAnOutOfRangeNumber(t *testing.T) {
	t.Parallel()

	chosen, out, err := pick(t, "99\n1\n")
	if err != nil {
		t.Fatalf("pickCamera() = %v", err)
	}
	if !strings.Contains(out, "no 99 in the list") {
		t.Errorf("the output does not reject the number:\n%s", out)
	}
	if !strings.Contains(chosen.Title, "399.1") {
		t.Errorf("chose %q after the rejection", chosen.Title)
	}
}

func TestPickerQuits(t *testing.T) {
	t.Parallel()

	if _, _, err := pick(t, "q\n"); err == nil {
		t.Fatal("q returned no error; nothing was selected")
	}
}

func TestPickerHandlesClosedInput(t *testing.T) {
	t.Parallel()

	if _, _, err := pick(t, ""); err == nil {
		t.Fatal("closed input returned no error")
	}
}

func TestPickerRejectsAnEmptyInventory(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	_, err := pickCamera(nil, cordobaCentre, strings.NewReader("\n"), &out)

	if err == nil {
		t.Fatal("an empty inventory returned no error")
	}
	if !strings.Contains(err.Error(), "eye daemon --once") {
		t.Errorf("error = %q, want it to say how to fill the inventory", err)
	}
}

func TestPickerPagesLongLists(t *testing.T) {
	t.Parallel()

	many := make([]observation.Entity, 0, 40)
	for i := range 40 {
		many = append(many, cam(
			string(rune('a'+i%26)),
			"A-4 km "+strings.Repeat("0", i%3+1),
			37.0+float64(i)/100, -4.7, "https://example.org/x.jpg",
		))
	}

	var out strings.Builder
	_, _ = pickCamera(many, cordobaCentre, strings.NewReader("q\n"), &out)

	if lines := strings.Count(out.String(), " km  "); lines > pickerPageSize {
		t.Errorf("showed %d options, want at most %d", lines, pickerPageSize)
	}
	if !strings.Contains(out.String(), "more match") {
		t.Errorf("the output does not say the list is truncated:\n%s", out.String())
	}
}
