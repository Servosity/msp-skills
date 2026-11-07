// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package nerdioblackbox

// The Nerdio-only credential rules, each proven on the built binaries against
// the fake NMM instance and the fake Entra tenant (blackbox_test.go):
//   - tenant-secrets-shown-never-stored: a device's LAPS password, its
//     BitLocker recovery keys and secure variable values are customer secrets
//     the operator asks for by name. The command (and the MCP read) shows
//     them to the caller who asked; no file the program writes (the local
//     store, a delivered file, the journal, the cache) ever holds one, a sync
//     included; a credential-named member of an ordinary response is masked
//     by name (credential-fields-masked-by-name).
//   - credential-names-match-nerdio-data: AVD session data (sessionId,
//     sessionState, sessionHostName) is not a credential and prints; a
//     sessionToken beside it is masked by name.
//   - mcp-endpoint-write-confirm / mcp-mirror-write-confirm, and the
//     autoscale-audit --baseline file read: nerdio_execute and the mirrored
//     scripted-actions fan-run (a write in many accounts) send nothing without
//     confirm: true, and fleet_autoscale_audit has no baseline argument (a
//     call naming one is refused and the file is never read).

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tenantSecrets are the customer secrets the fake answers on the tenant-secret
// paths.
func tenantSecrets() []string { return []string{bbLAPS, bbBitLocker, bbSecureVar, bbAcctSecVar} }

// holdsTenantSecret reports a tenant secret in text, as written or as a JSON
// encoder spells it.
func holdsTenantSecret(text string) string {
	for _, s := range tenantSecrets() {
		if holdsForm(text, s) {
			return s
		}
	}
	return ""
}

func TestTenantSecretsShownNeverStored(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			// The quiet fake: a unit that repeats a credential is withheld
			// whole (TestNoCredentialInAnyOutput), so the shown secrets and the
			// masked member are proven on answers that repeat none.
			sv := quietServer(t)
			e := newEnv(t, sv, mode)
			shown := map[string][]string{
				bbLAPS:       {"devices", "laps", bbAccount, bbDevice},
				bbBitLocker:  {"devices", "bitlocker-keys", bbAccount, bbDevice, "--json"},
				bbSecureVar:  {"secure-variables", "list", "--json"},
				bbAcctSecVar: {"secure-variables", "account-list", bbAccount, "--json"},
			}
			for secret, args := range shown {
				stdout, stderr := e.runSplit(t, "", args...)
				if !strings.Contains(stdout, secret) {
					t.Errorf("%v: the tenant secret the operator asked for is not on stdout:\n%.1500s\nstderr: %.500s", args, stdout, stderr)
				}
				assertNoEcho(t, fmt.Sprintf("output of %v", args), stdout+stderr)
			}
			if stdout, _ := e.runSplit(t, "", "devices", "laps", bbAccount, bbDevice, "--json"); !strings.Contains(stdout, bbLAPS) {
				t.Errorf("devices laps --json: the LAPS password is not on stdout:\n%.1500s", stdout)
			}
			// The ways a response can reach disk: a delivered file, an automatic
			// read's local store (then read back locally), and sync.
			deliver := filepath.Join(e.home, "laps-delivered.json")
			e.run(t, "devices", "laps", bbAccount, bbDevice, "--json", "--deliver", "file:"+deliver)
			out := e.run(t, "secure-variables", "list", "--data-source", "local", "--json")
			out += e.run(t, "devices", "laps", bbAccount, bbDevice, "--data-source", "local", "--json")
			syncOut := e.run(t, "sync", "--resources", "secure-variables", "--json")
			out += syncOut
			out += e.run(t, "secure-variables", "list", "--data-source", "local", "--json")
			if s := holdsTenantSecret(out); s != "" {
				t.Errorf("a local read or sync printed the tenant secret %q from disk:\n%.2000s", s, out)
			}
			// The MCP read shows the LAPS password to the agent that asked.
			mcp := e.mcpCalls(t, []map[string]any{{"name": "nerdio_read", "arguments": map[string]any{
				"endpoint_id": "devices.laps", "params": map[string]any{"account_id": 101, "device_id": bbDevice}}}})
			if !strings.Contains(mcp, bbLAPS) {
				t.Errorf("nerdio_read devices.laps did not return the LAPS password:\n%.1500s", mcp)
			}
			assertNoEcho(t, "MCP session", mcp)
			// An ordinary response's credential-named member is masked by name.
			stdout, _ := e.runSplit(t, "", "accounts", "--json")
			if !strings.Contains(stdout, "HQ-Record") || strings.Contains(stdout, bbGzipResp) || !strings.Contains(stdout, `"accessToken": "[withheld]"`) {
				t.Errorf("accounts --json: want the record with its accessToken masked:\n%.1500s", stdout)
			}
			var scanned int
			var sawSyncedRow bool
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				data, rerr := os.ReadFile(path)
				if rerr != nil {
					return nil
				}
				scanned++
				rel, _ := filepath.Rel(e.home, path)
				if s := holdsTenantSecret(string(data)); s != "" {
					t.Errorf("%s holds the tenant secret %q", rel, s)
				}
				sawSyncedRow = sawSyncedRow || (strings.HasSuffix(d.Name(), ".db") && strings.Contains(string(data), "DomainJoinPassword"))
				return nil
			})
			if scanned < 3 {
				t.Fatalf("only %d files under the home; the commands did not run", scanned)
			}
			if strings.Contains(syncOut, `"success": 1`) && !sawSyncedRow {
				t.Errorf("sync reported the secure variables stored, but no store holds the row; the store scan is vacuous:\n%.1500s", syncOut)
			}
			if b, err := os.ReadFile(deliver); err != nil || !strings.Contains(string(b), "localadmin") {
				t.Errorf("the delivered file lost the LAPS record itself (err %v):\n%.1000s", err, b)
			}
			checkMode(t, sv, mode)
		})
	}
}

// AVD session data reads as data, not as a credential: host-pools sessions
// prints sessionId, sessionState and sessionHostName (and the MCP read
// returns them), while the sessionToken in the same records is masked by name.
func TestAVDSessionDataPrintsAndSessionTokenWithheld(t *testing.T) {
	t.Parallel()
	sv := quietServer(t)
	e := newEnv(t, sv, credPair)
	stdout, stderr := e.runSplit(t, "", "host-pools", "sessions", bbAccount, bbSub, bbRG, bbPool, "--json")
	mcp := e.mcpCalls(t, []map[string]any{{"name": "nerdio_read", "arguments": map[string]any{
		"endpoint_id": "host-pools.sessions", "params": map[string]any{"account_id": 101, "subscription_id": bbSub, "resource_group": bbRG, "pool_name": bbPool}}}})
	for where, out := range map[string]string{"host-pools sessions": stdout, "MCP host-pools.sessions": mcp} {
		// The values themselves (sessionState "Active" is checked as a member
		// on stdout below: the word alone could come from elsewhere).
		for _, v := range []string{"avd-session-4f2a", "HQ-Host-0.contoso.local"} {
			if !strings.Contains(out, v) {
				t.Errorf("%s: AVD session data %s was withheld:\n%.2000s", where, v, out)
			}
		}
		if strings.Contains(out, bbSessionTok) {
			t.Errorf("%s printed the sessionToken:\n%.2000s", where, out)
		}
		assertNoEcho(t, where, out)
	}
	for _, v := range []string{`"sessionId": "avd-session-4f2a"`, `"sessionState": "Active"`, `"sessionHostName": "HQ-Host-0.contoso.local"`, `"sessionToken": "[withheld]"`} {
		if !strings.Contains(stdout, v) {
			t.Errorf("host-pools sessions: want the member %s:\n%.1500s\nstderr: %.500s", v, stdout, stderr)
		}
	}
	checkMode(t, sv, credPair)
}

// The MCP writes that change NMM: nerdio_execute (every state-changing
// endpoint) and the mirrored scripted-actions fan-run (executes a script in
// many accounts). Without confirm set to boolean true, each returns a preview
// and nothing reaches the NMM instance; with the pair in the environment not
// even a token is minted.
func TestMCPWritesWithoutConfirmSendNothing(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credPair)
	var calls []map[string]any
	for _, confirm := range []any{nil, false, "true", 1} {
		args := map[string]any{"endpoint_id": "scripted-actions.run-account", "params": map[string]any{"account_id": 101, "script_id": 7, "minutesToWait": 5}}
		fan := map[string]any{"script_id": "7", "accounts": bbAccount + ",102"}
		if confirm != nil {
			args["confirm"], fan["confirm"] = confirm, confirm
		}
		calls = append(calls, map[string]any{"name": "nerdio_execute", "arguments": args}, map[string]any{"name": "scripted_actions_fan_run", "arguments": fan})
	}
	out := e.mcpCalls(t, calls)
	replies := mcpReplies(out)
	for i, c := range calls {
		if r := replies[i+1]; !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") {
			t.Errorf("%v returned no preview:\n%.1000s", c, r)
		}
	}
	if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
		t.Errorf("an unconfirmed write sent %v (%d token request(s))", methods, mints)
	}
	assertNoEcho(t, "MCP session", out)
}

// fleet autoscale-audit --baseline reads a local JSON file (mcp:read-flags):
// the MCP tool has no baseline argument, and a call naming one is refused
// before anything runs (no request, the file never read).
func TestMCPAutoscaleAuditHasNoBaselineArgument(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credPair)
	listed := e.mcpRaw(t, []map[string]any{{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}})
	var props map[string]any
	for _, line := range strings.Split(listed, "\n") {
		var msg struct {
			ID     int `json:"id"`
			Result struct {
				Tools []struct {
					Name        string `json:"name"`
					InputSchema struct {
						Properties map[string]any `json:"properties"`
					} `json:"inputSchema"`
				} `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &msg) != nil || msg.ID != 1 {
			continue
		}
		for _, tool := range msg.Result.Tools {
			if tool.Name == "fleet_autoscale_audit" {
				props = tool.InputSchema.Properties
			}
		}
	}
	if props == nil {
		t.Fatalf("tools/list has no fleet_autoscale_audit tool:\n%.1500s", listed)
	}
	if _, ok := props["accounts"]; !ok {
		t.Fatalf("fleet_autoscale_audit lists no accounts argument; the schema read is wrong: %v", props)
	}
	if _, ok := props["baseline"]; ok {
		t.Errorf("fleet_autoscale_audit offers a baseline argument (a local file path) over MCP")
	}
	baseline := filepath.Join(e.home, "baseline.json")
	const marker = "BaselineMarker-6c2e9a"
	if err := os.WriteFile(baseline, []byte(`{"isEnabled":false,"note":"`+marker+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := e.mcpCalls(t, []map[string]any{
		{"name": "fleet_autoscale_audit", "arguments": map[string]any{"baseline": baseline}},
		{"name": "fleet_autoscale_audit", "arguments": map[string]any{"accounts": bbAccount, "baseline": baseline}},
	})
	replies := mcpReplies(out)
	for id := 1; id <= 2; id++ {
		if r := replies[id]; !strings.Contains(r, `"isError":true`) || !strings.Contains(r, "baseline") {
			t.Errorf("call %d naming baseline was not refused:\n%.1000s", id, r)
		}
	}
	if strings.Contains(out, marker) {
		t.Errorf("the baseline file was read:\n%.1500s", out)
	}
	if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
		t.Errorf("a refused call sent %v (%d token request(s))", methods, mints)
	}
	// Control: the same tool without baseline runs the audit.
	if ok := e.mcpCalls(t, []map[string]any{{"name": "fleet_autoscale_audit", "arguments": map[string]any{"accounts": bbAccount}}}); !strings.Contains(ok, "pool-east") {
		t.Errorf("control: fleet_autoscale_audit without baseline did not audit:\n%.1500s", ok)
	}
}
