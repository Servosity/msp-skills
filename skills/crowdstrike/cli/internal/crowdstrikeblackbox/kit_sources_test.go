// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-profiles-held,
// credkit-url-credentials-held): the sources the program reads after the kit
// started, held by the startup discovery in internal/credwire.

package crowdstrikeblackbox

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// writtenUnder is every file under home but skip, concatenated.
func writtenUnder(t *testing.T, home, skip string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() != skip {
			if data, rerr := os.ReadFile(p); rerr == nil {
				b.Write(data)
			}
		}
		return nil
	})
	return b.String()
}

// credkit-profiles-held: a run-profile store is read after the kit started, so
// startup holds what it carries as a source: a value under a flag a profile does
// not apply (an unknown flag, every duplicate of it, a credential flag's name,
// --client-id included) and a base-url value (this CLI has no such flag). A
// saved --select stays usable, and so does a URL in an allowed flag such as
// --deliver (free text is no source, contract 1). A store that does not parse
// is held by the kit's HoldBody.
func TestKitRunProfilesHeldAtStartup(t *testing.T) {
	t.Parallel()
	const (
		unknown = "unknownflagvalue-mmmm"
		first   = "firstduplicate-rrrr"
		secret  = "savedsecretflag-wwww"
		id      = "savedclientid-kkkk"
		urlPw   = "baseurlpw-hhhh"
		broken  = "brokenstorevalue-tttt"
	)
	home := t.TempDir()
	cfg := filepath.Join(home, "config")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(cfg, "profiles.json")
	// nosuchflag twice: json.Unmarshal keeps only the second.
	body := fmt.Sprintf(`{"profiles":{"p":{"name":"p","values":{"select":"hostname,status","deliver":"webhook:https://h.example/hook","nosuchflag":%q,"nosuchflag":%q,"client-secret":%q,"client-id":%q,"base-url":"https://u:%s@api.example.com"}}}}`, first, unknown, secret, id, urlPw)
	if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env := cleanEnv(home)
	var out strings.Builder
	for _, v := range []string{unknown, first, secret, id, urlPw} {
		out.WriteString(runWith(t, env, "feedback", "repeats "+v, "--json"))
	}
	out.WriteString(runWith(t, env, "feedback", "the saved select hostname,status and webhook:https://h.example/hook", "--json"))
	written := writtenUnder(t, home, "profiles.json")
	if !strings.Contains(written, "hostname,status") || !strings.Contains(written, "h.example/hook") {
		t.Errorf("an allowed profile value was held:\n%s\n%s", out.String(), written)
	}
	for _, v := range []string{unknown, first, secret, id, urlPw} {
		if strings.Contains(out.String(), v) || strings.Contains(written, v) {
			t.Errorf("%s was not held:\n%s\n%s", v, out.String(), written)
		}
	}
	// A store that does not parse has no names to tell apart.
	if err := os.WriteFile(store, []byte(`{"profiles":{"p":{"values":{"select":"`+broken+`"`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := runWith(t, env, "feedback", "repeats "+broken, "--json")
	if strings.Contains(got, broken) || strings.Contains(writtenUnder(t, home, "profiles.json"), broken) {
		t.Errorf("a value in a broken store was not held:\n%s", got)
	}
}

// shownAsToken reports whether text carries secret as a whole token (contract
// 2(a), the rule for a value shorter than 8 characters).
func shownAsToken(text, secret string) bool {
	return regexp.MustCompile(`(^|[^A-Za-z0-9])` + regexp.QuoteMeta(secret) + `([^A-Za-z0-9]|$)`).MatchString(text)
}

// credkit-url-credentials-held: the password inside a URL setting is held from
// startup by the kit's own URL reader (credkit.HoldURL), in every form a typo
// leaves it in, at any length, numbers included. The program prints the setting
// back (a connection error, doctor's base_url) and must never print the
// password with it, from the environment or from config.toml.
func TestKitURLSettingPasswordNeverPrinted(t *testing.T) {
	t.Parallel()
	for _, in := range []struct{ url, pw string }{
		{"https//u:87654321@127.0.0.1:1", "87654321"},
		{"u:87654321@127.0.0.1:1", "87654321"},
		{"https;//u:87654321@127.0.0.1:1", "87654321"},
		{"https//u:xQz@127.0.0.1:1", "xQz"},
	} {
		t.Run(in.url, func(t *testing.T) {
			t.Parallel()
			sinks := map[string]string{}
			commands := [][]string{{"devices", "query-by-filter", "--json"}, {"doctor"}, {"doctor", "--json"}}
			pair := []string{"FALCON_CLIENT_ID=" + bbClientID, "FALCON_CLIENT_SECRET=" + bbClientSecret}
			home := t.TempDir()
			env := cleanEnv(home, append([]string{"CROWDSTRIKE_BASE_URL=" + in.url}, pair...)...)
			for _, args := range commands {
				sinks["env: "+strings.Join(args, " ")] = runWith(t, env, args...)
			}
			home = t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, "config"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "config", "config.toml"), []byte("base_url = \""+in.url+"\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			env = cleanEnv(home, pair...)
			for _, args := range commands {
				sinks["config.toml: "+strings.Join(args, " ")] = runWith(t, env, args...)
			}
			for where, text := range sinks {
				if shownAsToken(text, in.pw) {
					t.Errorf("%s printed the URL password %q:\n%.1500s", where, in.pw, text)
				}
			}
		})
	}
}
