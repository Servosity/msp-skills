// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// shownAsToken reports whether text carries secret as a whole token: each side
// is the start or end of text or a character that is not a letter or digit
// (contract 2(a), the rule for a value shorter than 8 characters).
func shownAsToken(text, secret string) bool {
	return regexp.MustCompile(`(^|[^A-Za-z0-9])` + regexp.QuoteMeta(secret) + `([^A-Za-z0-9]|$)`).MatchString(text)
}

// Class credential-in-url-setting: the password inside a URL setting is held
// from startup by the kit's own URL reader (credkit.HoldURL), in every form a
// typo leaves it in, at any length, numbers included. The program prints the
// setting back (a retry line, a connection error, doctor's base_url, the
// refusal of a saved credential) and must never print the password with it,
// from the environment, from config.toml or from credentials.toml, on the CLI
// or through the MCP server.
func TestURLSettingPasswordNeverPrinted(t *testing.T) {
	t.Parallel()
	for _, in := range []struct{ url, pw string }{
		{"https//u:87654321@127.0.0.1:1", "87654321"},
		{"u:87654321@127.0.0.1:1", "87654321"},
		{"https ://u:87654321@127.0.0.1:1", "87654321"},
		{"https;//u:87654321@127.0.0.1:1", "87654321"},
		{"https//:87654321@127.0.0.1:1", "87654321"},
		{"https//u:xQz@127.0.0.1:1", "xQz"},
		{"u:xQz@127.0.0.1:1", "xQz"},
		{"https//u:x9@127.0.0.1:1", "x9"},
	} {
		t.Run(in.url, func(t *testing.T) {
			t.Parallel()
			sinks := map[string]string{}
			commands := [][]string{{"vm", "list", "--json"}, {"doctor"}, {"doctor", "--json"}}
			login := []string{"VSPHERE_USERNAME=" + bbUser, "VSPHERE_PASSWORD=" + bbPassword}

			// The environment.
			home := t.TempDir()
			env := cleanEnv(home, append([]string{"VSPHERE_HOME=" + home, "VSPHERE_BASE_URL=" + in.url}, login...)...)
			for _, args := range commands {
				sinks["env: "+strings.Join(args, " ")] = runWith(t, env, args...)
			}
			sinks["env: MCP vm_list"] = (&bbEnv{home: home, env: env}).mcpCalls(t, []map[string]any{{"name": "vm_list", "arguments": map[string]any{}}})

			// config.toml.
			home = t.TempDir()
			writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+in.url+"\"\n")
			env = cleanEnv(home, append([]string{"VSPHERE_HOME=" + home}, login...)...)
			for _, args := range commands {
				sinks["config.toml: "+strings.Join(args, " ")] = runWith(t, env, args...)
			}
			sinks["config.toml: MCP vm_list"] = (&bbEnv{home: home, env: env}).mcpCalls(t, []map[string]any{{"name": "vm_list", "arguments": map[string]any{}}})

			// credentials.toml, saved for that address and refused for another:
			// the refusal names the address it was saved for.
			home = t.TempDir()
			writeFile(t, filepath.Join(home, "data", "credentials.toml"), "base_url = \""+in.url+"\"\nusername = \"ro@a.local\"\npassword = \"StoredForTypo-5c2e9a\"\n")
			env = cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_BASE_URL=https://vc.example.invalid")
			for _, args := range commands {
				sinks["credentials.toml: "+strings.Join(args, " ")] = runWith(t, env, args...)
			}
			// The refusal is withheld whole, or the kit refuses to start on a
			// password it finds in public text: either way it ran.
			if got := sinks["credentials.toml: vm list --json"]; !strings.Contains(got, "not applied") && !strings.Contains(got, withheldNotice) && !strings.Contains(got, "refusing to start") {
				t.Fatalf("the saved credential was not refused; the credentials.toml case did not run:\n%.1500s", got)
			}

			for where, text := range sinks {
				if shownAsToken(text, in.pw) {
					t.Errorf("%s printed the URL password %q:\n%.1500s", where, in.pw, text)
				}
			}
		})
	}
	t.Run("feedback endpoint", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		env := cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_FEEDBACK_ENDPOINT=https://u:!!!@feedback.example:bad/")
		if out := runWith(t, env, "feedback", "note", "--send", "--json"); strings.Contains(out, "!!!") {
			t.Errorf("feedback note --send printed the endpoint's password:\n%.1500s", out)
		}
	})
	t.Run("an address with no credential still prints", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		env := cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_BASE_URL=https//vcenter.example.invalid:1")
		if out := runWith(t, env, "doctor"); strings.Contains(out, withheldNotice) || !strings.Contains(out, "vcenter.example.invalid") {
			t.Errorf("doctor withheld an address with no credential:\n%.1500s", out)
		}
	})
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
