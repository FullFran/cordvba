package datex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// The kinds this adapter emits.
const (
	kindRoadIncident  = "road_incident"
	kindRoadworks     = "roadworks"
	kindRoadClosure   = "road_closure"
	kindWeatherHazard = "weather_hazard"
)

// situationPublication is the SituationPublication payload: every road
// situation DGT considers active across Spain.
//
// `payload` is the ROOT element (<d2:payload xsi:type="sit:SituationPublication">),
// so the situations are addressed directly rather than through a wrapper, the
// same way the DevicePublication is read.
type situationPublication struct {
	XMLName xml.Name `xml:"payload"`

	PublicationTime string      `xml:"publicationTime"`
	Situations      []situation `xml:"situation"`
}

// situation groups the records DGT considers one event. A single situation
// routinely carries several records — a lane closure, an obstruction and a
// speed restriction are three statements about one set of roadworks — and each
// is a separate observation.
type situation struct {
	ID string `xml:"id,attr"`
	// OverallSeverity is the situation-wide claim. A record that states its
	// own severity overrides it.
	OverallSeverity   string            `xml:"overallSeverity"`
	InformationStatus string            `xml:"headerInformation>informationStatus"`
	Records           []situationRecord `xml:"situationRecord"`
}

// situationRecord is one statement about one stretch of road.
//
// Type is the xsi:type discriminator, and it decides everything: DATEX II
// subclasses situationRecord and puts the meaningful field in the subclass.
type situationRecord struct {
	Type    string `xml:"type,attr"`
	ID      string `xml:"id,attr"`
	Version string `xml:"version,attr"`

	CreationReference string `xml:"situationRecordCreationReference"`
	CreationTime      string `xml:"situationRecordCreationTime"`
	VersionTime       string `xml:"situationRecordVersionTime"`
	Probability       string `xml:"probabilityOfOccurrence"`
	Severity          string `xml:"severity"`
	SourceID          string `xml:"source>sourceIdentification"`

	ValidityStatus string `xml:"validity>validityStatus"`
	StartTime      string `xml:"validity>validityTimeSpecification>overallStartTime"`
	EndTime        string `xml:"validity>validityTimeSpecification>overallEndTime"`

	CauseType string `xml:"cause>causeType"`
	// Detail captures whichever detailedCauseType child DGT used, by name
	// and value, without this adapter having to enumerate them. The profile
	// gains new ones and a schema addition must not take the provider down.
	Detail struct {
		Any struct {
			XMLName xml.Name
			Value   string `xml:",chardata"`
		} `xml:",any"`
	} `xml:"cause>detailedCauseType"`

	Location situationLocation `xml:"locationReference"`

	ComplianceOption string `xml:"complianceOption"`

	// The subclass fields. Exactly one of these is set on a well-formed
	// record, and which one it is says what the record is about.
	ManagementType      string `xml:"roadOrCarriagewayOrLaneManagementType"`
	ObstructionType     string `xml:"obstructionType"`
	AbnormalTrafficType string `xml:"abnormalTrafficType"`
	SpeedManagementType string `xml:"speedManagementType"`
	TemporarySpeedLimit string `xml:"temporarySpeedLimit"`
	RoadConditionType   string `xml:"nonWeatherRelatedRoadConditionType"`
	PoorEnvironmentType string `xml:"poorEnvironmentType"`
	InstructionType     string `xml:"generalInstructionToRoadUsersType"`
	GenericName         string `xml:"genericSituationRecordName"`
}

// situationLocation is the DGT profile's location reference. It is either a
// point or a linear extent along one road; the presence of the linear subtree
// is what tells them apart.
type situationLocation struct {
	Type string `xml:"type,attr"`

	RoadName        string `xml:"supplementaryPositionalDescription>roadInformation>roadName"`
	RoadDestination string `xml:"supplementaryPositionalDescription>roadInformation>roadDestination"`
	Carriageway     string `xml:"supplementaryPositionalDescription>carriageway>carriageway"`
	LaneUsage       string `xml:"supplementaryPositionalDescription>carriageway>lane>laneUsage"`
	// Description is the one free-text slot in the profile, and it holds
	// whatever the operator typed.
	Description          string `xml:"supplementaryPositionalDescription>locationDescription>values>value"`
	GeographicDescriptor string `xml:"supplementaryPositionalDescription>geographicDescriptor"`

	PointDirection string     `xml:"tpegPointLocation>tpegDirection"`
	Point          datexPoint `xml:"tpegPointLocation>point"`

	LinearDirection string     `xml:"tpegLinearLocation>tpegDirection"`
	From            datexPoint `xml:"tpegLinearLocation>from"`
	To              datexPoint `xml:"tpegLinearLocation>to"`
}

// datexPoint is the TPEG non-junction point the DGT profile uses everywhere:
// a coordinate plus the Spanish extension carrying the administrative names.
type datexPoint struct {
	Latitude  float64 `xml:"pointCoordinates>latitude"`
	Longitude float64 `xml:"pointCoordinates>longitude"`

	KilometerPoint      string `xml:"_tpegNonJunctionPointExtension>extendedTpegNonJunctionPoint>kilometerPoint"`
	Municipality        string `xml:"_tpegNonJunctionPointExtension>extendedTpegNonJunctionPoint>municipality"`
	Province            string `xml:"_tpegNonJunctionPointExtension>extendedTpegNonJunctionPoint>province"`
	AutonomousCommunity string `xml:"_tpegNonJunctionPointExtension>extendedTpegNonJunctionPoint>autonomousCommunity"`
}

// position converts the coordinate, reporting whether the point states one at
// all. Null island is treated as absent: DGT emits it for records whose
// geocoding failed.
func (p datexPoint) position() (observation.Point, bool) {
	pos := observation.Point{Lat: p.Latitude, Lon: p.Longitude}
	if !pos.Valid() || (pos.Lat == 0 && pos.Lon == 0) {
		return observation.Point{}, false
	}
	return pos, true
}

// SituationProvider turns the national road situation feed into records.
//
// It ingests the whole of Spain on purpose. Filtering by geography is the
// query layer's job; an adapter that dropped everything outside Córdoba would
// make "is the A-4 cut north of Madrid?" unanswerable for a journey that
// starts here.
type SituationProvider struct {
	src    source.Source
	client *httpx.Client

	validators httpx.Validators
}

// NewSituations builds the DGT road situation provider.
func NewSituations(src source.Source, client *httpx.Client) *SituationProvider {
	return &SituationProvider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *SituationProvider) Info() source.Source { return p.src }

// Poll fetches the publication and emits one record per situation record it
// can carry honestly.
func (p *SituationProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	resp, err := p.client.Get(ctx, p.src.URL, p.validators)
	switch {
	case errors.Is(err, httpx.ErrNotModified):
		p.validators = resp.Validators
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrDatex, err)
	}
	p.validators = resp.Validators

	var doc situationPublication
	if err := decodeXML(resp.DecodeUTF8(), &doc); err != nil {
		return nil, fmt.Errorf("%w: parse situations: %w", ErrDatex, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: p.src.URL,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	published := parseTime(doc.PublicationTime)
	records := make([]observation.Record, 0, len(doc.Situations))
	seen := 0

	for _, sit := range doc.Situations {
		for i := range sit.Records {
			seen++
			rec, ok := p.record(sit, &sit.Records[i], published, resp.FetchedAt, prov)
			if !ok {
				continue
			}
			records = append(records, rec)
		}
	}

	if len(records) == 0 && seen > 0 {
		return nil, fmt.Errorf("%w: %d situation records parsed but none usable", ErrDatex, seen)
	}
	return records, nil
}

// record normalizes one situation record, reporting whether eye carries it.
func (p *SituationProvider) record(
	sit situation,
	r *situationRecord,
	published, fetchedAt time.Time,
	prov observation.Provenance,
) (observation.Record, bool) {
	kind, ok := kindOf(r)
	if !ok || r.ID == "" {
		return observation.Record{}, false
	}

	// The record's own time, never eye's. Their difference is the source
	// latency and merging them would sell stale data as live.
	observedAt := parseTime(r.VersionTime)
	if observedAt.IsZero() {
		observedAt = parseTime(r.CreationTime)
	}
	if observedAt.IsZero() {
		observedAt = published
	}
	if observedAt.IsZero() {
		// A statement with no time cannot be placed in history, and
		// stamping it with the fetch time would invent one.
		return observation.Record{}, false
	}

	at, geometry := locate(&r.Location)
	stamp := observedAt.UTC().Format(time.RFC3339)

	rec := observation.Record{
		ID:          p.src.ID + ":" + r.ID + ":" + stamp,
		Source:      p.src.ID,
		Kind:        kind,
		Topic:       p.src.Topic,
		ObservedAt:  observedAt,
		FetchedAt:   fetchedAt,
		Geometry:    geometry,
		Title:       situationTitle(r, at),
		Description: situationDescription(r, at),
		Severity:    severityOf(r.Severity, sit.OverallSeverity),
		Confidence:  1,
		// The competent authority declaring the state of its own road
		// network. Nothing about it is preliminary.
		Quality: observation.QualityOfficial,
		// The publisher's own identifier for this statement. It survives
		// re-polling untouched while the statement stands, and a new
		// version of the same incident arrives under a new id.
		LocalKey:   r.ID,
		DedupeKey:  p.src.ID + ":" + r.ID + ":" + stamp,
		Payload:    situationPayload(sit, r, at),
		Provenance: prov,
	}
	if pos, stated := at.position(); stated {
		rec.Position = &pos
	}
	if from := parseTime(r.StartTime); !from.IsZero() {
		rec.ValidFrom = &from
	}
	// ValidUntil is left nil when the publisher announced no end. An
	// invented one would have eye claim a road reopens at a time nobody
	// stated. ExpiresAt is left nil for the same reason it is on AEMET
	// warnings: a situation that has passed is still the record of what was
	// said, and ValidUntil is what says it no longer applies.
	if until := parseTime(r.EndTime); !until.IsZero() {
		rec.ValidUntil = &until
	}

	if err := rec.Validate(); err != nil {
		return observation.Record{}, false
	}
	return rec, true
}

// kindOf maps a situation record onto eye's vocabulary, reporting whether the
// record is one this adapter carries at all.
//
// The xsi:type decides whether eye has an honest shape for the record; the
// management type and the cause decide which road kind it is.
//
//	roadClosed / carriagewayClosures                      -> road_closure
//	sit:PoorEnvironmentConditions, cause poorEnvironment   -> weather_hazard
//	cause roadMaintenance                                  -> roadworks
//	anything else in the carried set                       -> road_incident
//
// sit:GeneralInstructionOrMessageToRoadUsers is deliberately NOT carried. It
// is an advisory addressed to drivers ("drive carefully"), not a statement
// about the road, and the closest kind here would misfile it as an incident
// that is not happening. Anything DGT adds to the profile later is skipped for
// the same reason: a wrong kind is worse than a missing record.
func kindOf(r *situationRecord) (string, bool) {
	switch localName(r.Type) {
	case "RoadOrCarriagewayOrLaneManagement",
		"GeneralObstruction",
		"GenericSituationRecord",
		"AbnormalTraffic",
		"NonWeatherRelatedRoadConditions",
		"SpeedManagement",
		"PoorEnvironmentConditions":
	default:
		return "", false
	}

	// A closure is the operative fact for anyone on the road, whatever
	// caused it.
	switch r.ManagementType {
	case "roadClosed", "carriagewayClosures":
		return kindRoadClosure, true
	}
	if localName(r.Type) == "PoorEnvironmentConditions" || r.CauseType == "poorEnvironment" {
		return kindWeatherHazard, true
	}
	if r.CauseType == "roadMaintenance" {
		return kindRoadworks, true
	}
	return kindRoadIncident, true
}

// severityMap converts the DATEX II SeverityEnum onto eye's 0..5 scale.
//
// The scale is coarse on purpose, so a road closure and a river level can sit
// in one ranked list. DATEX's own seven values collapse onto it like this, and
// nothing else in the record is treated as a severity signal: probability,
// compliance and vehicle restrictions are all product metadata and stay in
// Payload.
var severityMap = map[string]observation.Severity{
	"none":    observation.SeverityNone,
	"lowest":  observation.SeverityInfo,
	"low":     observation.SeverityLow,
	"medium":  observation.SeverityModerate,
	"high":    observation.SeverityHigh,
	"highest": observation.SeverityCritical,
	"unknown": observation.SeverityInfo,
}

// severityOf reads the record's severity, falling back to the situation-wide
// claim it belongs to.
//
// A record that states nothing and belongs to a situation that states nothing
// gets SeverityInfo: DGT publishing a road situation at all is information,
// and promoting silence to "moderate" would be eye's invention, not DGT's
// statement.
func severityOf(record, overall string) observation.Severity {
	if s, ok := severityMap[strings.TrimSpace(record)]; ok {
		return s
	}
	if s, ok := severityMap[strings.TrimSpace(overall)]; ok {
		return s
	}
	return observation.SeverityInfo
}

// locate resolves where the record sits and, for a linear extent, the shape it
// covers.
//
// A segment is placed at the point the publisher states as its start, never at
// a midpoint eye computed: DGT says the closure runs from km 120.4 to km
// 120.2, and inventing km 120.3 would put the record where nothing was
// reported.
func locate(loc *situationLocation) (datexPoint, json.RawMessage) {
	from, hasFrom := loc.From.position()
	to, hasTo := loc.To.position()

	if hasFrom && hasTo {
		// RFC 7946: longitude first. A map library fed lat/lon renders
		// Córdoba in the Indian Ocean and reports no error at all.
		geometry, err := json.Marshal(map[string]any{
			"type":        "LineString",
			"coordinates": [][2]float64{{from.Lon, from.Lat}, {to.Lon, to.Lat}},
		})
		if err == nil {
			return loc.From, geometry
		}
	}
	if hasFrom {
		return loc.From, nil
	}
	if hasTo {
		return loc.To, nil
	}
	return loc.Point, nil
}

// direction returns whichever of the two location subtrees stated one.
func (l *situationLocation) direction() string {
	if l.LinearDirection != "" {
		return l.LinearDirection
	}
	return l.PointDirection
}

// subject names what the record is about, in the publisher's own vocabulary.
//
// The subclass field is preferred because it is the specific claim. For a
// GenericSituationRecord there is no useful subclass field — DGT fills it with
// the literal "incident" — so the detailed cause carries the meaning instead.
func subject(r *situationRecord) string {
	for _, candidate := range []string{
		r.ManagementType,
		r.ObstructionType,
		r.AbnormalTrafficType,
		r.SpeedManagementType,
		r.RoadConditionType,
		r.PoorEnvironmentType,
		r.InstructionType,
	} {
		if candidate != "" {
			return candidate
		}
	}
	if detail := strings.TrimSpace(r.Detail.Any.Value); detail != "" {
		return detail
	}
	if r.GenericName != "" {
		return r.GenericName
	}
	return r.CauseType
}

// situationTitle composes a one-line label, following the DevicePublication
// idiom: the road and its kilometre point, then what is happening there.
func situationTitle(r *situationRecord, at datexPoint) string {
	parts := make([]string, 0, 2)
	switch {
	case r.Location.RoadName != "" && at.KilometerPoint != "":
		parts = append(parts, r.Location.RoadName+" km "+at.KilometerPoint)
	case r.Location.RoadName != "":
		parts = append(parts, r.Location.RoadName)
	}
	if what := humanize(subject(r)); what != "" {
		parts = append(parts, what)
	}
	if len(parts) == 0 {
		return "road situation"
	}
	return strings.Join(parts, " · ")
}

// situationDescription prefers whatever the publisher wrote over anything eye
// composes, and composes only when DGT left every free-text field empty.
//
// The DGT profile has no generalPublicComment; its two free-text slots are the
// operator's locationDescription and the roadDestination on the road
// reference. Both are Spanish as written, and both beat a sentence eye
// assembled from enumerations.
func situationDescription(r *situationRecord, at datexPoint) string {
	if written := strings.TrimSpace(r.Location.Description); written != "" {
		return written
	}
	if written := strings.TrimSpace(r.Location.RoadDestination); written != "" {
		return written
	}

	head := humanize(subject(r))
	if head == "" {
		head = "road situation"
	}
	if r.Location.RoadName != "" {
		head += " on the " + r.Location.RoadName
	}
	if at.KilometerPoint != "" {
		head += " at km " + at.KilometerPoint
	}

	parts := []string{head}
	if place := place(at); place != "" {
		parts = append(parts, place)
	}
	if dir := r.Location.direction(); dir != "" && dir != "unknown" {
		parts = append(parts, humanize(dir))
	}
	return capitalize(strings.Join(parts, ", ")) + "."
}

// place renders the administrative names the Spanish extension carries.
func place(at datexPoint) string {
	switch {
	case at.Municipality != "" && at.Province != "":
		return at.Municipality + " (" + at.Province + ")"
	case at.Municipality != "":
		return at.Municipality
	default:
		return at.Province
	}
}

// situationPayload keeps the source-specific remainder, including the raw
// DATEX type and the administrative names a Córdoba filter needs.
//
// Nothing here is a severity, a confidence or a position in disguise. DGT's
// probabilityOfOccurrence in particular is the publisher's own product
// metadata and stays exactly where it belongs.
func situationPayload(sit situation, r *situationRecord, at datexPoint) json.RawMessage {
	fields := map[string]any{
		"datex_type":          r.Type,
		"situation_id":        sit.ID,
		"situation_record_id": r.ID,
	}
	set := func(key, value string) {
		if value != "" {
			fields[key] = value
		}
	}

	set("situation_record_version", r.Version)
	set("situation_record_creation_reference", r.CreationReference)
	set("information_status", sit.InformationStatus)
	set("validity_status", r.ValidityStatus)
	set("probability_of_occurrence", r.Probability)
	set("severity", r.Severity)
	set("overall_severity", sit.OverallSeverity)
	set("cause_type", r.CauseType)
	set("detailed_cause_type", r.Detail.Any.XMLName.Local)
	set("detailed_cause", strings.TrimSpace(r.Detail.Any.Value))
	set("subject", subject(r))
	set("compliance_option", r.ComplianceOption)
	set("temporary_speed_limit", r.TemporarySpeedLimit)
	set("source_identification", r.SourceID)

	set("road_name", r.Location.RoadName)
	set("road_destination", r.Location.RoadDestination)
	set("location_description", r.Location.Description)
	set("geographic_descriptor", r.Location.GeographicDescriptor)
	set("carriageway", r.Location.Carriageway)
	set("lane_usage", r.Location.LaneUsage)
	set("direction", r.Location.direction())

	set("kilometer_point", at.KilometerPoint)
	set("municipality", at.Municipality)
	set("province", at.Province)
	set("autonomous_community", at.AutonomousCommunity)

	// The far end of a linear extent, so the segment stays legible without
	// re-reading the geometry.
	if _, ok := r.Location.To.position(); ok {
		set("segment_end_kilometer_point", r.Location.To.KilometerPoint)
		set("segment_end_municipality", r.Location.To.Municipality)
		set("segment_end_province", r.Location.To.Province)
	}

	payload, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return payload
}

// localName strips the namespace prefix from an xsi:type value.
func localName(qname string) string {
	if i := strings.IndexByte(qname, ':'); i >= 0 {
		return qname[i+1:]
	}
	return qname
}

// humanize turns a DATEX enumeration literal into readable English:
// "singleAlternateLineTraffic" becomes "single alternate line traffic".
func humanize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// capitalize upper-cases the first rune, leaving the rest alone.
func capitalize(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+len(string(r)):]
	}
	return s
}

// Compile-time proof of the port this adapter satisfies.
var _ provider.Provider = (*SituationProvider)(nil)
