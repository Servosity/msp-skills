// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package exoblackbox

// The credential classes the reviews named on earlier adopters, each from its failing
// input, driven through the built binaries wired to the shared credential kit
// (handfixes.json credkit-wiring).

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// honestEnv is an API that echoes nothing but what routes names, by cmdlet.
func honestEnv(t *testing.T, routes map[string]string, extra ...string) (*server, *bbEnv) {
	t.Helper()
	sv := echoServer(t)
	sv.honest.Store(true)
	sv.routes = routes
	return sv, newEnv(t, sv, true).with(extra...)
}

// A realistic Get-Mailbox and Get-InboxRule answer survives whole on every surface:
// ordinary identifiers (mailbox names, GUIDs, UPNs, an Identity) are not held.
func TestRealisticListingsSurviveUnwithheld(t *testing.T) {
	const mailbox = `{"@odata.context":"https://outlook.office365.com/adminapi/beta/t/$metadata#Mailbox","value":[{` +
		`"Identity":"Rosa Diaz","Name":"Rosa Diaz","DisplayName":"Rosa Diaz","Alias":"rosa","UserPrincipalName":"rosa@contoso.example",` +
		`"PrimarySmtpAddress":"rosa@contoso.example","EmailAddresses":["SMTP:rosa@contoso.example","smtp:r.diaz@contoso.example"],` +
		`"Guid":"8f7c2f4e-3a61-4b1d-9d7a-1e2f3a4b5c6d","ExchangeGuid":"1d2c3b4a-5e6f-4a7b-8c9d-0e1f2a3b4c5d",` +
		`"ExternalDirectoryObjectId":"5b6c7d8e-9f0a-4b1c-8d2e-3f4a5b6c7d8e","RecipientTypeDetails":"UserMailbox",` +
		`"ForwardingSmtpAddress":null,"DeliverToMailboxAndForward":false,"HiddenFromAddressListsEnabled":false,` +
		`"LitigationHoldEnabled":true,"AccountDisabled":false,"WhenChanged":"2026-10-20T10:00:00Z","WhenCreated":"2023-03-02T08:30:00Z",` +
		`"ProhibitSendQuota":"49 GB (52,613,349,376 bytes)","UsageLocation":"United States"}]}`
	const rule = `{"value":[{"Identity":"Rosa Diaz\\7340032991","RuleIdentity":"7340032991","Name":"Move invoices","Enabled":true,"Priority":1,` +
		`"From":["ap@vendor.example"],"SubjectContainsWords":["invoice"],"MoveToFolder":"Rosa Diaz:\\Finance","ForwardTo":null,` +
		`"RedirectTo":null,"DeleteMessage":false,"StopProcessingRules":true,"MailboxOwnerId":"Rosa Diaz",` +
		`"Description":"If the message: includes 'invoice' in the subject Take the following actions: move the message to folder 'Finance'"}]}`
	_, e := honestEnv(t, map[string]string{"Get-Mailbox": mailbox, "Get-InboxRule": rule})
	out := e.run(t, "mailboxes", "list", bbTenant, "--json") + e.run(t, "inbox-rules", "list", bbTenant, "--mailbox", "rosa@contoso.example", "--json")
	mcp := e.mcpCalls(t, []map[string]any{
		{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "mailboxes.list", "params": map[string]any{"tenantId": bbTenant}}},
		{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "inbox_rules.list", "params": map[string]any{"tenantId": bbTenant, "Mailbox": "rosa@contoso.example"}}},
	})
	for where, text := range map[string]string{"CLI": out, "MCP": mcp} {
		if strings.Contains(text, bbWithheld) {
			t.Errorf("%s: an ordinary listing was withheld:\n%.1500s", where, text)
		}
		for _, want := range []string{"Rosa Diaz", "rosa@contoso.example", "8f7c2f4e-3a61-4b1d-9d7a-1e2f3a4b5c6d", "Move invoices"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s lost %q:\n%.1500s", where, want, text)
			}
		}
	}
}

// The cmdlet API under the kit: the envelope, the route and X-AnchorMailbox survive
// on both binaries, and the token is sent once, to the configured host.
func TestCmdletTransportUnderTheKit(t *testing.T) {
	var mu sync.Mutex
	type seen struct{ path, cmdlet, anchor, auth string }
	var got []seen
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, seen{r.URL.Path, cmdletOf(body), r.Header.Get("X-AnchorMailbox"), r.Header.Get("Authorization")})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"Identity":"rosa@example.com","Name":"HQ-Rule"}]}`))
	}))
	e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true)
	e.run(t, "inbox-rules", "list", bbTenant, "--mailbox", "rosa@example.com", "--json")
	e.mcpCalls(t, []map[string]any{{"name": "exchange-online_read", "arguments": map[string]any{"endpoint_id": "inbox_rules.list", "params": map[string]any{"tenantId": bbTenant, "Mailbox": "rosa@example.com"}}}})
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("want one request per binary, got %+v", got)
	}
	for _, r := range got {
		if r.path != "/adminapi/beta/"+bbTenant+"/InvokeCommand" || r.cmdlet != "Get-InboxRule" || r.anchor == "" || r.auth != "Bearer "+bbToken {
			t.Errorf("request lost its shape under the kit: %+v", r)
		}
	}
}

// pemFile writes srv's certificate for EXCHANGE_ONLINE_CA_CERT.
func pemFile(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	ca := filepath.Join(shortTemp(t), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return ca
}

// The kit refuses a credential over plain http, and says so.
func TestCredentialIsNotSentOverPlainHTTP(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			hits.Add(1)
		}
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	t.Cleanup(srv.Close)
	e := newEnv(t, &server{url: srv.URL, caFile: ""}, true)
	out := e.run(t, "mailboxes", "list", bbTenant, "--json")
	if hits.Load() != 0 {
		t.Fatalf("the token went over plain http %d time(s):\n%.1000s", hits.Load(), out)
	}
}

// Allowed origins come from the config the run really uses. A discovered default's
// base_url is only held: --config and EXCHANGE_ONLINE_CONFIG win over it (item 8b), so
// the token still reaches the base the program uses.
func TestExplicitConfigWinsOverADiscoveredDefault(t *testing.T) {
	var real atomic.Int32
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+bbToken {
			real.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"Identity":"HQ-Record"}]}`))
	}))
	e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true).without("EXCHANGE_ONLINE_BASE_URL")
	dir := filepath.Join(e.home, ".config", "exchange-online-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("base_url = \"https://discovered.example\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	named := filepath.Join(e.home, "named.toml")
	if err := os.WriteFile(named, []byte(fmt.Sprintf("base_url = %q\n", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{
		e.run(t, "mailboxes", "list", bbTenant, "--json", "--config", named),
		e.with("EXCHANGE_ONLINE_CONFIG="+named).run(t, "mailboxes", "list", bbTenant, "--json"),
	} {
		if !strings.Contains(out, "HQ-Record") {
			t.Errorf("the named config's base was refused:\n%.1000s", out)
		}
	}
	if real.Load() != 2 {
		t.Errorf("want the token at the named base twice, got %d", real.Load())
	}
}

// A store records the base URL its token was issued for. It must not become the base
// credentials may go to: a token named on this run still reaches the run's own base.
func TestAStoreRecordingABaseURLDoesNotMoveTheOrigin(t *testing.T) {
	sv, e := honestEnv(t, nil)
	e.without("EXCHANGE_ONLINE_TOKEN").runIn(t, bbStored+"\n", "auth", "set-token")
	store := findFile(t, e.home, "credentials.toml")
	data, err := os.ReadFile(store)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := []byte("base_url = \"https://issued.example\"\n" + strings.ReplaceAll(string(data), "base_url", "# base_url"))
	if err := os.WriteFile(store, rewritten, 0o600); err != nil {
		t.Fatal(err)
	}
	out := e.run(t, "mailboxes", "list", bbTenant, "--json")
	if !strings.Contains(out, "HQ-Record") || sv.hits.Load() == 0 {
		t.Errorf("a named token was refused at the run's own base:\n%.1000s", out)
	}
}

func findFile(t *testing.T, root, name string) string {
	t.Helper()
	var found string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == name {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatalf("no %s under %s", name, root)
	}
	return found
}

// stored-credentials-beat-named: a token named on this run wins over a stored one, on
// a write, on both binaries, including a stale auth_header in the config file.
func TestNamedTokenWinsOverAStoredOne(t *testing.T) {
	var mu sync.Mutex
	var writes []string
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.HasPrefix(cmdletOf(body), "Get-") {
			mu.Lock()
			writes = append(writes, r.Header.Get("Authorization"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"Identity":"rosa@example.com"}]}`))
	}))
	e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, false)
	e.runIn(t, bbStored+"\n", "auth", "set-token")
	cfg := filepath.Join(e.home, "stale.toml")
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("auth_header = \"Bearer %s\"\n", bbStored)), 0o600); err != nil {
		t.Fatal(err)
	}
	named := e.with("EXCHANGE_ONLINE_TOKEN=" + bbToken)
	named.run(t, "mailboxes", "set", bbTenant, "--identity", "rosa@example.com", "--type", "Shared", "--json")
	named.run(t, "mailboxes", "set", bbTenant, "--identity", "rosa@example.com", "--type", "Shared", "--json", "--config", cfg)
	named.mcpCalls(t, []map[string]any{{"name": "exchange-online_execute", "arguments": map[string]any{"endpoint_id": "mailboxes.set", "params": map[string]any{"tenantId": bbTenant, "Identity": "rosa@example.com", "Type": "Shared"}, "confirm": true}}})
	mu.Lock()
	defer mu.Unlock()
	if len(writes) != 3 {
		t.Fatalf("want three writes, got %q", writes)
	}
	for i, got := range writes {
		if got != "Bearer "+bbToken {
			t.Errorf("write %d carried the stored token, not the named one", i)
		}
	}
}

// stdin-held-late: a --stdin body's credential field and a piped set-token value are
// held before the parser can repeat them; an ordinary body still works, pretty-printed
// or one line, previews included, and the word set-token alone reads no pipe.
func TestStdinHeldBeforeTheFirstByte(t *testing.T) {
	_, e := honestEnv(t, nil)
	const secret = "PipedSecret" + "Xq71Lm"
	for _, body := range []string{
		fmt.Sprintf(`{"Identity":"rosa@example.com","Password":%q}`, secret),
		fmt.Sprintf("{\n  \"Identity\": \"rosa@example.com\",\n  \"Password\": %q\n}\n", secret),
	} {
		out := e.runIn(t, body, "mailboxes", "set", bbTenant, "--stdin", "--"+secret)
		if strings.Contains(out, secret) {
			t.Errorf("the parser repeated a --stdin credential:\n%.800s", out)
		}
		if preview := e.runIn(t, body, "mailboxes", "set", bbTenant, "--stdin", "--dry-run", "--json"); strings.Contains(preview, secret) {
			t.Errorf("a preview printed the credential:\n%.800s", preview)
		}
	}
	for _, body := range []string{
		`{"Identity":"rosa@example.com","DisplayName":"Rosa Diaz"}`,
		"{\n  \"Identity\": \"rosa@example.com\",\n  \"DisplayName\": \"Rosa Diaz\"\n}\n",
	} {
		preview := e.runIn(t, body, "mailboxes", "set", bbTenant, "--stdin", "--dry-run", "--json")
		if strings.Contains(preview, bbWithheld) || !strings.Contains(preview, "Rosa Diaz") {
			t.Errorf("the preview of an ordinary body was lost:\n%.800s", preview)
		}
	}
	out := e.runIn(t, secret+"\n", "auth", "set-token", "--"+secret)
	if strings.Contains(out, secret) {
		t.Errorf("the parser repeated a piped token:\n%.800s", out)
	}
	cmd := exec.Command(bbCLI, "mailboxes", "list", "set-token", "--help")
	cmd.Env, cmd.Dir = e.env, e.home
	r, w, err := os.Pipe() // a pipe that never closes
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	defer r.Close()
	cmd.Stdin = r
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("a command naming set-token: %v\n%.500s", err, b)
	}
}

// held-non-credential-member: a write's own answer repeats its ordinary parameters,
// and a credential parameter beside them must not withhold them or fail a write that
// went through.
func TestWriteAnswerSurvivesACredentialParameter(t *testing.T) {
	_, e := honestEnv(t, map[string]string{"Set-Mailbox": `{"value":[{"Identity":"rosa@example.com","DisplayName":"Rosa Diaz","Result":"done"}]}`})
	body := `{"Identity":"rosa@example.com","DisplayName":"Rosa Diaz","Password":"WriteParamPw-5k2Q"}`
	out := e.runIn(t, body, "mailboxes", "set", bbTenant, "--stdin", "--json")
	if strings.Contains(out, bbWithheld) || !strings.Contains(out, "Rosa Diaz") {
		t.Errorf("a write's answer was withheld for an ordinary member:\n%.800s", out)
	}
	if strings.Contains(out, "WriteParamPw-5k2Q") {
		t.Errorf("the credential parameter printed:\n%.800s", out)
	}
}

// profiles.json: a run profile keeps no credential-named value, and a URL password in
// any of its fields is held at startup (item 8c).
func TestRunProfileKeepsNoCredential(t *testing.T) {
	_, e := honestEnv(t, nil)
	dir := filepath.Dir(findOrMakeConfigDir(t, e))
	const urlPw = "ProfileUrlPw-Ck7Z"
	store := filepath.Join(dir, "profiles.json")
	raw, _ := json.Marshal(map[string]any{"profiles": map[string]any{"p1": map[string]any{
		"name": "p1", "description": "posts to https://u:" + urlPw + "@hooks.example/x",
		"values": map[string]string{"token": "ProfileFlagPw-9Rw2", "limit": "7"}}}})
	if err := os.WriteFile(store, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out := e.run(t, "profile", "show", "p1", "--json") + e.run(t, "profile", "list", "--json")
	for _, secret := range []string{urlPw, "ProfileFlagPw-9Rw2"} {
		if strings.Contains(out, secret) {
			t.Errorf("a profile credential printed: %s\n%.800s", secret, out)
		}
	}
}

// findOrMakeConfigDir is the CLI's config directory under e.home, with a placeholder
// file so its parent exists.
func findOrMakeConfigDir(t *testing.T, e *bbEnv) string {
	t.Helper()
	dir := filepath.Join(e.home, ".config", "exchange-online-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "config.toml")
}

// The kit's notice replaces a whole unit: stdout stays parseable text, not a partial
// document with the credential cut out.
func TestWithheldUnitIsWholeNotCutUp(t *testing.T) {
	_, e := honestEnv(t, map[string]string{"Get-Mailbox": fmt.Sprintf(`{"value":[{"Identity":"rosa@example.com","Name":%q}]}`, bbToken)})
	var out bytes.Buffer
	cmd := exec.Command(bbCLI, "mailboxes", "list", bbTenant, "--json")
	cmd.Env, cmd.Dir, cmd.Stdout = e.env, e.home, &out
	_ = cmd.Run()
	if strings.Contains(out.String(), bbToken) || strings.Contains(out.String(), "rosa@example.com") {
		t.Errorf("a unit repeating a held value was not withheld whole:\n%.800s", out.String())
	}
}
