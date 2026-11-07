// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential classes this connector closes through the shared credential
// kit (handfixes.json credkit-wiring), each on the built binaries
// (CONNECTWISE_RMM_LEAK_BIN_DIR runs them on binaries built elsewhere). Every
// leak case was seen failing on binaries built from the tree before the kit
// was wired, and carries a control, so a run that never reached the sink
// cannot pass.

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"connectwise-rmm-pp-cli/internal/credkit"
)

// Fixture credentials: runs of letters no ordinary output shares an
// 8-character piece with.
const (
	fxClientID     = "0e30kqzwkqzwkqzwkqzw"
	fxClientSecret = "cwsecqzkwmxvjpqzrtmk"
)

// seenRequest is one request a recording fake served.
type seenRequest struct {
	method, path, auth, body string
}

// recordingAPI is fakeAPI that also records every request it served.
func recordingAPI(t *testing.T, answer func(http.Header, *http.Request) (int, string)) (baseURL string, cert *x509.Certificate, seen func() []seenRequest) {
	t.Helper()
	var mu sync.Mutex
	var log []seenRequest
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(b)) // answer may read the body
		mu.Lock()
		log = append(log, seenRequest{r.Method, r.URL.Path, r.Header.Get("Authorization"), string(b)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		status, body := answer(w.Header(), r)
		if loc := w.Header().Get("Location"); loc != "" && status == 0 {
			status = http.StatusTemporaryRedirect
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, srv.Certificate(), func() []seenRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenRequest(nil), log...)
	}
}

// caFile writes every certificate into one PEM bundle for CONNECTWISE_RMM_CA_CERT.
func caFile(t *testing.T, certs ...*x509.Certificate) string {
	t.Helper()
	var b []byte
	for _, c := range certs {
		b = append(b, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})...)
	}
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// (b) A minted token never reaches stdout, stderr, an MCP message or a file
// other than the 0600 credential store: the auto-mint (CLI and MCP), the
// token `auth login` mints (read back by the next run), and the tokens
// `scopes` mints and discards. The fake API repeats the minted token in a
// record or an error; the control repeats a value nothing holds, and it
// prints, so each case proves the echo reaches the sink.
func TestMintedTokenNeverPrinted(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const minted = "mintedqzvkjwxmtpbrqz"
	const control = "echoctlwqzvkxmjtpbzr"
	echoing := func(echo string) (string, string) {
		base, ca := fakeAPI(t, func(_ http.Header, r *http.Request) (int, string) {
			if isToken(r) {
				return 200, tokenJSON(minted)
			}
			return 200, companyPage("issued " + echo)
		})
		return base, ca
	}
	creds := func(home, base, ca string) []string {
		return closedEnv(home, "CONNECTWISE_RMM_BASE_URL="+base, "CONNECTWISE_RMM_CA_CERT="+ca,
			"CONNECTWISE_RMM_CLIENT_ID="+fxClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret)
	}
	t.Run("auto-mint", func(t *testing.T) {
		base, ca := echoing(control)
		out, errOut, _ := runSplit(t, cliBin, creds(t.TempDir(), base, ca), "companies", "list", "--json")
		if !strings.Contains(out+errOut, control) {
			t.Fatalf("control: the echoed value did not print, so this case proves nothing:\n%s%s", out, errOut)
		}
		base, ca = echoing(minted)
		home := t.TempDir()
		out, errOut, _ = runSplit(t, cliBin, creds(home, base, ca), "companies", "list", "--json")
		assertNoPiece(t, "companies list output", out+errOut, minted)
		assertNoPieceInFiles(t, home, minted)
		replies, mcpErr, _ := mcpRun(t, mcpBin, creds(t.TempDir(), base, ca),
			call("connectwise-rmm_read", map[string]any{"endpoint_id": "companies.list"}),
			call("companies_list", map[string]any{}))
		if reply(replies, 2) == "" || reply(replies, 3) == "" {
			t.Fatalf("the MCP calls went unanswered:\n%.3000s\n%s", replies, mcpErr)
		}
		assertNoPiece(t, "MCP messages", replies+mcpErr, minted)
	})
	t.Run("auth login", func(t *testing.T) {
		base, ca := echoing(minted)
		home := t.TempDir()
		env := creds(home, base, ca)
		out, errOut, code := runSplit(t, cliBin, env, "auth", "login")
		if code != 0 || !strings.Contains(out, "Logged in") {
			t.Fatalf("auth login did not log in (exit %d):\n%s%s", code, out, errOut)
		}
		assertNoPiece(t, "auth login output", out+errOut, minted)
		// The next run reads the stored token from credentials.toml.
		stored := closedEnv(home, "CONNECTWISE_RMM_BASE_URL="+base, "CONNECTWISE_RMM_CA_CERT="+ca)
		out, errOut, _ = runSplit(t, cliBin, stored, "companies", "list", "--json")
		if !strings.Contains(out+errOut, "HQ-Record") && !strings.Contains(out+errOut, credkit.Notice("stdout")) {
			t.Fatalf("the run after login printed neither the record nor the kit's notice:\n%s%s", out, errOut)
		}
		assertNoPiece(t, "companies list after login", out+errOut, minted)
		assertNoPieceInFiles(t, home, minted)
	})
	t.Run("scopes probe", func(t *testing.T) {
		// The first scope mints the token and discards it; the second is
		// refused with an error that repeats it in a field no redaction reads.
		probe := func(echo string) string {
			base, ca := fakeAPI(t, func(_ http.Header, r *http.Request) (int, string) {
				b, _ := io.ReadAll(r.Body)
				if strings.Contains(string(b), "scope.second") {
					return 400, fmt.Sprintf(`{"error":"invalid_scope","error_description":"scope refused for holder of %s"}`, echo)
				}
				return 200, tokenJSON(minted)
			})
			home := t.TempDir()
			out, errOut, _ := runSplit(t, cliBin, creds(home, base, ca), "scopes", "--scopes", "scope.first scope.second", "--json")
			assertNoPieceInFiles(t, home, minted)
			return out + errOut
		}
		if out := probe(control); !strings.Contains(out, control) {
			t.Fatalf("control: the denial text did not print, so this case proves nothing:\n%s", out)
		}
		assertNoPiece(t, "scopes output", probe(minted), minted)
	})
}

// (c) A credential is refused before it is sent over plain http, to another
// origin, or across a redirect to another origin: the API client (a stored
// bearer token) and the token request (the client secret in its JSON body).
func TestCredentialRefusedOffOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const bearer = "bearerqzvkjwxmtpzrkq"
	carried := func(reqs []seenRequest) []string {
		var bad []string
		for _, r := range reqs {
			if strings.Contains(r.auth, bearer) || strings.Contains(r.body, fxClientSecret) || strings.Contains(r.body, fxClientID) {
				bad = append(bad, r.method+" "+r.path)
			}
		}
		return bad
	}
	t.Run("plain http, stored bearer", func(t *testing.T) {
		var mu sync.Mutex
		var auths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			auths = append(auths, r.Header.Get("Authorization"))
			mu.Unlock()
			_, _ = io.WriteString(w, companyPage("plain"))
		}))
		t.Cleanup(srv.Close)
		home := t.TempDir()
		writeConfig(t, home, fmt.Sprintf("auth_header = \"Bearer %s\"\n", bearer), false)
		out, errOut, _ := runSplit(t, cliBin, closedEnv(home, "CONNECTWISE_RMM_BASE_URL="+srv.URL), "companies", "list", "--json")
		mu.Lock()
		defer mu.Unlock()
		for _, a := range auths {
			if strings.Contains(a, bearer) {
				t.Errorf("the bearer token went over plain http:\n%s%s", out, errOut)
			}
		}
	})
	t.Run("plain http, client secret to the token endpoint", func(t *testing.T) {
		var mu sync.Mutex
		var bodies []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, string(b))
			mu.Unlock()
			_, _ = io.WriteString(w, tokenJSON("plaintokqzvkjwxmtpbr"))
		}))
		t.Cleanup(srv.Close)
		out, errOut, _ := runSplit(t, cliBin, closedEnv(t.TempDir(), "CONNECTWISE_RMM_BASE_URL="+srv.URL,
			"CONNECTWISE_RMM_CLIENT_ID="+fxClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret), "auth", "login")
		mu.Lock()
		defer mu.Unlock()
		for _, b := range bodies {
			if strings.Contains(b, fxClientSecret) {
				t.Errorf("the client secret went over plain http:\n%s%s", out, errOut)
			}
		}
	})
	t.Run("token URL on another origin", func(t *testing.T) {
		apiBase, apiCert, _ := recordingAPI(t, func(_ http.Header, r *http.Request) (int, string) { return 200, companyPage("api") })
		otherBase, otherCert, otherSeen := recordingAPI(t, func(_ http.Header, r *http.Request) (int, string) { return 200, tokenJSON("othertokqzvkjwxmtpbr") })
		env := closedEnv(t.TempDir(), "CONNECTWISE_RMM_BASE_URL="+apiBase, "CONNECTWISE_RMM_TOKEN_URL="+otherBase+"/v1/token",
			"CONNECTWISE_RMM_CA_CERT="+caFile(t, apiCert, otherCert),
			"CONNECTWISE_RMM_CLIENT_ID="+fxClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret)
		for _, args := range [][]string{{"auth", "login"}, {"companies", "list", "--json"}, {"scopes", "--scopes", "scope.first"}} {
			out, errOut, _ := runSplit(t, cliBin, env, args...)
			if bad := carried(otherSeen()); len(bad) > 0 {
				t.Errorf("%v sent the client credentials to another origin (%v):\n%s%s", args, bad, out, errOut)
			}
		}
	})
	t.Run("API redirect to another origin", func(t *testing.T) {
		otherBase, otherCert, otherSeen := recordingAPI(t, func(_ http.Header, r *http.Request) (int, string) { return 200, companyPage("moved") })
		apiBase, apiCert, apiSeen := recordingAPI(t, func(h http.Header, r *http.Request) (int, string) {
			h.Set("Location", otherBase+r.URL.Path)
			return http.StatusTemporaryRedirect, ""
		})
		home := t.TempDir()
		writeConfig(t, home, fmt.Sprintf("auth_header = \"Bearer %s\"\n", bearer), false)
		out, errOut, _ := runSplit(t, cliBin, closedEnv(home, "CONNECTWISE_RMM_BASE_URL="+apiBase, "CONNECTWISE_RMM_CA_CERT="+caFile(t, apiCert, otherCert)), "companies", "list", "--json")
		if len(apiSeen()) == 0 {
			t.Fatalf("the API was never called, so this case proves nothing:\n%s%s", out, errOut)
		}
		if bad := carried(otherSeen()); len(bad) > 0 {
			t.Errorf("the bearer token followed a redirect to another origin (%v):\n%s%s", bad, out, errOut)
		}
	})
	t.Run("token redirect to another origin", func(t *testing.T) {
		otherBase, otherCert, otherSeen := recordingAPI(t, func(_ http.Header, r *http.Request) (int, string) { return 200, tokenJSON("redirtokqzvkjwxmtpbr") })
		apiBase, apiCert, apiSeen := recordingAPI(t, func(h http.Header, r *http.Request) (int, string) {
			h.Set("Location", otherBase+r.URL.Path)
			return http.StatusTemporaryRedirect, ""
		})
		env := closedEnv(t.TempDir(), "CONNECTWISE_RMM_BASE_URL="+apiBase, "CONNECTWISE_RMM_CA_CERT="+caFile(t, apiCert, otherCert),
			"CONNECTWISE_RMM_CLIENT_ID="+fxClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret)
		out, errOut, _ := runSplit(t, cliBin, env, "auth", "login")
		if len(apiSeen()) == 0 {
			t.Fatalf("the token endpoint was never called, so this case proves nothing:\n%s%s", out, errOut)
		}
		if bad := carried(otherSeen()); len(bad) > 0 {
			t.Errorf("the client secret followed a redirect to another origin (%v):\n%s%s", bad, out, errOut)
		}
	})
}

// (e) A token-endpoint error body whose token key is written with JSON escapes
// (kit-findings row 26) prints no token: auth login, scopes and the auto-mint.
// The kit reads declared response fields with their keys unescaped. The
// control writes the token under a key that is no credential, and it prints.
func TestTokenErrorEscapedKeyPrintsNoToken(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const tok = "escapedtokqzvkjwxmtp"
	run := func(key string) (cli map[string]string, mcp string) {
		base, ca := fakeAPI(t, func(_ http.Header, r *http.Request) (int, string) {
			return 400, `{"error":"invalid_client","` + key + `":"` + tok + `"}`
		})
		env := closedEnv(t.TempDir(), "CONNECTWISE_RMM_BASE_URL="+base, "CONNECTWISE_RMM_CA_CERT="+ca,
			"CONNECTWISE_RMM_CLIENT_ID="+fxClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret)
		cli = map[string]string{}
		for _, args := range [][]string{{"auth", "login"}, {"scopes", "--scopes", "scope.first", "--json"}, {"companies", "list", "--json"}} {
			out, errOut, _ := runSplit(t, cliBin, env, args...)
			cli[strings.Join(args, " ")] = out + errOut
		}
		replies, mcpErr, _ := mcpRun(t, mcpBin, env, call("connectwise-rmm_read", map[string]any{"endpoint_id": "companies.list"}))
		return cli, replies + mcpErr
	}
	ctl, _ := run("detail")
	if !strings.Contains(ctl["auth login"], tok) {
		t.Fatalf("control: the error body did not print, so this test proves nothing:\n%s", ctl["auth login"])
	}
	// The escapes are built from a backslash rune so no tool that decodes
	// escapes in source text can turn them back into the plain key.
	bs := string(rune(92))
	for _, key := range []string{"acc" + bs + "u0065ss_token", bs + "u0061ccess_token", "access" + bs + "u005ftoken"} {
		cli, mcp := run(key)
		for cmd, out := range cli {
			assertNoPiece(t, key+": "+cmd, out, tok)
		}
		assertNoPiece(t, key+": MCP", mcp, tok)
	}
}

// (f) The client ID is held (the manifest marks it sensitive; it is half the
// client-credentials pair), and holding it withholds neither the API host nor
// ordinary output: an Asio client ID is its 4-character region prefix plus a
// unique part, and no 8-character piece of it is public text. A dry run names
// the API host whole and the MCP tool list answers whole, from the environment
// and from config.toml; a response that repeats the client ID is withheld.
func TestClientIDDoesNotWithholdTheAPIHost(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	for _, src := range []string{"environment", "config.toml", "legacy rmm_client_id"} {
		t.Run(src, func(t *testing.T) {
			home := t.TempDir()
			var env []string
			switch src {
			case "environment":
				env = closedEnv(home, "CONNECTWISE_RMM_CLIENT_ID="+fxClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret)
			case "config.toml":
				writeConfig(t, home, fmt.Sprintf("client_id = %q\nclient_secret = %q\n", fxClientID, fxClientSecret), false)
				env = closedEnv(home)
			default:
				writeConfig(t, home, fmt.Sprintf("rmm_client_id = %q\nrmm_client_secret = %q\n", fxClientID, fxClientSecret), false)
				env = closedEnv(home)
			}
			out, errOut, _ := runSplit(t, cliBin, env, "companies", "list", "--dry-run")
			if !strings.Contains(out+errOut, "https://openapi.service.itsupport247.net/api/platform/v1/company/companies") || strings.Contains(out+errOut, "credkit: withheld") {
				t.Errorf("the dry run did not name the API host whole:\n%s%s", out, errOut)
			}
			replies, mcpErr, _ := mcpRun(t, mcpBin, env, rpc{"tools/list", map[string]any{}})
			if list := reply(replies, 2); !strings.Contains(list, `"tools":[`) || !strings.Contains(list, "connectwise-rmm_read") || strings.Contains(replies+mcpErr, "credkit: withheld") {
				t.Errorf("tools/list did not answer whole:\n%.2000s\nstderr:\n%s", replies, mcpErr)
			}
			base, ca := fakeAPI(t, func(_ http.Header, r *http.Request) (int, string) {
				if isToken(r) {
					return 200, tokenJSON("cidacctokqzvkjwxmtpb")
				}
				return 200, companyPage("issued to " + fxClientID)
			})
			live := append(append([]string{}, env...), "CONNECTWISE_RMM_BASE_URL="+base, "CONNECTWISE_RMM_CA_CERT="+ca)
			out, errOut, _ = runSplit(t, cliBin, live, "companies", "list", "--json")
			if strings.Contains(out+errOut, fxClientID) || !strings.Contains(out+errOut, credkit.Notice("stdout")) {
				t.Errorf("a response repeating the client ID was not withheld:\n%s%s", out, errOut)
			}
		})
	}
}

// (g) A --stdin body's credential fields are held before anything prints: a
// dry-run preview of a create or update never repeats a password in the body.
// The body still reaches its own API when sent for real.
func TestStdinBodyCredentialHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const pw = "stdinpwqzvkjwxmtpbrq"
	body := `{"summary":"Printer offline","password":"` + pw + `"}`
	pretty := "{\n  \"name\": \"Branch\",\n  \"password\": \"" + pw + "\"\n}\n"
	patch := `[{"op":"replace","path":"/password","value":"` + pw + `"}]`
	for _, c := range []struct {
		stdin string
		args  []string
	}{
		{body, []string{"tickets", "create", "--stdin", "--dry-run"}},
		{pretty, []string{"sites", "create", "c-1", "--stdin", "--dry-run"}},
		{patch, []string{"tickets", "update", "t-1", "--stdin", "--dry-run"}},
	} {
		t.Run(strings.Join(c.args[:2], " "), func(t *testing.T) {
			home := t.TempDir()
			out, errOut, code := runSplitStdin(t, cliBin, closedEnv(home, "CONNECTWISE_RMM_CLIENT_ID="+fxClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret), c.stdin, c.args...)
			if strings.TrimSpace(out+errOut) == "" && code != credkit.ExitRefused {
				t.Fatalf("the preview printed nothing, so this case proves nothing")
			}
			assertNoPiece(t, "preview", out+errOut, pw)
			assertNoPieceInFiles(t, home, pw)
		})
	}
	t.Run("sent for real", func(t *testing.T) {
		base, cert, seen := recordingAPI(t, func(_ http.Header, r *http.Request) (int, string) {
			if isToken(r) {
				return 200, tokenJSON("stdintokqzvkjwxmtpbr")
			}
			return 200, `{"ticketId":"t-1"}`
		})
		env := closedEnv(t.TempDir(), "CONNECTWISE_RMM_BASE_URL="+base, "CONNECTWISE_RMM_CA_CERT="+caFile(t, cert),
			"CONNECTWISE_RMM_CLIENT_ID="+fxClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret)
		out, errOut, _ := runSplitStdin(t, cliBin, env, body, "tickets", "create", "--stdin")
		var sent bool
		for _, r := range seen() {
			sent = sent || (!strings.HasSuffix(r.path, "/v1/token") && strings.Contains(r.body, pw))
		}
		if !sent {
			t.Errorf("the held body never reached its own API:\n%s%s\n%v", out, errOut, seen())
		}
		assertNoPiece(t, "create output", out+errOut, pw)
	})
}

// The regional host the client ID picks (0e31 Europe) is where this run sends
// its credentials when no base URL is configured: the run mints and calls the
// API there and is not refused. A token URL the config names on a regional
// host other than the base URL's origin is where the program really mints, so
// the pair goes there and the minted token goes to the base URL. (A token URL
// on any other origin is refused: TestCredentialRefusedOffOrigin; the binding
// itself takes only the three exact regional origins:
// config.TestHoldForAsioRegionBindsOnlyTheRegionalOrigins.)
func TestRegionalHostFollowsTheClientID(t *testing.T) {
	cliBin, _ := binaries(t)
	const euHost = "openapi.service.euplatform.connectwise.com"
	const euClientID = "0e31kqzwkqzwkqzwkqzw"
	proxy, ca, served := hostAPI(t, euHost, func(_ http.Header, r *http.Request) (int, string) {
		if isToken(r) {
			return 200, tokenJSON("eutokenqzvkjwxmtpbrq")
		}
		return 200, companyPage("EU-Record")
	})
	t.Run("inferred region", func(t *testing.T) {
		env := closedEnv(t.TempDir(), "HTTPS_PROXY="+proxy, "CONNECTWISE_RMM_CA_CERT="+ca,
			"CONNECTWISE_RMM_CLIENT_ID="+euClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret)
		out, errOut, code := runSplit(t, cliBin, env, "companies", "list", "--json")
		if code != 0 || !strings.Contains(out, "EU-Record") {
			t.Fatalf("the EU tenant's run did not reach its regional host (exit %d):\n%s%s\nserved: %v", code, out, errOut, served())
		}
		var minted, called bool
		for _, s := range served() {
			minted = minted || strings.HasPrefix(s, "POST "+euHost+" /v1/token")
			called = called || (strings.Contains(s, "/company/companies") && strings.HasSuffix(s, "auth=present"))
		}
		if !minted || !called {
			t.Errorf("want a mint and an authenticated call on %s, served: %v", euHost, served())
		}
	})
	t.Run("token URL on the regional host, base URL elsewhere", func(t *testing.T) {
		before := len(served())
		apiBase, apiCert, seen := recordingAPI(t, func(_ http.Header, r *http.Request) (int, string) { return 200, companyPage("API-Record") })
		env := closedEnv(t.TempDir(), "HTTPS_PROXY="+proxy, "NO_PROXY=127.0.0.1",
			"CONNECTWISE_RMM_CA_CERT="+caFile(t, apiCert, mustReadCert(t, ca)),
			"CONNECTWISE_RMM_BASE_URL="+apiBase, "CONNECTWISE_RMM_TOKEN_URL=https://"+euHost+"/v1/token",
			"CONNECTWISE_RMM_CLIENT_ID="+euClientID, "CONNECTWISE_RMM_CLIENT_SECRET="+fxClientSecret)
		out, errOut, code := runSplit(t, cliBin, env, "companies", "list", "--json")
		if code != 0 || !strings.Contains(out, "API-Record") {
			t.Fatalf("the run did not mint on %s and call %s (exit %d):\n%s%s\nserved: %v", euHost, apiBase, code, out, errOut, served())
		}
		var minted bool
		for _, s := range served()[before:] {
			minted = minted || strings.HasPrefix(s, "POST "+euHost+" /v1/token")
		}
		var called bool
		for _, r := range seen() {
			called = called || (strings.Contains(r.path, "/company/companies") && r.auth == "Bearer eutokenqzvkjwxmtpbrq")
		}
		if !minted || !called {
			t.Errorf("want the mint on %s and the minted token on %s; served %v, API saw %v", euHost, apiBase, served()[before:], seen())
		}
	})
}

func mustReadCert(t *testing.T, pemFile string) *x509.Certificate {
	t.Helper()
	b, err := os.ReadFile(pemFile)
	if err != nil {
		t.Fatal(err)
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		t.Fatal("no PEM block in " + pemFile)
	}
	c, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
