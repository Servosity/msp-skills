// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"path/filepath"
	"testing"

	"kaseya-vsa-pp-cli/internal/cliutil"
	"kaseya-vsa-pp-cli/internal/cliutil/testenv"
)

// TestNoEnvSwitchDisablesTLSVerification: an environment variable must never
// turn certificate checks off for the connection that carries the API token.
func TestNoEnvSwitchDisablesTLSVerification(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, v := range []string{"1", "true", "yes", "on"} {
		t.Setenv("KASEYA_VSA_SKIP_TLS_VERIFY", v)
		cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.SkipTLSVerify {
			t.Fatalf("KASEYA_VSA_SKIP_TLS_VERIFY=%s disabled TLS verification", v)
		}
	}
}
