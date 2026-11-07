// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// WIN-TEST-HOME-ONLY-ISOLATION: on Windows os.UserHomeDir reads USERPROFILE,
// not HOME, so a test that sandboxes only HOME reads and writes the runner's
// real profile there (config_noconfigwrite_test.go failed this way on the
// Windows CI row). Every test in this module that redirects HOME must also
// redirect USERPROFILE, which testenv.Isolate does.
func TestNoTestIsolatesOnlyHOME(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		s := string(b)
		if strings.Contains(s, `Setenv("HOME"`) && !strings.Contains(s, `"USERPROFILE"`) {
			t.Errorf("%s sets HOME without USERPROFILE; use testenv.Isolate(t)", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
