// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: cross-distributor licence true-up from local + file positions.
// pp:data-source local

package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/cliutil"

	"golang.org/x/text/unicode/norm"
)

// trueupInput is one position row from any source, keyed by canonical column name.
type trueupInput struct {
	Source string
	Row    positionRow
}

type trueupRow struct {
	Customer      string             `json:"customer"`
	MatchedOn     string             `json:"matched_on"`
	SKUKey        string             `json:"sku_key"`
	SKUName       string             `json:"sku_name"`
	SeatsBySource map[string]float64 `json:"seats_by_source"` // known seat counts only
	RowsBySource  map[string]int     `json:"rows_by_source"`
	// UnknownSeatRows counts rows per source whose seat count is unknown; their
	// seats are in no total, and the row is flagged seats_incomplete.
	UnknownSeatRows map[string]int     `json:"unknown_seat_rows_by_source"`
	TotalSeats      float64            `json:"total_seats"`
	TotalKnown      bool               `json:"total_seats_known"` // false when a supplier row has an unknown count: total_seats then covers the known rows only
	TrialSeats      float64            `json:"trial_seats"`       // TD SYNNEX trial seats inside total_seats; billed seats are compared with the rest
	BilledSeats     *float64           `json:"billed_seats"`
	UnitCostBySrc   map[string]float64 `json:"unit_cost_by_source"`
	UnitCostCur     map[string]string  `json:"unit_cost_currency"` // per source; a source with mixed currencies gets no unit cost
	Sources         int                `json:"sources"`
	Flags           []string           `json:"flags"`
	group           int                // the customer group, a stable tie-break
}

type trueupView struct {
	Rows      []trueupRow      `json:"rows"`
	Unmatched []map[string]any `json:"unmatched"`
	Inputs    map[string]int   `json:"inputs"`
}

var nonAlnum = regexp.MustCompile(`[^\p{L}\p{M}\p{N}]+`) // any script, vowel signs kept: 北京/東京 Inc and किरण/कुरण Inc stay apart

// thousandsNum matches "1,000" or "12,345.50"; a lone comma ("1,5") stays an error.
var thousandsNum = regexp.MustCompile(`^-?\d{1,3}(,\d{3})+(\.\d+)?$`)

// parseAmount reads a number from an export cell, accepting thousands separators.
func parseAmount(v string) (float64, error) {
	v = strings.TrimSpace(v)
	if thousandsNum.MatchString(v) {
		v = strings.ReplaceAll(v, ",", "")
	}
	f, err := strconv.ParseFloat(v, 64)
	// ponytail: same 1e12 bound as ION amounts, so a sum cannot overflow to Inf.
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 1e12 {
		return 0, fmt.Errorf("not a number: %q", v)
	}
	return f, nil
}

// cellString renders a JSON cell: null is empty, numbers keep every digit
// (fmt.Sprint would turn null into "<nil>" and 1234567 into 1.234567e+06).
func cellString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

// maxPositionFileBytes caps one --with/--billed file.
const maxPositionFileBytes = 50 << 20

// checkPositionFile limits --with/--billed to regular .csv/.json export files
// outside hidden directories and this CLI's own config, data, state and cache
// directories. The command is read-only over MCP, so an agent must not be able
// to point it at a credential file and read it back through unmatched rows.
func checkPositionFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	resolved := abs
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		resolved = r
	}
	for _, p := range []string{abs, resolved} {
		if ext := strings.ToLower(filepath.Ext(p)); ext != ".csv" && ext != ".json" {
			return fmt.Errorf("%s: position files must be .csv or .json exports", path)
		}
		for _, part := range strings.Split(filepath.ToSlash(p), "/") {
			if strings.HasPrefix(part, ".") {
				return fmt.Errorf("%s: refusing a hidden file or a file in a hidden directory", path)
			}
		}
	}
	for _, dirFn := range []func() (string, error){cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir} {
		d, err := dirFn()
		if err != nil || d == "" {
			continue
		}
		if r, err := filepath.EvalSymlinks(d); err == nil {
			d = r
		}
		if rel, err := filepath.Rel(d, resolved); err == nil && !strings.HasPrefix(rel, "..") {
			return fmt.Errorf("%s: refusing a file inside this CLI's own directories", path)
		}
	}
	st, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file", path)
	}
	if st.Size() > maxPositionFileBytes {
		return fmt.Errorf("%s: larger than %d MB", path, maxPositionFileBytes>>20)
	}
	return nil
}

// normalizeDomain lowercases a domain and strips a scheme, a leading www. and
// any path, so https://www.Acme.com/ and acme.com compare equal.
func normalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	return strings.TrimSuffix(strings.TrimPrefix(d, "www."), ".")
}

// normalizeCompany lowercases, drops punctuation and trailing legal suffixes so
// "SilverLeaf, Inc." and "silverleaf inc" compare equal. Exact after normalizing.
func normalizeCompany(name string) string {
	words := strings.Fields(nonAlnum.ReplaceAllString(strings.ToLower(norm.NFKC.String(name)), " "))
	suffixes := map[string]bool{"inc": true, "llc": true, "ltd": true, "limited": true, "corp": true, "corporation": true,
		"co": true, "company": true, "gmbh": true, "bv": true, "pty": true, "plc": true, "sa": true, "ag": true, "srl": true, "ab": true, "as": true}
	for len(words) > 1 && suffixes[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	return strings.Join(words, " ")
}

// parsePositionFile reads CSV (header row) or a JSON array of objects, renames
// columns via mapping (incoming -> canonical), and fills distributor from the
// file name when the column is empty. A missing or unreadable seat count, or
// an unreadable cost or price, makes that value unknown (never 0): the row
// still joins its customer and SKU, which are then flagged seats_incomplete
// and get no billing verdict. Only inactive rows come back as rejected.
func parsePositionFile(name string, data []byte, mapping map[string]string) ([]positionRow, []map[string]any, error) {
	records := make([]map[string]string, 0)
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))) // Excel and PSA exports often start with a BOM
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var objs []map[string]any
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.UseNumber() // keep every digit of large numeric customer IDs
		if err := dec.Decode(&objs); err != nil {
			return nil, nil, fmt.Errorf("%s: parsing JSON: %w", name, err)
		}
		if _, err := dec.Token(); err != io.EOF {
			return nil, nil, fmt.Errorf("%s: parsing JSON: unexpected content after the array", name)
		}
		for _, o := range objs {
			rec := map[string]string{}
			for k, v := range o {
				rec[k] = cellString(v)
			}
			records = append(records, rec)
		}
	} else {
		r := csv.NewReader(bytes.NewReader(trimmed))
		r.FieldsPerRecord = -1
		r.TrimLeadingSpace = true
		header, err := r.Read()
		if err != nil {
			return nil, nil, fmt.Errorf("%s: reading CSV header: %w", name, err)
		}
		seen := map[string]bool{}
		for _, h := range header {
			if k := strings.ToLower(strings.TrimSpace(h)); seen[k] {
				return nil, nil, fmt.Errorf("%s: column %q appears twice; keep one", name, strings.TrimSpace(h))
			} else {
				seen[k] = true
			}
		}
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, fmt.Errorf("%s: reading CSV: %w", name, err)
			}
			m := map[string]string{}
			for i, h := range header {
				if i < len(rec) {
					m[strings.TrimSpace(h)] = strings.TrimSpace(rec[i])
				}
			}
			records = append(records, m)
		}
	}
	defaultDist := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	rows := make([]positionRow, 0, len(records))
	rejected := make([]map[string]any, 0)
	for n, rec := range records {
		canon := map[string]string{}
		from := map[string]string{}
		for k, v := range rec {
			orig := k
			if to, ok := mapping[strings.ToLower(k)]; ok {
				k = to
			}
			k = strings.ToLower(k)
			// Two columns landing on one name (seats plus --map quantity=seats)
			// would be read in random map order; refuse rather than guess.
			if prev, dup := from[k]; dup {
				return nil, nil, fmt.Errorf("%s: columns %q and %q both map to %q; drop one or change --map", name, prev, orig, k)
			}
			from[k] = orig
			canon[k] = v
		}
		// An identity a redactor blanked is unknown, never a key (Codex round 27, P1).
		for _, k := range []string{"customer_id", "customer_name", "tenant_id", "customer_domain", "sku_key"} {
			if strings.Contains(strings.ToLower(canon[k]), strings.ToLower(redacted)) {
				canon[k] = ""
			}
		}
		dist := canon["distributor"]
		if dist == "" {
			dist = defaultDist
		}
		reason, seatsProblem := "", ""
		seats, err := parseAmount(canon["seats"])
		switch {
		case canon["seats"] == "" || strings.EqualFold(canon["seats_known"], "false"):
			seats, seatsProblem = 0, "missing seats" // a position export row with seats_known false carries no count (Codex round 17, P1)
		case err != nil:
			seats, seatsProblem = 0, "invalid seats "+strconv.Quote(canon["seats"])
		}
		costKnown := canon["monthly_cost"] != "" && !strings.EqualFold(canon["cost_known"], "false")
		priceKnown := canon["monthly_price"] != "" && !strings.EqualFold(canon["price_known"], "false")
		var cost, price float64
		if costKnown {
			if cost, err = parseAmount(canon["monthly_cost"]); err != nil {
				cost, costKnown = 0, false
			}
		}
		if priceKnown {
			if price, err = parseAmount(canon["monthly_price"]); err != nil {
				price, priceKnown = 0, false
			}
		}
		// The TD SYNNEX side counts only active subscriptions, so a cancelled
		// row in a file must not count either.
		if reason == "" && inactiveStatus[firstStatusWord(canon["status"])] {
			reason = "status " + strconv.Quote(canon["status"]) + " is not active"
		}
		if reason != "" {
			rejected = append(rejected, map[string]any{"distributor": strings.ToLower(dist), "row": n + 1, "customer_name": canon["customer_name"],
				"sku_key": strings.ToUpper(canon["sku_key"]), "reason": reason})
			continue
		}
		rows = append(rows, positionRow{
			Schema: positionSchema, Distributor: strings.ToLower(dist), CustomerID: canon["customer_id"],
			CustomerName: canon["customer_name"], TenantID: canon["tenant_id"], CustomerDomain: canon["customer_domain"],
			Vendor: canon["vendor"], Product: canon["product"], SKUKey: fileSKUKey(canon["sku_key"]), SKUName: canon["sku_name"],
			Seats: seats, SeatsKnown: seatsProblem == "", seatsProblem: seatsProblem, MonthlyCost: cost, MonthlyPrice: price,
			Currency: canon["currency"], Status: canon["status"], SourceID: canon["source_id"], CostKnown: costKnown, PriceKnown: priceKnown,
		})
	}
	return rows, rejected, nil
}

// msPartNumber is a full Microsoft part number: a 12-character product ID, a
// 4-character SKU ID (digits or letters: 0001, 000B, 000P), then term and
// billing segments (CFQ7TTC0LFLX:0001:P1Y:Y; adversarial round 25, P1).
var msPartNumber = regexp.MustCompile(`^([A-Z0-9]{12}:[A-Z0-9]{4}):`)

// fileSKUKey reduces a file sku_key that is a full Microsoft part number to
// product:SKU, the way ION part numbers are reduced (adversarial round 22).
// Other colon-separated keys are kept whole, so two vendor SKUs that share a
// prefix never merge (adversarial round 23).
func fileSKUKey(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if m := msPartNumber.FindStringSubmatch(v); m != nil {
		return m[1]
	}
	return v
}

// inactiveStatus are file statuses that mean the licence is no longer held,
// matching ionSub.Inactive on the TD SYNNEX side (a suspended licence counts on
// both sides). The first word is judged: "Cancelled (refunded)" is cancelled,
// "Pending cancellation" is still active.
var inactiveStatus = map[string]bool{"cancelled": true, "canceled": true, "deleted": true, "inactive": true,
	"expired": true, "terminated": true, "disabled": true, "failed": true, "stopped": true, "error": true}

func firstStatusWord(s string) string {
	if f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) }); len(f) > 0 {
		return f[0]
	}
	return ""
}

// mixedCurrency marks a source whose rows for one customer+SKU use more than one currency.
const mixedCurrency = "\x00mixed"

// trueup joins positions by customer (tenant_id, then customer_domain, then
// normalized name, unioned across rows) and sku_key. External rows whose
// customer never meets a td-synnex row, and any row that lacks a sku_key or
// (on the TD SYNNEX side) a seat count, are unmatched. The "psa" distributor is
// treated as billed seats, not a supplier; with billed set, a customer and SKU
// that holds supplier seats and no billed row is flagged not_billed.
func trueup(inputs []positionRow, withFiles, billed bool) trueupView {
	parent := make([]int, len(inputs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	// Each TD SYNNEX customer is one group (its rows join on the customer ID).
	// External rows are then matched one at a time: a row collects every group
	// its keys point at (customer ID within its own source, tenant, normalized
	// domain, normalized name) and joins them only when they hold at most one
	// TD SYNNEX customer and nothing in the merged group conflicts (two customer
	// IDs from one source, or two tenants). A row that points at more than one
	// customer is ambiguous: it joins nothing and its keys are not indexed, so
	// it cannot steer later rows. Different domains do not block a join (one
	// customer is often acme.com in one export and acme.onmicrosoft.com in
	// another); a group with more than one custom domain is flagged instead.
	type ident struct {
		ids     map[string]string // source -> customer ID
		tenant  string
		domains map[string]bool
		srcs    map[string]bool
		hasION  bool
	}
	newIdent := func(r positionRow) ident {
		id := ident{ids: map[string]string{}, tenant: strings.ToLower(strings.TrimSpace(r.TenantID)), domains: map[string]bool{},
			srcs: map[string]bool{r.Distributor: true}, hasION: r.Distributor == "td-synnex"}
		if r.CustomerID != "" {
			id.ids[r.Distributor] = r.CustomerID
		}
		if d := normalizeDomain(r.CustomerDomain); d != "" {
			id.domains[d] = true
		}
		return id
	}
	combine := func(x, y ident) (ident, bool) {
		out := ident{ids: map[string]string{}, tenant: x.tenant, domains: map[string]bool{}, srcs: map[string]bool{}, hasION: x.hasION || y.hasION}
		for d, id := range x.ids {
			out.ids[d] = id
		}
		for d, id := range y.ids {
			if other, ok := out.ids[d]; ok && other != id {
				return out, false
			}
			out.ids[d] = id
		}
		if out.tenant == "" {
			out.tenant = y.tenant
		} else if y.tenant != "" && y.tenant != out.tenant {
			return out, false
		}
		for _, m := range []map[string]bool{x.domains, y.domains} {
			for d := range m {
				out.domains[d] = true
			}
		}
		for _, m := range []map[string]bool{x.srcs, y.srcs} {
			for d := range m {
				out.srcs[d] = true
			}
		}
		return out, true
	}
	idents := make([]ident, len(inputs))
	for i, r := range inputs {
		idents[i] = newIdent(r)
	}
	// A struct key: no text in a source or an ID can make two pairs one key
	// ("pax8:eu"+"C" vs "pax8"+"eu:C", or the same with a NUL; Codex rounds 31
	// and 32, adversarial round 30).
	type ikey struct{ kind, src, val string }
	keysOf := func(r positionRow) []ikey {
		keys := make([]ikey, 0, 4)
		if r.CustomerID != "" {
			keys = append(keys, ikey{"cid", r.Distributor, r.CustomerID})
		}
		for _, k := range [][3]string{{"tenant", strings.ToLower(strings.TrimSpace(r.TenantID)), r.TenantID}, {"domain", normalizeDomain(r.CustomerDomain), r.CustomerDomain}, {"name", normalizeCompany(r.CustomerName), r.CustomerName}} {
			// A value the redactor blanked is no key: "[redacted]" would join
			// every customer it blanked, and a file customer named Redacted
			// (Codex round 29, P1).
			if k[1] != "" && !strings.Contains(k[2], redacted) {
				keys = append(keys, ikey{k[0], "", k[1]})
			}
		}
		return keys
	}
	index := map[ikey][]int{}
	register := func(i int) {
		for _, k := range keysOf(inputs[i]) {
			index[k] = append(index[k], i)
		}
	}
	join := func(a, b int, merged ident) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
		idents[find(b)] = merged
	}
	rank := map[string]int{"name": 1, "domain": 2, "tenant": 3}
	type event struct {
		row  int
		kind string
	}
	events := make([]event, 0)
	ambiguous := map[int]bool{}      // external rows whose keys point at more than one customer
	ambiguousNear := map[int][]int{} // the rows whose customers an ambiguous row pointed at
	for i, r := range inputs {
		if r.Distributor != "td-synnex" {
			continue
		}
		if js := index[ikey{"cid", "td-synnex", r.CustomerID}]; r.CustomerID != "" && len(js) > 0 {
			merged, _ := combine(idents[find(js[0])], idents[i])
			join(i, js[0], merged)
		}
		register(i)
	}
	for i, r := range inputs {
		if r.Distributor == "td-synnex" {
			continue
		}
		roots := make([]int, 0, 2)
		kindOf := map[int]string{}
		for _, k := range keysOf(r) {
			for _, j := range index[k] {
				root := find(j)
				if _, seen := kindOf[root]; !seen {
					kindOf[root] = k.kind // keys run strongest first
					roots = append(roots, root)
				}
			}
		}
		merged, ok, ion := idents[i], true, 0
		for _, root := range roots {
			if idents[root].hasION {
				ion++
			}
			if merged, ok = combine(merged, idents[root]); !ok {
				break
			}
		}
		if !ok || ion > 1 {
			ambiguous[i] = true
			// The row may belong to any of these customers, so none of them gets
			// a billing verdict for its SKU (adversarial round 18, P2).
			for _, root := range roots {
				ambiguousNear[i] = append(ambiguousNear[i], root)
			}
			continue
		}
		for _, root := range roots {
			other := false
			for src := range idents[root].srcs {
				if src != r.Distributor {
					other = true
				}
			}
			join(i, root, merged)
			if kindOf[root] != "cid" && other {
				events = append(events, event{i, kindOf[root]})
			}
		}
		register(i)
	}
	groupHasION := map[int]bool{}
	for i, r := range inputs {
		if r.Distributor == "td-synnex" {
			groupHasION[find(i)] = true
		}
	}
	type key struct {
		group int
		sku   string
	}
	agg := map[key]*trueupRow{}
	groupMatch := map[int]string{}
	for _, e := range events {
		g := find(e.row)
		if rank[e.kind] > rank[groupMatch[g]] {
			groupMatch[g] = e.kind
		}
	}
	costBySrc := map[key]map[string]float64{}   // summed monthly cost per source
	pricedSeats := map[key]map[string]float64{} // seats on rows that carried a cost
	costCur := map[key]map[string]string{}      // currency per source, or mixedCurrency
	seatsUnknown := map[key]bool{}              // a TD SYNNEX row for this customer+SKU has no seat count
	blankSKU := map[int]bool{}                  // a matched customer has a file row with no sku_key
	trialSeats := map[key]float64{}
	view := trueupView{Rows: make([]trueupRow, 0), Unmatched: make([]map[string]any, 0), Inputs: map[string]int{}}
	for i, r := range inputs {
		view.Inputs[r.Distributor]++
		g := find(i)
		ion := r.Distributor == "td-synnex"
		// A row with no SKU identity is listed, never grouped (Codex round 16,
		// P1: two subscriptions with no SKU identifiers merged into one
		// duplicate). A row with an unknown seat count joins its group, adds no
		// seats, and keeps the group from any billing verdict.
		if r.SKUKey == "" && (ion || (groupHasION[g] && !ambiguous[i])) {
			blankSKU[g] = true // the row (a file row or a TD SYNNEX subscription) may belong to any of this customer's SKUs
		}
		if r.SKUKey == "" || (!ion && (!groupHasION[g] || ambiguous[i])) {
			reason := "customer not found in TD SYNNEX positions"
			switch {
			case r.SKUKey == "":
				reason = "missing sku_key"
			case ambiguous[i]:
				reason = "ambiguous customer match: its tenant, domain or name point at different customers"
			}
			var seats any = r.Seats
			if !r.SeatsKnown {
				seats = nil
				if r.seatsProblem != "" {
					reason += "; " + r.seatsProblem
				}
			}
			view.Unmatched = append(view.Unmatched, map[string]any{"distributor": r.Distributor, "customer_name": r.CustomerName,
				"tenant_id": r.TenantID, "customer_domain": r.CustomerDomain, "sku_key": r.SKUKey, "seats": seats, "reason": reason})
			continue
		}
		k := key{g, r.SKUKey}
		t := agg[k]
		if costBySrc[k] == nil {
			costBySrc[k], pricedSeats[k], costCur[k] = map[string]float64{}, map[string]float64{}, map[string]string{}
		}
		if t == nil {
			t = &trueupRow{group: g, Customer: r.CustomerName, SKUKey: r.SKUKey, SKUName: r.SKUName, SeatsBySource: map[string]float64{}, UnknownSeatRows: map[string]int{},
				RowsBySource: map[string]int{}, UnitCostBySrc: map[string]float64{}, UnitCostCur: map[string]string{}, Flags: make([]string, 0)}
			agg[k] = t
		}
		if r.Distributor == "td-synnex" {
			t.Customer = r.CustomerName // prefer the ION organization name
			if r.SKUName != "" {
				t.SKUName = r.SKUName
			}
		}
		t.RowsBySource[r.Distributor]++
		if !r.SeatsKnown {
			t.UnknownSeatRows[r.Distributor]++
			seatsUnknown[k] = true
			continue // no seats to add and no unit cost to weigh
		}
		t.SeatsBySource[r.Distributor] += r.Seats
		if ion && r.Trial {
			trialSeats[k] += r.Seats
		}
		if r.Seats > 0 && r.CostKnown { // known zero or negative costs (free seats, credits) count too
			cur := strings.ToUpper(r.Currency)
			if prev, seen := costCur[k][r.Distributor]; !seen {
				costCur[k][r.Distributor] = cur
			} else if prev != cur {
				costCur[k][r.Distributor] = mixedCurrency
			}
			costBySrc[k][r.Distributor] += r.MonthlyCost
			pricedSeats[k][r.Distributor] += r.Seats
		}
	}
	unsure := map[key]bool{} // a customer+SKU an ambiguous row, or a row with no sku_key, may belong to
	for i, near := range ambiguousNear {
		for _, j := range near {
			if inputs[i].SKUKey == "" {
				blankSKU[find(j)] = true // an ambiguous row with no sku_key may belong to any SKU of any of them (Codex round 24, P1)
			} else {
				unsure[key{find(j), inputs[i].SKUKey}] = true
			}
		}
	}
	for k, t := range agg {
		for src, cost := range costBySrc[k] {
			if cur := costCur[k][src]; cur != mixedCurrency {
				if u := round2(cost / pricedSeats[k][src]); !math.IsInf(u, 0) && !math.IsNaN(u) {
					t.UnitCostBySrc[src] = u // a tiny seat count would divide to Inf and break the JSON report
					t.UnitCostCur[src] = cur
				}
			}
		}
		t.MatchedOn = groupMatch[k.group]
		if t.MatchedOn == "" {
			t.MatchedOn = "single-source"
		}
		suppliers := 0
		for src := range t.RowsBySource {
			if src == "psa" {
				if seats, ok := t.SeatsBySource[src]; ok {
					b := seats
					t.BilledSeats = &b
				}
				continue
			}
			suppliers++
			t.TotalSeats += t.SeatsBySource[src]
		}
		t.Sources = suppliers
		t.TotalKnown = true // no supplier rows is a known zero (a SKU only the PSA bills)
		for src, n := range t.UnknownSeatRows {
			if src != "psa" && n > 0 {
				t.TotalKnown = false
			}
		}
		if suppliers > 1 {
			t.Flags = append(t.Flags, "multi_source")
		}
		dup := false
		for src, n := range t.RowsBySource {
			if src != "psa" && n > 1 {
				t.Flags = append(t.Flags, "duplicate_in_"+src)
				dup = true
			}
		}
		custom := 0
		for d := range idents[k.group].domains {
			if !strings.HasSuffix(d, ".onmicrosoft.com") {
				custom++
			}
		}
		if custom > 1 {
			t.Flags = append(t.Flags, "domain_mismatch")
		}
		t.TrialSeats = trialSeats[k]
		if t.TrialSeats != 0 {
			t.Flags = append(t.Flags, "trial")
		}
		paid := t.TotalSeats - t.TrialSeats
		switch {
		case seatsUnknown[k]:
			// A TD SYNNEX seat count is unknown, so the totals and any billing
			// verdict would be stated from part of the data (adversarial round 15).
			t.Flags = append(t.Flags, "seats_incomplete")
		case withFiles && (unsure[k] || blankSKU[k.group]):
			t.Flags = append(t.Flags, "match_ambiguous") // an unmatched row may belong here
		case t.BilledSeats != nil && *t.BilledSeats != paid:
			t.Flags = append(t.Flags, "billed_mismatch")
		case billed && t.BilledSeats == nil && paid != 0:
			// Seats paid for with no PSA row at all are the costliest mismatch
			// (adversarial round 14, P1: they carried no flag).
			t.Flags = append(t.Flags, "not_billed")
		}
		sort.Strings(t.Flags)
		if !withFiles && !dup {
			continue // ION-only mode reports duplicates only
		}
		view.Rows = append(view.Rows, *t)
	}
	sort.SliceStable(view.Rows, func(i, j int) bool {
		if len(view.Rows[i].Flags) != len(view.Rows[j].Flags) {
			return len(view.Rows[i].Flags) > len(view.Rows[j].Flags)
		}
		if view.Rows[i].Customer != view.Rows[j].Customer {
			return view.Rows[i].Customer < view.Rows[j].Customer
		}
		if view.Rows[i].SKUKey != view.Rows[j].SKUKey {
			return view.Rows[i].SKUKey < view.Rows[j].SKUKey
		}
		return view.Rows[i].group < view.Rows[j].group // two customers of one name keep one order (adversarial round 25)
	})
	return view
}

// reservedDistributor refuses a file distributor named like the local store
// or, outside --billed, like the PSA. Names are judged loosely: "TD SYNNEX" or
// "TD_Synnex" in a file would otherwise become a second supplier (adversarial
// round 14, P2).
func reservedDistributor(name string, billed bool) error {
	switch n := normalizeCompany(name); {
	case strings.HasPrefix(strings.ReplaceAll(n, " ", ""), "tdsynnex") || strings.Contains(n, "streamone"):
		return fmt.Errorf("distributor %q is reserved for the local store; rename it", name)
	case n == "psa" && !billed:
		return fmt.Errorf("distributor %q is reserved for --billed; pass the file with --billed", name)
	}
	return nil
}

// totalText is the supplier seat total, or "unknown" when no supplier row
// has a known count.
func totalText(r trueupRow) string { return seatsText(r.TotalSeats, r.TotalKnown) }

func newNovelPositionTrueupCmd(flags *rootFlags) *cobra.Command {
	var withFiles, maps []string
	var billed, dbPath string

	cmd := &cobra.Command{
		Use:   "trueup",
		Short: "Compare seats per customer and SKU across TD SYNNEX and Pax8, Sherweb or PSA export files (files are CLI-only; over MCP it lists SKUs held in more than one TD SYNNEX subscription)",
		Long: "Join TD SYNNEX positions from the local store with position files from other distributors or a PSA. Files " +
			"are CSV (header row) or a JSON array in the " + positionSchema + " column names (" + strings.Join(positionColumns, ", ") + "); " +
			"--map renames incoming columns, for example --map quantity=seats --map company=customer_name. distributor " +
			"defaults to the file name. Files must be .csv or .json exports outside hidden directories; a row with a missing or " +
			"unreadable seat count (from any source) adds no seats and flags its customer and SKU seats_incomplete, with no billing verdict. Customers match on tenant_id, then customer_domain, then exact normalized name " +
			"(punctuation and legal suffixes like Inc or Ltd dropped), and the match basis is reported as matched_on. " +
			"Rows join on sku_key (Microsoft product:SKU, for example CFQ7TTC0LDPB:0001; a full part number such as CFQ7TTC0LDPB:0001:P1Y:Y in a file is reduced to it). Rows that match no TD SYNNEX " +
			"customer, or have no sku_key, are listed as unmatched instead of guessed. --billed marks a PSA export as billed " +
			"seats and flags billed_mismatch, and not_billed where paid supplier seats have no billed row at all (TD SYNNEX trial seats are flagged trial and not counted as billable); " +
			"a customer and SKU whose TD SYNNEX seat count ION did not send is flagged seats_incomplete and gets no billing verdict, and one an ambiguous unmatched row may belong to is flagged match_ambiguous with no billing verdict. Without any files it reports SKUs a customer holds in more than one TD SYNNEX " +
			"subscription.\n\n" +
			"Use this command to compare licence positions across distributors or against PSA billing. Do NOT use this " +
			"command to export TD SYNNEX rows for another tool; use 'position export' instead. Do NOT use it for TD " +
			"SYNNEX-only margin; use 'margin' instead.",
		Example: strings.Trim(`
  td-synnex-cli position trueup --agent
  td-synnex-cli position trueup --with pax8-positions.csv --map company=customer_name --map sku=sku_key --map quantity=seats --agent
  td-synnex-cli position trueup --with pax8.csv --with sherweb.json --map quantity=seats --map company=customer_name
  td-synnex-cli position trueup --billed psa-agreements.csv --map billed_qty=seats`, "\n"),
		// mcp:write-flags drops --with and --billed from the MCP tool and rejects them there: a read-only tool
		// that reads any path an agent names could read back a credential file through unmatched rows.
		// Over MCP the tool runs the TD SYNNEX-only duplicate check; the CLI keeps both flags.
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:write-flags": "with,billed", "pp:data-source": "local", "pp:happy-args": "--map=quantity=seats"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "position trueup")
			}
			mapping := map[string]string{}
			for _, m := range maps {
				from, to, ok := strings.Cut(m, "=")
				if !ok || strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
					return usageErr(fmt.Errorf("--map must be incoming=canonical, for example quantity=seats; got %q", m))
				}
				mapping[strings.ToLower(strings.TrimSpace(from))] = strings.ToLower(strings.TrimSpace(to))
			}
			inputs := make([]positionRow, 0)
			rejected := make([]map[string]any, 0)
			read := func(path, forceDist string) error {
				if err := checkPositionFile(path); err != nil {
					return usageErr(err)
				}
				data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- operator-named --with/--billed export, vetted by checkPositionFile above (.csv/.json regular file, no hidden path, outside this CLI's own dirs, size-capped)
				if err != nil {
					return usageErr(fmt.Errorf("reading %s: %w", path, err))
				}
				rows, bad, err := parsePositionFile(path, data, mapping)
				if err != nil {
					return usageErr(err)
				}
				for _, b := range bad {
					if forceDist != "" {
						b["distributor"] = forceDist
					}
					rejected = append(rejected, b)
				}
				for _, r := range rows {
					if forceDist != "" {
						r.Distributor = forceDist
					}
					if err := reservedDistributor(r.Distributor, forceDist != ""); err != nil {
						return usageErr(fmt.Errorf("%s: %w", path, err))
					}
					inputs = append(inputs, r)
				}
				return nil
			}
			for _, f := range withFiles {
				if err := read(f, ""); err != nil {
					return err
				}
			}
			if billed != "" {
				if err := read(billed, "psa"); err != nil {
					return err
				}
			}
			book, closeDB, err := openIONBook(cmd, flags, dbPath, "subscriptions", "customers")
			if err != nil {
				return err
			}
			defer closeDB()
			if book != nil {
				inputs = append(ionPositions(book, false), inputs...)
			}
			view := trueup(inputs, len(withFiles) > 0 || billed != "", billed != "")
			view.Unmatched = append(view.Unmatched, rejected...)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printIONJSON(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Rows) == 0 && len(view.Unmatched) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing to reconcile: no duplicate or cross-source SKUs found.")
				return nil
			}
			table := make([][]string, 0, len(view.Rows))
			for _, r := range view.Rows {
				srcs := make([]string, 0, len(r.SeatsBySource))
				for s := range r.RowsBySource {
					v := strconv.FormatFloat(r.SeatsBySource[s], 'f', -1, 64)
					switch _, known := r.SeatsBySource[s]; {
					case r.UnknownSeatRows[s] > 0 && !known:
						v = "unknown"
					case r.UnknownSeatRows[s] > 0:
						v += "+unknown"
					}
					srcs = append(srcs, s+"="+v)
				}
				sort.Strings(srcs)
				table = append(table, []string{r.Customer, r.SKUKey, strings.Join(srcs, " "), totalText(r), r.MatchedOn, strings.Join(r.Flags, ",")})
			}
			if err := printTable(cmd.OutOrStdout(), []string{"CUSTOMER", "SKU KEY", "SEATS BY SOURCE", "TOTAL", "MATCHED ON", "FLAGS"}, table); err != nil {
				return err
			}
			if len(view.Unmatched) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d unmatched rows (customer not in TD SYNNEX positions, a missing sku_key, or an inactive file row); use --json to list them with the reason.\n", len(view.Unmatched))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&withFiles, "with", nil, "Position file from another distributor (CSV or JSON); repeatable")
	cmd.Flags().StringVar(&billed, "billed", "", "PSA billed-seat position file; its seats are compared, not added")
	cmd.Flags().StringArrayVar(&maps, "map", nil, "Rename an incoming column to a canonical one, incoming=canonical; repeatable")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's synced database)")
	return cmd
}
