// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"path/filepath"
	"reflect"
	"testing"
)

// The companion-CLI bundle ships riverside-fm-cli.exe next to riverside-fm-mcp.exe
// on Windows. The 4.4.0 print stat'ed only the extensionless sibling, so a
// Windows bundle never found its own CLI and fell through to PATH. These pin
// the hudu-shaped resolution order: sibling .exe first on windows, then the
// bare name; only the bare name elsewhere; and the PATH lookup name per OS.

func TestSiblingCLICandidatesPerOS(t *testing.T) {
	exe := filepath.Join("C:", "bundle", "riverside-fm-mcp.exe")
	dir := filepath.Dir(exe)
	cases := []struct {
		goos string
		want []string // the leading candidates, in order
	}{
		{"windows", []string{filepath.Join(dir, "riverside-fm-cli.exe"), filepath.Join(dir, "riverside-fm-cli")}},
		{"darwin", []string{filepath.Join(dir, "riverside-fm-cli")}},
		{"linux", []string{filepath.Join(dir, "riverside-fm-cli")}},
	}
	for _, tc := range cases {
		got := siblingCLICandidates(tc.goos, "amd64", exe)
		if len(got) < len(tc.want) || !reflect.DeepEqual(got[:len(tc.want)], tc.want) {
			t.Errorf("siblingCLICandidates(%q) = %v, want it to start with %v", tc.goos, got, tc.want)
		}
		for _, c := range got {
			if filepath.Dir(c) != dir {
				t.Errorf("siblingCLICandidates(%q) left the bundle dir: %q", tc.goos, c)
			}
		}
	}
}

func TestCLIExecutableNamePerOS(t *testing.T) {
	if got := cliExecutableName("windows"); got != "riverside-fm-cli.exe" {
		t.Errorf("windows PATH lookup name = %q, want riverside-fm-cli.exe", got)
	}
	for _, goos := range []string{"darwin", "linux"} {
		if got := cliExecutableName(goos); got != "riverside-fm-cli" {
			t.Errorf("%s PATH lookup name = %q, want riverside-fm-cli", goos, got)
		}
	}
}
