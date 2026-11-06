// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kbmsblackbox

// Fix round 1, class "profile store bricks startup / profile credentials held
// precisely". profiles.json was a Declarations.Files entry, so the kit read it
// as a credential file and held every string in it ("profiles", "daily",
// "name", "values", "select", "Id"), and ordinary program text then held a
// startup credential: after one `profile save`, every command refused to
// start. A saved profile value under a credential flag's name (password,
// client-secret, mfa-code, code) or under base-url is a source (publish-common
// kit item 8(c)), held at startup, every duplicate included; every other
// value, an account code among them, is not a credential and still prints.

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const refusedToStart = "credkit: refusing to start"

// runCode runs the CLI in env and returns everything it printed and its exit
// status. A run that does not finish in a minute fails the test.
func runCode(t *testing.T, env []string, stdin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env, cmd.Dir = env, strings.TrimPrefix(env[1], "HOME=")
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.String(), ee.ExitCode()
		}
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String(), 0
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("%v: did not finish in 60s:\n%.1500s", args, out.String())
		return "", -1
	}
}

// profileStoreUnder is the one profiles.json the program wrote under home.
func profileStoreUnder(t *testing.T, home string) string {
	t.Helper()
	var found []string
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "profiles.json" {
			found = append(found, path)
		}
		return nil
	})
	if len(found) != 1 {
		t.Fatalf("want one profiles.json under the home, got %v", found)
	}
	return found[0]
}

// profileEnv is a clean home whose API host refuses connections at once, so
// doctor's reachability probe answers the same way on every run and machine.
func profileEnv(t *testing.T) []string {
	t.Helper()
	return cleanEnv(t.TempDir(), "KASEYA_BMS_BASE_URL=https://127.0.0.1:1")
}

func TestSavedProfileNeverRefusesStartup(t *testing.T) {
	t.Parallel() // its own home: independent of every other test
	env := profileEnv(t)
	home := strings.TrimPrefix(env[1], "HOME=")
	// What each command does on a home with no profile store yet.
	probes := [][]string{{"profile", "list", "--json"}, {"doctor", "--json"}, {"version"}}
	before := map[string]int{}
	for _, p := range probes {
		out, code := runCode(t, env, "", p...)
		if strings.Contains(out, refusedToStart) {
			t.Fatalf("%v refused to start before any save:\n%.1500s", p, out)
		}
		before[strings.Join(p, " ")] = code
	}
	check := func(after string, also ...[]string) {
		t.Helper()
		for _, p := range append(probes, also...) {
			out, code := runCode(t, env, "", p...)
			if strings.Contains(out, refusedToStart) {
				t.Errorf("after %s, %v refused to start (exit %d):\n%.1500s", after, p, code, out)
				continue
			}
			if want, ok := before[strings.Join(p, " ")]; ok && code != want {
				t.Errorf("after %s, %v exited %d, before it exited %d:\n%.1500s", after, p, code, want, out)
			}
		}
	}

	out, code := runCode(t, env, "", "profile", "save", "daily", "--select", "Id")
	if code != 0 || !strings.Contains(out, `saved profile "daily"`) {
		t.Fatalf("profile save daily --select Id: exit %d:\n%.1500s", code, out)
	}
	profileStoreUnder(t, home)
	check("profile save daily --select Id", []string{"profile", "show", "daily"})
	if out, code := runCode(t, env, "", "profile", "show", "daily"); code != 0 || !strings.Contains(out, `"Id"`) {
		t.Errorf("profile show daily: exit %d, want the saved --select Id shown:\n%.1500s", code, out)
	}
	if out, code := runCode(t, env, "", "profile", "list"); code != 0 || !strings.Contains(out, "daily") {
		t.Errorf("profile list: exit %d, want daily listed:\n%.1500s", code, out)
	}
	if out, code := runCode(t, env, "", "profile", "use", "daily"); code != 0 || !strings.Contains(out, "--select Id") {
		t.Errorf("profile use daily: exit %d, want --select Id:\n%.1500s", code, out)
	}

	// A profile's name is no credential, the name of a credential flag
	// included: its profile is read and printed like any other.
	for _, name := range []string{"password", "secret", "client-id", "code", "base-url"} {
		if out, code := runCode(t, env, "", "profile", "save", name, "--select", "Id"); code != 0 {
			t.Fatalf("profile save %s: exit %d:\n%.1500s", name, code, out)
		}
		if out, code := runCode(t, env, "", "profile", "show", name); code != 0 || !strings.Contains(out, `"Id"`) || !strings.Contains(out, `"`+name+`"`) {
			t.Errorf("profile show %s: exit %d, want the profile shown:\n%.1500s", name, code, out)
		}
	}
	if out, code := runCode(t, env, "", "profile", "list"); code != 0 || !strings.Contains(out, "client-id") || !strings.Contains(out, "daily") {
		t.Errorf("profile list: exit %d, want every saved profile listed:\n%.1500s", code, out)
	}
	check("profiles named after credential flags", []string{"profile", "show", "password"})

	// Every other command that writes a file the startup path reads.
	deliverTo := filepath.Join(home, "delivered.json")
	if out, code := runCode(t, env, "", "profile", "save", "weekly", "--select", "Id", "--deliver", "file:"+deliverTo, "--description", "weekly report"); code != 0 {
		t.Fatalf("profile save weekly --deliver: exit %d:\n%.1500s", code, out)
	}
	check("profile save --deliver", []string{"profile", "show", "weekly"})
	if out, code := runCode(t, env, "SetTok-6Wq2Nz8Rk4Lp\n", "auth", "set-token"); code != 0 {
		t.Fatalf("auth set-token: exit %d:\n%.1500s", code, out)
	}
	check("auth set-token", []string{"profile", "show", "daily"}, []string{"auth", "status"})
	if out, code := runCode(t, env, "", "auth", "logout"); code != 0 {
		t.Fatalf("auth logout: exit %d:\n%.1500s", code, out)
	}
	check("auth logout", []string{"profile", "show", "daily"})
	if out, code := runCode(t, env, "", "profile", "delete", "weekly", "--yes"); code != 0 {
		t.Fatalf("profile delete weekly --yes: exit %d:\n%.1500s", code, out)
	}
	check("profile delete")

	// The MCP server reads the same declarations at startup.
	e := &bbEnv{home: home, env: env}
	if mcp := e.mcpPhases(t); strings.Contains(mcp, refusedToStart) || !strings.Contains(mcp, "serverInfo") {
		t.Errorf("the MCP server did not start after a profile save:\n%.1500s", mcp)
	}
}

// Fixture credentials: no 8-character piece of any is a word a message prints.
const (
	pfPassword1    = "Dp1a-Zx" + "7cB4nM8q" // the first of two "password" members
	pfPassword2    = "Dp2b-Lk" + "5jH3gF9w" // the second, the one the program reads
	pfClientSecret = "Cs4n-Qy" + "8vT2hJ6d"
	pfMFACode      = "Mf9x-Rk2pW7cN3t"
	pfCode1        = "Cd5h-Jt8zV1mQ4w" // the first of two "code" members
	pfCode2        = "X9b2K7m4Q8p3N6r5"
	pfOldCode      = "Oc3s-Wy6bK9dT2v"      // a "code" in a profile a later duplicate name replaces
	pfURLPassword  = "Ub6r-Xn" + "3qL8sK2y" // the password inside a saved base-url
	pfURLQuery     = "Uq2w-Hd7mP4zR9c"      // a query value inside it
	pfURLPassword2 = "Us3k-Vb" + "9tN6pM1x" // the password inside a saved base_url
	pfAccountCode  = "ACME-HQ01"            // an account code: not a credential
)

func profileSecrets() []string {
	return []string{pfPassword1, pfPassword2, pfClientSecret, pfMFACode, pfCode1, pfCode2, pfOldCode,
		pfURLPassword, pfURLQuery, pfURLPassword2}
}

// profileFixture is a store as a person or an older build may leave it: the
// credential values a run profile refuses to save today, duplicate members,
// a duplicate profile name, and "Values" spelled as Go's decoder also reads
// it. Each credential is in a profile of its own, so the kit's withholding of
// one value never hides another that was not held.
func profileFixture() string {
	return `{"profiles":{` +
		`"daily":{"name":"daily","description":"nightly sync","values":{"select":"Id","account-code":"` + pfAccountCode + `"}},` +
		`"ops-pw":{"name":"ops-pw","values":{"password":"` + pfPassword1 + `","password":"` + pfPassword2 + `","select":"Id"}},` +
		`"ops-cs":{"name":"ops-cs","values":{"client-secret":"` + pfClientSecret + `","select":"Id"}},` +
		`"ops-mfa":{"name":"ops-mfa","values":{"mfa-code":"` + pfMFACode + `","select":"Id"}},` +
		`"ops-code":{"name":"ops-code","values":{"code":"` + pfCode1 + `","code":"` + pfCode2 + `","select":"Id"}},` +
		`"ops-url":{"name":"ops-url","values":{"base-url":"https://ops:` + pfURLPassword + `@api.bms.kaseya.com/v2","select":"Id"}},` +
		`"ops-query":{"name":"ops-query","values":{"base-url":"https://api.bms.kaseya.com/v2?key=` + pfURLQuery + `","select":"Id"}},` +
		`"svc":{"name":"svc","Values":{"base_url":"https://svc:` + pfURLPassword2 + `@api.bms.kaseya.com/"}},` +
		`"old":{"name":"old","values":{"code":"` + pfOldCode + `"}},` +
		`"old":{"name":"old","values":{"select":"Id"}}` +
		`}}`
}

// canaryFixture is the same store plus one profile per value the program
// would never print (an earlier duplicate member, a profile a later
// duplicate name replaces), each printing that value as its description, so
// it is seen to be held.
func canaryFixture() string {
	return strings.TrimSuffix(profileFixture(), `}}`) + `,` +
		`"c-pw1":{"name":"c-pw1","description":"` + pfPassword1 + `","values":{"select":"Id"}},` +
		`"c-code1":{"name":"c-code1","description":"` + pfCode1 + `","values":{"select":"Id"}},` +
		`"c-old":{"name":"c-old","description":"` + pfOldCode + `","values":{"select":"Id"}}` +
		`}}`
}

// storeEnv is a clean home whose profile store, written by the program
// first, is then replaced by fixture.
func storeEnv(t *testing.T, fixture string) []string {
	t.Helper()
	env := profileEnv(t)
	if out, code := runCode(t, env, "", "profile", "save", "seed", "--select", "Id"); code != 0 {
		t.Fatalf("profile save seed: exit %d:\n%.1500s", code, out)
	}
	store := profileStoreUnder(t, strings.TrimPrefix(env[1], "HOME="))
	if err := os.WriteFile(store, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	return env
}

func assertNoProfileSecret(t *testing.T, env []string, runs ...[]string) {
	t.Helper()
	for _, r := range runs {
		out, code := runCode(t, env, "", r...)
		if strings.Contains(out, refusedToStart) {
			t.Errorf("%v refused to start (exit %d):\n%.1500s", r, code, out)
		}
		for _, s := range profileSecrets() {
			if holdsForm(out, s) {
				t.Errorf("%v prints the saved credential %q:\n%.1500s", r, s, out)
			}
		}
	}
}

func TestSavedProfileCredentialsNeverPrint(t *testing.T) {
	t.Parallel() // its own home: independent of every other test
	env := storeEnv(t, profileFixture())
	var runs [][]string
	for _, name := range []string{"ops-pw", "ops-cs", "ops-mfa", "ops-code", "ops-url", "ops-query", "svc"} {
		runs = append(runs, []string{"profile", "show", name}, []string{"profile", "show", name, "--json"},
			[]string{"profile", "use", name}, []string{"profile", "use", name, "--json"})
	}
	runs = append(runs, []string{"profile", "show", "old"}, []string{"profile", "list"}, []string{"profile", "list", "--json"},
		[]string{"doctor", "--json"})
	assertNoProfileSecret(t, env, runs...)
	// Values that are no credential still print, and the store is still read.
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"profile", "show", "daily"}, []string{`"Id"`, pfAccountCode, "nightly sync"}},
		{[]string{"profile", "use", "daily"}, []string{"--select Id", "--account-code " + pfAccountCode}},
		{[]string{"profile", "show", "old"}, []string{`"Id"`}},
		{[]string{"profile", "list", "--json"}, []string{`"daily"`, `"ops-pw"`, `"ops-code"`, `"svc"`, `"old"`, "nightly sync"}},
	} {
		out, code := runCode(t, env, "", c.args...)
		if code != 0 {
			t.Errorf("%v: exit %d:\n%.1500s", c.args, code, out)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v does not print %q, which is no credential:\n%.1500s", c.args, w, out)
			}
		}
	}
	// Every duplicate is held, the members the program never reads included.
	canary := storeEnv(t, canaryFixture())
	assertNoProfileSecret(t, canary,
		[]string{"profile", "show", "c-pw1"}, []string{"profile", "show", "c-code1"}, []string{"profile", "show", "c-old"})
	if out, code := runCode(t, canary, "", "profile", "show", "daily"); code != 0 || !strings.Contains(out, pfAccountCode) {
		t.Errorf("profile show daily with the canaries saved: exit %d, want the account code shown:\n%.1500s", code, out)
	}
}
