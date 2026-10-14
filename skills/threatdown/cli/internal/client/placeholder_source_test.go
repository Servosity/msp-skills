// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"errors"
	"strings"
	"testing"

	"threatdown-pp-cli/internal/config"
)

// Adversarial review round 11 (2026-10-14): a placeholder from the
// environment was reported as "configured in config.toml", a file that held
// no credential and that auth login cannot override.
func TestPlaceholderErrorNamesTheEnvVar(t *testing.T) {
	cfg := &config.Config{Path: "/tmp/home/config.toml"}
	t.Setenv("THREATDOWN_CLIENT_ID", "admin")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "<your-client-secret>")
	err := authPlaceholderCredentialError(cfg)
	if !errors.Is(err, ErrPlaceholderCredential) || !strings.Contains(err.Error(), "$THREATDOWN_CLIENT_SECRET") || strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("env placeholder must name the variable, got %v", err)
	}
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	if err := authPlaceholderCredentialError(cfg); !strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("a file placeholder still names the file, got %v", err)
	}
}
