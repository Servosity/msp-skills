// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: shared local-store model for the StreamOne ION novel commands.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/store"
)

// flexFloat accepts a JSON number or a numeric string ("1", "26.208").
// ION returns licence counts as strings and money as numbers. Empty and null
// read as 0; anything else that is not a finite number is an error, so a bad
// value fails the row (loadIONBook reports it) instead of becoming 0. Every
// ION amount, count and total the licence book reads is an optFloat, so a
// value ION did not send is unknown, never 0.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("not a number: %q", s)
	}
	*f = flexFloat(v)
	return nil
}

// optFloat is a flexFloat that also records whether ION sent a readable
// value, so a missing cost is not mistaken for a free seat. A value that is
// not a finite number ("n/a", an object) is unknown too, never an error: a
// failed row would drop the whole subscription or order, and its readable
// seats with it (adversarial round 18, P1).
type optFloat struct {
	V   float64
	Set bool
}

func (o *optFloat) UnmarshalJSON(b []byte) error {
	var f flexFloat
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	// ponytail: a magnitude past 1e12 is no real price or seat count and would
	// overflow to Inf in sums (adversarial round 20), so it is unknown too.
	if err := f.UnmarshalJSON(b); err != nil || s == "" || s == "null" || math.Abs(float64(f)) > 1e12 {
		*o = optFloat{}
		return nil
	}
	o.V, o.Set = float64(f), true
	return nil
}

// redactedValue is v after a JSON round trip through the response redactor.
func redactedValue(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return redacted
	}
	var out any
	if err := json.Unmarshal(redactSecretsJSON(b), &out); err != nil {
		return redacted
	}
	return out
}

// lenientDecode unmarshals a JSON object into v (a pointer to a struct) so
// that one field of an unexpected JSON type never drops the whole record:
// a number or bool sent for a string field becomes its literal text (an
// epoch for a date then parses as no date, so it is unknown), "true" or
// "false" for a bool field its value, and any other mismatch (an object for
// a date, a string for a list) is left unset, which the licence book reads as
// unknown (adversarial round 19, P1: a numeric renewalDate dropped a
// subscription and its seats). Nested structs and lists are judged the same
// way; optFloat fields judge themselves.
func lenientDecode(data []byte, v any) error {
	fixed, ok := coerceJSON(data, reflect.TypeOf(v).Elem())
	if !ok {
		return fmt.Errorf("not a JSON object, or a status, trial flag or customer ID of an unreadable type")
	}
	return json.Unmarshal(fixed, v)
}

var jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

// coerceJSON returns msg fitted to type t, or ok false when it cannot fit.
func coerceJSON(msg json.RawMessage, t reflect.Type) (json.RawMessage, bool) {
	return coerceJSONTag(msg, t, "")
}

// coerceJSONTag is coerceJSON for a field with an ion tag: "exact" fits only a
// JSON string (a status sent as a number or bool would read as its text,
// active); "text" keeps an object or list as its JSON text (a currency ION
// sent as {"code":"EUR"} is then a currency of its own, never absent and
// never the subscription's; adversarial round 22).
func coerceJSONTag(msg json.RawMessage, t reflect.Type, tag string) (json.RawMessage, bool) {
	exact := tag == "exact"
	msg = bytes.TrimSpace(msg)
	if len(msg) == 0 {
		return nil, false
	}
	if string(msg) == "null" {
		// A status or trial flag sent as null is unreadable, not absent: it
		// would read as active or paid (Codex round 25, P1). A null customer ID
		// is just no customer, its own group.
		return msg, !(tag == "exact" || (tag == "strict" && t.Kind() == reflect.Bool))
	}
	if reflect.PointerTo(t).Implements(jsonUnmarshalerType) {
		return msg, true
	}
	switch t.Kind() {
	case reflect.String:
		if exact && msg[0] != '"' {
			return nil, false
		}
		switch c := msg[0]; {
		case c == '"':
			return msg, true
		case tag == "text" && (c == '{' || c == '['):
			var buf bytes.Buffer
			if json.Compact(&buf, msg) != nil {
				return nil, false
			}
			b, _ := json.Marshal(buf.String())
			return b, true
		case c == '-' || (c >= '0' && c <= '9') || string(msg) == "true" || string(msg) == "false":
			b, _ := json.Marshal(string(msg))
			return b, true
		}
		return nil, false
	case reflect.Bool:
		var s string
		switch {
		case string(msg) == "true" || string(msg) == "false":
			return msg, true
		case json.Unmarshal(msg, &s) == nil && (strings.EqualFold(s, "true") || strings.EqualFold(s, "false")):
			return json.RawMessage(strings.ToLower(s)), true
		}
		return nil, false
	case reflect.Pointer:
		return coerceJSON(msg, t.Elem())
	case reflect.Slice:
		var items []json.RawMessage
		if json.Unmarshal(msg, &items) != nil {
			// A list ION sent in another shape still held something: it reads
			// as one unknown element (an undated billing row, an item of unknown
			// resource), never as no rows (adversarial round 21).
			items = []json.RawMessage{json.RawMessage("null")}
		}
		out := make([]json.RawMessage, 0, len(items))
		for _, it := range items {
			fixed, ok := coerceJSON(it, t.Elem())
			if !ok {
				fixed = json.RawMessage("null") // an unreadable element stays as an unknown one, never vanishes
			}
			out = append(out, fixed)
		}
		b, _ := json.Marshal(out)
		return b, true
	case reflect.Struct:
		var raw map[string]json.RawMessage
		if json.Unmarshal(msg, &raw) != nil {
			return nil, false
		}
		for k, val := range raw {
			for i := 0; i < t.NumField(); i++ {
				if name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); name != "" && strings.EqualFold(name, k) {
					tag := t.Field(i).Tag.Get("ion")
					fixed, ok := coerceJSONTag(val, t.Field(i).Type, tag)
					switch {
					case ok:
						raw[k] = fixed
					case tag == "strict" || tag == "exact":
						// An unreadable status, trial flag or owner would read as a
						// guess (active, paid, unowned), so the record is skipped
						// with a warning instead (adversarial round 20).
						return nil, false
					default:
						delete(raw, k)
					}
					break
				}
			}
		}
		b, _ := json.Marshal(raw)
		return b, true
	}
	return msg, true
}

// unredacted is an identifier, or "" when the response redactor blanked it:
// "[redacted]" is no key, so two records it blanked never join (adversarial
// round 22: two order items keyed "[redacted]" shared a seat history).
func unredacted(ids ...*string) {
	for _, id := range ids {
		if strings.Contains(*id, redacted) {
			*id = ""
		}
	}
}

func (c *ionCustomer) UnmarshalJSON(b []byte) error {
	type plain ionCustomer
	err := lenientDecode(b, (*plain)(c))
	unredacted(&c.Name)
	return err
}

func (s *ionSub) UnmarshalJSON(b []byte) error {
	type plain ionSub
	err := lenientDecode(b, (*plain)(s))
	unredacted(&s.ID, &s.CustomerID, &s.SubscriptionID, &s.CCPProductID, &s.CCPSkuID, &s.CCPPlanID,
		&s.ProviderProductID, &s.ProviderSkuID, &s.MfgPartNumber)
	return err
}

func (o *ionOrder) UnmarshalJSON(b []byte) error {
	type plain ionOrder
	err := lenientDecode(b, (*plain)(o))
	unredacted(&o.OrderID, &o.CustomerID)
	for i := range o.Items {
		it := &o.Items[i]
		unredacted(&it.ResourceID, &it.ProductID, &it.SkuID, &it.PlanID)
	}
	return err
}

type ionCustomer struct {
	Name         string `json:"name"` // accounts/{a}/customers/{c}
	Organization string `json:"customerOrganization"`
	ContactName  string `json:"customerName"`
	Email        string `json:"customerEmail"`
	Status       string `json:"customerStatus"`
	CreateTime   string `json:"createTime"`
}

// ID returns the numeric customer ID, the last segment of the resource name.
func (c ionCustomer) ID() string { return lastSegment(c.Name) }

type ionBillingRow struct {
	BillingMonth string   `json:"billingMonth"`
	SellerCost   optFloat `json:"sellerCost"`
	CustomerCost optFloat `json:"customerCost"`
	MSRP         optFloat `json:"msrp"`
	Currency     string   `json:"currency" ion:"text"`
}

type ionSub struct {
	ID                    string          `json:"id"`
	CustomerID            string          `json:"customerId" ion:"strict"`
	CustomerName          string          `json:"customerName"`
	SubscriptionID        string          `json:"subscriptionId"` // vendor ID; order item resourceId
	SubscriptionName      string          `json:"subscriptionName"`
	DisplayName           string          `json:"subscriptionDisplayName"`
	ResourceType          string          `json:"resourceType"`
	CloudProviderID       string          `json:"cloudProviderId"`
	CCPProductID          string          `json:"ccpProductId"`
	CCPSkuID              string          `json:"ccpSkuId"`
	CCPPlanID             string          `json:"ccpPlanId"`
	ProviderProductID     string          `json:"subscriptionProductId"`
	ProviderSkuID         string          `json:"subscriptionSkuId"`
	MfgPartNumber         string          `json:"mfgPartNumber"`
	Seats                 optFloat        `json:"subscriptionTotalLicenses"` // Set when ION sent a licence count
	Status                string          `json:"subscriptionStatus" ion:"exact"`
	StartDate             string          `json:"subscriptionStartDate"`
	EndDate               string          `json:"subscriptionEndDate"`
	RenewalDate           string          `json:"renewalDate"`
	CancellationAllowedTo string          `json:"cancellationAllowedUntilDate"`
	BillingCycle          string          `json:"subscriptionBillingCycle"`
	BillingTerm           string          `json:"subscriptionBillingTerm"`
	RenewStatus           string          `json:"subscriptionRenewStatus"`
	AutoRenew             *bool           `json:"autoRenew"`
	IsTrial               bool            `json:"isTrial" ion:"strict"`
	Price                 optFloat        `json:"price"` // Set when ION sent a price, zero included
	Cost                  optFloat        `json:"cost"`
	MSRP                  optFloat        `json:"msrp"`
	Currency              string          `json:"currency" ion:"text"`
	BillingCurrency       string          `json:"billingCurrency" ion:"text"`
	BillingData           []ionBillingRow `json:"billingData"`
	ProductInfo           struct {
		ProductDisplayName string `json:"productDisplayName"`
		SkuDisplayName     string `json:"skuDisplayName"`
		PlanDisplayName    string `json:"planDisplayName"`
	} `json:"ccpProductInfo"`
}

// Label is the best human name for the subscription.
func (s ionSub) Label() string {
	for _, v := range []string{s.DisplayName, s.SubscriptionName, s.ProductInfo.SkuDisplayName, s.CCPSkuID} {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return s.ID
}

// SKULabel names the SKU itself, never the customer's friendly name.
func (s ionSub) SKULabel() string {
	for _, v := range []string{s.ProductInfo.SkuDisplayName, s.SubscriptionName, s.CCPSkuID} {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return s.ID
}

// CurrencyCode returns the subscription currency.
func (s ionSub) CurrencyCode() string {
	if s.Currency != "" {
		return s.Currency
	}
	return s.BillingCurrency
}

// AutoRenewState is "on", "off" or "unknown" from autoRenew, then subscriptionRenewStatus.
func (s ionSub) AutoRenewState() string {
	if s.AutoRenew != nil {
		if *s.AutoRenew {
			return "on"
		}
		return "off"
	}
	switch strings.ToUpper(s.RenewStatus) {
	case "ENABLED":
		return "on"
	case "DISABLED":
		return "off"
	}
	return "unknown"
}

// Inactive reports subscriptions that no longer bill or renew.
func (s ionSub) Inactive() bool {
	switch strings.ToUpper(s.Status) {
	case "CANCELLED", "DELETED", "EXPIRED", "DISABLED", "FAILED", "STOPPED", "ERROR":
		return true
	}
	return false
}

// SKUKey is the cross-distributor join key: Microsoft "<product>:<sku>"
// (the first two segments of a part number like CFQ7TTC0LDPB:0001:P1Y:M),
// else the provider product/SKU pair, else the ION SKU ID. It is empty when ION
// sent none of them; callers must not group empty keys together.
func (s ionSub) SKUKey() string {
	// One reduction for ION and position files: only a Microsoft-shaped part
	// number loses its term segments; any other part number is kept whole, so
	// two vendor SKUs that share a prefix never merge (Codex round 26, P1).
	// A Microsoft-shaped part number reduces to product:SKU; else ION's own
	// product/SKU pair; else any other part number (ACME-STANDARD) whole
	// (Codex round 27, P1; adversarial round 26: the pair outranks a part
	// number that is not Microsoft-shaped).
	if pn := strings.ToUpper(strings.TrimSpace(s.MfgPartNumber)); msPartNumber.MatchString(pn) {
		return fileSKUKey(pn)
	}
	// A pair whose part holds ':' could read two ways ("A:B"+"C" or
	// "A"+"B:C"), so it is no key (adversarial round 30).
	if s.ProviderProductID != "" && s.ProviderSkuID != "" && !strings.Contains(s.ProviderProductID+s.ProviderSkuID, ":") {
		return strings.ToUpper(s.ProviderProductID + ":" + s.ProviderSkuID)
	}
	if strings.TrimSpace(s.MfgPartNumber) != "" {
		return fileSKUKey(s.MfgPartNumber)
	}
	return strings.ToUpper(s.CCPSkuID)
}

// RenewOn is the renewal date, else the end date; zero for open-ended terms.
func (s ionSub) RenewOn() time.Time {
	for _, v := range []string{s.RenewalDate, s.EndDate} {
		if t := parseIONTime(v); !t.IsZero() && t.Year() < 9000 {
			return t
		}
	}
	return time.Time{}
}

// cycleMonthsByWord maps billing cycle and term words to months per period.
var cycleMonthsByWord = map[string]int{
	"monthly": 1, "month": 1, "p1m": 1,
	"quarterly": 3, "quarter": 3, "p3m": 3,
	"semiannual": 6, "semiannually": 6, "biannual": 6, "halfyearly": 6, "p6m": 6,
	"annual": 12, "annually": 12, "yearly": 12, "year": 12, "p1y": 12, "p12m": 12,
	"biennial": 24, "biennially": 24, "p2y": 24, "p24m": 24,
	"triennial": 36, "triennially": 36, "p3y": 36, "p36m": 36,
}

// monthlyFactor converts a per-billing-period amount to per-month from the
// billing cycle: an ISO-8601 period (P1M, P1Y, P3Y) or a word (monthly,
// Semi-Annual, ...). ok is false when the cycle is blank or not recognized: an
// unknown cycle makes every amount unknown, never a monthly one. The billing
// term is the commitment length, not the billing period (a P1Y NCE term is
// often billed monthly), so it is never used in the cycle's place
// (adversarial round 15 and Codex round 17, P1).
func monthlyFactor(cycle string) (float64, bool) {
	w := strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == ' ' {
			return -1
		}
		return r
	}, strings.ToLower(strings.TrimSpace(cycle)))
	if n, ok := cycleMonthsByWord[w]; ok {
		return 1 / float64(n), true
	}
	return 0, false
}

// seatsText and moneyText render an amount for a human table, or "unknown"
// when ION did not send it.
func seatsText(v float64, known bool) string {
	if !known {
		return "unknown"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// optPtr is the value, or nil (JSON null) when ION sent none.
func optPtr(o optFloat) *float64 {
	if !o.Set {
		return nil
	}
	v := o.V
	return &v
}

// seatsJSON is the licence count, or nil (JSON null) when ION sent none.
func seatsJSON(o optFloat) any {
	if !o.Set {
		return nil
	}
	return o.V
}

func moneyText(v float64, known bool) string {
	if !known {
		return "unknown"
	}
	return money(v)
}

// nowFunc is the clock for staleness checks; tests pin it.
var nowFunc = func() time.Time { return time.Now().UTC() }

// parseBillingMonth reads ION billingMonth values, which come both padded and
// unpadded ("2025-09", "2025-9"); comparing them as strings sorts 2025-9 last.
func parseBillingMonth(v string) (time.Time, bool) {
	t, err := time.Parse("2006-1", strings.TrimSpace(v))
	return t, err == nil
}

// billingMonthKey is the zero-padded "2006-01" form, so keys sort by date.
func billingMonthKey(v string) string {
	if t, ok := parseBillingMonth(v); ok {
		return t.Format("2006-01")
	}
	return ""
}

// ionMoney is a subscription's monthly amounts in one currency. An amount ION
// did not send is not known: Price or Cost is then 0 and must not be read.
type ionMoney struct {
	Price, Cost, MSRP float64
	PriceKnown        bool // ION reported a price (zero included); false when it sent none or only part of it
	CostKnown         bool // ION reported a cost (zero included); false when it sent none or only part of it
	MSRPKnown         bool // ION reported an MSRP (zero included); false when it sent none or only part of it
	Currency          string
	Basis             string // "billing:<month>", "subscription", either joined with "+", or "unknown_cycle"
}

// MonthlyMoney returns monthly customer price, seller cost and MSRP, the
// currency those amounts are in, and the basis used. Monthly billing prefers
// the newest billed month (billingData) when it is no older than one cycle and
// its rows share one currency. Annual and longer cycles bill lumpily (a term
// charge, then prorations and credits, and a renewal that may switch cycle),
// so they use the subscription's own price and cost normalized to a month;
// only when ION sends neither are the billed rows of the trailing cycle summed
// instead. When a billed month lacks the price or the cost, the subscription's
// own price and cost are used whole if ION sent both in the same currency.
// Halves are never mixed, because a billed month can be partial (a proration
// or a credit) while the subscription figure is a full period (adversarial
// round 15, P1), and a price or cost ION sent on the subscription is never
// replaced by billed rows (Codex round 15, P1). Otherwise the missing half
// stays unknown, and a billing cycle the CLI does not recognize leaves both
// unknown. Amounts are as ION reports them for the
// subscription; they are not multiplied by the seat count.
func (s ionSub) MonthlyMoney() ionMoney {
	f, ok := monthlyFactor(s.BillingCycle)
	if !ok {
		return ionMoney{Currency: s.CurrencyCode(), Basis: "unknown_cycle"}
	}
	var latest time.Time
	for _, b := range s.BillingData {
		if t, ok := parseBillingMonth(b.BillingMonth); ok && t.After(latest) {
			latest = t
		}
	}
	cycleMonths := int(math.Round(1 / f))
	sub := ionMoney{Price: s.Price.V * f, Cost: s.Cost.V * f, PriceKnown: s.Price.Set, CostKnown: s.Cost.Set,
		MSRP: s.MSRP.V * f, MSRPKnown: s.MSRP.Set, Currency: s.CurrencyCode(), Basis: "subscription"}
	billed, billedOK := s.billedCycle(latest, cycleMonths, f)
	// A price ION sent, zero included, is a price (Codex round 15, P1: an
	// explicit 0 fell through to the billed rows).
	primary, other := billed, sub
	if !billedOK || (cycleMonths > 1 && (s.Price.Set || s.Cost.Set)) {
		primary, other, billedOK = sub, billed, billedOK && cycleMonths > 1
	}
	if primary.Basis != "subscription" && !(primary.PriceKnown && primary.CostKnown) && other.PriceKnown && other.CostKnown && other.Currency == primary.Currency {
		return other
	}
	return primary
}

// billedCycle sums the billed rows of the trailing cycle ending at the newest
// month, scaled to a month; ok is false when the newest month is older than
// one cycle or the rows mix currencies. One row without a customerCost or a
// sellerCost leaves that total unknown.
func (s ionSub) billedCycle(latest time.Time, cycleMonths int, f float64) (ionMoney, bool) {
	if !latest.IsZero() && nowFunc().Before(latest.AddDate(0, cycleMonths+1, 0)) {
		from := latest.AddDate(0, 1-cycleMonths, 0) // the cycle's first month; equal to latest for monthly billing
		basis := "billing:" + latest.Format("2006-01")
		if cycleMonths > 1 {
			basis = "billing:" + from.Format("2006-01") + ".." + latest.Format("2006-01")
		}
		m := ionMoney{Basis: basis, PriceKnown: true, CostKnown: true, MSRPKnown: true}
		mixed, first := false, true
		for _, b := range s.BillingData {
			t, ok := parseBillingMonth(b.BillingMonth)
			if !ok {
				// A row with no readable month may fall in the cycle, so the
				// billed totals are incomplete (adversarial round 19).
				m.PriceKnown, m.CostKnown, m.MSRPKnown = false, false, false
				continue
			}
			if t.Before(from) || t.After(latest) {
				continue
			}
			cur := b.Currency
			if cur == "" {
				cur = s.CurrencyCode()
			}
			// An unknown currency is a currency of its own: it never joins a
			// USD total (Codex round 23, P1).
			if first {
				m.Currency, first = cur, false
			} else if cur != m.Currency {
				mixed = true
				break
			}
			m.Price += b.CustomerCost.V
			m.PriceKnown = m.PriceKnown && b.CustomerCost.Set
			m.Cost += b.SellerCost.V
			m.CostKnown = m.CostKnown && b.SellerCost.Set
			m.MSRP += b.MSRP.V
			m.MSRPKnown = m.MSRPKnown && b.MSRP.Set
		}
		if !mixed {
			m.Price, m.Cost, m.MSRP = m.Price*f, m.Cost*f, m.MSRP*f
			if !m.PriceKnown {
				m.Price = 0
			}
			if !m.CostKnown {
				m.Cost = 0
			}
			return m, true
		}
	}
	return ionMoney{}, false
}

type ionOrderItem struct {
	ResourceID   string   `json:"resourceId"`
	Action       string   `json:"action"`
	ProductID    string   `json:"productId"`
	SkuID        string   `json:"skuId"`
	PlanID       string   `json:"planId"`
	Quantity     optFloat `json:"quantity"` // Set when ION sent a seat count
	Status       string   `json:"status"`
	ProductName  string   `json:"productName"`
	SkuName      string   `json:"skuName"`
	PlanName     string   `json:"planName"`
	Intention    string   `json:"purchaseIntention"`
	ProviderName string   `json:"providerName"`
	Attributes   []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"attributes"`
}

func (it ionOrderItem) attr(name string) string {
	for _, a := range it.Attributes {
		if strings.EqualFold(a.Name, name) {
			return a.Value
		}
	}
	return ""
}

type ionOrder struct {
	OrderID      string         `json:"orderId"`
	CustomerID   string         `json:"customerId" ion:"strict"`
	DisplayName  string         `json:"displayName"`
	Status       string         `json:"status" ion:"exact"`
	CurrencyCode string         `json:"currencyCode" ion:"text"`
	Total        optFloat       `json:"total"`
	UserName     string         `json:"userName"`
	CreateTime   string         `json:"createTime"`
	UpdateTime   string         `json:"updateTime"`
	Items        []ionOrderItem `json:"orderItems"`
}

func (o ionOrder) Created() time.Time { return parseIONTime(o.CreateTime) }
func (o ionOrder) Updated() time.Time {
	if t := parseIONTime(o.UpdateTime); !t.IsZero() {
		return t
	}
	return o.Created()
}

// ionBook is the synced licence book held in memory for one command run.
type ionBook struct {
	Customers map[string]ionCustomer // keyed by numeric customer ID
	Subs      []ionSub
	Orders    []ionOrder
	Skipped   map[string]int // rows per resource that failed to decode
	SkipErr   string         // first decode error, for the warning
}

func (b *ionBook) skip(resource, id string, err error) {
	b.Skipped[resource]++
	if b.SkipErr == "" {
		b.SkipErr = resource + " " + id + ": " + err.Error()
	}
}

// CustomerLabel returns the organization name for a customer ID.
func (b *ionBook) CustomerLabel(id, fallback string) string {
	if c, ok := b.Customers[id]; ok && c.Organization != "" {
		return c.Organization
	}
	if fallback != "" {
		return fallback
	}
	return id
}

func parseIONTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "0001-") {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func lastSegment(name string) string {
	name = strings.TrimRight(name, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// loadIONBook reads the requested resource types from the local store.
// Each ListScan drains fully before the next query runs.
func loadIONBook(ctx context.Context, db *store.Store, resources ...string) (*ionBook, error) {
	b := &ionBook{Customers: map[string]ionCustomer{}, Skipped: map[string]int{}}
	for _, r := range resources {
		var scanErr error
		err := db.ListScan(r, func(id string, data json.RawMessage) bool {
			if ctx.Err() != nil {
				scanErr = ctx.Err()
				return false
			}
			switch r {
			case "customers":
				var c ionCustomer
				if err := json.Unmarshal(data, &c); err != nil {
					b.skip(r, id, err)
				} else if c.ID() != "" { // an empty ID would claim every record that lacks one
					b.Customers[c.ID()] = c
				}
			case "subscriptions":
				var s ionSub
				if err := json.Unmarshal(data, &s); err != nil {
					b.skip(r, id, err)
				} else {
					b.Subs = append(b.Subs, s)
				}
			case "orders":
				var o ionOrder
				if err := json.Unmarshal(data, &o); err != nil {
					b.skip(r, id, err)
				} else {
					b.Orders = append(b.Orders, o)
				}
			}
			return true
		})
		if err != nil {
			return nil, fmt.Errorf("reading local %s: %w", r, err)
		}
		if scanErr != nil {
			return nil, scanErr
		}
	}
	sort.SliceStable(b.Subs, func(i, j int) bool { return b.Subs[i].ID < b.Subs[j].ID })
	return b, nil
}

// openIONBook resolves the DB path, emits the missing-mirror and sync hints,
// and loads the book. A nil book with nil error means "no local mirror":
// callers print an empty result.
func openIONBook(cmd *cobra.Command, flags *rootFlags, dbPath string, resources ...string) (*ionBook, func(), error) {
	explicitDB := dbPath != ""
	if explicitDBFlag(cmd) && strings.TrimSpace(dbPath) == "" {
		return nil, func() {}, usageErr(fmt.Errorf("--db needs a file path"))
	}
	if !explicitDB {
		dbPath = defaultDBPath("td-synnex-cli")
	}
	_, statErr := os.Stat(dbPath)
	if explicitDB && flags != nil {
		// The freshness hook checked only the default store, so its verdict says
		// nothing about a store chosen with --db. The default path is not resolved
		// here: resolving it can adopt or move a store as a side effect.
		decision := "not-checked"
		if os.IsNotExist(statErr) {
			decision = "no-store"
		}
		flags.freshnessMeta = map[string]any{"decision": decision, "db": dbPath, "reason": "--db selects a store the freshness check does not cover"}
	}
	if os.IsNotExist(statErr) {
		fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: td-synnex-cli sync --resources %s --db %s\n", dbPath, strings.Join(resources, ","), dbPath)
		return nil, func() {}, nil
	}
	db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, func() {}, configErr(fmt.Errorf("opening local store: %w", err))
	}
	if len(resources) > 0 && !hintIfUnsynced(cmd, db, resources[0]) {
		hintIfStale(cmd, db, resources[0], flags.maxAge) // ponytail: the primary resource stands in for the set
	}
	b, err := loadIONBook(cmd.Context(), db, resources...)
	if err != nil {
		_ = db.Close() // read-only handle; the load error is the one to report
		return nil, func() {}, err
	}
	if len(b.Skipped) > 0 {
		parts := make([]string, 0, len(b.Skipped))
		for r, n := range b.Skipped {
			parts = append(parts, fmt.Sprintf("%d %s", n, r))
		}
		sort.Strings(parts)
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipped %s rows the CLI could not decode (first: %s); results leave them out\n", strings.Join(parts, ", "), b.SkipErr)
	}
	return b, func() { _ = db.Close() }, nil // read-only handle: nothing to flush
}

// printIONJSON is printJSONFiltered for the local-store novel commands, plus
// the freshness hook's verdict in the --agent meta. Without it an agent sees
// {"meta":{"source":"local"},"results":[]} both when nothing is due and when
// the store was never synced and the refresh failed (said only on stderr).
func printIONJSON(w io.Writer, v any, flags *rootFlags) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	meta := map[string]any{"source": resolveAgentOutputSource(flags, json.RawMessage(raw))}
	if flags != nil && flags.freshnessMeta != nil {
		meta["freshness"] = flags.freshnessMeta
	}
	return printOutputWithFlagsMeta(w, json.RawMessage(raw), flags, meta)
}

// printTable renders a fixed-column human table.
func printTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	return tw.Flush()
}

func money(v float64) string { return strconv.FormatFloat(round2(v), 'f', 2, 64) }

// explicitDBFlag reports whether the command was given its own --db. root.go
// skips the default-store auto-refresh for it: the refresh would resolve,
// create and fill the default store although the command reads only the
// named file.
// argsNameDB reports a --db flag in raw arguments, for paths that run before
// or without flag parsing.
func argsNameDB(args []string) bool {
	for _, a := range args { // ponytail: "--" may be a flag value, so every argument counts
		if a == "--db" || strings.HasPrefix(a, "--db=") {
			return true
		}
	}
	return false
}

func explicitDBFlag(cmd *cobra.Command) bool {
	f := cmd.Flags().Lookup("db")
	// Even blank: openIONBook refuses it rather than fall back to the default
	// store. A db a --profile applied sets the value without marking Changed
	// (adversarial round 28, Codex round 30).
	return f != nil && (f.Changed || f.Value.String() != "")
}

// subsByResource maps a vendor subscription ID to its synced subscription.
// An ID two synced subscriptions share matches neither, so an order item is
// never joined to the wrong record (adversarial round 28).
func subsByResource(b *ionBook) map[string]ionSub {
	out, dup := map[string]ionSub{}, map[string]bool{}
	for _, s := range b.Subs {
		if s.SubscriptionID == "" {
			continue
		}
		k := strings.ToLower(s.SubscriptionID)
		if _, seen := out[k]; seen {
			dup[k] = true
		}
		out[k] = s
	}
	for k := range dup {
		delete(out, k)
	}
	return out
}
