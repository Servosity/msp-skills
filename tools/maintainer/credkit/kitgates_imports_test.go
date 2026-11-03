// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Every connector vendors the kit, and the security gate scans it like
// connector code with no suppression available: the kit itself imports no
// unsafe and no cgo (its FIONREAD ioctl goes through golang.org/x/sys/unix).
func TestKitGatesNoUnsafeImport(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fs := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fs, f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range af.Imports {
			if p, _ := strconv.Unquote(im.Path.Value); p == "unsafe" || p == "C" {
				t.Errorf("%s imports %q", f, p)
			}
		}
	}
}
