// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

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
	}, append(trapEnv(), extra...)...)
}

func runWith(t *testing.T, env []string, args ...string) string {
	t.Helper()
	return runWithIn(t, env, "", args...)
}

func runWithIn(t *testing.T, env []string, stdin string, args ...string) string {
	t.Helper()
	count(&cliRuns, 1)
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
	if runtime.GOOS == "windows" {
		t.Skip("the loose-config path needs POSIX group/other mode bits, which Windows does not have")
	}
	t.Parallel() // its own fake runZero and home
	const storePw, loosePw, slashPw = "StorePassw0rd1x", "OnlyUrlPw-Q7r9x", "Sl4sh/Passw0rd"
	var out strings.Builder
	// A global credential store recorded for one runZero host (its base_url
	// carries the password) and refused for a config that names another.
	h1 := t.TempDir()
	out.WriteString(runWithIn(t, cleanEnv(h1, "RUNZERO_BASE_URL=https://ro:"+storePw+"@runzero-store.example.com/api/v1.0"),
		"OTStoreKey1x-9c3e5a7b\n", "auth", "set-token"))
	other := filepath.Join(h1, "other")
	_ = os.MkdirAll(other, 0o700)
	_ = os.WriteFile(filepath.Join(other, "config.toml"), []byte("base_url = \"https://other.example.com\"\n"), 0o600)
	refused := runWith(t, cleanEnv(h1, "RUNZERO_CONFIG="+filepath.Join(other, "config.toml")), "version")
	// A refusal that repeats a held value is withheld whole by the
	// credential kit: its notice is the refusal having printed.
	if !strings.Contains(refused, "not applied") && !strings.Contains(refused, kitNotice("stderr")) {
		t.Fatalf("the refused-store case did not run (no refusal printed):\n%s", refused)
	}
	out.WriteString(refused)
	// Loose config files whose base_url does not parse (a bad port, a raw "/"
	// in the password, braces in the host). An environment credential
	// (RUNZERO_API_KEY) is what makes the refusal print; on the last, a
	// stored key is present too.
	for i, u := range []string{
		"https://ro:" + loosePw + "@other.example.com:bad/",
		"https://ro:" + slashPw + "@other.example.com/v1",
		"https://ro:" + loosePw + "@{tenant}.other.example.com/v1",
		"https://ro:" + loosePw + "@other.example.com:bad/v1",
	} {
		h := t.TempDir()
		p := filepath.Join(h, "loose.toml")
		_ = os.WriteFile(p, []byte("base_url = \""+u+"\"\n"), 0o644)
		_ = os.Chmod(p, 0o644)
		if i == 3 {
			out.WriteString(runWithIn(t, cleanEnv(h), "OTStoredKey3x-2d8f6b4e\n", "auth", "set-token"))
		}
		got := runWith(t, cleanEnv(h, "RUNZERO_CONFIG="+p, "RUNZERO_API_KEY=CTEnvKey2x-6a1c9e3b"), "version") // gitleaks:allow synthetic test fixture
		if i != 1 && !strings.Contains(got, "refused") && !strings.Contains(got, kitNotice("stderr")) {
			t.Fatalf("the loose-config case did not run (no refusal printed):\n%s", got)
		}
		out.WriteString(got)
	}
	for _, pw := range []string{storePw, loosePw, slashPw, "Passw0rd", "OTStoreKey1x-9c3e5a7b", "CTEnvKey2x-6a1c9e3b", "OTStoredKey3x-2d8f6b4e"} {
		if strings.Contains(out.String(), pw) {
			t.Errorf("output repeats the file password or key %q:\n%s", pw, out.String())
		}
	}
}

// Class credential-source-precedence: every place the key can come from
// (RUNZERO_API_KEY, the credential store, the environment over a stored key
// or over a config file's api_key, a config file's api_key, auth_header or
// access_token, or api_key beside access_token) sends exactly the one key it
// should, in the one header Authorization: Bearer <key>, never another way
// (an X-Api-Key header, a second, doubled or Basic Authorization header, the
// query), from the CLI and the MCP server alike; the credentials a source
// holds but does not send never print, whole or typed back where the program
// repeats them.
func TestEveryCredentialSourceSendsOneKey(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	for mode := credStored; mode <= credConfigAccess; mode++ {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			var out strings.Builder
			for _, args := range [][]string{
				{"org", "get-assets", "--json"},
				{"runzero-export", "services-json", "--json"},
				{"account", "get-keys", "--json"},
				{"org", "get-asset", bbAssetID, "--json"},
				{"account", "get-organizations", "--json"},
				{"runzero-export", "certificates-json", "--json"},
				{"org", "create-scan", bbSiteID, "--agent-id", bbAgentID, "--targets", bbCIDR, "--yes", "--json"},
				{"auth", "status"},
				{"doctor"},
			} {
				out.WriteString(e.run(t, args...))
			}
			for _, held := range mode.held() {
				out.WriteString(e.run(t, "org", "get-asset", held, "--json"))
				out.WriteString(e.run(t, "account", "remove-key", held, "--dry-run"))
			}
			out.WriteString(e.mcpCalls(t, []map[string]any{
				read("org.get-assets", nil),
				read("org.get-asset", map[string]any{"asset_id": bbAssetID}),
				read("account.get-keys", nil),
				read("account.get-organizations", nil),
			}))
			// auth logout clears what the source stored; neither it nor the
			// status after it prints a credential.
			out.WriteString(e.run(t, "auth", "logout"))
			out.WriteString(e.run(t, "auth", "status"))
			got := out.String()
			// The fake echoes the key it was sent, so the credential kit
			// withholds each answer whole: its notice on stdout is the
			// battery having fetched.
			if !strings.Contains(got, bbHost) && !strings.Contains(got, kitNotice("stdout")) {
				t.Fatalf("the battery fetched nothing:\n%.2000s", got)
			}
			assertNoEcho(t, "output", got)
			if n, all := sv.sawKey(mode.sent()); n < 9 || len(all) != 1 {
				t.Errorf("want every request to carry Authorization: Bearer %s (the %s key), got %d of it and distinct values %q", mode.sent(), mode, n, all)
			}
			checkOneAuthWay(t, sv)
		})
	}
}
