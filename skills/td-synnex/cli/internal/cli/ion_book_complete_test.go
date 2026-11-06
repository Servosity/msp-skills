// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json: ion-book-refuses-unproven-store): the licence
// book answers only from a store whose every resource it reads was walked to a
// proven end of pagination.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"td-synnex-pp-cli/internal/cliutil/testenv"
	"td-synnex-pp-cli/internal/store"
)

// ionPagedServer serves the documented list shapes (spec.yaml listSubscriptions
// -> SubscriptionList {items, paginationResponse.totalSize}, offset pagination of
// 100; listCustomers -> CustomerList {customers, nextPageToken}; listOrders ->
// OrderList {orders, nextPageToken}) for one customer holding nSubs monthly
// subscriptions of one seat at price 10 and cost 8 USD.
func ionPagedServer(t *testing.T, nSubs int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/accounts/10505/subscriptions":
			offset, _ := strconv.Atoi(r.URL.Query().Get("pagination.offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("pagination.limit"))
			if limit <= 0 {
				limit = 100
			}
			items := make([]map[string]any, 0, limit)
			for i := offset; i < nSubs && i < offset+limit; i++ {
				items = append(items, map[string]any{
					"id": strconv.Itoa(5000 + i), "customerId": "108536", "subscriptionId": fmt.Sprintf("vendor-%d", i),
					"subscriptionName": "Microsoft 365 Business Basic", "subscriptionStatus": "ACTIVE",
					"subscriptionTotalLicenses": "1", "subscriptionBillingCycle": "monthly",
					"price": 10, "cost": 8, "currency": "USD",
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "paginationResponse": map[string]any{"totalSize": nSubs}})
		case "/accounts/10505/customers":
			_, _ = w.Write([]byte(`{"customers":[{"name":"accounts/10505/customers/108536","customerOrganization":"Contoso","customerStatus":"ACTIVE"}],"nextPageToken":""}`))
		case "/accounts/10505/orders":
			_, _ = w.Write([]byte(`{"orders":[],"nextPageToken":""}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func runIONCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func showTotals(t *testing.T, out string) customerShowTotal {
	t.Helper()
	start := strings.Index(out, "{")
	if start < 0 {
		t.Fatalf("no JSON in output: %s", out)
	}
	var v customerShowView
	if err := json.NewDecoder(strings.NewReader(out[start:])).Decode(&v); err != nil {
		t.Fatalf("decoding customers show: %v\n%s", err, out)
	}
	if len(v.Totals) != 1 {
		t.Fatalf("want one currency total, got %+v", v.Totals)
	}
	return v.Totals[0]
}

// Codex round 1 (reprint 4.35.1), P1 incomplete read reported as complete: an
// auto-refresh fetches one page per resource, and the book then reported 100
// of 101 subscriptions with seats, price and cost marked complete. Every book
// command must refuse a store whose read resources were not walked to a proven
// end (here the cold-store refresh, reproduced with sync --latest-only), and
// answer once a full sync completes them.
func TestBookRefusesStoreNotProvenComplete(t *testing.T) {
	testenv.Isolate(t)
	srv := ionPagedServer(t, 101)
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "10505")
	t.Setenv("TD_SYNNEX_NO_AUTO_REFRESH", "1")
	db := filepath.Join(t.TempDir(), "book.db")

	if out, err := runIONCLI(t, "sync", "--resources", "subscriptions,customers,orders", "--latest-only", "--db", db, "--json"); err != nil {
		t.Fatalf("one-page sync: %v\n%s", err, out)
	}
	book := [][]string{
		{"customers", "show", "108536"},
		{"margin", "--by", "customer"},
		{"margin", "--by", "customer", "--months", "3"},
		{"subscriptions", "renewals"},
		{"subscriptions", "changes"},
		{"orders", "stuck"},
		{"position", "export"},
		{"position", "trueup"},
	}
	for _, args := range book {
		out, err := runIONCLI(t, append(args, "--db", db, "--json")...)
		if err == nil {
			t.Errorf("%s answered from a store holding one page of 101 subscriptions:\n%s", strings.Join(args, " "), out)
			continue
		}
		if !strings.Contains(err.Error(), "subscriptions") || !strings.Contains(err.Error(), "sync") {
			t.Errorf("%s: the refusal must name the unproven resource and the sync that completes it: %v", strings.Join(args, " "), err)
		}
	}

	// A store that was never synced is not a proven-empty book either.
	missing := filepath.Join(t.TempDir(), "never-synced.db")
	if out, err := runIONCLI(t, "orders", "stuck", "--db", missing, "--json"); err == nil {
		t.Errorf("orders stuck answered from a store that does not exist:\n%s", out)
	}

	// The other direction: a full sync proves the end of every list and the
	// book answers with all 101 subscriptions.
	if out, err := runIONCLI(t, "sync", "--resources", "subscriptions,customers,orders", "--db", db, "--json"); err != nil {
		t.Fatalf("full sync: %v\n%s", err, out)
	}
	out, err := runIONCLI(t, "customers", "show", "108536", "--db", db, "--json")
	if err != nil {
		t.Fatalf("customers show after a full sync: %v\n%s", err, out)
	}
	tot := showTotals(t, out)
	if tot.Seats != 101 || tot.MonthlyPrice != 1010 || tot.MonthlyCost != 808 || !tot.SeatsComplete || !tot.PriceComplete || !tot.CostComplete {
		t.Fatalf("after a full sync want 101 seats, 1010 price, 808 cost, all complete; got %+v", tot)
	}
	for _, args := range book[1:] {
		if out, err := runIONCLI(t, append(args, "--db", db, "--json")...); err != nil {
			t.Errorf("%s refused a proven-complete store: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// ionFullPageServer serves 100 customers, 100 subscriptions and 100 orders: a
// full first page on every list. tokenKey decides how the page-token lists end:
// with "nextPageToken":"" in the body (the list's explicit end) or with the
// key left out (no statement either way).
func ionFullPageServer(t *testing.T, tokenKey bool) *httptest.Server {
	t.Helper()
	end := ""
	if tokenKey {
		end = `,"nextPageToken":""`
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var b strings.Builder
		switch r.URL.Path {
		case "/accounts/10505/customers":
			b.WriteString(`{"customers":[`)
			for i := 0; i < 100; i++ {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"name":"accounts/10505/customers/%d","customerOrganization":"Customer %d","customerStatus":"ACTIVE"}`, 108500+i, i)
			}
			b.WriteString(`]` + end + `}`)
		case "/accounts/10505/orders":
			b.WriteString(`{"orders":[`)
			for i := 0; i < 100; i++ {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"name":"accounts/10505/customers/108500/orders/%d","customerId":"108500","status":"COMPLETED"}`, 9000+i)
			}
			b.WriteString(`]` + end + `}`)
		case "/accounts/10505/subscriptions":
			offset, _ := strconv.Atoi(r.URL.Query().Get("pagination.offset"))
			b.WriteString(`{"items":[`)
			for i := offset; i < 100; i++ {
				if i > offset {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"id":"%d","customerId":"108500","subscriptionId":"v-%d","subscriptionStatus":"ACTIVE","subscriptionTotalLicenses":"1","subscriptionBillingCycle":"monthly","price":10,"cost":8,"currency":"USD"}`, 5000+i, i)
			}
			b.WriteString(`],"paginationResponse":{"totalSize":100}}`)
		default:
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(b.String()))
	}))
}

// An account with exactly 100 customers or orders ends its page-token list on
// a full page. ION continues a list only through nextPageToken (an empty
// pageToken returns the FIRST page), so a body that carries nextPageToken as
// an empty string is the list's end, and the book answers. A full page with
// no nextPageToken key states nothing and stays unproven.
func TestBookFullLastPageEndsOnlyOnAnEmptyToken(t *testing.T) {
	for _, tokenKey := range []bool{true, false} {
		t.Run(fmt.Sprintf("tokenKey=%v", tokenKey), func(t *testing.T) {
			testenv.Isolate(t)
			srv := ionFullPageServer(t, tokenKey)
			defer srv.Close()
			t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
			t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
			t.Setenv("TD_SYNNEX_ACCOUNT_ID", "10505")
			t.Setenv("TD_SYNNEX_NO_AUTO_REFRESH", "1")
			db := filepath.Join(t.TempDir(), "book.db")
			out, err := runIONCLI(t, "sync", "--resources", "subscriptions,customers,orders", "--db", db, "--json")
			if tokenKey && err != nil {
				t.Fatalf("full sync of lists ending on an empty nextPageToken: %v\n%s", err, out)
			}
			out, err = runIONCLI(t, "customers", "show", "108500", "--db", db, "--json")
			if !tokenKey {
				if err == nil || !strings.Contains(err.Error(), "customers") {
					t.Fatalf("a full page without nextPageToken must stay unproven: err=%v\n%s", err, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("customers show: %v\n%s", err, out)
			}
			if tot := showTotals(t, out); tot.Seats != 100 || !tot.SeatsComplete {
				t.Fatalf("want 100 seats, complete; got %+v", tot)
			}
		})
	}
}

// ionTwoCustomerServer serves two customers with subscriptions and honours the
// documented customerId filter on listSubscriptions.
func ionTwoCustomerServer(t *testing.T) *httptest.Server {
	t.Helper()
	subs := map[string]int{"108536": 3, "108537": 2}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/accounts/10505/subscriptions":
			want := r.URL.Query().Get("customerId")
			items := make([]map[string]any, 0)
			for _, cust := range []string{"108536", "108537"} {
				if want != "" && want != cust {
					continue
				}
				for i := 0; i < subs[cust]; i++ {
					items = append(items, map[string]any{"id": cust + "-" + strconv.Itoa(i), "customerId": cust, "subscriptionId": "v-" + cust + strconv.Itoa(i),
						"subscriptionStatus": "ACTIVE", "subscriptionTotalLicenses": "1", "subscriptionBillingCycle": "monthly", "price": 10, "cost": 8, "currency": "USD"})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "paginationResponse": map[string]any{"totalSize": len(items)}})
		case "/accounts/10505/customers":
			_, _ = w.Write([]byte(`{"customers":[{"name":"accounts/10505/customers/108536","customerOrganization":"Contoso"},{"name":"accounts/10505/customers/108537","customerOrganization":"Fabrikam"}],"nextPageToken":""}`))
		case "/accounts/10505/orders":
			_, _ = w.Write([]byte(`{"orders":[],"nextPageToken":""}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

// A walk narrowed by --param, --global-param or --resource-param reaches the
// end of a filtered list (here the documented customerId filter), not the
// resource's whole list, so it never proves the book complete.
func TestFilteredSyncNeverProvesTheBook(t *testing.T) {
	testenv.Isolate(t)
	srv := ionTwoCustomerServer(t)
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "10505")
	t.Setenv("TD_SYNNEX_NO_AUTO_REFRESH", "1")
	for _, filter := range [][]string{
		{"--resource-param", "subscriptions:customerId=108536"},
		{"--param", "customerId=108536"},
		{"--global-param", "customerId=108536"},
	} {
		db := filepath.Join(t.TempDir(), "book.db")
		args := append([]string{"sync", "--resources", "subscriptions,customers,orders", "--db", db, "--json"}, filter...)
		if out, err := runIONCLI(t, args...); err != nil {
			t.Fatalf("filtered sync %v: %v\n%s", filter, err, out)
		}
		out, err := runIONCLI(t, "margin", "--by", "customer", "--db", db, "--json")
		if err == nil || !strings.Contains(err.Error(), "subscriptions") {
			t.Errorf("margin answered from a sync filtered by %v (one of two customers): err=%v\n%s", filter, err, out)
		}
		if out, err := runIONCLI(t, "sync", "--resources", "subscriptions,customers,orders", "--db", db, "--json"); err != nil {
			t.Fatalf("unfiltered sync: %v\n%s", err, out)
		}
		out, err = runIONCLI(t, "margin", "--by", "customer", "--db", db, "--json")
		if err != nil || !strings.Contains(out, "108537") {
			t.Errorf("margin after an unfiltered sync must answer for both customers: err=%v\n%s", err, out)
		}
	}
}

// workflow archive --full used to reset every resource with SaveSyncState,
// which records the attempt as complete before any page is fetched; a run that
// stopped early (here its --timeout, during the first resource) left the
// resources it never reached proven complete with no rows.
func TestWorkflowArchiveFullNeverClaimsUnwalkedComplete(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // never answers: the archive's timeout ends the run
	}))
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "10505")
	t.Setenv("TD_SYNNEX_NO_AUTO_REFRESH", "1")
	db := filepath.Join(t.TempDir(), "book.db")
	_, _ = runIONCLI(t, "workflow", "archive", "--full", "--timeout", "300ms", "--db", db, "--json")
	for _, args := range [][]string{{"orders", "stuck"}, {"margin", "--by", "customer"}, {"position", "export"}} {
		out, err := runIONCLI(t, append(args, "--db", db, "--json")...)
		if err == nil {
			t.Errorf("%s answered from resources workflow archive --full never walked:\n%s", strings.Join(args, " "), out)
		}
	}
}

// ionSplitListServer serves an unfiltered listSubscriptions list that starts
// with customer 108537's 100 subscriptions (the whole first offset page of
// 100) followed by customer 108536's 101, and honours the documented
// customerId filter and offset pagination.
func ionSplitListServer(t *testing.T) *httptest.Server {
	t.Helper()
	type sub struct{ id, customer string }
	var all []sub
	for i := 0; i < 100; i++ {
		all = append(all, sub{strconv.Itoa(7000 + i), "108537"})
	}
	for i := 0; i < 101; i++ {
		all = append(all, sub{strconv.Itoa(5000 + i), "108536"})
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/accounts/10505/subscriptions":
			q := r.URL.Query()
			var list []sub
			for _, s := range all {
				if want := q.Get("customerId"); want == "" || want == s.customer {
					list = append(list, s)
				}
			}
			offset, _ := strconv.Atoi(q.Get("pagination.offset"))
			limit, _ := strconv.Atoi(q.Get("pagination.limit"))
			if limit <= 0 {
				limit = 100
			}
			items := make([]map[string]any, 0, limit)
			for i := offset; i < len(list) && i < offset+limit; i++ {
				items = append(items, map[string]any{
					"id": list[i].id, "customerId": list[i].customer, "subscriptionId": "v-" + list[i].id,
					"subscriptionStatus": "ACTIVE", "subscriptionTotalLicenses": "1", "subscriptionBillingCycle": "monthly",
					"price": 10, "cost": 8, "currency": "USD",
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "paginationResponse": map[string]any{"totalSize": len(list)}})
		case "/accounts/10505/customers":
			_, _ = w.Write([]byte(`{"customers":[{"name":"accounts/10505/customers/108536","customerOrganization":"Contoso"},{"name":"accounts/10505/customers/108537","customerOrganization":"Fabrikam"}],"nextPageToken":""}`))
		case "/accounts/10505/orders":
			_, _ = w.Write([]byte(`{"orders":[],"nextPageToken":""}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func ionBookEnv(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "10505")
	t.Setenv("TD_SYNNEX_NO_AUTO_REFRESH", "1")
}

func mustSync(t *testing.T, args ...string) {
	t.Helper()
	if out, err := runIONCLI(t, append([]string{"sync"}, args...)...); err != nil {
		t.Fatalf("sync %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// execStore writes sync_state directly, to recreate a row an earlier writer
// left behind.
func execStore(t *testing.T, dbPath, query string, args ...any) {
	t.Helper()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("opening %s: %v", dbPath, err)
	}
	defer s.Close()
	if _, err := s.DB().Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// wantSeats runs customers show and requires a complete total of seats.
func wantSeats(t *testing.T, dbPath, customer string, seats float64) {
	t.Helper()
	out, err := runIONCLI(t, "customers", "show", customer, "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("customers show %s: %v\n%s", customer, err, out)
	}
	start := strings.Index(out, "{")
	if start < 0 {
		t.Fatalf("customers show %s: no JSON in output: %s", customer, out)
	}
	var v customerShowView
	if err := json.NewDecoder(strings.NewReader(out[start:])).Decode(&v); err != nil {
		t.Fatalf("decoding customers show %s: %v\n%s", customer, err, out)
	}
	if len(v.Totals) != 1 || v.Totals[0].Seats != seats || !v.Totals[0].SeatsComplete {
		t.Fatalf("customers show %s: want %v seats, complete; got %d subscriptions, totals %+v", customer, seats, len(v.Subscriptions), v.Totals)
	}
}

// wantBookRefusal requires customers show to refuse with exit 10 and name the
// exact sync that proves the book.
func wantBookRefusal(t *testing.T, dbPath, customer string) {
	t.Helper()
	out, err := runIONCLI(t, "customers", "show", customer, "--db", dbPath, "--json")
	if err == nil {
		t.Fatalf("customers show %s answered from a store that does not prove the whole subscription list:\n%s", customer, out)
	}
	hint := "run: td-synnex-cli sync --resources customers,subscriptions,orders --db " + dbPath
	if code := ExitCode(err); code != 10 || !strings.Contains(err.Error(), "subscriptions") || !strings.Contains(err.Error(), hint) {
		t.Fatalf("customers show %s: want exit 10 naming subscriptions and %q; got exit %d: %v", customer, hint, code, err)
	}
}

// Codex round 2 (reprint 4.35.1), P1 incomplete read reported as complete: a
// filtered walk capped by --max-pages kept its resume offset, because filters
// stayed out of the checkpoint scope. The unfiltered retry resumed at offset
// 100, skipped customer 108537's 100 subscriptions (the first page of the
// whole list), ended naturally and proved the book, which then answered an
// empty subscription list for 108537. The unfiltered walk must rebind its own
// scope and walk the list from its start.
func TestUnfilteredRetryNeverResumesAFilteredWalk(t *testing.T) {
	testenv.Isolate(t)
	srv := ionSplitListServer(t)
	defer srv.Close()
	ionBookEnv(t, srv)
	db := filepath.Join(t.TempDir(), "book.db")
	mustSync(t, "--resources", "subscriptions,customers,orders", "--db", db, "--json", "--max-pages", "1", "--resource-param", "subscriptions:customerId=108536")
	mustSync(t, "--resources", "subscriptions,customers,orders", "--db", db, "--json")
	wantSeats(t, db, "108537", 100)
	wantSeats(t, db, "108536", 101)
}

// Codex round 2 (reprint 4.35.1), P1 incomplete read reported as complete: the
// filtered-walk invalidation ran after the completion flag was committed, so a
// crash between the two writes left a filtered walk proving the book. The row
// below is exactly that state (the invalidation changes only
// last_attempt_complete). A walk narrowed by a filter never proves the book,
// whatever its completion flag says; the sync the refusal names does.
func TestFilteredWalkProvesNothingWhateverItsFlag(t *testing.T) {
	testenv.Isolate(t)
	srv := ionTwoCustomerServer(t)
	defer srv.Close()
	ionBookEnv(t, srv)
	db := filepath.Join(t.TempDir(), "book.db")
	mustSync(t, "--resources", "customers,orders", "--db", db, "--json")
	mustSync(t, "--resources", "subscriptions", "--db", db, "--json", "--resource-param", "subscriptions:customerId=108536")
	execStore(t, db, `UPDATE sync_state SET last_attempt_complete = 1 WHERE resource_type = 'subscriptions'`)
	wantBookRefusal(t, db, "108537")

	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")
	wantSeats(t, db, "108537", 2)
	wantSeats(t, db, "108536", 3)
}

// Codex round 2 (reprint 4.35.1), P1 incomplete read reported as complete: a
// store a pre-fix build (published 0.1.2) synced with a filtered walk carries
// last_attempt_complete = 1 under a scope key with no param line (filters were
// not in the key) and no proof line. After the upgrade the book must refuse
// every such flag until a fresh unfiltered sync, then answer.
func TestBookRefusesProofWrittenBeforeTheScopeFix(t *testing.T) {
	for name, legacyScope := range map[string]string{
		"scope_key defaulted by the column migration": "",
		"pre-fix tenant scope key":                    "template:accountId=10505",
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			srv := ionTwoCustomerServer(t)
			defer srv.Close()
			ionBookEnv(t, srv)
			db := filepath.Join(t.TempDir(), "book.db")
			mustSync(t, "--resources", "subscriptions,customers,orders", "--db", db, "--json", "--resource-param", "subscriptions:customerId=108536")
			execStore(t, db, `UPDATE sync_state SET last_attempt_complete = 1, scope_key = ?`, legacyScope)
			wantBookRefusal(t, db, "108537")

			mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")
			wantSeats(t, db, "108537", 2)
			wantSeats(t, db, "108536", 3)
		})
	}
}
