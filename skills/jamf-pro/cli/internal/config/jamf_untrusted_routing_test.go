// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json untrusted-config-routing).
// A config file other users can write must never choose where a credential
// goes: not by base_url, not by [template_vars] server, and not later, through
// a save that rewrites the file owner-only with the refused routing in it.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"jamf-pro-pp-cli/internal/cliutil"
)

const evilHost = "evil-collector.example"

func writeLooseConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), configFileName())
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	grantWorld(t, p)
	return p
}

// A loose file holding only [template_vars] server must not route the
// environment's client secret to that server.
func TestLooseConfigServerDoesNotRouteEnvSecret(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("JAMF_PRO_SERVER", "")
	t.Setenv("JAMF_PRO_BASE_URL", "")
	t.Setenv("JAMF_PRO_CLIENT_ID", "client-id-1")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "env-secret-must-not-route")
	p := writeLooseConfig(t, "[template_vars]\nserver = \""+evilHost+"\"\n")

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.TemplateVars["server"] == evilHost && !cfg.CredentialRouteBlocked() {
		t.Fatalf("loose config chose the server %q for the environment's secret", evilHost)
	}
	if cfg.TemplateVars["server"] == evilHost {
		t.Fatalf("TemplateVars = %#v, want the loose file's server dropped", cfg.TemplateVars)
	}
}

// The environment's server still applies over a loose file's.
func TestLooseConfigServerYieldsToEnvServer(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("JAMF_PRO_BASE_URL", "")
	t.Setenv("JAMF_PRO_SERVER", "acme.jamfcloud.com")
	p := writeLooseConfig(t, "[template_vars]\nserver = \""+evilHost+"\"\n")

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got := cfg.TemplateVars["server"]; got != "acme.jamfcloud.com" {
		t.Fatalf("server = %q, want the environment's", got)
	}
	if cfg.CredentialRouteBlocked() {
		t.Fatal("route blocked although the file chose no route that is used")
	}
}

// A run that overrides a refused file's routing from the environment and then
// saves a token must not rewrite the file owner-only with the refused routing.
func TestLooseConfigRoutingNotRepublishedBySave(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	for name, body := range map[string]string{
		"base_url":     "base_url = \"https://" + evilHost + "\"\n",
		"template_var": "[template_vars]\nserver = \"" + evilHost + "\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			isolateCredentials(t)
			t.Setenv("JAMF_PRO_BASE_URL", "https://acme.jamfcloud.com")
			t.Setenv("JAMF_PRO_SERVER", "acme.jamfcloud.com")
			t.Setenv("JAMF_PRO_CLIENT_ID", "client-id-1")
			t.Setenv("JAMF_PRO_CLIENT_SECRET", "env-secret-must-not-route")
			p := writeLooseConfig(t, body)

			cfg, err := Load(p)
			if err != nil {
				t.Fatalf("Load error: %v", err)
			}
			if err := cfg.SaveTokens("client-id-1", "", "access-1", "", time.Now().Add(time.Hour)); err != nil {
				t.Fatalf("SaveTokens: %v", err)
			}
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), evilHost) {
				t.Fatalf("save re-published the refused routing:\n%s", data)
			}

			// The next run, the variables gone, must not route anything there.
			t.Setenv("JAMF_PRO_BASE_URL", "")
			t.Setenv("JAMF_PRO_SERVER", "")
			next, err := Load(p)
			if err != nil {
				t.Fatalf("second Load error: %v", err)
			}
			if strings.Contains(next.BaseURL, evilHost) || next.TemplateVars["server"] == evilHost {
				t.Fatalf("second run routes to the refused server: base=%q vars=%#v", next.BaseURL, next.TemplateVars)
			}
		})
	}
}

// A saved credential is bound to the server this run talked to, not to the
// file's unresolved template, so the next run with the same server accepts it.
func TestSavedCredentialBoundToLiveServer(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("JAMF_PRO_BASE_URL", "https://acme.jamfcloud.com")
	t.Setenv("JAMF_PRO_SERVER", "acme.jamfcloud.com")
	t.Setenv("JAMF_PRO_CLIENT_ID", "client-id-1")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "env-secret-1")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if err := cfg.SaveTokens("client-id-1", "", "access-1", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}
	creds, _, err := cliutil.LoadCredentialsWithStatus()
	if err != nil || creds == nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if !cliutil.CredentialOriginsMatch(creds.BaseURL, "https://acme.jamfcloud.com") {
		t.Fatalf("credential bound to %q, want the live server", creds.BaseURL)
	}
}

// An opaque URL (no slashes after the scheme) keeps no password in the record.
func TestIssuedForURLOpaqueKeepsNoPassword(t *testing.T) {
	for _, raw := range []string{"https:user:pw-secret@acme.jamfcloud.com", "https:pw-secret@acme.jamfcloud.com"} {
		if got := issuedForURL(raw); strings.Contains(got, "pw-secret") {
			t.Fatalf("issuedForURL(%q) = %q, keeps the password", raw, got)
		}
	}
}
