// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The three credential classes the press 4.34.2 print's last Codex round left
// open (.review/followups.md items 9 to 11 of the adoption), each closed
// through the shared credential kit and the connector's own config rewrite
// (handfixes.json credkit-wiring, config-file-holds-no-credential), plus the
// templated base URL the kit must see resolved. Each input is the one the
// review named, on the built binary.

import (
	"encoding/base32"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// leakKey is SALESBUILDR_API_KEY in these tests: a run of letters no ordinary
// output shares an 8-character piece with.
const leakKey = "sbkey-zqwmkzqwmqzkwzqm"

// apiEnv is a closed environment that sends leakKey to the fake at base.
func apiEnv(home, base, ca string, extra ...string) []string {
	return closedEnv(home, append([]string{"SALESBUILDR_BASE_URL=" + base, "SALESBUILDR_CA_CERT=" + ca, "SALESBUILDR_API_KEY=" + leakKey}, extra...)...)
}

// wholeToken reports whether text carries v with no letter or digit on
// either side (contract 2(a): how a value under 8 characters leaks).
func wholeToken(text, v string) bool {
	return regexp.MustCompile(`(^|[^A-Za-z0-9])` + regexp.QuoteMeta(v) + `([^A-Za-z0-9]|$)`).MatchString(text)
}

// reached fails unless the response reached the output, whole or as the
// kit's notice, so a run that printed nothing proves nothing.
func reached(t *testing.T, args []string, out, errOut string) {
	t.Helper()
	if !strings.Contains(out+errOut, "customFields") && !strings.Contains(out+errOut, "withheld") {
		t.Fatalf("salesbuildr-cli %s: the response reached no output, so this run proves nothing\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), out, errOut)
	}
}

// readCommands are the reads each response class is driven through: JSON and
// the human table.
var readCommands = [][]string{{"company", "get", "cmp-1", "--json"}, {"company", "get", "cmp-1"}}

// TestShortHeldValueEncoded is open class short-held-value-encoded: a
// three-character password a response declares (customFields.password) must
// not print as a double-spaced character-code list in another member, nor
// plainly, on stdout, stderr or any file the run writes.
func TestShortHeldValueEncoded(t *testing.T) {
	cliBin, _ := binaries(t)
	for _, c := range []struct {
		name, body string
		list       string // a spelling that must not appear anywhere
	}{
		{"double-spaced character codes", `{"name":"[97,  90,  57]","customFields":{"password":"aZ9"}}`, "97,  90,  57"},
		{"plain echo", `{"name":"aZ9","customFields":{"password":"aZ9"}}`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			base, ca, _ := fakeAPI(t, c.body, "")
			home := t.TempDir()
			env := apiEnv(home, base, ca)
			for _, args := range readCommands {
				out, errOut, _ := runSplit(t, cliBin, env, args...)
				reached(t, args, out, errOut)
				for where, text := range map[string]string{"stdout": out, "stderr": errOut} {
					if c.list != "" && strings.Contains(text, c.list) {
						t.Errorf("%s %s carries the password's character codes %q:\n%s", strings.Join(args, " "), where, c.list, text)
					}
					if wholeToken(text, "aZ9") {
						t.Errorf("%s %s carries the password aZ9:\n%s", strings.Join(args, " "), where, text)
					}
				}
			}
			for path, text := range filesUnder(t, home) {
				if (c.list != "" && strings.Contains(text, c.list)) || wholeToken(text, "aZ9") {
					t.Errorf("%s carries the password aZ9 or its character codes", path)
				}
			}
		})
	}
}

// TestNonStringCredentialNotHeld is open class non-string-credential-not-held:
// a credential that is a number, a JSON number a response declares
// (customFields.otp) or a TOML integer under a config credential key, must
// not print in any spelling of that number (scientific notation included).
func TestNonStringCredentialNotHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	t.Run("response number", func(t *testing.T) {
		base, ca, _ := fakeAPI(t, `{"name":"7.36251e5","customFields":{"otp":736251}}`, "")
		home := t.TempDir()
		env := apiEnv(home, base, ca)
		for _, args := range readCommands {
			out, errOut, _ := runSplit(t, cliBin, env, args...)
			reached(t, args, out, errOut)
			for _, form := range []string{"7.36251e5", "736251"} {
				if strings.Contains(out+errOut, form) {
					t.Errorf("%s prints %q, a spelling of the held otp 736251:\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), form, out, errOut)
				}
			}
		}
		for path, text := range filesUnder(t, home) {
			if strings.Contains(text, "7.36251e5") || strings.Contains(text, "736251") {
				t.Errorf("%s carries a spelling of the held otp 736251", path)
			}
		}
	})
	t.Run("TOML integer in the config", func(t *testing.T) {
		// The discovered default config carries the integer; the run reads an
		// explicit one, so the default's values are only held (contract 1: a
		// config file's credential fields, a file the run does not use included).
		base, ca, _ := fakeAPI(t, `{"name":"4.8261937e7","note":"48,261,937","customFields":{}}`, "")
		home := t.TempDir()
		writeConfig(t, home, "client_secret = 48261937\n", false)
		explicit := writeConfig(t, home, "# the config this run reads\n", true)
		env := apiEnv(home, base, ca)
		for _, args := range readCommands {
			args = append([]string{"--config", explicit}, args...)
			out, errOut, _ := runSplit(t, cliBin, env, args...)
			if !strings.Contains(out+errOut, "note") && !strings.Contains(out+errOut, "withheld") {
				t.Fatalf("the response reached no output, so this run proves nothing\nstdout:\n%s\nstderr:\n%s", out, errOut)
			}
			for _, form := range []string{"4.8261937e7", "48,261,937", "48261937"} {
				if strings.Contains(out+errOut, form) {
					t.Errorf("%s prints %q, a spelling of the config's integer client_secret:\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), form, out, errOut)
				}
			}
		}
	})
}

// TestConfigHeaderNeverKeepsTheNewKey is open class
// credential-persisted-outside-the-store: config.toml carries a [headers]
// value that is the base32 of the key `auth set-token` then saves. The config
// rewrite (outside the kit, a declared exemption) must not keep it: the key
// never lands in config.toml in any spelling, and the run says it left the
// header out (or fails). A header unrelated to the key is kept, so the
// rewrite does not simply drop every header.
func TestConfigHeaderNeverKeepsTheNewKey(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "ABCDEFGH" // the reviewed input
	enc := base32.StdEncoding.EncodeToString([]byte(key))
	home := t.TempDir()
	path := writeConfig(t, home, "[headers]\nX-Trace = \""+enc+"\"\nX-Team = \"team-blue-west\"\n", false)
	_, errOut, code := runSplitIn(t, cliBin, closedEnv(home), key+"\n", "auth", "set-token")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, form := range []string{key, strings.TrimRight(enc, "=")} {
		if strings.Contains(string(data), form) {
			t.Errorf("config.toml keeps %q, a spelling of the key auth set-token saved:\n%s", form, data)
		}
	}
	if code == 0 && !strings.Contains(errOut, "X-Trace") {
		t.Errorf("auth set-token exited 0 without saying it left X-Trace out of config.toml:\nstderr:\n%s", errOut)
	}
	if code == 0 && !strings.Contains(string(data), "team-blue-west") {
		t.Errorf("config.toml lost a header unrelated to the key:\n%s", data)
	}
}

// TestTenantHostIsTheAllowedOrigin: the built-in base URL is the template
// https://{tenant}.salesbuildr.com/public-api, which the kit cannot read as
// an origin. With SALESBUILDR_TENANT=acme the key must reach
// acme.salesbuildr.com (here: the CONNECT a proxy receives, so no request
// leaves the machine); with SALESBUILDR_BASE_URL it must reach that fake; and
// a request carrying it anywhere else must be refused before it is sent.
func TestTenantHostIsTheAllowedOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	t.Run("tenant host through a proxy", func(t *testing.T) {
		var mu sync.Mutex
		var connects []string
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			connects = append(connects, r.Method+" "+r.Host)
			mu.Unlock()
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(proxy.Close)
		home := t.TempDir()
		env := closedEnv(home, "SALESBUILDR_TENANT=acme", "SALESBUILDR_API_KEY="+leakKey, "HTTPS_PROXY="+proxy.URL, "https_proxy="+proxy.URL)
		out, errOut, _ := runSplit(t, cliBin, env, "company", "get", "cmp-1", "--json")
		mu.Lock()
		defer mu.Unlock()
		if !containsString(connects, "CONNECT acme.salesbuildr.com:443") {
			t.Errorf("the request for tenant acme never reached acme.salesbuildr.com (proxy saw %v):\nstdout:\n%s\nstderr:\n%s", connects, out, errOut)
		}
	})
	t.Run("tenant from config.toml through a proxy", func(t *testing.T) {
		var mu sync.Mutex
		var connects []string
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			connects = append(connects, r.Method+" "+r.Host)
			mu.Unlock()
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(proxy.Close)
		home := t.TempDir()
		writeConfig(t, home, "[template_vars]\ntenant = \"acme\"\n", false)
		env := closedEnv(home, "SALESBUILDR_API_KEY="+leakKey, "HTTPS_PROXY="+proxy.URL, "https_proxy="+proxy.URL)
		out, errOut, _ := runSplit(t, cliBin, env, "company", "get", "cmp-1", "--json")
		mu.Lock()
		defer mu.Unlock()
		if !containsString(connects, "CONNECT acme.salesbuildr.com:443") {
			t.Errorf("the request for the config's tenant acme never reached acme.salesbuildr.com (proxy saw %v):\nstdout:\n%s\nstderr:\n%s", connects, out, errOut)
		}
	})
	t.Run("base URL fake with a tenant set", func(t *testing.T) {
		base, ca, seen := fakeAPI(t, `{"id":"cmp-1","name":"HQ-Record","customFields":{}}`, leakKey)
		home := t.TempDir()
		out, errOut, _ := runSplit(t, cliBin, apiEnv(home, base, ca, "SALESBUILDR_TENANT=acme"), "company", "get", "cmp-1", "--json")
		if seen() == 0 {
			t.Errorf("the key never reached the configured fake:\nstdout:\n%s\nstderr:\n%s", out, errOut)
		}
	})
	t.Run("key refused to another origin", func(t *testing.T) {
		// The API echoes the key; --deliver posts the answer to another origin,
		// which must receive nothing.
		base, ca, _ := fakeAPI(t, `{"id":"cmp-1","name":"`+leakKey+`","customFields":{}}`, "")
		var mu sync.Mutex
		hits := 0
		other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits++
			mu.Unlock()
		}))
		t.Cleanup(other.Close)
		home := t.TempDir()
		out, errOut, code := runSplit(t, cliBin, apiEnv(home, base, ca, "SALESBUILDR_TENANT=acme"), "company", "get", "cmp-1", "--json", "--deliver", "webhook:"+other.URL+"/hook")
		mu.Lock()
		defer mu.Unlock()
		if hits != 0 {
			t.Errorf("a request carrying the key reached another origin %d times:\nstdout:\n%s\nstderr:\n%s", hits, out, errOut)
		}
		if code == 0 {
			t.Errorf("the refused delivery exited 0:\nstdout:\n%s\nstderr:\n%s", out, errOut)
		}
		if strings.Contains(out+errOut, leakKey) {
			t.Errorf("the key printed:\nstdout:\n%s\nstderr:\n%s", out, errOut)
		}
	})
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
