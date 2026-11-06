// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cometblackbox

// Hand-written: class operator-input. What the operator hands over (argv, a saved
// profile, stdin, an input file, an MCP argument) is held before anything can print
// it, in every form it arrives in. Each case is a review's failing input.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeProfiles(t *testing.T, e *bbEnv, store string) {
	t.Helper()
	dir := filepath.Join(e.home, "profiles-config")
	e.env = append(e.env, "COMET_BACKUP_CONFIG_DIR="+dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(store), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWebAuthnSignHeldInMCPPreview(t *testing.T) {
	t.Parallel()
	e := newEnv(t, cleanServer(t), false)
	const sign = "webAuthnSignVal-4f8c2e"
	out := e.mcpCalls(t, []map[string]any{{"name": "comet-backup_execute", "arguments": map[string]any{
		"endpoint_id": "users.delete", "params": map[string]any{"TargetUser": "acme", "WebAuthnSign": sign},
	}}})
	assertNotPrinted(t, "execute preview", out, sign)
}

func TestImportRecordHeldBeforeItsMapDecode(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	const dup, broken = "importDupMember-3d7a1b", "importBrokenLine-7c2f9e"
	sv.mu.Lock()
	sv.refuse = "already taken: " + dup
	sv.mu.Unlock()
	input := filepath.Join(e.home, "records.jsonl")
	body := `{"TargetUser":"acme","TargetPassword":"` + dup + `","TargetPassword":"later-value"}` + "\n" +
		`{"TargetUser":"acme","TargetPassword":"` + broken + `", broken` + "\n"
	if err := os.WriteFile(input, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := e.run(t, "import", "users", "--input", input, "--yes")
	assertNotPrinted(t, "import", out, dup, broken)
	if !strings.Contains(out, "credkit: withheld") {
		t.Errorf("no notice, so the refusal never repeated the member:\n%s", out)
	}
}

func TestProfileEnabledStdinHeld(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	const dup = "profileStdinDup-1e6b4d"
	sv.mu.Lock()
	sv.extra = dup
	sv.mu.Unlock()
	writeProfiles(t, e, `{"profiles":{"p":{"name":"p","values":{"stdin":"true"}}}}`)
	out := e.runIn(t, `{"Password":"`+dup+`","Password":"later-value"}`, "constellation", "last-report", "--profile", "p", "--json")
	assertNotPrinted(t, "profile stdin", out, dup)
}

// A webhook URL carrying its own credential is not sent: the value is held, and a
// held value goes only to the Comet Server. An ordinary query value is sent.
func TestDeliverURLWithItsOwnCredentialIsNotSent(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	count := func() int { sv.mu.Lock(); defer sv.mu.Unlock(); return len(sv.sunk) }
	e.run(t, "server", "version", "--json", "--deliver", "webhook:"+strings.Replace(e.sink, "://", "://user:ownHookPass-1234@", 1))
	e.run(t, "server", "version", "--json", "--deliver", "webhook:"+e.sink+"/h?token=ownHookTok-5678")
	if n := count(); n != 0 {
		t.Fatalf("a webhook URL carrying a credential was sent to (%d bodies)", n)
	}
	e.run(t, "server", "version", "--json", "--deliver", "webhook:"+e.sink+"/h?channel=ops")
	if n := count(); n != 1 {
		t.Fatalf("an ordinary webhook URL was not sent to (%d bodies)", n)
	}
}

// auth logout rewrites the config. An environment override is not config: saved, it
// would write a URL's password to disk and pin that server for every later run.
func TestLogoutKeepsTheEnvironmentOutOfTheConfig(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	const pw = "logoutUrlPw-7e2c9a"
	host := strings.TrimPrefix(sv.url, "https://")
	for name, kv := range map[string]string{
		"base url": "COMET_BACKUP_BASE_URL=https://usrxyz:" + pw + "@" + host,
		"server":   "COMET_BACKUP_SERVER=usrxyz:" + pw + "@" + host,
	} {
		e := newEnv(t, sv, false)
		e.env = append(e.env, kv)
		e.run(t, "auth", "logout")
		for _, hit := range [][]string{filesHolding(t, e.home, pw), filesHolding(t, e.home, host)} {
			if len(hit) > 0 {
				t.Errorf("%s: logout saved the environment's value in %v", name, hit)
			}
		}
	}
}

// A value that merely starts with a brace is not a credential document.
func TestBraceShapedArgumentStillRuns(t *testing.T) {
	t.Parallel()
	e := newEnv(t, cleanServer(t), false)
	if out := e.run(t, "users", "list", "--select", "{x}", "--json"); strings.TrimSpace(out) == "" {
		t.Error("the program stopped with no message")
	}
}

// A write preview names what it would send with its credentials redacted, so an
// agent can read it before confirming.
func TestExecutePreviewRedactsItsBody(t *testing.T) {
	t.Parallel()
	e := newEnv(t, cleanServer(t), false)
	const pw = "previewTargetPw-3f9b1c"
	out := e.mcpCalls(t, []map[string]any{{"name": "comet-backup_execute", "arguments": map[string]any{
		"endpoint_id": "users.add", "params": map[string]any{"TargetUser": "acme", "TargetPassword": pw},
	}}})
	assertNotPrinted(t, "execute preview", out, pw)
	if !strings.Contains(out, "confirmation_required") || !strings.Contains(out, "acme") {
		t.Errorf("no usable preview:\n%.600s", out)
	}
}

// Adversarial round 2: a value is not a credential for looking like the start of a
// document or for naming a word the program prints. Text the operator typed still runs.
func TestOrdinaryTextArgumentsStillRun(t *testing.T) {
	t.Parallel()
	e := newEnv(t, cleanServer(t), false)
	for _, args := range [][]string{
		{"users", "set-profile", "--target-user", "acme", "--profile-data", `{"AccountName":"[VIP] Acme password vault"}`, "--dry-run", "--json"},
		{"recall", "[error] password reset failed", "--agent"},
		{"recall", `{"failed backups"}`},
		{"users", "list", "--select", "{id,key}", "--json"},
		{"recall", "why did https://docs.example.com/page?key=id fail", "--agent"},
		{"recall", "why did https://docs.example.com/page?sort_key=id fail", "--agent"},
		{"recall", "why did https://docs.example.com/page?key=code fail", "--agent"},
		{"recall", "why did https://docs.example.com/page?page_token=1 fail", "--agent"},
	} {
		cmd := exec.Command(bbCLI, args...)
		cmd.Env, cmd.Dir = e.env, e.home
		out, _ := cmd.CombinedOutput()
		if code := cmd.ProcessState.ExitCode(); code == 78 || strings.TrimSpace(string(out)) == "" {
			t.Errorf("%v: exit %d, output %q", args, code, out)
		}
	}
}

// The contract names a password inside base_url. A raw / # or ? cannot end it early.
func TestBaseURLPasswordWithReservedCharactersHeld(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	const pw = "abc/SlashPass9k8j7h"
	host := strings.TrimPrefix(sv.url, "https://")
	for name, setup := range map[string]func(e *bbEnv){
		"base url env": func(e *bbEnv) { e.env = append(e.env, "COMET_BACKUP_BASE_URL=https://usr:"+pw+"@"+host) },
		"server env":   func(e *bbEnv) { e.env = append(e.env, "COMET_BACKUP_SERVER=usr:"+pw+"@"+host) },
	} {
		e := newEnv(t, sv, false)
		setup(e)
		for _, args := range [][]string{{"server", "version", "--json"}, {"doctor", "--json"}} {
			assertNotPrinted(t, name+" "+args[0], e.run(t, args...), "SlashPass9k8j7h")
		}
	}
	e := newEnv(t, sv, false)
	cfg := filepath.Join(e.home, "slash.toml")
	if err := os.WriteFile(cfg, []byte(`base_url = "https://usr:`+pw+`@`+host+`"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertNotPrinted(t, "config", e.run(t, "server", "version", "--json", "--config", cfg), "SlashPass9k8j7h")
}

