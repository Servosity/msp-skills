// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-missing-ca-no-panic).

package client

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/config"
)

// TestMissingCACertIsAnErrorNotAPanic: `abb tasks --ca-cert /missing/ca.pem`
// left the session manager without an HTTP client, and ConfigureDSM
// dereferenced it before the caller could report the bad CA file.
func TestMissingCACertIsAnErrorNotAPanic(t *testing.T) {
	testenv.Isolate(t)
	missing := filepath.Join(t.TempDir(), "missing", "ca.pem")
	c := New(&config.Config{BaseURL: "https://nas.example.lan:5001", SynologyAccount: "a", SynologyPassword: "PW-missing-ca-1", CACert: missing}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil); err == nil {
		t.Fatal("a missing CA file did not fail the request")
	}
}
