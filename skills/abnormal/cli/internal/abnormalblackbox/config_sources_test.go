// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package abnormalblackbox

// Codex r2: a configuration or credential file's values reach diagnostics
// whether the file is used or refused, and a URL that does not parse is
// printed as written. A password inside any of them must never print: every
// file is held when its bytes are read (cliutil.HoldConfigFile), and the kit
// withholds the diagnostic that repeats it.

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
	// A global credential store recorded for one Abnormal host (its base_url
	// carries the password) and refused for a config that names another.
	h1 := t.TempDir()
	runWithIn(t, cleanEnv(h1, "ABNORMAL_BASE_URL=https://ro:"+storePw+"@abnormal-store.example.com"), storePw+"\n", "auth", "set-token")
	other := filepath.Join(h1, "other")
	_ = os.MkdirAll(other, 0o700)
	_ = os.WriteFile(filepath.Join(other, "config.toml"), []byte("base_url = \"https://other.example.com\"\n"), 0o600)
	refused := runWith(t, cleanEnv(h1, "ABNORMAL_CONFIG="+filepath.Join(other, "config.toml")), "version")
	if !strings.Contains(refused, "not applied") && !strings.Contains(refused, "credkit: withheld") {
		t.Fatalf("the refused-store case did not run (no refusal printed):\n%s", refused)
	}
	out.WriteString(refused)
	// Loose config files whose base_url does not parse (a bad port, a raw "/"
	// in the password, braces in the host). An environment credential
	// (ABNORMAL_API_TOKEN) is what makes the refusal print.
	// Windows has no group/other mode bits, so no config file is loose there.
	if runtime.GOOS != "windows" {
		for i, u := range []string{
			"https://ro:" + loosePw + "@other.example.com:bad/",
			"https://ro:" + slashPw + "@other.example.com/v1",
			"https://ro:" + loosePw + "@{tenant}.other.example.com/v1",
		} {
			h := t.TempDir()
			p := filepath.Join(h, "loose.toml")
			_ = os.WriteFile(p, []byte("base_url = \""+u+"\"\n"), 0o644)
			_ = os.Chmod(p, 0o644)
			got := runWith(t, cleanEnv(h, "ABNORMAL_CONFIG="+p, "ABNORMAL_API_TOKEN=Ev0wPnEw0vPn"), "version")
			if i != 1 && !strings.Contains(got, "refused") && !strings.Contains(got, "credkit: withheld") {
				t.Fatalf("the loose-config case did not run (no refusal printed):\n%s", got)
			}
			out.WriteString(got)
		}
	}
	for _, pw := range []string{storePw, loosePw, slashPw, "Passw0rd", "Ev0wPnEw0vPn"} {
		if strings.Contains(out.String(), pw) {
			t.Errorf("output repeats the file password %q:\n%s", pw, out.String())
		}
	}
}
