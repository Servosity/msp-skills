// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Package appdirectblackbox runs the built CLI and MCP binaries against an
// AppDirect marketplace fake that echoes every credential it was sent back in
// every encoding it can, whole and split across lines and adjacent values,
// and checks what a reviewer cannot read off the code (appdirect handfixes
// output-chokepoint, store-never-keeps-secrets, no-response-cache,
// no-credential-in-files, response-credentials-held, api-origin-pinned,
// dry-run-sends-nothing, mcp-endpoint-write-confirm, client-credentials-mint,
// minted-token-bound-to-its-client):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the APPDIRECT_CLIENT_ID + APPDIRECT_CLIENT_SECRET
//     pair, the access token the fake's token endpoint mints for that pair
//     (fresh in every process, or cached by an earlier run), the token stored
//     with auth set-token, the MCP HTTP token, a credential-named flag value,
//     and the credentials a response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     learn journal, the jobs ledger, config.toml), and the credential store
//     never holds an environment pair or the token minted for one;
//   - no credential leaves for a host other than the configured marketplace
//     by redirect, the token mint included;
//   - the MCP server sends a write only when the call sets confirm to the
//     boolean true, and --dry-run sends nothing at all (no mint either).
//
// The fake models AppDirect: the API under <marketplace>/api, the token
// endpoint at <marketplace>/oauth2/token on the same origin (POST, a form body
// grant_type=client_credentials&scope=..., the client authenticated with HTTP
// Basic, answered {"access_token","expires_in"}); billing lists are plain JSON
// arrays, account v2 lists are pages (content).
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder. The fake teaches the holder nothing:
// it echoes a credential only under names that do not read as credentials and
// never in a shape the holder learns from (an auth-scheme value such as
// "Bearer <v>" or "Basic <v>" is held under any name), so a value the program
// failed to hold from its real source (the environment, the store, the mint,
// the argv) is printed, and the scan fails. TestEchoTeachesTheHolderNothing
// proves it.
package appdirectblackbox

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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bbClientID     = "clientId-3e9a7c1d5b2f8a46"           // APPDIRECT_CLIENT_ID
	bbClientSecret = "clientSecret-9d2b6f4a1c8e7305ab"     // APPDIRECT_CLIENT_SECRET
	bbMintAccess   = "mintedAccess-5c1e9a3f7b2d4e60c8"     // the access_token the fake's token endpoint mints for the pair
	bbStored       = "storedToken-71c3e9b05d2a4f68e1"      // auth set-token (stdin), the credential store
	bbArgvSec      = "-argvSecret-91c3e7a5"                // --client-secret on the command line (starts with "-")
	bbShortResp    = "Sh0rtPw"                             // a 7-character credential a response carries
	bbBOMResp      = "BomRespSecret-6a1c"                  // a credential in a BOM-prefixed response
	bbGzipResp     = "GzipRespToken-4f2a9c1e"              // a credential in a gzip-encoded response
	bbHTTPToken    = "mcpHttpToken-0123456789abcdefABCDEF" // APPDIRECT_MCP_HTTP_TOKEN
	bbScope        = "ROLE_PARTNER ROLE_PARTNER_READ"      // the default APPDIRECT_OAUTH_SCOPE: sent in the token form, not a credential
	bbCompany      = "co-1042"                             // a company uuid
	bbUser         = "user-5150"                           // a user uuid
	bbSub          = "sub-7731"                            // a subscription id
	bbDenied       = "co-denied"                           // a company uuid the fake answers 401
	bbControl      = "ctlEchoValue-7a1b9c2d3e"             // not a credential: the fake echoes it like one (TestEchoTeachesTheHolderNothing)
)

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
	dir, err := os.MkdirTemp("", "appdirect-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "appdirect-cli"+exe)
	bbMCP = filepath.Join(dir, "appdirect-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/appdirect-cli", bbMCP: "../../cmd/appdirect-mcp"} {
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
	url     string // the API base URL: <marketplace>/api
	caFile  string
	mu      sync.Mutex
	offHost int
	methods []string       // API requests but the token endpoint, "METHOD /path"
	headers map[string]int // Authorization header values those requests carried
	mints   int            // token endpoint requests (POST <marketplace>/oauth2/token)
	minted  int            // ... that the fake answered with the minted token
	badMint []string       // ... the fake refused, and why
	denied  int            // API requests the fake answered 401 (the client may re-mint after one)
	// redirectCompanies sends the companies list (a sync resource) to the
	// other host too, so sync meets a redirect.
	redirectCompanies bool
	// redirectToken sends the token endpoint to the other host (a 307 would
	// resend the request and its Basic client credentials).
	redirectToken bool
}

// tlsServer starts an HTTP/2 TLS fake.
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// Records shaped the way spec.yaml documents them, so the store, the six
// novel local-store views (reconcile, subs changed, payments unpaid,
// pipeline, pipeline stale, company show) and the exports have something
// real to read. Dates are epoch milliseconds.
var (
	recent         = strconv.FormatInt(time.Now().Add(-48*time.Hour).UnixMilli(), 10)
	old            = strconv.FormatInt(time.Now().Add(-90*24*time.Hour).UnixMilli(), 10)
	companyRecord  = `"uuid":"co-1042","name":"HQ-Record","enabled":true,"creationDate":` + old
	userRecord     = `"id":"user-5150","email":"hq@example.com","firstName":"HQ","lastName":"Admin","company":{"id":"co-1042"}`
	subRecord      = `"id":"sub-7731","status":"ACTIVE","creationDate":` + recent + `,"company":{"id":"co-1042","name":"HQ-Record"},"order":{"id":"ord-1042"},"product":{"name":"HQ-Product"}`
	invoiceRecord  = `"id":"inv-1042","invoiceId":"inv-1042","status":"UNPAID","total":120.5,"currency":"USD","dueDate":` + old + `,"creationDate":` + old + `,"company":{"id":"co-1042"},"orderIds":["ord-9"]`
	paymentRecord  = `"id":"pay-1042","result":"FAILED","amount":42,"currency":"USD","method":"CREDIT_CARD","date":` + recent + `,"company":{"id":"co-1042"}`
	opportunityRec = `"id":"opp-1042","name":"HQ-Opportunity","status":"OPEN","createdOn":` + old + `,"ownerUser":{"email":"owner@example.com"},"customerUser":{"email":"buyer@example.com","company":{"id":"co-1042"}}`
)

// page wraps items the way an account v2 list answers: a page of content.
func page(items ...string) string {
	return `{"links":[],"content":[` + strings.Join(items, ",") + `],"page":{"size":50,"totalElements":` + fmt.Sprint(len(items)) + `,"totalPages":1,"number":0}}`
}

// array wraps items the way a billing list answers: a plain JSON array.
func array(items ...string) string { return "[" + strings.Join(items, ",") + "]" }

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{headers: map[string]int{}}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(array()))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/oauth2/token" {
			sv.token(w, r, body, other.URL)
			return
		}
		sent := sentValues(r, body)
		echo := echoObject(sent)
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/api")
		get := r.Method == http.MethodGet
		seg := strings.Split(strings.Trim(p, "/"), "/")
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		if h := r.Header.Get("Authorization"); h != "" {
			sv.headers[h]++
		}
		redirectCompanies := sv.redirectCompanies
		sv.mu.Unlock()
		echoID := ""
		if len(sent) > 0 {
			// A record whose identifier or name repeats the first value it was sent.
			echoID = fmt.Sprintf(`{"id":%q,"uuid":%q,"name":"Echo-ID","status":"ACTIVE","company":{"id":"co-1042"}}`, sent[0], sent[0])
		}
		list := func(wrap func(...string) string, rec string) string {
			items := []string{fmt.Sprintf(`{%s,"echo":%s}`, rec, echo)}
			if echoID != "" {
				items = append(items, echoID)
			}
			return wrap(items...)
		}
		switch {
		case strings.HasPrefix(p, "/lead/") || strings.Contains(p, "/"+bbDenied):
			// Rejected: the error repeats what it was sent.
			sv.mu.Lock()
			sv.denied++
			sv.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"code":"UNAUTHORIZED","message":"bad request %s","detail":"invalid request %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/channel/v1/events" || (redirectCompanies && p == "/account/v2/companies"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && len(seg) == 4 && seg[0] == "account" && seg[2] == "companies":
			// company get: the echo split across values, lines and spaces.
			fmt.Fprintf(w, `{%s,"split":%s}`, companyRecord, splitObject(sent))
		case get && len(seg) == 5 && seg[2] == "companies" && seg[4] == "memberships":
			// company memberships: the echo, and a member whose id repeats
			// the value it was sent.
			fmt.Fprint(w, list(page, userRecord))
		case get && len(seg) == 4 && seg[2] == "users":
			// user get: BOM-prefixed, with a password.
			fmt.Fprint(w, "\xef\xbb\xbf"+fmt.Sprintf(`{%s,"note":%q,"password":%q}`, userRecord, bbBOMResp, bbBOMResp))
		case get && len(seg) == 4 && seg[0] == "billing" && seg[2] == "subscriptions":
			// subscription get: a short credential.
			fmt.Fprintf(w, `{%s,"note":%q,"password":%q}`, subRecord, bbShortResp, bbShortResp)
		case get && p == "/account/v2/companies":
			// companies list, sync, export: gzip-encoded, with an access
			// token, the echo, and a record whose identifier repeats the value
			// it was sent.
			items := []string{fmt.Sprintf(`{%s,"accessToken":%q,"echo":%s}`, companyRecord, bbGzipResp, echoObject(append(sent, bbGzipResp)))}
			if echoID != "" {
				items = append(items, echoID)
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprint(zw, page(items...))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case get && p == "/account/v2/users":
			fmt.Fprint(w, list(page, userRecord))
		case get && p == "/billing/v1/subscriptions":
			fmt.Fprint(w, list(array, subRecord))
		case get && p == "/billing/v1/invoices":
			fmt.Fprint(w, list(array, invoiceRecord))
		case get && p == "/billing/v1/payments":
			fmt.Fprint(w, list(array, paymentRecord))
		case get && p == "/assistedSales/v1/opportunities":
			fmt.Fprint(w, list(page, opportunityRec))
		case r.Method == http.MethodPost && p == "/integration/v2/billing/usage":
			// An asynchronous submit: the job id repeats the first value the
			// request carried (the bearer token), as a record id does above.
			job := "req-1042"
			if len(sent) > 0 {
				job = sent[0]
			}
			fmt.Fprintf(w, `{"requestId":%q,"status":"PENDING","echo":%s}`, job, echo)
		case !get:
			// Every write echoes what it was sent.
			fmt.Fprintf(w, `{%s,"echo":%s}`, companyRecord, echo)
		default:
			fmt.Fprint(w, array(fmt.Sprintf(`{"id":"rec-1","name":"HQ-Other","echo":%s}`, echo)))
		}
	}))
	sv.url = srv.URL + "/api"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// token emulates AppDirect's token endpoint: POST <marketplace>/oauth2/token
// on the API origin, a form body grant_type=client_credentials&scope=<the
// default scope>, the client authenticated with HTTP Basic (never in the body,
// never a bearer token), answered with {"access_token","expires_in",
// "token_type"}. It answers only the configured pair, and never echoes a
// request on success. A refusal repeats what it was sent, and the client ID
// and secret it decoded from the Basic header, under names that are not
// credentials (a real token endpoint may name the client it refused).
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
	id, secret, basic := r.BasicAuth()
	why := ""
	switch {
	case r.Method != http.MethodPost:
		why = "method " + r.Method
	case !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		why = "content type " + r.Header.Get("Content-Type")
	case ferr != nil:
		why = "body is not a form"
	case strings.HasPrefix(r.Header.Get("Authorization"), "Bearer"):
		why = "a bearer token on the token request"
	case !basic:
		why = "no HTTP Basic client authentication"
	case form.Get("grant_type") != "client_credentials":
		why = "grant_type " + form.Get("grant_type")
	case form.Get("scope") != bbScope:
		why = "scope " + form.Get("scope")
	case form.Has("client_id") || form.Has("client_secret"):
		why = "the client pair in the body"
	case id != bbClientID:
		why = "unknown client"
	case secret != bbClientSecret:
		why = "wrong client secret"
	}
	sv.mu.Lock()
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
		fmt.Fprintf(w, `{"error":"invalid_client","error_description":"rejected %s","echo":%s}`, strings.Join(posted, " "), echoObject(posted))
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

func (sv *server) reset() {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.methods, sv.headers, sv.mints, sv.minted, sv.badMint, sv.offHost, sv.denied = nil, map[string]int{}, 0, 0, nil, 0, 0
}

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its Authorization. AppDirect has no
// static-token variable: the environment can carry only the client pair.
type credMode int

const (
	credStored         credMode = iota // none in the environment: the battery runs on a token stored with auth set-token (a token with no client)
	credPairOverStored                 // a token stored with auth set-token, and a client pair in the environment: the pair mints its own token in every process and keeps it in memory; the stored token is held, never sent, never replaced
	credPair                           // the client pair, APPDIRECT_NO_CONFIG_WRITE=1: every process mints its own token, nothing is cached
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

// newEnv builds a clean environment against the fake with the credentials
// mode names. Every user directory (APPDIRECT_HOME, HOME, the XDG and Windows
// dirs, TMPDIR) is inside a fresh t.TempDir. credStored and credPairOverStored
// first store bbStored with auth set-token; credPairCached first runs one read
// so the stored client pair mints a token and caches it in the credential
// store. The fake's counters are cleared after either.
func newEnv(t *testing.T, sv *server, mode credMode) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDIRECT_HOME=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"APPDIRECT_BASE_URL=" + sv.url, "APPDIRECT_CA_CERT=" + sv.caFile,
		"APPDIRECT_CLI_PATH=" + bbCLI, "APPDIRECT_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	e := &bbEnv{home: home, env: env}
	pair := []string{"APPDIRECT_CLIENT_ID=" + bbClientID, "APPDIRECT_CLIENT_SECRET=" + bbClientSecret}
	switch mode {
	case credStored, credPairOverStored:
		if out := e.runIn(t, bbStored+"\n", "auth", "set-token"); !e.credentialStoreHolds(bbStored) {
			t.Fatalf("auth set-token stored no token:\n%.1500s", out)
		}
		if mode == credPairOverStored {
			e.env = append(e.env, pair...)
		}
	case credPair:
		e.env = append(e.env, append(pair, "APPDIRECT_NO_CONFIG_WRITE=1")...)
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
		out := e.run(t, "account", "resource-company-read-all-companies-get", "--json")
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
	return out.String() + "\n" + errb.String()
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

// importFile writes an import input: one company whose name repeats tok, so
// the create request carries it and the fake echoes it. It lives outside the
// home: it is the test's input, not a file the program wrote.
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "companies.jsonl")
	if err := os.WriteFile(p, []byte(`{"name":"Imported `+tok+`","industry":"d"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// novelResources are the sync resources the six novel views read.
const novelResources = "account-v2-companies,account,billing-v1-subscriptions,billing-v1-invoices,billing-v1-payments,assisted-sales-v1-opportunities"

// battery runs every output path; tok is a credential the process holds
// (credMode.held).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) [][]string {
	companies := e.importFile(t, tok)
	list := []string{"account", "resource-company-read-all-companies-get"}
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
		{"account", "resource-user-read-user-get", bbUser, "--json"},
		{"billing", "resource-subscription-read-subscription-get", bbSub, "--json"},
		{"account", "resource-company-read-company-get", bbCompany, "--json"},
		{"lead", "resource-app-reseller-api-get-all-get", "--json"},
		{"channel", "resource-other-read-events-get", "--json"},
		{"billing", "resource-subscription-read-subscriptions-get", "--json"},
		{"account", "resource-company-read-company-memberships-get", bbCompany, "--json"},
		// Writes the fake echoes, a rejected write, an asynchronous submit
		// whose job id repeats the bearer token (the jobs ledger), and import.
		{"account", "resource-company-patch-company-patch", bbCompany, "--name", "HQ", "--json"},
		{"billing", "resource-other-update-subscription-patch", bbSub, "--status", "ACTIVE", "--external-id", "HQ " + tok, "--json"},
		{"account", "resource-company-patch-company-patch", bbDenied, "--name", "HQ", "--json"},
		{"integration", "resource-other-bill-usage-v2-post", "--billable", "--idempotency-key", "usage-1042", "--json"},
		{"import", "account", "--input", companies, "--json"},
		// The local store, exports and the six novel local-store views.
		{"sync", "--resources", novelResources, "--json"},
		{"export", "account", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"export", "billing", "--format", "json"},
		{"reconcile", "--json"},
		{"subs", "changed", "--since", "3650d", "--json"},
		{"payments", "unpaid", "--since", "3650d", "--json"},
		{"pipeline", "--json"},
		{"pipeline", "stale", "--days", "1", "--json"},
		{"company", "show", bbCompany, "--json"},
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
		// A credential on the command line: a credential-named flag, an unknown
		// flag, a positional, feedback text.
		with(list, "--client-secret", bbArgvSec),
		with(list, "--json", "--"+tok),
		{"account", "resource-company-read-company-get", tok, "--json"},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Previews: a dry run sends nothing (auth login neither mints nor saves).
		{"account", "resource-company-patch-company-patch", bbCompany, "--name", "Echo " + tok, "--dry-run"},
		{"account", "resource-company-create-company-post", "--name", "Echo " + tok, "--dry-run"},
		{"auth", "login", "--client-id", bbClientID, "--client-secret", bbClientSecret, "--dry-run"},
		{"import", "account", "--input", companies, "--dry-run"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find companies " + tok, "--resource", bbCompany, "--resource-type", "account-v2-companies",
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
		return map[string]any{"name": "appdirect_read", "arguments": args}
	}
	none := map[string]any{}
	return []map[string]any{
		read("account.resource-company-read-all-companies-get", nil),
		read("account.resource-user-read-user-get", map[string]any{"userUuid": bbUser}),
		read("billing.resource-subscription-read-subscription-get", map[string]any{"subscriptionId": bbSub}),
		read("account.resource-company-read-company-get", map[string]any{"companyUuid": bbCompany}),
		read("billing.resource-subscription-read-subscriptions-get", nil),
		read("lead.resource-app-reseller-api-get-all-get", nil),
		{"name": "appdirect_search", "arguments": map[string]any{"query": "companies"}},
		{"name": "appdirect_get", "arguments": map[string]any{"endpoint_id": "account.resource-company-read-all-companies-get"}},
		{"name": "reconcile", "arguments": none},
		{"name": "pipeline_stale", "arguments": none},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// mcpWrites is every kind of write tool, called without confirm: each must
// answer with a preview and send nothing. appdirect_execute previews as a
// confirmation_required object; the mirrored import previews as the
// command's --dry-run plan followed by the gate's "Preview only: nothing was
// sent" note.
func mcpWrites() []map[string]any {
	exec := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "appdirect_execute", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		exec("account.resource-company-patch-company-patch", map[string]any{"companyUuid": bbCompany, "name": "Preview Co"}),
		exec("account.resource-company-create-company-post", map[string]any{"name": "Preview Co"}),
		exec("account.resource-company-delete-company-membership-delete", map[string]any{"companyUuid": bbCompany, "userUuid": bbUser}),
		exec("billing.resource-other-update-subscription-patch", map[string]any{"subscriptionId": bbSub, "status": "ACTIVE", "externalId": "Preview"}),
		exec("billing.resource-subscription-cancel-subscription-delete", map[string]any{"subscriptionId": bbSub}),
		exec("integration.resource-other-bill-usage-v2-post", map[string]any{"billable": true}),
		{"name": "import", "arguments": map[string]any{"resource": "account"}},
	}
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
}

// checkMode asserts the fake saw the credential mode the test set up: every
// API request carried the mode's header and no other, and the token endpoint
// was used exactly as the mode says (in every process for an environment pair
// the files do not hold, with no refusal; never for a stored token; for a
// cached token only to replace it after the API answered 401, as the client
// does when a token may have been revoked).
func checkMode(t *testing.T, sv *server, mode credMode) {
	t.Helper()
	n, all := sv.sawHeader(mode.header())
	if n == 0 || len(all) != 1 {
		t.Errorf("%s: want every API request to carry %q, got %d of it and %d distinct values", mode, mode.header(), n, len(all))
	}
	_, mints, minted, bad := sv.counts()
	sv.mu.Lock()
	denied := sv.denied
	sv.mu.Unlock()
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
				assertNoEcho(t, "file "+rel, string(data))
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
			// The battery really wrote data, so the scan above is not vacuous.
			if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
				t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
			}
			if b, err := os.ReadFile(filepath.Join(e.home, "export.jsonl")); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
				t.Errorf("export.jsonl: want the exported record (err %v):\n%.1500s", err, b)
			}
			var sawJobs bool
			var sawCompany, sawSubscription, sawLearning bool
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				sawJobs = sawJobs || d.Name() == "jobs.jsonl"
				if strings.HasSuffix(d.Name(), ".db") {
					if b, rerr := os.ReadFile(path); rerr == nil {
						sawCompany = sawCompany || bytes.Contains(b, []byte("HQ-Record"))
						// sync read the billing lists as well as the account pages.
						sawSubscription = sawSubscription || bytes.Contains(b, []byte("HQ-Product"))
						// The teach run's learning row was written, its secret withheld in place.
						sawLearning = sawLearning || bytes.Contains(b, []byte("rotated"))
					}
				}
				return nil
			})
			if !sawCompany || !sawSubscription {
				t.Errorf("no local store holds the fetched records (company %v, subscription %v)", sawCompany, sawSubscription)
			}
			if !sawLearning {
				t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
			}
			if !sawJobs {
				t.Errorf("the asynchronous submit recorded no job; the jobs ledger scan did not run")
			}
		})
	}
}

// Class secret-in-output, with an environment pair over a stored token (the
// stored token held and unused), with the client pair minting in every
// process, and with the client pair on a cached minted token.
func TestNoCredentialInAnyOutput(t *testing.T) {
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
			// The novel views read what sync wrote, so their scan is not vacuous.
			if out := outputs["reconcile --json"]; !strings.Contains(out, bbSub) {
				t.Errorf("reconcile printed no synced subscription:\n%.1500s", out)
			}
			if out := outputs["company show "+bbCompany+" --json"]; !strings.Contains(out, "HQ-Product") {
				t.Errorf("company show printed no synced subscription:\n%.1500s", out)
			}
			// The data still prints, the echo is withheld in place, and JSON
			// output stays valid JSON.
			cmd := exec.Command(bbCLI, "account", "resource-company-read-all-companies-get", "--json")
			cmd.Env, cmd.Dir = e.env, e.home
			stdout, err := cmd.Output()
			if err != nil {
				t.Fatalf("account resource-company-read-all-companies-get --json: %v", err)
			}
			var doc any
			if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) || !bytes.Contains(stdout, []byte("text withheld")) {
				t.Fatalf("want valid JSON with the record and the withheld notice, got:\n%.1500s", stdout)
			}
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			if !strings.Contains(out, "HQ-Record") {
				t.Fatalf("MCP results lost the record itself:\n%.2000s", out)
			}
			// The fake echoed the header it really received, so the scans above
			// covered it; no other credential source was sent.
			checkMode(t, sv, mode)
		})
	}
}

// The oracle is not vacuous: the fake echoes a value that is not a credential
// (bbControl, sent as a company uuid) exactly as it echoes the credentials it
// is sent, and the output prints it, so the echo shapes teach the holder
// nothing; the credential the same responses echo is still withheld.
func TestEchoTeachesTheHolderNothing(t *testing.T) {
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			out := e.run(t, "account", "resource-company-read-company-memberships-get", bbControl, "--json")
			out += e.run(t, "account", "resource-company-read-company-get", bbControl, "--json")
			out += e.mcpCalls(t, []map[string]any{{"name": "appdirect_read", "arguments": map[string]any{
				"endpoint_id": "account.resource-company-read-company-memberships-get", "params": map[string]any{"companyUuid": bbControl}}}})
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
// marketplace is refused before it is followed, from the CLI, sync and MCP
// alike, and in the client-credentials mode a redirected token endpoint never
// receives the client credentials either.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Run("API requests", func(t *testing.T) {
		for _, mode := range []credMode{credStored, credPair} {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			redirect := e.run(t, "channel", "resource-other-read-events-get", "--json")
			sv.mu.Lock()
			sv.redirectCompanies = true
			sv.mu.Unlock()
			syncOut := e.run(t, "sync", "--resources", "account-v2-companies", "--json")
			e.mcpCalls(t, []map[string]any{
				{"name": "appdirect_read", "arguments": map[string]any{"endpoint_id": "channel.resource-other-read-events-get"}},
				{"name": "appdirect_read", "arguments": map[string]any{"endpoint_id": "account.resource-company-read-all-companies-get"}},
			})
			sv.mu.Lock()
			n := sv.offHost
			var redirected int
			for _, m := range sv.methods {
				if strings.HasSuffix(m, "/channel/v1/events") || strings.HasSuffix(m, "/account/v2/companies") {
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
			for where, out := range map[string]string{"channel events": redirect, "sync": syncOut} {
				if !strings.Contains(out, "leave the configured AppDirect host") {
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
		out := e.run(t, "account", "resource-company-read-all-companies-get", "--json")
		out += e.mcpCalls(t, []map[string]any{{"name": "appdirect_read", "arguments": map[string]any{"endpoint_id": "account.resource-company-read-all-companies-get"}}})
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
		if !strings.Contains(out, "leave the configured AppDirect host") && !strings.Contains(out, "refused cross-origin redirect") && !strings.Contains(out, "HTTP 307") {
			t.Fatalf("want the redirect refused, got:\n%.1500s", out)
		}
		assertNoEcho(t, "output", out)
	})
}

// A rejected mint (a wrong secret, the token endpoint's refusal echoing what
// it was sent and the client it decoded) returns promptly and prints nothing
// it was sent, from the CLI, from sync and from MCP.
func TestRejectedMintPrintsNoSecret(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credPair)
	const wrong = "wrongClientSecret-6d2a9f1c4e"
	e.env = append(withoutEnv(e.env, "APPDIRECT_CLIENT_SECRET"), "APPDIRECT_CLIENT_SECRET="+wrong)
	out := e.run(t, "account", "resource-company-read-all-companies-get", "--json")
	out += e.run(t, "sync", "--resources", "account-v2-companies", "--json")
	out += e.mcpCalls(t, []map[string]any{{"name": "appdirect_read", "arguments": map[string]any{"endpoint_id": "account.resource-company-read-all-companies-get"}}})
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
// call sets confirm, though the battery calls every kind of write tool; each
// write answers with a preview. In the minted mode the token endpoint is the
// only POST, and it is not a write.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
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

// Control for the test above: with confirm set to boolean true the same write
// reaches the API, so the gate previews rather than drops; the string "true"
// still previews and sends nothing at all (in the minted mode, not even a
// mint).
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			update := func(confirm any) []map[string]any {
				return []map[string]any{{"name": "appdirect_execute", "arguments": map[string]any{
					"endpoint_id": "account.resource-company-patch-company-patch", "params": map[string]any{"companyUuid": bbCompany, "name": "HQ map"}, "confirm": confirm}}}
			}
			preview := e.mcpPhases(t, update("true"))
			if !strings.Contains(preview, "confirmation_required") {
				t.Fatalf("confirm \"true\" (a string) returned no preview:\n%.1500s", preview)
			}
			if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
				t.Errorf("confirm \"true\" (a string) sent %v (%d token requests)", methods, mints)
			}
			e.mcpPhases(t, update(true))
			methods, _, _, _ := sv.counts()
			var sent bool
			for _, m := range methods {
				sent = sent || m == "PATCH /api/account/v2/companies/"+bbCompany
			}
			if !sent {
				t.Fatalf("confirm true did not send the PATCH; requests: %v", methods)
			}
			checkMode(t, sv, mode)
		})
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, and never
// mints, from a cold start with a stored token, the client pair, or the
// client pair on a cached token.
func TestDryRunSendsNothing(t *testing.T) {
	for _, mode := range []credMode{credStored, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			companies := e.importFile(t, "Dry")
			for _, args := range [][]string{
				{"account", "resource-company-patch-company-patch", bbCompany, "--name", "Dry Co", "--dry-run"},
				{"account", "resource-company-create-company-post", "--name", "Dry Co", "--dry-run"},
				{"account", "resource-company-delete-company-membership-delete", bbCompany, bbUser, "--dry-run"},
				{"billing", "resource-other-update-subscription-patch", bbSub, "--status", "ACTIVE", "--external-id", "Dry", "--dry-run"},
				{"billing", "resource-subscription-cancel-subscription-delete", bbSub, "--dry-run"},
				{"integration", "resource-other-bill-usage-v2-post", "--billable", "--dry-run"},
				{"auth", "login", "--client-id", bbClientID, "--client-secret", bbClientSecret, "--dry-run"},
				{"import", "account", "--input", companies, "--dry-run"},
			} {
				// Each run reaches its preview, so the count below is not vacuous
				// (a flag error would send nothing either). auth login refuses
				// outright under APPDIRECT_NO_CONFIG_WRITE, sending nothing.
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
			e.run(t, "account", "resource-company-patch-company-patch", bbCompany, "--name", "Dry Co")
			if methods, _, _, _ := sv.counts(); len(methods) != 1 || methods[0] != "PATCH /api/account/v2/companies/"+bbCompany {
				t.Errorf("control (%s): want the one PATCH, got %v", mode, methods)
			}
			checkMode(t, sv, mode)
		})
	}
}

// per-marketplace-token-url + api-origin-pinned, on the built binary: auth
// login with only APPDIRECT_BASE_URL set to a reseller marketplace (no
// APPDIRECT_TOKEN_URL) mints at that marketplace's /oauth2/token and nowhere
// else. Every non-loopback request the binary makes goes through a recording
// proxy (HTTPS_PROXY, HTTP_PROXY), which answers nothing, so a mint aimed at
// AppDirect's own marketplace is caught without reaching the network.
func TestAuthLoginMintsOnlyAtItsMarketplace(t *testing.T) {
	var mu sync.Mutex
	var escaped []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		escaped = append(escaped, r.Method+" "+r.Host+r.URL.String())
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(proxy.Close)
	sv := echoServer(t)
	e := newEnv(t, sv, credPair) // the pair in the environment; login must be allowed to save
	e.env = append(withoutEnv(e.env, "APPDIRECT_NO_CONFIG_WRITE"),
		"HTTPS_PROXY="+proxy.URL, "HTTP_PROXY="+proxy.URL, "https_proxy="+proxy.URL, "http_proxy="+proxy.URL, "NO_PROXY=", "no_proxy=")
	out := e.run(t, "auth", "login")
	mu.Lock()
	left := append([]string(nil), escaped...)
	mu.Unlock()
	if len(left) != 0 {
		t.Fatalf("auth login sent %d request(s) to another host: %v\n%.1500s", len(left), left, out)
	}
	_, mints, minted, bad := sv.counts()
	if mints != 1 || minted != 1 {
		t.Fatalf("auth login: want one accepted mint at the configured marketplace, got %d requests, %d accepted (refused %v):\n%.1500s", mints, minted, bad, out)
	}
	if !strings.Contains(out, "Logged in") {
		t.Fatalf("auth login did not report success:\n%.1500s", out)
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
	// marketplace without another mint, and still nothing leaves.
	sv.reset()
	e.env = withoutEnv(withoutEnv(e.env, "APPDIRECT_CLIENT_ID"), "APPDIRECT_CLIENT_SECRET")
	if read := e.run(t, "account", "resource-company-read-all-companies-get", "--json"); !strings.Contains(read, "HQ-Record") {
		t.Fatalf("the read after login fetched nothing:\n%.1500s", read)
	}
	if _, mints, _, _ := sv.counts(); mints != 0 {
		t.Errorf("the read after login minted %d time(s); the saved token was not used", mints)
	}
	if n, _ := sv.sawHeader("Bearer " + bbMintAccess); n == 0 {
		t.Errorf("the read after login did not carry the token login saved")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(escaped) != 0 {
		t.Fatalf("a request left for another host: %v", escaped)
	}
}
