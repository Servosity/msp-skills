// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring): what is Maxio's own on the
// built binaries. HTTP Basic with the key as username and the constant password
// x, a site that fills the API host, card and bank secrets in requests and
// responses, and the two MCP write gates.

package maxioblackbox

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func basicOf(key string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(key+":x"))
}

// basic-constant-password-public and legacy-basic-env: every credential source
// sends the one header Maxio takes, and a MAXIO_PASSWORD other than x (the
// published connector never sent one) is neither sent nor printed.
func TestBasicHeaderIsTheKeyAndTheConstantPassword(t *testing.T) {
	const legacyKey, legacyPass, stored = "LegacyKey-" + "6f2b8d4e1a", "NotX-" + "3c7e1b5a", "StoredKey-" + "71c3e9b05d"
	f := newFake(t)
	read := []string{"customers", "read", plainID, "--json"}
	lastAuth := func() string {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.auths) == 0 {
			return ""
		}
		return f.auths[len(f.auths)-1]
	}
	base := func(home string) []string { return append(baseEnv(home), "MAXIO_BASE_URL="+f.srv.URL) }

	out, code := runWith(t, env(t.TempDir(), f), "", read...)
	if code != 0 || lastAuth() != basicOf(apiKey) {
		t.Errorf("MAXIO_API_KEY: exit %d, header %q:\n%s", code, lastAuth(), out)
	}
	out, code = runWith(t, append(base(t.TempDir()), "MAXIO_USERNAME="+legacyKey, "MAXIO_PASSWORD=x"), "", read...)
	if code != 0 || lastAuth() != basicOf(legacyKey) {
		t.Errorf("MAXIO_USERNAME with x: exit %d, header %q:\n%s", code, lastAuth(), out)
	}
	assertNoneOf(t, "legacy pair", out, legacyKey)
	out, _ = runWith(t, append(base(t.TempDir()), "MAXIO_USERNAME="+legacyKey, "MAXIO_PASSWORD="+legacyPass), "", "customers", "read", legacyPass, "--json")
	assertNoneOf(t, "a password other than x", out, legacyKey, legacyPass)
	home := t.TempDir()
	if out, code = runWith(t, base(home), stored+"\n", "auth", "set-token"); code != 0 {
		t.Fatalf("auth set-token: exit %d:\n%s", code, out)
	}
	assertNoneOf(t, "auth set-token", out, stored)
	out, code = runWith(t, base(home), "", read...)
	if code != 0 || lastAuth() != basicOf(stored) {
		t.Errorf("the credential store: exit %d, header %q:\n%s", code, lastAuth(), out)
	}
	scanFiles(t, home, stored)
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.auths {
		if strings.Contains(a, base64.StdEncoding.EncodeToString([]byte(legacyPass))) || a == basicOf(legacyPass) {
			t.Errorf("the refused password was sent: %q", a)
		}
	}
}

// basic-constant-password-public: the documented password x is public. A record
// whose words include it reaches the operator whole, on every credential
// spelling, and the API key is still held.
func TestConstantPasswordIsNeverHeld(t *testing.T) {
	f := newFake(t)
	for name, environ := range map[string][]string{
		"MAXIO_API_KEY": env(t.TempDir(), f),
		"legacy pair":   append(baseEnv(t.TempDir()), "MAXIO_BASE_URL="+f.srv.URL, "MAXIO_USERNAME="+apiKey, "MAXIO_PASSWORD=x"),
	} {
		out, code := runWith(t, environ, "", "customers", "read", xID, "--json")
		if code != 0 || strings.Contains(out, kitNotice) || !strings.Contains(out, "size x large") {
			t.Errorf("%s: a record with the word x was withheld (exit %d):\n%s", name, code, out)
		}
	}
}

// site-required and site-origin-pinned: the site fills the API host. No site
// names the variable and sends nothing; a site that would move the host never
// carries the key to it. A proxy trap sees every request a binary tries.
func TestSiteNeverMovesTheKey(t *testing.T) {
	var dials atomic.Int32
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dials.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(trap.Close)
	base := func() []string {
		return append(baseEnv(t.TempDir()), "MAXIO_API_KEY="+apiKey, "HTTPS_PROXY="+trap.URL, "HTTP_PROXY="+trap.URL, "NO_PROXY=", "no_proxy=")
	}
	list := []string{"customers-json", "list-customers", "--json"}
	out, code := runWith(t, base(), "", list...)
	if code == 0 || !strings.Contains(out, "MAXIO_SITE") {
		t.Errorf("no site: want a failure naming MAXIO_SITE, exit %d:\n%s", code, out)
	}
	assertNoneOf(t, "no site", out, apiKey)
	for _, site := range []string{"evil.example/x?", "x.attacker", "a@b", "evil.example#", "evil.example:443"} {
		out, code := runWith(t, append(base(), "MAXIO_SITE="+site), "", list...)
		if code == 0 {
			t.Errorf("MAXIO_SITE=%q listed customers:\n%s", site, out)
		}
		assertNoneOf(t, "MAXIO_SITE="+site, out, apiKey)
	}
	for _, site := range []string{"evil", "evil.example/x?"} {
		out, _ := runWith(t, base(), "", "sync", "--resources", "customers-json", "--path-context", "site="+site, "--json")
		assertNoneOf(t, "sync --path-context site="+site, out, apiKey)
	}
	if n := dials.Load(); n != 0 {
		t.Errorf("%d request(s) got past the site check and dialed out", n)
	}
}

// response-credentials-redacted: an ordinary subscription list carries a card's
// vault_token, customer_vault_token and the self-service token. The list still
// reaches the operator with those values replaced, and no file keeps them.
func TestCredentialFieldsInAnOrdinaryListAreRedactedNotWithheld(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	out, code := runWith(t, env(home, f), "", "subscriptions-json", "list-subscriptions", "--json")
	if code != 0 || strings.Contains(out, kitNotice) || !strings.Contains(out, "HQ-Record") {
		t.Errorf("a subscription list with card vault tokens was not answered (exit %d):\n%s", code, out)
	}
	assertNoneOf(t, "subscription list", out, vaultToken, customerVault, selfServiceToken)
	if out, code = runWith(t, env(home, f), "", "sync", "--resources", "subscriptions-json", "--json"); code != 0 {
		t.Errorf("sync: exit %d:\n%s", code, out)
	}
	assertNoneOf(t, "sync", out, vaultToken, customerVault, selfServiceToken)
	// A response credential is redacted; a vault reference as short as "1" is
	// not held, so the word 1 stays readable in the same answer.
	out, code = runWith(t, env(home, f), "", "payment-profiles", "read", "44", "--json")
	if code != 0 || !strings.Contains(out, "HQ-Record") || !strings.Contains(out, "44") {
		t.Errorf("a payment profile read was not answered (exit %d):\n%s", code, out)
	}
	assertNoneOf(t, "payment profile", out, responseToken)
	scanFiles(t, home, vaultToken, customerVault, selfServiceToken, responseToken)
}

// maxio-credential-names-exact: names that read as credentials but are billing
// data are never withheld or redacted.
func TestMaxioPublicFieldsAreNotHeld(t *testing.T) {
	f := newFake(t)
	out, code := runWith(t, env(t.TempDir(), f), "", "customers", "read", publicID, "--json")
	if code != 0 || strings.Contains(out, kitNotice) {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, want := range []string{"payment_success", "XXXX-XXXX-XXXX-1111", "4242", "XXXX4321", "HQ-Authorizer", "hq-dedupe-7731", "9988771", "secure-checkout"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is billing data and must print:\n%s", want, out)
		}
	}
}

// credential-stdin-held: a --stdin body is read whole, pretty-printed or not. A
// create's echo of a plain body still prints; a card number or CVV in the body
// is held, so the echo of that body never prints it.
func TestStdinBodyIsHeldAndItsPlainEchoPrints(t *testing.T) {
	const number, cvv = "4111" + "111111111111", "Cvv-" + "7c1d9a"
	f := newFake(t)
	home := t.TempDir()
	pretty := "{\n  \"customer\": {\n    \"first_name\": \"Stdin\",\n    \"last_name\": \"Body\",\n    \"email\": \"s@example.com\",\n    \"organization\": \"Pretty Org\"\n  }\n}\n"
	out, code := runWith(t, env(home, f), pretty, "customers-json", "create-customer", "--stdin", "--json")
	if code != 0 || strings.Contains(out, kitNotice) || !strings.Contains(out, "Pretty Org") {
		t.Errorf("a pretty-printed --stdin create was not answered (exit %d):\n%s", code, out)
	}
	card := `{"payment_profile":{"customer_id":1,"first_name":"HQ","last_name":"Card","full_number":"` + number + `","cvv":"` + cvv + `","expiration_month":"12","expiration_year":"2030"}}`
	out, _ = runWith(t, env(home, f), card, "payment-profiles-json", "create-payment-profile", "--stdin", "--json")
	assertNoneOf(t, "card --stdin", out, number, cvv)
	out, _ = runWith(t, env(home, f), card, "payment-profiles-json", "create-payment-profile", "--stdin", "--dry-run", "--json")
	assertNoneOf(t, "card --stdin --dry-run", out, number, cvv)
	// A body that does not parse is still read for its credential fields.
	broken := `{"payment_profile":{"full_number":"` + number + `","cvv":"` + cvv + `",`
	out, _ = runWith(t, env(home, f), broken, "payment-profiles-json", "create-payment-profile", "--stdin", "--json")
	assertNoneOf(t, "a --stdin body that does not parse", out, number, cvv)
	out, _ = runWith(t, env(home, f), broken, "payment-profiles-json", "create-payment-profile", "--stdin", "--dry-run", "--json")
	assertNoneOf(t, "a --stdin body that does not parse, --dry-run", out, number, cvv)
	scanFiles(t, home, number, cvv)
}

// credential-flags-held: a card secret on the command line never prints, a value
// that starts with a dash included.
func TestCredentialFlagIsHeld(t *testing.T) {
	const cvv = "-argvSecret-" + "91c3e7a5"
	f := newFake(t)
	home := t.TempDir()
	out, _ := runWith(t, env(home, f), "", "payment-profiles-json", "create-payment-profile", "--payment-profile-customer-id", "1",
		"--payment-profile-full-number", "4111"+"111111111111", "--payment-profile-cvv="+cvv, "--dry-run", "--json")
	assertNoneOf(t, "--payment-profile-cvv", out, cvv, "4111"+"111111111111")
	if w := f.writes(); len(w) != 0 {
		t.Errorf("--dry-run sent %v", w)
	}
}

// credential-positional-held: the one-time token is a path value. The API's
// not-found repeats the path it was asked for, and the token still never prints.
func TestOneTimeTokenPositionalIsHeld(t *testing.T) {
	const token = "tok_oneTime" + "9d2c7b1e5a"
	f := newFake(t)
	home := t.TempDir()
	out, _ := runWith(t, env(home, f), "", "one-time-tokens", token, "--json")
	assertNoneOf(t, "one-time-tokens", out, token)
	if f.apiHits.Load() == 0 {
		t.Error("the request never reached the API, so the echo was never offered")
	}
	scanFiles(t, home, token)
}

// Class credential-split-across-output-records. Factory r2 failing input: a key
// whose two halves a client sends as the ids of two requests; the stdio protocol
// stream then spelled the key across two replies. Each part is 8 or more
// characters, so the kit withholds it alone, and a replacement that would carry
// the id ends the session.
func TestMCPKeySplitAcrossProtocolRepliesNeverPrints(t *testing.T) {
	const a, b = "IdHalfOne", "IdHalfTwo"
	home := t.TempDir()
	stdout, stderr := runMCPOnce(t, append(baseEnv(home), "MAXIO_API_KEY="+a+b), initializeFrame,
		`{"jsonrpc":"2.0","id":"`+a+`","method":"ping"}`, `{"jsonrpc":"2.0","id":"`+b+`","method":"ping"}`)
	for _, part := range []string{a, b} {
		if strings.Contains(stdout, part) || strings.Contains(stderr, part) {
			t.Errorf("the protocol stream spells the held value (%s):\nstdout:\n%s\nstderr:\n%s", part, stdout, stderr)
		}
	}
	// Factory r2's other shape: a key that ends in a word of the stand-in notice.
	stdout, stderr = runMCPOnce(t, append(baseEnv(t.TempDir()), "MAXIO_API_KEY="+a+"withheld"), initializeFrame,
		`{"jsonrpc":"2.0","id":"`+a+`","method":"ping"}`)
	if strings.Contains(stdout, a) || strings.Contains(stderr, a) {
		t.Errorf("the protocol stream carries a part of the held value:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

// mcp-endpoint-write-confirm: an MCP write reaches the API only when confirm is
// the boolean true, and maxio_read refuses a write outright.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	f := newFake(t)
	s := startMCP(t, env(t.TempDir(), f))
	del := func(confirm any) map[string]any {
		args := map[string]any{"endpoint_id": "customers.delete", "params": map[string]any{"id": plainID}}
		if confirm != nil {
			args["confirm"] = confirm
		}
		return args
	}
	for _, confirm := range []any{nil, "true", 1, false} {
		if r := s.call("maxio_execute", del(confirm)); !strings.Contains(r, "confirmation_required") {
			t.Errorf("confirm %v did not preview: %.800s", confirm, r)
		}
	}
	if r := s.call("maxio_read", map[string]any{"endpoint_id": "customers.delete", "params": map[string]any{"id": plainID}}); !strings.Contains(r, "changes state") {
		t.Errorf("maxio_read did not refuse a delete: %.800s", r)
	}
	if r := s.call("import", map[string]any{"resource": "customers-json"}); !strings.Contains(r, "Preview only: nothing was sent") {
		t.Errorf("the mirrored import did not preview: %.800s", r)
	}
	if w := f.writes(); len(w) != 0 {
		t.Fatalf("a write reached the API without confirm true: %v", w)
	}
	s.call("maxio_execute", del(true))
	if w := f.writes(); len(w) != 1 || w[0] != "DELETE /customers/"+plainID+".json" {
		t.Errorf("confirm true: requests %v, want the one delete", w)
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API.
func TestDryRunSendsNothing(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	for _, args := range [][]string{
		{"customers-json", "create-customer", "--customer-organization", "Dry", "--customer-first-name", "D", "--customer-last-name", "R", "--customer-email", "d@example.com", "--dry-run"},
		{"customers", "update", plainID, "--customer-organization", "Dry", "--dry-run"},
		{"customers", "delete", plainID, "--yes", "--dry-run"},
		{"events", "record", "hq-usage", "--chargify-subscription-id", "15", "--yes", "--dry-run"},
	} {
		out, _ := runWith(t, env(home, f), "", args...)
		assertNoneOf(t, strings.Join(args, " "), out, apiKey)
	}
	if w := f.writes(); len(w) != 0 {
		t.Errorf("--dry-run sent %v", w)
	}
}

// A redirect to another origin never carries the key: the kit withholds it and
// the other host receives no credential.
func TestRedirectNeverCarriesTheKey(t *testing.T) {
	f := newFake(t)
	out, _ := runWith(t, env(t.TempDir(), f), "", "customers", "read", redirectID, "--json")
	assertNoneOf(t, "a redirected read", out, apiKey)
	f.assertOthersSawNothing(t)
}

// The two files that may hold a credential are private: a saved key leaves the
// store and the config file 0600.
func TestCredentialFilesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	const stored = "StoredKey-" + "9a1c5e7b3d"
	home := t.TempDir()
	cfg := filepath.Join(home, "private", "config.toml")
	writePrivate(t, cfg, "site = \"acme\"\n")
	if out, code := runWith(t, append(baseEnv(home), "MAXIO_CONFIG="+cfg), stored+"\n", "auth", "set-token"); code != 0 {
		t.Fatalf("auth set-token: exit %d:\n%s", code, out)
	}
	var seen int
	_ = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr == nil && strings.Contains(string(data), stored) {
			seen++
			if m := info.Mode().Perm(); m != 0o600 {
				t.Errorf("%s holds the key with mode %o", path, m)
			}
		}
		return nil
	})
	if seen == 0 {
		t.Fatal("no file holds the saved key; the case did not run")
	}
}

// export-file-closed-on-error: export --output returns after the API refuses,
// with an error, and the request did reach the API.
func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	out, code := runWith(t, env(home, f), "", "export", "invoices-json", "--output", filepath.Join(home, "invoices.jsonl"))
	if code == 0 {
		t.Fatalf("export --output succeeded on a 404:\n%s", out)
	}
	if f.apiHits.Load() == 0 {
		t.Fatalf("export never asked the API; the test did not run:\n%s", out)
	}
}
