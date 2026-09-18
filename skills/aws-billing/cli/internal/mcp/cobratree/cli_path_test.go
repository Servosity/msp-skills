// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"path/filepath"
	"reflect"
	"testing"
)

// The companion-CLI bundle ships aws-billing-cli.exe next to aws-billing-mcp.exe
// on Windows. The 4.19.0 print stat'ed a single sibling name (the .exe on Windows,
// with no bare-name fallback), unlike the hudu-era candidate list. These pin
// the hudu-shaped resolution order: sibling .exe first on windows, then the
// bare name; only the bare name elsewhere; and the PATH lookup name per OS.

func TestSiblingCLICandidatesPerOS(t *testing.T) {
	exe := filepath.Join("C:", "bundle", "aws-billing-mcp.exe")
	dir := filepath.Dir(exe)
	cases := []struct {
		goos string
		want []string
	}{
		{"windows", []string{filepath.Join(dir, "aws-billing-cli.exe"), filepath.Join(dir, "aws-billing-cli")}},
		{"darwin", []string{filepath.Join(dir, "aws-billing-cli")}},
		{"linux", []string{filepath.Join(dir, "aws-billing-cli")}},
	}
	for _, tc := range cases {
		if got := siblingCLICandidates(tc.goos, exe); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("siblingCLICandidates(%q) = %v, want %v", tc.goos, got, tc.want)
		}
	}
}

func TestCLIExecutableNamePerOS(t *testing.T) {
	if got := cliExecutableName("windows"); got != "aws-billing-cli.exe" {
		t.Errorf("windows PATH lookup name = %q, want aws-billing-cli.exe", got)
	}
	for _, goos := range []string{"darwin", "linux"} {
		if got := cliExecutableName(goos); got != "aws-billing-cli" {
			t.Errorf("%s PATH lookup name = %q, want aws-billing-cli", goos, got)
		}
	}
}
