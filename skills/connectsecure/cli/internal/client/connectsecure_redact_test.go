// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards the hand-fix dry-run-redacts-secret-body.

package client

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"connectsecure-pp-cli/internal/config"
)

func TestDryRunRedactsSecretBody(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	c := New(&config.Config{BaseURL: "http://127.0.0.1:1"}, 5*time.Second, 0)
	c.DryRun = true
	body := map[string]any{"name": "nightly", "password": "SENTINEL-PW", "params": map[string]any{"username": "u", "community": "SENTINEL-PARAM"}}
	_, _, _ = c.PostWithParamsAndHeaders(context.Background(), "/r/company/scheduler", nil, body, nil)
	os.Stderr = orig
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if strings.Contains(string(out), "SENTINEL") {
		t.Fatalf("dry run echoed a secret:\n%s", out)
	}
	if !strings.Contains(string(out), "nightly") || !strings.Contains(string(out), "[redacted]") {
		t.Fatalf("dry run lost the non-secret preview:\n%s", out)
	}
}
