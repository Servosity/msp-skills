// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package knowbe4blackbox

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

	"knowbe4-pp-cli/internal/credkit"
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
	// A global credential store recorded for one KnowBe4 host (its base_url
	// carries the password) and refused for a config that names another.
	h1 := t.TempDir()
	runWithIn(t, cleanEnv(h1, "KNOWBE4_BASE_URL=https://ro:"+storePw+"@knowbe4-store.example.com"), storePw+"\n", "auth", "set-token")
	other := filepath.Join(h1, "other")
	_ = os.MkdirAll(other, 0o700)
	_ = os.WriteFile(filepath.Join(other, "config.toml"), []byte("base_url = \"https://other.example.com\"\n"), 0o600)
	refused := runWith(t, cleanEnv(h1, "KNOWBE4_CONFIG="+filepath.Join(other, "config.toml")), "version")
	if !strings.Contains(refused, "not applied") && !strings.Contains(refused, credkit.Notice(credkit.SinkStderr)) {
		t.Fatalf("the refused-store case did not run (no refusal printed):\n%s", refused)
	}
	out.WriteString(refused)
	// Loose config files whose base_url does not parse (a bad port, a raw "/"
	// in the password, braces in the host). An environment credential
	// (KNOWBE4_API_KEY) is what makes the refusal print, with the User Event
	// key beside it; on the last, a stored token is present too.
	const envHeldA, envHeldB = "EnvPassw0rd2x", "EnvEventPassw0rd5x"
	for i, u := range []string{
		"https://ro:" + loosePw + "@other.example.com:bad/",
		"https://ro:" + slashPw + "@other.example.com/v1",
		"https://ro:" + loosePw + "@{tenant}.other.example.com/v1",
		"https://ro:" + loosePw + "@other.example.com:bad/v1",
	} {
		h := t.TempDir()
		p := filepath.Join(h, "loose.toml")
		if err := os.WriteFile(p, []byte("base_url = \""+u+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		makeLoose(t, p)
		if i == 3 {
			runWithIn(t, cleanEnv(h), "StoredPassw0rd3x\n", "auth", "set-token")
		}
		got := runWith(t, cleanEnv(h, "KNOWBE4_CONFIG="+p, "KNOWBE4_API_KEY="+envHeldA, "KNOWBE4_USER_EVENT_API_KEY="+envHeldB), "version")
		if i != 1 && !strings.Contains(got, "refused") && !strings.Contains(got, credkit.Notice(credkit.SinkStderr)) {
			t.Fatalf("the loose-config case did not run (no refusal printed):\n%s", got)
		}
		out.WriteString(got)
	}
	for _, pw := range []string{storePw, loosePw, slashPw, "Passw0rd", envHeldA, envHeldB, "StoredPassw0rd3x"} {
		if strings.Contains(out.String(), pw) {
			t.Errorf("output repeats the file password %q:\n%s", pw, out.String())
		}
	}
}

// Class credential-source-precedence: every place the Reporting key can come
// from (KNOWBE4_API_KEY, the credential store, the environment over a stored
// key, a config file's api_key or access_token, or both config keys) sends
// exactly the one key it should, in the one header Authorization: Bearer
// <key>, never another way (an X-Api-Key header, a second or scheme-less
// Authorization header, the query), from the CLI and the MCP server alike;
// the User Event key (KNOWBE4_USER_EVENT_API_KEY) goes the same way to its own
// host only, and neither key ever reaches the other API's host; the
// credentials a source holds but does not send never print, whole or typed
// back where the program repeats them.
func TestEveryCredentialSourceSendsOneKey(t *testing.T) {
	t.Parallel()
	for mode := credStored; mode <= credConfigBoth; mode++ {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := quietServer(t)
			e := newEnv(t, sv, mode)
			var out strings.Builder
			for _, args := range [][]string{
				{"users", "list", "--json"},
				{"users", "get", bbUserID, "--json"},
				{"store-purchases", "list", "--json"},
				{"account", "info", "--json"},
				{"events", "list", "--json"},
				{"events", "get", bbEventID, "--json"},
				{"events", "create", "--target-user", bbClickEmail, "--event-type", bbEventType, "--yes", "--json"},
				{"auth", "status"},
				{"doctor"},
			} {
				out.WriteString(e.run(t, args...))
			}
			for _, held := range mode.held() {
				out.WriteString(e.run(t, "users", "get", held, "--json"))
				out.WriteString(e.run(t, "events", "create", "--target-user", bbClickEmail, "--event-type", bbEventType, "--description", "HQ "+held, "--dry-run"))
			}
			out.WriteString(e.mcpCalls(t, []map[string]any{
				tool("users_list", map[string]any{}),
				tool("users_get", map[string]any{"id": bbUserID}),
				tool("events_list", map[string]any{}),
				tool("events_types", map[string]any{}),
			}))
			// auth logout clears what the source stored; neither it nor the
			// status after it prints a credential.
			out.WriteString(e.run(t, "auth", "logout"))
			out.WriteString(e.run(t, "auth", "status"))
			got := out.String()
			if !strings.Contains(got, "HQ-Record") || !strings.Contains(got, bbEventType) {
				t.Fatalf("the battery fetched nothing from one of the APIs:\n%.2000s", got)
			}
			assertNoEcho(t, "output", got)
			if n, all := sv.sawKey(mode.sent()); n < 5 || len(all) != 1 {
				t.Errorf("want every Reporting request to carry Authorization: Bearer %s (the %s key), got %d of it and distinct values %q", mode.sent(), mode, n, all)
			}
			if n, all := sv.ev.sawKey(bbEventHeld); n < 4 || len(all) != 1 {
				t.Errorf("want every User Event request to carry Authorization: Bearer %s, got %d of it and distinct values %q", bbEventHeld, n, all)
			}
			checkKeysStayHome(t, sv)
		})
	}
}
