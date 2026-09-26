package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidOverrides is returned when the operator's deployment overrides
// cannot be turned into a valid restriction of the registry.
var ErrInvalidOverrides = errors.New("overrides: invalid")

// SourceOverrides narrows which sources the scheduler may poll.
//
// Only and Disable are the allow-list and deny-list an operator writes; both
// naming the same id is a contradiction Validate refuses to guess at.
// TopicsOnly narrows by topic instead, and composes with the other two rather
// than replacing them.
type SourceOverrides struct {
	Only       []string
	Disable    []string
	TopicsOnly []string
}

// RetentionOverride pins one source's effective retention, in place of
// whatever the registry (and, for an ephemeral source, the adapter's own TTL)
// would otherwise produce.
//
// Kind and TTL are mutually exclusive by construction: a historical override
// never carries a TTL, and an ephemeral override always carries a positive
// one. That is what keeps "historical with a ttl" — the conflict #73 already
// rejects at the registry — impossible to express here too, without a second
// check re-litigating it.
type RetentionOverride struct {
	Kind Retention
	// TTL applies only when Kind is RetentionEphemeral.
	TTL time.Duration
}

// Overrides is the parsed, validated content of the optional per-deployment
// overrides file. Its zero value is "no overrides at all": every method on it
// behaves exactly as if the file were absent, which is what lets a caller use
// it unconditionally instead of branching on whether one was loaded.
type Overrides struct {
	Sources SourceOverrides
	// Retention maps a source id to the retention that replaces the
	// registry's own for that id. An id absent from this map keeps the
	// registry's decision untouched.
	Retention map[string]RetentionOverride
}

// Validate checks the overrides against the registry they apply to: every id
// and topic named must be real, and only/disable must not name the same id
// twice. It does not — and cannot — check whether an id is currently
// schedulable; that is what Permits and DisabledByOperator answer, and
// answering it here would let an override that merely restricts a held
// source look like an error when it is not.
func (o Overrides) Validate(sources []Source) error {
	ids := make(map[string]bool, len(sources))
	topics := make(map[string]bool, len(sources))
	for _, s := range sources {
		ids[s.ID] = true
		topics[s.Topic] = true
	}

	onlySet := make(map[string]bool, len(o.Sources.Only))
	for _, id := range o.Sources.Only {
		if !ids[id] {
			return fmt.Errorf("%w: sources.only: unknown source id %q", ErrInvalidOverrides, id)
		}
		onlySet[id] = true
	}
	for _, id := range o.Sources.Disable {
		if !ids[id] {
			return fmt.Errorf("%w: sources.disable: unknown source id %q", ErrInvalidOverrides, id)
		}
		if onlySet[id] {
			return fmt.Errorf("%w: sources: %q is in both only and disable", ErrInvalidOverrides, id)
		}
	}
	for _, topic := range o.Sources.TopicsOnly {
		if !topics[topic] {
			return fmt.Errorf("%w: sources.topics_only: unknown topic %q", ErrInvalidOverrides, topic)
		}
	}
	for id := range o.Retention {
		if !ids[id] {
			return fmt.Errorf("%w: retention: unknown source id %q", ErrInvalidOverrides, id)
		}
	}
	return nil
}

// Permits reports whether the operator's overrides, on their own, allow this
// source to be scheduled. It says nothing about whether the registry (or, for
// a personal source, the machine) already allows it — composing that
// question is DisabledByOperator's job, and the scheduling seam's after it.
//
// The zero value permits everything: an absent overrides file must behave
// exactly like today, and this is what lets every caller use Overrides
// unconditionally instead of checking whether one was loaded.
func (o Overrides) Permits(s Source) bool {
	if len(o.Sources.Only) > 0 && !containsString(o.Sources.Only, s.ID) {
		return false
	}
	if containsString(o.Sources.Disable, s.ID) {
		return false
	}
	if len(o.Sources.TopicsOnly) > 0 && !containsString(o.Sources.TopicsOnly, s.Topic) {
		return false
	}
	return true
}

// Schedulable reports whether the scheduler may poll this source at all: the
// registry's own automation gate, the machine's personal-source opt-in, and
// the operator's overrides, composed in that order.
//
// The order is the whole point. Overrides restrict; they can only remove a
// source the first two gates already permit, never add one they do not — a
// source held by the registry (review_terms, manual_link, an
// undocumented_personal source without its opt-in) can never be enabled from
// here, because Permits is never even consulted once an earlier gate has
// already said no.
func (o Overrides) Schedulable(s Source, allowPersonal bool) bool {
	if !s.Automation.Pollable() {
		return false
	}
	if s.Access.Personal() && !allowPersonal {
		return false
	}
	return o.Permits(s)
}

// DisabledByOperator reports whether this source would be scheduled if the
// operator had written no overrides at all, but is not, because these
// overrides restrict it.
//
// It is deliberately false for anything the registry (or the machine's
// personal-source opt-in) already holds back: that source is not disabled BY
// THE OPERATOR, it is held for a reason of its own, and folding the two
// together would bury the more specific answer `eye sources` exists to give.
func (o Overrides) DisabledByOperator(s Source, allowPersonal bool) bool {
	if !s.Automation.Pollable() {
		return false
	}
	if s.Access.Personal() && !allowPersonal {
		return false
	}
	return !o.Permits(s)
}

// EffectiveRetention resolves a source's retention: the registry's own value,
// unless the operator's overrides pin this id to something else. overridden
// reports which case applied, since only an override may replace whatever
// TTL the adapter itself would otherwise compute — the registry's own
// historical/ephemeral split never does that; see applyRetention in
// observation/application for where this composition is consumed.
func (o Overrides) EffectiveRetention(s Source) (kind Retention, ttl time.Duration, overridden bool) {
	ov, ok := o.Retention[s.ID]
	if !ok {
		return s.Retention.Effective(), 0, false
	}
	return ov.Kind, ov.TTL, true
}

// containsString reports whether v is in list. Sources overrides.yaml names
// are few enough per deployment that a linear scan is simpler than building a
// set for a one-shot Permits call.
func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
