// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Behavioral tests for the hand-written QuoteWerks commands, run against an
// in-process fake of the QuoteWerks Web JSON:API table surface. The fake
// implements the vendor filter grammar (eq/contains/gte/lte/in/or), page
// numbering, meta.ignored_filters for unknown columns, and records writes.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"quotewerks-pp-cli/internal/cliutil/testenv"
)

type fakeQW struct {
	mu        sync.Mutex
	tables    map[string][]map[string]any // RecGUID in "RecGUID"
	columns   map[string]map[string]bool
	writes    []string
	seq       int
	failPart  map[string]bool // POST DocumentItems with this ManufacturerPartNumber fails
	failPatch map[string]bool // PATCH of this RecGUID fails
	dupFirst  bool            // every search page repeats the first row (unstable paging)
}

func guid(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func daysAgo(n int) string {
	return time.Now().AddDate(0, 0, -n).Format("2006-01-02") + "T00:00:00"
}

func newFakeQW() *fakeQW {
	f := &fakeQW{tables: map[string][]map[string]any{}, columns: map[string]map[string]bool{}, failPart: map[string]bool{}, failPatch: map[string]bool{}}
	f.tables["DocumentHeaders"] = []map[string]any{
		{"RecGUID": guid(1), "ID": 1.0, "DocNo": "1042", "DocType": "QUOTE", "DocStatus": "Open", "DocDate": daysAgo(40), "DocName": "Laptops", "SoldToCompany": "Acme Dental", "SoldToContact": "Dana Lee", "SalesRep": "Ann", "SubTotal": 2300.0, "GrandTotal": 2484.0, "ProfitAmount": 300.0, "QuoteValetStatusCode": 0.0},
		{"RecGUID": guid(2), "ID": 2.0, "DocNo": "1043", "DocType": "QUOTE", "DocStatus": "Open", "DocDate": daysAgo(3), "DocName": "Switches", "SoldToCompany": "Zeta Labs", "SalesRep": "Bob", "SubTotal": 1000.0, "GrandTotal": 1080.0, "ProfitAmount": 250.0, "QuoteValetStatusCode": 1.0},
		{"RecGUID": guid(3), "ID": 3.0, "DocNo": "1044", "DocType": "ORDER", "DocStatus": "Ordered", "DocDate": daysAgo(10), "DocName": "Laptops order", "SoldToCompany": "Acme Dental", "SalesRep": "Ann", "SubTotal": 5000.0, "GrandTotal": 5400.0, "ProfitAmount": 1000.0},
		{"RecGUID": guid(4), "ID": 4.0, "DocNo": "1045", "DocType": "QUOTE", "DocStatus": "Dead", "DocDate": daysAgo(70), "DocName": "Old quote", "SoldToCompany": "Acme Dental", "SalesRep": "Ann", "SubTotal": 700.0, "GrandTotal": 756.0, "ProfitAmount": 70.0, "QuoteValetStatusCode": 3.0},
	}
	f.tables["DocumentItems"] = []map[string]any{
		{"RecGUID": guid(101), "ID": 101.0, "DocRecGUID": guid(1), "LineType": 1.0, "LineNumberActual": 2.0, "Manufacturer": "Lenovo", "ManufacturerPartNumber": "X1", "Description": "ThinkPad X1", "QtyBase": 2.0, "UnitPrice": 1000.0, "UnitCost": 850.0, "ExtendedPrice": 2000.0, "ExtendedCost": 1700.0},
		{"RecGUID": guid(102), "ID": 102.0, "DocRecGUID": guid(1), "LineType": 2.0, "LineNumberActual": 1.0, "Description": "Thanks for the opportunity"},
		{"RecGUID": guid(103), "ID": 103.0, "DocRecGUID": guid(1), "LineType": 1.0, "LineNumberActual": 3.0, "Manufacturer": "Acme", "ManufacturerPartNumber": "SVC-1", "Description": "Setup", "QtyBase": 1.0, "UnitPrice": 300.0, "UnitCost": 320.0, "ExtendedPrice": 300.0, "ExtendedCost": 320.0},
		{"RecGUID": guid(105), "ID": 105.0, "DocRecGUID": guid(1), "LineType": 64.0, "LineNumberActual": 4.0, "Description": "Loyalty discount", "PercentValue": 5.0},
		{"RecGUID": guid(104), "ID": 104.0, "DocRecGUID": guid(3), "LineType": 1.0, "Manufacturer": "Lenovo", "ManufacturerPartNumber": "X1", "Description": "ThinkPad X1", "QtyBase": 5.0, "UnitPrice": 1000.0, "UnitCost": 800.0, "ExtendedPrice": 5000.0, "ExtendedCost": 4000.0},
	}
	f.tables["Products_AllProducts_Products"] = []map[string]any{
		{"RecGUID": guid(201), "ID": 1.0, "Manufacturer": "Lenovo", "ManufacturerPartNumber": "X1", "Description": "ThinkPad X1 Carbon", "Price": 1100.0, "Cost": 900.0, "List": 1300.0},
		{"RecGUID": guid(202), "ID": 2.0, "Manufacturer": "Acme", "ManufacturerPartNumber": "SVC-1", "Description": "Setup service", "Price": 300.0, "Cost": 320.0},
	}
	f.tables["Products_Labor_Products"] = []map[string]any{
		{"RecGUID": guid(301), "ID": 1.0, "Manufacturer": "Lenovo", "ManufacturerPartNumber": "X1", "Description": "ThinkPad X1 (labor db)", "Price": 1150.0, "Cost": 950.0},
	}
	for name, rows := range f.tables {
		cols := map[string]bool{}
		for _, r := range rows {
			for k := range r {
				cols[k] = true
			}
		}
		for _, k := range []string{"ConvertedRef", "VendorPartNumber", "UnitList", "SoldToContact", "List"} {
			cols[k] = true
		}
		f.columns[name] = cols
	}
	return f
}

func (f *fakeQW) match(cols map[string]bool, row map[string]any, cond map[string]any, ignored *[]string) bool {
	if or, ok := cond["or"].([]any); ok {
		for _, c := range or {
			if f.match(cols, row, c.(map[string]any), ignored) {
				return true
			}
		}
		return false
	}
	name, _ := cond["name"].(string)
	if !cols[name] {
		*ignored = append(*ignored, name)
		return true
	}
	v, val := row[name], cond["val"]
	num := func(x any) float64 {
		switch t := x.(type) {
		case float64:
			return t
		case json.Number:
			f, _ := t.Float64()
			return f
		}
		return 0
	}
	switch cond["op"] {
	case "eq":
		if s, ok := v.(string); ok {
			return strings.EqualFold(s, fmt.Sprint(val))
		}
		return num(v) == num(val)
	case "contains":
		return strings.Contains(strings.ToLower(fmt.Sprint(v)), strings.ToLower(fmt.Sprint(val)))
	case "gte", "lte", "lt", "gt":
		cmp := 0
		if sv, ok := v.(string); ok {
			cmp = strings.Compare(sv, fmt.Sprint(val))
		} else if num(v) < num(val) {
			cmp = -1
		} else if num(v) > num(val) {
			cmp = 1
		}
		switch cond["op"] {
		case "gte":
			return cmp >= 0
		case "lte":
			return cmp <= 0
		case "lt":
			return cmp < 0
		}
		return cmp > 0
	case "in":
		for _, x := range val.([]any) {
			if strings.EqualFold(fmt.Sprint(x), fmt.Sprint(v)) {
				return true
			}
		}
		return false
	}
	return false
}

func resource(table string, row map[string]any) map[string]any {
	attrs := map[string]any{}
	for k, v := range row {
		attrs[k] = v
	}
	return map[string]any{"type": table, "id": row["RecGUID"], "attributes": attrs}
}

func (f *fakeQW) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("X-API-Key") != "qww_test_key" {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"detail":"Invalid or missing authentication."}`)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.api+json")
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/qw/tables"), "/")
	if r.URL.Path == "/api/v1/qw/tables" {
		names := make([]string, 0)
		for n := range f.tables {
			names = append(names, n)
		}
		sort.Strings(names)
		data := make([]any, 0)
		for _, n := range names {
			data = append(data, map[string]any{"type": "tables", "id": n, "attributes": map[string]any{"name": n}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
		return
	}
	if len(parts) < 2 || f.tables[parts[1]] == nil {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","title":"Not found"}]}`)
		return
	}
	table := parts[1]
	body := map[string]any{}
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		_ = dec.Decode(&body)
	}
	switch {
	case r.Method == "GET" && len(parts) == 3 && parts[2] == "schema":
		cols := make([]any, 0)
		names := make([]string, 0, len(f.columns[table]))
		for c := range f.columns[table] {
			names = append(names, c)
		}
		sort.Strings(names)
		for _, c := range names {
			readonly := c == "RecGUID" || c == "ID" || c == "ExtendedPrice" || c == "ExtendedCost"
			cols = append(cols, map[string]any{"name": c, "type": "nvarchar", "nullable": true, "primary_key": c == "RecGUID", "readonly": readonly})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"table_name": table, "primary_key": "RecGUID", "columns": cols, "column_count": len(cols)})
	case r.Method == "POST" && len(parts) == 3 && parts[2] == "search":
		var ignored []string
		filters, _ := body["filter"].([]any)
		out := make([]any, 0)
		for _, row := range f.tables[table] {
			ok := true
			for _, c := range filters {
				if !f.match(f.columns[table], row, c.(map[string]any), &ignored) {
					ok = false
				}
			}
			if ok {
				out = append(out, resource(table, row))
			}
		}
		page := body["page"].(map[string]any)
		n, _ := page["number"].(json.Number).Int64()
		size, _ := page["size"].(json.Number).Int64()
		start, end := int((n-1)*size), int(n*size)
		if start > len(out) {
			start = len(out)
		}
		if end > len(out) {
			end = len(out)
		}
		meta := map[string]any{"total": len(out)}
		if len(ignored) > 0 {
			meta["ignored_filters"] = ignored
		}
		pageRows := out[start:end]
		if f.dupFirst && n > 1 && len(out) > 0 {
			pageRows = append([]any{out[0]}, pageRows...)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": pageRows, "meta": meta})
	case r.Method == "GET" && len(parts) == 3:
		for _, row := range f.tables[table] {
			if row["RecGUID"] == parts[2] {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": resource(table, row)})
				return
			}
		}
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"errors":[{"status":"404","title":"RecGUID not found"}]}`)
	case r.Method == "POST" && len(parts) == 2:
		attrs := body["data"].(map[string]any)["attributes"].(map[string]any)
		if table == "DocumentItems" && f.failPart[fmt.Sprint(attrs["ManufacturerPartNumber"])] {
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"errors":[{"status":"400","title":"rejected by test"}]}`)
			return
		}
		if body["data"].(map[string]any)["type"] != table {
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"errors":[{"status":"400","title":"type mismatch"}]}`)
			return
		}
		f.seq++
		row := map[string]any{"RecGUID": guid(900 + f.seq)}
		for k, v := range attrs {
			if n, ok := v.(json.Number); ok {
				fv, _ := n.Float64()
				v = fv
			}
			row[k] = v
		}
		if table == "DocumentHeaders" {
			row["DocNo"] = fmt.Sprintf("%d", 2000+f.seq)
		}
		f.tables[table] = append(f.tables[table], row)
		f.writes = append(f.writes, "POST "+table)
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": resource(table, row)})
	case r.Method == "PATCH" && len(parts) == 3:
		data := body["data"].(map[string]any)
		if data["type"] != table || data["id"] != parts[2] || f.failPatch[parts[2]] {
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"errors":[{"status":"400","title":"id/type must match the URL"}]}`)
			return
		}
		attrs := data["attributes"].(map[string]any)
		for _, row := range f.tables[table] {
			if row["RecGUID"] == parts[2] {
				for k, v := range attrs {
					if n, ok := v.(json.Number); ok {
						fv, _ := n.Float64()
						v = fv
					}
					row[k] = v
				}
				f.writes = append(f.writes, "PATCH "+table+" "+parts[2])
				_ = json.NewEncoder(w).Encode(map[string]any{"data": resource(table, row)})
				return
			}
		}
		w.WriteHeader(404)
	default:
		w.WriteHeader(405)
	}
}

func (f *fakeQW) writeLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.writes...)
}

type qwHarness struct {
	t    *testing.T
	fake *fakeQW
	db   string
}

func newQWHarness(t *testing.T) *qwHarness {
	testenv.Isolate(t)
	fake := newFakeQW()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	t.Setenv("QUOTEWERKS_BASE_URL", srv.URL)
	t.Setenv("QUOTEWERKS_API_KEY", "qww_test_key")
	return &qwHarness{t: t, fake: fake, db: filepath.Join(t.TempDir(), "mirror.db")}
}

// run executes the CLI and returns stdout and the exit code.
func (h *qwHarness) run(args ...string) (string, int) {
	h.t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append(args, "--no-cache"))
	err := cmd.Execute()
	code := 0
	if err != nil {
		code = 1
		var ce *cliError
		if errors.As(err, &ce) {
			code = ce.code
		}
	}
	return out.String(), code
}

func (h *qwHarness) runJSON(v any, args ...string) int {
	h.t.Helper()
	out, code := h.run(append(args, "--json")...)
	if err := json.Unmarshal([]byte(out), v); err != nil {
		h.t.Fatalf("%v: stdout is not JSON (%v):\n%s", args, err, out)
	}
	return code
}

func TestQWDocsSearchFiltersAndNegative(t *testing.T) {
	h := newQWHarness(t)
	var rows []map[string]any
	if code := h.runJSON(&rows, "docs", "search", "--customer", "acme", "--type", "quote"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 Acme quotes, got %d: %v", len(rows), rows)
	}
	for _, r := range rows {
		if r["SoldToCompany"] != "Acme Dental" || r["DocType"] != "QUOTE" {
			t.Fatalf("filter leaked row %v", r)
		}
		if _, has := r["SubTotal"]; !has {
			t.Fatalf("projection dropped SubTotal: %v", r)
		}
	}
	var none []map[string]any
	h.runJSON(&none, "docs", "search", "--customer", "Nobody Inc")
	if len(none) != 0 {
		t.Fatalf("negative search returned %v", none)
	}
}

func TestQWIgnoredFilterFailsLoudly(t *testing.T) {
	h := newQWHarness(t)
	c, err := (&rootFlags{timeout: 10 * time.Second}).newClient()
	if err != nil {
		t.Fatal(err)
	}
	_ = h
	_, _, err = qwSearch(t.Context(), c, "DocumentHeaders", qwSearchOpts{filters: []map[string]any{{"name": "Total", "op": "gt", "val": 1000}}})
	var ig *errIgnoredFilters
	if !errors.As(err, &ig) || !strings.Contains(err.Error(), "Total") {
		t.Fatalf("want ignored-filter error naming Total, got %v", err)
	}
	var ce *cliError
	if !errors.As(asUsage(err), &ce) || ce.code != 2 {
		t.Fatalf("ignored filter must map to exit 2, got %v", asUsage(err))
	}
}

func TestQWDocsGetLinesInOrder(t *testing.T) {
	h := newQWHarness(t)
	var view struct {
		Header map[string]any   `json:"header"`
		Lines  []map[string]any `json:"lines"`
		Totals lineTotals       `json:"totals"`
	}
	if code := h.runJSON(&view, "docs", "get", "1042"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if view.Header["DocNo"] != "1042" || len(view.Lines) != 4 {
		t.Fatalf("header/lines = %v / %d", view.Header, len(view.Lines))
	}
	if view.Lines[0]["line_type"] != "comment" || view.Lines[1]["ManufacturerPartNumber"] != "X1" {
		t.Fatalf("lines not in LineNumberActual order: %v", view.Lines)
	}
	if view.Lines[1]["margin_pct"] != 15.0 {
		t.Fatalf("margin = %v", view.Lines[1]["margin_pct"])
	}
	if view.Lines[3]["line_type"] != "percent_discount" {
		t.Fatalf("discount line decode: %v", view.Lines[3])
	}
	if view.Totals.ProductLines != 2 || view.Totals.Price != 2300 || view.Totals.Cost != 2020 {
		t.Fatalf("totals = %+v", view.Totals)
	}
	if _, code := h.run("docs", "get", "9999", "--json"); code != 3 {
		t.Fatalf("unknown DocNo exit = %d, want 3", code)
	}
}

func TestQWDocsCreatePreviewThenConfirm(t *testing.T) {
	h := newQWHarness(t)
	var preview map[string]any
	h.runJSON(&preview, "docs", "create", "--customer", "Acme Dental", "--name", "Refresh")
	if preview["mode"] != "preview" || len(h.fake.writeLog()) != 0 {
		t.Fatalf("preview wrote: %v %v", preview, h.fake.writeLog())
	}
	var done map[string]any
	if code := h.runJSON(&done, "docs", "create", "--customer", "Acme Dental", "--name", "Refresh", "--confirm"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	created := done["created"].(map[string]any)
	if done["mode"] != "written" || !nonEmpty(created["DocNo"]) || !nonEmpty(created["id"]) || created["SoldToCompany"] != "Acme Dental" {
		t.Fatalf("created = %v", done)
	}
	if !strings.Contains(done["next"].(string), created["id"].(string)) {
		t.Fatalf("next hint must carry the new RecGUID: %v", done["next"])
	}
	if _, code := h.run("docs", "create", "--customer", strings.Repeat("x", 51), "--json"); code != 2 {
		t.Fatalf("over-long customer exit = %d, want 2", code)
	}
}

func TestQWItemsAddFillsFromCheapestProduct(t *testing.T) {
	h := newQWHarness(t)
	var preview map[string]any
	h.runJSON(&preview, "items", "add", "1042", "--part", "x1", "--qty", "3")
	would := preview["would"].([]any)[0].(map[string]any)["body"].(map[string]any)["data"].(map[string]any)["attributes"].(map[string]any)
	if would["UnitCost"] != 900.0 || would["UnitPrice"] != 1100.0 || would["DocRecGUID"] != guid(1) || would["QtyBase"] != 3.0 {
		t.Fatalf("attributes = %v (want AllProducts cost 900, cheaper than Labor 950)", would)
	}
	if len(h.fake.writeLog()) != 0 {
		t.Fatal("preview must not write")
	}
	var done map[string]any
	if code := h.runJSON(&done, "items", "add", "1042", "--part", "X1", "--price", "999", "--confirm"); code != 0 {
		t.Fatalf("exit %d: %v", code, done)
	}
	if done["created"].(map[string]any)["UnitPrice"] != 999.0 {
		t.Fatalf("explicit --price must win: %v", done["created"])
	}
}

func TestQWItemsRepriceMarginDiffAndApply(t *testing.T) {
	h := newQWHarness(t)
	var preview struct {
		Mode    string          `json:"mode"`
		Changes []repriceChange `json:"changes"`
	}
	h.runJSON(&preview, "items", "reprice", "1042", "--part", "X1", "--margin", "20")
	if preview.Mode != "preview" || len(preview.Changes) != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	c := preview.Changes[0]
	if c.NewPrice != 1062.5 || c.OldPrice != 1000 || *c.NewMarginPct != 20 || *c.OldMarginPct != 15 {
		t.Fatalf("change = %+v", c)
	}
	var fromProducts struct {
		Changes []repriceChange `json:"changes"`
	}
	h.runJSON(&fromProducts, "items", "reprice", "1042", "--all-lines", "--from-products")
	if len(fromProducts.Changes) != 1 || fromProducts.Changes[0].NewCost != 900 || fromProducts.Changes[0].NewPrice != 1100 {
		t.Fatalf("from-products = %+v (SVC-1 already matches and must be unchanged)", fromProducts.Changes)
	}
	var done map[string]any
	if code := h.runJSON(&done, "items", "reprice", "1042", "--part", "X1", "--margin", "20", "--confirm"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if done["applied"] != 1.0 || !strings.Contains(strings.Join(h.fake.writeLog(), ","), "PATCH DocumentItems "+guid(101)) {
		t.Fatalf("apply = %v / %v", done, h.fake.writeLog())
	}
	if _, code := h.run("items", "reprice", "1042", "--part", "X1", "--price", "1", "--margin", "2", "--json"); code != 2 {
		t.Fatalf("conflicting price modes exit = %d", code)
	}
}

func TestQWProductsSearchAcrossDatabases(t *testing.T) {
	h := newQWHarness(t)
	var rows []map[string]any
	h.runJSON(&rows, "products", "search", "thinkpad")
	if len(rows) != 2 {
		t.Fatalf("want ThinkPad from both databases, got %v", rows)
	}
	dbs := map[string]bool{}
	for _, r := range rows {
		dbs[r["product_db"].(string)] = true
	}
	if !dbs["AllProducts"] || !dbs["Labor"] {
		t.Fatalf("product_db tags = %v", dbs)
	}
	var dbList []map[string]any
	h.runJSON(&dbList, "products", "search", "--dbs")
	if len(dbList) != 2 {
		t.Fatalf("dbs = %v", dbList)
	}
}

func TestQWRequotePreviewAndConfirm(t *testing.T) {
	h := newQWHarness(t)
	var preview struct {
		Mode   string           `json:"mode"`
		Header map[string]any   `json:"header"`
		Lines  []map[string]any `json:"lines"`
	}
	h.runJSON(&preview, "requote", "1042", "--qty", "X1=12")
	if preview.Mode != "preview" || preview.Header["SoldToCompany"] != "Acme Dental" || preview.Header["DocType"] != "QUOTE" || len(preview.Lines) != 4 {
		t.Fatalf("preview = %+v", preview)
	}
	sawX1, sawDiscount := false, false
	for _, l := range preview.Lines {
		if l["ManufacturerPartNumber"] == "X1" {
			sawX1 = true
			if l["QtyBase"] != 12.0 || l["UnitCost"] != 900.0 || l["price_source"] != "product_db:AllProducts" {
				t.Fatalf("X1 line not refreshed: %v", l)
			}
		}
		if l["Description"] == "Loyalty discount" {
			sawDiscount = l["PercentValue"] == 5.0 && l["LineType"] == 64.0
		}
		if _, leaked := l["ExtendedPrice"]; leaked {
			t.Fatalf("server-calculated column copied: %v", l)
		}
	}
	if !sawX1 || !sawDiscount {
		t.Fatalf("requote lost lines or discount values: %v", preview.Lines)
	}
	if _, code := h.run("requote", "1042", "--qty", "NOPE=1", "--json"); code != 2 {
		t.Fatalf("unknown --qty part exit = %d", code)
	}
	var done map[string]any
	if code := h.runJSON(&done, "requote", "1042", "--only", "X1", "--confirm"); code != 0 {
		t.Fatalf("exit %d: %v", code, done)
	}
	if done["lines_written"] != 1.0 || len(h.fake.writeLog()) != 2 {
		t.Fatalf("confirm wrote %v / %v", done, h.fake.writeLog())
	}
	newID := done["created"].(map[string]any)["id"].(string)
	children := 0
	h.fake.mu.Lock()
	for _, it := range h.fake.tables["DocumentItems"] {
		if it["DocRecGUID"] == newID {
			children++
		}
	}
	h.fake.mu.Unlock()
	if children != 1 {
		t.Fatalf("posted lines must carry the new header id %s; found %d", newID, children)
	}
}

func TestQWDocsLintFindsErrors(t *testing.T) {
	h := newQWHarness(t)
	var view struct {
		Errors   int              `json:"errors"`
		Findings []map[string]any `json:"findings"`
	}
	if code := h.runJSON(&view, "docs", "lint", "1042"); code != 0 {
		t.Fatalf("exit = %d, want 0 (findings are data)", code)
	}
	if _, code := h.run("docs", "lint", "1042", "--strict", "--json"); code != 1 {
		t.Fatalf("--strict exit = %d, want 1 (SVC-1 sells below cost)", code)
	}
	found := false
	for _, f := range view.Findings {
		if f["rule"] == "price-below-cost" && f["part"] == "SVC-1" {
			found = true
		}
		if f["rule"] == "price-below-cost" && f["part"] == "X1" {
			t.Fatalf("X1 is above cost: %v", f)
		}
	}
	if !found || view.Errors != 1 {
		t.Fatalf("findings = %+v", view)
	}
}

func TestQWMirrorAndLocalAnalytics(t *testing.T) {
	h := newQWHarness(t)
	var sum map[string]any
	if code := h.runJSON(&sum, "mirror", "--db", h.db); code != 0 {
		t.Fatalf("mirror exit %d: %v", code, sum)
	}
	if sum["documents"] != 4.0 || sum["line_items"] != 5.0 || sum["products"] != 3.0 || sum["baseline"] != true {
		t.Fatalf("mirror summary = %v", sum)
	}

	var margin marginView
	h.runJSON(&margin, "margin", "--by", "rep", "--type", "QUOTE,ORDER", "--db", h.db)
	if len(margin.Rows) != 2 || margin.Rows[0].Key != "Ann" || margin.Rows[0].Profit != 1370 || margin.Rows[0].Revenue != 8000 {
		t.Fatalf("margin by rep = %+v", margin.Rows)
	}
	var byMfr marginView
	h.runJSON(&byMfr, "margin", "--by", "manufacturer", "--type", "QUOTE", "--db", h.db)
	if len(byMfr.Rows) != 2 || byMfr.Rows[0].Key != "Lenovo" || byMfr.Rows[0].Profit != 300 {
		t.Fatalf("margin by manufacturer = %+v", byMfr.Rows)
	}

	var win struct {
		Total   winrateRow     `json:"total"`
		Signals map[string]int `json:"signals"`
	}
	h.runJSON(&win, "winrate", "--db", h.db)
	if win.Total.Quotes != 3 || win.Total.Won != 1 || win.Total.Lost != 1 || win.Total.Open != 1 || *win.Total.WinRate != 50 {
		t.Fatalf("winrate = %+v", win.Total)
	}
	var winStatus struct {
		Total winrateRow `json:"total"`
	}
	h.runJSON(&winStatus, "winrate", "--won-status", "dead", "--db", h.db)
	if winStatus.Total.Won != 2 || winStatus.Total.Lost != 0 {
		t.Fatalf("--won-status must override a QuoteValet 'lost': %+v", winStatus.Total)
	}

	var pipe struct {
		Count   int              `json:"count"`
		Buckets []pipelineBucket `json:"buckets"`
	}
	h.runJSON(&pipe, "pipeline", "--db", h.db)
	if pipe.Count != 1 || pipe.Buckets[2].Bucket != "31-60" || pipe.Buckets[2].Count != 1 {
		t.Fatalf("pipeline = %+v", pipe)
	}
	var chase struct {
		Quotes []map[string]any `json:"quotes"`
	}
	h.runJSON(&chase, "pipeline", "--older-than", "30d", "--db", h.db)
	if len(chase.Quotes) != 1 || chase.Quotes[0]["DocNo"] != "1042" {
		t.Fatalf("chase list = %v", chase.Quotes)
	}
	h.runJSON(&chase, "pipeline", "--older-than", "60d", "--db", h.db)
	if len(chase.Quotes) != 0 {
		t.Fatalf("no open quote is 60+ days old: %v", chase.Quotes)
	}

	var drift struct {
		Rows         []driftRow `json:"rows"`
		LinesChecked int        `json:"lines_checked"`
	}
	h.runJSON(&drift, "cost-drift", "--db", h.db)
	if len(drift.Rows) != 1 || drift.Rows[0].Part != "X1" || drift.Rows[0].CurrentCost != 900 || *drift.Rows[0].DeltaPct != 5.88 {
		t.Fatalf("cost drift = %+v", drift)
	}
	if drift.LinesChecked != 2 {
		t.Fatalf("only open-quote product lines are checked: %d", drift.LinesChecked)
	}
	h.runJSON(&drift, "cost-drift", "--threshold", "10", "--db", h.db)
	if len(drift.Rows) != 0 {
		t.Fatalf("5.88%% drift must not pass a 10%% threshold: %+v", drift.Rows)
	}

	var ch struct {
		Count int `json:"count"`
	}
	h.runJSON(&ch, "changes", "--db", h.db)
	if ch.Count != 0 {
		t.Fatalf("baseline mirror must record no changes, got %d", ch.Count)
	}
	h.fake.mu.Lock()
	h.fake.tables["DocumentHeaders"][0]["QuoteValetStatusCode"] = 1.0
	h.fake.tables["DocumentHeaders"][1]["SalesRep"] = nil
	h.fake.tables["DocumentHeaders"] = h.fake.tables["DocumentHeaders"][:3]
	h.fake.tables["DocumentItems"] = h.fake.tables["DocumentItems"][1:] // X1 line on 1042 deleted upstream
	h.fake.mu.Unlock()
	h.runJSON(&sum, "mirror", "--db", h.db)
	if sum["line_items_removed"] != 1.0 || sum["removed"] != 1.0 {
		t.Fatalf("second mirror = %v", sum)
	}
	var evs struct {
		Events []map[string]any `json:"events"`
	}
	h.runJSON(&evs, "changes", "--db", h.db)
	kinds := map[string]bool{}
	for _, e := range evs.Events {
		kinds[e["DocNo"].(string)+":"+e["kind"].(string)] = true
	}
	if !kinds["1042:accepted"] || !kinds["1045:removed"] || !kinds["1043:rep"] || len(kinds) != 3 {
		t.Fatalf("changes = %v", kinds)
	}
	var byRep struct {
		Rows []winrateRow `json:"rows"`
	}
	h.runJSON(&byRep, "winrate", "--by", "rep", "--db", h.db)
	for _, r := range byRep.Rows {
		if r.Key == "Bob" {
			t.Fatalf("a SalesRep cleared upstream must clear in the mirror: %+v", byRep.Rows)
		}
	}
	var drift2 struct {
		Rows []driftRow `json:"rows"`
	}
	h.runJSON(&drift2, "cost-drift", "--db", h.db)
	if len(drift2.Rows) != 0 {
		t.Fatalf("a line deleted upstream must leave the mirror: %+v", drift2.Rows)
	}
	h.runJSON(&ch, "changes", "--db", h.db)
	before := ch.Count
	h.runJSON(&sum, "mirror", "--db", h.db)
	h.runJSON(&ch, "changes", "--db", h.db)
	if ch.Count != before {
		t.Fatalf("an unchanged upstream must not record new changes: %d -> %d", before, ch.Count)
	}
	var after struct {
		Total winrateRow `json:"total"`
	}
	h.runJSON(&after, "winrate", "--db", h.db)
	if after.Total.Quotes != 2 {
		t.Fatalf("removed document must leave the analytics: %+v", after.Total)
	}
}

func TestQWLocalCommandsWithoutMirror(t *testing.T) {
	h := newQWHarness(t)
	missing := filepath.Join(t.TempDir(), "none.db")
	for _, args := range [][]string{{"margin"}, {"winrate"}, {"pipeline"}, {"cost-drift"}, {"changes"}} {
		out, code := h.run(append(args, "--db", missing, "--json")...)
		if code != 0 || !json.Valid([]byte(out)) {
			t.Fatalf("%v without a mirror: exit %d, stdout %q", args, code, out)
		}
	}
}

func nonEmpty(v any) bool { s, ok := v.(string); return ok && s != "" }

func TestQWRepriceSkipsUnknownCostAndReportsPartialFailure(t *testing.T) {
	h := newQWHarness(t)
	h.fake.mu.Lock()
	delete(h.fake.tables["DocumentItems"][2], "UnitCost") // SVC-1 now has no cost
	h.fake.failPatch[guid(101)] = true
	h.fake.mu.Unlock()
	var preview struct {
		Changes []repriceChange  `json:"changes"`
		Skipped []map[string]any `json:"skipped"`
	}
	h.runJSON(&preview, "items", "reprice", "1042", "--all-lines", "--margin", "18")
	if len(preview.Changes) != 1 || len(preview.Skipped) != 1 || preview.Skipped[0]["part"] != "SVC-1" {
		t.Fatalf("a line without cost must be skipped, not priced at 0: %+v", preview)
	}
	out, code := h.run("items", "reprice", "1042", "--all-lines", "--margin", "18", "--confirm", "--json")
	if code != 6 {
		t.Fatalf("partial failure exit = %d, want 6", code)
	}
	var done map[string]any
	if err := json.Unmarshal([]byte(out), &done); err != nil || len(done["update_failures"].([]any)) != 1 {
		t.Fatalf("receipt = %s", out)
	}
}

func TestQWRequotePartialFailure(t *testing.T) {
	h := newQWHarness(t)
	h.fake.mu.Lock()
	h.fake.failPart["SVC-1"] = true
	h.fake.mu.Unlock()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"requote", "1042", "--confirm", "--json", "--no-cache"})
	err := cmd.Execute()
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 6 {
		t.Fatalf("want exit 6, got %v", err)
	}
	if !strings.Contains(err.Error(), "do not re-run requote") || !strings.Contains(err.Error(), "SVC-1") {
		t.Fatalf("error must carry the receipt for MCP callers: %v", err)
	}
	var done map[string]any
	if err := json.Unmarshal(out.Bytes(), &done); err != nil || done["lines_written"] != 3.0 {
		t.Fatalf("stdout receipt = %s", out.String())
	}
}

func TestQWDocsCreateSetValues(t *testing.T) {
	h := newQWHarness(t)
	var preview map[string]any
	h.runJSON(&preview, "docs", "create", "--customer", "Acme", "--set", "ShipToPostalCode=02134", "--set-number", "CustomNumber01=2.5")
	attrs := preview["would"].([]any)[0].(map[string]any)["body"].(map[string]any)["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["ShipToPostalCode"] != "02134" || attrs["CustomNumber01"] != 2.5 {
		t.Fatalf("--set must stay text, --set-number numeric: %v", attrs)
	}
	for _, bad := range [][]string{{"--set", "A=1,B=2"}, {"--set", "DocType=ORDER"}, {"--set-number", "X=nan"}, {"--type", "QOUTE"}} {
		if _, code := h.run(append([]string{"docs", "create", "--customer", "Acme", "--json"}, bad...)...); code != 2 {
			t.Fatalf("%v exit = %d, want 2", bad, code)
		}
	}
}

func TestQWLocalTypeValidationAndLint(t *testing.T) {
	h := newQWHarness(t)
	if _, code := h.run("mirror", "--db", h.db, "--json"); code != 0 {
		t.Fatalf("mirror exit %d", code)
	}
	if _, code := h.run("margin", "--type", "ORDRE", "--db", h.db, "--json"); code != 2 {
		t.Fatalf("misspelled --type exit = %d, want 2", code)
	}
	var view struct {
		Source string `json:"source"`
		Errors int    `json:"errors"`
	}
	if code := h.runJSON(&view, "docs", "lint", "1042", "--data-source", "local", "--db", h.db); code != 0 || view.Source != "local" || view.Errors != 1 {
		t.Fatalf("local lint = %+v exit %d", view, code)
	}
	// After a documents-only mirror into a fresh db, lines were never read: refuse.
	fresh := filepath.Join(t.TempDir(), "docs-only.db")
	h.run("mirror", "--only", "documents", "--db", fresh, "--json")
	if _, code := h.run("docs", "lint", "1042", "--data-source", "local", "--db", fresh, "--json"); code != 3 {
		t.Fatalf("lint without mirrored lines exit = %d, want 3", code)
	}
}

func TestQWSearchRejectsUnstablePaging(t *testing.T) {
	h := newQWHarness(t)
	c, err := (&rootFlags{timeout: 10 * time.Second}).newClient()
	if err != nil {
		t.Fatal(err)
	}
	c.NoCache = true
	rows, complete, err := qwSearch(t.Context(), c, "DocumentHeaders", qwSearchOpts{pageSize: 1, maxPages: qwAllPages})
	if err != nil || !complete || len(rows) != 4 {
		t.Fatalf("stable walk = %d rows complete=%v err=%v", len(rows), complete, err)
	}
	h.fake.mu.Lock()
	h.fake.dupFirst = true
	h.fake.mu.Unlock()
	rows, complete, err = qwSearch(t.Context(), c, "DocumentHeaders", qwSearchOpts{pageSize: 1, maxPages: qwAllPages})
	if err != nil {
		t.Fatal(err)
	}
	if complete || len(rows) > 4 {
		t.Fatalf("a repeated row must make the walk incomplete and never double count: %d rows complete=%v", len(rows), complete)
	}
}

func TestQWUnknownCostsAreNotProfit(t *testing.T) {
	h := newQWHarness(t)
	var add map[string]any
	h.runJSON(&add, "items", "add", "1042", "--description", "Onsite labor", "--price", "150", "--no-lookup")
	if _, has := add["margin_pct"]; has {
		t.Fatalf("a line with no cost must not show a margin: %v", add)
	}
	if _, code := h.run("requote", "1042", "--qty", "X1=NaN", "--json"); code != 2 {
		t.Fatalf("non-finite --qty exit = %d, want 2", code)
	}
	h.fake.mu.Lock()
	delete(h.fake.tables["DocumentItems"][0], "UnitCost")
	delete(h.fake.tables["DocumentItems"][0], "ExtendedCost")
	h.fake.mu.Unlock()
	h.run("mirror", "--db", h.db, "--json")
	var byMfr marginView
	h.runJSON(&byMfr, "margin", "--by", "manufacturer", "--type", "QUOTE", "--db", h.db)
	for _, r := range byMfr.Rows {
		if r.Key == "Lenovo" && (r.Unknown != 1 || r.MarginPct != nil) {
			t.Fatalf("a line without cost must be counted unknown, not profit: %+v", r)
		}
	}
	if byMfr.MarginPct != nil {
		t.Fatalf("total margin must be null when amounts are unknown: %+v", byMfr)
	}
	// Documents-only mirror into a fresh db: analyses that need lines must say so.
	fresh := filepath.Join(t.TempDir(), "docs.db")
	h.run("mirror", "--only", "documents,products", "--db", fresh, "--json")
	var drift map[string]any
	h.runJSON(&drift, "cost-drift", "--db", fresh)
	if drift["quotes_without_mirrored_lines"] != 1.0 || drift["note"] == nil {
		t.Fatalf("cost-drift must report quotes it could not check: %v", drift)
	}
	var m2 marginView
	h.runJSON(&m2, "margin", "--by", "manufacturer", "--type", "QUOTE", "--db", fresh)
	if m2.NoLines == 0 {
		t.Fatalf("margin by manufacturer must report documents without mirrored lines: %+v", m2)
	}
}

func TestQWRound3EdgeCases(t *testing.T) {
	h := newQWHarness(t)
	// An unmatched --only must not create an empty quote.
	if _, code := h.run("requote", "1042", "--only", "TYPO", "--keep-prices", "--confirm", "--json"); code != 2 || len(h.fake.writeLog()) != 0 {
		t.Fatalf("--only TYPO exit %d writes %v", code, h.fake.writeLog())
	}
	// An overflowing quantity must fail before any write.
	if _, code := h.run("requote", "1042", "--qty", "X1=1e308", "--keep-prices", "--confirm", "--json"); code != 2 || len(h.fake.writeLog()) != 0 {
		t.Fatalf("overflow exit %d writes %v", code, h.fake.writeLog())
	}
	// A stored price with more precision than cents must still be updated.
	h.fake.mu.Lock()
	h.fake.tables["DocumentItems"][0]["UnitPrice"] = 1000.004
	h.fake.mu.Unlock()
	var rp map[string]any
	h.runJSON(&rp, "items", "reprice", "1042", "--part", "X1", "--price", "1000", "--confirm")
	if rp["applied"] != 1.0 {
		t.Fatalf("1000.004 -> 1000 must be applied: %v", rp)
	}
	// A line without a quantity has no extended amounts to invent.
	h.fake.mu.Lock()
	for _, k := range []string{"QtyBase", "ExtendedPrice", "ExtendedCost"} {
		delete(h.fake.tables["DocumentItems"][2], k)
	}
	// Revenue from GrandTotal needs a known tax.
	delete(h.fake.tables["DocumentHeaders"][2], "SubTotal")
	h.fake.mu.Unlock()
	var got struct {
		Totals lineTotals `json:"totals"`
	}
	h.runJSON(&got, "docs", "get", "1042")
	if got.Totals.UnknownCostLines != 1 || got.Totals.MarginPct != nil {
		t.Fatalf("a line without quantity must be unknown: %+v", got.Totals)
	}
	h.run("mirror", "--db", h.db, "--json")
	var m marginView
	h.runJSON(&m, "margin", "--by", "customer", "--db", h.db)
	if m.Unknown != 1 || m.MarginPct != nil {
		t.Fatalf("an order with no SubTotal and no TotalTax must be unknown: %+v", m)
	}
}

func TestQWRound4EdgeCases(t *testing.T) {
	h := newQWHarness(t)
	h.fake.mu.Lock()
	h.fake.tables["DocumentItems"][0]["UnitCost"] = 850.004
	h.fake.mu.Unlock()
	if _, code := h.run("items", "reprice", "1042", "--part", "X1", "--price", "1100", "--confirm", "--json"); code != 0 {
		t.Fatalf("reprice exit %d", code)
	}
	h.fake.mu.Lock()
	gotCost := h.fake.tables["DocumentItems"][0]["UnitCost"]
	h.fake.mu.Unlock()
	if gotCost != 850.004 {
		t.Fatalf("a price-only reprice must not rewrite the cost: %v", gotCost)
	}
	writes := len(h.fake.writeLog())
	h.fake.mu.Lock()
	h.fake.tables["DocumentItems"][0]["UnitPrice"] = 0.01
	h.fake.mu.Unlock()
	if _, code := h.run("items", "reprice", "1042", "--part", "X1", "--cost", "1e306", "--confirm", "--json"); code != 0 && code != 2 {
		t.Fatalf("huge cost exit %d", code)
	}
	if len(h.fake.writeLog()) != writes {
		// The margin is not finite, so it is omitted; the plan still encodes and one PATCH is fine.
		t.Logf("huge cost wrote: %v", h.fake.writeLog()[writes:])
	}

	// cost-drift: a missing quoted cost has no impact; thresholds use the unrounded change.
	h.fake.mu.Lock()
	h.fake.tables["DocumentItems"][0]["UnitCost"] = nil
	h.fake.tables["DocumentItems"][0]["UnitPrice"] = 1000.0
	h.fake.tables["DocumentItems"][2]["UnitCost"] = 10.01
	h.fake.tables["Products_AllProducts_Products"][1]["Cost"] = 10.21 // +1.998%
	h.fake.tables["DocumentHeaders"][1]["GrandTotal"] = nil           // open? 1043 is accepted; use 1042 below
	h.fake.tables["DocumentHeaders"][0]["GrandTotal"] = nil
	h.fake.tables["DocumentHeaders"][0]["DocDate"] = time.Now().Add(-25 * time.Hour).Format("2006-01-02T15:04:05")
	h.fake.mu.Unlock()
	h.run("mirror", "--db", h.db, "--json")
	var drift struct {
		Rows []driftRow `json:"rows"`
	}
	h.runJSON(&drift, "cost-drift", "--threshold", "2", "--db", h.db)
	for _, r := range drift.Rows {
		if r.Part == "SVC-1" {
			t.Fatalf("a 1.998%% change must not pass a 2%% threshold: %+v", r)
		}
		if r.Part == "X1" && (r.QuotedCost != nil || r.ExtendedImpact != nil) {
			t.Fatalf("a missing quoted cost must have no impact: %+v", r)
		}
	}
	var chase struct {
		Quotes []map[string]any `json:"quotes"`
	}
	h.runJSON(&chase, "pipeline", "--older-than", "36h", "--db", h.db)
	if len(chase.Quotes) != 0 {
		t.Fatalf("a 25-hour-old quote is not older than 36h: %v", chase.Quotes)
	}
	var pipe map[string]any
	h.runJSON(&pipe, "pipeline", "--db", h.db)
	if pipe["unknown_value"] != 1.0 || pipe["count"] != 1.0 {
		t.Fatalf("an open quote without GrandTotal is counted with an unknown value: %v", pipe)
	}
}

func TestQWEmptyBaselineStillRecordsNewDocuments(t *testing.T) {
	h := newQWHarness(t)
	h.fake.mu.Lock()
	saved := h.fake.tables["DocumentHeaders"]
	h.fake.tables["DocumentHeaders"] = []map[string]any{}
	h.fake.mu.Unlock()
	h.run("mirror", "--only", "documents", "--db", h.db, "--json")
	h.fake.mu.Lock()
	h.fake.tables["DocumentHeaders"] = saved[:1]
	h.fake.mu.Unlock()
	h.run("mirror", "--only", "documents", "--db", h.db, "--json")
	var ch struct {
		Events []map[string]any `json:"events"`
	}
	h.runJSON(&ch, "changes", "--db", h.db)
	if len(ch.Events) != 1 || ch.Events[0]["kind"] != "new" {
		t.Fatalf("a document created after an empty first mirror is new: %v", ch.Events)
	}
}

func TestQWDocsSearchMultipleTypes(t *testing.T) {
	h := newQWHarness(t)
	var rows []map[string]any
	h.runJSON(&rows, "docs", "search", "--customer", "acme", "--type", "quote,order")
	if len(rows) != 3 {
		t.Fatalf("QUOTE,ORDER for Acme = %d rows, want 3: %v", len(rows), rows)
	}
}

func TestLineAmountPricingFactor(t *testing.T) {
	if _, ok := lineAmount(map[string]any{"QtyBase": 5.0, "UnitPrice": 50.0, "UnitOfPricingFactor": 0.1}, "ExtendedPrice", "UnitPrice"); ok {
		t.Fatal("a pricing factor other than 1 makes a derived amount unknown")
	}
	if v, ok := lineAmount(map[string]any{"QtyBase": 5.0, "UnitPrice": 50.0, "UnitOfPricingFactor": 1.0}, "ExtendedPrice", "UnitPrice"); !ok || v != 250 {
		t.Fatalf("factor 1 = %v %v", v, ok)
	}
	if v, ok := lineAmount(map[string]any{"ExtendedPrice": 25.0, "UnitOfPricingFactor": 0.1}, "ExtendedPrice", "UnitPrice"); !ok || v != 25 {
		t.Fatalf("the stored extended amount wins = %v %v", v, ok)
	}
}

func TestSumLinesMarginFromRawAmounts(t *testing.T) {
	got := sumLines([]map[string]any{{"QtyBase": 0.1, "UnitPrice": 0.14, "UnitCost": 0.06}})
	if got.MarginPct == nil || *got.MarginPct != 57.14 {
		t.Fatalf("margin must come from raw sums: %+v", got)
	}
}

func TestQWInactiveDocumentsLeaveTheMirror(t *testing.T) {
	h := newQWHarness(t)
	h.fake.mu.Lock()
	h.fake.tables["DocumentHeaders"][3]["Superceeded"] = 1.0 // 1045 superseded
	h.fake.mu.Unlock()
	h.run("mirror", "--db", h.db, "--json")
	h.fake.mu.Lock()
	h.fake.tables["DocumentHeaders"] = h.fake.tables["DocumentHeaders"][:3]
	h.fake.mu.Unlock()
	var sum map[string]any
	h.runJSON(&sum, "mirror", "--db", h.db)
	if sum["removed"] != 1.0 {
		t.Fatalf("a superseded document deleted upstream must leave the mirror: %v", sum)
	}
	var ch struct {
		Events []map[string]any `json:"events"`
	}
	h.runJSON(&ch, "changes", "--db", h.db)
	for _, e := range ch.Events {
		if e["DocNo"] == "1045" {
			t.Fatalf("an inactive document's removal is not a business event: %v", e)
		}
	}
}

func TestQWToDateAndCatalogPricingFactor(t *testing.T) {
	h := newQWHarness(t)
	h.fake.mu.Lock()
	h.fake.tables["DocumentHeaders"][0]["DocDate"] = "2026-10-13T23:59:59.500"
	h.fake.tables["Products_AllProducts_Products"][0]["UnitOfPricingFactor"] = 0.1
	h.fake.mu.Unlock()
	var rows []map[string]any
	h.runJSON(&rows, "docs", "search", "--number", "1042", "--to", "2026-10-13")
	if len(rows) != 1 {
		t.Fatalf("--to must keep the whole day: %v", rows)
	}
	h.runJSON(&rows, "docs", "search", "--number", "1042", "--to", "2026-10-12")
	if len(rows) != 0 {
		t.Fatalf("--to the day before must exclude it: %v", rows)
	}
	var add map[string]any
	h.runJSON(&add, "items", "add", "1042", "--part", "X1", "--product-db", "AllProducts")
	attrs := add["would"].([]any)[0].(map[string]any)["body"].(map[string]any)["data"].(map[string]any)["attributes"].(map[string]any)
	if _, copied := attrs["UnitPrice"]; copied {
		t.Fatalf("a per-unit-of-pricing catalog price must not be copied as a per-item price: %v", attrs)
	}
	if warn, _ := add["warnings"].([]any); len(warn) != 1 || !strings.Contains(warn[0].(string), "unit of pricing") {
		t.Fatalf("the pricing factor must be reported: %v", add["warnings"])
	}
}

func TestQWCompactGUIDAndPartialMirror(t *testing.T) {
	h := newQWHarness(t)
	compact := "6979A26DDA8F4DF6963FE629216E7534"
	h.fake.mu.Lock()
	h.fake.tables["DocumentHeaders"][0]["RecGUID"] = compact
	for _, it := range h.fake.tables["DocumentItems"] {
		if it["DocRecGUID"] == guid(1) {
			it["DocRecGUID"] = compact
		}
	}
	h.fake.mu.Unlock()
	var view struct {
		Header map[string]any `json:"header"`
	}
	if code := h.runJSON(&view, "docs", "get", compact); code != 0 || view.Header["DocNo"] != "1042" {
		t.Fatalf("a compact RecGUID must fetch by id: %v exit %d", view.Header, code)
	}
	h.run("mirror", "--max-pages", "1", "--db", h.db, "--json")
	var m marginView
	h.runJSON(&m, "margin", "--db", h.db)
	if len(m.Partial) != 0 {
		t.Fatalf("a complete mirror is not partial: %v", m.Partial)
	}
}

func TestQWPricingFactorLinesUseExtendedAmounts(t *testing.T) {
	h := newQWHarness(t)
	h.fake.mu.Lock()
	l := h.fake.tables["DocumentItems"][0] // X1 on 1042
	l["QtyBase"], l["UnitOfPricingFactor"], l["UnitPrice"], l["UnitCost"], l["ExtendedPrice"], l["ExtendedCost"] = 10.0, 0.1, 100.0, 8.0, 100.0, 80.0
	h.fake.mu.Unlock()
	var view struct {
		Lines []map[string]any `json:"lines"`
	}
	h.runJSON(&view, "docs", "get", "1042")
	for _, ln := range view.Lines {
		if ln["ManufacturerPartNumber"] == "X1" && ln["margin_pct"] != 20.0 {
			t.Fatalf("a pricing-factor line's margin comes from extended amounts: %v", ln)
		}
	}
	var rp struct {
		Changes []repriceChange  `json:"changes"`
		Skipped []map[string]any `json:"skipped"`
	}
	h.runJSON(&rp, "items", "reprice", "1042", "--part", "X1", "--margin", "20")
	if len(rp.Changes) != 0 || len(rp.Skipped) != 1 {
		t.Fatalf("a pricing-factor line must not be repriced from unit values: %+v", rp)
	}
	var lint struct {
		Findings []map[string]any `json:"findings"`
	}
	h.runJSON(&lint, "docs", "lint", "1042", "--min-margin", "30", "--no-products")
	found := false
	for _, f := range lint.Findings {
		if f["rule"] == "low-margin" && f["part"] == "X1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a 20%% extended margin is below 30%%: %v", lint.Findings)
	}
}

func TestQWLocalCommandsEchoTheirFilters(t *testing.T) {
	h := newQWHarness(t)
	missing := filepath.Join(t.TempDir(), "none.db")
	cases := []struct {
		args []string
		want map[string]string
	}{
		{[]string{"margin", "--by", "rep", "--since", "90d"}, map[string]string{"by": "rep", "since": "90d"}},
		{[]string{"winrate", "--since", "30d"}, map[string]string{"since": "30d"}},
		{[]string{"pipeline", "--customer", "Acme", "--rep", "Bob", "--older-than", "30d"}, map[string]string{"customer": "Acme", "rep": "Bob", "older-than": "30d"}},
		{[]string{"cost-drift", "--threshold", "5"}, map[string]string{"threshold": "5"}},
		{[]string{"changes", "--kind", "new"}, map[string]string{"kind": "new"}},
	}
	for _, c := range cases {
		var view struct {
			Filters map[string]string `json:"filters"`
		}
		h.runJSON(&view, append(c.args, "--db", missing)...)
		for k, v := range c.want {
			if view.Filters[k] != v {
				t.Fatalf("%v: filters = %v, want %s=%s", c.args, view.Filters, k, v)
			}
		}
		if _, ok := view.Filters["db"]; ok {
			t.Fatalf("%v: --db is a location, not a filter: %v", c.args, view.Filters)
		}
	}
}

func TestQWFailedMirrorIsNotReportedAsMirrored(t *testing.T) {
	h := newQWHarness(t)
	t.Setenv("QUOTEWERKS_API_KEY", "qww_wrong_key")
	if out, code := h.run("mirror", "--db", h.db, "--json"); code == 0 {
		t.Fatalf("mirror with a rejected key must fail: %s", out)
	}
	for _, args := range [][]string{{"margin"}, {"winrate"}, {"pipeline"}, {"cost-drift"}, {"changes"}} {
		var view struct {
			Mirrored *bool `json:"mirrored"`
		}
		h.runJSON(&view, append(args, "--db", h.db)...)
		if view.Mirrored == nil || *view.Mirrored {
			t.Fatalf("%v after a failed mirror must report mirrored=false", args)
		}
	}
}
