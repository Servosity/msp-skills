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
