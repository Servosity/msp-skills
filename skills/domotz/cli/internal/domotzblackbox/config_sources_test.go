// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package domotzblackbox

// Helpers that run the CLI in a clean environment of its own home.

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

// Class credential-source-precedence: every place the API key can come from
// (DOMOTZ_API_KEY, the credential store, the environment over a stored key,
// a config file's api_key or both config keys) sends exactly the one key it
// should, raw in the one header X-Api-Key: <key>, never with a scheme word,
// in an Authorization header or in the query, from the CLI and the MCP
// server alike; a config file's access_token alone sends no key at all
// (Domotz takes only the API key); the credentials a source holds but does
// not send never print, whole or typed back where the program repeats them.
func TestEveryCredentialSourceSendsOneKey(t *testing.T) {
	t.Parallel()
	for mode := credStored; mode <= credConfigBoth; mode++ {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			var out strings.Builder
			for _, args := range [][]string{
				{"agent", "list", "--json"},
				{"agent", "device", "get", bbAgentID, bbDeviceID, "--json"},
				{"custom-driver", "list", "--json"},
				{"auth", "status"},
				{"doctor"},
			} {
				out.WriteString(e.run(t, args...))
			}
			for _, held := range mode.held() {
				out.WriteString(e.run(t, "agent", "get", held, "--json"))
				out.WriteString(e.run(t, "custom-tag", "create", "--name", "HQ "+held, "--dry-run"))
			}
			out.WriteString(e.mcpCalls(t, []map[string]any{
				read("agent.list", map[string]any{}),
				read("agent.device.get", onDevice(nil)),
			}))
			// auth logout clears what the source stored; neither it nor the
			// status after it prints a credential.
			out.WriteString(e.run(t, "auth", "logout"))
			out.WriteString(e.run(t, "auth", "status"))
			got := out.String()
			if !strings.Contains(got, "HQ-Record") {
				t.Fatalf("the battery fetched nothing:\n%.2000s", got)
			}
			assertNoEcho(t, "output", got)
			n, all := sv.sawKey(mode.sent())
			if mode.sent() == "" {
				if len(all) != 0 {
					t.Errorf("the %s mode holds no API key, yet requests carried X-Api-Key values %q", mode, all)
				}
			} else if n < 5 || len(all) != 1 {
				t.Errorf("want every request to carry X-Api-Key: %s (the %s key), got %d of it and distinct values %q", mode.sent(), mode, n, all)
			}
			sv.mu.Lock()
			if sv.otherAuth != 0 {
				t.Errorf("%d request(s) carried a credential another way (an Authorization header, a second or scheme-prefixed X-Api-Key, a query key); Domotz takes only X-Api-Key: <key>", sv.otherAuth)
			}
			sv.mu.Unlock()
		})
	}
}
