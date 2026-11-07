// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json review-r1-kit-child-and-config-writes):
// the publish review's round-1 classes, each driven on the built binary.

package credwire

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A browser helper `auth setup --launch` starts is another program: it must
// not inherit the API key (contract section 5, child process environment).
func TestBrowserHelperInheritsNoCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper is rundll32 on Windows; a stand-in cannot replace it on PATH")
	}
	const key = "Kq7Zt3Lw" + "9Xv2Pn5R"
	bin := t.TempDir()
	dump := filepath.Join(t.TempDir(), "helper-env.txt")
	script := "#!/bin/sh\nenv > '" + dump + "'\n"
	for _, name := range []string{"open", "xdg-open"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	e := newEnv(t, nil, "AFI_API_KEY="+key)
	for i, kv := range e.vars {
		if strings.HasPrefix(kv, "PATH=") {
			e.vars[i] = "PATH=" + bin + string(os.PathListSeparator) + strings.TrimPrefix(kv, "PATH=")
		}
	}
	r := e.run(t, "", "auth", "setup", "--launch")
	var got []byte
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(dump); err == nil && len(b) > 0 {
			got = b
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got == nil {
		t.Fatalf("the browser helper never started (exit %d):\n%s", r.code, r.out)
	}
	if strings.Contains(string(got), key) {
		t.Fatalf("the browser helper inherited the API key:\n%s", got)
	}
}

// A base URL set in the environment for one run is not configuration: a save
// (auth set-token) must not persist it, and with it any password it carries.
func TestSetTokenPersistsNoEnvironmentBaseURL(t *testing.T) {
	const pw = "Wd4Hs8Jm2Qb6Tc9Y"
	e := newEnv(t, nil)
	e.vars = append(e.vars, "AFI_BASE_URL="+strings.Replace(e.url, "https://", "https://operator:"+pw+"@", 1))
	r := e.run(t, "Np3Gx7Rk5Vf2Lz8M\n", "auth", "set-token")
	if r.code != 0 {
		t.Fatalf("auth set-token exit %d:\n%s", r.code, r.out)
	}
	for p, body := range e.files(t) {
		if strings.Contains(body, pw) {
			t.Fatalf("%s carries the environment base URL's password:\n%s", p, body)
		}
	}
}
