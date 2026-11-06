// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Class export-truncates-prior-file: a failed export leaves the file a previous
// good export wrote as it was. The file is written beside the target and renamed
// over it only once the export finished.
func TestFailedExportKeepsThePriorFile(t *testing.T) {
	t.Parallel()
	const prior = "{\"vm\":\"vm-1021\",\"name\":\"Prior-Good-Export\"}\n"
	for name, sv := range map[string]func(*testing.T) *server{
		"the list answers 404": func(t *testing.T) *server {
			return vmListVCenter(t, "x", http.StatusNotFound, `{"error_type":"NOT_FOUND"}`)
		},
		"the list is not JSON": func(t *testing.T) *server {
			return vmListVCenter(t, "x", http.StatusOK, `[{"vm":"vm-1021",`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, sv(t), true)
			file := filepath.Join(e.home, "existing.json")
			if err := os.WriteFile(file, []byte(prior), 0o600); err != nil {
				t.Fatal(err)
			}
			out, code := runCode(t, e, "export", "vm", "--format", "json", "--output", file)
			if code == 0 {
				t.Fatalf("the control failed: the export succeeded:\n%.1200s", out)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != prior {
				t.Fatalf("a failed export changed the prior file to %q (%v):\n%.1200s", got, err, out)
			}
			if leftovers, _ := filepath.Glob(filepath.Join(e.home, ".export-*")); len(leftovers) != 0 {
				t.Fatalf("the failed export left %v", leftovers)
			}
		})
	}
	t.Run("control: a good export replaces it", func(t *testing.T) {
		e := newEnv(t, quietServer(t), true)
		file := filepath.Join(e.home, "existing.json")
		if err := os.WriteFile(file, []byte(prior), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, code := runCode(t, e, "export", "vm", "--format", "json", "--output", file); code != 0 {
			t.Fatalf("export rc=%d:\n%.1200s", code, out)
		}
		got, err := os.ReadFile(file)
		if err != nil || !strings.Contains(string(got), "HQ-Record") || strings.Contains(string(got), "Prior-Good-Export") {
			t.Fatalf("the good export wrote %q (%v)", got, err)
		}
		if runtime.GOOS == "windows" {
			return // Windows reports 0666 for every file
		}
		if info, err := os.Stat(file); err == nil && info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("the export left mode %v", info.Mode().Perm())
		}
	})
}
