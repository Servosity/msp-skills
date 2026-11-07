// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/config"
)

// TestRegionHarnessPlaceholder guards hand-fix region-harness-placeholder:
// the verify harness's "mock" region resolves only under the harness env
// with a base-URL override; every other combination still fails closed.
func TestRegionHarnessPlaceholder(t *testing.T) {
	cfg := &config.Config{TemplateVars: map[string]string{"region": "mock"}}
	for _, tc := range []struct {
		verify, baseURL string
		ok              bool
	}{
		{"1", "http://127.0.0.1:9", true},
		{"", "http://127.0.0.1:9", false},
		{"1", "", false},
		{"", "", false},
	} {
		t.Setenv(cliutil.VerifyEnvVar, tc.verify)
		t.Setenv(cliutil.DogfoodEnvVar, "")
		t.Setenv("ESET_PROTECT_BASE_URL", tc.baseURL)
		got, err := esetRegion(cfg)
		if tc.ok && (err != nil || got != "eu") {
			t.Errorf("verify=%q base=%q: got %q, %v; want eu", tc.verify, tc.baseURL, got, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("verify=%q base=%q: region mock accepted as %q", tc.verify, tc.baseURL, got)
		}
	}
}
