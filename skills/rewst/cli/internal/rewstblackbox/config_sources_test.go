// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

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

// cleanEnv is a clean environment rooted at home (HOME second, which
// runWithIn reads), every non-loopback request sent to the proxy trap.
func cleanEnv(home string, extra ...string) []string {
	return append(append([]string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "REWST_HOME=" + filepath.Join(home, ".rewst-home"), "USERPROFILE=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"APPDATA=" + filepath.Join(home, "AppData", "Roaming"), "LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
	}, trapEnv()...), extra...)
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
	budget(t)
	const storePw, loosePw, slashPw = "StorePassw0rd1x", "OnlyUrlPw-Q7r9x", "Sl4sh/Passw0rd"
	var out strings.Builder
	// A global credential store recorded for one Rewst host (its base_url
	// carries the password) and refused for a config that names another.
	h1 := t.TempDir()
	runWithIn(t, cleanEnv(h1, "REWST_BASE_URL=https://ro:"+storePw+"@rewst-store.example.com"), storePw+"\n", "auth", "set-token")
	other := filepath.Join(h1, "other")
	_ = os.MkdirAll(other, 0o700)
	_ = os.WriteFile(filepath.Join(other, "config.toml"), []byte("base_url = \"https://other.example.com\"\n"), 0o600)
	refused := runWith(t, cleanEnv(h1, "REWST_CONFIG="+filepath.Join(other, "config.toml")), "version")
	// The refusal repeats the store's base URL, whose password the kit
	// holds, so it is withheld whole.
	if !strings.Contains(refused, "not applied") && !kitWithheld(refused) {
		t.Fatalf("the refused-store case did not run (no refusal printed):\n%s", refused)
	}
	out.WriteString(refused)
	// Loose config files whose base_url does not parse (a bad port, a raw "/"
	// in the password, braces in the host). An environment credential
	// (REWST_API_TOKEN) is what makes the refusal print; on the last, a
	// stored token is present too.
	for i, u := range []string{
		"https://ro:" + loosePw + "@other.example.com:bad/",
		"https://ro:" + slashPw + "@other.example.com/v1",
		"https://ro:" + loosePw + "@{tenant}.other.example.com/v1",
		"https://ro:" + loosePw + "@other.example.com:bad/v1",
	} {
		h := t.TempDir()
		p := filepath.Join(h, "loose.toml")
		_ = os.WriteFile(p, []byte("base_url = \""+u+"\"\n"), 0o644)
		loosen(t, p)
		if i == 3 {
			runWithIn(t, cleanEnv(h), "StoredPassw0rd3x\n", "auth", "set-token")
		}
		got := runWith(t, cleanEnv(h, "REWST_CONFIG="+p, "REWST_API_TOKEN="+"EnvPassw0rd2x"), "version")
		if i != 1 && !strings.Contains(got, "refused") && !kitWithheld(got) {
			t.Fatalf("the loose-config case did not run (no refusal printed):\n%s", got)
		}
		out.WriteString(got)
	}
	for _, pw := range []string{storePw, loosePw, slashPw, "Passw0rd", "EnvPassw0rd2x", "StoredPassw0rd3x"} {
		if strings.Contains(out.String(), pw) {
			t.Errorf("output repeats the file password %q:\n%s", pw, out.String())
		}
	}
}

// Class credential-source-precedence: every place the token can come from
// (REWST_API_TOKEN, the credential store, the environment over a stored
// token, a config file's api_token, access_token alone or auth_header, or
// every config key) sends exactly the one token it should, in the one header
// Authorization: Bearer <token> (the auth_header value as given), never
// another way (an X-Api-Key header, a second or doubly prefixed
// Authorization header, the query), on REST-shaped reads and writes and on
// GraphQL POSTs (the novels, doctor's probe, a mutation through --stdin),
// from the CLI and the MCP server alike; the credentials a source holds but
// does not send never print, whole or typed back where the program repeats
// them. (REWST_API_TOKEN given already prefixed is the P2
// TestPrefixedEnvTokenDoublePrefixedKnownP2.)
func TestEveryCredentialSourceSendsOneKey(t *testing.T) {
	budget(t)
	for mode := credStored; mode <= credConfigHeader; mode++ {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode).lean()
			var out strings.Builder
			// The default run: every source sends its one key on a novel's GraphQL
			// POST and an MCP REST-shaped read; the config file with every key also
			// runs a REST read, doctor, --stdin, previews of what it holds and a
			// mirrored MCP novel; it and the credential store run logout.
			rich := fullSweep || mode == credConfigBoth
			wantKeys, wantGQL := 8, 3
			if !rich {
				wantKeys, wantGQL = 2, 1
			}
			reads := [][]string{
				{"organizations", "get", "--json"},
				{"api-clients", bbAPIClientID, "--json"},
				{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID, "--json"},
				{"org-variables", "get", "--visible-for-org-id", bbOrgID, "--json"},
				{"users", "get", "--json"},
				{"health", "--org", bbOrgID, "--json"},
				{"pack-delete-responses", "--pack-id", bbPackID, "--yes", "--json"},
				{"auth", "status"},
				{"doctor"},
			}
			mcp := []map[string]any{
				read("organizations.get", map[string]any{}),
				read("pack-configs.get", map[string]any{"orgId": bbOrgID}),
				tool("failures", map[string]any{"org": bbOrgID, "since": "7d"}),
				read("users.get", map[string]any{}),
			}
			if !fullSweep {
				reads = [][]string{{"health", "--org", bbOrgID, "--json"}}
				if rich {
					reads = append(reads, []string{"api-clients", bbDenied, "--json"}, []string{"failures", "--org", bbOrgID, "--json"}, []string{"doctor"})
				}
				mcp = []map[string]any{read("users.get", map[string]any{})}
				if rich {
					mcp = append(mcp, tool("failures", map[string]any{"org": bbOrgID, "since": "7d"}))
				}
			}
			for _, args := range reads {
				out.WriteString(e.run(t, args...))
			}
			if rich {
				out.WriteString(e.runIn(t, gqlBody(mutCreateClient), "organizations", "create", "--stdin", "--yes", "--json"))
				held := mode.held()
				if !fullSweep {
					held = held[:min(2, len(held))]
				}
				for _, held := range held {
					if fullSweep {
						out.WriteString(e.run(t, "api-clients", held, "--json"))
					}
					out.WriteString(e.run(t, "org-variables", "create", "--name", "HQ "+held, "--value", "x", "--category", "general", "--org-id", bbOrgID,
						"--cascade", "false", "--dry-run"))
				}
			}
			out.WriteString(e.mcpCalls(t, mcp))
			if rich || mode == credStored {
				// auth logout clears what the source stored; neither it nor the
				// status after it prints a credential.
				out.WriteString(e.run(t, "auth", "logout"))
				out.WriteString(e.run(t, "auth", "status"))
			}
			got := out.String()
			// The fake echoes the token, so the kit withholds most answers
			// whole; the request counts below prove the battery ran.
			if !strings.Contains(got, "HQ-Record") && !kitWithheld(got) {
				t.Fatalf("the battery fetched nothing:\n%.2000s", got)
			}
			assertNoEcho(t, "output", got)
			if n, all := sv.sawKey(mode.sent()); n < wantKeys || len(all) != 1 {
				t.Errorf("want every request to carry Authorization: Bearer %s (the %s token), got %d of it and distinct values %q", mode.sent(), mode, n, all)
			}
			var gql int
			for _, m := range sv.allMethods() {
				if strings.Contains(m, " query:") || strings.Contains(m, " mutation:") {
					gql++
				}
			}
			if gql < wantGQL {
				t.Errorf("only %d GraphQL POST(s) reached the fake; the novels, doctor's probe and the mutation did not all run: %v", gql, sv.allMethods())
			}
			checkOneAuth(t, sv, mode)
		})
	}
}
