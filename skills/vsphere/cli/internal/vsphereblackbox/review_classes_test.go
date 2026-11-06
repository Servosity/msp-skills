// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCode runs the CLI and returns everything it printed with its exit code.
func runCode(t *testing.T, e *bbEnv, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env, cmd.Dir = e.env, e.home
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(out), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return string(out), 0
}

// serverEnv points e at sv, replacing the address and the trust root.
func serverEnv(e *bbEnv, sv *server) []string {
	return append(append([]string(nil), e.env...), "VSPHERE_BASE_URL="+sv.url, "VSPHERE_CA_CERT="+sv.caFile)
}

// Class credential-in-url-setting, the unparsable half: a base URL with a typo
// still holds its password, before a retry line, an error or doctor repeats it.
func TestBrokenBaseURLNeverPrintsItsPassword(t *testing.T) {
	t.Parallel()
	const urlPw = "UrlSecret-55443322"
	// With no "@" there is no userinfo, and the kit's URL reader (credkit.HoldURL)
	// reads none: what follows "u:" is a port to it. The deleted hand-written
	// reader held it; this connector no longer reads URLs itself (Kit notes,
	// .review/more-pass h1 report).
	noUserinfo := map[string]bool{
		"https://u:" + urlPw: true, "https:/u:" + urlPw: true, "https:u:" + urlPw: true,
		"1https://u:" + urlPw: true, "https ://u:" + urlPw: true,
	}
	for _, base := range []string{
		"https//u:" + urlPw + "@127.0.0.1:1",
		"u:" + urlPw + "@127.0.0.1:1",
		"ht!tp://u:" + urlPw + "@127.0.0.1:1",
		"https://u:" + urlPw,
		"https:/u:" + urlPw,
		"https:u:" + urlPw,
		"1https://u:" + urlPw,
		"https ://u:" + urlPw,
	} {
		t.Run(base, func(t *testing.T) {
			if noUserinfo[base] {
				t.Skip("kit note: credkit.HoldURL reads no password from an address with no \"@\"")
			}
			t.Parallel()
			e := newEnv(t, quietServer(t), true)
			env := append(append([]string(nil), e.env...), "VSPHERE_BASE_URL="+base)
			fromEnv := &bbEnv{home: e.home, env: env}
			for _, args := range [][]string{{"vm", "list", "--json"}, {"doctor"}} {
				if out, _ := runCode(t, fromEnv, args...); strings.Contains(out, urlPw) {
					t.Errorf("env, %v printed the password:\n%.1200s", args, out)
				}
			}
			dir := filepath.Join(e.home, "cfg")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			cfg := filepath.Join(dir, "c.toml")
			if err := os.WriteFile(cfg, []byte("base_url = \""+base+"\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if out, _ := runCode(t, e, "--config", cfg, "vm", "list", "--json"); strings.Contains(out, urlPw) {
				t.Errorf("config, printed the password:\n%.1200s", out)
			}
		})
	}
	t.Run("an address with no credential still names itself", func(t *testing.T) {
		e := newEnv(t, quietServer(t), true)
		e.env = append(e.env, "VSPHERE_BASE_URL=vcenter.example.com")
		if out, _ := runCode(t, e, "vm", "list", "--json"); strings.Contains(out, withheldNotice) {
			t.Fatalf("a plain host was withheld:\n%.1200s", out)
		}
	})
}

// Class credential-sent-to-another-vcenter: a saved password goes only to the
// vCenter it was saved for, however the address was changed afterwards. The
// environment selects the target; it does not re-point a saved credential.
func TestSavedCredentialsStayWithTheirVCenter(t *testing.T) {
	t.Parallel()
	logins := func(sv *server) int {
		n := 0
		for _, r := range sv.requests() {
			if r == "POST /api/session" || strings.HasSuffix(r, "/SessionManager/SessionManager/Login") {
				n++
			}
		}
		return n
	}
	t.Run("credentials file, environment names another vCenter", func(t *testing.T) {
		a, b := quietServer(t), quietServer(t)
		e := newEnv(t, a, false)
		e.run(t, "auth", "set-credentials", bbUser, bbStored)
		if e.run(t, "vm", "list", "--json"); logins(a) == 0 {
			t.Fatalf("the control failed: no login at the vCenter the password was saved for")
		}
		atB := &bbEnv{home: e.home, env: serverEnv(e, b)}
		if out, _ := runCode(t, atB, "vm", "list", "--json"); logins(b) != 0 {
			t.Fatalf("the password saved for A logged in at B:\n%.1200s", out)
		}
	})
	t.Run("config file, environment names another vCenter", func(t *testing.T) {
		a, b := quietServer(t), quietServer(t)
		e := newEnv(t, a, false)
		cfg := filepath.Join(e.home, "c.toml")
		body := "base_url = \"" + a.url + "\"\nusername = \"" + bbUser + "\"\npassword = \"" + bbStored + "\"\n"
		if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		e.run(t, "--config", cfg, "vm", "list", "--json")
		if logins(a) == 0 {
			t.Fatalf("the control failed: no login at the vCenter the config names")
		}
		atB := &bbEnv{home: e.home, env: serverEnv(e, b)}
		if out, _ := runCode(t, atB, "--config", cfg, "vm", "list", "--json"); logins(b) != 0 {
			t.Fatalf("the config's password for A logged in at B:\n%.1200s", out)
		}
	})
	t.Run("default config names another vCenter than the credentials file", func(t *testing.T) {
		a, b := quietServer(t), quietServer(t)
		e := newEnv(t, b, false)
		e.run(t, "auth", "set-credentials", bbUser, bbStored)
		logged := logins(b)
		var kept []string
		for _, kv := range e.env {
			if !strings.HasPrefix(kv, "VSPHERE_BASE_URL=") {
				kept = append(kept, kv)
			}
		}
		e.env = append(kept, "VSPHERE_CA_CERT="+a.caFile)
		dir := filepath.Join(e.home, ".config", "vsphere-cli")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("base_url = \""+a.url+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, _ := runCode(t, e, "vm", "list", "--json"); logins(a) != 0 || logins(b) != logged {
			t.Fatalf("the password saved for B reached A:\n%.1200s", out)
		}
	})
	t.Run("a password in the environment goes where the environment says", func(t *testing.T) {
		a, b := quietServer(t), quietServer(t)
		e := newEnv(t, a, true)
		atB := &bbEnv{home: e.home, env: serverEnv(e, b)}
		atB.run(t, "vm", "list", "--json")
		if logins(b) == 0 {
			t.Fatal("an operator's own environment was refused")
		}
	})
}

// Class store-shared-across-vcenters: one login on two vCenters keeps two local
// stores. A MoRef repeats across vCenters, so one store reports one's VM as the
// other's.
func TestLocalDataStaysWithItsVCenter(t *testing.T) {
	t.Parallel()
	a, b := quietServer(t), quietServer(t)
	e := newEnv(t, a, true)
	e.run(t, "sync", "--json")
	if out := e.run(t, "vm", "list", "--data-source", "local", "--json"); !strings.Contains(out, "HQ-Record") {
		t.Fatalf("the control failed: A's own store has no record:\n%.1200s", out)
	}
	atB := &bbEnv{home: e.home, env: serverEnv(e, b)}
	if out := atB.run(t, "vm", "list", "--data-source", "local", "--json"); strings.Contains(out, "HQ-Record") {
		t.Fatalf("B reported A's records:\n%.1200s", out)
	}
}

// Class export-false-success: a command that counts what it wrote never reports
// success over a file the kit took a record out of.
func TestExportsFailWhenARecordWasWithheld(t *testing.T) {
	t.Parallel()
	e := newEnv(t, echoServer(t), true)
	file := filepath.Join(e.home, "export.json")
	for _, format := range []string{"json", "jsonl"} {
		out, code := runCode(t, e, "export", "vm", "--format", format, "--output", file)
		if code == 0 || strings.Contains(out, "Exported ") {
			t.Errorf("export --format %s: rc=%d over a file with a record withheld:\n%.1200s", format, code, out)
		}
	}
	out, code := runCode(t, e, "rvtools", "export", "--out", filepath.Join(e.home, "rv"))
	if code == 0 {
		t.Errorf("rvtools export: rc=0 over tabs with a row withheld:\n%.1200s", out)
	}
	quiet := newEnv(t, quietServer(t), true)
	if out, code := runCode(t, quiet, "export", "vm", "--format", "json", "--output", filepath.Join(quiet.home, "export.json")); code != 0 || !strings.Contains(out, "Exported ") {
		t.Errorf("the control failed: export rc=%d:\n%.1200s", code, out)
	}
	if out, code := runCode(t, quiet, "rvtools", "export", "--out", filepath.Join(quiet.home, "rv")); code != 0 {
		t.Errorf("the control failed: rvtools export rc=%d:\n%.1200s", code, out)
	}
}
