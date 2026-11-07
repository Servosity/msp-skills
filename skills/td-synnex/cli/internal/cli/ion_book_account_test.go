// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json: ion-book-one-account): the licence book
// answers only from walks of the ION account it is asked about.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"td-synnex-pp-cli/internal/cliutil/testenv"
	"td-synnex-pp-cli/internal/store"
)

// ionTwoAccountServer serves the documented list shapes (listCustomers,
// listSubscriptions, listOrders) for two ION accounts, 10505 (customers
// 108536, 108537) and 20000 (customers 208536, 208537), each customer holding
// one monthly subscription of one seat, and answers createCustomer with the
// new customer <first two digits of the account>9999.
func ionTwoAccountServer(t *testing.T) *httptest.Server {
	t.Helper()
	customers := map[string][]string{"10505": {"108536", "108537"}, "20000": {"208536", "208537"}}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 3 || parts[0] != "accounts" || customers[parts[1]] == nil {
			http.NotFound(w, r)
			return
		}
		acct := parts[1]
		switch {
		case parts[2] == "customers" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "accounts/" + acct + "/customers/" + acct[:2] + "9999", "customerOrganization": "New Org"})
		case parts[2] == "customers":
			items := []map[string]any{}
			for _, c := range customers[acct] {
				items = append(items, map[string]any{"name": "accounts/" + acct + "/customers/" + c, "customerOrganization": "Org " + c})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"customers": items, "nextPageToken": ""})
		case parts[2] == "subscriptions":
			items := []map[string]any{}
			for _, c := range customers[acct] {
				items = append(items, ionOneSeat(c))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "paginationResponse": map[string]any{"totalSize": len(items)}})
		case parts[2] == "orders":
			_, _ = w.Write([]byte(`{"orders":[],"nextPageToken":""}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func ionOneSeat(customer string) map[string]any {
	return map[string]any{"id": customer + "-0", "customerId": customer, "subscriptionId": "v-" + customer, "subscriptionStatus": "ACTIVE",
		"subscriptionTotalLicenses": "1", "subscriptionBillingCycle": "monthly", "price": 10, "cost": 8, "currency": "USD"}
}

// wantMarginCustomers runs margin --by customer and requires it to answer
// exactly for want, of the four customers the two accounts hold.
func wantMarginCustomers(t *testing.T, dbPath string, want ...string) {
	t.Helper()
	out, err := runIONCLI(t, "margin", "--by", "customer", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("margin: %v\n%s", err, out)
	}
	for _, c := range []string{"108536", "108537", "208536", "208537"} {
		in := false
		for _, w := range want {
			in = in || w == c
		}
		if strings.Contains(out, c) != in {
			t.Fatalf("margin for TD_SYNNEX_ACCOUNT_ID=%s: want customers %v, customer %s present=%v:\n%s", os.Getenv("TD_SYNNEX_ACCOUNT_ID"), want, c, !in, out)
		}
	}
}

// wantForeignRefusal requires the book to refuse (exit 10, the exact sync)
// and name the account the store's rows were walked for.
func wantForeignRefusal(t *testing.T, dbPath, customer, found string) {
	t.Helper()
	wantBookRefusal(t, dbPath, customer)
	_, err := runIONCLI(t, "customers", "show", customer, "--db", dbPath, "--json")
	if !strings.Contains(fmt.Sprint(err), found) {
		t.Fatalf("customers show %s: the refusal must name the account the store holds (%s): %v", customer, found, err)
	}
}

// Adversarial review 2026-11-06, P1 (the book accepts another account's walk):
// --path-context accountId=20000 walked account 20000 into account 10505's
// store, and the book answered for 10505 from 20000's rows. sync refuses any
// accountId, from --path-context or a param, that is not the store's account.
func TestSyncRefusesAnotherAccountsWalk(t *testing.T) {
	testenv.Isolate(t)
	srv := ionTwoAccountServer(t)
	defer srv.Close()
	ionBookEnv(t, srv)
	db := filepath.Join(t.TempDir(), "book.db")
	for _, foreign := range [][]string{
		{"--path-context", "accountId=20000"},
		{"--param", "accountId=20000"},
		{"--global-param", "accountId=20000"},
		{"--resource-param", "customers:accountId=20000"},
	} {
		out, err := runIONCLI(t, append([]string{"sync", "--resources", "customers,subscriptions,orders", "--db", db, "--json"}, foreign...)...)
		if code := ExitCode(err); code != 2 || !strings.Contains(fmt.Sprint(err), "10505") || !strings.Contains(fmt.Sprint(err), "TD_SYNNEX_ACCOUNT_ID=20000") {
			t.Errorf("sync %v into account 10505's store: want exit 2 naming 10505 and TD_SYNNEX_ACCOUNT_ID=20000; got exit %d: %v\n%s", foreign, code, err, out)
		}
	}
	// The store's own account is not another account.
	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json", "--path-context", "accountId=10505")
	wantMarginCustomers(t, db, "108536", "108537")
}

// A store an explicit --db shares between accounts: each walk empties the
// lists another account's walk left (sync never prunes them), and the book
// refuses the lists of an account it is not asked about instead of merging.
func TestBookAnswersOnlyFromWalksOfItsAccount(t *testing.T) {
	testenv.Isolate(t)
	srv := ionTwoAccountServer(t)
	defer srv.Close()
	ionBookEnv(t, srv)
	db := filepath.Join(t.TempDir(), "book.db")

	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "20000")
	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "10505")
	wantForeignRefusal(t, db, "108536", "20000")

	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")
	wantMarginCustomers(t, db, "108536", "108537")
	wantSeats(t, db, "108536", 1)

	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "20000")
	wantForeignRefusal(t, db, "208536", "10505")
	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")
	wantMarginCustomers(t, db, "208536", "208537")
}

// A store written before the fix can hold another account's rows under a
// whole-walk proof of this one (sync 20000 then sync 10505 never pruned) and
// records no account. The book refuses it; one sync empties and re-walks it.
func TestBookRefusesAStoreNotTracedToItsAccount(t *testing.T) {
	testenv.Isolate(t)
	srv := ionTwoAccountServer(t)
	defer srv.Close()
	ionBookEnv(t, srv)
	db := filepath.Join(t.TempDir(), "book.db")
	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")

	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	cust, _ := json.Marshal(map[string]any{"name": "accounts/20000/customers/208536", "customerOrganization": "Org 208536"})
	sub, _ := json.Marshal(ionOneSeat("208536"))
	for resource, row := range map[string][]byte{"customers": cust, "subscriptions": sub} {
		if _, _, err := s.UpsertBatch(resource, []json.RawMessage{row}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB().Exec(`DROP TABLE IF EXISTS ion_store_account`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	wantBookRefusal(t, db, "208536")
	wantBookRefusal(t, db, "108536")

	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")
	wantMarginCustomers(t, db, "108536", "108537")
	if out, err := runIONCLI(t, "customers", "show", "208536", "--db", db, "--json"); ExitCode(err) != 3 {
		t.Fatalf("customers show 208536 after a sync of 10505: want exit 3 (not in 10505's book); got %v\n%s", err, out)
	}
}

// customers create 20000 saved account 20000's new customer into the
// configured account 10505's store, and the book answered for it. A command
// aimed at another account saves nothing locally; one aimed at the store's
// account still does.
func TestForeignAccountWriteBackStaysOutOfTheBook(t *testing.T) {
	testenv.Isolate(t)
	srv := ionTwoAccountServer(t)
	defer srv.Close()
	ionBookEnv(t, srv)
	mustSync(t, "--resources", "customers,subscriptions,orders", "--json")
	create := func(account ...string) {
		t.Helper()
		args := append(append([]string{"customers", "create"}, account...), "--customer-organization", "New Org", "--customer-name", "Jane Doe",
			"--customer-email", "jane@neworg.example", "--language-code", "en", "--customer-address-street", "1 Main St",
			"--customer-address-city", "Austin", "--customer-address-state", "TX", "--customer-address-zip", "78701",
			"--customer-address-country", "US", "--json")
		if out, err := runIONCLI(t, args...); err != nil {
			t.Fatalf("customers create %v: %v\n%s", account, err, out)
		}
	}
	create("20000")
	if out, err := runIONCLI(t, "customers", "show", "209999", "--json"); ExitCode(err) != 3 {
		t.Fatalf("the 10505 book answered for account 20000's new customer 209999: %v\n%s", err, out)
	}
	if v, ok := os.LookupEnv(noLocalDataEnvVar); ok {
		t.Fatalf("%s=%q outlived the command aimed at another account", noLocalDataEnvVar, v)
	}
	create()
	if out, err := runIONCLI(t, "customers", "show", "109999", "--json"); err != nil {
		t.Fatalf("account 10505's own new customer must still reach its book: %v\n%s", err, out)
	}
}

// The account record alone is not trusted: a latest walk whose scope names
// another account (what a build without the sync refusal writes for
// --path-context accountId=20000) refuses the book whatever the record says.
func TestBookScopeMustNameItsAccount(t *testing.T) {
	testenv.Isolate(t)
	srv := ionTwoAccountServer(t)
	defer srv.Close()
	ionBookEnv(t, srv)
	db := filepath.Join(t.TempDir(), "book.db")
	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")
	execStore(t, db, `UPDATE sync_state SET scope_key = replace(scope_key, 'template:accountId=10505', 'template:accountId=20000') WHERE resource_type = 'subscriptions'`)
	wantForeignRefusal(t, db, "108536", "20000")
}
