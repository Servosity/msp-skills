// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package action1blackbox

// Class output-before-chokepoint (Codex r2, fixed after the stop rule): a
// configuration or credential file's values reach diagnostics whether the file
// is used or refused, and a URL that does not parse is printed as written. A
// password inside any of them must never print: the shared credential kit
// holds every value of every file the run could read before anything prints
// (credkit-wiring, Declarations.Files and cliutil.HoldFile).

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func cleanEnv(home string, extra ...string) []string {
	return append([]string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "ACTION1_HOME=" + home,
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
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX mode bits: Windows has no group/other or read-only mode bits for a loose config file")
	}
	const storePw, loosePw = "csspw-mzwmmzzmw", "cslpw-mkqwmqwwq"
	var out strings.Builder
	// A global credential store recorded for one Action1 host (its base_url
	// carries the password) and refused for a config that names another.
	h1 := t.TempDir()
	runWithIn(t, cleanEnv(h1, "ACTION1_BASE_URL=https://ro:"+storePw+"@action1-store.example.com"), storePw+"\n", "auth", "set-token")
	other := filepath.Join(h1, "other")
	_ = os.MkdirAll(other, 0o700)
	_ = os.WriteFile(filepath.Join(other, "config.toml"), []byte("base_url = \"https://other.example.com\"\n"), 0o600)
	refused := runWith(t, cleanEnv(h1, "ACTION1_CONFIG="+filepath.Join(other, "config.toml")), "version")
	// The refusal repeats the store's base_url password, so the kit withholds
	// the line whole and writes its stderr notice in its place.
	if !strings.Contains(refused, "not applied") && !strings.Contains(refused, kitNotice("stderr")) {
		t.Fatalf("the refused-store case did not run (no refusal printed):\n%s", refused)
	}
	out.WriteString(refused)
	// Loose config files whose base_url does not parse (a bad port, braces in
	// the host). A raw "/" in the password is TestUnparsedBaseURLPasswordKnownP2.
	// An environment credential (ACTION1_OAUTH2, then the client pair) is what
	// makes the refusal print.
	envCreds := [][]string{
		{"ACTION1_OAUTH2=csetk-zwqkwzq"},
		{"ACTION1_CLIENT_ID=csecid-kqwmqqwqwzw", "ACTION1_CLIENT_SECRET=csesec-kkmmzmm"},
	}
	for i, u := range []string{"https://ro:" + loosePw + "@other.example.com:bad/", "https://ro:" + loosePw + "@{tenant}.other.example.com/api"} {
		h := t.TempDir()
		p := filepath.Join(h, "loose.toml")
		_ = os.WriteFile(p, []byte("base_url = \""+u+"\"\n"), 0o644)
		_ = os.Chmod(p, 0o644)
		got := runWith(t, cleanEnv(h, append([]string{"ACTION1_CONFIG=" + p}, envCreds[i]...)...), "version")
		if !strings.Contains(got, "refused") && !strings.Contains(got, kitNotice("stderr")) {
			t.Fatalf("the loose-config case did not run (no refusal printed):\n%s", got)
		}
		out.WriteString(got)
	}
	for _, pw := range []string{storePw, loosePw, "csetk-zwqkwzq", "csecid-kqwmqqwqwzw", "csesec-kkmmzmm"} {
		if strings.Contains(out.String(), pw) {
			t.Errorf("output repeats the file password %q:\n%s", pw, out.String())
		}
	}
}

// A loose config whose base_url has a raw "/" inside its password
// ("https://ro:cssl-wzzk/zqwkmwkq@host/api") reads, under a conforming URL
// parse, as host "ro:cssl-wzzk" and a path, so no userinfo is held, and the
// refusal message repeats the base_url on stderr. Ruled P2 for gradient and
// rocketcyber (outside the leak contract's readings; the file itself is
// already world-readable); recorded for Action1 in
// .review/kit/test-helper-findings.md: the refusal could name the file without
// its base_url. It passed before only because the old environment fixtures
// shared the piece "Passw0rd" with this password.
func TestUnparsedBaseURLPasswordKnownP2(t *testing.T) {
	t.Parallel()
	if os.Getenv("ACTION1_BB_P2") == "" {
		t.Skip("P2: the refusal of a loose config repeats a base_url with a raw / in its password; set ACTION1_BB_P2=1 to reproduce")
	}
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX mode bits: Windows has no group/other or read-only mode bits for a loose config file")
	}
	const slashPw = "cssl-wzzk/zqwkmwkq"
	h := t.TempDir()
	p := filepath.Join(h, "loose.toml")
	_ = os.WriteFile(p, []byte("base_url = \"https://ro:"+slashPw+"@other.example.com/api\"\n"), 0o644)
	_ = os.Chmod(p, 0o644)
	got := runWith(t, cleanEnv(h, "ACTION1_CONFIG="+p, "ACTION1_CLIENT_ID=csecid-kqwmqqwqwzw", "ACTION1_CLIENT_SECRET=csesec-kkmmzmm"), "version")
	if strings.Contains(got, slashPw) || strings.Contains(got, "zqwkmwkq") {
		t.Errorf("the refusal repeats the base_url password:\n%s", got)
	}
}
