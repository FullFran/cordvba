package domain_test

import (
	"testing"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
)

func TestNormalizeReducesATitleToItsCore(t *testing.T) {
	t.Parallel()

	// Every pair here is one real happening written two ways: different
	// case, accents, punctuation or spacing, same words.
	same := [][2]string{
		{"Incendio en la Ribera", "INCENDIO EN LA RIBERA."},
		{"A-4  km 399,1 · CÓRDOBA", "A-4 km 399,1 - Cordoba"},
		{"Feria de Córdoba", "FERIA DE CORDOBA"},
	}
	for _, pair := range same {
		a, b := observation.Normalize(pair[0]), observation.Normalize(pair[1])
		if a != b {
			t.Errorf("Normalize(%q) = %q\nNormalize(%q) = %q\nwant them equal",
				pair[0], a, pair[1], b)
		}
	}
}

func TestNormalizeKeepsGenuinelyDifferentTitlesApart(t *testing.T) {
	t.Parallel()

	// Normalisation that collapses these has stopped being useful and
	// started inventing matches.
	different := [][2]string{
		{"Incendio en la Ribera", "Incendio en el Zoco"},
		{"Pablo López", "Pablo Alborán"},
		{"A-4 km 399", "A-4 km 400"},
	}
	for _, pair := range different {
		if observation.Normalize(pair[0]) == observation.Normalize(pair[1]) {
			t.Errorf("Normalize collapsed %q and %q into %q",
				pair[0], pair[1], observation.Normalize(pair[0]))
		}
	}
}

// Normalisation matches wording, not meaning. Two publishers who chose
// genuinely different words for one concert stay apart here, and that is the
// deterministic layer working as specified: recognising them is the fuzzy
// layer's job, with a threshold somebody can inspect. Recorded as a test so
// the limit is a decision rather than a surprise.
func TestNormalizeDoesNotMatchDifferentWording(t *testing.T) {
	t.Parallel()

	promotional := observation.Normalize("Pablo López - El niño del espacio")
	tour := observation.Normalize("PABLO LOPEZ. Gira 2026")

	if promotional == tour {
		t.Error("the deterministic layer claimed a match it cannot justify")
	}
}

func TestNormalizeOnNothing(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"", "   ", "···", "---"} {
		if got := observation.Normalize(s); got != "" {
			t.Errorf("Normalize(%q) = %q, want empty", s, got)
		}
	}
}

// record builds a record with the fields identity is computed from.
func record(kind, title string, at time.Time, pos *observation.Point) observation.Record {
	return observation.Record{
		Kind: kind, Title: title, ObservedAt: at, Position: pos,
	}
}

func TestFingerprintMatchesTheSameHappeningAcrossSources(t *testing.T) {
	t.Parallel()

	morning := time.Date(2026, 8, 28, 9, 14, 0, 0, time.UTC)
	afternoon := time.Date(2026, 8, 28, 17, 2, 0, 0, time.UTC)

	// Two newspapers, the same fire, hours apart.
	a := record("article", "Incendio en la Ribera", morning, nil)
	a.Source = "diariocordoba"
	b := record("article", "INCENDIO EN LA RIBERA", afternoon, nil)
	b.Source = "cordopolis"

	if a.Fingerprint() != b.Fingerprint() {
		t.Errorf("the same story from two papers fingerprinted differently:\n%s\n%s",
			a.Fingerprint(), b.Fingerprint())
	}
	// The source must not take part: a fingerprint that carries it can only
	// ever match a record against itself, which is the defect this replaces.
	if a.Fingerprint() == "" {
		t.Fatal("empty fingerprint")
	}
}

func TestFingerprintSeparatesDifferentDays(t *testing.T) {
	t.Parallel()

	day1 := record("event", "Concierto de Pablo López", time.Date(2026, 8, 28, 21, 30, 0, 0, time.UTC), nil)
	day2 := record("event", "Concierto de Pablo López", time.Date(2026, 9, 28, 21, 30, 0, 0, time.UTC), nil)

	if day1.Fingerprint() == day2.Fingerprint() {
		t.Error("two concerts a month apart share an identity")
	}
}

// The whole point of ADR-0009: a field inside the identity is a field whose
// change can never be detected. The start time must therefore stay out.
func TestFingerprintSurvivesTheChangesItMustDetect(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)

	before := record("event", "Pablo López", day.Add(21*time.Hour+30*time.Minute), nil)
	before.Severity = observation.SeverityInfo

	after := record("event", "Pablo López", day.Add(22*time.Hour), nil)
	after.Severity = observation.SeverityModerate
	after.Description = "Retrasado media hora"

	if before.Fingerprint() != after.Fingerprint() {
		t.Errorf("a rescheduled event lost its identity, so the change is undetectable:\n%s\n%s",
			before.Fingerprint(), after.Fingerprint())
	}
}

func TestFingerprintSeparatesKinds(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	article := record("article", "Ronda de los Tejares", at, nil)
	camera := record("camera", "Ronda de los Tejares", at, nil)

	if article.Fingerprint() == camera.Fingerprint() {
		t.Error("a camera and an article about the same street share an identity")
	}
}

func TestFingerprintUsesPositionWhenThereIsOne(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	tendillas := &observation.Point{Lat: 37.8882, Lon: -4.7794}
	rabanales := &observation.Point{Lat: 37.9147, Lon: -4.7211}

	// Same words, ten kilometres apart: not the same thing.
	here := record("incident", "Retención", at, tendillas)
	there := record("incident", "Retención", at, rabanales)
	if here.Fingerprint() == there.Fingerprint() {
		t.Error("two incidents 10 km apart share an identity")
	}

	// The same incident reported with slightly different coordinates by two
	// publishers is still one incident.
	nudged := &observation.Point{Lat: 37.88823, Lon: -4.77937}
	almost := record("incident", "Retención", at, nudged)
	if here.Fingerprint() != almost.Fingerprint() {
		t.Errorf("a few metres of coordinate noise split one incident in two:\n%s\n%s",
			here.Fingerprint(), almost.Fingerprint())
	}
}

// A record eye cannot identify must say so rather than be given a plausible
// identity that groups it with unrelated things.
func TestFingerprintOfAnUnidentifiableRecord(t *testing.T) {
	t.Parallel()

	blank := record("article", "   ", time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), nil)
	if blank.Fingerprint() != "" {
		t.Errorf("Fingerprint() = %q for a record with no title, want empty", blank.Fingerprint())
	}
}
