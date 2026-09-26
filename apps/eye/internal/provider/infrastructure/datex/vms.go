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

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// kindVmsMessage is what a sign displaying something becomes.
const kindVmsMessage = "vms_message"

// blankPictograms are the codes DGT uses for a panel area that is showing
// nothing: 0 is the empty slot and NEGRO is a black, unlit area. Neither is a
// message, and treating them as one would fill the city view with signs that
// are switched off.
var blankPictograms = map[string]bool{"": true, "0": true, "NEGRO": true}

// vmsPublication is the VmsPublication payload: what every motorway sign in
// Spain is displaying right now.
//
// It is a STATUS publication and carries no coordinates at all — each sign is
// identified only by a reference to a VmsController in a table published
// elsewhere. Records therefore have no Position, which is the honest outcome:
// inventing one would put a sign on a road it is not on.
type vmsPublication struct {
	XMLName xml.Name `xml:"payload"`

	PublicationTime   string                `xml:"publicationTime"`
	InformationStatus string                `xml:"headerInformation>informationStatus"`
	Controllers       []vmsControllerStatus `xml:"vmsControllerStatus"`
}

// vmsControllerStatus is one sign controller and the signs hanging off it.
type vmsControllerStatus struct {
	Controller struct {
		ID          string `xml:"id,attr"`
		TargetClass string `xml:"targetClass,attr"`
	} `xml:"vmsControllerReference"`

	Table struct {
		ID string `xml:"id,attr"`
	} `xml:"vmsControllerTableReference"`

	Signs []vmsSign `xml:"vmsStatus"`
}

// vmsSign is one physical panel. The DGT profile wraps every level in an
// element of its own name, so the inner vmsStatus is not a typo.
type vmsSign struct {
	Index    string       `xml:"vmsIndex,attr"`
	Messages []vmsMessage `xml:"vmsStatus>vmsMessage"`
}

// vmsMessage is one of the messages a sign cycles through.
type vmsMessage struct {
	Index       string           `xml:"messageIndex,attr"`
	TimeLastSet string           `xml:"vmsMessage>timeLastSet"`
	Areas       []vmsDisplayArea `xml:"vmsMessage>displayAreaSettings"`
}

// vmsDisplayArea is one region of the panel: a pictogram or a block of text.
type vmsDisplayArea struct {
	Index    string `xml:"displayAreaIndex,attr"`
	Settings struct {
		Type              string        `xml:"type,attr"`
		PictogramURL      string        `xml:"pictogramDisplayUrl"`
		PictogramCode     string        `xml:"pictogram>customPictogramCode"`
		PictogramFlashing string        `xml:"pictogram>pictogramFlashing"`
		Lines             []vmsTextLine `xml:"textLine"`
	} `xml:"displayAreaSettings"`
}

// vmsTextLine is one line of the panel's text, wrapped twice by the profile.
type vmsTextLine struct {
	Index    string `xml:"lineIndex,attr"`
	Text     string `xml:"textLine>textLine"`
	Flashing string `xml:"textLine>lineFlashing"`
}

// VmsProvider turns the variable message sign feed into records.
//
// A sign displaying nothing is not news, and two thirds of the national fleet
// is dark at any moment. Those are skipped rather than emitted as empty
// records, the same way the GTFS-RT adapter says nothing about a punctual
// train.
type VmsProvider struct {
	src    source.Source
	client *httpx.Client

	validators httpx.Validators
}

// NewVms builds the DGT variable message sign provider.
func NewVms(src source.Source, client *httpx.Client) *VmsProvider {
	return &VmsProvider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *VmsProvider) Info() source.Source { return p.src }

// Poll fetches the publication and emits one record per sign that is
// currently displaying something.
func (p *VmsProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	resp, err := p.client.Get(ctx, p.src.URL, p.validators)
	switch {
	case errors.Is(err, httpx.ErrNotModified):
		p.validators = resp.Validators
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrDatex, err)
	}
	p.validators = resp.Validators

	var doc vmsPublication
	if err := decodeXML(resp.DecodeUTF8(), &doc); err != nil {
		return nil, fmt.Errorf("%w: parse vms: %w", ErrDatex, err)
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
	records := make([]observation.Record, 0, len(doc.Controllers))

	for _, ctrl := range doc.Controllers {
		for i := range ctrl.Signs {
			rec, ok := p.record(doc, ctrl, &ctrl.Signs[i], published, resp.FetchedAt, prov)
			if !ok {
				continue
			}
			records = append(records, rec)
		}
	}
	// A publication in which every sign is dark is a valid, quiet answer,
	// not a failure. Only an unparseable body is a failure, and that was
	// already reported above.
	return records, nil
}

// record normalizes one sign, reporting whether it is displaying anything.
func (p *VmsProvider) record(
	doc vmsPublication,
	ctrl vmsControllerStatus,
	sign *vmsSign,
	published, fetchedAt time.Time,
	prov observation.Provenance,
) (observation.Record, bool) {
	if ctrl.Controller.ID == "" {
		return observation.Record{}, false
	}

	texts, pictograms, urls, lastSet := sign.displayed()
	if len(texts) == 0 && len(pictograms) == 0 {
		return observation.Record{}, false
	}

	// The moment the operator set the message, never the moment eye read
	// it. A sign set at 14:30 and fetched at 00:51 is ten hours of latency,
	// and merging the two would hide it.
	observedAt := lastSet
	if observedAt.IsZero() {
		observedAt = published
	}
	if observedAt.IsZero() {
		return observation.Record{}, false
	}

	local := ctrl.Controller.ID + ":" + signIndex(sign.Index)
	stamp := observedAt.UTC().Format(time.RFC3339)

	title := strings.Join(texts, " / ")
	if title == "" {
		title = "pictogram " + strings.Join(pictograms, ", ")
	}

	rec := observation.Record{
		ID:         p.src.ID + ":" + local + ":" + stamp,
		Source:     p.src.ID,
		Kind:       kindVmsMessage,
		Topic:      p.src.Topic,
		ObservedAt: observedAt,
		FetchedAt:  fetchedAt,
		Title:      title,
		// Position stays nil: see the vmsPublication comment. The feed
		// states no coordinates and eye does not invent them.
		Description: vmsDescription(texts, pictograms),
		// The sign says something is happening; it does not say how bad
		// it is. Reading a severity out of the pictogram code would be
		// eye's guess dressed as DGT's statement.
		Severity:   observation.SeverityInfo,
		Confidence: 1,
		Quality:    observation.QualityOfficial,
		LocalKey:   local,
		DedupeKey:  p.src.ID + ":" + local + ":" + stamp,
		Payload:    vmsPayload(doc, ctrl, sign, texts, pictograms, urls),
		Provenance: prov,
	}

	if err := rec.Validate(); err != nil {
		return observation.Record{}, false
	}
	return rec, true
}

// displayed collects what the sign is actually showing: one entry per distinct
// message text, the meaningful pictogram codes, their image URLs, and the
// latest moment any of it was set.
//
// A sign cycling the same text twice is showing one message, so duplicates are
// folded rather than repeated in the title.
func (s *vmsSign) displayed() (texts, pictograms, urls []string, lastSet time.Time) {
	seenText := make(map[string]bool)
	seenCode := make(map[string]bool)

	for _, msg := range s.Messages {
		if t := parseTime(msg.TimeLastSet); t.After(lastSet) {
			lastSet = t
		}

		var lines []string
		for _, area := range msg.Areas {
			if code := strings.TrimSpace(area.Settings.PictogramCode); !blankPictograms[code] && !seenCode[code] {
				seenCode[code] = true
				pictograms = append(pictograms, code)
				if url := strings.TrimSpace(area.Settings.PictogramURL); url != "" {
					urls = append(urls, url)
				}
			}
			for _, line := range area.Settings.Lines {
				if text := collapseSpace(line.Text); text != "" {
					lines = append(lines, text)
				}
			}
		}

		text := strings.Join(lines, " ")
		if text == "" || seenText[text] {
			continue
		}
		seenText[text] = true
		texts = append(texts, text)
	}
	return texts, pictograms, urls, lastSet
}

// vmsDescription renders what the panel shows, text first and the pictogram
// codes after it, so a pictogram-only sign still describes itself.
func vmsDescription(texts, pictograms []string) string {
	parts := make([]string, 0, 2)
	if len(texts) > 0 {
		parts = append(parts, strings.Join(texts, "\n"))
	}
	if len(pictograms) > 0 {
		parts = append(parts, "pictograms: "+strings.Join(pictograms, ", "))
	}
	return strings.Join(parts, "\n")
}

// vmsPayload keeps the sign's own identifiers and the raw content, so a later
// join against a sign inventory can place it on a map.
func vmsPayload(
	doc vmsPublication,
	ctrl vmsControllerStatus,
	sign *vmsSign,
	texts, pictograms, urls []string,
) json.RawMessage {
	fields := map[string]any{
		"controller_id": ctrl.Controller.ID,
		"vms_index":     signIndex(sign.Index),
		"messages":      len(sign.Messages),
		"text_lines":    texts,
		"pictograms":    pictograms,
	}
	if len(urls) > 0 {
		// Pointers to DGT's pictogram images, not the images. eye
		// records where a symbol can be fetched and never stores one.
		fields["pictogram_urls"] = urls
	}
	if ctrl.Controller.TargetClass != "" {
		fields["controller_target_class"] = ctrl.Controller.TargetClass
	}
	if ctrl.Table.ID != "" {
		fields["controller_table_id"] = ctrl.Table.ID
	}
	if doc.InformationStatus != "" {
		fields["information_status"] = doc.InformationStatus
	}
	if len(sign.Messages) > 0 {
		fields["time_last_set"] = sign.Messages[0].TimeLastSet
	}

	payload, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return payload
}

// signIndex defaults the panel index, which DGT omits on single-panel
// controllers.
func signIndex(index string) string {
	if index == "" {
		return "1"
	}
	return index
}

// collapseSpace folds the panel's layout whitespace — leading padding, the
// carriage returns that separate physical lines — into single spaces, so a
// title reads as one line without losing a word.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Compile-time proof of the port this adapter satisfies.
var _ provider.Provider = (*VmsProvider)(nil)
