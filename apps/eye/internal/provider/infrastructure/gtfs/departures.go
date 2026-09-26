package gtfs

import (
	"sort"
	"time"
)

// Departure is one scheduled call resolved to a real moment.
type Departure struct {
	Stop      Stop
	TripID    string
	RouteName string
	Headsign  string
	Departure Duration
	// When is the departure as an absolute time, with GTFS's past-midnight
	// hours resolved onto the correct day.
	When time.Time
}

// Departures returns the scheduled departures from the given stops, within a
// horizon of now.
//
// The join is the point of a GTFS reader: stop_times gives a time of day,
// trips gives a service pattern, and calendar says whether that pattern runs
// today. A reader that skips the calendar produces a plausible timetable for
// days on which the train does not exist.
func (f *Feed) Departures(stops map[string]bool, now time.Time, horizon time.Duration) []Departure {
	if len(stops) == 0 {
		return nil
	}

	// Service days are checked for today and the two days either side, so a
	// departure at 25:10 on yesterday's service day is still found.
	days := []time.Time{
		now.AddDate(0, 0, -1).Truncate(24 * time.Hour),
		now.Truncate(24 * time.Hour),
		now.AddDate(0, 0, 1).Truncate(24 * time.Hour),
	}

	active := make(map[string]map[string]bool, len(days))
	for _, day := range days {
		key := day.Format("20060102")
		active[key] = map[string]bool{}
		for id := range f.serviceIDs() {
			if f.runsOn(id, day) {
				active[key][id] = true
			}
		}
	}

	limit := now.Add(horizon)
	var out []Departure

	for _, st := range f.StopTimes {
		if !stops[st.StopID] || st.Departure == 0 {
			continue
		}
		trip, ok := f.Trips[st.TripID]
		if !ok {
			continue
		}

		for _, day := range days {
			if !active[day.Format("20060102")][trip.ServiceID] {
				continue
			}
			when := day.Add(time.Duration(st.Departure))
			if when.Before(now) || when.After(limit) {
				continue
			}

			out = append(out, Departure{
				Stop:      f.Stops[st.StopID],
				TripID:    trip.ID,
				RouteName: f.routeName(trip.RouteID),
				Headsign:  trip.Headsign,
				Departure: st.Departure,
				When:      when,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].When.Before(out[j].When) })
	return out
}

// serviceIDs is every service the feed mentions, from either calendar file.
func (f *Feed) serviceIDs() map[string]bool {
	out := make(map[string]bool, len(f.Calendar)+len(f.Dates))
	for id := range f.Calendar {
		out[id] = true
	}
	for id := range f.Dates {
		out[id] = true
	}
	return out
}

// runsOn reports whether a service operates on a given day.
//
// Exceptions win over the weekly pattern, which is what calendar_dates is for:
// a public holiday removes a weekday service, and a special working Saturday
// adds one.
func (f *Feed) runsOn(serviceID string, day time.Time) bool {
	for _, ex := range f.Dates[serviceID] {
		if sameDay(ex.Date, day) {
			return ex.Added
		}
	}

	svc, ok := f.Calendar[serviceID]
	if !ok {
		return false
	}
	if !svc.Start.IsZero() && day.Before(svc.Start) {
		return false
	}
	if !svc.End.IsZero() && day.After(svc.End.Add(24*time.Hour-time.Second)) {
		return false
	}
	return svc.Days[int(day.Weekday())]
}

// routeName renders a route's most useful label.
func (f *Feed) routeName(id string) string {
	route, ok := f.Routes[id]
	if !ok {
		return id
	}
	if route.ShortName != "" {
		return route.ShortName
	}
	return route.LongName
}

// sameDay compares two times by calendar date.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
