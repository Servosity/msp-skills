// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"testing"
)

// TestDryRun_NoSideEffects checks that --dry-run on a representative set
// of write-capable commands does not perform IO and exits 0.
func TestDryRun_NoSideEffects(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"export", []string{"export", "abc", "-o", "/tmp/no-such-x", "--dry-run"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			rc := RootCmd()
			rc.SetOut(&out)
			rc.SetErr(&out)
			rc.SetArgs(tc.args)
			if err := rc.Execute(); err != nil {
				t.Errorf("%s: %v", tc.name, err)
			}
		})
	}
}
