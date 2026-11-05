// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entry
// "credkit-wiring" (class secret-echo).

package cli

import (
	"avanan-pp-cli/internal/credkit"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"avanan-pp-cli/internal/cliutil"
)

// TestLocalFileSinksRefuseAHeldValue: every local file the CLI appends
// operator or vendor text to (job ledger, feedback log, teach audit and error
// logs, saved profiles) refuses a held credential.
func TestLocalFileSinksRefuseAHeldValue(t *testing.T) {
	if !heldChild(t) {
		return
	}
	home := isolateAvananEnv(t)
	const v = "sink-held-value-FS1"
	credkit.Hold("config:unclassified", v)
	_ = RecordJob(JobRow{JobID: "job-1", Error: "vendor said " + v})
	_ = appendFeedback(FeedbackEntry{Text: "my key is " + v})
	_ = appendLearningsAudit(map[string]any{"note": v})
	writeTeachErrLog("teach: missing --query (args=[" + v + "])")
	_ = saveProfileStore(&profileStore{Profiles: map[string]Profile{"p": {Name: "p", Values: map[string]string{"tenant": v}}}})
	assertStoredNowhere(t, home, v)
}

// TestConfigCredentialsAreHeldBeforeFlagsParse: a config file's credentials
// are held when the process starts, so a run that fails on its flags (before
// any command loads config) cannot print one: whichever file the run selects
// (AVANAN_CONFIG, --config once or repeated, --home), in any TOML form the
// loader accepts, and in a malformed file.
func TestConfigCredentialsAreHeldBeforeFlagsParse(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	tenantConfig := func(t *testing.T, tenant string) string {
		restore, err := cliutil.SetHomeOverride(tenant)
		if err != nil {
			t.Fatal(err)
		}
		defer restore()
		dir, err := cliutil.ConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		return filepath.Join(dir, "config.toml")
	}
	for _, tc := range []struct {
		name, value, body string
		file              func(t *testing.T, home string) string // default: AVANAN_CONFIG
		flags             func(home string) []string
	}{
		{name: "config-from-env", value: "early-app-id-EP1", body: `app_id = "early-app-id-EP1"`},
		{name: "explicit-config-flag", value: "early-app-id-EP2", body: `app_id = "early-app-id-EP2"`,
			file: func(_ *testing.T, home string) string { return filepath.Join(home, "elsewhere", "other.toml") },
			flags: func(home string) []string {
				return []string{"--config", filepath.Join(home, "elsewhere", "other.toml")}
			}},
		{name: "repeated-config-flag", value: "early-app-id-EP3", body: `client_secret = "early-app-id-EP3"`,
			file: func(_ *testing.T, home string) string { return filepath.Join(home, "b.toml") },
			flags: func(home string) []string {
				return []string{"--config", filepath.Join(home, "a.toml"), "--config=" + filepath.Join(home, "b.toml")}
			}},
		{name: "home-flag", value: "tenant-secret-817", body: `client_secret = "tenant-secret-817"`,
			file:  func(t *testing.T, home string) string { return tenantConfig(t, filepath.Join(home, "TENANT")) },
			flags: func(home string) []string { return []string{"--home", filepath.Join(home, "TENANT")} }},
		{name: "upper-case-key", value: "early-file-secret-UC1", body: `APP_ID = "early-file-secret-UC1"`},
		{name: "multiline-string", value: "early-file-secret-UC2", body: "app_id = \"\"\"\nearly-file-secret-UC2\"\"\""},
		{name: "quoted-key", value: "early-file-secret-UC3", body: `"app_id" = 'early-file-secret-UC3'`},
		{name: "dotted-header-key", value: "early-file-secret-UC4", body: `headers.X-Key = "early-file-secret-UC4"`},
		{name: "inline-header-table", value: "early-file-secret-UC5", body: `headers = { X-Key = "early-file-secret-UC5" }`},
		{name: "header-table-comment", value: "early-file-secret-UC6", body: "[headers] # upstream\nX-Key = \"early-file-secret-UC6\""},
		{name: "malformed-multiline", value: "early-file-secret-UC7", body: "app_id = '''\nearly-file-secret-UC7'''\nbroken ="},
		{name: "malformed-inline-table", value: "early-file-secret-UC8", body: "headers = { X-Key = \"early-file-secret-UC8\" }\nbroken ="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := isolateAvananEnv(t)
			path := filepath.Join(home, "config.toml")
			if tc.file != nil {
				path = tc.file(t, home)
				t.Setenv("AVANAN_CONFIG", "")
			}
			writeSandboxFile(t, path, tc.body+"\n")
			var args []string
			if tc.flags != nil {
				args = tc.flags(home)
			}
			cmd := exec.Command(bin, append(args, "--timeout", tc.value, "auth", "status")...)
			cmd.Env = os.Environ()
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Run(); err == nil {
				t.Fatalf("an invalid --timeout was accepted:\n%s", out.String())
			}
			assertNoCredentialForm(t, "flag-parse output", out.String(), tc.value)
		})
	}
}

// TestStartupDiagnosticsAreChecked: what the CLI prints while it resolves its
// paths at start (an ignored path override's warning) passes the check, after
// the env credentials are held.
func TestStartupDiagnosticsAreChecked(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	isolateAvananEnv(t)
	const v = "secret-dir-OR1"
	for _, env := range []string{"AVANAN_CONFIG_DIR", "AVANAN_DATA_DIR", "AVANAN_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		cmd := exec.Command(bin, "version")
		cmd.Env = append(os.Environ(), "AVANAN_CONFIG=", "AVANAN_HOME=", "AVANAN_CONFIG_DIR=", "AVANAN_DATA_DIR=", "AVANAN_STATE_DIR=", "AVANAN_CACHE_DIR=", "AVANAN_CLIENT_SECRET="+v, env+"="+v)
		out, _ := cmd.CombinedOutput()
		assertNoCredentialForm(t, env+" startup output", string(out), v)
	}
}

// TestHeaderComponentsAreHeldUnquoted: a [headers] cookie value written in
// quotes is held bare, so a sink that sees only the bare value (the feedback
// log) refuses it.
func TestHeaderComponentsAreHeldUnquoted(t *testing.T) {
	bin := buildCommand(t, "../../cmd/avanan-cli")
	home := isolateAvananEnv(t)
	const v = "cookie-secret-Q1"
	writeSandboxFile(t, filepath.Join(home, "config.toml"), "[headers]\nCookie = 'sid=\""+v+"\"; theme=dark'\n")
	cmd := exec.Command(bin, "feedback", "cookie "+v)
	cmd.Env = os.Environ()
	out, _ := cmd.CombinedOutput()
	assertNoCredentialForm(t, "feedback output", string(out), v)
	assertStoredNowhere(t, home, v)
}
