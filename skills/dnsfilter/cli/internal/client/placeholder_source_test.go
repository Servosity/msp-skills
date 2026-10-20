// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "placeholder-remedy-names-the-source" entry.

package client

import (
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/config"
	"dnsfilter-pp-cli/internal/store"
)

// TestPlaceholderRemedyNamesTheSource: a placeholder key from the environment
// is reported as the environment variable, not the config file, and every
// form of the message passes the read-only MCP text rule.
func TestPlaceholderRemedyNamesTheSource(t *testing.T) {
	cfg := &config.Config{Path: "/home/x/config.toml", AuthSource: "env:DNSFILTER_API_KEY"}
	if got := authPlaceholderCredentialErrorWithSetup(cfg, "fix").Error(); !strings.Contains(got, "the DNSFILTER_API_KEY environment variable") || strings.Contains(got, "config.toml") {
		t.Fatalf("env placeholder: %q", got)
	}
	cfg.AuthSource = "config"
	if got := authPlaceholderCredentialErrorWithSetup(cfg, "fix").Error(); !strings.Contains(got, "/home/x/config.toml") {
		t.Fatalf("config placeholder: %q", got)
	}
	cfg.CredentialSource = "credentials file"
	if got := authPlaceholderCredentialErrorWithSetup(cfg, "fix").Error(); !strings.Contains(got, "saved credentials file") || strings.Contains(got, "config.toml") {
		t.Fatalf("credentials-file placeholder: %q", got)
	}
	setup := `export DNSFILTER_API_KEY="your-token-here" (PowerShell: $env:DNSFILTER_API_KEY = "your-token-here")`
	for _, src := range [][2]string{{"env:DNSFILTER_API_KEY", ""}, {"config", "credentials file"}, {"config", "config file"}} {
		c := &config.Config{Path: "/home/x/config.toml", AuthSource: src[0], CredentialSource: src[1]}
		if msg := authPlaceholderCredentialErrorWithSetup(c, setup).Error(); store.TextNamesSecret(msg) {
			t.Fatalf("read-only MCP would withhold the remedy: %q", msg)
		}
	}
}
