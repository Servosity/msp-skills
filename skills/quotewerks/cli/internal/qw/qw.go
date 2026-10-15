// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package qw holds the QuoteWerks Web domain logic behind the typed quote
// commands: JSON:API parsing, the filter grammar, LineType decoding, margin
// math, product-database discovery, quote outcome classification and the
// pre-send lint rules. It does no IO, so every rule is testable offline.
//
// Column names are the ones the QuoteWerks Web API returns (vendor KB
// "Creating & Updating Quotes", the vendor demo dashboard, and the desktop
// developer integration notes for LineType values).
package qw

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Table and column names used by the typed commands.
const (
	TableHeaders = "DocumentHeaders"
	TableItems   = "DocumentItems"
)

// Resource is one JSON:API resource object as returned by
// /api/v1/qw/tables/{table}. ID is the RecGUID when the table has one.
type Resource struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Attributes map[string]any `json:"attributes"`
}

// Page is a parsed JSON:API list (or single) response.
type Page struct {
	Data []Resource
	Meta map[string]any
}

// ParsePage decodes a JSON:API document whose data is an array or a single
// object. Numbers are kept as json.Number so money values do not lose precision
// before Num converts them. Resource ids may be strings or numbers (the spec:
// "RecGUID when available, otherwise integer PK"). A bare array or object
// without a "data" member is accepted as plain rows.
func ParsePage(raw []byte) (Page, error) {
	var top any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&top); err != nil {
		return Page{}, fmt.Errorf("decoding JSON:API response: %w", err)
	}
	p := Page{}
	var items []any
	switch t := top.(type) {
	case []any:
		for _, it := range t {
			items = append(items, map[string]any{"attributes": it, "id": idOf(it)})
		}
	case map[string]any:
		if meta, ok := t["meta"].(map[string]any); ok {
			p.Meta = meta
		}
		data, hasData := t["data"]
		switch d := data.(type) {
		case []any:
			items = d
		case map[string]any:
			items = []any{d}
		case nil:
			if !hasData {
				items = []any{map[string]any{"attributes": t, "id": idOf(t)}}
			}
		default:
			return Page{}, fmt.Errorf("decoding JSON:API response: unexpected data %T", d)
		}
	default:
		return Page{}, fmt.Errorf("decoding JSON:API response: unexpected document %T", t)
	}
	for _, it := range items {
		obj, ok := it.(map[string]any)
		if !ok {
			return Page{}, fmt.Errorf("decoding JSON:API response: resource is %T, not an object", it)
		}
		r := Resource{Type: Str(obj["type"]), ID: Str(obj["id"])}
		r.Attributes, _ = obj["attributes"].(map[string]any)
		if r.Attributes == nil {
			r.Attributes = map[string]any{}
		}
		p.Data = append(p.Data, r)
	}
	return p, nil
}

// idOf picks a row id from a plain (non-JSON:API) object.
func idOf(v any) string {
	m, _ := v.(map[string]any)
	for _, k := range []string{"RecGUID", "id", "ID"} {
		if s := Str(m[k]); s != "" {
			return s
		}
	}
	return ""
}

// Flat returns the resource as one flat row: "id" plus every attribute.
func (r Resource) Flat() map[string]any {
	out := make(map[string]any, len(r.Attributes)+1)
	for k, v := range r.Attributes {
		out[k] = v
	}
	out["id"] = r.ID
	return out
}

// IgnoredFilters reports filters the server dropped (meta.ignored_filters).
// The vendor contract lets the server ignore a filter on an unknown or
// restricted column and return the unfiltered rows, which is how a mistyped
// column silently widens a result set.
func IgnoredFilters(meta map[string]any) []string {
	raw, ok := meta["ignored_filters"]
	if !ok || raw == nil {
		return nil
	}
	var out []string
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			out = append(out, describeIgnored(item))
		}
	case map[string]any:
		for k, item := range v {
			out = append(out, k+": "+describeIgnored(item))
		}
	case string:
		if v != "" {
			out = append(out, v)
		}
	default:
		out = append(out, fmt.Sprint(v))
	}
	sort.Strings(out)
	return out
}

func describeIgnored(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if name, ok := t["name"].(string); ok && name != "" {
			if reason, ok := t["reason"].(string); ok && reason != "" {
				return name + " (" + reason + ")"
			}
			return name
		}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// Cond builds one filter object in the vendor grammar.
func Cond(name, op string, val any) map[string]any {
	c := map[string]any{"name": name, "op": op}
	if op != "is_null" && op != "is_not_null" {
		c["val"] = val
	}
	return c
}

// Or groups conditions with OR logic.
func Or(conds ...map[string]any) map[string]any {
	items := make([]any, 0, len(conds))
	for _, c := range conds {
		items = append(items, c)
	}
	return map[string]any{"or": items}
}

// Num converts a JSON attribute value to float64. ok is false for null,
// missing and non-numeric values.
func Num(v any) (float64, bool) {
	switch t := v.(type) {
	case nil:
		return 0, false
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	case bool:
		return 0, false
	}
	return 0, false
}

// NumOr returns Num(v) or def when v is not numeric.
func NumOr(v any, def float64) float64 {
	if f, ok := Num(v); ok {
		return f
	}
	return def
}

// Str returns a trimmed string for a scalar attribute value.
func Str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// Round2 rounds to cents.
func Round2(v float64) float64 { return math.Round(v*100) / 100 }

// LineType values from the QuoteWerks developer integration notes.
var lineTypeNames = []struct {
	bit  int
	name string
}{
	{1, "product"}, {2, "comment"}, {4, "subtotal"}, {8, "group_header"},
	{16, "running_subtotal"}, {32, "section_header"}, {64, "percent_discount"},
	{128, "percent_charge"}, {256, "heading"}, {512, "summary"},
}

// LineTypeName decodes a LineType value. A missing value reads as product
// because POSTs default LineType to 1.
func LineTypeName(v any) string {
	n, ok := Num(v)
	if !ok {
		return "product"
	}
	var names []string
	for _, lt := range lineTypeNames {
		if int(n)&lt.bit != 0 {
			names = append(names, lt.name)
		}
	}
	if len(names) == 0 {
		return "unknown(" + strconv.Itoa(int(n)) + ")"
	}
	return strings.Join(names, "+")
}

// IsProductLine reports whether a line carries a priced product or service.
func IsProductLine(attrs map[string]any) bool {
	n, ok := Num(attrs["LineType"])
	if !ok {
		return true
	}
	return int(n)&1 != 0
}

// LineAttributes bits (QuoteWerks Appendix: Developer Integration Notes):
// Exclude 1, HidePrice 2, DontPrint 4, SubItem 8, Option 16, Alternate Price
// Overridden 32, Print Picture 64, IsRecurring 128, SelectedOption 256,
// HideQuantity 512, QV Quantity Change 1024.
const (
	AttrExclude        = 1
	AttrOption         = 16
	AttrIsRecurring    = 128
	AttrSelectedOption = 256
)

// TotalsContribution says whether a line's amounts count toward document
// totals. Exclude lines never count; an Option line counts only when
// SelectedOption is also set. A recurring line counts only when the document
// includes the first payment in its total, which is not a line column, so its
// contribution is unknown rather than guessed.
func TotalsContribution(attrs map[string]any) (counts, known bool) {
	a, ok := Num(attrs["LineAttributes"])
	if !ok {
		return true, true
	}
	bits := int(a)
	switch {
	case bits&AttrExclude != 0:
		return false, true
	case bits&AttrOption != 0 && bits&AttrSelectedOption == 0:
		return false, true
	case bits&AttrIsRecurring != 0:
		return false, false
	}
	return true, true
}

// MarginPct is (price-cost)/price*100. ok is false when price is zero.
func MarginPct(price, cost float64) (float64, bool) {
	// A zero cost is how a missing cost is stored (docs lint reports it as
	// zero-cost), so the margin is unknown, not 100%.
	if price == 0 || cost == 0 {
		return 0, false
	}
	m, ok := MarginRaw(price, cost)
	if !ok {
		return 0, false
	}
	if r := Round2(m); !math.IsInf(r, 0) && !math.IsNaN(r) {
		return r, true
	}
	return 0, false
}

// MarginRaw is the unrounded margin percent, for threshold comparisons.
func MarginRaw(price, cost float64) (float64, bool) {
	if price == 0 {
		return 0, false
	}
	m := (price - cost) / price * 100
	if math.IsInf(m, 0) || math.IsNaN(m) {
		return 0, false
	}
	return m, true
}

// EffectiveQty is the quantity a line is priced on: QtyTotal when present,
// otherwise QtyBase times QtyMultiplier1..4 (each defaults to 1). Vendor:
// "QtyTotal ... is calculated by multiplying QtyBase*QtyM1*QtyM2*QtyM3*QtyM4".
func EffectiveQty(attrs map[string]any) (float64, bool) {
	if t, ok := Num(attrs["QtyTotal"]); ok {
		return t, true
	}
	q, ok := Num(attrs["QtyBase"])
	if !ok {
		return 0, false
	}
	for i := 1; i <= 4; i++ {
		if m, ok := Num(attrs["QtyMultiplier"+strconv.Itoa(i)]); ok {
			q *= m
		}
	}
	return q, true
}

// IsActiveDocument applies the vendor's guidance for document queries
// (TemplateType=0 AND Superceeded=0 AND IsAutoRecoverDocForDocRecGUID=”).
// The columns come from QuoteWerks Desktop; when a tenant's rows do not carry
// them the document counts as active.
func IsActiveDocument(attrs map[string]any) bool {
	if t, ok := Num(attrs["TemplateType"]); ok && t != 0 {
		return false
	}
	switch v := attrs["Superceeded"].(type) {
	case bool:
		if v {
			return false
		}
	default:
		if n, ok := Num(v); ok && n != 0 {
			return false
		}
	}
	return Str(attrs["IsAutoRecoverDocForDocRecGUID"]) == ""
}

// PriceForMargin returns the unit price that yields marginPct over cost.
func PriceForMargin(cost, marginPct float64) (float64, error) {
	if marginPct >= 100 || marginPct <= -100 {
		return 0, fmt.Errorf("margin %.2f%% is out of range; use a value between -100 and 100", marginPct)
	}
	return Round2(cost / (1 - marginPct/100)), nil
}

// PriceForMarkup returns cost marked up by markupPct.
func PriceForMarkup(cost, markupPct float64) float64 {
	return Round2(cost * (1 + markupPct/100))
}

var productTableRe = regexp.MustCompile(`(?i)^products_(.+)_products$`)

// ProductDB returns the product database name for a Products_<Db>_Products
// table. QuoteWerks Web stores one table per product database.
func ProductDB(table string) (string, bool) {
	m := productTableRe.FindStringSubmatch(strings.TrimSpace(table))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// TableNames pulls table names out of a GET /api/v1/qw/tables page. The
// response shape is a JSON:API list whose resource id or name attribute
// carries the table name; both are read so either shape works.
func TableNames(p Page) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	for _, r := range p.Data {
		add(r.ID)
		for _, k := range []string{"name", "table_name", "TableName", "Name"} {
			if s, ok := r.Attributes[k].(string); ok {
				add(s)
			}
		}
	}
	sort.Strings(out)
	return out
}

// NormalizePart canonicalizes a part number for matching: surrounding space
// and letter case only. Inner spaces are significant ("A B" and "AB" can be
// different SKUs).
func NormalizePart(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// ParseDate reads QuoteWerks date values ("2026-09-14T00:00:00", RFC3339 or a
// bare date).
func ParseDate(v any) (time.Time, bool) {
	s := Str(v)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// FilterDate formats a time for a DocDate filter value.
func FilterDate(t time.Time) string { return t.Format("2006-01-02T00:00:00") }

// AgeBucket groups an age in days for pipeline aging.
func AgeBucket(days int) string {
	switch {
	case days <= 7:
		return "0-7"
	case days <= 30:
		return "8-30"
	case days <= 60:
		return "31-60"
	case days <= 90:
		return "61-90"
	}
	return "90+"
}

// AgeBuckets lists buckets in display order.
var AgeBuckets = []string{"0-7", "8-30", "31-60", "61-90", "90+"}

// Outcome of a quote.
const (
	OutcomeWon  = "won"
	OutcomeLost = "lost"
	OutcomeOpen = "open"
)

// Classifier decides whether a quote is won, lost or open.
type Classifier struct {
	WonStatuses  map[string]bool
	LostStatuses map[string]bool
}

// NewClassifier builds a classifier from comma-separated DocStatus values.
func NewClassifier(won, lost string) Classifier {
	return Classifier{WonStatuses: csvSet(won), LostStatuses: csvSet(lost)}
}

func csvSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
			out[p] = true
		}
	}
	return out
}

// Classify returns the outcome and the signal that decided it. Precedence:
// the tenant's own DocStatus lists, then QuoteValetStatusCode (1/2 accepted,
// 3 lost). Online QuoteValet acceptance misses deals won by phone, so the
// signal is reported. ConvertedRef is not used: QuoteWerks defines it as the
// document a record was converted FROM, so it says nothing about a win.
func (c Classifier) Classify(attrs map[string]any) (outcome, signal string) {
	status := strings.ToLower(Str(attrs["DocStatus"]))
	if status != "" && c.WonStatuses[status] {
		return OutcomeWon, "DocStatus"
	}
	if status != "" && c.LostStatuses[status] {
		return OutcomeLost, "DocStatus"
	}
	if code, ok := Num(attrs["QuoteValetStatusCode"]); ok {
		switch int(code) {
		case 1, 2:
			return OutcomeWon, "QuoteValetStatusCode"
		case 3:
			return OutcomeLost, "QuoteValetStatusCode"
		}
		return OutcomeOpen, "QuoteValetStatusCode"
	}
	return OutcomeOpen, "none"
}

// QuoteValetLabel decodes QuoteValetStatusCode.
func QuoteValetLabel(v any) string {
	code, ok := Num(v)
	if !ok {
		return ""
	}
	switch int(code) {
	case 0:
		return "not accepted"
	case 1:
		return "accepted"
	case 2:
		return "accepted (proxy)"
	case 3:
		return "lost"
	}
	return "code " + strconv.Itoa(int(code))
}

// Product is one product-database row reduced to the pricing fields.
type Product struct {
	DB                     string  `json:"product_db"`
	Table                  string  `json:"table"`
	ID                     string  `json:"id"`
	Manufacturer           string  `json:"Manufacturer,omitempty"`
	ManufacturerPartNumber string  `json:"ManufacturerPartNumber,omitempty"`
	VendorPartNumber       string  `json:"VendorPartNumber,omitempty"`
	Description            string  `json:"Description,omitempty"`
	Price                  float64 `json:"Price"`
	Cost                   float64 `json:"Cost"`
	List                   float64 `json:"List"`
	HasPrice               bool    `json:"-"`
	HasCost                bool    `json:"-"`
	HasList                bool    `json:"-"`
	// PricingFactor marks a row priced per unit of pricing other than
	// one; its amounts are not on a per-item basis and are left unknown.
	PricingFactor bool `json:"pricing_factor,omitempty"`
}

// MarshalJSON writes unknown Price, Cost and List as null, never as 0.
func (p Product) MarshalJSON() ([]byte, error) {
	amount := func(v float64, ok bool) *float64 {
		if !ok {
			return nil
		}
		return &v
	}
	return json.Marshal(struct {
		DB                     string   `json:"product_db"`
		Table                  string   `json:"table"`
		ID                     string   `json:"id"`
		Manufacturer           string   `json:"Manufacturer,omitempty"`
		ManufacturerPartNumber string   `json:"ManufacturerPartNumber,omitempty"`
		VendorPartNumber       string   `json:"VendorPartNumber,omitempty"`
		Description            string   `json:"Description,omitempty"`
		Price                  *float64 `json:"Price"`
		Cost                   *float64 `json:"Cost"`
		List                   *float64 `json:"List"`
		PricingFactor          bool     `json:"pricing_factor,omitempty"`
	}{p.DB, p.Table, p.ID, p.Manufacturer, p.ManufacturerPartNumber, p.VendorPartNumber, p.Description,
		amount(p.Price, p.HasPrice), amount(p.Cost, p.HasCost), amount(p.List, p.HasList), p.PricingFactor})
}

// HasPricingFactor reports a UnitOfPricingFactor other than 1. Vendor KB:
// "Extended Price = (QTY) * (UNIT OF PRICING FACTOR) * (UNIT PRICE)"; the unit
// of measure factor only feeds shipping weight. Such amounts are never
// compared with, copied to or derived for per-item amounts.
func HasPricingFactor(attrs map[string]any) bool {
	v, ok := Num(attrs["UnitOfPricingFactor"])
	return ok && v != 1
}

// ProductFromAttrs builds a Product from a product-table row. A zero or
// missing Price, Cost or List counts as unknown: catalogs use 0 for "not
// set", and treating it as real would zero out quoted prices and costs.
func ProductFromAttrs(table, id string, attrs map[string]any) Product {
	db, _ := ProductDB(table)
	p := Product{
		DB: db, Table: table, ID: id,
		Manufacturer:           Str(attrs["Manufacturer"]),
		ManufacturerPartNumber: Str(attrs["ManufacturerPartNumber"]),
		VendorPartNumber:       Str(attrs["VendorPartNumber"]),
		Description:            Str(attrs["Description"]),
	}
	if HasPricingFactor(attrs) {
		p.PricingFactor = true
		return p
	}
	p.Price, p.HasPrice = positive(attrs["Price"])
	p.Cost, p.HasCost = positive(attrs["Cost"])
	p.List, p.HasList = positive(attrs["List"])
	return p
}

func positive(v any) (float64, bool) {
	f, ok := Num(v)
	if !ok || f <= 0 {
		return 0, false
	}
	return f, true
}

// ProductIndex matches quoted lines to product rows.
type ProductIndex struct {
	byMPN    map[string][]Product
	byVendor map[string][]Product
}

// NewProductIndex indexes products by normalized part numbers.
func NewProductIndex(products []Product) *ProductIndex {
	idx := &ProductIndex{byMPN: map[string][]Product{}, byVendor: map[string][]Product{}}
	for _, p := range products {
		if k := NormalizePart(p.ManufacturerPartNumber); k != "" {
			idx.byMPN[k] = append(idx.byMPN[k], p)
		}
		if k := NormalizePart(p.VendorPartNumber); k != "" {
			idx.byVendor[k] = append(idx.byVendor[k], p)
		}
	}
	return idx
}

// Match finds the product for a quoted line: manufacturer part number first
// (preferring the same manufacturer), then vendor part number. Among several
// candidates the lowest cost wins, which is what a buyer would order from.
func (idx *ProductIndex) Match(manufacturer, mpn, vendorPart string) (Product, bool) {
	pick := func(cands []Product) (Product, bool) {
		if len(cands) == 0 {
			return Product{}, false
		}
		m := strings.ToUpper(strings.TrimSpace(manufacturer))
		var same []Product
		for _, c := range cands {
			if m != "" && strings.ToUpper(c.Manufacturer) == m {
				same = append(same, c)
			}
		}
		if len(same) > 0 {
			cands = same
		}
		best := cands[0]
		for _, c := range cands[1:] {
			if c.HasCost && (!best.HasCost || c.Cost < best.Cost) {
				best = c
			}
		}
		return best, true
	}
	if p, ok := pick(idx.byMPN[NormalizePart(mpn)]); ok && NormalizePart(mpn) != "" {
		return p, true
	}
	if NormalizePart(vendorPart) != "" {
		return pick(idx.byVendor[NormalizePart(vendorPart)])
	}
	return Product{}, false
}

// Finding is one pre-send audit result.
type Finding struct {
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	ItemID   string `json:"item_id,omitempty"`
	Part     string `json:"part,omitempty"`
}

// Severities.
const (
	SevError   = "error"
	SevWarning = "warning"
	SevInfo    = "info"
)

// SoldToCompanyMax is the column length cap the API enforces.
const SoldToCompanyMax = 50

// Lint audits one document. idx may be nil when no product rows are available.
func Lint(header map[string]any, lines []map[string]any, idx *ProductIndex, minMargin float64) []Finding {
	out := make([]Finding, 0)
	company := Str(header["SoldToCompany"])
	switch {
	case company == "":
		out = append(out, Finding{Severity: SevWarning, Rule: "missing-customer", Message: "SoldToCompany is empty"})
	case len([]rune(company)) > SoldToCompanyMax:
		out = append(out, Finding{Severity: SevError, Rule: "customer-too-long", Message: fmt.Sprintf("SoldToCompany is %d characters; QuoteWerks Web caps it at %d", len([]rune(company)), SoldToCompanyMax)})
	}
	var sumExt float64
	var haveExt, uncertainLines bool
	for _, line := range lines {
		if !IsProductLine(line) {
			continue
		}
		counts, known := TotalsContribution(line)
		if !known {
			uncertainLines = true
		} else if !counts {
			continue // an excluded line is not printed or calculated
		}
		id := Str(line["id"])
		part := Str(line["ManufacturerPartNumber"])
		label := part
		if label == "" {
			label = Str(line["Description"])
		}
		price, hasPrice := Num(line["UnitPrice"])
		cost, hasCost := Num(line["UnitCost"])
		if HasPricingFactor(line) {
			// Only the price is scaled by the unit of pricing factor, so
			// price-versus-cost checks use the stored extended amounts.
			price, hasPrice = Num(line["ExtendedPrice"])
			cost, hasCost = Num(line["ExtendedCost"])
		}
		qty, hasQty := EffectiveQty(line)
		if ext, ok := Num(line["ExtendedPrice"]); ok && counts {
			sumExt += ext
			haveExt = true
		}
		var prod Product
		var found bool
		if idx != nil && !HasPricingFactor(line) {
			prod, found = idx.Match(Str(line["Manufacturer"]), part, Str(line["VendorPartNumber"]))
		}
		if hasQty && qty <= 0 {
			out = append(out, Finding{Severity: SevError, Rule: "bad-quantity", Message: fmt.Sprintf("%s has quantity %v", label, qty), ItemID: id, Part: part})
		}
		if !hasCost || cost == 0 {
			sev := SevWarning
			msg := fmt.Sprintf("%s has no unit cost, so its margin is unknown", label)
			if found && prod.HasCost && prod.Cost > 0 {
				sev = SevError
				msg = fmt.Sprintf("%s has no unit cost but %s lists cost %.2f", label, prod.DB, prod.Cost)
			}
			out = append(out, Finding{Severity: sev, Rule: "zero-cost", Message: msg, ItemID: id, Part: part})
		}
		if hasPrice && hasCost && cost > 0 && price < cost {
			out = append(out, Finding{Severity: SevError, Rule: "price-below-cost", Message: fmt.Sprintf("%s sells at %.2f below its cost %.2f", label, price, cost), ItemID: id, Part: part})
		} else if hasPrice && hasCost && cost > 0 {
			if raw, ok := MarginRaw(price, cost); ok && raw < minMargin {
				out = append(out, Finding{Severity: SevWarning, Rule: "low-margin", Message: fmt.Sprintf("%s margin %.3f%% is below %.2f%%", label, raw, minMargin), ItemID: id, Part: part})
			}
		}
		if part == "" {
			out = append(out, Finding{Severity: SevWarning, Rule: "missing-part-number", Message: fmt.Sprintf("line %q has no ManufacturerPartNumber", label), ItemID: id})
		}
		if found {
			if hasCost && prod.HasCost && math.Abs(prod.Cost-cost) > 0.01 && cost != 0 {
				out = append(out, Finding{Severity: SevWarning, Rule: "cost-mismatch", Message: fmt.Sprintf("%s quoted cost %.2f, %s cost %.2f", label, cost, prod.DB, prod.Cost), ItemID: id, Part: part})
			}
			if hasPrice && prod.HasPrice && math.Abs(prod.Price-price) > 0.01 {
				out = append(out, Finding{Severity: SevInfo, Rule: "price-differs-from-catalog", Message: fmt.Sprintf("%s quoted price %.2f, %s price %.2f", label, price, prod.DB, prod.Price), ItemID: id, Part: part})
			}
		}
	}
	if sub, ok := Num(header["SubTotal"]); ok && haveExt && !uncertainLines && math.Abs(sub-sumExt) > 0.01 {
		out = append(out, Finding{Severity: SevInfo, Rule: "subtotal-differs", Message: fmt.Sprintf("SubTotal %.2f differs from the sum of product-line ExtendedPrice %.2f (optional lines and discounts can explain this)", sub, Round2(sumExt))})
	}
	return out
}

// CountSeverity counts findings at a severity.
func CountSeverity(fs []Finding, sev string) int {
	n := 0
	for _, f := range fs {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

// ChangeEvent is one difference between two mirror snapshots of a document.
type ChangeEvent struct {
	ID       string `json:"id"`
	DocID    string `json:"doc_id"`
	DocNo    string `json:"DocNo,omitempty"`
	DocType  string `json:"DocType,omitempty"`
	Customer string `json:"SoldToCompany,omitempty"`
	SalesRep string `json:"SalesRep,omitempty"`
	Kind     string `json:"kind"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	At       string `json:"at"`
}

// Change kinds.
const (
	ChangeNew      = "new"
	ChangeAccepted = "accepted"
	ChangeLost     = "lost"
	ChangeStatus   = "status"
	ChangeValue    = "value"
	ChangeRep      = "rep"
	ChangeType     = "type"
	ChangeRemoved  = "removed"
)

// NewChangeEvent builds one change event; the id is unique per document,
// kind and mirror run (RFC3339Nano run time).
func NewChangeEvent(doc map[string]any, kind, from, to string, at time.Time) ChangeEvent {
	stamp := at.UTC().Format(time.RFC3339Nano)
	id := Str(doc["id"])
	return ChangeEvent{
		ID: id + ":" + kind + ":" + stamp, DocID: id, DocNo: Str(doc["DocNo"]), DocType: Str(doc["DocType"]),
		Customer: Str(doc["SoldToCompany"]), SalesRep: Str(doc["SalesRep"]), Kind: kind, From: from, To: to, At: stamp,
	}
}

// DiffDocs compares the previous mirror snapshot with freshly fetched headers.
// Removed candidates are only reported when complete is true: the fetch was
// unfiltered and read every page, so absence can mean deletion (callers still
// confirm each one). A document missing from prev counts as new only when
// inPrevScope says the previous mirror would have fetched it; nil means always.
func DiffDocs(prev map[string]map[string]any, cur []map[string]any, complete bool, at time.Time, inPrevScope func(map[string]any) bool) []ChangeEvent {
	out := make([]ChangeEvent, 0)
	ev := func(doc map[string]any, kind, from, to string) {
		out = append(out, NewChangeEvent(doc, kind, from, to, at))
	}
	seen := make(map[string]bool, len(cur))
	for _, doc := range cur {
		id := Str(doc["id"])
		seen[id] = true
		old, ok := prev[id]
		if !ok {
			if inPrevScope == nil || inPrevScope(doc) {
				ev(doc, ChangeNew, "", Str(doc["DocStatus"]))
			}
			continue
		}
		oldQV, oldHas := Num(old["QuoteValetStatusCode"])
		newQV, newHas := Num(doc["QuoteValetStatusCode"])
		if newHas && (!oldHas || oldQV != newQV) {
			switch int(newQV) {
			case 1, 2:
				ev(doc, ChangeAccepted, QuoteValetLabel(old["QuoteValetStatusCode"]), QuoteValetLabel(doc["QuoteValetStatusCode"]))
			case 3:
				ev(doc, ChangeLost, QuoteValetLabel(old["QuoteValetStatusCode"]), QuoteValetLabel(doc["QuoteValetStatusCode"]))
			}
		}
		for _, f := range []struct{ col, kind string }{{"DocStatus", ChangeStatus}, {"SalesRep", ChangeRep}, {"DocType", ChangeType}} {
			if a, b := Str(old[f.col]), Str(doc[f.col]); a != b {
				ev(doc, f.kind, a, b)
			}
		}
		a, aok := Num(old["GrandTotal"])
		b, bok := Num(doc["GrandTotal"])
		amount := func(v float64, ok bool) string {
			if !ok {
				return ""
			}
			return strconv.FormatFloat(Round2(v), 'f', 2, 64)
		}
		if aok != bok || (aok && amount(a, true) != amount(b, true)) { // compare what is shown
			ev(doc, ChangeValue, amount(a, aok), amount(b, bok))
		}
	}
	if complete {
		ids := make([]string, 0, len(prev))
		for id := range prev {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if !seen[id] {
				ev(prev[id], ChangeRemoved, Str(prev[id]["DocStatus"]), "")
			}
		}
	}
	return out
}
