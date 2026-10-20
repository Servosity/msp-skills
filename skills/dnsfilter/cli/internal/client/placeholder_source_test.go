// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "placeholder-remedy-names-the-source" entry.

package client

import (
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/config"
)

// TestPlaceholderRemedyNamesTheSource: a placeholder key from the environment
// is reported as the environment variable, not the config file.
func TestPlaceholderRemedyNamesTheSource(t *testing.T) {
	cfg := &config.Config{Path: "/home/x/config.toml", AuthSource: "env:DNSFILTER_API_KEY"}
	if got := authPlaceholderCredentialErrorWithSetup(cfg, "fix").Error(); !strings.Contains(got, "the DNSFILTER_API_KEY environment variable") || strings.Contains(got, "config.toml") {
		t.Fatalf("env placeholder: %q", got)
	}
	cfg.AuthSource = "config"
	if got := authPlaceholderCredentialErrorWithSetup(cfg, "fix").Error(); !strings.Contains(got, "/home/x/config.toml") {
		t.Fatalf("config placeholder: %q", got)
	}
}
