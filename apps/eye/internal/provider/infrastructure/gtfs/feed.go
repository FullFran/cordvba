// Package gtfs reads GTFS feeds: the timetable format every transit operator
// publishes.
//
// One parser serves RENFE, AUCORSA and anything else on a national access
// point. The feed is a zip of CSV files, so the standard library covers it
// entirely — archive/zip and encoding/csv, no dependency.
package gtfs

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ErrGTFS is returned when a feed cannot be read.
var ErrGTFS = errors.New("gtfs")

// maxCSVRecords caps a single file. A national feed has tens of thousands of
// stop times; a malformed one could claim to have billions.
const maxCSVRecords = 2_000_000

// Feed is the subset of a GTFS archive eye uses.
type Feed struct {
	Stops     map[string]Stop
	Routes    map[string]Route
	Trips     map[string]Trip
	StopTimes []StopTime
	Calendar  map[string]Service
	Dates     map[string][]CalendarDate
}

// Stop is a station or halt.
type Stop struct {
	ID   string
	Name string
	Lat  float64
	Lon  float64
}

// Route is a line.
type Route struct {
	ID        string
	ShortName string
	LongName  string
}

// Trip is one run of a route on a service pattern.
type Trip struct {
	ID          string
	RouteID     string
	ServiceID   string
	Headsign    string
	DirectionID string
}

// StopTime is a scheduled call at a stop.
type StopTime struct {
	TripID    string
	StopID    string
	Departure Duration
	Arrival   Duration
	Sequence  int
}

// Service is a weekly service pattern with a validity window.
type Service struct {
	ID    string
	Days  [7]bool // Sunday..Saturday, matching time.Weekday
	Start time.Time
	End   time.Time
}

// CalendarDate is an exception to a service pattern.
type CalendarDate struct {
	ServiceID string
	Date      time.Time
	Added     bool
}

// Duration is a GTFS time-of-day, which may exceed 24 hours: a train leaving at
// 25:10 departs at 01:10 on the following service day, and collapsing that to
// 01:10 would move it a day earlier.
type Duration time.Duration

// String renders a GTFS time as HH:MM, keeping hours past midnight visible.
func (d Duration) String() string {
	total := time.Duration(d)
	return fmt.Sprintf("%02d:%02d", int(total.Hours()), int(total.Minutes())%60)
}

// Clock renders the wall-clock time, wrapping hours past midnight.
func (d Duration) Clock() string {
	total := time.Duration(d)
	return fmt.Sprintf("%02d:%02d", int(total.Hours())%24, int(total.Minutes())%60)
}

// Parse reads a GTFS archive.
func Parse(body []byte) (*Feed, error) {
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("%w: open archive: %w", ErrGTFS, err)
	}

	feed := &Feed{
		Stops:    map[string]Stop{},
		Routes:   map[string]Route{},
		Trips:    map[string]Trip{},
		Calendar: map[string]Service{},
		Dates:    map[string][]CalendarDate{},
	}

	readers := map[string]func([]string, map[string]int){
		"stops.txt":          feed.addStop,
		"routes.txt":         feed.addRoute,
		"trips.txt":          feed.addTrip,
		"stop_times.txt":     feed.addStopTime,
		"calendar.txt":       feed.addService,
		"calendar_dates.txt": feed.addCalendarDate,
	}

	for name, add := range readers {
		if err := readCSV(z, name, add); err != nil {
			// calendar.txt is optional when calendar_dates.txt carries
			// every service day, which some operators do.
			if errors.Is(err, errNoSuchFile) && (name == "calendar.txt" || name == "calendar_dates.txt") {
				continue
			}
			return nil, err
		}
	}

	if len(feed.Stops) == 0 {
		return nil, fmt.Errorf("%w: feed has no stops", ErrGTFS)
	}
	return feed, nil
}

// errNoSuchFile reports an absent optional file.
var errNoSuchFile = errors.New("gtfs: file not in archive")

// readCSV streams one file through a row handler.
func readCSV(z *zip.Reader, name string, add func([]string, map[string]int)) error {
	var file *zip.File
	for _, f := range z.File {
		// Some producers nest the feed in a directory.
		if f.Name == name || strings.HasSuffix(f.Name, "/"+name) {
			file = f
			break
		}
	}
	if file == nil {
		return fmt.Errorf("%w: %s", errNoSuchFile, name)
	}

	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("%w: open %s: %w", ErrGTFS, name, err)
	}
	defer func() { _ = rc.Close() }()

	reader := csv.NewReader(newBOMTrimmer(rc))
	reader.FieldsPerRecord = -1 // producers pad rows inconsistently
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("%w: %s header: %w", ErrGTFS, name, err)
	}

	index := make(map[string]int, len(header))
	for i, h := range header {
		index[strings.TrimSpace(strings.ToLower(h))] = i
	}

	for count := 0; count < maxCSVRecords; count++ {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			// One bad row must not discard a 50,000-row timetable.
			continue
		}
		add(row, index)
	}
	return nil
}

// field reads a column by name, tolerating its absence.
func field(row []string, index map[string]int, name string) string {
	i, ok := index[name]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// addStop records a stop.
func (f *Feed) addStop(row []string, index map[string]int) {
	id := field(row, index, "stop_id")
	if id == "" {
		return
	}
	lat, _ := strconv.ParseFloat(field(row, index, "stop_lat"), 64)
	lon, _ := strconv.ParseFloat(field(row, index, "stop_lon"), 64)

	f.Stops[id] = Stop{ID: id, Name: field(row, index, "stop_name"), Lat: lat, Lon: lon}
}

// addRoute records a route.
func (f *Feed) addRoute(row []string, index map[string]int) {
	id := field(row, index, "route_id")
	if id == "" {
		return
	}
	f.Routes[id] = Route{
		ID:        id,
		ShortName: field(row, index, "route_short_name"),
		LongName:  field(row, index, "route_long_name"),
	}
}

// addTrip records a trip.
func (f *Feed) addTrip(row []string, index map[string]int) {
	id := field(row, index, "trip_id")
	if id == "" {
		return
	}
	f.Trips[id] = Trip{
		ID:          id,
		RouteID:     field(row, index, "route_id"),
		ServiceID:   field(row, index, "service_id"),
		Headsign:    field(row, index, "trip_headsign"),
		DirectionID: field(row, index, "direction_id"),
	}
}

// addStopTime records a scheduled call.
func (f *Feed) addStopTime(row []string, index map[string]int) {
	trip := field(row, index, "trip_id")
	stop := field(row, index, "stop_id")
	if trip == "" || stop == "" {
		return
	}
	seq, _ := strconv.Atoi(field(row, index, "stop_sequence"))

	f.StopTimes = append(f.StopTimes, StopTime{
		TripID:    trip,
		StopID:    stop,
		Departure: parseGTFSTime(field(row, index, "departure_time")),
		Arrival:   parseGTFSTime(field(row, index, "arrival_time")),
		Sequence:  seq,
	})
}

// addService records a weekly pattern.
func (f *Feed) addService(row []string, index map[string]int) {
	id := field(row, index, "service_id")
	if id == "" {
		return
	}

	svc := Service{ID: id}
	for i, day := range []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"} {
		svc.Days[i] = field(row, index, day) == "1"
	}
	svc.Start = parseGTFSDate(field(row, index, "start_date"))
	svc.End = parseGTFSDate(field(row, index, "end_date"))

	f.Calendar[id] = svc
}

// addCalendarDate records an exception.
func (f *Feed) addCalendarDate(row []string, index map[string]int) {
	id := field(row, index, "service_id")
	date := parseGTFSDate(field(row, index, "date"))
	if id == "" || date.IsZero() {
		return
	}
	f.Dates[id] = append(f.Dates[id], CalendarDate{
		ServiceID: id,
		Date:      date,
		Added:     field(row, index, "exception_type") == "1",
	})
}

// parseGTFSTime reads HH:MM:SS, where HH may exceed 23.
func parseGTFSTime(s string) Duration {
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return 0
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0
	}
	sec := 0
	if len(parts) > 2 {
		sec, _ = strconv.Atoi(parts[2])
	}
	return Duration(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second)
}

// parseGTFSDate reads YYYYMMDD.
func parseGTFSDate(s string) time.Time {
	t, err := time.Parse("20060102", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t
}

// newBOMTrimmer strips a UTF-8 byte order mark, which GTFS producers emit
// often enough that the first column name would otherwise never match.
func newBOMTrimmer(r io.Reader) io.Reader {
	buf := make([]byte, 3)
	n, err := io.ReadFull(r, buf)
	if err != nil && n == 0 {
		return r
	}
	if n == 3 && bytes.Equal(buf, []byte{0xEF, 0xBB, 0xBF}) {
		return r
	}
	return io.MultiReader(bytes.NewReader(buf[:n]), r)
}
