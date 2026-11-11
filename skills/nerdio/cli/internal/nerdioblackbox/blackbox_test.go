// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package nerdioblackbox runs the built CLI and MCP binaries against a fake
// Nerdio Manager for MSP (NMM) instance that echoes every credential it was
// sent back in every encoding it can, whole and split across lines and
// adjacent values, with the Entra ID token endpoint on its origin, and checks
// what a reviewer cannot read off the code (nerdio handfixes credkit-wiring,
// credential-fields-masked-by-name, no-response-cache, api-origin-pinned,
// dry-run-sends-nothing, mcp-endpoint-write-confirm, client-credentials-mint,
// minted-token-bound-to-its-client, token-url-from-operator-only,
// oauth-scope-required):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the NERDIO_CLIENT_ID + NERDIO_CLIENT_SECRET
//     pair, the access token the fake Entra tenant mints for that pair
//     (fresh in every process, or cached by an earlier run), the token stored
//     with auth set-token, the MCP HTTP token, a credential-named flag value,
//     and the credentials a response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     learn journal, config.toml), and the credential store never holds an
//     environment pair or the token minted for one;
//   - no credential leaves for a host other than the configured NMM instance
//     and the configured Entra token URL by redirect, the token mint
//     included, and the client pair leaves only in the token POST;
//   - the MCP server sends a write only when the call sets confirm to the
//     boolean true, and --dry-run sends nothing at all (no mint either).
//
// The fakes model Nerdio: every MSP runs its own NMM instance
// (NERDIO_BASE_URL, the API under /rest-api/v1 and /rest-api/v1-beta, lists
// answered as bare JSON arrays, no paging), and the token comes from the
// MSP's Entra ID tenant (NERDIO_TOKEN_URL,
// <tenant host>/<TENANT_ID>/oauth2/v2.0/token, served here on the NMM
// instance's origin: the credential kit sends the pair only to base_url and
// Entra's own hosts, which no loopback fake can be: POST, a form body
// grant_type=client_credentials&scope=<NERDIO_OAUTH_SCOPE> with the client
// pair as client_id and client_secret, answered {"access_token","expires_in"}).
//
// A unit (a printed document, a written record, a request) that repeats a
// credential is withheld whole by the shared credential kit, with its notice;
// a quiet fake (quietServer) repeats no credential, and its records must reach
// every sink, so a withheld unit is the credential and not a broken sink. A
// response member named as a credential (accessToken, sessionToken) is masked
// by name, so the record around it still prints.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder. The fake teaches the holder nothing:
// it echoes a credential only under names that do not read as credentials and
// never in a shape the holder learns from (an auth-scheme value such as
// "Bearer <v>" or "Basic <v>" is held under any name), so a value the program
// failed to hold from its real source (the environment, the store, the mint,
// the argv) is printed, and the scan fails. TestEchoTeachesTheHolderNothing
// proves it.
package nerdioblackbox

import (
	"bytes"
	"compress/gzip"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bbClientID     = "clientId-3e9a7c1d5b2f8a46"                             // NERDIO_CLIENT_ID
	bbClientSecret = "clientSecret-9d2b6f4a1c8e7305ab"                       // NERDIO_CLIENT_SECRET
	bbMintAccess   = "mintedAccess-5c1e9a3f7b2d4e60c8"                       // the access_token the fake Entra tenant mints for the pair
	bbStored       = "storedToken-71c3e9b05d2a4f68e1"                        // auth set-token (stdin), the credential store
	bbArgvSec      = "-argvSecret-91c3e7a5"                                  // --client-secret on the command line (starts with "-")
	bbShortResp    = "Sh0rtPw"                                               // a 7-character credential a response carries (sessionToken, a declared ResponseField)
	bbBOMResp      = "BomRespSecret-6a1c"                                    // a credential in a BOM-prefixed response
	bbGzipResp     = "GzipRespToken-4f2a9c1e"                                // a credential in a gzip-encoded response
	bbHTTPToken    = "mcpHttpToken-0123456789abcdefABCDEF"                   // NERDIO_MCP_HTTP_TOKEN
	bbScope        = "0e1f2a3b-4c5d-4e6f-8a9b-0c1d2e3f4a5b/.default"         // NERDIO_OAUTH_SCOPE: the NMM application's scope, sent in the token form, not a credential
	bbTenant       = "7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a"                  // the Entra tenant in NERDIO_TOKEN_URL
	bbAccount      = "101"                                                   // an NMM account id
	bbDevice       = "550e8400-e29b-41d4-a716-446655440000"                  // an Intune device id
	bbUser         = "5c6d7e8f-9a0b-4c1d-8e2f-3a4b5c6d7e8f"                  // an Entra user object id
	bbReservation  = "res-7731"                                              // a reservation id
	bbSub          = "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"                  // an Azure subscription id
	bbRG           = "rg-avd-east"                                           // a resource group
	bbPool         = "pool-east"                                             // a host pool name
	bbDenied       = "denied-4f1e"                                           // an id the fake answers 401
	bbControl      = "ctlEchoValue-7a1b9c2d3e"                               // not a credential: the fake echoes it like one (TestEchoTeachesTheHolderNothing)
	bbLAPS         = "LapsTenantPassw0rd-6e2b9d4f1a"                         // a device's LAPS password: a tenant secret the operator asks for by name
	bbBitLocker    = "BitLockerRecovery-118822-334455-667788"                // a device's BitLocker recovery key: a tenant secret
	bbSecureVar    = "SecureVarValue-DomainJoin-5c9e1b"                      // an MSP-level secure variable value: a tenant secret
	bbAcctSecVar   = "SecureVarValue-Account-2f7a4d"                         // an account secure variable value: a tenant secret
	bbSessionTok   = "avdSessTok-8d3f1a6c2e9b7"                              // a sessionToken an AVD sessions response carries: a credential (never its member's name: any 8-character piece of a held value is withheld, kit contract 2(b))
	bbTokenPath    = "/" + bbTenant + "/oauth2/v2.0/token"                   // the token path on the Entra host
	bbTokenRefusal = "token host took a request that was not the token POST" // the fake's own marker
)

// withheldNotice is the start of what the credential kit prints in place of a
// unit that carried a credential.
const withheldNotice = "[credkit: withheld"

// fieldMask is what the client prints in place of a credential-named member.
const fieldMask = `"[withheld]"`

func secrets() []string {
	return []string{bbClientID, bbClientSecret, bbMintAccess, bbStored, bbArgvSec,
		bbShortResp, bbBOMResp, bbGzipResp, bbHTTPToken}
}

// echoed is every value the fake echoes when a request carries it: the
// secrets and the control value.
func echoed() []string { return append(secrets(), bbControl) }

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "nerdio-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "nerdio-cli"+exe)
	bbMCP = filepath.Join(dir, "nerdio-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/nerdio-cli", bbMCP: "../../cmd/nerdio-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// echoForms is every encoding of s the server sends back whole. It holds no
// auth-scheme form ("Bearer <s>"): the holder learns a Bearer-shaped value
// under any name, so echoing one would teach it every secret it was sent and
// make every scan below vacuous (the salesbuildr oracle blind spot). The raw
// form already finds a printed "Bearer <s>".
func echoForms(s string) []string {
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return []string{
		s,
		"x" + s + "x",
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
	}
}

func halves(s string) (string, string) { return s[:len(s)/2], s[len(s)/2:] }

// jsonUnicode spells s as \uXXXX escapes, so the raw value appears only once
// a JSON decoder has read it.
func jsonUnicode(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

// echoObject repeats each of sent whole, in every form, under names that do
// not read as credentials.
func echoObject(sent []string) string {
	parts := []string{`"note":"echo"`}
	for i, s := range sent {
		for j, f := range echoForms(s) {
			v, _ := json.Marshal(f)
			parts = append(parts, fmt.Sprintf(`"e%d_%d":%s`, i, j, v))
		}
		parts = append(parts, fmt.Sprintf(`"u%d":"%s"`, i, jsonUnicode(s)))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// splitObject repeats each of sent split: across two adjacent values, across
// an embedded newline, and across a space.
func splitObject(sent []string) string {
	parts := []string{`"note":"split"`}
	for i, s := range sent {
		a, b := halves(s)
		parts = append(parts, fmt.Sprintf(`"s%d_a":%q,"s%d_b":%q,"s%d_nl":%q,"s%d_sp":%q`, i, a, i, b, i, a+"\n"+b, i, a+" "+b))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// sentValues is every known value the request carried (its headers, the URL
// or the body): a real API can echo only what it was sent.
func sentValues(r *http.Request, body []byte) []string {
	seen := fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
	if u, err := url.PathUnescape(r.URL.EscapedPath()); err == nil {
		seen += "\n" + u
	}
	var out []string
	for _, s := range echoed() {
		if strings.Contains(seen, s) {
			out = append(out, s)
		}
	}
	return out
}

type server struct {
	url      string // the API base URL: the NMM instance root
	tokenURL string // NERDIO_TOKEN_URL: the token endpoint, on the API's origin (see echoServer)
	caFile   string
	mu       sync.Mutex
	// quiet repeats no credential it was sent (only bbControl) and repeats no
	// response credential under another name: a real NMM instance, the
	// control for what reaches a sink.
	quiet    bool
	offHost  int
	methods  []string       // API requests, "METHOD /path"
	headers  map[string]int // Authorization header values those requests carried
	mints    int            // token host requests that were the token POST path
	minted   int            // ... that the fake answered with the minted token
	badMint  []string       // ... the fake refused, and why
	scopes   []string       // the scope of each token request
	tokenOff []string       // token host requests that were not the token POST path at all
	denied   int            // API requests the fake answered 401 (the client may re-mint after one)
	// redirectAccounts sends the accounts list (a sync resource) to the
	// other host too, so sync meets a redirect.
	redirectAccounts bool
	// redirectToken sends the token endpoint to the other host (a 307 would
	// resend the request and the client pair in its form body).
	redirectToken bool
}

// tlsServer starts an HTTP/2 TLS fake. Every httptest TLS server presents the
// same localhost certificate, so one CA file trusts the NMM instance, the
// Entra tenant and the other host alike.
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// Records shaped the way spec.yaml and the novel commands read them.
const (
	accountRecord  = `"id":101,"name":"HQ-Record","tenantId":"6a5b4c3d-2e1f-4a0b-9c8d-7e6f5a4b3c2d"`
	userRecord     = `"id":"5c6d7e8f-9a0b-4c1d-8e2f-3a4b5c6d7e8f","displayName":"HQ Admin","userPrincipalName":"hq@contoso.example"`
	reservationRec = `"id":"res-7731","orderName":"HQ-Reservation","quantity":2`
	deviceRecord   = `"id":"550e8400-e29b-41d4-a716-446655440000","name":"HQ-Device","complianceState":"compliant"`
	poolRecord     = `"name":"pool-east","subscriptionId":"a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d","resourceGroup":"rg-avd-east","description":"HQ-Pool"`
	hostRecord     = `"name":"HQ-Host-0","powerState":"Running","status":"Available"`
	scriptRecord   = `"id":7,"name":"HQ-Script","description":"rotate local admin"`
	scheduleRecord = `"id":9,"name":"HQ-Schedule"`
)

// array wraps items the way every NMM list answers: a bare JSON array.
func array(items ...string) string { return "[" + strings.Join(items, ",") + "]" }

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{headers: map[string]int{}}
	// A third host the API and the token host redirect to: a credential must
	// never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(array()))
	}))
	// The NMM instance, and the token endpoint on its origin: the credential
	// kit sends the client pair only to base_url and Entra's sign-in hosts
	// (internal/credwire), and a loopback fake can be neither of those
	// hosts, so the token path is served here (as the verify and class
	// harnesses serve it). The pair may still leave only in the token POST:
	// an API request carrying HTTP Basic is recorded as a token-path miss.
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == bbTokenPath {
			sv.token(w, r, body, other.URL)
			return
		}
		if _, _, basic := r.BasicAuth(); basic {
			sv.mu.Lock()
			sv.tokenOff = append(sv.tokenOff, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
			sv.mu.Unlock()
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, bbTokenRefusal)
			return
		}
		sent := sentValues(r, body)
		sv.mu.Lock()
		quiet := sv.quiet
		sv.mu.Unlock()
		if quiet {
			sent = slices.DeleteFunc(sent, func(v string) bool { return v != bbControl })
		}
		echo := echoObject(sent)
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasPrefix(r.URL.Path, "/rest-api/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		p := r.URL.Path
		get := r.Method == http.MethodGet
		seg := strings.Split(strings.Trim(strings.TrimPrefix(p, "/rest-api/"), "/"), "/")
		last := seg[len(seg)-1]
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		if h := r.Header.Get("Authorization"); h != "" {
			sv.headers[h]++
		}
		redirectAccounts := sv.redirectAccounts
		sv.mu.Unlock()
		echoID := ""
		if len(sent) > 0 {
			// A record whose identifier or name repeats the first value it was sent.
			echoID = fmt.Sprintf(`{"id":%q,"name":"Echo-ID","status":"Active"}`, sent[0])
		}
		list := func(rec string) string {
			items := []string{fmt.Sprintf(`{%s,"echo":%s}`, rec, echo)}
			if echoID != "" {
				items = append(items, echoID)
			}
			return array(items...)
		}
		switch {
		case p == "/rest-api/v1/app-role-assignments" || p == "/rest-api/v1/costestimator/list" || strings.Contains(p, "/"+bbDenied):
			// Rejected: the error repeats what it was sent.
			sv.mu.Lock()
			sv.denied++
			sv.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"errorCode":"Unauthorized","errorMessage":"bad request %s","details":"invalid request %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/rest-api/v1/accounts/"+bbAccount+"/workspace" || (redirectAccounts && p == "/rest-api/v1/accounts"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && last == "local-admin-password":
			// Tenant secrets the operator asks for by name: shown, never stored.
			fmt.Fprintf(w, `{"accountName":"localadmin","password":%q,"passwordExpirationDateTime":"2026-12-01T00:00:00Z"}`, bbLAPS)
		case get && last == "bitlocker-keys":
			fmt.Fprintf(w, `[{"id":"blk-1","volumeType":"operatingSystemVolume","key":%q}]`, bbBitLocker)
		case get && p == "/rest-api/v1/secure-variables":
			fmt.Fprintf(w, `[{"name":"DomainJoinPassword","value":%q,"scriptedActions":[7]}]`, bbSecureVar)
		case get && len(seg) == 4 && seg[1] == "accounts" && last == "secure-variables":
			fmt.Fprintf(w, `[{"name":"AccountAdminPassword","value":%q}]`, bbAcctSecVar)
		case get && len(seg) == 5 && seg[0] == "v1-beta" && seg[3] == "devices":
			// device get: the echo split across values, lines and spaces.
			fmt.Fprintf(w, `{%s,"split":%s}`, deviceRecord, splitObject(sent))
		case quiet && get && len(seg) == 5 && seg[3] == "users":
			// user get: BOM-prefixed, the access token under its own name only.
			fmt.Fprint(w, "\xef\xbb\xbf"+fmt.Sprintf(`{%s,"accessToken":%q}`, userRecord, bbBOMResp))
		case quiet && get && len(seg) == 5 && seg[3] == "reservations":
			fmt.Fprintf(w, `{%s,"sessionToken":%q}`, reservationRec, bbShortResp)
		case get && len(seg) == 5 && seg[3] == "users":
			// user get: BOM-prefixed, with a password.
			fmt.Fprint(w, "\xef\xbb\xbf"+fmt.Sprintf(`{%s,"note":%q,"accessToken":%q}`, userRecord, bbBOMResp, bbBOMResp))
		case get && len(seg) == 5 && seg[3] == "reservations":
			// reservation get: a short credential.
			fmt.Fprintf(w, `{%s,"note":%q,"sessionToken":%q}`, reservationRec, bbShortResp, bbShortResp)
		case get && p == "/rest-api/v1/accounts":
			// accounts list, sync, export: gzip-encoded, with an access
			// token, the echo, and a record whose identifier repeats the value
			// it was sent.
			items := []string{fmt.Sprintf(`{%s,"accessToken":%q,"echo":%s}`, accountRecord, bbGzipResp, echoObject(append(sent, bbGzipResp)))}
			if quiet {
				items = []string{fmt.Sprintf(`{%s,"accessToken":%q,"echo":%s}`, accountRecord, bbGzipResp, echo)}
			}
			if echoID != "" {
				items = append(items, echoID)
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprint(zw, array(items...))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case get && last == "host-pool":
			fmt.Fprint(w, list(poolRecord))
		case get && last == "hosts":
			fmt.Fprint(w, list(hostRecord))
		case get && last == "autoscale-configuration":
			fmt.Fprintf(w, `{"isEnabled":true,"scaleInRestriction":{"enable":false},"echo":%s}`, echo)
		case get && last == "sessions":
			// AVD user sessions: session data under session names, and one
			// credential (a session token) beside it.
			fmt.Fprintf(w, `[{"sessionId":"avd-session-4f2a","sessionState":"Active","sessionHostName":"HQ-Host-0.contoso.local","userPrincipalName":"hq@contoso.example","sessionToken":%q,"echo":%s}]`, bbSessionTok, echo)
		case get && p == "/rest-api/v1/scripted-actions":
			fmt.Fprint(w, list(scriptRecord))
		case get && p == "/rest-api/v1/schedules":
			fmt.Fprint(w, list(scheduleRecord))
		case get && (p == "/rest-api/v1/usages" || p == "/rest-api/v1/invoices"):
			fmt.Fprintf(w, `[{"accountId":101,"total":120.5,"paid":false,"echo":%s}]`, echo)
		case get && len(seg) == 3 && seg[1] == "job":
			fmt.Fprintf(w, `{"id":%s,"status":"Completed","echo":%s}`, seg[2], echo)
		case r.Method == http.MethodPost && last == "execution":
			// An asynchronous submit: NMM answers a job id. The request id
			// repeats the first value the request carried (the bearer token),
			// as a record id does above.
			req := "req-1042"
			if len(sent) > 0 {
				req = sent[0]
			}
			fmt.Fprintf(w, `{"job":{"id":1042,"status":"Pending"},"requestId":%q,"echo":%s}`, req, echo)
		case !get:
			// Every write echoes what it was sent.
			fmt.Fprintf(w, `{%s,"job":{"id":1043,"status":"Pending"},"echo":%s}`, poolRecord, echo)
		default:
			fmt.Fprint(w, array(fmt.Sprintf(`{"id":"rec-1","name":"HQ-Other","echo":%s}`, echo)))
		}
	}))
	sv.url = srv.URL
	sv.tokenURL = srv.URL + bbTokenPath
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// quietServer is echoServer without the credential echoes: a real NMM
// instance, whose records must reach every sink the echoing server's are
// withheld from.
func quietServer(t *testing.T) *server {
	t.Helper()
	sv := echoServer(t)
	sv.quiet = true
	return sv
}

// token emulates the Entra tenant's token endpoint: POST
// <tenant host>/<TENANT_ID>/oauth2/v2.0/token, a form body
// grant_type=client_credentials&scope=<NERDIO_OAUTH_SCOPE> with the client
// pair as client_id and client_secret (never HTTP Basic, never a bearer token),
// answered with {"access_token","expires_in","token_type"}. It answers only
// the configured pair and scope, and never echoes a request on success. A
// refusal repeats what it was sent, and the client ID and secret it read
// from the form, under names that are not credentials (a real token
// endpoint may name the client it refused).
func (sv *server) token(w http.ResponseWriter, r *http.Request, body []byte, otherURL string) {
	sv.mu.Lock()
	sv.mints++
	redirect := sv.redirectToken
	sv.mu.Unlock()
	if redirect {
		http.Redirect(w, r, otherURL+r.URL.Path, http.StatusTemporaryRedirect)
		return
	}
	form, ferr := url.ParseQuery(string(body))
	id, secret := form.Get("client_id"), form.Get("client_secret")
	why := ""
	switch {
	case r.Method != http.MethodPost:
		why = "method " + r.Method
	case !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		why = "content type " + r.Header.Get("Content-Type")
	case ferr != nil:
		why = "body is not a form"
	case r.Header.Get("Authorization") != "":
		why = "an Authorization header on the token request"
	case form.Get("grant_type") != "client_credentials":
		why = "grant_type " + form.Get("grant_type")
	case form.Get("scope") != bbScope:
		why = "scope " + form.Get("scope")
	case id != bbClientID:
		why = "unknown client"
	case secret != bbClientSecret:
		why = "wrong client secret"
	}
	sv.mu.Lock()
	sv.scopes = append(sv.scopes, form.Get("scope"))
	if why != "" {
		sv.badMint = append(sv.badMint, why)
	} else {
		sv.minted++
	}
	sv.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if why != "" {
		posted := sentValues(r, body)
		for _, v := range []string{id, secret} {
			if v != "" {
				posted = append(posted, v)
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"invalid_client","error_description":"AADSTS7000215: rejected %s","echo":%s}`, strings.Join(posted, " "), echoObject(posted))
		return
	}
	fmt.Fprintf(w, `{"access_token":%q,"expires_in":3600,"token_type":"Bearer"}`, bbMintAccess)
}

// sawHeader reports how many API requests carried Authorization: v, and the
// distinct header values the fake received.
func (sv *server) sawHeader(v string) (int, []string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var all []string
	for h := range sv.headers {
		all = append(all, h)
	}
	return sv.headers[v], all
}

// counts reports the API requests and the token endpoint requests so far.
func (sv *server) counts() (methods []string, mints, minted int, bad []string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return append([]string(nil), sv.methods...), sv.mints, sv.minted, append([]string(nil), sv.badMint...)
}

// tokenHostOther reports the requests that carried the client pair (HTTP
// Basic) anywhere but the token path (the guard lets none through).
func (sv *server) tokenHostOther() []string {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return append([]string(nil), sv.tokenOff...)
}

func (sv *server) reset() {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.methods, sv.headers, sv.mints, sv.minted, sv.badMint, sv.offHost, sv.denied = nil, map[string]int{}, 0, 0, nil, 0, 0
	sv.scopes, sv.tokenOff = nil, nil
}

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its Authorization. Nerdio has no
// static-token variable: the environment can carry only the client pair.
type credMode int

const (
	credStored         credMode = iota // none in the environment: the battery runs on a token stored with auth set-token (a token with no client)
	credPairOverStored                 // a token stored with auth set-token, and a client pair in the environment: the pair mints its own token in every process and keeps it in memory; the stored token is held, never sent, never replaced
	credPair                           // the client pair, NERDIO_NO_CONFIG_WRITE=1: every process mints its own token, nothing is cached
	credPairCached                     // the client pair stored in the credential store, and the token an earlier run minted for it cached beside it
)

func (m credMode) String() string {
	return [...]string{"stored token", "env pair over a stored token", "client pair minted", "client pair cached token"}[m]
}

// header is the Authorization value every API request must carry in mode m.
func (m credMode) header() string {
	if m == credStored {
		return "Bearer " + bbStored
	}
	return "Bearer " + bbMintAccess
}

// held is a credential the process holds from its start in mode m (the one a
// test types on the command line or into a URL).
func (m credMode) held() string {
	switch m {
	case credStored:
		return bbStored
	case credPairCached:
		return bbMintAccess // loaded from the credential store
	default:
		return bbClientSecret
	}
}

// mints reports whether every process of mode m mints its own token.
func (m credMode) mints() bool { return m == credPair || m == credPairOverStored }

// newEnv builds a clean environment against the fakes with the credentials
// mode names: NERDIO_BASE_URL is the fake NMM instance, NERDIO_TOKEN_URL the
// fake Entra tenant, NERDIO_OAUTH_SCOPE the NMM application's scope. Every
// user directory (NERDIO_HOME, HOME, the XDG and Windows dirs, TMPDIR) is
// inside a fresh t.TempDir. credStored and credPairOverStored first store
// bbStored with auth set-token; credPairCached first runs one read so the
// stored client pair mints a token and caches it in the credential store. The
// fake's counters are cleared after either.
func newEnv(t *testing.T, sv *server, mode credMode) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "NERDIO_HOME=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"NERDIO_BASE_URL=" + sv.url, "NERDIO_TOKEN_URL=" + sv.tokenURL, "NERDIO_OAUTH_SCOPE=" + bbScope,
		"NERDIO_CA_CERT=" + sv.caFile, "NERDIO_CLI_PATH=" + bbCLI, "NERDIO_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	e := &bbEnv{home: home, env: env}
	pair := []string{"NERDIO_CLIENT_ID=" + bbClientID, "NERDIO_CLIENT_SECRET=" + bbClientSecret}
	switch mode {
	case credStored, credPairOverStored:
		if out := e.runIn(t, bbStored+"\n", "auth", "set-token"); !e.credentialStoreHolds(bbStored) {
			t.Fatalf("auth set-token stored no token:\n%.1500s", out)
		}
		if mode == credPairOverStored {
			e.env = append(e.env, pair...)
		}
	case credPair:
		e.env = append(e.env, append(pair, "NERDIO_NO_CONFIG_WRITE=1")...)
	case credPairCached:
		// A token is cached only beside the pair it was minted for: the pair
		// lives in the credential store (a token minted for an environment
		// pair is kept in its process, minted-token-bound-to-its-client).
		store := filepath.Join(home, "data", "credentials.toml")
		if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
			t.Fatal(err)
		}
		stored := "base_url = " + strconv.Quote(sv.url) + "\nclient_id = " + strconv.Quote(bbClientID) + "\nclient_secret = " + strconv.Quote(bbClientSecret) + "\n"
		if err := os.WriteFile(store, []byte(stored), 0o600); err != nil {
			t.Fatal(err)
		}
		out := e.run(t, "accounts", "--json")
		if _, mints, minted, bad := sv.counts(); mints != 1 || minted != 1 {
			t.Fatalf("priming the cached token: want one accepted mint, got %d requests, %d accepted (refused: %v):\n%.1500s", mints, minted, bad, out)
		}
		if !e.credentialStoreHolds(bbMintAccess) {
			t.Fatalf("priming the cached token: the minted token was not cached in the credential store:\n%.1500s", out)
		}
	}
	sv.reset()
	return e
}

// credentialStoreHolds reports whether a credential store file under the
// home holds v.
func (e *bbEnv) credentialStoreHolds(v string) bool {
	var found bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "credentials") {
			if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte(v)) {
				found = true
			}
		}
		return nil
	})
	return found
}

func (e *bbEnv) run(t *testing.T, args ...string) string {
	t.Helper()
	return e.runIn(t, "", args...)
}

func (e *bbEnv) runIn(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	out, errb := e.runSplit(t, stdin, args...)
	return out + "\n" + errb
}

// runSplit runs the CLI and returns its stdout and stderr apart.
func (e *bbEnv) runSplit(t *testing.T, stdin string, args ...string) (string, string) {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env = e.env
	cmd.Dir = e.home
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%v: timed out", args)
	}
	return out.String(), errb.String()
}

// mcpCalls runs one MCP stdio session and returns everything the server
// wrote: every tools/call result and its stderr.
func (e *bbEnv) mcpCalls(t *testing.T, calls []map[string]any) string {
	t.Helper()
	return e.mcpPhases(t, calls)
}

// mcpPhases runs one MCP stdio session, sending each phase of tools/call
// requests only after every earlier call was answered (mcp-go serves the calls
// of one phase concurrently), and returns everything the server wrote. The
// calls are numbered from 1 in the order given.
func (e *bbEnv) mcpPhases(t *testing.T, phases ...[]map[string]any) string {
	t.Helper()
	cmd := exec.Command(bbMCP)
	cmd.Env = e.env
	cmd.Dir = e.home
	stdin, _ := cmd.StdinPipe()
	var out, errb lockedBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(stdin)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	id := 0
	deadline := time.Now().Add(120 * time.Second)
	for _, calls := range phases {
		for _, c := range calls {
			id++
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		}
		for time.Now().Before(deadline) && strings.Count(out.String(), `"id":`) < id+1 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	_ = stdin.Close()
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
	}
	return out.String() + "\n" + errb.String()
}

// mcpReplies maps each JSON-RPC reply an MCP session wrote to its request id.
func mcpReplies(out string) map[int]string {
	replies := map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		var msg struct {
			ID *int `json:"id"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && msg.ID != nil {
			replies[*msg.ID] = line
		}
	}
	return replies
}

// lockedBuffer is a bytes.Buffer safe to poll while a process writes to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// holdsForm reports whether text holds f as written or as a JSON encoder
// spells it inside a string (an ASCII85 form carries quotes, backslashes and
// HTML characters a JSON writer escapes).
func holdsForm(text, f string) bool {
	if strings.Contains(text, f) {
		return true
	}
	j, _ := json.Marshal(f)
	inner := string(j[1 : len(j)-1])
	return inner != f && strings.Contains(text, inner)
}

// assertNoEcho fails when text holds a secret in any form the server sent,
// or both halves of one (a split the reader could rejoin).
func assertNoEcho(t *testing.T, where, text string) {
	t.Helper()
	for _, s := range secrets() {
		for _, f := range echoForms(s) {
			if holdsForm(text, f) {
				t.Errorf("%s holds a credential as %q", where, f)
			}
		}
		if a, b := halves(s); strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
	}
}

// importFile writes an import input: one resource-group link whose name
// repeats tok, so the create request carries it and the fake echoes it. It
// lives outside the home: it is the test's input, not a file the program
// wrote.
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "resource-groups.jsonl")
	if err := os.WriteFile(p, []byte(`{"resourceGroup":"Imported `+tok+`","subscriptionId":"`+bbSub+`"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// syncResources are the default sync partitions the fake answers (all but
// cost-estimator, which it refuses with a 401).
const syncResources = "accounts,autoscale-profiles,directories,environment-variables,resource-groups,schedules,scripted-actions"

// battery runs every output path; tok is a credential the process holds
// (credMode.held).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) [][]string {
	groups := e.importFile(t, tok)
	list := []string{"accounts"}
	pool := []string{bbAccount, bbSub, bbRG, bbPool}
	with := func(base []string, more ...string) []string { return append(append([]string{}, base...), more...) }
	return [][]string{
		// Every output mode.
		with(list, "--json"),
		with(list),
		with(list, "--csv"),
		with(list, "--plain"),
		with(list, "--json", "--receipt"),
		with(list, "--json", "--audit-dir", filepath.Join(e.home, "audit")),
		with(list, "--json", "--deliver", "file:"+deliverFile),
		// Every response shape: BOM-prefixed, short credential, split echo,
		// 401 echo, refused redirect, an id that repeats a held value.
		{"users", "get", bbAccount, bbUser, "--json"},
		{"reservations", "get", bbAccount, bbReservation, "--json"},
		{"devices", "get", bbAccount, bbDevice, "--json"},
		{"app-roles", "assignments", "--json"},
		{"workspaces", "list", bbAccount, "--json"},
		{"scripted-actions", "list", "--json"},
		with([]string{"host-pools", "list", bbAccount}, "--json"),
		with(append([]string{"hosts", "list"}, pool...), "--json"),
		with(append([]string{"host-pools", "sessions"}, pool...), "--json"),
		// Writes the fake echoes, a rejected write, an asynchronous submit
		// whose request id repeats the bearer token, a credential request
		// field (link-tenant's ARM and Graph access tokens), and import.
		{"host-pools", "create", bbAccount, "--name", "HQ", "--user-experience", "PooledMultiUserDesktop", "--json"},
		{"reservations", "update", bbAccount, bbReservation, "--order-name", "HQ " + tok, "--json"},
		{"reservations", "update", bbAccount, bbDenied, "--order-name", "HQ", "--json"},
		{"scripted-actions", "run-account", bbAccount, "7", "--minutes-to-wait", "5", "--json"},
		{"provisioning", "link-tenant", "--subscription-id", bbSub, "--azure-access-token", bbArgvSec, "--graph-access-token", bbArgvSec + "-graph", "--company-name", "HQ", "--json"},
		{"import", "resource-groups", "--input", groups, "--json"},
		// The local store, exports, the novel fan-outs and local reads.
		{"sync", "--resources", syncResources, "--json"},
		{"export", "accounts", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"export", "scripted-actions", "--format", "json"},
		{"accounts", "--data-source", "local", "--json"},
		{"fleet", "host-estate", "--json"},
		{"fleet", "autoscale-audit", "--json"},
		{"fleet", "billing-rollup", "--period", "2026-05-01:2026-05-31", "--json"},
		{"usages", "drift", "--from", "2026-04-01:2026-04-30", "--to", "2026-05-01:2026-05-31", "--json"},
		{"job", "wait", "1042", "--interval", "1s", "--timeout", "30s", "--json"},
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
		// A credential on the command line: a credential-named flag, an unknown
		// flag, a positional, feedback text.
		with(list, "--client-secret", bbArgvSec),
		with(list, "--json", "--"+tok),
		{"devices", "get", bbAccount, tok, "--json"},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Previews: a dry run sends nothing (auth login neither mints nor saves).
		{"reservations", "update", bbAccount, bbReservation, "--order-name", "Echo " + tok, "--dry-run"},
		{"host-pools", "create", bbAccount, "--name", "Echo " + tok, "--user-experience", "PooledMultiUserDesktop", "--dry-run"},
		{"auth", "login", "--client-id", bbClientID, "--client-secret", bbClientSecret, "--dry-run"},
		{"import", "resource-groups", "--input", groups, "--dry-run"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find accounts " + tok, "--resource", bbAccount, "--resource-type", "accounts",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		{"doctor"},
		{"auth", "status"},
	}
}

// mcpReads are read-only tools across every response shape the fake has.
func mcpReads() []map[string]any {
	read := func(id string, params map[string]any) map[string]any {
		args := map[string]any{"endpoint_id": id}
		if params != nil {
			args["params"] = params
		}
		return map[string]any{"name": "nerdio_read", "arguments": args}
	}
	none := map[string]any{}
	return []map[string]any{
		read("accounts.list", nil),
		read("users.get", map[string]any{"account_id": 101, "user_id": bbUser}),
		read("reservations.get", map[string]any{"account_id": 101, "reservation_id": bbReservation}),
		read("devices.get", map[string]any{"account_id": 101, "device_id": bbDevice}),
		read("scripted-actions.list", nil),
		read("app-roles.assignments", nil),
		{"name": "nerdio_search", "arguments": map[string]any{"query": "accounts"}},
		{"name": "nerdio_get", "arguments": map[string]any{"endpoint_id": "accounts.list"}},
		{"name": "fleet_host_estate", "arguments": none},
		{"name": "fleet_autoscale_audit", "arguments": none},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// mcpWrites is every kind of write tool, called without confirm: each must
// answer with a preview and send nothing. nerdio_execute previews as a
// confirmation_required object; the mirrored writes (import, scripted-actions
// fan-run) preview as the command's --dry-run plan followed by the gate's
// "Preview only: nothing was sent" note.
func mcpWrites() []map[string]any {
	exec := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "nerdio_execute", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		exec("host-pools.create", map[string]any{"account_id": 101, "name": "Preview Pool", "wvdPoolUserExperience": "PooledMultiUserDesktop"}),
		exec("reservations.update", map[string]any{"account_id": 101, "reservation_id": bbReservation, "orderName": "Preview"}),
		exec("reservations.delete", map[string]any{"account_id": 101, "reservation_id": bbReservation}),
		exec("hosts.restart", map[string]any{"account_id": 101, "subscription_id": bbSub, "resource_group": bbRG, "pool_name": bbPool, "host_name": "HQ-Host-0"}),
		exec("scripted-actions.run-account", map[string]any{"account_id": 101, "script_id": 7, "minutesToWait": 5}),
		{"name": "scripted_actions_fan_run", "arguments": map[string]any{"script_id": "7", "accounts": bbAccount}},
		{"name": "import", "arguments": map[string]any{"resource": "resource-groups"}},
	}
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
}

// checkMode asserts the fakes saw the credential mode the test set up: every
// API request carried the mode's header and no other, the token host took
// nothing but the token POST, and the token endpoint was used exactly as the
// mode says (in every process for an environment pair the files do not hold,
// with no refusal and the configured scope; never for a stored token; for a
// cached token only to replace it after the API answered 401, as the client
// does when a token may have been revoked).
func checkMode(t *testing.T, sv *server, mode credMode) {
	t.Helper()
	n, all := sv.sawHeader(mode.header())
	if n == 0 || len(all) != 1 {
		t.Errorf("%s: want every API request to carry %q, got %d of it and %d distinct values", mode, mode.header(), n, len(all))
	}
	if off := sv.tokenHostOther(); len(off) != 0 {
		t.Errorf("%s: the token host received requests that were not the token POST: %v", mode, off)
	}
	_, mints, minted, bad := sv.counts()
	sv.mu.Lock()
	denied := sv.denied
	scopes := append([]string(nil), sv.scopes...)
	sv.mu.Unlock()
	for _, s := range scopes {
		if s != bbScope {
			t.Errorf("%s: a token request asked for scope %q, want NERDIO_OAUTH_SCOPE %q", mode, s, bbScope)
		}
	}
	switch {
	case mode.mints():
		if minted == 0 || len(bad) != 0 {
			t.Errorf("%s: want the pair to mint and no refused mint, got %d accepted, refused %v", mode, minted, bad)
		}
	case mode == credPairCached:
		if mints > denied || len(bad) != 0 {
			t.Errorf("%s: want a mint only after a 401 (%d of them), got %d token requests (refused %v)", mode, denied, mints, bad)
		}
	case mints != 0:
		t.Errorf("%s: want no mint (the token is stored), got %d token requests (refused %v)", mode, mints, bad)
	}
}

// Class credential-persisted-in-files, in every credential mode.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel() // the batteries overlap: every mode has its own fake and home
	for _, mode := range []credMode{credStored, credPairOverStored, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, args := range battery(t, e, deliverFile, mode.held()) {
				e.run(t, args...)
			}
			e.mcpCalls(t, mcpBattery())
			checkMode(t, sv, mode)
			if _, _, minted, _ := sv.counts(); mode.mints() && minted < 5 {
				// The environment pair's token lives in its process: no
				// process can reuse another's token from disk.
				t.Errorf("%s: only %d mints across the battery; a token was reused from disk", mode, minted)
			}
			// What the credential store may keep: the token it was given
			// (set-token, or the mint for the pair it holds) and that pair.
			keeps := map[credMode][]string{credStored: {bbStored}, credPairOverStored: {bbStored},
				credPairCached: {bbClientID, bbClientSecret, bbMintAccess}}[mode]
			var scanned int
			var sawCreds bool
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				data, rerr := os.ReadFile(path)
				if rerr != nil {
					return nil
				}
				scanned++
				rel, _ := filepath.Rel(e.home, path)
				if strings.HasPrefix(d.Name(), "credentials") {
					// The credential store: the one file allowed to hold a
					// credential, never an environment pair, the token minted
					// for one, or one a response or the command line carried.
					for _, k := range keeps {
						sawCreds = sawCreds || (bytes.Contains(data, []byte(k)) && (k == bbStored || k == bbMintAccess))
					}
					if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
						t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
					}
				secret:
					for _, s := range secrets() {
						for _, k := range keeps {
							if s == k {
								continue secret
							}
						}
						if bytes.Contains(data, []byte(s)) {
							t.Errorf("the credential store %s holds %q, which it was never given to keep", rel, s)
						}
					}
					return nil
				}
				if !synccpuStoreFile(path) { // synccpu: the local store is not checked
					assertNoEcho(t, "file "+rel, string(data))
				}
				return nil
			})
			if scanned < 5 {
				t.Fatalf("only %d files written; the battery did not run", scanned)
			}
			if len(keeps) > 0 && !sawCreds {
				t.Errorf("no credential store holds the %s", mode)
			}
			if len(keeps) == 0 && e.credentialStoreHolds(bbMintAccess) {
				t.Errorf("%s: a credential store holds the minted token", mode)
			}
		})
	}
}

// The control for TestNoCredentialInAnyWrittenFile: an NMM instance that
// repeats no credential has its records reach every file sink (a delivered
// file, an export, the local store, the learn tables, the feedback ledger),
// and a credential-named member is masked there, so a write withheld under
// the echoing fake is the credential and not a broken sink.
func TestOrdinaryRecordsReachEveryFile(t *testing.T) {
	t.Parallel() // the batteries overlap: every mode has its own fake and home
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := quietServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, args := range [][]string{
				{"accounts", "--json", "--deliver", "file:" + deliverFile},
				{"sync", "--resources", syncResources, "--json"},
				{"export", "accounts", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
				{"teach", "--query", "find accounts", "--resource", bbAccount, "--resource-type", "accounts", "--notes", "rotated", "--no-validate"},
				{"feedback", "an ordinary note"},
			} {
				if out := e.run(t, args...); strings.Contains(out, withheldNotice) {
					t.Errorf("%v withheld an ordinary record:\n%.1500s", args, out)
				}
			}
			if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) || !bytes.Contains(b, []byte(fieldMask)) {
				t.Errorf("%s: want the delivered record, its accessToken masked (err %v):\n%.1500s", filepath.Base(deliverFile), err, b)
			}
			if b, err := os.ReadFile(filepath.Join(e.home, "export.jsonl")); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
				t.Errorf("export.jsonl: want the exported record (err %v):\n%.1500s", err, b)
			}
			var sawAccount, sawScript, sawLearning, sawFeedback bool
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					return nil
				}
				if !strings.HasPrefix(d.Name(), "credentials") {
					assertNoEcho(t, "file "+d.Name(), string(b))
				}
				if strings.Contains(d.Name(), ".db") {
					sawAccount = sawAccount || bytes.Contains(b, []byte("HQ-Record"))
					// sync read the scripted actions as well as the accounts.
					sawScript = sawScript || bytes.Contains(b, []byte("HQ-Script"))
					sawLearning = sawLearning || bytes.Contains(b, []byte("rotated"))
				}
				sawFeedback = sawFeedback || bytes.Contains(b, []byte("an ordinary note"))
				return nil
			})
			if !sawAccount || !sawScript {
				t.Errorf("no local store holds the fetched records (account %v, scripted action %v)", sawAccount, sawScript)
			}
			if !sawLearning {
				t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
			}
			if !sawFeedback {
				t.Errorf("no file holds the feedback note")
			}
		})
	}
}

// Class secret-in-output, with an environment pair over a stored token (the
// stored token held and unused), with the client pair minting in every
// process, and with the client pair on a cached minted token.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel() // the batteries overlap: every mode has its own fake and home
	for _, mode := range []credMode{credPairOverStored, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			outputs := map[string]string{}
			for _, args := range battery(t, e, deliverFile, mode.held()) {
				out := e.run(t, args...)
				outputs[strings.Join(args, " ")] = out
				assertNoEcho(t, fmt.Sprintf("output of %v", args), out)
			}
			// The unit that repeats a credential is withheld whole, with the kit's notice.
			if out := outputs["accounts --json"]; !strings.Contains(out, withheldNotice) {
				t.Errorf("accounts --json repeats a credential and printed no notice:\n%.1500s", out)
			}
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			// The fake echoed the header it really received, so the scans above
			// covered it; no other credential source was sent.
			checkMode(t, sv, mode)
		})
	}
}

// The control for TestNoCredentialInAnyOutput: an NMM instance that repeats no
// credential has its records printed, on the CLI and through the MCP server,
// JSON output stays JSON with a credential-named member masked in place, and
// the local reads and fan-outs print what sync stored.
func TestOrdinaryRecordsStillPrint(t *testing.T) {
	t.Parallel() // the batteries overlap: every mode has its own fake and home
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := quietServer(t)
			e := newEnv(t, sv, mode)
			cmd := exec.Command(bbCLI, "accounts", "--json")
			cmd.Env, cmd.Dir = e.env, e.home
			stdout, err := cmd.Output()
			if err != nil {
				t.Fatalf("accounts --json: %v\n%.1500s", err, stdout)
			}
			var doc any
			if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) || !bytes.Contains(stdout, []byte(fieldMask)) {
				t.Fatalf("want valid JSON with the record and its accessToken masked, got:\n%.1500s", stdout)
			}
			for _, c := range []struct {
				args []string
				want string
			}{
				{[]string{"users", "get", bbAccount, bbUser, "--json"}, "HQ Admin"},
				{[]string{"reservations", "get", bbAccount, bbReservation, "--json"}, "HQ-Reservation"},
				{[]string{"sync", "--resources", syncResources, "--json"}, "sync_summary"},
				{[]string{"accounts", "--data-source", "local", "--json"}, "HQ-Record"},
				{[]string{"fleet", "host-estate", "--json"}, "HQ-Host-0"},
			} {
				out := e.run(t, c.args...)
				if strings.Contains(out, withheldNotice) || !strings.Contains(out, c.want) {
					t.Errorf("%v lost its record (want %q):\n%.1500s", c.args, c.want, out)
				}
				assertNoEcho(t, fmt.Sprint(c.args), out)
			}
			out := e.mcpCalls(t, mcpReads())
			if strings.Contains(out, withheldNotice) || strings.Count(out, "HQ-Record") < 2 {
				t.Fatalf("MCP results lost the record:\n%.3000s", out)
			}
			assertNoEcho(t, "MCP session", out)
		})
	}
}

// The oracle is not vacuous: the fake echoes a value that is not a credential
// (bbControl, sent as a device id) in every shape it echoes the credentials it
// is sent, and the output prints it, so the echo shapes teach the holder
// nothing. (The quiet fake echoes only the control: a unit that also repeats a
// credential is withheld whole.)
func TestEchoTeachesTheHolderNothing(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := quietServer(t)
			e := newEnv(t, sv, mode)
			out := e.run(t, "devices", "apps", bbAccount, bbControl, "--json")
			out += e.run(t, "devices", "get", bbAccount, bbControl, "--json")
			out += e.mcpCalls(t, []map[string]any{{"name": "nerdio_read", "arguments": map[string]any{
				"endpoint_id": "devices.apps", "params": map[string]any{"account_id": 101, "device_id": bbControl}}}})
			for _, f := range echoForms(bbControl) {
				if !holdsForm(out, f) {
					t.Errorf("the echo of the control value as %q was withheld or never sent: the fake teaches the holder, so the credential scans are vacuous:\n%.2000s", f, out)
				}
			}
			if a, b := halves(bbControl); !strings.Contains(out, a+" "+b) {
				t.Errorf("the split echo of the control value was withheld:\n%.2000s", out)
			}
			assertNoEcho(t, "output", out)
			checkMode(t, sv, mode)
		})
	}
}

// Class credential-sent-to-another-host: a redirect off the configured
// NMM instance is refused before it is followed, from the CLI, sync and MCP
// alike, and in the client-credentials mode a redirected token endpoint never
// sends the client credentials to another host either.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	t.Run("API requests", func(t *testing.T) {
		for _, mode := range []credMode{credStored, credPair} {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			redirect := e.run(t, "workspaces", "list", bbAccount, "--json")
			sv.mu.Lock()
			sv.redirectAccounts = true
			sv.mu.Unlock()
			syncOut := e.run(t, "sync", "--resources", "accounts", "--json")
			e.mcpCalls(t, []map[string]any{
				{"name": "nerdio_read", "arguments": map[string]any{"endpoint_id": "workspaces.list", "params": map[string]any{"account_id": 101}}},
				{"name": "nerdio_read", "arguments": map[string]any{"endpoint_id": "accounts.list"}},
			})
			sv.mu.Lock()
			n := sv.offHost
			var redirected int
			for _, m := range sv.methods {
				if strings.HasSuffix(m, "/accounts/"+bbAccount+"/workspace") || strings.HasSuffix(m, " /rest-api/v1/accounts") {
					redirected++
				}
			}
			sv.mu.Unlock()
			if n != 0 {
				t.Fatalf("%s: the other host was contacted %d time(s)", mode, n)
			}
			if redirected < 4 {
				t.Fatalf("%s: only %d request(s) met a redirect; the CLI, sync and MCP calls did not all run", mode, redirected)
			}
			for where, out := range map[string]string{"workspaces list": redirect, "sync": syncOut} {
				if !strings.Contains(out, "leave the configured Nerdio host") {
					t.Fatalf("%s: %s: want the pin refusal, got:\n%.1500s", mode, where, out)
				}
				assertNoEcho(t, where, out)
			}
		}
	})
	t.Run("token endpoint", func(t *testing.T) {
		sv := echoServer(t)
		sv.redirectToken = true
		e := newEnv(t, sv, credPair)
		out := e.run(t, "accounts", "--json")
		out += e.mcpCalls(t, []map[string]any{{"name": "nerdio_read", "arguments": map[string]any{"endpoint_id": "accounts.list"}}})
		methods, mints, _, _ := sv.counts()
		if mints < 2 {
			t.Fatalf("the token endpoint saw %d request(s); the CLI and MCP mints did not both run:\n%.1500s", mints, out)
		}
		sv.mu.Lock()
		n := sv.offHost
		sv.mu.Unlock()
		if n != 0 {
			t.Fatalf("the redirected token endpoint reached the other host %d time(s)", n)
		}
		if len(methods) != 0 {
			t.Fatalf("API requests went out without a minted token: %v", methods)
		}
		// The token request follows no redirect (Codex r1 P2
		// token-redirect-deadlock): the redirect is refused as the answer.
		if !strings.Contains(out, "leave the configured Nerdio host") && !strings.Contains(out, "refused cross-origin redirect") && !strings.Contains(out, "HTTP 307") {
			t.Fatalf("want the redirect refused, got:\n%.1500s", out)
		}
		assertNoEcho(t, "output", out)
	})
}

// A rejected mint (a wrong secret, the token endpoint's refusal echoing what
// it was sent and the client it decoded) returns promptly and prints nothing
// it was sent, from the CLI, from sync and from MCP.
func TestRejectedMintPrintsNoSecret(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credPair)
	const wrong = "wrongClientSecret-6d2a9f1c4e"
	e.env = append(withoutEnv(e.env, "NERDIO_CLIENT_SECRET"), "NERDIO_CLIENT_SECRET="+wrong)
	out := e.run(t, "accounts", "--json")
	out += e.run(t, "sync", "--resources", "accounts", "--json")
	out += e.mcpCalls(t, []map[string]any{{"name": "nerdio_read", "arguments": map[string]any{"endpoint_id": "accounts.list"}}})
	_, mints, minted, bad := sv.counts()
	if mints == 0 || minted != 0 || len(bad) == 0 {
		t.Fatalf("want only refused mints, got %d requests, %d accepted (refused %v)", mints, minted, bad)
	}
	for _, why := range bad {
		if why != "wrong client secret" {
			t.Fatalf("a mint was refused for %q, not the wrong secret: the request was malformed", why)
		}
	}
	if !strings.Contains(out, "401") && !strings.Contains(strings.ToLower(out), "invalid_client") && !strings.Contains(strings.ToLower(out), "unauthorized") {
		t.Errorf("the refusal was not reported:\n%.2000s", out)
	}
	for _, f := range append(echoForms(wrong), echoForms(bbClientID)...) {
		if strings.Contains(out, f) {
			t.Errorf("output repeats the posted credential as %q:\n%.2000s", f, out)
		}
	}
	assertNoEcho(t, "output", out)
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a GET while no
// call sets confirm, though the battery calls every kind of write tool
// (nerdio_execute, the mirrored scripted-actions fan-run, import); each write
// answers with a preview. In the minted mode the token POST goes to the token
// host, and it is not a write.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, "HQ-Record") {
				t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
			}
			replies := mcpReplies(out)
			for i, call := range mcpWrites() {
				id := len(mcpReads()) + i + 1
				if r := replies[id]; !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") {
					t.Errorf("%v without confirm did not return a preview:\n%.1500s", call, r)
				}
			}
			assertNoEcho(t, "MCP session", out)
			methods, _, _, _ := sv.counts()
			if len(methods) == 0 {
				t.Fatal("no request reached the fake")
			}
			for _, m := range methods {
				if !strings.HasPrefix(m, "GET ") {
					t.Errorf("the MCP server sent %s without confirm", m)
				}
			}
			checkMode(t, sv, mode)
		})
	}
}

// Control for the test above: with confirm set to boolean true the same
// writes reach the API (nerdio_execute and the mirrored scripted-actions
// fan-run), so the gate previews rather than drops; the string "true" still
// previews and sends nothing at all (in the minted mode, not even a mint).
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			update := func(confirm any) []map[string]any {
				return []map[string]any{
					{"name": "nerdio_execute", "arguments": map[string]any{
						"endpoint_id": "reservations.update", "params": map[string]any{"account_id": 101, "reservation_id": bbReservation, "orderName": "HQ map"}, "confirm": confirm}},
					{"name": "scripted_actions_fan_run", "arguments": map[string]any{"script_id": "7", "accounts": bbAccount, "confirm": confirm}},
				}
			}
			preview := e.mcpPhases(t, update("true"))
			if strings.Count(preview, "confirmation_required")+strings.Count(preview, "Preview only: nothing was sent") < 2 {
				t.Fatalf("confirm \"true\" (a string) returned no preview:\n%.1500s", preview)
			}
			if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
				t.Errorf("confirm \"true\" (a string) sent %v (%d token requests)", methods, mints)
			}
			e.mcpPhases(t, update(true))
			methods, _, _, _ := sv.counts()
			var sentUpdate, sentRun bool
			for _, m := range methods {
				sentUpdate = sentUpdate || m == "PUT /rest-api/v1/accounts/101/reservations/"+bbReservation
				sentRun = sentRun || m == "POST /rest-api/v1/accounts/101/scripted-actions/7/execution"
			}
			if !sentUpdate || !sentRun {
				t.Fatalf("confirm true did not send both writes (update %v, fan-run %v); requests: %v", sentUpdate, sentRun, methods)
			}
			checkMode(t, sv, mode)
		})
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, and never
// mints, from a cold start with a stored token, the client pair, or the
// client pair on a cached token.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			groups := e.importFile(t, "Dry")
			for _, args := range [][]string{
				{"reservations", "update", bbAccount, bbReservation, "--order-name", "Dry", "--dry-run"},
				{"host-pools", "create", bbAccount, "--name", "Dry Pool", "--user-experience", "PooledMultiUserDesktop", "--dry-run"},
				{"reservations", "delete", bbAccount, bbReservation, "--dry-run"},
				{"hosts", "restart", bbAccount, bbSub, bbRG, bbPool, "HQ-Host-0", "--dry-run"},
				{"scripted-actions", "run-account", bbAccount, "7", "--dry-run"},
				{"scripted-actions", "fan-run", "7", "--accounts", bbAccount, "--dry-run"},
				{"auth", "login", "--client-id", bbClientID, "--client-secret", bbClientSecret, "--dry-run"},
				{"import", "resource-groups", "--input", groups, "--dry-run"},
			} {
				// Each run reaches its preview, so the count below is not vacuous
				// (a flag error would send nothing either). auth login refuses
				// outright under NERDIO_NO_CONFIG_WRITE, sending nothing.
				out := e.run(t, args...)
				if !strings.Contains(out, "dry_run") && !strings.Contains(out, "would ") && !strings.Contains(out, "dry run") &&
					!(mode == credPair && args[0] == "auth" && strings.Contains(out, "no credential may be written")) {
					t.Errorf("%v (%s) printed no preview:\n%.1500s", args, mode, out)
				}
				assertNoEcho(t, fmt.Sprintf("preview of %v", args), out)
			}
			if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
				t.Errorf("--dry-run (%s) sent %v and %d token request(s)", mode, methods, mints)
			}
			// Control: the same write without --dry-run reaches the fake.
			e.run(t, "reservations", "update", bbAccount, bbReservation, "--order-name", "Dry")
			if methods, _, _, _ := sv.counts(); len(methods) != 1 || methods[0] != "PUT /rest-api/v1/accounts/101/reservations/"+bbReservation {
				t.Errorf("control (%s): want the one PUT, got %v", mode, methods)
			}
			checkMode(t, sv, mode)
		})
	}
}

// recordingProxy answers every request it receives 403 and records it. Set as
// HTTPS_PROXY and HTTP_PROXY, it receives every request the binary makes to a
// host that is not loopback, so a request aimed at the real Entra ID (the
// placeholder tenant) or any other real host is caught without reaching the
// network.
func recordingProxy(t *testing.T) (proxyURL string, escaped func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.Host+r.URL.String())
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(proxy.Close)
	return proxy.URL, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func withProxy(env []string, proxyURL string) []string {
	for _, k := range []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "http_proxy", "NO_PROXY", "no_proxy"} {
		env = withoutEnv(env, k)
	}
	return append(env, "HTTPS_PROXY="+proxyURL, "HTTP_PROXY="+proxyURL, "https_proxy="+proxyURL, "http_proxy="+proxyURL, "NO_PROXY=", "no_proxy=")
}

// token-url-from-operator-only + api-origin-pinned, on the built binary: auth
// login mints at NERDIO_TOKEN_URL (the Entra tenant, another host than the
// NMM instance) asking for exactly NERDIO_OAUTH_SCOPE, and nowhere else. Every
// non-loopback request the binary makes goes through a recording proxy, which
// answers nothing, so a mint aimed at a real host is caught without reaching
// the network.
func TestAuthLoginMintsOnlyAtItsTokenURL(t *testing.T) {
	t.Parallel()
	proxy, escaped := recordingProxy(t)
	sv := echoServer(t)
	e := newEnv(t, sv, credPair) // the pair in the environment; login must be allowed to save
	e.env = withProxy(withoutEnv(e.env, "NERDIO_NO_CONFIG_WRITE"), proxy)
	out := e.run(t, "auth", "login")
	if left := escaped(); len(left) != 0 {
		t.Fatalf("auth login sent %d request(s) to another host: %v\n%.1500s", len(left), left, out)
	}
	_, mints, minted, bad := sv.counts()
	if mints != 1 || minted != 1 {
		t.Fatalf("auth login: want one accepted mint at the configured token URL, got %d requests, %d accepted (refused %v):\n%.1500s", mints, minted, bad, out)
	}
	if !strings.Contains(out, "Logged in") {
		t.Fatalf("auth login did not report success:\n%.1500s", out)
	}
	if off := sv.tokenHostOther(); len(off) != 0 {
		t.Errorf("auth login sent the token host something other than the token POST: %v", off)
	}
	if methods, _, _, _ := sv.counts(); len(methods) != 0 {
		t.Errorf("auth login sent API requests: %v", methods)
	}
	assertNoEcho(t, "auth login output", out)
	if !e.credentialStoreHolds(bbMintAccess) {
		t.Errorf("auth login stored no token in the credential store")
	}
	for _, v := range []string{bbClientID, bbClientSecret} {
		if e.credentialStoreHolds(v) {
			t.Errorf("auth login persisted the environment client credential %q", v)
		}
	}
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), "credentials") {
			return nil
		}
		if data, rerr := os.ReadFile(path); rerr == nil {
			rel, _ := filepath.Rel(e.home, path)
			assertNoEcho(t, "file "+rel, string(data))
		}
		return nil
	})
	// The token it saved authenticates the next run against the same
	// NMM instance without another mint, and still nothing leaves.
	sv.reset()
	e.env = withoutEnv(withoutEnv(e.env, "NERDIO_CLIENT_ID"), "NERDIO_CLIENT_SECRET")
	// The echoing fake repeats the bearer token, so the record may arrive as
	// the kit's notice; the header check below proves the read was sent.
	if read := e.run(t, "accounts", "--json"); !strings.Contains(read, "HQ-Record") && !strings.Contains(read, withheldNotice) {
		t.Fatalf("the read after login fetched nothing:\n%.1500s", read)
	}
	if _, mints, _, _ := sv.counts(); mints != 0 {
		t.Errorf("the read after login minted %d time(s); the saved token was not used", mints)
	}
	if n, _ := sv.sawHeader("Bearer " + bbMintAccess); n == 0 {
		t.Errorf("the read after login did not carry the token login saved")
	}
	if left := escaped(); len(left) != 0 {
		t.Fatalf("a request left for another host: %v", left)
	}
}

// token-url-from-operator-only and oauth-scope-required, on the built binary:
// with NERDIO_TOKEN_URL unset, still naming the placeholder tenant (a real
// host: login.microsoftonline.com/YOUR_TENANT_ID/...), or NERDIO_OAUTH_SCOPE
// unset, neither auth login nor the automatic mint of an API command sends
// anything to any server (the fake NMM instance, the fake Entra tenant, or a
// real host through the recording proxy), and the error names the variable
// to set.
func TestNoTokenURLOrScopeSendsNothing(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, variable string
		env            func(sv *server) []string
	}{
		{"NERDIO_TOKEN_URL unset", "NERDIO_TOKEN_URL", func(*server) []string { return nil }},
		{"NERDIO_TOKEN_URL the placeholder tenant", "NERDIO_TOKEN_URL", func(*server) []string {
			return []string{"NERDIO_TOKEN_URL=https://login.microsoftonline.com/YOUR_TENANT_ID/oauth2/v2.0/token"}
		}},
		{"NERDIO_OAUTH_SCOPE unset", "NERDIO_OAUTH_SCOPE", func(sv *server) []string { return []string{"NERDIO_TOKEN_URL=" + sv.tokenURL} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			proxy, escaped := recordingProxy(t)
			sv := echoServer(t)
			e := newEnv(t, sv, credPair)
			env := withoutEnv(withoutEnv(withoutEnv(e.env, "NERDIO_NO_CONFIG_WRITE"), "NERDIO_TOKEN_URL"), "NERDIO_OAUTH_SCOPE")
			if c.variable != "NERDIO_OAUTH_SCOPE" {
				env = append(env, "NERDIO_OAUTH_SCOPE="+bbScope)
			}
			e.env = withProxy(append(env, c.env(sv)...), proxy)
			for _, args := range [][]string{{"auth", "login"}, {"accounts", "--json"}} {
				out := e.run(t, args...)
				if !strings.Contains(out, c.variable) {
					t.Errorf("%v: the error does not name %s:\n%.1500s", args, c.variable, out)
				}
				assertNoEcho(t, fmt.Sprintf("output of %v", args), out)
			}
			out := e.mcpCalls(t, []map[string]any{{"name": "nerdio_read", "arguments": map[string]any{"endpoint_id": "accounts.list"}}})
			if !strings.Contains(out, c.variable) {
				t.Errorf("MCP read: the error does not name %s:\n%.1500s", c.variable, out)
			}
			methods, mints, _, _ := sv.counts()
			if left := escaped(); len(methods) != 0 || mints != 0 || len(left) != 0 || len(sv.tokenHostOther()) != 0 {
				t.Errorf("something was sent: API %v, %d token request(s), token host other %v, other hosts %v", methods, mints, sv.tokenHostOther(), left)
			}
			if e.credentialStoreHolds(bbClientSecret) || e.credentialStoreHolds(bbClientID) {
				t.Errorf("a refused login persisted the client pair")
			}
		})
	}
}
