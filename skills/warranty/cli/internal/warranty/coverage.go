// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-novels): vendor responses parsed into
// one coverage-line shape.

package warranty

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Lookup results recorded per device (warranty_devices.last_result).
const (
	ResultOK                 = "ok"
	ResultNotFound           = "not_found"           // vendor has no record (Lenovo 100, Dell tag absent)
	ResultAmbiguous          = "ambiguous"           // Lenovo 101: pass SERIAL.MACHINETYPE
	ResultInvalid            = "invalid"             // Dell invalid: true
	ResultDuplicated         = "duplicated"          // Dell duplicated: true (lines still kept)
	ResultNotInOrg           = "not_in_org"          // Apple 404: not in the configured organization
	ResultError              = "error"               // the request failed
	ResultVendorUnknown      = "vendor_unknown"      // no vendor could be decided
	ResultVendorUnconfigured = "vendor_unconfigured" // that vendor's credentials are not set
)

// Line kinds.
const (
	KindWarranty  = "warranty"
	KindContract  = "contract"
	KindAppleCare = "applecare"
)

// Line is one coverage line, the same shape for every vendor.
type Line struct {
	LineID       string `json:"line_id"`
	Kind         string `json:"kind"`
	Description  string `json:"description"`
	ServiceLevel string `json:"service_level,omitempty"`
	Start        string `json:"start,omitempty"` // YYYY-MM-DD
	End          string `json:"end,omitempty"`   // YYYY-MM-DD; empty = no end date
	// Hardware is false for lines that do not cover hardware repair (Dell
	// service levels D, DL, PJ, PR per PSWarranty). Those never set the
	// device's coverage end.
	Hardware bool   `json:"hardware"`
	Status   string `json:"status,omitempty"`
	// Active is false for a line the vendor marks inactive or canceled; such a
	// line never extends coverage whatever its end date says.
	Active    bool `json:"active"`
	Renewable bool `json:"renewable,omitempty"`
}

// Record is one device's normalized lookup result.
type Record struct {
	Serial       string `json:"serial"`
	Vendor       string `json:"vendor"`
	Result       string `json:"result"`
	Message      string `json:"message,omitempty"`
	Model        string `json:"model,omitempty"`
	ShipDate     string `json:"ship_date,omitempty"`
	PurchaseDate string `json:"purchase_date,omitempty"`
	Lines        []Line `json:"lines"`
}

// dellNonHardware are the Dell service-level codes PSWarranty filters as not
// hardware warranty (KelvinTegelaar/PowerShellWarrantyReports Get-DellWarranty.ps1).
var dellNonHardware = map[string]bool{"D": true, "DL": true, "PJ": true, "PR": true}

// inactiveStatus matches contract statuses that must not extend coverage
// (the same rule LanternOps/breeze lenovoProvider.ts applies).
var inactiveStatus = regexp.MustCompile(`(?i)cancel|pending|suspend|inactive|terminat|expired`)

var isoDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// Day normalizes a vendor date to YYYY-MM-DD (the date as written, UTC for
// RFC 3339 values). Unparseable or empty input gives "".
func Day(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if isoDate.MatchString(s) {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC().Format("2006-01-02")
		}
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return s[:10]
		}
		return ""
	}
	for _, layout := range []string{"01/02/2006", "1/2/2006", "2006/01/02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func lineID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(h[:8])
}

// ---- Dell --------------------------------------------------------------

type dellAsset struct {
	ServiceTag             string            `json:"serviceTag"`
	ShipDate               string            `json:"shipDate"`
	ProductLineDescription string            `json:"productLineDescription"`
	SystemDescription      string            `json:"systemDescription"`
	Duplicated             bool              `json:"duplicated"`
	Invalid                bool              `json:"invalid"`
	Entitlements           []dellEntitlement `json:"entitlements"`
}

type dellEntitlement struct {
	ItemNumber              string `json:"itemNumber"`
	StartDate               string `json:"startDate"`
	EndDate                 string `json:"endDate"`
	EntitlementType         string `json:"entitlementType"`
	ServiceLevelCode        string `json:"serviceLevelCode"`
	ServiceLevelDescription string `json:"serviceLevelDescription"`
}

// ParseDell parses a Dell asset-entitlements array into one Record per
// requested tag. A requested tag the response does not mention is not_found.
func ParseDell(body []byte, requested []string) ([]Record, error) {
	var assets []dellAsset
	if err := json.Unmarshal(body, &assets); err != nil {
		return nil, fmt.Errorf("Dell asset-entitlements: expected a JSON array: %w", err)
	}
	byTag := map[string]dellAsset{}
	for _, a := range assets {
		tag := NormalizeSerial(a.ServiceTag)
		if tag == "" {
			continue
		}
		if prev, ok := byTag[tag]; ok {
			// A tag repeated in one response: keep every entitlement line.
			prev.Entitlements = append(prev.Entitlements, a.Entitlements...)
			prev.Invalid = prev.Invalid || a.Invalid
			prev.Duplicated = true
			byTag[tag] = prev
			continue
		}
		byTag[tag] = a
	}
	out := make([]Record, 0, len(requested))
	for _, tag := range requested {
		tag = NormalizeSerial(tag)
		rec := Record{Serial: tag, Vendor: Dell, Lines: []Line{}}
		a, ok := byTag[tag]
		switch {
		case !ok:
			rec.Result = ResultNotFound
		case a.Invalid:
			rec.Result = ResultInvalid
		default:
			rec.Result = ResultOK
			if a.Duplicated {
				rec.Result = ResultDuplicated
			}
			rec.Model = firstNonEmpty(a.ProductLineDescription, a.SystemDescription)
			rec.ShipDate = Day(a.ShipDate)
			for _, e := range a.Entitlements {
				code := strings.ToUpper(strings.TrimSpace(e.ServiceLevelCode))
				rec.Lines = append(rec.Lines, Line{
					LineID:       lineID(Dell, e.ItemNumber, code, e.StartDate, e.EndDate),
					Kind:         KindWarranty,
					Description:  firstNonEmpty(e.ServiceLevelDescription, e.ItemNumber),
					ServiceLevel: code,
					Start:        Day(e.StartDate),
					End:          Day(e.EndDate),
					Hardware:     !dellNonHardware[code],
					Status:       e.EntitlementType,
					Active:       true,
				})
			}
		}
		rec.Lines = dedupeLines(rec.Lines)
		out = append(out, rec)
	}
	return out, nil
}

// ---- Lenovo ------------------------------------------------------------

type lenovoWarranty struct {
	Serial       string `json:"Serial"`
	ErrorCode    int    `json:"ErrorCode"`
	ErrorMessage string `json:"ErrorMessage"`
	Product      string `json:"Product"`
	Purchased    string `json:"Purchased"`
	Shipped      string `json:"Shipped"`
	UpgradeURL   string `json:"UpgradeUrl"`
	Warranty     []struct {
		ID          string `json:"ID"`
		Name        string `json:"Name"`
		Description string `json:"Description"`
		Type        string `json:"Type"`
		Start       string `json:"Start"`
		End         string `json:"End"`
	} `json:"Warranty"`
	Contract []struct {
		Contract   string `json:"Contract"`
		ItemNumber string `json:"ItemNumber"`
		SLA        string `json:"SLA"`
		Status     string `json:"Status"`
		Start      string `json:"Start"`
		End        string `json:"End"`
	} `json:"Contract"`
}

// ParseLenovo parses a Lenovo /warranty response: one object for one
// Serial, a list for several. Each requested serial gets one Record; a
// requested serial the response does not mention is not_found.
func ParseLenovo(body []byte, requested []string) ([]Record, error) {
	var list []lenovoWarranty
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, fmt.Errorf("Lenovo warranty: %w", err)
		}
	} else {
		var one lenovoWarranty
		if err := json.Unmarshal(body, &one); err != nil {
			return nil, fmt.Errorf("Lenovo warranty: expected a JSON object or array: %w", err)
		}
		list = []lenovoWarranty{one}
	}
	bySerial := map[string]lenovoWarranty{}
	for _, w := range list {
		if s := NormalizeSerial(w.Serial); s != "" {
			bySerial[s] = w
		}
	}
	out := make([]Record, 0, len(requested))
	for _, s := range requested {
		s = NormalizeSerial(s)
		rec := Record{Serial: s, Vendor: Lenovo, Lines: []Line{}}
		w, ok := bySerial[s]
		if !ok && len(requested) == 1 && len(list) == 1 && list[0].Serial == "" {
			// One serial asked, one record back without a Serial field: it
			// can only be the answer to that serial.
			w, ok = list[0], true
		}
		if !ok {
			// A SERIAL.MACHINETYPE request may come back keyed by the bare serial.
			if sn, _, found := strings.Cut(s, "."); found {
				w, ok = bySerial[sn]
			}
		}
		switch {
		case !ok:
			rec.Result = ResultNotFound
		case w.ErrorCode == 100:
			rec.Result = ResultNotFound
			rec.Message = w.ErrorMessage
		case w.ErrorCode == 101:
			rec.Result = ResultAmbiguous
			rec.Message = firstNonEmpty(w.ErrorMessage, "several machines share this serial; look it up as SERIAL.MACHINETYPE")
		case w.ErrorCode != 0:
			rec.Result = ResultError
			rec.Message = fmt.Sprintf("Lenovo error code %d: %s", w.ErrorCode, w.ErrorMessage)
		default:
			rec.Result = ResultOK
			rec.Model = w.Product
			rec.ShipDate = Day(w.Shipped)
			rec.PurchaseDate = Day(w.Purchased)
			for _, l := range w.Warranty {
				rec.Lines = append(rec.Lines, Line{
					LineID:       lineID(Lenovo, "W", l.ID, l.Start, l.End),
					Kind:         KindWarranty,
					Description:  firstNonEmpty(l.Name, l.Description, l.ID),
					ServiceLevel: l.Type,
					Start:        Day(l.Start),
					End:          Day(l.End),
					Hardware:     true,
					Active:       true,
				})
			}
			for _, c := range w.Contract {
				rec.Lines = append(rec.Lines, Line{
					LineID:       lineID(Lenovo, "C", c.Contract, c.ItemNumber, c.Start, c.End),
					Kind:         KindContract,
					Description:  strings.TrimSpace("Contract " + c.Contract),
					ServiceLevel: c.SLA,
					Start:        Day(c.Start),
					End:          Day(c.End),
					Hardware:     true,
					Status:       c.Status,
					Active:       !inactiveStatus.MatchString(c.Status),
					Renewable:    w.UpgradeURL != "",
				})
			}
		}
		rec.Lines = dedupeLines(rec.Lines)
		out = append(out, rec)
	}
	return out, nil
}

// ---- Apple -------------------------------------------------------------

// ParseAppleCoverage parses an Apple Business appleCareCoverage response for
// one serial.
func ParseAppleCoverage(serial string, body []byte) (Record, error) {
	var resp struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Status          string `json:"status"`
				PaymentType     string `json:"paymentType"`
				Description     string `json:"description"`
				StartDateTime   string `json:"startDateTime"`
				EndDateTime     string `json:"endDateTime"`
				IsRenewable     bool   `json:"isRenewable"`
				IsCanceled      bool   `json:"isCanceled"`
				AgreementNumber string `json:"agreementNumber"`
			} `json:"attributes"`
		} `json:"data"`
	}
	rec := Record{Serial: NormalizeSerial(serial), Vendor: Apple, Result: ResultOK, Lines: []Line{}}
	if err := json.Unmarshal(body, &resp); err != nil {
		return rec, fmt.Errorf("Apple appleCareCoverage: %w", err)
	}
	if resp.Data == nil {
		return rec, fmt.Errorf("Apple appleCareCoverage: response has no data array")
	}
	for _, d := range resp.Data {
		a := d.Attributes
		rec.Lines = append(rec.Lines, Line{
			LineID:       lineID(Apple, d.ID, a.Description, a.StartDateTime),
			Kind:         KindAppleCare,
			Description:  firstNonEmpty(a.Description, d.ID),
			ServiceLevel: a.PaymentType,
			Start:        Day(a.StartDateTime),
			End:          Day(a.EndDateTime),
			Hardware:     true,
			Status:       a.Status,
			Active:       strings.EqualFold(a.Status, "ACTIVE") && !a.IsCanceled,
			Renewable:    a.IsRenewable && !a.IsCanceled,
		})
	}
	rec.Lines = dedupeLines(rec.Lines)
	return rec, nil
}

// AppleNextCursor returns meta.paging.nextCursor from an Apple Business list
// response, or "" on the last page.
func AppleNextCursor(body []byte) string {
	var resp struct {
		Meta struct {
			Paging struct {
				NextCursor string `json:"nextCursor"`
			} `json:"paging"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(body, &resp)
	return strings.TrimSpace(resp.Meta.Paging.NextCursor)
}

// AppendLines adds src to dst, dropping lines already present.
func AppendLines(dst, src []Line) []Line {
	return dedupeLines(append(append([]Line{}, dst...), src...))
}

// ParseAppleDevice fills model and order date from an Apple Business
// orgDevices/{id} response.
func ParseAppleDevice(body []byte, rec *Record) error {
	var resp struct {
		Data struct {
			Attributes struct {
				DeviceModel   string `json:"deviceModel"`
				ProductType   string `json:"productType"`
				OrderDateTime string `json:"orderDateTime"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("Apple orgDevices: %w", err)
	}
	a := resp.Data.Attributes
	if m := firstNonEmpty(a.DeviceModel, a.ProductType); m != "" {
		rec.Model = m
	}
	if d := Day(a.OrderDateTime); d != "" {
		rec.PurchaseDate = d
	}
	return nil
}

// ---- Summary -----------------------------------------------------------

// Summary is the device-level view of its coverage lines.
type Summary struct {
	// End is the latest end date among active hardware lines; with none, the
	// latest past end date among inactive ones, so coverage that ended reads as
	// expired ("" when neither). An inactive line ending on or after asOf stays
	// out: it does not cover the device, and it has not ended by that date.
	End string `json:"coverage_end"`
	// Open is true when an active hardware line has no end date (AppleCare+
	// for Business): the device stays covered past End.
	Open bool `json:"coverage_open"`
	// Fingerprint changes whenever any line changes.
	Fingerprint string `json:"-"`
}

// Summarize computes the coverage end, the open flag and a fingerprint, judging
// inactive lines on asOf (YYYY-MM-DD; "" is today).
func Summarize(lines []Line, asOf string) Summary {
	var s Summary
	ended := ""
	if asOf == "" {
		asOf = time.Now().UTC().Format("2006-01-02")
	}
	keys := make([]string, 0, len(lines))
	for _, l := range lines {
		keys = append(keys, strings.Join([]string{l.LineID, l.Start, l.End, l.Status, fmt.Sprint(l.Active),
			fmt.Sprint(l.Renewable), l.ServiceLevel, l.Description, fmt.Sprint(l.Hardware)}, "|"))
		if l.Hardware && !l.Active && l.End < asOf && l.End > ended {
			ended = l.End
		}
		if !l.Hardware || !l.Active || l.Start != "" && l.Start > asOf {
			continue // a line covers from its start date on
		}
		if l.End == "" {
			if l.Kind == KindAppleCare {
				s.Open = true
			}
			continue
		}
		if l.End > s.End {
			s.End = l.End
		}
	}
	// A hardware line starting no later than the day after the covered end
	// continues the coverage (Dell EXTENDED after INITIAL, a Lenovo upgrade after
	// base warranty); only a gap before a future line leaves the end where it is.
	for grew := s.End != ""; grew && !s.Open; {
		grew = false
		next := dayAfter(s.End)
		for _, l := range lines {
			if !l.Hardware || !l.Active || l.Start == "" || l.Start <= asOf || l.Start > next {
				continue
			}
			if l.End == "" {
				if l.Kind == KindAppleCare {
					s.Open = true
				}
				continue
			}
			if l.End > s.End {
				s.End, grew = l.End, true
			}
		}
	}
	if s.End == "" && !s.Open {
		s.End = ended
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		s.Fingerprint = lineID(keys...)
	}
	return s
}

// Status classifies a device on asOf (YYYY-MM-DD): covered, expiring (ends
// within withinDays), expired, or unknown (no coverage data).
func Status(sum Summary, asOf string, withinDays int) (status string, daysLeft *int) {
	if sum.Open {
		return "covered", nil
	}
	if sum.End == "" {
		return "unknown", nil
	}
	n := DaysBetween(asOf, sum.End)
	if n == nil {
		return "unknown", nil
	}
	switch {
	case *n < 0:
		return "expired", n
	case *n <= withinDays:
		return "expiring", n
	}
	return "covered", n
}

// dayAfter returns the day after a YYYY-MM-DD date, or the date itself when it
// does not parse.
func dayAfter(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

// DaysBetween returns to - from in days, or nil when either is not a date.
func DaysBetween(from, to string) *int {
	a, err1 := time.Parse("2006-01-02", from)
	b, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil {
		return nil
	}
	n := int(b.Sub(a).Hours() / 24)
	return &n
}

func dedupeLines(lines []Line) []Line {
	seen := map[string]bool{}
	out := make([]Line, 0, len(lines))
	for _, l := range lines {
		if seen[l.LineID] {
			continue
		}
		seen[l.LineID] = true
		out = append(out, l)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
