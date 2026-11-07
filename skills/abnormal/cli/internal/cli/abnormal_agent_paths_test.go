// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"runtime"
	"strings"
	"testing"
)

// Class own-paths-read-as-escapes: on Windows every directory agent-context
// prints is a backslash path, and a segment such as \001 (the last element of
// every t.TempDir there) or \2024 reads as an octal string escape. With any
// credential held, the kit then decodes the whole 78 KB description in every
// escape reading and withholds it (CI windows row, PR #554). Windows accepts
// forward slashes, so the CLI prints its own paths in that form there.
func TestAgentContextPathsCarryNoBackslashOnWindows(t *testing.T) {
	for _, p := range []string{
		`C:\Users\RUNNER~1\AppData\Local\Temp\TestAgentContextIsNeverWithheld2813097316\001\AppData\Roaming\abnormal-cli`,
		`D:\2024\abnormal-cli`,
		`\\server\share\abnormal-cli`,
	} {
		got := agentContextPath("windows", p)
		if strings.Contains(got, `\`) {
			t.Errorf("windows %q printed as %q: want no backslash", p, got)
		}
		if strings.ReplaceAll(p, `\`, "/") != got {
			t.Errorf("windows %q printed as %q: want the same path with forward slashes", p, got)
		}
	}
	// A backslash is an ordinary file name character elsewhere: never rewritten.
	for _, goos := range []string{"linux", "darwin"} {
		if p := `/home/a\b/abnormal-cli`; agentContextPath(goos, p) != p {
			t.Errorf("%s rewrote %q", goos, p)
		}
	}
}

// The builder prints every directory through agentContextPath: on Windows
// none carries a backslash.
func TestAgentContextPrintsItsPathsThroughTheWindowsForm(t *testing.T) {
	paths := buildAgentContextPaths()
	for name, got := range map[string]string{"config_dir": paths.ConfigDir, "data_dir": paths.DataDir, "state_dir": paths.StateDir, "cache_dir": paths.CacheDir} {
		if got == "" {
			t.Errorf("%s is empty", name)
		}
		if runtime.GOOS == "windows" && strings.Contains(got, `\`) {
			t.Errorf("%s = %q: want forward slashes on Windows", name, got)
		}
	}
}
