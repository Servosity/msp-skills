// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeperblackbox

// Class output-before-chokepoint (Codex r2): a configuration or credential file's
// values reach diagnostics whether the file is used or refused, and a URL that does
// not parse is printed as written. A password inside any of them must never print:
// every file value is held at startup, before any output is checked.

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
	// A global credential store recorded for one Service Mode host (its base_url
	// carries the password) and refused for a config that names another. The refusal
	// names the host, so the kit withholds it whole.
	h1 := shortTemp(t)
	runWithIn(t, cleanEnv(h1, "KEEPER_BASE_URL=https://ro:"+storePw+"@keeper-store.example.com"), storePw+"\n", "auth", "set-token")
	other := filepath.Join(h1, "other")
	_ = os.MkdirAll(other, 0o700)
	_ = os.WriteFile(filepath.Join(other, "config.toml"), []byte("base_url = \"https://other.example.com\"\n"), 0o600)
	refused := runWith(t, cleanEnv(h1, "KEEPER_CONFIG="+filepath.Join(other, "config.toml")), "version")
	if !strings.Contains(refused, bbWithheld) {
		t.Fatalf("the refused-store case did not run (no notice printed):\n%s", refused)
	}
	out.WriteString(refused)
	// The same refusal with no password in the store's URL prints in clear, so the
	// notice above is the password, not a refusal the kit always withholds.
	h2 := shortTemp(t)
	runWithIn(t, cleanEnv(h2, "KEEPER_BASE_URL=https://keeper-store.example.com"), storePw+"\n", "auth", "set-token")
	other2 := filepath.Join(h2, "other")
	_ = os.MkdirAll(other2, 0o700)
	_ = os.WriteFile(filepath.Join(other2, "config.toml"), []byte("base_url = \"https://other.example.com\"\n"), 0o600)
	clear := runWith(t, cleanEnv(h2, "KEEPER_CONFIG="+filepath.Join(other2, "config.toml")), "version")
	if !strings.Contains(clear, "not applied") || strings.Contains(clear, bbWithheld) {
		t.Fatalf("a refusal with no credential in it was not printed in clear:\n%s", clear)
	}
	out.WriteString(clear)
	// Loose config files whose base_url does not parse, one with a raw "/" in
	// the password.
	for i, u := range []string{"https://ro:" + loosePw + "@other.example.com:bad/", "https://ro:" + slashPw + "@other.example.com/api"} {
		h := shortTemp(t)
		p := filepath.Join(h, "loose.toml")
		_ = os.WriteFile(p, []byte("base_url = \""+u+"\"\n"), 0o644)
		makeLoose(t, p)
		got := runWith(t, cleanEnv(h, "KEEPER_CONFIG="+p, "KEEPER_API_KEY=EnvPass"+"w0rd2x"), "version")
		if i == 0 && !strings.Contains(got, "refused") && !strings.Contains(got, bbWithheld) {
			t.Fatalf("the loose-config case did not run (no refusal printed):\n%s", got)
		}
		out.WriteString(got)
	}
	for _, pw := range []string{storePw, loosePw, slashPw, "Passw0rd"} {
		if strings.Contains(out.String(), pw) {
			t.Errorf("output repeats the file password %q:\n%s", pw, out.String())
		}
	}
}
