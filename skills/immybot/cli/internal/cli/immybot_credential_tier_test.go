// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/cliutil/testenv"
)

// The credential tier (internal/cliutil/immybot_credential_tier.go): GET
// /api/v1/auth/get-azure-tenant-auth-details/{azureTenantPrincipalId} answers
// AzureTenantAuthDetails, and POST /api/v1/auth/update-azure-tenant-auth-details
// echoes it under authDetails; both carry customAppRegSecret, the client secret
// of the tenant's custom Azure app registration. The CLI prints it to the
// terminal that asked and writes it nowhere else.

const (
	credTierPrincipal = "11111111-1111-1111-1111-111111111111"
	// json.Marshal escapes <, & and >, so the JSON-escaped form differs from
	// the plain one; + / = make the base64 forms differ from each other.
	credTierSecret = "credTierSentinel<&>+/=Q9zX"
	// A non-tier read served beside it: proves the scan sees a cache write.
	credTierControl = "credTierControlSentinel7Kq"
)

type credTierStub struct {
	srv  *httptest.Server
	hits atomic.Int32
	seen atomic.Value
}

func newCredTierStub(t *testing.T) *credTierStub {
	t.Helper()
	s := &credTierStub{}
	s.seen.Store("")
	// The spec shapes carry no id, and both store writers (the read
	// write-through and the mutation write-through) skip a row they cannot
	// key, so the secret misses the mirror today only by that accident (the
	// typed access table has a custom_app_reg_secret column). The ids here
	// make both rows keyable, so the tests prove the store refuses them on
	// purpose.
	details := map[string]any{
		"id":                      42,
		"azureTenantPrincipalId":  credTierPrincipal,
		"selectedPermissionLevel": "CustomAppRegistration",
		"customAppRegAppId":       "22222222-2222-2222-2222-222222222222",
		"customAppRegSecret":      credTierSecret,
	}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.seen.Store(r.Method + " " + r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		var body any
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/auth/get-azure-tenant-auth-details/"):
			body = details
		case r.URL.Path == "/api/v1/auth/update-azure-tenant-auth-details":
			body = map[string]any{"id": 42, "authDetails": details, "defaultAppRegistrationUpdated": false}
		case r.URL.Path == "/api/v1/auth/get-ip-addresses":
			body = []any{map[string]any{"id": 1, "ipAddress": credTierControl}}
		case r.URL.Path == "/api/v1/auth":
			body = map[string]any{"userId": 7, "tenantId": 1, "displayName": credTierControl}
		default:
			w.WriteHeader(http.StatusNotFound)
			body = map[string]any{"error": "not found"}
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// credTierHome isolates every user directory under one temp home and points
// the CLI at the stub with a pre-minted token.
func credTierHome(t *testing.T, baseURL string) string {
	t.Helper()
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	cfgPath := filepath.Join(home, "config.toml")
	cfg := fmt.Sprintf("base_url = %q\naccess_token = %q\ntoken_expiry = 2099-01-01T00:00:00Z\n", baseURL, "cred-tier-test-token")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMMYBOT_CONFIG", cfgPath)
	t.Setenv("IMMYBOT_BASE_URL", baseURL)
	for _, k := range []string{"IMMYBOT_CLIENT_ID", "IMMYBOT_CLIENT_SECRET", "IMMYBOT_TENANT_ID", "IMMYBOT_SUBDOMAIN", "IMMYBOT_TOKEN_URL", "IMMYBOT_NO_LOCAL_DATA", "PRINTING_PRESS_VERIFY", cliutil.MCPChildEnvVar} {
		t.Setenv(k, "")
	}
	return home
}

func credTierRun(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := RootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}

// credTierForms is every encoding a writer could plausibly use for value.
func credTierForms(value string) map[string][]byte {
	raw := []byte(value)
	escaped, _ := json.Marshal(value)
	return map[string][]byte{
		"plain":          raw,
		"json-escaped":   bytes.Trim(escaped, `"`),
		"base64":         []byte(base64.StdEncoding.EncodeToString(raw)),
		"base64-raw":     []byte(base64.RawStdEncoding.EncodeToString(raw)),
		"base64-url":     []byte(base64.URLEncoding.EncodeToString(raw)),
		"base64-url-raw": []byte(base64.RawURLEncoding.EncodeToString(raw)),
		"hex":            []byte(hex.EncodeToString(raw)),
		"hex-upper":      []byte(strings.ToUpper(hex.EncodeToString(raw))),
	}
}

// credTierFilesHolding lists every file under root holding value in any form.
func credTierFilesHolding(t *testing.T, root, value string) []string {
	t.Helper()
	forms := credTierForms(value)
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for name, form := range forms {
			if bytes.Contains(data, form) {
				rel, _ := filepath.Rel(root, path)
				found = append(found, rel+" ("+name+")")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return found
}

func credTierPrinted(stdout string) bool {
	for _, form := range credTierForms(credTierSecret) {
		if strings.Contains(stdout, string(form)) {
			return true
		}
	}
	return false
}

// TestCredentialTierGetPrintsButNeverCachesOrStores: the GET prints the
// secret to the terminal that asked, but neither the response cache nor the
// local mirror receives it, in any encoding, on any data source. A second run
// reaches the vendor again (nothing was served from a cache). A control read
// on an ordinary endpoint does land in the cache, so the scan can see one.
func TestCredentialTierGetPrintsButNeverCachesOrStores(t *testing.T) {
	stub := newCredTierStub(t)
	home := credTierHome(t, stub.srv.URL)

	runs := [][]string{
		{"access", "get-get-azure-tenant-auth-details-by-azure-tenant-principal-id", credTierPrincipal, "--json"},
		{"access", "get-get-azure-tenant-auth-details-by-azure-tenant-principal-id", credTierPrincipal},
		{"access", "get-get-azure-tenant-auth-details-by-azure-tenant-principal-id", credTierPrincipal, "--data-source", "live", "--json"},
		{"access", "get", credTierPrincipal, "--agent"},
	}
	for i, args := range runs {
		before := stub.hits.Load()
		stdout, stderr, err := credTierRun(t, args...)
		if err != nil {
			t.Fatalf("%v: %v (stderr=%s)", args, err, stderr)
		}
		if !credTierPrinted(stdout) {
			t.Errorf("%v: the secret was not printed to the terminal that asked: %s", args, stdout)
		}
		if stub.hits.Load() == before {
			t.Errorf("run %d %v was answered without reaching the vendor: a cached credential was served", i, args)
		}
	}
	if found := credTierFilesHolding(t, home, credTierSecret); len(found) > 0 {
		t.Errorf("the credential reached disk: %v", found)
	}

	if _, stderr, err := credTierRun(t, "access", "get-get-ip-addresses", "--json"); err != nil {
		t.Fatalf("control read: %v (stderr=%s)", err, stderr)
	}
	if found := credTierFilesHolding(t, home, credTierControl); len(found) == 0 {
		t.Fatal("control: an ordinary GET left nothing on disk, so the credential scan proves nothing")
	}
}

// TestCredentialTierUpdatePrintsButNeverStores: the update echoes the stored
// details, secret included. The CLI prints the echo and the mutation
// write-through never files it in the mirror.
func TestCredentialTierUpdatePrintsButNeverStores(t *testing.T) {
	stub := newCredTierStub(t)
	home := credTierHome(t, stub.srv.URL)

	stdout, stderr, err := credTierRun(t, "access", "create-update-azure-tenant-auth-details",
		"--auth-details-azure-tenant-principal-id", credTierPrincipal,
		"--auth-details-selected-permission-level", "CustomAppRegistration", "--json")
	if err != nil {
		t.Fatalf("update: %v (stderr=%s)", err, stderr)
	}
	if stub.hits.Load() == 0 {
		t.Fatal("update did not reach the vendor")
	}
	if !credTierPrinted(stdout) {
		t.Errorf("the echoed secret was not printed to the terminal that asked: %s", stdout)
	}
	if found := credTierFilesHolding(t, home, credTierSecret); len(found) > 0 {
		t.Errorf("the credential reached disk: %v", found)
	}
}

// TestCredentialTierRefusedInMCPSpawnedCLI: a CLI process the MCP server
// spawned (the cobratree mirror and recipe intents run one with
// IMMYBOT_MCP_CHILD=1) refuses every route onto a tier endpoint before a
// request, naming the terminal command. `export access <id>` is a mirrored
// tool that reads the same path through resourceDetailPaths. The control
// export of an ordinary resource still reaches the vendor from that process.
func TestCredentialTierRefusedInMCPSpawnedCLI(t *testing.T) {
	stub := newCredTierStub(t)
	credTierHome(t, stub.srv.URL)
	t.Setenv(cliutil.MCPChildEnvVar, "1")

	const getCmd = "immybot-cli access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id"
	refused := []struct {
		args []string
		want string
	}{
		{[]string{"access", "get-get-azure-tenant-auth-details-by-azure-tenant-principal-id", credTierPrincipal, "--json"}, getCmd},
		{[]string{"access", "create-update-azure-tenant-auth-details", "--auth-details-azure-tenant-principal-id", credTierPrincipal, "--json"}, "immybot-cli access create-update-azure-tenant-auth-details"},
		{[]string{"export", "access", credTierPrincipal}, getCmd},
	}
	for _, tc := range refused {
		stdout, stderr, err := credTierRun(t, tc.args...)
		if err == nil {
			t.Errorf("%v ran inside an MCP-spawned CLI process (stdout=%s)", tc.args, stdout)
			continue
		}
		msg := err.Error() + stdout + stderr
		if !strings.Contains(msg, tc.want) {
			t.Errorf("%v: refusal does not name %q: %s", tc.args, tc.want, msg)
		}
		if strings.Contains(msg, credTierSecret) {
			t.Errorf("%v: refusal carries the secret: %s", tc.args, msg)
		}
	}
	if n := stub.hits.Load(); n != 0 {
		t.Errorf("an MCP-spawned CLI process reached a credential endpoint %d time(s) (last %v)", n, stub.seen.Load())
	}

	if _, stderr, err := credTierRun(t, "export", "access"); err != nil {
		t.Fatalf("control export: %v (stderr=%s)", err, stderr)
	}
	if stub.hits.Load() == 0 {
		t.Fatal("control: an ordinary export from the MCP child reached nothing, so the refusal proves nothing")
	}
}

// TestCredentialTierCommandsAreHiddenFromMCP: both tier commands carry
// mcp:hidden, so neither the cobratree mirror nor a recipe intent can expose
// them, independent of the pp:endpoint classification that keeps endpoint
// commands out of the mirror today.
func TestCredentialTierCommandsAreHiddenFromMCP(t *testing.T) {
	tier := map[string]bool{
		"/api/v1/auth/get-azure-tenant-auth-details/{azureTenantPrincipalId}": true,
		"/api/v1/auth/update-azure-tenant-auth-details":                       true,
	}
	found := 0
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			walk(child)
		}
		if !tier[cmd.Annotations["pp:path"]] {
			return
		}
		found++
		if cmd.Annotations["mcp:hidden"] != "true" {
			t.Errorf("%q is a credential-tier command without mcp:hidden", cmd.CommandPath())
		}
	}
	walk(RootCmd())
	if found != 2 {
		t.Errorf("found %d credential-tier commands, want 2", found)
	}
}

// The tier's answer reaches the terminal that asked and nothing else (round-1
// next-batch review): --deliver to a file or a webhook, and `export access
// <id>` (another command reaching the member), are refused before the
// secret is fetched.
func TestCredentialTierReachesNoSinkButTheTerminal(t *testing.T) {
	stub := newCredTierStub(t)
	home := credTierHome(t, stub.srv.URL)
	var hooked atomic.Int32
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hooked.Add(1) }))
	t.Cleanup(hook.Close)
	get := []string{"access", "get-get-azure-tenant-auth-details-by-azure-tenant-principal-id", credTierPrincipal, "--json"}
	for name, args := range map[string][]string{
		"--deliver webhook": append(append([]string{}, get...), "--deliver", "webhook:"+hook.URL+"/in"),
		"--deliver file":    append(append([]string{}, get...), "--deliver", "file:"+filepath.Join(home, "tier-out.json")),
		"export access":     {"export", "access", credTierPrincipal},
	} {
		before := stub.hits.Load()
		stdout, _, err := credTierRun(t, args...)
		if err == nil || stub.hits.Load() != before || credTierPrinted(stdout) {
			t.Errorf("%s: not refused before the request (err %v, vendor reached %d times)", name, err, stub.hits.Load()-before)
		}
	}
	if hooked.Load() != 0 {
		t.Errorf("the webhook received the tier's answer %d times", hooked.Load())
	}
	if found := credTierFilesHolding(t, home, credTierSecret); len(found) > 0 {
		t.Errorf("the credential reached disk: %v", found)
	}
}
