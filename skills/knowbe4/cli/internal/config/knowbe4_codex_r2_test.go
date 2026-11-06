// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"knowbe4-pp-cli/internal/credkit"
)

// Codex review round 2 (knowbe4 4.34.2 reprint), class
// persistence-gate-skipped-under-agentcookie: the gate judged each header
// value alone and then left the headers out of the combined check, so a held
// key split between a header value and another non-credential field was
// written. The failing input from the review: a marked (.agentcookie-managed)
// 0600 config.toml with [headers] X-Tail = "ujmikolp0987654321", and
// KNOWBE4_API_KEY=<the 36-character test value>
// KNOWBE4_REGION=qazwsxedcrfvtgbyhn running `auth set-token`: the save put
// the key's prefix in template_vars.region and kept its suffix in
// headers.X-Tail. Every field the write persists is judged together, header
// values included, against every credential the configuration carries, with
// the marker and without it. The last check is the credential kit's, against
// what it holds (KNOWBE4_API_KEY from startup), so this runs in a child that
// holds the key.
func TestR2ConfigWriteJudgesEveryFieldTogether(t *testing.T) {
	if runInChild(t) {
		return
	}
	const heldVal = "qazwsxedcrfvtgbyhnujmikolp0987654321"
	const prefix, suffix = "qazwsxedcrfvtgbyhn", "ujmikolp0987654321"
	credkit.Hold("env:KNOWBE4_API_KEY", heldVal)
	for _, managed := range []bool{true, false} {
		isolateCredentials(t)
		dir := t.TempDir()
		path := filepath.Join(dir, "config.toml")
		fixture := []byte("[headers]\nX-Tail = \"" + suffix + "\"\nX-Keep = \"keep-me\"\n")
		if err := os.WriteFile(path, fixture, 0o600); err != nil {
			t.Fatal(err)
		}
		if managed {
			if err := os.WriteFile(filepath.Join(dir, ".agentcookie-managed"), []byte("managed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("KNOWBE4_API_KEY", heldVal)
		t.Setenv("KNOWBE4_REGION", prefix)
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		// The save is refused (the file stays as the operator wrote it) or
		// writes no piece of the key: template_vars.region alone carries an
		// 18-character piece of it.
		saveErr := cfg.SaveTokens("", "", "ManagedToken-new", "", cfg.TokenExpiry) // auth set-token
		got, _ := os.ReadFile(path)
		if saveErr != nil && string(got) != string(fixture) || saveErr == nil && echoes(string(got), []string{heldVal}) {
			t.Errorf("managed %v: config.toml holds the API key split across template_vars.region and headers.X-Tail (save error %v):\n%s", managed, saveErr, got)
		}
	}

	// The same split with a credential the base URL carries: its userinfo
	// password in a header value is a credential too.
	isolateCredentials(t)
	t.Setenv("KNOWBE4_REGION", "")
	path := filepath.Join(t.TempDir(), "config.toml")
	const pw = "UrlUserinfoPw-4e9b1c"
	if err := os.WriteFile(path, []byte("base_url = \"https://ro:"+pw+"@us.api.knowbe4.com/v1\"\n[headers]\nX-Trace = \""+pw+"\"\nX-Keep = \"keep-me\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = cfg.SaveTokens("", "", "PlainToken-new", "", cfg.TokenExpiry)
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), pw) {
		t.Errorf("config.toml keeps the base URL's userinfo password in a header:\n%s", got)
	}
	if !strings.Contains(string(got), "keep-me") {
		t.Errorf("config.toml lost the unrelated header X-Keep:\n%s", got)
	}
}

// Class credential-to-other-api-host, instance 1: the Reporting client's own
// credential is what it sends, from the effective configuration, never a
// value an environment variable overrode.
func TestR2SentReportingCredentialsAreTheEffectiveKey(t *testing.T) {
	isolateCredentials(t)
	path := filepath.Join(t.TempDir(), "kb4.toml")
	if err := os.WriteFile(path, []byte("api_key = \"FileLayerKey-61c0d2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KNOWBE4_API_KEY", "EnvReportingKey-3a9e57")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(SentReportingCredentials(cfg), "\n")
	if !strings.Contains(got, "EnvReportingKey-3a9e57") {
		t.Errorf("SentReportingCredentials misses the key the client sends:\n%s", got)
	}
	if strings.Contains(got, "FileLayerKey-61c0d2") {
		t.Errorf("SentReportingCredentials holds the overridden file layer's key:\n%s", got)
	}
	if h := cfg.AuthHeader(); !strings.Contains(got, h) {
		t.Errorf("SentReportingCredentials misses the Authorization header %q:\n%s", h, got)
	}
}
