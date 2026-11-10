// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package umbrellablackbox

// The credential classes the reviews named, each from its failing input, driven
// through the built binaries wired to the shared credential kit
// (handfixes.json credkit-wiring).

import (
	"bytes"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// honestEnv is an API that echoes nothing but what routes names.
func honestEnv(t *testing.T, routes map[string]string, extra ...string) (*server, *bbEnv) {
	t.Helper()
	sv := echoServer(t)
	sv.honest.Store(true)
	sv.routes = routes
	return sv, newEnv(t, sv, true).with(extra...)
}

const networksPath = "/deployments/v2/networks"

// The base32 of an 8-byte password beside its redacted member: the member is
// replaced by name, the encoded echo withholds the unit.
func TestEncodedPasswordBesideItsMember(t *testing.T) {
	t.Parallel()
	const encoded = "OBQXG43XN5ZGI==="
	_, e := honestEnv(t, map[string]string{networksPath: `[{"originId":1,"password":"password","name":"` + encoded + `"}]`})
	out := e.run(t, "networks", "list", "--json")
	if strings.Contains(out, encoded) {
		t.Fatalf("the base32 of a held password printed:\n%.1000s", out)
	}
	if !strings.Contains(out, bbWithheld) {
		t.Fatalf("want the kit's notice, got:\n%.1000s", out)
	}
}

// A 4-character credential split across adjacent values (no length floor), and
// the same value in every encoding, under a name only the connector's own walker
// recognizes.
func TestShortCredentialSplitOrEncoded(t *testing.T) {
	t.Parallel()
	const pw = "Qz7k"
	a85 := make([]byte, ascii85.MaxEncodedLen(len(pw)))
	a85 = a85[:ascii85.Encode(a85, []byte(pw))]
	var dec, uesc, pct strings.Builder
	for _, c := range []byte(pw) {
		fmt.Fprintf(&dec, "&#%d;", c)
		fmt.Fprintf(&uesc, "\\u%04x", c)
		fmt.Fprintf(&pct, "%%%02X", c)
	}
	forms := map[string]string{
		"split":   `"Qz","7k"`,
		"plain":   `"x ` + pw + ` y"`,
		"base64":  `"` + base64.StdEncoding.EncodeToString([]byte(pw)) + `"`,
		"hex":     `"` + hex.EncodeToString([]byte(pw)) + `"`,
		"base32":  `"` + base32.StdEncoding.EncodeToString([]byte(pw)) + `"`,
		"ascii85": strconv.Quote(string(a85)),
		"percent": `"` + pct.String() + `"`,
		"html":    `"` + dec.String() + `"`,
		"json-u":  `"` + uesc.String() + `"`,
	}
	for name, form := range forms {
		t.Run(name, func(t *testing.T) {
			_, e := honestEnv(t, map[string]string{networksPath: `[{"originId":1,"privateKey":"` + pw + `","notes":[` + form + `]}]`})
			out := e.run(t, "networks", "list", "--json")
			if !strings.Contains(out, bbWithheld) {
				t.Fatalf("%s: want the kit's notice, got:\n%.1000s", name, out)
			}
		})
	}
}

// The control for the class above: the same credential with nothing repeating it
// prints the record, the member replaced by name, so the notice above is the echo's.
func TestShortCredentialAloneKeepsTheRecord(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{networksPath: `[{"originId":1,"privateKey":"Qz7k","notes":["an ordinary note"]}]`})
	out := e.run(t, "networks", "list", "--json")
	if strings.Contains(out, bbWithheld) || !strings.Contains(out, "an ordinary note") || !strings.Contains(out, "[redacted]") {
		t.Fatalf("want the record with its credential replaced, got:\n%.1000s", out)
	}
}

// A rejected mint echoing the key and secret returns instead of hanging (runIn's
// timeout fails a hang), with and without a child org, and the echo never prints.
func TestRejectedMintEchoNeverPrints(t *testing.T) {
	t.Parallel()
	sv, e := honestEnv(t, nil)
	sv.rejectMint.Store(true)
	assertNoEcho(t, "rejected mint", e.run(t, "networks", "list", "--json"))
	assertNoEcho(t, "rejected child mint", e.run(t, "--org", "1234567", "networks", "list", "--json"))
	assertNoEcho(t, "rejected MCP mint", e.mcpCalls(t, []map[string]any{{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "networks.list"}}}))
}

// Class 7: a token the program minted, parent or child org, is held before anything
// can print it, and so is the Authorization header built from it.
func TestMintedTokensAreHeld(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, token string
		args        []string
	}{
		{"parent", bbMinted, []string{"networks", "list", "--json"}},
		{"child org", bbChild, []string{"--org", "1234567", "networks", "list", "--json"}},
		{"the parent's header", "Bearer " + bbMinted, []string{"networks", "list", "--json"}},
		{"the child org's header", "Bearer " + bbChild, []string{"--org", "1234567", "networks", "list", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, e := honestEnv(t, map[string]string{networksPath: `[{"originId":1,"name":"Rec","notes":"` + tc.token + `"}]`})
			out := e.run(t, tc.args...)
			if strings.Contains(out, tc.token) || !strings.Contains(out, bbWithheld) {
				t.Fatalf("want the kit's notice, got:\n%.1000s", out)
			}
		})
	}
}

// A password that is literally "hidden" is not a mask.
func TestPasswordThatReadsLikeAMask(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{networksPath: `[{"originId":1,"password":"hidden","notes":"the word hidden"}]`})
	if out := e.run(t, "networks", "list", "--json"); strings.Contains(out, "the word hidden") {
		t.Fatalf("a password spelled hidden was taken for a mask:\n%.1000s", out)
	}
}

// Every member of a duplicate key is held: encoding/json keeps only the last, so
// a value the echo repeats would go unseen.
func TestDuplicateMemberEchoNeverPrints(t *testing.T) {
	t.Parallel()
	const value = "DupVal-Zq41Wx93"
	_, e := honestEnv(t, map[string]string{networksPath: `[{"originId":1,"privateKey":"` + value + `","privateKey":null,"notes":"` + value + `"}]`})
	if out := e.run(t, "networks", "list", "--json"); strings.Contains(out, value) {
		t.Fatalf("a duplicate member's value printed:\n%.800s", out)
	}
}

// Class startup-credential-before-parse, on every path a config can come from
// (class 2): a value in the file is held before a malformed config is parsed, so
// the parser's error text cannot carry it, on any of the routes Load reads.
func TestStartupHeldBeforeAMalformedConfig(t *testing.T) {
	t.Parallel()
	const inFile = "fileSecretBeforeParse-5Qk2"
	body := []byte("client_secret = \"" + inFile + "\nbroken =")
	sv, e := honestEnv(t, nil)
	write := func(path string) string {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	_ = sv
	cases := map[string]func() string{
		"--config": func() string {
			return e.run(t, "--config", write(filepath.Join(e.home, "a", "bad.toml")), "networks", "list")
		},
		"CISCO_UMBRELLA_CONFIG": func() string {
			return e.with("CISCO_UMBRELLA_CONFIG="+write(filepath.Join(e.home, "b", "bad.toml"))).run(t, "networks", "list")
		},
		"--home": func() string {
			h := filepath.Join(e.home, "c")
			write(filepath.Join(h, "config", "config.toml"))
			return e.run(t, "--home", h, "networks", "list")
		},
		"CISCO_UMBRELLA_HOME": func() string {
			h := filepath.Join(e.home, "d")
			write(filepath.Join(h, "config", "config.toml"))
			return e.with("CISCO_UMBRELLA_HOME="+h).run(t, "networks", "list")
		},
	}
	for name, run := range cases {
		out := run()
		if strings.Contains(out, inFile) {
			t.Errorf("%s: a config value printed from a parse error:\n%.800s", name, out)
		}
	}
	// The environment's secret too.
	bad := write(filepath.Join(e.home, "e", "bad.toml"))
	if err := os.WriteFile(bad, []byte("client_secret = \""+bbSecret+"\nbroken ="), 0o600); err != nil {
		t.Fatal(err)
	}
	assertNoEcho(t, "malformed config", e.run(t, "--config", bad, "networks", "list"))
}

// Class 7: the credential reaches the configured API each way the program can be
// pointed at one, and never another host. A token request carries both the key and
// the secret, on Basic, to the token URL.
func TestCredentialReachesTheConfiguredHostOnly(t *testing.T) {
	t.Parallel()
	sv, e := honestEnv(t, nil)
	cfg := filepath.Join(e.home, "pinned.toml")
	if err := os.WriteFile(cfg, []byte("base_url = \""+sv.url+"\"\ntoken_url = \""+sv.url+"/auth/v2/token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]func() string{
		"CISCO_UMBRELLA_BASE_URL": func() string { return e.run(t, "networks", "list", "--json") },
		"config file": func() string {
			return e.without("CISCO_UMBRELLA_BASE_URL", "CISCO_UMBRELLA_TOKEN_URL").run(t, "--config", cfg, "networks", "list", "--json")
		},
		"child org": func() string { return e.run(t, "--org", "1234567", "networks", "list", "--json") },
	} {
		before := sv.hits.Load()
		out := run()
		if sv.hits.Load() == before || !strings.Contains(out, "HQ-Network") {
			t.Errorf("%s: want the record, got:\n%.1000s", name, out)
		}
		if u, p := sv.mintBasic(); u != bbKey || p != bbSecret {
			t.Errorf("%s: the token request carried Basic %q / %q", name, u, p)
		}
		if sv.off() != 0 {
			t.Errorf("%s: another host received a request", name)
		}
	}
}

// Class credential-sent-to-another-host, the token exchange: a token URL on a host
// nobody declared gets no request, the key and secret stay home, whichever way
// they were given; a same-host token URL that redirects away is refused too.
func TestTokenURLNeverLeavesTheDeclaredHosts(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	for _, c := range []struct {
		env  []string
		args []string
	}{
		{[]string{"CISCO_UMBRELLA_TOKEN_URL=" + sv.otherURL + "/auth/v2/token"}, []string{"auth", "login"}},
		{[]string{"CISCO_UMBRELLA_TOKEN_URL=" + sv.otherURL + "/auth/v2/token"}, []string{"auth", "login", "--client-id", bbKey, "--client-secret", bbSecret}},
		{[]string{"CISCO_UMBRELLA_TOKEN_URL=" + sv.otherURL + "/auth/v2/token"}, []string{"networks", "list", "--json"}},
		{[]string{"CISCO_UMBRELLA_TOKEN_URL=" + sv.url + "/deployments/v2/internaldomains"}, []string{"auth", "login"}},
		{[]string{"CISCO_UMBRELLA_TOKEN_URL=" + sv.url + "/deployments/v2/internaldomains"}, []string{"networks", "list", "--json"}},
	} {
		out := e.with(c.env...).run(t, c.args...)
		if n := sv.off(); n != 0 {
			t.Fatalf("%v with %v: another host received %d request(s):\n%.1500s", c.args, c.env, n, out)
		}
		assertNoEcho(t, "token URL run output", out)
	}
}

// Class credential-sent-to-another-host, plain http: neither a saved token nor the
// key and secret are sent in the clear.
func TestCredentialIsNotSentOverPlainHTTP(t *testing.T) {
	t.Parallel()
	var carried, hits atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		seen := fmt.Sprint(r.Header) + string(body)
		if u, p, ok := r.BasicAuth(); ok {
			seen += u + p
		}
		for _, s := range secrets() {
			if strings.Contains(seen, s) {
				carried.Add(1)
			}
		}
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(plain.Close)
	_, e := honestEnv(t, nil)
	e.runIn(t, bbMinted+"\n", "auth", "set-token")
	saved := e.without("CISCO_UMBRELLA_API_KEY", "CISCO_UMBRELLA_API_SECRET", "CISCO_UMBRELLA_BASE_URL", "CISCO_UMBRELLA_TOKEN_URL").
		with("CISCO_UMBRELLA_BASE_URL="+plain.URL, "CISCO_UMBRELLA_TOKEN_URL="+plain.URL+"/auth/v2/token")
	out := saved.run(t, "networks", "list", "--json")
	if carried.Load() != 0 {
		t.Fatalf("the saved token went over plain http %d time(s):\n%.800s", carried.Load(), out)
	}
	env := e.without("CISCO_UMBRELLA_BASE_URL", "CISCO_UMBRELLA_TOKEN_URL").
		with("CISCO_UMBRELLA_BASE_URL="+plain.URL, "CISCO_UMBRELLA_TOKEN_URL="+plain.URL+"/auth/v2/token")
	for _, args := range [][]string{{"networks", "list", "--json"}, {"auth", "login"}} {
		out = env.run(t, args...)
		if carried.Load() != 0 {
			t.Fatalf("%v: the key and secret went over plain http %d time(s):\n%.800s", args, carried.Load(), out)
		}
	}
}

// An MCP write's preview shows its arguments, so a credential at any depth under a
// name no declaration lists is held before it runs.
func TestMCPPreviewHoldsANestedCredential(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	out := e.mcpCalls(t, []map[string]any{{"name": "cisco-umbrella_execute", "arguments": map[string]any{
		"endpoint_id": "users.create",
		"params":      map[string]any{"email": "u@example.com", "settings": map[string]any{"privateKey": bbMCPPass}},
	}}})
	assertNoEcho(t, "MCP preview", out)
}

// An API key record is ordinary data around two credentials: the listing and the
// wrapped record stay readable, with those members replaced by name, and a repeat
// of one anywhere withholds the unit.
func TestAPIKeyRecordsStayReadable(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	out := e.run(t, "api-keys", "list", "--json")
	if strings.Contains(out, bbListedKey) || strings.Contains(out, bbWithheld) || !strings.Contains(out, "Networks RW") || !strings.Contains(out, "[redacted]") {
		t.Fatalf("api-keys list: want the record with its clientId replaced:\n%.1000s", out)
	}
	const client, secret = "wrappedHalfOne-" + "5e7a1c", "wrappedHalfTwo-" + "9d3b2f"
	_, e = honestEnv(t, map[string]string{"/admin/v2/apiKeys/k9": `{"key":{"id":"k9","name":"Reports RO","clientId":"` + client + `","clientSecret":"` + secret + `"}}`})
	out = e.run(t, "api-keys", "get", "k9", "--json")
	if strings.Contains(out, client) || strings.Contains(out, secret) || strings.Contains(out, bbWithheld) || !strings.Contains(out, "Reports RO") {
		t.Fatalf("api-keys get: want the wrapped record with its credentials replaced:\n%.1000s", out)
	}
	_, e = honestEnv(t, map[string]string{"/admin/v2/apiKeys/k9": `{"id":"k9","name":"Reports RO","key":"` + secret + `"}`})
	out = e.run(t, "api-keys", "get", "k9", "--json")
	if strings.Contains(out, secret) {
		t.Fatalf("api-keys get: a string-valued key printed:\n%.1000s", out)
	}
}

// The S3 bucket key's IDs are credentials by name, whatever their shape; a repeat
// of one takes the whole unit with it.
func TestS3BucketKeyIDsHeldByName(t *testing.T) {
	t.Parallel()
	const current, old = "rotatedHalfOne-3a7d91", "rotatedHalfTwo-9f1c44"
	body := `{"oldKeyId":"` + old + `","currentKeyId":"` + current + `","secretAccessKey":"` + bbS3Secret + `","keyCreationDate":"2025-03-12T22:00:00"}`
	_, e := honestEnv(t, map[string]string{networksPath: body})
	out := e.run(t, "networks", "list", "--data-source", "live", "--json")
	if strings.Contains(out, current) || strings.Contains(out, old) || strings.Contains(out, bbS3Secret) || !strings.Contains(out, "2025-03-12") {
		t.Fatalf("want the record with its keys replaced:\n%.1000s", out)
	}
	_, e = honestEnv(t, map[string]string{networksPath: `{"currentKeyId":"` + current + `","note":"rotated to ` + current + `"}`})
	if out := e.run(t, "networks", "list", "--data-source", "live", "--json"); strings.Contains(out, current) || !strings.Contains(out, bbWithheld) {
		t.Fatalf("want the kit's notice for a repeated key id:\n%.1000s", out)
	}
}

// Class 3: every nonempty value of a credential flag is held however it is spelled
// and whatever it is made of, before anything can print it.
func TestCredentialFlagValuesAreHeldHoweverSpelled(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	for _, c := range []struct{ value, flag string }{
		{"flagEqualsValue-7d1e", "--password=flagEqualsValue-7d1e"},
		{"-leadingDash-3c9a", "--password=-leadingDash-3c9a"},
		{"-----", "--password=-----"},
	} {
		out := e.run(t, "users", "create", "--email", "a@example.com", c.flag, "--role-id", "2", "--dry-run", "--json")
		if strings.Contains(out, c.value) {
			t.Errorf("%s printed:\n%.800s", c.flag, out)
		}
		// A malformed line prints a usage error, before any command runs.
		out = e.run(t, "users", "create", "--email", "a@example.com", c.flag, "--bogus-flag")
		if strings.Contains(out, c.value) {
			t.Errorf("%s printed in a usage error:\n%.800s", c.flag, out)
		}
	}
	out := e.run(t, "users", "create", "--email", "a@example.com", "--password", "-----", "--role-id", "2", "--dry-run", "--json")
	if strings.Contains(out, "-----") {
		t.Errorf("--password ----- printed:\n%.800s", out)
	}
}

// Class 3, where only the argv hold knows the value: a token endpoint's refusal
// that names the client it was sent. Every spelling and every character class is
// held; the control proves the refusal's text does print.
func TestCredentialFlagValueEchoedByTheTokenEndpoint(t *testing.T) {
	t.Parallel()
	refusal := func(client string) map[string]string {
		return map[string]string{"/auth/v2/token": `{"error":"invalid_client","error_description":"unknown client ` + client + `"}`}
	}
	for _, v := range []string{"flagEqualsValue-7d1e", "-leadingDash-3c9a", "-----", "---", "Ab1"} {
		sv, e := honestEnv(t, refusal(v))
		sv.routeStatus.Store(http.StatusUnauthorized)
		if out := e.run(t, "auth", "login", "--client-id="+v, "--client-secret", "flagSecretValue-1c9e"); strings.Contains(out, v) {
			t.Errorf("--client-id=%s printed:\n%.800s", v, out)
		}
	}
	const control = "notOnTheCommandLine-5b2f"
	sv, e := honestEnv(t, refusal(control))
	sv.routeStatus.Store(http.StatusUnauthorized)
	if out := e.run(t, "auth", "login", "--client-id", "someOtherClient-8a1d", "--client-secret", "flagSecretValue-1c9e"); !strings.Contains(out, control) {
		t.Fatalf("the refusal's text does not print, so this test proves nothing:\n%.800s", out)
	}
}

// The learn journal's session key, its verb chain and its flag names reach the
// journal file through the kit: an entry carrying a held value is not written,
// and an ordinary one is.
func TestJournalNeverHoldsAHeldValue(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	e.with("CISCO_UMBRELLA_LEARN_SESSION="+bbSecret).run(t, "networks", "list", "--json")
	e.run(t, "networks", "list", "--"+bbSecret)
	readJournal := func() string {
		var all bytes.Buffer
		_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && filepath.Base(filepath.Dir(path)) == "learn" {
				if b, rerr := os.ReadFile(path); rerr == nil {
					all.Write(b)
				}
			}
			return nil
		})
		return all.String()
	}
	assertNoEcho(t, "learn journal", readJournal())
	e.with("CISCO_UMBRELLA_LEARN_SESSION=ordinary-session").run(t, "networks", "list", "--json")
	if got := readJournal(); !strings.Contains(got, "ordinary-session") {
		t.Fatalf("an ordinary run wrote no journal entry, so this test proved nothing:\n%.800s", got)
	}
}

// Class store-write-bypasses-chokepoint, its failing input: teach --notes with a
// declared secret reached search_learnings. The statement is refused; an ordinary
// note lands. A credential no source declared is out of contract.
func TestTeachNoteWithADeclaredSecretIsNotStored(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	e.run(t, "teach", "--query", "find networks", "--resource", "123", "--resource-type", "networks", "--notes", "rotated "+bbSecret, "--no-validate")
	e.run(t, "teach", "--query", "find networks", "--resource", "124", "--resource-type", "networks", "--notes", "rotated on schedule", "--no-validate")
	saw := false
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil {
				if !synccpuStoreFile(path) { // synccpu: the local store is not checked
					assertNoEcho(t, d.Name(), string(b))
				}
				saw = saw || bytes.Contains(b, []byte("rotated on schedule"))
			}
		}
		return nil
	})
	if !saw {
		t.Fatal("the ordinary note was not stored, so this test proved nothing")
	}
}

// The MCP server hands a mirrored child the key and secret on stdin: the kit starts
// no child with a credential in its environment.
func TestMCPChildGetsTheCredentials(t *testing.T) {
	t.Parallel()
	sv, e := honestEnv(t, nil)
	out := e.mcpCalls(t, []map[string]any{{"name": "fleet_health", "arguments": map[string]any{}}})
	if u, p := sv.mintBasic(); u != bbKey || p != bbSecret {
		t.Fatalf("the child minted with Basic %q / %q:\n%.1500s", u, p, out)
	}
	assertNoEcho(t, "fleet_health", out)
}

// Class preview-sends-anyway: under --dry-run nothing reaches the API (token
// endpoint included) and no credential is saved.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	kept := filepath.Join(e.home, "kept.jsonl")
	if err := os.WriteFile(kept, []byte("an earlier export\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := sv.hits.Load()
	for _, args := range [][]string{
		{"auth", "login", "--dry-run"},
		{"auth", "login", "--client-id", bbLoginKey, "--client-secret", bbLoginSec, "--dry-run"},
		{"networks", "list", "--json", "--dry-run"},
		{"--org", "1234567", "networks", "list", "--dry-run"},
		{"api-keys", "create", "--name", "bb", "--scopes", `["reports.aggregations:read"]`, "--dry-run"},
		{"users", "create", "--email", "a@example.com", "--password", bbPassword, "--role-id", "2", "--json", "--dry-run"},
		{"sync", "--resources", "networks", "--dry-run"},
		{"export", "networks", "-o", filepath.Join(e.home, "dry.jsonl"), "--dry-run"},
		{"export", "networks", "-o", kept, "--dry-run"},
		{"doctor", "--dry-run"},
		{"fleet", "health", "--dry-run"},
	} {
		out := e.run(t, args...)
		if n := sv.hits.Load() - before; n != 0 {
			t.Errorf("%v sent %d request(s) under --dry-run:\n%.600s", args, n, out)
		}
		before = sv.hits.Load()
	}
	e.runIn(t, bbMinted+"\n", "auth", "set-token", "--dry-run")
	if _, err := os.Stat(filepath.Join(e.home, "dry.jsonl")); err == nil {
		t.Errorf("export --dry-run created its output file")
	}
	if b, err := os.ReadFile(kept); err != nil || string(b) != "an earlier export\n" {
		t.Errorf("export --dry-run changed an existing file: %q (%v)", b, err)
	}
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "credentials") {
			return nil
		}
		if b, rerr := os.ReadFile(path); rerr == nil && (bytes.Contains(b, []byte(bbMinted)) || bytes.Contains(b, []byte(bbLoginSec))) {
			t.Errorf("a --dry-run saved a credential in %s", path)
		}
		return nil
	})
}

// Class credential-sent-to-another-host, outbound uploads: a feedback upload passes
// the kit like every other request: one carrying a held credential never leaves,
// and an ordinary one does.
func TestFeedbackUploadCarriesNoCredential(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var got bytes.Buffer
	posts := 0
	fb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got.Write(b)
		posts++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(fb.Close)
	_, e := honestEnv(t, nil)
	send := e.with("CISCO_UMBRELLA_FEEDBACK_ENDPOINT=" + fb.URL + "/fb")
	out := send.run(t, "feedback", "--send", "noted the key "+bbSecret+" in a ticket")
	out2 := send.run(t, "feedback", "--send", "an ordinary note about networks")
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 || !strings.Contains(got.String(), `"cli":"cisco-umbrella-cli"`) {
		t.Fatalf("want exactly the ordinary upload, got %d: %q\n%.600s\n%.600s", posts, got.String(), out, out2)
	}
	assertNoEcho(t, "feedback upload body", got.String())
	assertNoEcho(t, "feedback output", out)
}

// Class write-without-confirm: an MCP write previews and sends nothing without
// confirm: true.
func TestMCPWriteNeedsConfirm(t *testing.T) {
	t.Parallel()
	sv, e := honestEnv(t, nil)
	params := map[string]any{"email": "u@example.com", "firstname": "Rec", "lastname": "Ord", "roleId": 2}
	out := e.mcpCalls(t, []map[string]any{
		{"name": "cisco-umbrella_execute", "arguments": map[string]any{"endpoint_id": "users.create", "params": params}},
		{"name": "cisco-umbrella_execute", "arguments": map[string]any{"endpoint_id": "users.create", "params": params, "confirm": "true"}},
	})
	if sv.writes.Load() != 0 || !strings.Contains(out, "confirmation_required") {
		t.Fatalf("previews sent %d write(s); output:\n%.1500s", sv.writes.Load(), out)
	}
	e.mcpCalls(t, []map[string]any{
		{"name": "cisco-umbrella_execute", "arguments": map[string]any{"endpoint_id": "users.create", "params": params, "confirm": true}},
	})
	if sv.writes.Load() != 1 {
		t.Fatalf("confirm: true sent %d write(s), want 1", sv.writes.Load())
	}
}

// listFiles is every file under dir with its size.
func listFiles(dir string) map[string]int64 {
	got := map[string]int64{}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, ierr := d.Info(); ierr == nil {
				got[path] = info.Size()
			}
		}
		return nil
	})
	return got
}

// Class 4: an unconfirmed MCP preview changes nothing on this machine. It opens no
// writable store and writes no file, and a host-file flag is refused by name
// before anything runs.
func TestMCPPreviewWritesNothing(t *testing.T) {
	t.Parallel()
	sv, e := honestEnv(t, map[string]string{"/policies/v2/destinationlists": `{"status":{"code":200},"data":[{"id":"1","destination":"good.example.com"}]}`})
	// One session first, so the server's own start-up files are the baseline.
	e.mcpCalls(t, []map[string]any{{"name": "context", "arguments": map[string]any{}}})
	before := listFiles(e.home)
	out := e.mcpCalls(t, []map[string]any{
		{"name": "import", "arguments": map[string]any{"resource": "networks"}},
		{"name": "import", "arguments": map[string]any{"resource": "networks", "confirm": "true"}},
		{"name": "import", "arguments": map[string]any{"resource": "networks", "config": filepath.Join(e.home, "x.toml")}},
		{"name": "destination_lists_diff", "arguments": map[string]any{"destinationListId": "11", "entries": "bad.example.com", "apply": true}},
		{"name": "block_one_domain", "arguments": map[string]any{"id": "11", "body_json": `[{"destination":"blocked.example.com"}]`}},
	})
	if !strings.Contains(out, `unknown MCP parameter \"config\"`) {
		t.Fatalf("a host-file flag was not refused by name:\n%.1500s", out)
	}
	for _, want := range []string{"bad.example.com", "/policies/v2/destinationlists/11/destinations", "Preview only"} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %q in the previews, so this test proves something:\n%.1500s", want, out)
		}
	}
	after := listFiles(e.home)
	for path, size := range after {
		if old, ok := before[path]; !ok || old != size {
			t.Errorf("a preview touched %s (%d to %d bytes)", path, old, size)
		}
	}
	if sv.writes.Load() != 0 {
		t.Errorf("a preview sent %d write(s)", sv.writes.Load())
	}
}

// MCP recipe argv: a tool argument never becomes a flag of the mirrored CLI child.
func TestMCPArgumentNeverBecomesAFlag(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	target := filepath.Join(e.home, "mcp-deliver.json")
	out := e.mcpCalls(t, []map[string]any{{"name": "verdict", "arguments": map[string]any{"domain": "--deliver=file:" + target}}})
	if !strings.Contains(out, "flag-like positional argument") {
		t.Fatalf("want the flag-like positional refused, got:\n%.1500s", out)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("a tool argument wrote a file through --deliver")
	}
}
