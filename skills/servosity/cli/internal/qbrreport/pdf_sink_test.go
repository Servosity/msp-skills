// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the finished
// PDF reaches its destination through the credential kit, not from Chrome's
// own file write. Chrome's output is its own: only the kit's check of the
// bytes stands between a page and the --out file. A shell script named chrome
// stands in for the browser; the held value runs in a child of the test binary.

package qbrreport

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"servosity-msp-pp-cli/internal/credkit"
)

const pdfSinkHeld = "pdfsinkheld7Qz4kW"

// fakeChrome puts a script named chrome first on PATH: it writes body to the
// file its --print-to-pdf argument names.
func fakeChrome(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in browser is a POSIX shell script")
	}
	dir, err := os.MkdirTemp("", "qbr-bin-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	script := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in --print-to-pdf=*) printf '%s' '" + body + "' > \"${a#--print-to-pdf=}\";; esac; done\n"
	if err := os.WriteFile(filepath.Join(dir, "chrome"), []byte(script), 0o700); err != nil { // #nosec G306 -- a test's own stand-in executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRenderPDFPublishesThroughTheKit(t *testing.T) {
	const childEnv = "QBR_PDF_SINK_HELD_PASS"
	if os.Getenv(childEnv) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRenderPDFPublishesThroughTheKit$", "-test.v")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "--- PASS: TestRenderPDFPublishesThroughTheKit") {
			t.Fatalf("the held pass failed: %v\n%s", err, out)
		}
		return
	}
	credkit.Hold("config:auth_header", pdfSinkHeld)
	dir := t.TempDir()

	t.Run("a clean PDF reaches the destination", func(t *testing.T) {
		fakeChrome(t, "%PDF-1.7 clean page")
		out := filepath.Join(dir, "clean.pdf")
		if err := RenderPDF(&Report{}, out); err != nil {
			t.Fatal(err)
		}
		if b, err := os.ReadFile(out); err != nil || string(b) != "%PDF-1.7 clean page" {
			t.Fatalf("destination = %q, %v", b, err)
		}
	})

	t.Run("a PDF carrying a held value never reaches it", func(t *testing.T) {
		fakeChrome(t, "%PDF-1.7 "+pdfSinkHeld)
		out := filepath.Join(dir, "held.pdf")
		err := RenderPDF(&Report{}, out)
		if !errors.Is(err, credkit.ErrWithheld) {
			t.Errorf("RenderPDF = %v, want the kit's refusal", err)
		}
		if b, rerr := os.ReadFile(out); rerr == nil && strings.Contains(string(b), pdfSinkHeld) {
			t.Errorf("the destination carries the held value: %q", b)
		}
	})
}
