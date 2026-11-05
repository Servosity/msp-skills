// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a username or password in SUPEROPS_BASE_URL
// is never shown in any output (superops-base-url-never-printed).

package cli

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/cliutil/testenv"
)

// The userinfo a SUPEROPS_BASE_URL carries in these tests. Both are long
// enough for every decoded form to be looked for.
const (
	urlSecretUser = "urlUserZq81Kd"
	urlSecretPass = "test-secret-cli-password"
)

const urlSecretRawQuery = "query{ getTicketList(input:{page:1,pageSize:5}){ listInfo{ hasMore } } }"

// urlSecretForms is the username, the password and the pair, as written and
// in every form an output could carry them in: URL-escaped, base64 (all four
// alphabets), hex (both cases) and JSON-escaped (short and \u escapes).
func urlSecretForms() []string {
	var out []string
	for _, v := range []string{urlSecretUser, urlSecretPass, urlSecretUser + ":" + urlSecretPass} {
		quoted, _ := json.Marshal(v)
		var uni strings.Builder
		for _, r := range v {
			fmt.Fprintf(&uni, `\u%04x`, r)
		}
		out = append(out,
			v,
			url.QueryEscape(v),
			url.PathEscape(v),
			base64.StdEncoding.EncodeToString([]byte(v)),
			base64.RawStdEncoding.EncodeToString([]byte(v)),
			base64.URLEncoding.EncodeToString([]byte(v)),
			base64.RawURLEncoding.EncodeToString([]byte(v)),
			hex.EncodeToString([]byte(v)),
			string(quoted[1:len(quoted)-1]),
			uni.String(),
		)
	}
	return out
}

// assertNoURLSecret fails when any text carries the base URL userinfo in any
// form, in any letter case.
func assertNoURLSecret(t *testing.T, where string, texts ...string) {
	t.Helper()
	for _, text := range texts {
		low := strings.ToLower(text)
		for _, form := range urlSecretForms() {
			if strings.Contains(low, strings.ToLower(form)) {
				t.Errorf("%s shows the base URL userinfo (%q):\n%s", where, form, text)
				return
			}
		}
	}
}

// runCLIProcess runs the CLI the way main does (Execute, with os.Args), with
// the process stdout and stderr captured: the client writes dry-run previews
// and retry notices straight to os.Stderr, and Cobra prints the returned
// error there.
func runCLIProcess(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outR, outW, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	errR, errW, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	drain := func(r *os.File, into chan<- string) {
		b, _ := io.ReadAll(r)
		into <- string(b)
	}
	outC, errC := make(chan string, 1), make(chan string, 1)
	go drain(outR, outC)
	go drain(errR, errC)
	origArgs, origOut, origErr := os.Args, os.Stdout, os.Stderr
	func() {
		defer func() { os.Args, os.Stdout, os.Stderr = origArgs, origOut, origErr }()
		os.Args = append([]string{"superops-cli"}, args...)
		os.Stdout, os.Stderr = outW, errW
		err = Execute()
	}()
	_ = outW.Close()
	_ = errW.Close()
	return <-outC, <-errC, err
}

var urlSecretOpRe = regexp.MustCompile(`\b(get\w+List|getKbItems|getWorklogEntries)\s*\(`)

// serveURLSecretStub answers POST /msp with one ticket for getTicketList and
// nothing for the rest, and every path under /err with HTTP 401 whose body
// echoes the Authorization header and the request URL.
func serveURLSecretStub(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/err") {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": []any{map[string]any{
				"message": "denied: authorization " + r.Header.Get("Authorization") + " for " + r.Host + r.URL.String(),
			}}})
			return
		}
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		m := urlSecretOpRe.FindStringSubmatch(req.Query)
		if r.Method != http.MethodPost || m == nil {
			_, _ = io.WriteString(w, `{"data":{}}`)
			return
		}
		var nodes []map[string]any
		if m[1] == "getTicketList" {
			nodes = append(nodes, map[string]any{"ticketId": "7300000000000001", "displayId": "M-1", "subject": "url secret ticket", "status": "Open", "updatedTime": "2026-01-01T00:00:00"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{m[1]: map[string]any{
			"nodes": nodes, "listInfo": map[string]any{"hasMore": false, "page": 1, "pageSize": 50, "totalCount": len(nodes)},
		}}})
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// closedLoopbackPort is a 127.0.0.1 port nothing listens on.
func closedLoopbackPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close()
	return port
}

func urlSecretBase(hostport string) string {
	return "http://" + urlSecretUser + ":" + urlSecretPass + "@" + hostport
}

// urlSecretEnv isolates the run and sets the connector environment. An empty
// base leaves SUPEROPS_BASE_URL unset.
func urlSecretEnv(t *testing.T, base, token string, dogfood bool) string {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("SUPEROPS_BASE_URL", base)
	t.Setenv("SUPEROPS_API_TOKEN", token)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-url")
	t.Setenv("SUPEROPS_REGION", "")
	t.Setenv(cliutil.VerifyEnvVar, "")
	if dogfood {
		// One attempt per request: the retry notice is covered by the
		// transport case that keeps the retries.
		t.Setenv(cliutil.DogfoodEnvVar, "1")
	} else {
		t.Setenv(cliutil.DogfoodEnvVar, "")
	}
	return home
}

// Every output the CLI makes, for every way the base URL reaches one: doctor
// (human, --json, --agent), --dry-run previews, live reads, a transport
// failure (with and without retries), an HTTP error whose body echoes the
// request, a base URL that does not parse, sync events and a delivered file.
// None shows the username or the password, in any form. The host and port
// are still shown where they were.
func TestBaseURLUserinfoIsNeverShown(t *testing.T) {
	host := serveURLSecretStub(t)
	good := urlSecretBase(host)
	dead := urlSecretBase("127.0.0.1:" + closedLoopbackPort(t))
	errBase := good + "/err"
	badPort := urlSecretBase("127.0.0.1:notaport")
	// The password holds an unescaped '/', and its '@' is escaped: the
	// parser reads the username as the host and the password as the path.
	displaced := "http://" + urlSecretUser + ":1/" + urlSecretPass + "%40" + host
	const token = "sop-url-secret-token-5d1c"

	cases := []struct {
		name    string
		base    string
		dogfood bool
		args    []string
		// want is text the output must still hold, so a case that never
		// reached the path it names fails instead of passing.
		want []string
	}{
		{"doctor", good, false, []string{"doctor"}, []string{"base_url: http://" + host}},
		{"doctor-json", good, false, []string{"doctor", "--json"}, []string{`"base_url": "http://` + host + `"`}},
		{"doctor-agent", good, false, []string{"doctor", "--agent"}, []string{"http://" + host}},
		{"alerts-dry-run", good, false, []string{"alerts", "--dry-run"}, []string{"GET http://" + host + "/graphql"}},
		{"tickets-list-dry-run", good, false, []string{"tickets", "list", "--dry-run"}, []string{"POST http://" + host + "/msp"}},
		{"tickets-list-dry-run-json", good, false, []string{"tickets", "list", "--dry-run", "--json"}, []string{"POST http://" + host + "/msp"}},
		{"raw-query-dry-run", good, false, []string{"raw", "query", urlSecretRawQuery, "--dry-run"}, []string{"getTicketList"}},
		{"sync-dry-run", good, false, []string{"sync", "--resources", "tickets", "--dry-run"}, []string{"/msp"}},
		{"tickets-list-live", good, false, []string{"tickets", "list", "--json"}, []string{"7300000000000001"}},
		{"tickets-list-live-agent", good, false, []string{"tickets", "list", "--agent"}, []string{"7300000000000001"}},
		{"raw-query-live", good, false, []string{"raw", "query", urlSecretRawQuery, "--json"}, []string{"getTicketList"}},
		{"sync-live", good, false, []string{"sync", "--resources", "tickets"}, []string{"sync_summary"}},
		{"tickets-list-transport-retries", dead, false, []string{"tickets", "list"}, []string{"retrying", refusedDialText()}},
		{"doctor-transport", dead, true, []string{"doctor"}, []string{"unreachable"}},
		{"doctor-json-transport", dead, true, []string{"doctor", "--json"}, []string{"unreachable"}},
		{"tickets-list-agent-transport", dead, true, []string{"tickets", "list", "--agent", "--data-source", "live"}, []string{refusedDialText()}},
		{"raw-query-transport", dead, true, []string{"raw", "query", urlSecretRawQuery}, []string{refusedDialText()}},
		{"sync-transport", dead, true, []string{"sync", "--resources", "tickets"}, []string{"sync_error"}},
		{"doctor-http-error", errBase, false, []string{"doctor", "--json"}, []string{"base_url"}},
		{"tickets-list-http-error", errBase, false, []string{"tickets", "list", "--agent"}, []string{"401"}},
		{"raw-query-http-error", errBase, false, []string{"raw", "query", urlSecretRawQuery, "--json"}, []string{"401"}},
		{"sync-http-error", errBase, false, []string{"sync", "--resources", "tickets"}, []string{"sync_"}},
		{"doctor-json-bad-port", badPort, true, []string{"doctor", "--json"}, []string{"base_url"}},
		{"tickets-list-bad-port", badPort, true, []string{"tickets", "list", "--json"}, []string{"invalid port"}},
		{"tickets-list-dry-run-bad-port", badPort, true, []string{"tickets", "list", "--dry-run"}, []string{"POST "}},
		{"raw-query-bad-port", badPort, true, []string{"raw", "query", urlSecretRawQuery}, []string{"invalid port"}},
		{"tickets-list-dry-run-displaced", displaced, false, []string{"tickets", "list", "--dry-run"}, []string{"base URL refused"}},
		{"doctor-json-displaced", displaced, true, []string{"doctor", "--json"}, []string{"base URL refused"}},
		{"tickets-list-displaced", displaced, true, []string{"tickets", "list", "--json"}, []string{"base URL refused"}},
		{"raw-query-displaced", displaced, true, []string{"raw", "query", urlSecretRawQuery}, []string{"base URL refused"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			urlSecretEnv(t, tc.base, token, tc.dogfood)
			stdout, stderr, err := runCLIProcess(t, tc.args...)
			errText := ""
			if err != nil {
				errText = err.Error()
			}
			assertNoURLSecret(t, "superops-cli "+strings.Join(tc.args, " "), stdout, stderr, errText)
			all := stdout + stderr + errText
			for _, w := range tc.want {
				if !strings.Contains(all, w) {
					t.Errorf("superops-cli %s never showed %q (the case did not reach its path):\nstdout:\n%s\nstderr:\n%s\nerr: %s", strings.Join(tc.args, " "), w, stdout, stderr, errText)
				}
			}
		})
	}

	// A query or fragment on the base URL is refused at config load. Its
	// value is one no case registers with the process-wide funnel, so only
	// the refusal keeps it out of request errors and retry notices.
	const queryValue = "QueryValueQx73Lm"
	for i, base := range []string{
		"http://127.0.0.1:" + closedLoopbackPort(t) + "/?sig=" + queryValue,
		"http://127.0.0.1:" + closedLoopbackPort(t) + "#" + queryValue,
	} {
		for _, args := range [][]string{
			{"raw", "query", urlSecretRawQuery},
			{"tickets", "list", "--agent", "--data-source", "live"},
			{"doctor", "--json"},
		} {
			t.Run(fmt.Sprintf("query-or-fragment-%d-%s", i, args[0]), func(t *testing.T) {
				urlSecretEnv(t, base, token, true)
				stdout, stderr, err := runCLIProcess(t, args...)
				all := stdout + stderr + fmt.Sprint(err)
				if strings.Contains(all, queryValue) {
					t.Errorf("superops-cli %s shows the base URL query or fragment:\n%s", strings.Join(args, " "), all)
				}
				if !strings.Contains(all, "base URL refused") {
					t.Errorf("superops-cli %s never showed the refusal:\n%s", strings.Join(args, " "), all)
				}
			})
		}
	}

	// --deliver writes the output to a file; --receipt is accepted.
	t.Run("deliver-file-and-receipt", func(t *testing.T) {
		home := urlSecretEnv(t, good, token, false)
		dest := filepath.Join(home, "delivered.json")
		stdout, stderr, err := runCLIProcess(t, "tickets", "list", "--json", "--receipt", "--deliver", "file:"+dest)
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		delivered, _ := os.ReadFile(dest)
		assertNoURLSecret(t, "tickets list --deliver", stdout, stderr, errText, string(delivered))
		if !strings.Contains(string(delivered), "7300000000000001") {
			t.Fatalf("nothing was delivered:\n%s\n%s\n%s", delivered, stderr, errText)
		}
	})

	// No token: the request goes out with the userinfo as HTTP Basic auth,
	// and the 401 body echoes the header back.
	t.Run("basic-auth-echo", func(t *testing.T) {
		urlSecretEnv(t, errBase, "", false)
		stdout, stderr, err := runCLIProcess(t, "raw", "query", urlSecretRawQuery, "--json")
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		assertNoURLSecret(t, "raw query without a token", stdout, stderr, errText)
	})
}

// A short username and password (shorter than the funnel searches decoded
// text for) are never shown. The password has three characters: the holder
// keeps it for the rest of this package's run, and a two-character value
// also matches random hashes other tests write. config.Load refuses a
// password shorter than cliutil.MinDetectableSecretLen
// (superops-credential-floor), so no request, preview or error is built from
// it, and the refusal shows none of it.
func TestShortBaseURLUserinfoIsNeverShown(t *testing.T) {
	host := serveURLSecretStub(t)
	dead := "127.0.0.1:" + closedLoopbackPort(t)
	forbidden := []string{"svc:pw7", "svc:***", "svc@", "//svc", base64.StdEncoding.EncodeToString([]byte("svc:pw7"))}
	for _, tc := range []struct {
		name, base, token string
		args              []string
		want              string
	}{
		{"doctor-json", "http://svc:pw7@" + host, "sop-url-secret-token-5d1c", []string{"doctor", "--json"}, "credential refused"},
		{"dry-run", "http://svc:pw7@" + host, "sop-url-secret-token-5d1c", []string{"tickets", "list", "--dry-run"}, "credential refused"},
		{"transport", "http://svc:pw7@" + dead, "sop-url-secret-token-5d1c", []string{"raw", "query", urlSecretRawQuery}, "credential refused"},
		{"basic-auth-echo", "http://svc:pw7@" + host + "/err", "", []string{"raw", "query", urlSecretRawQuery, "--json"}, "credential refused"},
		{"bad-port", "http://svc:pw7@127.0.0.1:notaport", "sop-url-secret-token-5d1c", []string{"tickets", "list", "--json"}, "credential refused"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			urlSecretEnv(t, tc.base, tc.token, true)
			stdout, stderr, err := runCLIProcess(t, tc.args...)
			errText := ""
			if err != nil {
				errText = err.Error()
			}
			all := stdout + stderr + errText
			for _, f := range forbidden {
				if strings.Contains(all, f) {
					t.Errorf("superops-cli %s shows %q:\n%s", strings.Join(tc.args, " "), f, all)
				}
			}
			if !strings.Contains(all, tc.want) {
				t.Errorf("superops-cli %s never showed %q (the case did not reach its path):\n%s", strings.Join(tc.args, " "), tc.want, all)
			}
		})
	}
}

// The jobs ledger keeps a wait failure's error text. A row an earlier build
// recorded with net/http's url.Error (username kept, password masked) is
// shown by jobs list and jobs get without the userinfo, and a new row is
// recorded without it.
func TestJobsLedgerNeverShowsBaseURLUserinfo(t *testing.T) {
	urlSecretEnv(t, "", "", true)
	legacyErr := `GET /graphql: Get "http://` + urlSecretUser + `:***@127.0.0.1:9/graphql": dial tcp 127.0.0.1:9: i/o timeout`
	if err := RecordJob(JobRow{JobID: "job-url-secret-1", Resource: "tasks", Endpoint: "list", Status: "submitted", Error: legacyErr}); err != nil {
		t.Fatal(err)
	}
	p, err := jobsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(p)
	assertNoURLSecret(t, "the jobs ledger file", string(stored))
	// An earlier build's row, written as it was.
	legacy, _ := json.Marshal(JobRow{JobID: "job-url-secret-2", Resource: "tasks", Endpoint: "list", Status: "submitted", Error: legacyErr})
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(append(legacy, '\n'))
	_ = f.Close()
	for _, args := range [][]string{{"jobs", "list", "--json"}, {"jobs", "get", "job-url-secret-2"}, {"jobs", "get", "job-url-secret-2", "--json"}} {
		stdout, stderr, err := runCLIProcess(t, args...)
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		assertNoURLSecret(t, strings.Join(args, " "), stdout, stderr, errText)
		if !strings.Contains(stdout, "i/o timeout") {
			t.Errorf("%v did not show the recorded error:\n%s\n%s", args, stdout, stderr)
		}
	}
}

// writeURLSecretConfig writes the default config file naming base_url with
// the userinfo, readable by the group and others, so the untrusted-config path
// refuses the credential and says why.
func writeURLSecretConfig(t *testing.T, base string) string {
	t.Helper()
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	makeReadableByOthers(t, path)
	return path
}

// The credential-refusal messages name the base URL: an untrusted config
// (group/world-readable) with an env token, the same with a stored token, and
// global credentials refused for an explicit --config (the target URL and
// the recorded one). The refusal is shown on stderr when it happens, in
// doctor and auth status, and as the command error; none shows the userinfo.
func TestBaseURLUserinfoIsNeverShownInCredentialRefusals(t *testing.T) {
	host := serveURLSecretStub(t)
	good := urlSecretBase(host)
	commands := [][]string{{"doctor"}, {"doctor", "--json"}, {"tickets", "list", "--json"}, {"auth", "status"}}

	t.Run("untrusted-config-env-token", func(t *testing.T) {
		for _, args := range commands {
			urlSecretEnv(t, "", "sop-url-secret-token-5d1c", true)
			writeURLSecretConfig(t, good)
			stdout, stderr, err := runCLIProcess(t, args...)
			errText := ""
			if err != nil {
				errText = err.Error()
			}
			assertNoURLSecret(t, strings.Join(args, " "), stdout, stderr, errText)
			if all := stdout + stderr + errText; !strings.Contains(all, "untrusted config names base_url") || !strings.Contains(all, host) {
				t.Errorf("%v: the refusal was not shown with the host:\n%s", args, all)
			}
		}
	})

	t.Run("untrusted-config-stored-token", func(t *testing.T) {
		for _, args := range commands {
			urlSecretEnv(t, "", "", true)
			if err := cliutil.SaveCredentials(&cliutil.Credentials{SuperopsApiToken: "sop-url-secret-stored-77e0"}); err != nil {
				t.Fatal(err)
			}
			writeURLSecretConfig(t, good)
			stdout, stderr, err := runCLIProcess(t, args...)
			errText := ""
			if err != nil {
				errText = err.Error()
			}
			assertNoURLSecret(t, strings.Join(args, " "), stdout, stderr, errText)
			if all := stdout + stderr + errText; !strings.Contains(all, "untrusted config names base_url") {
				t.Errorf("%v: the refusal was not shown:\n%s", args, all)
			}
		}
	})

	for _, tc := range []struct{ name, issued, target string }{
		{"global-credentials-target", "https://api.superops.ai", good},
		{"global-credentials-recorded", urlSecretBase("127.0.0.1:1"), "http://" + host},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			home := urlSecretEnv(t, tc.target, "", true)
			if err := cliutil.SaveCredentials(&cliutil.Credentials{BaseURL: tc.issued, SuperopsApiToken: "sop-url-secret-global-3a9f"}); err != nil {
				t.Fatal(err)
			}
			explicit := filepath.Join(home, "explicit", "config.toml")
			if err := os.MkdirAll(filepath.Dir(explicit), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(explicit, []byte("subdomain = \"acme-url\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"doctor", "--json"}, {"tickets", "list", "--json"}} {
				args = append([]string{"--config", explicit}, args...)
				stdout, stderr, err := runCLIProcess(t, args...)
				errText := ""
				if err != nil {
					errText = err.Error()
				}
				assertNoURLSecret(t, strings.Join(args, " "), stdout, stderr, errText)
				if all := stdout + stderr + errText; !strings.Contains(all, "global credentials for") {
					t.Errorf("%v: the refusal was not shown:\n%s", args, all)
				}
			}
		})
	}
}

// refusedDialText is how the OS words a dial to a closed local port.
func refusedDialText() string {
	if runtime.GOOS == "windows" {
		return "actively refused"
	}
	return "connection refused"
}
