package domain_test

import (
	"errors"
	"testing"
	"time"

	domain "github.com/FullFran/eye/internal/source/domain"
)

// validSource returns a registry entry that passes Validate.
func validSource() domain.Source {
	return domain.Source{
		ID:             "dgt-incidents",
		Authority:      "Direccion General de Trafico",
		Topic:          "transport",
		URL:            "https://nap.dgt.es/es/dataset/incidencias-dgt-datex2-v3-7",
		License:        "CC-BY-4.0",
		Format:         "datex2-3.7-xml",
		Access:         domain.AccessDocumentedAPI,
		Automation:     domain.AutomationEnabled,
		Interval:       60 * time.Second,
		PublishedEvery: 0,
	}
}

func TestAutomationStatusPollable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status domain.AutomationStatus
		want   bool
	}{
		{name: "enabled polls", status: domain.AutomationEnabled, want: true},
		{name: "review_terms does not", status: domain.AutomationReviewTerms, want: false},
		{name: "manual_link does not", status: domain.AutomationManualLink, want: false},
		{name: "disabled does not", status: domain.AutomationDisabled, want: false},
		{name: "unknown status fails closed", status: domain.AutomationStatus("probably-fine"), want: false},
		{name: "empty status fails closed", status: domain.AutomationStatus(""), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.status.Pollable(); got != tc.want {
				t.Errorf("Pollable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSourceValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*domain.Source)
		wantErr bool
	}{
		{name: "valid source", mutate: func(*domain.Source) {}},
		{name: "empty id", mutate: func(s *domain.Source) { s.ID = "" }, wantErr: true},
		{name: "empty authority", mutate: func(s *domain.Source) { s.Authority = "" }, wantErr: true},
		{name: "empty topic", mutate: func(s *domain.Source) { s.Topic = "" }, wantErr: true},
		{name: "empty url", mutate: func(s *domain.Source) { s.URL = "" }, wantErr: true},
		{name: "empty license", mutate: func(s *domain.Source) { s.License = "" }, wantErr: true},
		{name: "empty format", mutate: func(s *domain.Source) { s.Format = "" }, wantErr: true},
		{
			name:   "unspecified license is allowed",
			mutate: func(s *domain.Source) { s.License = "unspecified" },
		},
		{
			name:    "enabled source without interval",
			mutate:  func(s *domain.Source) { s.Interval = 0 },
			wantErr: true,
		},
		{
			name: "non-pollable source needs no interval",
			mutate: func(s *domain.Source) {
				s.Automation = domain.AutomationManualLink
				s.Access = domain.AccessPublicHTML
				s.Interval = 0
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := validSource()
			tc.mutate(&s)
			err := s.Validate()

			if tc.wantErr {
				if !errors.Is(err, domain.ErrInvalidSource) {
					t.Fatalf("Validate() = %v, want ErrInvalidSource", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

// An undocumented backend is the endpoint you find in DevTools behind a public
// viewer. The registry must refuse to schedule it however it is labelled.
func TestSourceRefusesToPollUndocumentedBackend(t *testing.T) {
	t.Parallel()

	s := validSource()
	s.Access = domain.AccessUndocumentedBackend
	s.Automation = domain.AutomationEnabled

	if err := s.Validate(); !errors.Is(err, domain.ErrInvalidSource) {
		t.Fatalf("Validate() = %v, want ErrInvalidSource", err)
	}
}

func TestSourceOption(t *testing.T) {
	t.Parallel()

	s := validSource()
	s.Options = map[string]string{"dataset": "camaras-de-trafico", "blank": ""}

	cases := []struct{ name, key, fallback, want string }{
		{name: "present", key: "dataset", fallback: "x", want: "camaras-de-trafico"},
		{name: "absent falls back", key: "missing", fallback: "x", want: "x"},
		{name: "empty falls back", key: "blank", fallback: "x", want: "x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := s.Option(tc.key, tc.fallback); got != tc.want {
				t.Errorf("Option(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestSourceOptionOnNilMap(t *testing.T) {
	t.Parallel()

	var s domain.Source
	if got := s.Option("anything", "fallback"); got != "fallback" {
		t.Errorf("Option() on nil map = %q, want the fallback", got)
	}
}
