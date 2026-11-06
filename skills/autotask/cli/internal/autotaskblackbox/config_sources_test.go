// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

// Class output-before-chokepoint (Codex r2, fixed after the stop rule): a
// configuration or credential file's values reach diagnostics whether the file
// is used or refused, and a URL that does not parse is printed as written. A
// password inside any of them must never print: every file value is held when
// its bytes are read (the kit's startup read, cliutil.HoldConfigFile after it),
// and a diagnostic that repeats one is withheld whole.

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
		// Nothing leaves the machine: a non-loopback host meets the recording proxy.
		"HTTPS_PROXY=" + proxyURL, "HTTP_PROXY=" + proxyURL, "https_proxy=" + proxyURL, "http_proxy=" + proxyURL, "NO_PROXY=", "no_proxy=",
	}, extra...)
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
	t.Parallel()
	const storePw, loosePw, slashPw = "StorePassw0rd1x", "OnlyUrlPw-Q7r9x", "Sl4sh/Passw0rd"
	var out strings.Builder
	// A global credential store recorded for one Autotask host (its base_url
	// carries the password) and refused for a config that names another.
	h1 := t.TempDir()
	runWithIn(t, cleanEnv(h1, "AUTOTASK_BASE_URL=https://ro:"+storePw+"@webservices-store.example.com"+apiBase), storePw+"\n", "auth", "set-token")
	other := filepath.Join(h1, "other")
	_ = os.MkdirAll(other, 0o700)
	_ = os.WriteFile(filepath.Join(other, "config.toml"), []byte("base_url = \"https://other.example.com"+apiBase+"\"\n"), 0o600)
	refused := runWith(t, cleanEnv(h1, "AUTOTASK_CONFIG="+filepath.Join(other, "config.toml")), "version")
	if !strings.Contains(refused, "not applied") && !strings.Contains(refused, "credkit: withheld") {
		t.Fatalf("the refused-store case did not run (no refusal printed):\n%s", refused)
	}
	out.WriteString(refused)
	// Loose config files whose base_url does not parse (a bad port, a raw "/"
	// in the password, braces in the host). An environment credential is what
	// makes the refusal print: each of the three Autotask names in turn.
	for i, u := range []string{
		"https://ro:" + loosePw + "@other.example.com:bad/",
		"https://ro:" + slashPw + "@other.example.com" + apiBase,
		"https://ro:" + loosePw + "@{zone}.other.example.com" + apiBase,
		"https://ro:" + loosePw + "@other.example.com:bad" + apiBase,
		"https://ro:" + loosePw + "@other.example.com:bad" + apiBase + "/",
	} {
		h := t.TempDir()
		p := filepath.Join(h, "loose.toml")
		_ = os.WriteFile(p, []byte("base_url = \""+u+"\"\n"), 0o644)
		loosen(t, p)
		envKey := "AUTOTASK_API_INTEGRATION_CODE=EnvPassw0rd2x"
		switch i {
		case 3:
			envKey = "AUTOTASK_PSA_SECRET=EnvPassw0rd2x"
		case 4:
			envKey = "AUTOTASK_PSA_USER_NAME=env-user-6c2e@psa-api.example" // an identifier: it is not on the never-print list
		}
		got := runWith(t, cleanEnv(h, "AUTOTASK_CONFIG="+p, envKey), "version")
		if i != 1 && !strings.Contains(got, "refused") && !strings.Contains(got, "credkit: withheld") {
			t.Fatalf("the loose-config case %d did not run (no refusal printed):\n%s", i, got)
		}
		out.WriteString(got)
	}
	for _, pw := range []string{storePw, loosePw, slashPw, "Passw0rd", "EnvPassw0rd2x"} {
		if strings.Contains(out.String(), pw) {
			t.Errorf("output repeats the file password %q:\n%s", pw, out.String())
		}
	}
}

// Class credential-source-precedence (autotask config-load order): every
// place the three auth headers can come from (the environment, the
// environment beside AUTOTASK_USERNAME, the credential store, the environment
// over the store, a 0600 config file, the environment over that file, and the
// file's code with the environment's UserName and Secret) sends exactly the
// one (ApiIntegrationCode, UserName, Secret) triple it should, never as an
// Authorization header, from the CLI and the MCP server alike; the values it
// holds but does not send never print, whole or typed back where the program
// repeats them.
func TestEveryCredentialSourceSendsOneKey(t *testing.T) {
	t.Parallel()
	for mode := credStored; mode <= credConfigMixed; mode++ {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			var out strings.Builder
			for _, args := range [][]string{
				e.api("tickets", "query", "--filter", allFilter, "--json"),
				e.api("tickets", "query-item", bbTicketID, "--json"),
				e.api("resources", "query", "--filter", allFilter, "--json"),
				{"auth", "status"},
				{"doctor"},
			} {
				out.WriteString(e.run(t, args...))
			}
			for _, held := range mode.held() {
				out.WriteString(e.run(t, e.api("tickets", "query-item", held, "--json")...))
				out.WriteString(e.run(t, e.api("tickets", "create-entity", "--title", "HQ "+held, "--dry-run")...))
			}
			out.WriteString(e.mcpCalls(t, []map[string]any{
				{"name": "autotask_read", "arguments": map[string]any{"endpoint_id": "tickets.query", "params": map[string]any{
					"filter": []any{map[string]any{"op": "gte", "field": "id", "value": 0}}}}},
				{"name": "autotask_read", "arguments": map[string]any{"endpoint_id": "tickets.query-item", "params": map[string]any{"id": bbTicketID}}},
			}))
			got := out.String()
			// Every response echoes, so the kit withholds each whole.
			if !strings.Contains(got, "HQ-Record") && !strings.Contains(got, "credkit: withheld") {
				t.Fatalf("the battery fetched nothing:\n%.2000s", got)
			}
			assertNoEcho(t, "output", got)
			n, all := sv.sawHeader(mode.sent())
			if n < 5 || len(all) != 1 {
				t.Errorf("want every request to carry the %s triple, got %d of it and distinct triples %v", mode, n, all)
			}
			sv.mu.Lock()
			if sv.auths != 0 {
				t.Errorf("%d request(s) carried an Authorization header; Autotask takes the credentials only in its three headers", sv.auths)
			}
			sv.mu.Unlock()
		})
	}
}

// loosen makes p readable by other local users, the shape the CLI refuses to
// load: group and other read bits on Unix, a BUILTIN\Users read ACE on Windows.
// Windows has no mode bits, and a file in the runner's temp directory inherits
// an ACL only its owner and the administrators can read, so without the ACE the
// refusal never ran there (CI #565: "the loose-config case 0 did not run").
func loosen(t *testing.T, p string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if b, err := exec.Command("icacls", p, "/grant", "*S-1-5-32-545:R").CombinedOutput(); err != nil {
			t.Fatalf("icacls could not grant Users read on %s: %v\n%s", p, err, b)
		}
		return
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
}
