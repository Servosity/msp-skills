// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"bufio"
	"crypto/tls"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"immybot-pp-cli/internal/cliutil"
)

func e2eSkipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("drives the built binaries over POSIX file modes; the kit's own tests run on every OS")
	}
}

// Every credential a process can start with is held before any argument is
// parsed: a flag that fails to parse echoes its text, whichever binary reads it.
// The flag here is the credential, the worst echo there is.
func TestEveryStartupSourceIsHeldBeforeArgumentsAreParsed(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, mcp := buildE2EBinaries(t)
	secret := e2eSecret("startup-source")
	inURL := func(host string) string {
		return (&url.URL{Scheme: "https", User: url.UserPassword("ops", secret), Host: host, Path: "/x"}).String()
	}
	envURL := func(name string) func(t *testing.T) []string {
		return func(t *testing.T) []string {
			t.Setenv(name, inURL("login.microsoftonline.com"))
			return nil
		}
	}
	configFile := func(body string) func(t *testing.T) []string {
		return func(t *testing.T) []string {
			e2eWriteConfig(t, body)
			return nil
		}
	}
	otherFile := func(flag string) func(t *testing.T) []string {
		return func(t *testing.T) []string {
			dir := t.TempDir()
			path := filepath.Join(dir, "other.toml")
			if flag == "--home" {
				t.Setenv("HOME", t.TempDir())
				home := filepath.Join(dir, "home")
				sub := filepath.Join(home, "config")
				if err := os.MkdirAll(sub, 0o700); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(sub, "config.toml")
				if err := os.WriteFile(path, []byte("client_secret = \""+secret+"\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return []string{"--home", home}
			}
			if err := os.WriteFile(path, []byte("client_secret = \""+secret+"\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return []string{"--config", path}
		}
	}
	for _, tc := range []struct {
		name  string
		setUp func(t *testing.T) []string
	}{
		{"IMMYBOT_TOKEN_URL password", envURL("IMMYBOT_TOKEN_URL")},
		{"IMMYBOT_FEEDBACK_ENDPOINT password", envURL("IMMYBOT_FEEDBACK_ENDPOINT")},
		{"IMMYBOT_BASE_URL password", envURL("IMMYBOT_BASE_URL")},
		{"HTTPS_PROXY password", envURL("HTTPS_PROXY")},
		{"HTTP_PROXY password", envURL("HTTP_PROXY")},
		{"ALL_PROXY password", envURL("ALL_PROXY")},
		{"https_proxy password", envURL("https_proxy")},
		{"http_proxy password", envURL("http_proxy")},
		{"all_proxy password", envURL("all_proxy")},
		{"proxy password with no scheme", func(t *testing.T) []string {
			t.Setenv("HTTPS_PROXY", "ops:"+secret+"@proxy.example:3128")
			return nil
		}},
		{"config client_secret", configFile("client_secret = \"" + secret + "\"\n")},
		{"config [headers]", configFile("[headers]\nX-Api-Key = \"" + secret + "\"\n")},
		{"malformed config", configFile("client_secret = \"" + secret + "\"\nnot toml [[[\n")},
		{"config token_url password", configFile("token_url = \"" + inURL("login.microsoftonline.com") + "\"\n")},
		{"--config file", otherFile("--config")},
		{"--home config", otherFile("--home")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStubImmy(t)
			e2eHome(t, s)
			extra := tc.setUp(t)
			if out := e2eRun(t, cli, "", append(extra, "--"+secret)...); e2eCarries(out, secret) {
				t.Fatalf("the CLI echoed the credential:\n%s", out)
			}
			if extra != nil {
				return // --config and --home are the CLI's own flags
			}
			if out := e2eRun(t, mcp, "", "--"+secret); e2eCarries(out, secret) {
				t.Fatalf("the MCP server echoed the credential:\n%s", out)
			}
		})
	}
}

// A request's declared credential fields are held as soon as the body exists,
// whichever way it arrived, so a preview that never reaches the outbound guard
// holds them too. spec.json components.schemas.CreateSmtpRequest.properties.password.
func TestACredentialFieldInARequestBodyIsHeldBeforeThePreview(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	secret := e2eSecret("smtp-stdin-password")
	for _, tc := range []struct {
		name, stdin string
		args        []string
	}{
		{"--stdin body", `{"host":"smtp.example","password":"` + secret + `"}`, []string{"smtp-configs", "create-by-tenant-id", "1", "--stdin", "--dry-run"}},
		{"--stdin nested field, other case", `{"host":"smtp.example","x":[{"PassWord":"` + secret + `"}]}`, []string{"smtp-configs", "create-by-tenant-id", "1", "--stdin", "--dry-run"}},
		{"import record", `{"host":"smtp.example","password":"` + secret + `"}`, []string{"import", "smtp-configs", "--input", "-", "--dry-run"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStubImmy(t)
			e2eHome(t, s)
			s.signIn(t)
			if out := e2eRun(t, cli, tc.stdin, tc.args...); e2eCarries(out, secret) {
				t.Fatalf("the preview printed the field:\n%s", out)
			}
		})
	}
}

// A credential a hand-written profile carries under a declared credential flag
// is held when the profile store is read.
func TestACredentialInAProfileIsHeldWhenTheStoreIsRead(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	s := newStubImmy(t)
	e2eHome(t, s)
	secret := e2eSecret("profile-client-secret")
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := `{"profiles":{"ops":{"name":"ops","values":{"client-secret":"` + secret + `"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(store), 0o600); err != nil {
		t.Fatal(err)
	}
	out := e2eRun(t, cli, "", "profile", "show", "ops", "--json")
	if !strings.Contains(out, "withheld") && !strings.Contains(out, "ops") {
		t.Fatalf("profile show proved nothing:\n%s", out)
	}
	if e2eCarries(out, secret) {
		t.Fatalf("profile show printed the credential:\n%s", out)
	}
}

// Holding a proxy's password keeps it out of every output and still lets it reach
// its own proxy.
func TestAProxyPasswordStillReachesItsProxy(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	s := newStubImmy(t)
	e2eHome(t, s)
	s.signIn(t)
	host, got := e2eStubProxy(t, s)
	secret := e2eSecret("proxy-password")
	t.Setenv("HTTPS_PROXY", (&url.URL{Scheme: "https", User: url.UserPassword("ops", secret), Host: host}).String())
	t.Setenv("IMMYBOT_BASE_URL", "https://alpha.immy.bot")
	out := e2eRun(t, cli, "", "access", "list", "--json", "--data-source", "live")
	if e2eCarries(out, secret) {
		t.Fatalf("the CLI printed the proxy password:\n%s", out)
	}
	select {
	case auth := <-got:
		if auth == "" {
			t.Fatalf("the proxy got no Proxy-Authorization:\n%s", out)
		}
	default:
		t.Fatalf("no CONNECT reached the proxy:\n%s", out)
	}
}

// e2eStubProxy is a TLS proxy that refuses every CONNECT and reports its
// Proxy-Authorization, so a request that gets past the kit is seen without
// resolving the host it names.
func e2eStubProxy(t *testing.T, s *stubImmy) (string, <-chan string) {
	t.Helper()
	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: s.srv.TLS.Certificates})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	got := make(chan string, 16)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			if req, err := http.ReadRequest(bufio.NewReader(c)); err == nil {
				got <- req.Header.Get("Proxy-Authorization")
			}
			_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
			_ = c.Close()
		}
	}()
	return l.Addr().String(), got
}

// A number under a credential field is held as it is written: the preview prints
// that spelling, not the one a float64 would give.
func TestANumericCredentialFieldIsHeldAsWritten(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	stdin := []string{"smtp-configs", "create-by-tenant-id", "1", "--stdin", "--dry-run"}
	imp := []string{"import", "smtp-configs", "--input", "-", "--dry-run"}
	for _, tc := range []struct {
		name, number string
		args         []string
	}{
		{"--stdin integer", "48151623", stdin},
		{"import integer", "48151623", imp},
		{"import exponent", "4.8151623e7", imp},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStubImmy(t)
			e2eHome(t, s)
			s.signIn(t)
			body := `{"host":"smtp.example","password":` + tc.number + `}`
			if out := e2eRun(t, cli, body, tc.args...); strings.Contains(out, tc.number) {
				t.Fatalf("the preview printed the number:\n%s", out)
			}
		})
	}
}

// A --stdin body is held whole from startup, so a credential under a field name
// no declaration knows, or in a body that does not parse, stays out of the preview.
func TestAStdinBodyIsHeldWhole(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	secret := e2eSecret("stdin-undeclared-field")
	for name, body := range map[string]string{
		"undeclared field": `{"host":"smtp.example","smtpAccessValue":"` + secret + `"}`,
		"body cut short":   `{"host":"smtp.example","password":"` + secret + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			s := newStubImmy(t)
			e2eHome(t, s)
			s.signIn(t)
			if out := e2eRun(t, cli, body, "smtp-configs", "create-by-tenant-id", "1", "--stdin", "--dry-run"); e2eCarries(out, secret) {
				t.Fatalf("the preview printed the body:\n%s", out)
			}
		})
	}
}

// e2eWriteProfiles writes the profile store under the current config directory.
func e2eWriteProfiles(t *testing.T, store string) {
	t.Helper()
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(store), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Every member of a duplicate key is held: a JSON decode keeps the last, and the
// first would go on printing wherever the store repeats it.
func TestEveryMemberOfADuplicateProfileKeyIsHeld(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	s := newStubImmy(t)
	e2eHome(t, s)
	first, last := e2eSecret("duplicate-first"), e2eSecret("duplicate-last")
	e2eWriteProfiles(t, `{"profiles":{"ops":{"name":"ops","description":"`+first+`","values":{"client-secret":"`+first+`","client-secret":"`+last+`"}}}}`)
	if out := e2eRun(t, cli, "", "profile", "list", "--json"); e2eCarries(out, first) {
		t.Fatalf("profile list printed the first member:\n%s", out)
	}
}

// A saved value stays readable only under a flag this CLI has and a profile would
// apply; anything else is held, as is a password inside a base-url value, from
// startup, whether or not this run reads the store.
func TestAProfileStoreIsFilteredByAnAllowList(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	secret := e2eSecret("profile-allow-list")
	allowed := "id,name-still-visible"
	for name, store := range map[string]string{
		"base-url":            `{"profiles":{"ops":{"name":"ops","values":{"select":"` + allowed + `","base-url":"` + secret + `"}}}}`,
		"unknown flag":        `{"profiles":{"ops":{"name":"ops","values":{"select":"` + allowed + `","typo-field":"` + secret + `"}}}}`,
		"credential flag":     `{"profiles":{"ops":{"name":"ops","values":{"select":"` + allowed + `","ppkg-options-wireless-key":"` + secret + `"}}}}`,
		"key in another case": `{"PROFILES":{"ops":{"name":"ops","VALUES":{"select":"` + allowed + `","typo-field":"` + secret + `"}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := newStubImmy(t)
			e2eHome(t, s)
			e2eWriteProfiles(t, store)
			if out := e2eRun(t, cli, "", "--"+secret); e2eCarries(out, secret) {
				t.Fatalf("held only once the store is read:\n%s", out)
			}
			out := e2eRun(t, cli, "", "profile", "show", "ops", "--json")
			if e2eCarries(out, secret) {
				t.Fatalf("profile show printed the credential:\n%s", out)
			}
		})
	}
	t.Run("an allowed value stays readable", func(t *testing.T) {
		s := newStubImmy(t)
		e2eHome(t, s)
		e2eWriteProfiles(t, `{"profiles":{"ops":{"name":"ops","description":"weekly audit run","values":{"select":"`+allowed+`","data-source":"https://host.example/x?limit=25"}}}}`)
		out := e2eRun(t, cli, "", "profile", "show", "ops", "--json")
		if !strings.Contains(out, allowed) || !strings.Contains(out, "weekly audit run") || !strings.Contains(out, "limit=25") {
			t.Fatalf("an allowed value was withheld:\n%s", out)
		}
	})
	t.Run("a boolean under a flag no profile applies stays readable", func(t *testing.T) {
		s := newStubImmy(t)
		e2eHome(t, s)
		e2eWriteProfiles(t, `{"profiles":{"ops":{"name":"ops","description":"weekly audit run","values":{"stdin":"true","typo-field":"false"}}}}`)
		out := e2eRun(t, cli, "", "profile", "show", "ops", "--json")
		if !strings.Contains(out, `"true"`) || !strings.Contains(out, "weekly audit run") {
			t.Fatalf("a boolean was held:\n%s", out)
		}
	})
	t.Run("a short password inside a base-url value is held", func(t *testing.T) {
		s := newStubImmy(t)
		e2eHome(t, s)
		e2eWriteProfiles(t, `{"profiles":{"ops":{"name":"ops","values":{"base-url":"https://ops:Zq7x2@host.example/"}}}}`)
		if out := e2eRun(t, cli, "", "--Zq7x2"); e2eCarries(out, "Zq7x2") {
			t.Fatalf("the password inside base-url was not held:\n%s", out)
		}
	})
	for _, word := range []string{"TrUe", "true", "false"} {
		t.Run("a credential that spells "+word+" is held", func(t *testing.T) {
			s := newStubImmy(t)
			e2eHome(t, s)
			e2eWriteProfiles(t, `{"profiles":{"ops":{"name":"ops","values":{"password":"`+word+`"}}}}`)
			out := e2eRun(t, cli, "", "profile", "show", "ops", "--json")
			if strings.Contains(out, `"`+word+`"`) {
				t.Fatalf("profile show printed the credential:\n%s", out)
			}
		})
	}
	for name, tc := range map[string]struct{ base, credential string }{
		"a bad host":   {"https://ops:Zq7x@bad%host/", "Zq7x"},
		"a bad escape": {"https://ops:Zq7x%zz@host.example/", "Zq7x%zz"},
		"no scheme":    {"ops:Zq7x@host.example:3128", "Zq7x"},
		"a raw slash":  {"https://ops:Zq/7x@host.example/", "Zq/7x"},
		"a bad query":  {"https://host.example/?a=%zz;access_token=Zq7x", "Zq7x"},
	} {
		t.Run("a credential inside a base-url value with "+name+" is held", func(t *testing.T) {
			s := newStubImmy(t)
			e2eHome(t, s)
			e2eWriteProfiles(t, `{"profiles":{"ops":{"name":"ops","description":"`+tc.credential+`","values":{"base-url":"`+tc.base+`"}}}}`)
			if out := e2eRun(t, cli, "", "profile", "list", "--json"); strings.Contains(out, tc.credential) {
				t.Fatalf("profile list printed the credential:\n%s", out)
			}
		})
	}
	t.Run("a store that is a bare empty string starts", func(t *testing.T) {
		s := newStubImmy(t)
		e2eHome(t, s)
		e2eWriteProfiles(t, `""`)
		if out := e2eRun(t, cli, "", "--version"); !strings.Contains(out, "immybot-cli") {
			t.Fatalf("the binary did not start:\n%s", out)
		}
	})
	t.Run("store under --home", func(t *testing.T) {
		s := newStubImmy(t)
		e2eHome(t, s)
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, "config"), 0o700); err != nil {
			t.Fatal(err)
		}
		store := `{"profiles":{"ops":{"name":"ops","values":{"typo-field":"` + secret + `"}}}}`
		if err := os.WriteFile(filepath.Join(home, "config", "profiles.json"), []byte(store), 0o600); err != nil {
			t.Fatal(err)
		}
		if out := e2eRun(t, cli, "", "--home", home, "--"+secret); e2eCarries(out, secret) {
			t.Fatalf("the store under --home was not held:\n%s", out)
		}
	})
}

// The origin the kit allows is the one the program will use: an explicit --config
// wins over a discovered default, whose values are only held.
func TestAnExplicitConfigChoosesTheAllowedOrigin(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	request := []string{"access", "list", "--json", "--data-source", "live"}
	for _, tc := range []struct {
		name string
		args func(t *testing.T, config string) []string
	}{
		{"--config", func(t *testing.T, config string) []string {
			path := filepath.Join(t.TempDir(), "chosen.toml")
			if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			return append([]string{"--config", path}, request...)
		}},
		{"IMMYBOT_CONFIG", func(t *testing.T, config string) []string {
			path := filepath.Join(t.TempDir(), "chosen.toml")
			if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("IMMYBOT_CONFIG", path)
			return request
		}},
		{"--home", func(t *testing.T, config string) []string {
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, "config"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "config", "config.toml"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			return append([]string{"--home", home}, request...)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			discovered, chosen := newStubImmy(t), newStubImmy(t)
			e2eHome(t, chosen)
			t.Setenv("IMMYBOT_BASE_URL", "")
			e2eWriteConfig(t, "base_url = \""+discovered.srv.URL+"\"\n")
			args := tc.args(t, "base_url = \""+chosen.srv.URL+"\"\naccess_token = \""+e2eSecret("chosen-token")+"\"\n")
			out := e2eRun(t, cli, "", args...)
			if chosen.reached() == 0 {
				t.Fatalf("the chosen instance was refused:\n%s", out)
			}
			if discovered.reached() != 0 {
				t.Fatalf("the discovered default's instance was reached:\n%s", out)
			}
		})
	}
}

// With no base_url the instance comes from the subdomain, and the kit allows it:
// the origin list names no instance of its own.
func TestTheSubdomainsInstanceIsTheAllowedOrigin(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	s := newStubImmy(t)
	e2eHome(t, s)
	t.Setenv("IMMYBOT_BASE_URL", "")
	t.Setenv("IMMYBOT_SUBDOMAIN", "alpha")
	e2eWriteConfig(t, "access_token = \""+e2eSecret("subdomain-token")+"\"\n")
	host, got := e2eStubProxy(t, s)
	t.Setenv("HTTPS_PROXY", "https://"+host)
	out := e2eRun(t, cli, "", "access", "list", "--json", "--data-source", "live")
	select {
	case <-got:
	default:
		t.Fatalf("the request to the subdomain's instance never left:\n%s", out)
	}
}

// An import record is held from its raw line: decoding keeps the last member of a
// duplicate key, and the first would go on printing wherever the record repeats it.
func TestAnImportRecordIsHeldFromItsRawLine(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	secret := e2eSecret("import-duplicate")
	line := `{"password":"` + secret + `","password":"","host":"` + secret + `"}`
	file := filepath.Join(t.TempDir(), "records.jsonl")
	if err := os.WriteFile(file, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		stdin string
		input string
	}{"--input file": {"", file}, "--input -": {line + "\n", "-"}} {
		t.Run(name, func(t *testing.T) {
			s := newStubImmy(t)
			e2eHome(t, s)
			s.signIn(t)
			if out := e2eRun(t, cli, tc.stdin, "import", "smtp-configs", "--input", tc.input, "--dry-run"); e2eCarries(out, secret) {
				t.Fatalf("the preview printed the first member:\n%s", out)
			}
		})
	}
}

// The kit holds a --stdin body it can see on the command line. A profile never
// turns the flag on, so no body reaches a command unseen.
func TestAProfileCannotTurnOnStdin(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	s := newStubImmy(t)
	e2eHome(t, s)
	s.signIn(t)
	e2eWriteProfiles(t, `{"profiles":{"ops":{"name":"ops","values":{"stdin":"true"}}}}`)
	secret := e2eSecret("profile-stdin")
	body := `{"host":"smtp.example","smtpAccessValue":"` + secret + `"}`
	if out := e2eRun(t, cli, body, "smtp-configs", "create-by-tenant-id", "1", "--profile", "ops", "--dry-run"); e2eCarries(out, secret) {
		t.Fatalf("the preview printed a body the profile let in:\n%s", out)
	}
}
