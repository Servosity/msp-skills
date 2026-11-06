// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package liongardblackbox

// Class output-before-chokepoint (Codex r2, fixed after the stop rule): a
// configuration or credential file's values reach diagnostics whether the file
// is used or refused, and a URL that does not parse is printed as written. A
// password inside any of them must never print: every file value is held when
// its bytes are read, and the chokepoint redacts the password of any URL it
// sees, held or not.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func cleanEnv(home string, extra ...string) []string {
	return append([]string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"APPDATA=" + filepath.Join(home, "AppData", "Roaming"), "LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
	}, extra...)
}

func runWith(t *testing.T, env []string, args ...string) string {
	t.Helper()
	return runWithIn(t, env, "", args...)
}

func runWithIn(t *testing.T, env []string, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env, cmd.Dir = env, strings.TrimPrefix(env[1], "HOME=")
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if _, isExit := err.(*exec.ExitError); err != nil && !isExit {
		t.Fatalf("%v: %v", args, err)
	}
	return string(out)
}

func TestConfigSourcesNeverPrintTheirCredentials(t *testing.T) {
	const storePw, loosePw, slashPw = "StorePassw0rd1x", "OnlyUrlPw-Q7r9x", "Sl4sh/Passw0rd"
	var out strings.Builder
	// A global credential store recorded for one Liongard host (its base_url
	// carries the password) and refused for a config that names another.
	h1 := t.TempDir()
	runWithIn(t, cleanEnv(h1, "LIONGARD_BASE_URL=https://ro:"+storePw+"@liongard-store.example.com"), storePw+"\n", "auth", "set-token")
	other := filepath.Join(h1, "other")
	_ = os.MkdirAll(other, 0o700)
	_ = os.WriteFile(filepath.Join(other, "config.toml"), []byte("base_url = \"https://other.example.com\"\n"), 0o600)
	refused := runWith(t, cleanEnv(h1, "LIONGARD_CONFIG="+filepath.Join(other, "config.toml")), "version")
	// A refusal that repeats a held value is withheld whole by the shared
	// credential kit: its notice is the refusal.
	if !strings.Contains(refused, "not applied") && !strings.Contains(refused, kitNotice) {
		t.Fatalf("the refused-store case did not run (no refusal printed):\n%s", refused)
	}
	out.WriteString(refused)
	// Loose config files whose base_url does not parse, one with a raw "/" in
	// the password. An environment credential (LIONGARD_API_KEY, then the
	// access key pair, then the session token alone) is what makes the
	// refusal print.
	envCreds := [][]string{
		{"LIONGARD_API_KEY=" + "EnvPas" + "sw0rd2x"},
		{"LIONGARD_ACCESS_KEY_ID=" + "EnvAcc" + "essKeyId-5c3a", "LIONGARD_ACCESS_KEY_SECRET=" + "EnvPas" + "sw0rd3x"},
		{"LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH=" + "EnvPas" + "sw0rd4x"},
	}
	for i, u := range []string{
		"https://ro:" + loosePw + "@other.example.com:bad/",
		"https://ro:" + slashPw + "@other.example.com/api/v1",
		"https://ro:" + loosePw + "@{instance}.other.example.com/api/v1",
	} {
		h := t.TempDir()
		p := filepath.Join(h, "loose.toml")
		if err := os.WriteFile(p, []byte("base_url = \""+u+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		loosenFile(t, p)
		got := runWith(t, cleanEnv(h, append([]string{"LIONGARD_CONFIG=" + p}, envCreds[i]...)...), "version")
		if i != 1 && !strings.Contains(got, "refused") && !strings.Contains(got, kitNotice) {
			t.Fatalf("the loose-config case did not run (no refusal printed):\n%s", got)
		}
		out.WriteString(got)
	}
	for _, pw := range []string{storePw, loosePw, slashPw, "Passw0rd", "EnvAccessKeyId-5c3a"} {
		if strings.Contains(out.String(), pw) {
			t.Errorf("output repeats the file password %q:\n%s", pw, out.String())
		}
	}
}

// A saved profile is a credential source: a hand-edited profiles.json whose
// value sits under a credential name is held before anything prints, so
// `profile use` withholds it. A control profile with no credential prints.
func TestSavedProfileCredentialHeld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvKey)
	const prof = "PrfHeld7c2eWq9Lm"
	dir := filepath.Join(e.home, ".config", "liongard-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	save := func(values string) {
		t.Helper()
		body := `{"profiles":{"p":{"name":"p","values":{` + values + `}}}}`
		if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	save(`"limit":"25"`)
	if out := e.run(t, "profile", "use", "p"); !strings.Contains(out, "--limit 25") {
		t.Fatalf("profile use did not read the saved profile; the test did not run:\n%.1500s", out)
	}
	save(`"password":"` + prof + `","limit":"25"`)
	out := e.run(t, "profile", "use", "p")
	if strings.Contains(out, prof) || !strings.Contains(out, kitNotice) {
		t.Fatalf("a saved profile's credential was not withheld:\n%.1500s", out)
	}
}

// A saved profile's base-url is a credential source too (contract section 1):
// a password or query value inside it is held at startup, in every profile, so
// neither `profile show` nor `profile use` can print it.
func TestSavedProfileBaseURLHeld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvKey)
	const tok, pw = "R7qnUrlTok2xT9wL4zm", "PwInUrl8kQ3vZr6Yt"
	dir := filepath.Join(e.home, ".config", "liongard-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"profiles":{"a":{"name":"a","values":{"limit":"5"}},"p":{"name":"p","values":{"base-url":"https://ops:` + pw +
		`@tenant.app.liongard.com/api/v1?token=` + tok + `"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"profile", "show", "p", "--json", "--no-learn"}, {"profile", "use", "p", "--no-learn"}, {"profile", "list", "--json", "--no-learn"}} {
		out := e.run(t, args...)
		for _, secret := range []string{tok, pw} {
			if strings.Contains(out, secret) || strings.Contains(out, secret[:8]) {
				t.Errorf("%v printed a credential from a saved profile's base-url:\n%.1500s", args, out)
			}
		}
	}
}
