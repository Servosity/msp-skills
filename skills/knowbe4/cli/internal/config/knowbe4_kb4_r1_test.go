// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"knowbe4-pp-cli/internal/cliutil/testenv"
)

// Codex review round 1 (knowbe4 4.34.2 reprint), class
// persistence-gate-skipped-under-agentcookie: with an .agentcookie-managed
// marker beside config.toml, save() wrote the whole configuration without
// the persistence gate, so a credential in base_url (its userinfo password or
// a credential query value, here from KNOWBE4_BASE_URL) was written into
// config.toml verbatim. The credential fields agentcookie manages are its
// declared secrets file's (agentcookie.toml [secrets.file]) and keep its
// contract; every other field passes the same gate as without the marker.
func TestAgentcookieConfigPassesThePersistenceGate(t *testing.T) {
	testenv.Isolate(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("base_url = \"https://us.api.knowbe4.com/v1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentcookie-managed"), []byte("managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KNOWBE4_BASE_URL", "https://ro:AgentcookiePw-6b2e9d@us.api.knowbe4.com/v1?api_key=AgentcookieKey-0f4a71")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = cfg.SaveTokens("", "", "agentcookieManaged-3c8e51", "", time.Now().Add(time.Hour))
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"AgentcookiePw-6b2e9d", "AgentcookieKey-0f4a71"} {
		if strings.Contains(string(got), s) {
			t.Errorf("an agentcookie-managed config.toml holds %q from base_url:\n%s", s, got)
		}
	}
}

// Class credential-to-other-api-host, the whole declared set: the User Event
// client refuses every Reporting credential the configuration sends, not only
// the key fields. The failing input: a credential-named configured header
// (headers."X-Api-Token") and a base_url carrying a userinfo password and an
// api_key query value were missing from ReportingCredentials, so an event
// description repeating them reached the User Event host.
func TestReportingCredentialsCoverEveryConfiguredSecret(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[headers]\n\"X-Api-Token\" = \"Bearer HeaderSecret-91c2e7\"\nAccept = \"application/json\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KNOWBE4_BASE_URL", "https://ro:BaseUrlPw-6b2e9d@us.api.knowbe4.com/v1?api_key=BaseUrlKey-0f4a71")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(ReportingCredentials(cfg), "\n")
	for _, want := range []string{"HeaderSecret-91c2e7", "BaseUrlPw-6b2e9d", "BaseUrlKey-0f4a71"} {
		if !strings.Contains(got, want) {
			t.Errorf("ReportingCredentials misses %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "application/json") {
		t.Errorf("ReportingCredentials holds a non-credential header value:\n%s", got)
	}
}
