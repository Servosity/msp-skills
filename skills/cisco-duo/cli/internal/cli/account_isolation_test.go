// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cisco-duo-pp-cli/internal/client"
	"cisco-duo-pp-cli/internal/cliutil"
	"cisco-duo-pp-cli/internal/cliutil/testenv"
	"github.com/spf13/cobra"
)

// Codex merge review (P1): a request that names its own subaccount (an
// account_id in a --stdin body or an import line, a billing command's own
// --account-id) reached that customer while its response was written into
// the pinned customer's, or the parent's, local store. These tests drive the
// real CLI against a stub Duo API with two subaccounts and look for each
// customer's marker in every local store file.

const (
	isoAcctA = "DAAAAAAAAAAAAAAAAAAA" // "Acme"
	isoAcctB = "DBBBBBBBBBBBBBBBBBBB" // "Bravo"
)

// isoDuo serves both subaccounts from one host and tags every response with
// the account that produced it (acme-marker / bravo-marker / parent-marker).
type isoDuo struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits []string // "METHOD path account_id"
}

func newIsoDuo(t *testing.T) *isoDuo {
	t.Helper()
	d := &isoDuo{}
	d.srv = httptest.NewServer(http.HandlerFunc(d.serve))
	t.Cleanup(d.srv.Close)
	return d
}

func (d *isoDuo) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	acct := r.URL.Query().Get("account_id")
	if s, ok := body["account_id"].(string); ok {
		acct = s
	}
	d.mu.Lock()
	d.hits = append(d.hits, r.Method+" "+r.URL.Path+" "+acct)
	d.mu.Unlock()
	tag, idChar := "parent", "P"
	switch acct {
	case isoAcctA:
		tag, idChar = "acme", "A"
	case isoAcctB:
		tag, idChar = "bravo", "B"
	}
	user := fmt.Sprintf(`{"user_id":"DU%s","username":"%s-marker-user","email":"u@%s.example","status":"active"}`, strings.Repeat(idChar, 18), tag, tag)
	w.Header().Set("Content-Type", "application/json")
	host := d.srv.Listener.Addr().String()
	switch {
	case r.URL.Path == "/accounts/v1/account/list":
		fmt.Fprintf(w, `{"stat":"OK","response":[{"account_id":%q,"name":"Acme","api_hostname":%q},{"account_id":%q,"name":"Bravo","api_hostname":%q}]}`, isoAcctA, host, isoAcctB, host)
	case strings.HasPrefix(r.URL.Path, "/accounts/"):
		fmt.Fprintf(w, `{"stat":"OK","response":{"account_id":"DCCCCCCCCCCCCCCCCCCC","name":"%s-marker-account","api_hostname":%q}}`, tag, host)
	case r.URL.Path == "/admin/v1/users/bulk_create":
		fmt.Fprintf(w, `{"stat":"OK","response":[%s]}`, user)
	case r.URL.Path == "/admin/v1/users" && r.Method == http.MethodPost:
		fmt.Fprintf(w, `{"stat":"OK","response":%s}`, user)
	case r.URL.Path == "/admin/v1/users":
		fmt.Fprintf(w, `{"stat":"OK","response":[%s],"metadata":{}}`, user)
	case r.URL.Path == "/admin/v1/billing/edition":
		fmt.Fprintf(w, `{"stat":"OK","response":{"edition":"%s-marker-edition"}}`, tag)
	default:
		fmt.Fprint(w, `{"stat":"OK","response":[]}`)
	}
}

// reached reports whether a request for path was answered for account.
func (d *isoDuo) reached(method, path, account string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, h := range d.hits {
		if h == method+" "+path+" "+account {
			return true
		}
	}
	return false
}

// isoEnv sandboxes the run (home, config, data dirs) and points the CLI at d.
func isoEnv(t *testing.T, d *isoDuo) string {
	t.Helper()
	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("CISCO_DUO_BASE_URL", d.srv.URL)
	t.Setenv("CISCO_DUO_API_HOST", d.srv.Listener.Addr().String())
	t.Setenv("CISCO_DUO_IKEY", "DIISOLATIONTEST00001")
	t.Setenv("CISCO_DUO_SKEY", "SECRETKEY")
	// Registered here so cleanup restores whatever --account-id writes.
	t.Setenv(client.DuoAccountEnv, "")
	t.Setenv(duoParentDataDirEnv, "")
	savedUser, savedInherited := userDataDir, inheritedBase
	userDataDir, inheritedBase = "", ""
	t.Cleanup(func() {
		userDataDir, inheritedBase = savedUser, savedInherited
		storeScopedRun.Store(false)
	})
	return home
}

// runIsoCLI executes the CLI in-process with stdin (the generated --stdin
// and import commands read os.Stdin directly).
func runIsoCLI(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(stdin), 0o600); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdin
	os.Stdin = in
	defer func() { os.Stdin = saved; _ = in.Close() }()
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), err
}

// storesHolding lists every local SQLite store file (db or WAL) under home
// whose bytes contain marker, relative to home.
func storesHolding(t *testing.T, home, marker string) []string {
	t.Helper()
	var found []string
	_ = filepath.WalkDir(home, func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() || !strings.Contains(de.Name(), ".db") {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && bytes.Contains(b, []byte(marker)) {
			rel, _ := filepath.Rel(home, p)
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	return found
}

// TestPinnedStdinBodyCannotNameAnotherSubaccount is the finding's repro:
// `--account-id A users bulk-create --stdin` with account_id=B created B's
// users and stored them in A's mirror.
func TestPinnedStdinBodyCannotNameAnotherSubaccount(t *testing.T) {
	d := newIsoDuo(t)
	home := isoEnv(t, d)
	out, err := runIsoCLI(t, `{"account_id":"`+isoAcctB+`","users":[{"username":"bob"}]}`,
		"users", "bulk-create", "--stdin", "--account-id", isoAcctA, "--json")
	if got := storesHolding(t, home, "bravo-marker"); len(got) != 0 {
		t.Fatalf("customer B's data was written into a local store: %v", got)
	}
	if err == nil {
		t.Fatalf("a body naming another subaccount must be refused under a pin; output: %s", out)
	}
	if d.reached("POST", "/admin/v1/users/bulk_create", isoAcctB) || d.reached("POST", "/admin/v1/users/bulk_create", isoAcctA) {
		t.Fatalf("the refused request must not reach any tenant; hits: %v", d.hits)
	}
	if !strings.Contains(out+err.Error(), "pinned") {
		t.Fatalf("the refusal must say why (pinned to another subaccount): %v %s", err, out)
	}
}

// TestPinnedStdinBodyNamingThePinIsStoredInItsOwnMirror is the positive
// control: the marker scan does see a stored response, and only in the
// pinned customer's directory.
func TestPinnedStdinBodyNamingThePinIsStoredInItsOwnMirror(t *testing.T) {
	d := newIsoDuo(t)
	home := isoEnv(t, d)
	// The pin by name, the body by ID: the same customer.
	out, err := runIsoCLI(t, `{"account_id":"`+isoAcctA+`","users":[{"username":"amy"}]}`,
		"users", "bulk-create", "--stdin", "--account-id", "Acme", "--json")
	if err != nil {
		t.Fatalf("a body naming the pinned subaccount must succeed: %v %s", err, out)
	}
	if !d.reached("POST", "/admin/v1/users/bulk_create", isoAcctA) {
		t.Fatalf("request not routed to the pinned subaccount; hits: %v", d.hits)
	}
	got := storesHolding(t, home, "acme-marker")
	if len(got) == 0 {
		t.Fatal("positive control: the created user must be stored, or the marker scan proves nothing")
	}
	for _, p := range got {
		if !strings.Contains(p, "/subaccounts/"+subaccountDirName("Acme")+"/") {
			t.Fatalf("customer A's data stored outside A's mirror: %v", got)
		}
	}
}

// TestUnpinnedStdinBodyCannotNameASubaccount: with nothing pinned the store
// is the parent's, so a body naming a subaccount would put that customer's
// users in the parent's mirror.
func TestUnpinnedStdinBodyCannotNameASubaccount(t *testing.T) {
	d := newIsoDuo(t)
	home := isoEnv(t, d)
	out, err := runIsoCLI(t, `{"account_id":"`+isoAcctB+`","users":[{"username":"bob"}]}`,
		"users", "bulk-create", "--stdin", "--json")
	if got := storesHolding(t, home, "bravo-marker"); len(got) != 0 {
		t.Fatalf("customer B's data was written into the parent's store: %v", got)
	}
	if err == nil || d.reached("POST", "/admin/v1/users/bulk_create", isoAcctB) {
		t.Fatalf("an unpinned body naming a subaccount must be refused before it is sent: %v %s %v", err, out, d.hits)
	}
	if !strings.Contains(out+err.Error(), "--account-id") {
		t.Fatalf("the refusal must point at --account-id: %v %s", err, out)
	}
}

// TestPinnedImportRefusesLinesForAnotherSubaccount: a bulk --input - stream
// with mixed account_ids sends only the pinned customer's lines.
func TestPinnedImportRefusesLinesForAnotherSubaccount(t *testing.T) {
	d := newIsoDuo(t)
	isoEnv(t, d)
	lines := strings.Join([]string{
		`{"username":"a1","account_id":"` + isoAcctA + `"}`,
		`{"username":"b1","account_id":"` + isoAcctB + `"}`,
		`{"username":"a2"}`,
	}, "\n") + "\n"
	out, err := runIsoCLI(t, lines, "import", "users", "--input", "-", "--account-id", isoAcctA, "--json")
	if err == nil {
		t.Fatalf("a line naming another subaccount must fail the import: %s", out)
	}
	if d.reached("POST", "/admin/v1/users", isoAcctB) {
		t.Fatalf("a line for customer B reached B while pinned to A; hits: %v", d.hits)
	}
	n := 0
	for _, h := range d.hits {
		if h == "POST /admin/v1/users "+isoAcctA {
			n++
		}
	}
	var summary struct {
		Succeeded, Failed int
	}
	_ = json.NewDecoder(strings.NewReader(out)).Decode(&summary)
	if n != 2 || summary.Succeeded != 2 || summary.Failed != 1 {
		t.Fatalf("the two lines for the pinned customer must still be imported (got %d) and one counted failed: %s", n, out)
	}
}

// TestBillingAccountFlagPinsTheRun: the billing commands send their own
// --account-id as the request's account_id; it must also pin the run, so a
// response (or an offline answer from the local store) belongs to the same
// customer as the request, even when CISCO_DUO_ACCOUNT_ID names another.
func TestBillingAccountFlagPinsTheRun(t *testing.T) {
	d := newIsoDuo(t)
	home := isoEnv(t, d)
	t.Setenv(client.DuoAccountEnv, isoAcctA)
	if err := pinSubaccountDataDir(isoAcctA); err != nil { // what init() does for an env pin
		t.Fatal(err)
	}
	out, err := runIsoCLI(t, "", "billing", "get-edition", "--account-id", isoAcctB, "--json")
	if err != nil || !d.reached("GET", "/admin/v1/billing/edition", isoAcctB) {
		t.Fatalf("billing get-edition --account-id B must reach B: %v %s %v", err, out, d.hits)
	}
	if got := client.PinnedAccountID(); got != isoAcctB {
		t.Fatalf("the billing --account-id must pin the run (store selection), pin is %q", got)
	}
	if dir := os.Getenv(duoDataDirEnv); filepath.Base(dir) != isoAcctB {
		t.Fatalf("the run must use B's store, data dir is %s", dir)
	}
	for _, p := range storesHolding(t, home, "bravo-marker") {
		if !strings.Contains(p, "/subaccounts/"+isoAcctB+"/") {
			t.Fatalf("customer B's billing data stored outside B's mirror: %s", p)
		}
	}
	// Unpinned, the flag is the pin: the request is not refused as a
	// subaccount request against the parent's store.
	t.Setenv(client.DuoAccountEnv, "")
	if err := pinSubaccountDataDir(""); err != nil {
		t.Fatal(err)
	}
	if out, err := runIsoCLI(t, "", "billing", "get-edition", "--account-id", "Bravo", "--json"); err != nil {
		t.Fatalf("unpinned billing get-edition --account-id must work: %v %s", err, out)
	}
}

// TestEveryOwnAccountFlagPinsTheRun keeps the class closed: a command that
// declares its own --account-id (shadowing the root flag) must pin through
// it, or its request and its store name different customers.
func TestEveryOwnAccountFlagPinsTheRun(t *testing.T) {
	root := RootCmd()
	seen := 0
	var visit func(c *cobra.Command)
	visit = func(c *cobra.Command) {
		for _, child := range c.Commands() {
			if f := child.Flags().Lookup("account-id"); f != nil {
				switch f.Value.(type) {
				case accountEnvFlag, pinningAccountFlag:
					seen++
				default:
					if !strings.HasPrefix(child.Annotations["pp:path"], "/accounts/") {
						t.Errorf("%s declares --account-id (%T) that does not pin the run", child.CommandPath(), f.Value)
					}
				}
			}
			visit(child)
		}
	}
	visit(root)
	if seen < 6 {
		t.Fatalf("expected at least the six billing commands to pin, saw %d", seen)
	}
}

// TestPinnedRunRefusesParentSubaccountWrites: accounts create/delete run as
// the MSP parent; their response must not be stored in a pinned customer's
// mirror. accounts list stores nothing and stays allowed.
func TestPinnedRunRefusesParentSubaccountWrites(t *testing.T) {
	d := newIsoDuo(t)
	home := isoEnv(t, d)
	out, err := runIsoCLI(t, "", "accounts", "create", "--name", "New Co", "--account-id", isoAcctA, "--json")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("accounts create under a pin must be a usage error: %v %s", err, out)
	}
	if d.reached("POST", "/accounts/v1/account/create", "") {
		t.Fatalf("the refused create reached the API: %v", d.hits)
	}
	if got := storesHolding(t, home, "-marker-account"); len(got) != 0 {
		t.Fatalf("a parent subaccount row was stored: %v", got)
	}
	if out, err := runIsoCLI(t, "", "accounts", "list", "--account-id", isoAcctA, "--json"); err != nil {
		t.Fatalf("accounts list stores nothing and must still work under a pin: %v %s", err, out)
	}
}

// TestFleetFanOutStillNamesEachSubaccount guards the other direction: fleet
// commands never touch the store, so their per-subaccount requests must not
// be refused by the unpinned guard (with or without a pin to clear).
func TestFleetFanOutStillNamesEachSubaccount(t *testing.T) {
	d := newIsoDuo(t)
	isoEnv(t, d)
	if out, err := runIsoCLI(t, "", "fleet", "find-user", "bravo-marker-user", "--account-id", isoAcctA, "--json"); err != nil {
		t.Fatalf("fleet find-user must fan out: %v %s", err, out)
	}
	if !d.reached("GET", "/admin/v1/users", isoAcctB) || !d.reached("GET", "/admin/v1/users", isoAcctA) {
		t.Fatalf("fleet find-user must query every subaccount; hits: %v", d.hits)
	}
}
