// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package umbrellablackbox

// Classes the 2026-11-03 review named, each from its failing input, driven through
// the built binaries.

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// credentialStore is the 0600 store's bytes, whichever directory holds it.
func credentialStore(t *testing.T, home string) string {
	t.Helper()
	var all bytes.Buffer
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "credentials.toml" {
			if b, rerr := os.ReadFile(path); rerr == nil {
				all.Write(b)
			}
		}
		return nil
	})
	return all.String()
}

// Class stdin-held-late, contract section 1: a value known at startup, stdin
// included, is held before the first byte of output. The argv parser reads the line
// before `auth set-token` reads the pipe, so its errors are the first output.
func TestPipedTokenIsHeldBeforeAnyOutput(t *testing.T) {
	t.Parallel()
	const tok = "review-secr-77"
	for name, c := range map[string]struct {
		stdin, value string
		args         []string
	}{
		"its failing input":     {tok + "\n", tok, []string{"auth", "set-token", "--" + tok}},
		"no newline":            {tok, tok, []string{"auth", "set-token", "--" + tok}},
		"a Windows line ending": {tok + " \r\n", tok, []string{"auth", "set-token", "--" + tok}},
		"a positional":          {tok + "\n", tok, []string{"auth", "set-token", tok}},
		"a flag's value":        {tok + "\n", tok, []string{"auth", "set-token", "--bogus=" + tok}},
		"flags first":           {tok + "\n", tok, []string{"--bogus=" + tok, "auth", "set-token"}},
		"a later line":          {"first-line-only\n" + tok + "\n", tok, []string{"auth", "set-token", "--" + tok}},
		"a short token":         {"Ab7\n", "Ab7", []string{"auth", "set-token", "--Ab7"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, e := honestEnv(t, nil)
			if out := e.runIn(t, c.stdin, c.args...); strings.Contains(out, c.value) {
				t.Fatalf("a token on stdin printed:\n%.800s", out)
			}
		})
	}
}

// The controls for the class above: the parser's error does print when the pipe does
// not hold the value, blank input is the command's own refusal and not a held value,
// and the bytes reach the command unchanged after the hold reads them.
func TestPipedTokenControls(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	if out := e.runIn(t, "some-other-value\n", "auth", "set-token", "--not-on-stdin"); !strings.Contains(out, "unknown flag: --not-on-stdin") {
		t.Errorf("the parser's error does not print, so the class above proves nothing:\n%.800s", out)
	}
	for _, blank := range []string{"", "\n", " \r\n\n"} {
		if out := e.runIn(t, blank, "auth", "set-token"); !strings.Contains(out, "empty token on stdin") {
			t.Errorf("stdin %q: want the command's own refusal:\n%.800s", blank, out)
		}
	}
	if store := credentialStore(t, e.home); strings.Contains(store, "access_token") {
		t.Errorf("a blank token was saved:\n%s", store)
	}
	const saved = "savedThroughThePipe-7Qk2"
	if out := e.runIn(t, saved+"\n", "auth", "set-token"); !strings.Contains(out, "Token saved") || strings.Contains(out, saved) {
		t.Errorf("auth set-token:\n%.800s", out)
	}
	if store := credentialStore(t, e.home); !strings.Contains(store, saved) {
		t.Errorf("the token did not reach the store through the hold:\n%s", store)
	}
	// Other commands never read stdin ahead of the program. Without the key in the
	// environment, which would win over the saved token (TestNamedKeyBeatsTheStoredOne).
	if out := e.without("CISCO_UMBRELLA_API_KEY", "CISCO_UMBRELLA_API_SECRET").runIn(t, "some-other-value\n", "auth", "status"); !strings.Contains(out, "Credentials present") {
		t.Errorf("auth status:\n%.800s", out)
	}
}

// A stdin that cannot be read saves nothing and refuses: a partial read is not a
// token. A pipe past the command's limit is the command's refusal, read no further.
func TestPipedTokenFailsClosed(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	dir, err := os.Open(e.home)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	cmd := exec.Command(bbCLI, "auth", "set-token")
	cmd.Env, cmd.Dir, cmd.Stdin = e.env, e.home, dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("an unreadable stdin exited 0:\n%.800s", out)
	}
	if store := credentialStore(t, e.home); strings.Contains(store, "access_token") {
		t.Errorf("an unreadable stdin saved a token:\n%s", store)
	}
	big := strings.Repeat("0123456789abcde\n", 1<<16) // 1 MiB, past the 64 KiB a token may be
	started := time.Now()
	if out := e.runIn(t, big, "auth", "set-token"); !strings.Contains(out, "exceeds") && !strings.Contains(out, bbWithheld) {
		t.Errorf("an oversized stdin:\n%.800s", out)
	}
	if store := credentialStore(t, e.home); strings.Contains(store, "access_token") {
		t.Errorf("an oversized stdin saved a token after %v:\n%.200s", time.Since(started), store)
	}
}

// Class raw-file-write-past-kit, its failing input: a response whose declared
// credential is one NUL byte holds that byte, and a lock file's one raw byte then
// stored the whole credential. No file under the home but the store may hold it.
func TestOneByteCredentialReachesNoFile(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{"/admin/v2/users": `{"data":[{"id":1,"password":"\u0000"}]}`})
	e.run(t, "users", "list", "--data-source", "live", "--json")
	var lock bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		lock = lock || strings.HasSuffix(d.Name(), ".lock")
		if strings.Contains(d.Name(), ".db") {
			return nil // the store, whose statements the kit's driver checks
		}
		if b, rerr := os.ReadFile(path); rerr == nil && bytes.Equal(b, []byte{0}) {
			t.Errorf("%s holds exactly the held credential", path)
		}
		return nil
	})
	if !lock {
		t.Fatal("the run took no lock, so this test proved nothing")
	}
}

// bbOwnPSK is a secret the operator chose, which Umbrella need not generate.
const bbOwnPSK = "OwnChosenPsk" + "7Qk2Lx9V"

// bbTunnelPrefix is a tunnel's public id prefix. psk is a credential member, so a
// value in it is held: a realistic prefix, since a one-character one silences output.
const bbTunnelPrefix = "edge-tunnel-prefix"

// oneTimeRefusal is the refusal's fixed text.
const oneTimeRefusal = "Umbrella returns this new secret only once"

// Class one-time-secret-discarded. Umbrella shows a generated secret once and this
// program withholds it (contract section 1), so these requests would destroy the only
// copy and, on a refresh, break a live integration. Refused before anything leaves:
// the stub sees no request at all, the token exchange included.
func TestOneTimeSecretRequestsAreRefused(t *testing.T) {
	t.Parallel()
	cli := []struct {
		name, stdin string
		args        []string
	}{
		{"api-keys create", "", []string{"api-keys", "create", "--name", "bb", "--scopes", `["reports.aggregations:read"]`, "--json"}},
		{"api-keys refresh", "", []string{"api-keys", "refresh", "k1", "--json"}},
		{"api-keys refresh, human output", "", []string{"api-keys", "refresh", "k1"}},
		{"api-keys refresh, a child org", "", []string{"--org", "1234567", "api-keys", "refresh", "k1", "--json"}},
		{"s3-bucket-keys", "", []string{"s3-bucket-keys", "--json"}},
		{"tunnels add, no secret", "", []string{"tunnels", "add", "--name", "t", "--json"}},
		{"tunnels add, an id prefix only", "", []string{"tunnels", "add", "--name", "t", "--authentication-parameters-id-prefix", "x", "--json"}},
		{"tunnels add, a body with no secret", `{"name":"t"}`, []string{"tunnels", "add", "--stdin", "--json"}},
		{"tunnels add, the response's path", `{"name":"t","client":{"authentication":{"parameters":{"secret":"` + bbOwnPSK + `"}}}}`, []string{"tunnels", "add", "--stdin", "--json"}},
		{"update-credentials, auto-rotate", "", []string{"tunnels", "update-credentials", "7", "--auto-rotate", "true", "--json"}},
		{"update-credentials, auto-rotate with a secret", "", []string{"tunnels", "update-credentials", "7", "--auto-rotate", "true", "--psk-id-prefix", bbTunnelPrefix, "--psk-secret", bbOwnPSK, "--json"}},
		{"update-credentials, no secret", "", []string{"tunnels", "update-credentials", "7", "--auto-rotate", "false", "--psk-id-prefix", bbTunnelPrefix, "--json"}},
		{"update-credentials, a body with no autoRotate", `{"psk":{"idPrefix":"` + bbTunnelPrefix + `","secret":"` + bbOwnPSK + `"}}`, []string{"tunnels", "update-credentials", "7", "--stdin", "--json"}},
		{"update-credentials, autoRotate as a string", `{"autoRotate":"false","psk":{"idPrefix":"` + bbTunnelPrefix + `","secret":"` + bbOwnPSK + `"}}`, []string{"tunnels", "update-credentials", "7", "--stdin", "--json"}},
	}
	for _, c := range cli {
		t.Run("CLI "+c.name, func(t *testing.T) {
			t.Parallel()
			sv, e := honestEnv(t, nil)
			out := e.runIn(t, c.stdin, c.args...)
			if !strings.Contains(out, oneTimeRefusal) || !strings.Contains(out, "dashboard") {
				t.Errorf("want the refusal, got:\n%.800s", out)
			}
			if strings.Contains(out, bbOwnPSK) {
				t.Errorf("the refusal repeats the secret:\n%.800s", out)
			}
			if n := sv.hits.Load(); n != 0 {
				t.Errorf("the stub received %d request(s):\n%.800s", n, out)
			}
		})
	}
	t.Run("a refusal exits nonzero", func(t *testing.T) {
		t.Parallel()
		_, e := honestEnv(t, nil)
		cmd := exec.Command(bbCLI, "api-keys", "refresh", "k1", "--json")
		cmd.Env, cmd.Dir = e.env, e.home
		if err := cmd.Run(); err == nil {
			t.Error("exit 0")
		}
	})
	mcp := map[string]map[string]any{
		"api_keys.create":                 {"name": "bb", "scopes": []string{"reports.aggregations:read"}},
		"api_keys.refresh":                {"apiKeyId": "k1"},
		"s3_bucket_keys.create":           {},
		"tunnels.add":                     {"name": "t"},
		"tunnels.update-credentials":      {"id": 7, "autoRotate": true},
		"tunnels.update-credentials#body": {"id": 7, "autoRotate": false, "psk": map[string]any{"idPrefix": bbTunnelPrefix}},
	}
	for id, params := range mcp {
		t.Run("MCP execute "+id, func(t *testing.T) {
			t.Parallel()
			sv, e := honestEnv(t, nil)
			endpoint, _, _ := strings.Cut(id, "#")
			out := e.mcpCalls(t, []map[string]any{{"name": "cisco-umbrella_execute", "arguments": map[string]any{"endpoint_id": endpoint, "params": params, "confirm": true}}})
			if !strings.Contains(out, oneTimeRefusal) || !strings.Contains(out, "dashboard") || !strings.Contains(out, `"isError":true`) {
				t.Errorf("want the refusal as a tool error, got:\n%.1500s", out)
			}
			if n := sv.hits.Load(); n != 0 {
				t.Errorf("the stub received %d request(s):\n%.1500s", n, out)
			}
		})
	}
	// The other way in: a mirrored child and the generic importer run the same client.
	t.Run("the importer", func(t *testing.T) {
		t.Parallel()
		sv, e := honestEnv(t, nil)
		in := filepath.Join(e.home, "rows.jsonl")
		if err := os.WriteFile(in, []byte(`{"name":"t"}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out := e.run(t, "import", "tunnels", "--input", in, "--json")
		if n := sv.writes.Load(); n != 0 {
			t.Errorf("the stub received %d write(s):\n%.800s", n, out)
		}
		if !strings.Contains(out, oneTimeRefusal) {
			t.Errorf("want the refusal, got:\n%.800s", out)
		}
	})
}

// The controls for the class above. A preview sends nothing and stays; so do reads;
// a tunnel secret the operator chose is theirs to keep, so those two requests go out;
// every other write to the same resources is not refused.
func TestOneTimeSecretControls(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"api-keys", "create", "--name", "bb", "--scopes", `["reports.aggregations:read"]`, "--dry-run"},
		{"api-keys", "refresh", "k1", "--dry-run"},
		{"s3-bucket-keys", "--dry-run"},
		{"tunnels", "add", "--name", "t", "--dry-run"},
		{"tunnels", "update-credentials", "7", "--auto-rotate", "true", "--dry-run"},
	} {
		sv, e := honestEnv(t, nil)
		out := e.run(t, args...)
		if strings.Contains(out, oneTimeRefusal) || !strings.Contains(out, "POST") || sv.hits.Load() != 0 {
			t.Errorf("%v: want a preview and no request (%d sent):\n%.800s", args, sv.hits.Load(), out)
		}
	}
	for name, c := range map[string]struct {
		routes map[string]string
		args   []string
	}{
		"tunnels add, own secret": {map[string]string{"/deployments/v2/tunnels": `{"id":7,"name":"t"}`},
			[]string{"tunnels", "add", "--name", "t", "--authentication-parameters-id-prefix", "x", "--authentication-parameters-secret", bbOwnPSK, "--json"}},
		"update-credentials, own secret": {map[string]string{"/deployments/v2/tunnels/7/keys": `{"id":7,"name":"t"}`},
			[]string{"tunnels", "update-credentials", "7", "--auto-rotate", "false", "--psk-id-prefix", bbTunnelPrefix, "--psk-secret", bbOwnPSK, "--json"}},
		"api-keys update": {map[string]string{"/admin/v2/apiKeys/k1": `{"key":{"id":"k1","name":"Renamed"}}`},
			[]string{"api-keys", "update", "k1", "--name", "Renamed", "--scopes", `["reports.aggregations:read"]`, "--json"}},
		"api-keys delete": {map[string]string{"/admin/v2/apiKeys/k1": `{}`},
			[]string{"api-keys", "delete", "k1", "--yes", "--json"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sv, e := honestEnv(t, c.routes)
			out := e.run(t, c.args...)
			if strings.Contains(out, oneTimeRefusal) || sv.writes.Load() != 1 {
				t.Errorf("want one write and no refusal, got %d:\n%.800s", sv.writes.Load(), out)
			}
		})
	}
	sv, e := honestEnv(t, nil)
	out := e.mcpCalls(t, []map[string]any{
		{"name": "cisco-umbrella_execute", "arguments": map[string]any{"endpoint_id": "api_keys.refresh", "params": map[string]any{"apiKeyId": "k1"}}},
		{"name": "cisco-umbrella_execute", "arguments": map[string]any{"endpoint_id": "tunnels.add", "params": map[string]any{"name": "t", "authentication": map[string]any{"parameters": map[string]any{"idPrefix": "x", "secret": bbOwnPSK}}}, "confirm": true}},
	})
	if strings.Count(out, oneTimeRefusal) != 0 || !strings.Contains(out, "confirmation_required") || sv.writes.Load() != 1 {
		t.Errorf("MCP: want a preview and one write with the operator's own secret, got %d:\n%.1500s", sv.writes.Load(), out)
	}
}

// The help text and the MCP search say so, before anyone tries.
func TestOneTimeSecretRefusalIsDocumented(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	for _, args := range [][]string{
		{"api-keys", "create", "--help"},
		{"api-keys", "refresh", "--help"},
		{"s3-bucket-keys", "--help"},
		{"tunnels", "add", "--help"},
		{"tunnels", "update-credentials", "--help"},
	} {
		if out := e.run(t, args...); !strings.Contains(out, "dashboard") {
			t.Errorf("%v does not name the dashboard:\n%.600s", args, out)
		}
	}
	root := filepath.Join("..", "..", "..")
	for _, doc := range []string{"README.md", "governance.md", "guide.md", "SKILL.md", "page.json", "CHANGELOG.md"} {
		b, err := os.ReadFile(filepath.Join(root, doc))
		if err != nil {
			t.Fatal(err)
		}
		if text := string(b); strings.Contains(text, "from the Umbrella dashboard") || !strings.Contains(strings.ToLower(text), "refused") {
			t.Errorf("%s still tells the reader to copy the secret from the dashboard, or does not say these actions are refused", doc)
		}
	}
	for _, q := range []string{"refresh the API key and secret", "rotate the S3 bucket key", "add a new tunnel", "rotate the tunnel credentials", "create an API key with scopes"} {
		out := e.mcpCalls(t, []map[string]any{{"name": "cisco-umbrella_search", "arguments": map[string]any{"query": q}}})
		if !strings.Contains(out, "dashboard") {
			t.Errorf("the MCP search for %q does not name the dashboard:\n%.1500s", q, out)
		}
	}
}

// Class credential-store-claim. The first API call with environment credentials
// saves the key, the secret and the bearer, later runs use them after the variables
// are unset, and auth logout deletes them. The docs say that.
func TestCredentialStoreIsWhatTheDocsSay(t *testing.T) {
	t.Parallel()
	holdsAll := func(store string) bool {
		return strings.Contains(store, bbKey) && strings.Contains(store, bbSecret) && strings.Contains(store, bbMinted)
	}
	sv, e := honestEnv(t, nil)
	live := []string{"networks", "list", "--data-source", "live", "--json"}
	e.run(t, live...)
	if !holdsAll(credentialStore(t, e.home)) {
		t.Errorf("an API call did not save the key, the secret and the bearer:\n%s", credentialStore(t, e.home))
	}
	if runtime.GOOS != "windows" {
		matches, _ := filepath.Glob(filepath.Join(e.home, ".local", "share", "*", "credentials.toml"))
		if len(matches) != 1 {
			t.Fatalf("want one credentials.toml in the data directory, got %v", matches)
		}
		if st, err := os.Stat(matches[0]); err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("credentials.toml is not owner-only: %v, %v", st, err)
		}
		if out := e.run(t, "doctor"); !strings.Contains(out, "data: "+filepath.Dir(matches[0])) {
			t.Errorf("doctor does not print the data directory:\n%.1500s", out)
		}
	}

	gone := e.without("CISCO_UMBRELLA_API_KEY", "CISCO_UMBRELLA_API_SECRET")
	gone.run(t, live...)
	if got := sv.auth(); got != "Bearer "+bbMinted {
		t.Errorf("with the variables unset, the request carried %q, want the saved bearer", got)
	}
	gone.run(t, "auth", "logout")
	if store := credentialStore(t, e.home); strings.Contains(store, bbKey) || strings.Contains(store, bbSecret) || strings.Contains(store, bbMinted) {
		t.Errorf("auth logout left a credential:\n%s", store)
	}
	gone.run(t, live...)
	if got := sv.auth(); got != "" {
		t.Errorf("after auth logout, the request carried %q", got)
	}
	e.run(t, live...)
	if !holdsAll(credentialStore(t, e.home)) {
		t.Error("with the variables still set, the next API call should save them again, as the docs say")
	}

	for _, c := range []struct {
		args  []string
		saves bool
	}{
		{[]string{"doctor"}, true},
		{[]string{"sync", "--resources", "networks"}, true},
		{[]string{"networks", "list", "--dry-run"}, false},
		{[]string{"networks", "list", "--data-source", "local", "--json"}, false},
		{[]string{"auth", "status"}, false},
		{[]string{"--org", "1234567", "networks", "list", "--data-source", "live", "--json"}, false},
	} {
		_, e := honestEnv(t, nil)
		e.run(t, c.args...)
		if store := credentialStore(t, e.home); holdsAll(store) != c.saves || !c.saves && store != "" {
			t.Errorf("%v: saves %v, want %v:\n%s", c.args, holdsAll(store), c.saves, store)
		}
	}
	_, e = honestEnv(t, nil)
	e.mcpCalls(t, []map[string]any{{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "networks.list"}}})
	if !holdsAll(credentialStore(t, e.home)) {
		t.Error("an MCP read should save them too, as the docs say")
	}

	root := filepath.Join("..", "..", "..")
	for doc, wrong := range map[string]string{
		"README.md":     "or transmitted anywhere by MSP Skills.\n",
		"governance.md": "nothing else on disk holds a\ncredential",
		"guide.md":      "",
		"SKILL.md":      "",
		"CHANGELOG.md":  "",
	} {
		b, err := os.ReadFile(filepath.Join(root, doc))
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		if wrong != "" && strings.Contains(text, wrong) {
			t.Errorf("%s still makes the old claim %q", doc, wrong)
		}
		for _, want := range []string{"credentials.toml", "auth logout", "not encrypted", "unset"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", doc, want)
			}
		}
	}
}

// Class tunnel-status-unknown. The spec's TunnelStateInformation.status is UP, DOWN,
// FAILED or UNKNOWN, "pending updated information": only DOWN and FAILED are down.
func TestTunnelStatusUnknownIsNotDown(t *testing.T) {
	t.Parallel()
	state := `[{"tunnelId":"1","status":"UP"},{"tunnelId":"2","status":"DOWN"},{"tunnelId":"3","status":"UNKNOWN"},{"tunnelId":"4","status":"FAILED"},{"tunnelId":"5","status":"DEGRADED"}]`
	_, e := honestEnv(t, map[string]string{
		"/admin/v2/managed/customers":       `[{"customerId":9,"customerName":"Initrode"}]`,
		"/deployments/v2/roamingcomputers":  `[]`,
		"/deployments/v2/virtualappliances": `[]`,
		"/deployments/v2/tunnelsState":      state,
	})
	fleet := e.run(t, "fleet", "health", "--json")
	if !strings.Contains(fleet, `"tunnels_down": 2`) || !strings.Contains(fleet, `"tunnels_total": 5`) {
		t.Errorf("fleet health: want 2 of 5 tunnels down:\n%.1500s", fleet)
	}
	if !strings.Contains(fleet, "tunnels: status check: 2 record(s)") {
		t.Errorf("fleet health: want the 2 unusable statuses reported as not checked:\n%.1500s", fleet)
	}
	e.run(t, "sync", "--resources", "tunnels-tunnels-state")
	hygiene := e.run(t, "hygiene", "--json")
	if !strings.Contains(hygiene, `"unchecked_checks": {
    "tunnel-down": 2`) {
		t.Errorf("hygiene: want the 2 unusable statuses under unchecked_checks:\n%.1500s", hygiene)
	}
	for id, down := range map[string]bool{"1": false, "2": true, "3": false, "4": true, "5": false} {
		if got := strings.Contains(hygiene, `"id": "`+id+`"`) || strings.Contains(hygiene, `"name": "`+id+`"`) || strings.Contains(hygiene, `"resource": "`+id+`"`); got != down {
			t.Errorf("hygiene: tunnel %s reported %v, want %v:\n%.1500s", id, got, down, hygiene)
		}
	}
}

// A key and secret in an owner-only config file authenticate, as doctor says they
// do. They used to send requests with no credential at all.
func TestConfigFileCredentialsAuthenticate(t *testing.T) {
	t.Parallel()
	const fileKey, fileSecret = "cfgFileKey-8f3a1c9d7e2b", "cfgFileSecret-4b7e2d9a1c6f8e30"
	sv, e := honestEnv(t, nil)
	dir := filepath.Join(e.home, ".config", "cisco-umbrella-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	conf := "umbrella_api_key = \"" + fileKey + "\"\numbrella_api_secret = \"" + fileSecret + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	live := []string{"networks", "list", "--data-source", "live", "--json"}
	// The environment wins over the file; --org keeps this run off the store.
	e.run(t, append([]string{"--org", "1234567"}, live...)...)
	if u, p := sv.mintBasic(); u != bbKey || p != bbSecret {
		t.Errorf("with both set, the mint used %q and %q, want the environment's", u, p)
	}
	gone := e.without("CISCO_UMBRELLA_API_KEY", "CISCO_UMBRELLA_API_SECRET")
	out := gone.run(t, live...)
	if u, p := sv.mintBasic(); u != fileKey || p != fileSecret {
		t.Errorf("the mint used %q and %q, want the config file's:\n%.800s", u, p, out)
	}
	if got := sv.auth(); got != "Bearer "+bbMinted {
		t.Errorf("the request carried %q, want the minted bearer", got)
	}
	if strings.Contains(out, fileKey) || strings.Contains(out, fileSecret) {
		t.Errorf("the output repeats a credential:\n%.800s", out)
	}
}

// A source with no entries reads as "remove everything". It used to, and exit 0.
func TestEmptySourceCannotEmptyAList(t *testing.T) {
	t.Parallel()
	lists := map[string]string{"/policies/v2/destinationlists/11/destinations": `{"status":{"code":200},"data":[{"id":"501","destination":"keep.example.com"}]}`}
	for name, c := range map[string]struct {
		stdin string
		args  []string
	}{
		"empty stdin":      {"", []string{"--file", "-"}},
		"only comments":    {"# nothing here\n\n", []string{"--file", "-"}},
		"blank entries":    {"", []string{"--entries", " , "}},
		"with --no-remove": {"", []string{"--file", "-", "--no-remove"}},
	} {
		t.Run("CLI "+name, func(t *testing.T) {
			t.Parallel()
			sv, e := honestEnv(t, lists)
			out := e.runIn(t, c.stdin, append(append([]string{"destination-lists", "diff", "11"}, c.args...), "--apply", "--json")...)
			if !strings.Contains(out, "no entries") || !strings.Contains(out, "Umbrella dashboard") {
				t.Errorf("want the refusal, got:\n%.800s", out)
			}
			if n := sv.hits.Load(); n != 0 {
				t.Errorf("the stub received %d request(s):\n%.800s", n, out)
			}
		})
	}
	t.Run("MCP", func(t *testing.T) {
		t.Parallel()
		sv, e := honestEnv(t, lists)
		out := e.mcpCalls(t, []map[string]any{{"name": "destination_lists_diff", "arguments": map[string]any{"destinationListId": "11", "entries": " , ", "apply": true, "confirm": true}}})
		if !strings.Contains(out, "no entries") {
			t.Errorf("want the refusal, got:\n%.1500s", out)
		}
		if n := sv.writes.Load(); n != 0 {
			t.Errorf("the stub received %d write(s):\n%.1500s", n, out)
		}
	})
	t.Run("without --apply it stays a plan", func(t *testing.T) {
		t.Parallel()
		sv, e := honestEnv(t, lists)
		out := e.runIn(t, "", "destination-lists", "diff", "11", "--file", "-", "--json")
		if !strings.Contains(out, "keep.example.com") || sv.writes.Load() != 0 {
			t.Errorf("want a plan that removes the entry, and no write:\n%.800s", out)
		}
	})
}

// The docs' claims about MCP previews and fleet health's scopes.
func TestPreviewAndScopeClaims(t *testing.T) {
	t.Parallel()
	read := func(doc string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", doc))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, doc := range []string{"README.md", "governance.md", "CHANGELOG.md"} {
		text := read(doc)
		if !strings.Contains(text, "write to Umbrella") || !strings.Contains(text, "only this machine") {
			t.Errorf("%s does not say that every write to Umbrella previews and that learn-store tools change only this machine", doc)
		}
	}
	for _, doc := range []string{"README.md", "page.json"} {
		if !strings.Contains(read(doc), "Admin customers scope") {
			t.Errorf("%s does not say fleet health also needs the Admin customers scope", doc)
		}
	}
	// Behind the first claim: a learn-store tool runs with no confirm and sends nothing.
	sv, e := honestEnv(t, nil)
	out := e.mcpCalls(t, []map[string]any{{"name": "teach", "arguments": map[string]any{"query": "find networks", "resource": "123", "resource-type": "networks", "notes": "a note", "no-validate": true}}})
	if strings.Contains(out, "confirmation_required") || sv.writes.Load() != 0 {
		t.Errorf("a learn-store tool asked for a confirm or wrote to Umbrella (%d):\n%.1500s", sv.writes.Load(), out)
	}
	if got := e.run(t, "learnings", "list", "--json"); !strings.Contains(got, "find networks") {
		t.Errorf("the learn-store tool changed nothing here:\n%.800s", got)
	}
}
