package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	store "github.com/FullFran/eye/internal/observation/infrastructure"
	providers "github.com/FullFran/eye/internal/provider/infrastructure"
	source "github.com/FullFran/eye/internal/source/domain"
)

// sourcesCommand lists the registry and, for each entry, whether eye is
// permitted to poll it and whether this build can read it.
//
// Those are two different questions and both are worth an honest answer.
func sourcesCommand() Command {
	return Command{
		Name:    "sources",
		Summary: "List the source registry and what eye may and can read",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("sources", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				asJSON    = fs.Bool("json", false, "output as JSON")
				topic     = fs.String("topic", "", "only sources of this topic")
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
				dataDir   = fs.String("data-dir", "", "override where the store and raw cache live")
			)
			if err := fs.Parse(args); err != nil {
				return err
			}

			rt, err := newRuntime(runtimeOptions{registry: *registryP, dataDir: *dataDir})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			list := rt.sources
			if *topic != "" {
				list = filterByTopic(list, []string{*topic})
			}
			sort.Slice(list, func(i, j int) bool {
				if list[i].Topic != list[j].Topic {
					return list[i].Topic < list[j].Topic
				}
				return list[i].ID < list[j].ID
			})

			// Health comes from what earlier runs recorded, so this
			// answers without polling anything.
			states, err := rt.store.States(ctx)
			if err != nil {
				return err
			}

			if *asJSON {
				return writeJSON(stdout, sourceViews(list, states, time.Now().UTC(), rt.cfg.AllowPersonalSources))
			}
			return renderSources(stdout, rt.registryPath, list, states, time.Now().UTC(), rt.cfg.AllowPersonalSources)
		},
	}
}

// sourceView is the JSON shape of a registry entry.
type sourceView struct {
	ID         string `json:"id"`
	Authority  string `json:"authority"`
	Topic      string `json:"topic"`
	Format     string `json:"format"`
	License    string `json:"license"`
	Access     string `json:"access"`
	Automation string `json:"automation"`
	Pollable   bool   `json:"pollable"`
	HasAdapter bool   `json:"has_adapter"`
	Interval   string `json:"interval,omitempty"`
	URL        string `json:"url"`
	Notes      string `json:"notes,omitempty"`

	// Health is what earlier runs recorded. It is absent for a source that
	// has never been polled, which is different from one that failed.
	Health *sourceHealthView `json:"health,omitempty"`
}

// sourceHealthView is the persisted polling state of one source.
type sourceHealthView struct {
	LastSuccess       *time.Time `json:"last_success,omitempty"`
	LastAttempt       *time.Time `json:"last_attempt,omitempty"`
	Stale             bool       `json:"stale"`
	ConsecutiveErrors int        `json:"consecutive_errors"`
	LastError         string     `json:"last_error,omitempty"`
	Records           int        `json:"records"`
}

// pollableNow reports whether the scheduler will actually fetch this source
// right now, which is the registry's permission and the machine's opt-in taken
// together rather than the registry's alone.
func pollableNow(s source.Source, allowPersonal bool) bool {
	if s.Access.Personal() && !allowPersonal {
		return false
	}
	return s.Automation.Pollable()
}

// staleAfter is how far past a source's own interval it may fall before eye
// calls it stale. Publishers are late; three intervals is late enough to say so.
const staleAfter = 3

// sourceViews projects the registry for machine consumption.
func sourceViews(list []source.Source, states map[string]store.SourceState, now time.Time, allowPersonal bool) []sourceView {
	out := make([]sourceView, 0, len(list))
	for _, s := range list {
		v := sourceView{
			ID:         s.ID,
			Authority:  s.Authority,
			Topic:      s.Topic,
			Format:     s.Format,
			License:    s.License,
			Access:     string(s.Access),
			Automation: string(s.Automation),
			Pollable:   pollableNow(s, allowPersonal),
			HasAdapter: providers.Supported(s.Format),
			URL:        s.URL,
			Notes:      s.Notes,
		}
		if s.Interval > 0 {
			v.Interval = s.Interval.String()
		}
		if st, ok := states[s.ID]; ok {
			v.Health = healthView(st, s, now)
		}
		out = append(out, v)
	}
	return out
}

// healthView projects persisted polling state, marking a source stale when it
// has fallen well past its own declared interval.
func healthView(st store.SourceState, s source.Source, now time.Time) *sourceHealthView {
	v := &sourceHealthView{
		ConsecutiveErrors: st.ConsecutiveErrors,
		LastError:         st.LastError,
		Records:           st.Records,
	}
	if !st.LastSuccess.IsZero() {
		last := st.LastSuccess
		v.LastSuccess = &last
	}
	if !st.LastAttempt.IsZero() {
		attempt := st.LastAttempt
		v.LastAttempt = &attempt
	}

	tolerance := s.Interval * staleAfter
	if tolerance <= 0 {
		tolerance = time.Hour
	}
	v.Stale = st.Health().Stale(now, tolerance)

	return v
}

// renderSources writes the human-readable registry table.
func renderSources(w io.Writer, registryPath string, list []source.Source, states map[string]store.SourceState, now time.Time, allowPersonal bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SOURCE\tTOPIC\tFORMAT\tSTATE\tLAST OK\tLICENCE\tAUTHORITY")

	var live, held, noAdapter, stale int
	for _, s := range list {
		state, _ := stateOf(s, allowPersonal)
		switch state {
		case "live":
			live++
		case "no adapter":
			noAdapter++
		default:
			held++
		}

		lastOK := "never"
		if st, ok := states[s.ID]; ok {
			h := healthView(st, s, now)
			switch {
			case h.LastSuccess == nil:
				lastOK = "failing"
			case h.Stale:
				lastOK = ageOf(*h.LastSuccess, now) + " · stale"
				stale++
			default:
				lastOK = ageOf(*h.LastSuccess, now)
			}
		}

		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.ID, s.Topic, s.Format, state, lastOK,
			ellipsis(s.License, 20), ellipsis(s.Authority, 28))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(w, "\n%s · %d live · %d held · %d awaiting an adapter\n",
		plural(len(list), "source", "sources"), live, held, noAdapter)
	if stale > 0 {
		_, _ = fmt.Fprintf(w, "%d live source(s) have not answered in a while — shown as stale, never as fresh.\n", stale)
	}
	_, _ = fmt.Fprintf(w, "registry: %s\n", registryPath)

	if held > 0 {
		_, _ = fmt.Fprintln(w, "\nHeld sources are not a failure. They are sources whose reuse terms")
		_, _ = fmt.Fprintln(w, "are unresolved, or that publish no documented machine interface.")
	}
	return nil
}

// stateOf describes what eye will actually do with a source.
//
// allowPersonal is the machine's opt-in for undocumented personal sources. It
// has to be part of this answer: a listing that calls a source "live" when the
// scheduler will not touch it is a listing that lies, which is the one thing
// this table exists not to do.
func stateOf(s source.Source, allowPersonal bool) (state string, pollable bool) {
	switch {
	case !s.Automation.Pollable():
		return string(s.Automation), false
	case !providers.Supported(s.Format):
		return "no adapter", false
	case s.Access.Personal() && !allowPersonal:
		return "personal, off", false
	case s.Access.Personal():
		return "personal", true
	default:
		return "live", true
	}
}
