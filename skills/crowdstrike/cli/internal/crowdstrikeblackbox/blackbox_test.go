// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package crowdstrikeblackbox runs the built CLI and MCP binaries against a
// CrowdStrike Falcon fake that echoes every credential it was sent back in
// every encoding it can, whole and split across lines and adjacent values,
// and checks what a reviewer cannot read off the code (crowdstrike handfixes
// credkit-wiring, store-never-keeps-secrets, no-response-cache,
// no-credential-in-files, api-origin-pinned,
// dry-run-sends-nothing, mcp-endpoint-write-confirm, client-credentials-mint,
// minted-token-bound-to-its-client, per-cloud-token-url,
// token-request-body-credentials, member-cid-token-pinned):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the FALCON_CLIENT_ID + FALCON_CLIENT_SECRET
//     pair, the access token the fake's token endpoint mints for that pair
//     (fresh in every process, or cached by an earlier run), the member_cid
//     token it mints for a Flight Control child CID (fleet sync), the token
//     stored with auth set-token, the MCP HTTP token, a credential-named flag
//     value, and the credentials a response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store and the fleet store, receipts, exports,
//     delivered files, the learn journal, config.toml), and the credential
//     store never holds an environment pair or the token minted for one;
//   - no credential leaves for a host other than the configured Falcon cloud
//     by redirect, the token mints included;
//   - the MCP server sends a write only when the call sets confirm to the
//     boolean true, and --dry-run sends nothing at all (no mint either).
//
// The fake models CrowdStrike: the API at the root of the cloud's origin, the
// token endpoint at <cloud>/oauth2/token on the same origin (POST, a form body
// grant_type=client_credentials&client_id=..&client_secret=..[&member_cid=..]
// [&scope=..], no HTTP Basic header and never a bearer token, answered
// {"access_token","token_type":"bearer","expires_in":1799}); lists answer
// {"meta":{"pagination":{"offset","limit","total"}},"resources":[...],
// "errors":[]}, query endpoints with ID strings in resources, combined
// endpoints with records (hosts keyed device_id).
//
// A unit (a printed document, a written record, a request) that repeats a
// credential is withheld whole by the shared credential kit, with its notice;
// a quiet fake (quietServer) repeats no credential, and its records must reach
// every sink, so a withheld unit is the credential and not a broken sink.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder. The fake teaches the holder nothing:
// it echoes a credential only under names that do not read as credentials and
// never in a shape the holder learns from (an auth-scheme value such as
// "Bearer <v>" or "Basic <v>" is held under any name), so a value the program
// failed to hold from its real source (the environment, the store, the mint,
// the argv) is printed, and the scan fails. TestEchoTeachesTheHolderNothing
// proves it.
package crowdstrikeblackbox

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
	bbClientID     = "clientId-3e9a7c1d5b2f8a46"           // FALCON_CLIENT_ID
	bbClientSecret = "clientSecret-9d2b6f4a1c8e7305ab"     // FALCON_CLIENT_SECRET
	bbMintAccess   = "mintedAccess-5c1e9a3f7b2d4e60c8"     // the access_token the fake's token endpoint mints for the pair
	bbMemberAccess = "memberAccess-2f8d6b1e9c4a7053d1"     // the access_token it mints for the pair and member_cid=bbChild
	bbStored       = "storedToken-71c3e9b05d2a4f68e1"      // auth set-token (stdin), the credential store
	bbArgvSec      = "-argvSecret-91c3e7a5"                // --client-secret on the command line (starts with "-")
	bbShortResp    = "Sh0rtPw"                             // a 7-character credential a response carries
	bbBOMResp      = "BomRespSecret-6a1c"                  // a credential in a BOM-prefixed response
	bbGzipResp     = "GzipRespToken-4f2a9c1e"              // a credential in a gzip-encoded response
	bbHTTPToken    = "mcpHttpToken-0123456789abcdefABCDEF" // CROWDSTRIKE_MCP_HTTP_TOKEN
	bbScope        = "devices:read alerts:read"            // CROWDSTRIKE_OAUTH_SCOPE: sent in the token form, not a credential
	bbChild        = "c0ffee00child00cid0000000000bb01"    // the one Flight Control child CID
	bbHost         = "dev-1042"                            // a host device_id
	bbPolicy       = "pol-7731"                            // a prevention policy id
	bbDenied       = "dev-denied"                          // an id the fake answers 401
	bbControl      = "ctlEchoValue-7a1b9c2d3e"             // not a credential: the fake echoes it like one (TestEchoTeachesTheHolderNothing)
)

// withheldNotice is the start of what the credential kit prints in place of a
// unit that carried a credential.
const withheldNotice = "[credkit: withheld"

func secrets() []string {
	return []string{bbClientID, bbClientSecret, bbMintAccess, bbMemberAccess, bbStored, bbArgvSec,
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
	dir, err := os.MkdirTemp("", "crowdstrike-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "crowdstrike-cli"+exe)
	bbMCP = filepath.Join(dir, "crowdstrike-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/crowdstrike-cli", bbMCP: "../../cmd/crowdstrike-mcp"} {
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
	url    string // the API base URL: the Falcon cloud's origin
	caFile string
	mu     sync.Mutex
	// quiet repeats no credential it was sent (only bbControl) and answers no
	// response credential: a real Falcon API, the control for what reaches a sink.
	quiet   bool
	offHost int
	methods []string       // API requests but the token endpoint, "METHOD /path"
	headers map[string]int // Authorization header values those requests carried
	// memberReads is "METHOD /path" of every API request that carried the
	// member_cid token.
	memberReads []string
	mints       int      // parent token requests (POST <cloud>/oauth2/token, no member_cid)
	minted      int      // ... that the fake answered with the minted token
	memberMints int      // member_cid token requests
	memberOK    int      // ... answered with the member_cid token
	badMint     []string // token requests the fake refused, and why
	denied      int      // API requests the fake answered 401 (the client may re-mint after one)
	// redirectHosts sends the combined hosts list (a sync and fleet sync
	// resource) to the other host too, so sync and fleet sync meet a redirect.
	redirectHosts bool
	// redirectToken sends the token endpoint to the other host (a 307 would
	// resend the request and the form carrying the client secret).
	redirectToken bool
}

// fleetChildReads are the requests fleet sync makes for a child CID with the
// member_cid token, and the only ones that may carry it.
var fleetChildReads = map[string]bool{
	"GET /devices/combined/devices/v1": true, "POST /alerts/combined/alerts/v1": true,
	"GET /spotlight/combined/vulnerabilities/v1": true, "GET /policy/combined/prevention/v1": true,
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

// Records shaped the way the Falcon API returns them, so the store, the fleet
// store, the fleet novels' local views and the exports have something real to
// read: a host (device_id), a prevention policy, an alert, a Spotlight
// vulnerability.
var (
	lastSeen   = time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	hostRecord = `"device_id":"dev-1042","hostname":"HQ-Record","status":"normal","platform_name":"Windows","last_seen":"` + lastSeen + `"`
	policyRec  = `"id":"pol-7731","name":"HQ-Policy","platform_name":"Windows","enabled":true`
	alertRec   = `"composite_id":"cid:ind:1042","display_name":"HQ-Alert","severity_name":"Critical","status":"new","created_timestamp":"` + lastSeen + `"`
	vulnRec    = `"id":"vul-1042","status":"open","cve":{"id":"CVE-2026-1042","severity":"CRITICAL"},"host_info":{"hostname":"HQ-Record"}`
)

// falconList wraps items the way a Falcon list answers: meta.pagination and
// the resources.
func falconList(items ...string) string {
	return falconPage(0, items...)
}

// falconPage is the page of items a list request at offset answers: the
// records from offset on (none past the end, which ends every walk), and the
// list's total.
func falconPage(offset int, items ...string) string {
	page := items
	if offset > 0 {
		page = nil
		if offset < len(items) {
			page = items[offset:]
		}
	}
	return `{"meta":{"query_time":0.01,"pagination":{"offset":` + strconv.Itoa(offset) + `,"limit":100,"total":` + strconv.Itoa(len(items)) + `},"trace_id":"trace-1042"},"resources":[` +
		strings.Join(page, ",") + `],"errors":[]}`
}

// requestOffset is the offset a list request asks for: the offset query
// parameter, or the offset member of a POST list body.
func requestOffset(r *http.Request, body []byte) int {
	if n, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil {
		return n
	}
	var b struct {
		Offset int `json:"offset"`
	}
	_ = json.Unmarshal(body, &b)
	return b.Offset
}

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{headers: map[string]int{}}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(falconList()))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/oauth2/token" {
			sv.token(w, r, body, other.URL)
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
		p := r.URL.Path
		get := r.Method == http.MethodGet
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+p)
		h := r.Header.Get("Authorization")
		if h != "" {
			sv.headers[h]++
		}
		if h == "Bearer "+bbMemberAccess {
			sv.memberReads = append(sv.memberReads, r.Method+" "+p)
		}
		redirectHosts := sv.redirectHosts
		sv.mu.Unlock()
		echoID := ""
		if len(sent) > 0 {
			// A record whose identifier or name repeats the first value it was sent.
			echoID = fmt.Sprintf(`{"id":%q,"device_id":%q,"hostname":"Echo-ID","status":"normal"}`, sent[0], sent[0])
		}
		offset := requestOffset(r, body)
		list := func(rec string) string {
			items := []string{fmt.Sprintf(`{%s,"echo":%s}`, rec, echo)}
			if echoID != "" {
				items = append(items, echoID)
			}
			return falconPage(offset, items...)
		}
		switch {
		case strings.HasPrefix(p, "/incidents/") || strings.Contains(r.URL.RawQuery+string(body), bbDenied):
			// Rejected: the error repeats what it was sent.
			sv.mu.Lock()
			sv.denied++
			sv.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"meta":{"query_time":0.01,"echo":%s},"resources":[],"errors":[{"code":401,"message":"access denied, authorization failed for %s"}]}`, echo, strings.Join(sent, " "))
		case p == "/devices/combined/devices-hidden/v1" || (redirectHosts && p == "/devices/combined/devices/v1"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && p == "/devices/entities/devices/v2":
			// host details: the echo split across values, lines and spaces.
			fmt.Fprint(w, falconList(fmt.Sprintf(`{%s,"split":%s}`, hostRecord, splitObject(sent))))
		case quiet && get && p == "/devices/entities/online-state/v1":
			fmt.Fprint(w, "\xef\xbb\xbf"+falconList(`{"id":"dev-1042","state":"online"}`))
		case quiet && get && p == "/policy/entities/prevention/v1":
			fmt.Fprint(w, falconList("{"+policyRec+"}"))
		case get && p == "/devices/entities/online-state/v1":
			// online state: BOM-prefixed, with a password.
			fmt.Fprint(w, "\xef\xbb\xbf"+falconList(fmt.Sprintf(`{"id":"dev-1042","state":"online","note":%q,"password":%q}`, bbBOMResp, bbBOMResp)))
		case get && p == "/policy/entities/prevention/v1":
			// a prevention policy: a short credential.
			fmt.Fprint(w, falconList(fmt.Sprintf(`{%s,"note":%q,"password":%q}`, policyRec, bbShortResp, bbShortResp)))
		case get && p == "/devices/combined/devices/v1":
			// the hosts list (sync, export, fleet sync): gzip-encoded, with an
			// access token, the echo, and a record whose identifier repeats
			// the value it was sent.
			items := []string{fmt.Sprintf(`{%s,"accessToken":%q,"echo":%s}`, hostRecord, bbGzipResp, echoObject(append(sent, bbGzipResp)))}
			if quiet {
				items = []string{fmt.Sprintf(`{%s,"echo":%s}`, hostRecord, echo)}
			}
			if echoID != "" {
				items = append(items, echoID)
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprint(zw, falconPage(offset, items...))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case get && p == "/devices/queries/devices/v1":
			// a query endpoint: ID strings, one repeating the first value sent.
			ids := []string{strconv.Quote(bbHost)}
			if len(sent) > 0 {
				ids = append(ids, strconv.Quote(sent[0]))
			}
			fmt.Fprint(w, falconPage(offset, ids...))
		case get && p == "/mssp/queries/children/v1":
			fmt.Fprint(w, falconPage(offset, strconv.Quote(bbChild)))
		case get && p == "/policy/combined/prevention/v1":
			fmt.Fprint(w, list(policyRec))
		case r.Method == http.MethodPost && p == "/alerts/combined/alerts/v1":
			fmt.Fprint(w, list(alertRec))
		case get && p == "/spotlight/combined/vulnerabilities/v1":
			fmt.Fprint(w, list(vulnRec))
		case !get:
			// Every write echoes what it was sent.
			fmt.Fprint(w, falconList(fmt.Sprintf(`{%s,"echo":%s}`, hostRecord, echo)))
		default:
			fmt.Fprint(w, list(`"id":"rec-1","name":"HQ-Other"`))
		}
	}))
	sv.url = srv.URL
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// quietServer is echoServer without the credential echoes: a real Falcon API,
// whose records must reach every sink the echoing server's are withheld from.
func quietServer(t *testing.T) *server {
	t.Helper()
	sv := echoServer(t)
	sv.quiet = true
	return sv
}

// token emulates CrowdStrike's token endpoint: POST <cloud>/oauth2/token on
// the API origin, an application/x-www-form-urlencoded body
// grant_type=client_credentials&client_id=..&client_secret=..&scope=<the
// configured scope>[&member_cid=<child CID>], no Authorization header at all
// (CrowdStrike reads the pair from the form: an HTTP Basic header, which
// 4.34.2 rendered, or a bearer token is refused), answered with
// {"access_token","token_type":"bearer","expires_in":1799}: the parent token
// for the pair alone, the member_cid token for the pair and the child CID. It
// answers only the configured pair, and never echoes a request on success. A
// refusal repeats what it was sent, and the client ID and secret it read from
// the form, under names that are not credentials (a real token endpoint may
// name the client it refused).
func (sv *server) token(w http.ResponseWriter, r *http.Request, body []byte, otherURL string) {
	form, ferr := url.ParseQuery(string(body))
	member := form.Get("member_cid")
	sv.mu.Lock()
	if member != "" {
		sv.memberMints++
	} else {
		sv.mints++
	}
	redirect := sv.redirectToken
	sv.mu.Unlock()
	if redirect {
		http.Redirect(w, r, otherURL+r.URL.Path, http.StatusTemporaryRedirect)
		return
	}
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
	case form.Get("client_id") != bbClientID:
		why = "unknown client"
	case form.Get("client_secret") != bbClientSecret:
		why = "wrong client secret"
	case member != "" && member != bbChild:
		why = "unknown member CID"
	}
	sv.mu.Lock()
	switch {
	case why != "":
		sv.badMint = append(sv.badMint, why)
	case member != "":
		sv.memberOK++
	default:
		sv.minted++
	}
	sv.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if why != "" {
		posted := sentValues(r, body)
		for _, v := range []string{form.Get("client_id"), form.Get("client_secret")} {
			if v != "" {
				posted = append(posted, v)
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"meta":{"query_time":0.01},"errors":[{"code":401,"message":"access denied, invalid client: rejected %s"}],"echo":%s}`, strings.Join(posted, " "), echoObject(posted))
		return
	}
	tok := bbMintAccess
	if member != "" {
		tok = bbMemberAccess
	}
	fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":1799}`, tok)
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

// counts reports the API requests and the parent token requests so far
// (sent, accepted), and every refused token request.
func (sv *server) counts() (methods []string, mints, minted int, bad []string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return append([]string(nil), sv.methods...), sv.mints, sv.minted, append([]string(nil), sv.badMint...)
}

// memberCounts reports the member_cid token requests (sent, accepted) and the
// API requests that carried the member_cid token.
func (sv *server) memberCounts() (mints, ok int, reads []string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.memberMints, sv.memberOK, append([]string(nil), sv.memberReads...)
}

func (sv *server) reset() {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.methods, sv.headers, sv.mints, sv.minted, sv.badMint, sv.offHost, sv.denied = nil, map[string]int{}, 0, 0, nil, 0, 0
	sv.memberReads, sv.memberMints, sv.memberOK = nil, 0, 0
}

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its Authorization. CrowdStrike has no
// static-token variable: the environment can carry only the client pair.
type credMode int

const (
	credStored         credMode = iota // none in the environment: the battery runs on a token stored with auth set-token (a token with no client)
	credPairOverStored                 // a token stored with auth set-token, and a client pair in the environment: the pair mints its own token in every process and keeps it in memory; the stored token is held, never sent, never replaced
	credPair                           // the client pair, CROWDSTRIKE_NO_CONFIG_WRITE=1: every process mints its own token, nothing is cached
	credPairCached                     // the client pair stored in the credential store, and the token an earlier run minted for it cached beside it
)

func (m credMode) String() string {
	return [...]string{"stored token", "env pair over a stored token", "client pair minted", "client pair cached token"}[m]
}

// header is the Authorization value every API request must carry in mode m
// (fleet sync's child reads carry the member_cid token instead).
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

// hasPair reports whether mode m has a parent client pair (fleet sync runs).
func (m credMode) hasPair() bool { return m != credStored }

// newEnv builds a clean environment against the fake with the credentials
// mode names. Every user directory (CROWDSTRIKE_HOME, HOME, the XDG and Windows
// dirs, TMPDIR) is inside a fresh t.TempDir. CROWDSTRIKE_OAUTH_SCOPE is set, so
// every token request must carry it (a setting, not a credential).
// credStored and credPairOverStored first store bbStored with auth set-token;
// credPairCached first runs one read so the stored client pair mints a token
// and caches it in the credential store. The fake's counters are cleared
// after either.
func newEnv(t *testing.T, sv *server, mode credMode) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "CROWDSTRIKE_HOME=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"CROWDSTRIKE_BASE_URL=" + sv.url, "CROWDSTRIKE_CA_CERT=" + sv.caFile, "CROWDSTRIKE_OAUTH_SCOPE=" + bbScope,
		"CROWDSTRIKE_CLI_PATH=" + bbCLI, "CROWDSTRIKE_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	e := &bbEnv{home: home, env: env}
	pair := []string{"FALCON_CLIENT_ID=" + bbClientID, "FALCON_CLIENT_SECRET=" + bbClientSecret}
	switch mode {
	case credStored, credPairOverStored:
		if out := e.runIn(t, bbStored+"\n", "auth", "set-token"); !e.credentialStoreHolds(bbStored) {
			t.Fatalf("auth set-token stored no token:\n%.1500s", out)
		}
		if mode == credPairOverStored {
			e.env = append(e.env, pair...)
		}
	case credPair:
		e.env = append(e.env, append(pair, "CROWDSTRIKE_NO_CONFIG_WRITE=1")...)
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
		out := e.run(t, "devices", "query-by-filter", "--json")
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

// importFile writes an import input: one host group whose name repeats tok,
// so the create request carries it and the fake echoes it. It lives outside
// the home: it is the test's input, not a file the program wrote.
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "host-groups.jsonl")
	if err := os.WriteFile(p, []byte(`{"name":"Imported `+tok+`","group_type":"static","description":"d"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// call is one CLI run: its arguments and what it reads on stdin.
type call struct {
	args  []string
	stdin string
}

// battery runs every output path; tok is a credential the process holds
// (credMode.held).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) []call {
	groups := e.importFile(t, tok)
	list := []string{"devices", "combined-by-filter"}
	with := func(base []string, more ...string) []string { return append(append([]string{}, base...), more...) }
	run := func(args ...string) call { return call{args: args} }
	in := func(stdin string, args ...string) call { return call{args: args, stdin: stdin} }
	return []call{
		// Every output mode.
		run(with(list, "--json")...),
		run(list...),
		run(with(list, "--csv")...),
		run(with(list, "--plain")...),
		run(with(list, "--json", "--receipt")...),
		run(with(list, "--json", "--audit-dir", filepath.Join(e.home, "audit"))...),
		run(with(list, "--json", "--deliver", "file:"+deliverFile)...),
		// Every response shape: BOM-prefixed, short credential, split echo,
		// 401 echo, refused redirect, an id that repeats a held value.
		run("devices", "get-online-state-v1", "--ids", bbHost, "--json"),
		run("policy", "get-prevention-policies", "--ids", bbPolicy, "--json"),
		run("devices", "get-details-v2", "--ids", bbHost, "--json"),
		run("incidents", "query", "--json"),
		run("devices", "combined-hidden-by-filter", "--json"),
		run("devices", "query-by-filter", "--json"),
		run("mssp", "query-children", "--json"),
		// Writes the fake echoes (host containment, a tag that repeats the
		// held value), a rejected write, and import.
		in(`{"ids":["`+bbHost+`"]}`, "devices", "perform-action-v2", "--action-name", "contain", "--stdin", "--json"),
		in(`{"action":"add","device_ids":["`+bbHost+`"],"tags":["FalconGroupingTags/HQ `+tok+`"]}`, "devices", "update-tags", "--stdin", "--json"),
		run("devices", "delete-host-groups", "--ids", bbDenied, "--json"),
		run("import", "devices", "--input", groups, "--json"),
		// The local store, exports, and the fleet novel: fleet sync lists the
		// children with the parent token, mints a member_cid token for the
		// child and reads it; the rollups read the fleet store.
		run("sync", "--resources", "devices,policy-combined-prevention-v1", "--json"),
		run("export", "devices", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")),
		run("export", "policy", "--format", "json"),
		run("fleet", "sync", "--all-cids", "--json"),
		run("fleet", "scorecard", "--json"),
		run("fleet", "vulns", "--json"),
		run("fleet", "stale", "--days", "1", "--json"),
		run("fleet", "alerts", "--json"),
		run("fleet", "search", "HQ", "--json"),
		run("fleet", "tenants", "--json"),
		run("fleet", "remediate", "--json"),
		run("fleet", "policy-drift", "--json"),
		run("fleet", "trend", "--json"),
		run("search", "HQ", "--json"),
		run("search", "Echo", "--json", "--data-source", "local"),
		// A credential on the command line: a credential-named flag, an unknown
		// flag, a flag value, feedback text.
		run(with(list, "--client-secret", bbArgvSec)...),
		run(with(list, "--json", "--"+tok)...),
		run("devices", "get-details-v2", "--ids", tok, "--json"),
		run("feedback", "noted the token "+tok+" in a ticket"),
		// Previews: a dry run sends nothing (auth login neither mints nor
		// saves; fleet sync mints no member_cid token).
		in(`{"ids":["Echo `+tok+`"]}`, "devices", "perform-action-v2", "--action-name", "contain", "--stdin", "--dry-run"),
		run("devices", "delete-host-groups", "--ids", "Echo "+tok, "--dry-run"),
		run("auth", "login", "--client-id", bbClientID, "--client-secret", bbClientSecret, "--dry-run"),
		run("import", "devices", "--input", groups, "--dry-run"),
		run("fleet", "sync", "--all-cids", "--dry-run"),
		// Learnings, playbooks and learn events land in the local store's own tables.
		run("teach", "--query", "find hosts "+tok, "--resource", bbHost, "--resource-type", "devices",
			"--notes", "rotated "+tok, "--playbook-notes", "use "+tok, "--no-validate"),
		run("doctor"),
		run("auth", "status"),
	}
}

// mcpReads are read-only tools across every response shape the fake has, and
// the fleet novels' local views.
func mcpReads() []map[string]any {
	read := func(id string, params map[string]any) map[string]any {
		args := map[string]any{"endpoint_id": id}
		if params != nil {
			args["params"] = params
		}
		return map[string]any{"name": "crowdstrike_read", "arguments": args}
	}
	none := map[string]any{}
	return []map[string]any{
		read("devices.combined-by-filter", nil),
		read("devices.get-online-state-v1", map[string]any{"ids": bbHost}),
		read("policy.get-prevention-policies", map[string]any{"ids": bbPolicy}),
		read("devices.get-details-v2", map[string]any{"ids": bbHost}),
		read("devices.query-by-filter", nil),
		read("incidents.query", nil),
		{"name": "crowdstrike_search", "arguments": map[string]any{"query": "hosts"}},
		{"name": "crowdstrike_get", "arguments": map[string]any{"endpoint_id": "devices.combined-by-filter"}},
		{"name": "fleet_scorecard", "arguments": none},
		{"name": "fleet_stale", "arguments": none},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// mcpWrites is every kind of write tool, called without confirm: each must
// answer with a preview and send nothing. crowdstrike_execute previews as a
// confirmation_required object; the mirrored import and fleet sync preview as
// the command's --dry-run plan followed by the gate's "Preview only: nothing
// was sent" note.
func mcpWrites() []map[string]any {
	exec := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "crowdstrike_execute", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		exec("devices.perform-action-v2", map[string]any{"action_name": "contain", "ids": []any{bbHost}}),
		exec("devices.create-host-groups", map[string]any{"resources": []any{map[string]any{"name": "Preview Group", "group_type": "static"}}}),
		exec("devices.delete-host-groups", map[string]any{"ids": "hg-1042"}),
		exec("devices.update-tags", map[string]any{"action": "add", "device_ids": []any{bbHost}, "tags": []any{"FalconGroupingTags/Preview"}}),
		exec("policy.delete-prevention-policies", map[string]any{"ids": bbPolicy}),
		exec("mssp.update-cidgroups", map[string]any{"resources": []any{map[string]any{"cid_group_id": "cg-1", "name": "Preview"}}}),
		{"name": "import", "arguments": map[string]any{"resource": "devices"}},
		{"name": "fleet_sync", "arguments": map[string]any{"all-cids": true}},
	}
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
}

// checkMode asserts the fake saw the credential mode the test set up: every
// API request carried the mode's header (fleet sync's child reads the
// member_cid token, and only those reads), and the token endpoint was used
// exactly as the mode says (for the parent token: in every process for an
// environment pair the files do not hold, with no refusal; never for a stored
// token; for a cached token only to replace it after the API answered 401, as
// the client does when a token may have been revoked). Every member_cid mint
// was accepted.
func checkMode(t *testing.T, sv *server, mode credMode) {
	t.Helper()
	n, all := sv.sawHeader(mode.header())
	var other []string
	for _, h := range all {
		if h != mode.header() && h != "Bearer "+bbMemberAccess {
			other = append(other, h)
		}
	}
	if n == 0 || len(other) != 0 {
		t.Errorf("%s: want every API request to carry %q (child reads the member_cid token), got %d of it and other values %q", mode, mode.header(), n, other)
	}
	memberMints, memberOK, memberReads := sv.memberCounts()
	for _, r := range memberReads {
		if !fleetChildReads[r] {
			t.Errorf("%s: %s carried the member_cid token; only fleet sync's child reads may", mode, r)
		}
	}
	if memberOK != memberMints {
		t.Errorf("%s: %d member_cid token requests, %d accepted", mode, memberMints, memberOK)
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
	case mints != 0 || memberMints != 0:
		t.Errorf("%s: want no mint (the token is stored), got %d token requests and %d member_cid ones (refused %v)", mode, mints, memberMints, bad)
	}
}

// Class credential-persisted-in-files, in every credential mode.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPairOverStored, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, c := range battery(t, e, deliverFile, mode.held()) {
				e.runIn(t, c.stdin, c.args...)
			}
			e.mcpCalls(t, mcpBattery())
			checkMode(t, sv, mode)
			if _, _, minted, _ := sv.counts(); mode.mints() && minted < 5 {
				// The environment pair's token lives in its process: no
				// process can reuse another's token from disk.
				t.Errorf("%s: only %d mints across the battery; a token was reused from disk", mode, minted)
			}
			if _, memberOK, memberReads := sv.memberCounts(); mode.hasPair() && (memberOK == 0 || len(memberReads) == 0) {
				t.Errorf("%s: fleet sync minted %d member_cid token(s) and read the child %d time(s); the fleet scan did not run", mode, memberOK, len(memberReads))
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
					// for one, a member_cid token, or one a response or the
					// command line carried.
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
		})
	}
}

// The control for TestNoCredentialInAnyWrittenFile: a Falcon API that repeats
// no credential has its records reach every file sink (a delivered file, an
// export, the local store, the learn tables, the fleet store), so a write
// withheld there is the credential and not a broken sink.
func TestOrdinaryRecordsReachEveryFile(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := quietServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			runs := [][]string{
				{"devices", "combined-by-filter", "--json", "--deliver", "file:" + deliverFile},
				{"sync", "--resources", "devices,policy-combined-prevention-v1", "--json"},
				{"export", "devices", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
				{"teach", "--query", "find hosts", "--resource", bbHost, "--resource-type", "devices", "--notes", "rotated", "--no-validate"},
				{"feedback", "an ordinary note"},
			}
			if mode.hasPair() {
				runs = append(runs, []string{"fleet", "sync", "--all-cids", "--json"})
			}
			for _, args := range runs {
				if out := e.run(t, args...); strings.Contains(out, withheldNotice) {
					t.Errorf("%v withheld an ordinary record:\n%.1500s", args, out)
				}
			}
			if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
				t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
			}
			if b, err := os.ReadFile(filepath.Join(e.home, "export.jsonl")); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
				t.Errorf("export.jsonl: want the exported record (err %v):\n%.1500s", err, b)
			}
			var sawHost, sawPolicy, sawLearning, sawFleet, sawFeedback bool
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					return nil
				}
				if strings.Contains(d.Name(), ".db") {
					sawHost = sawHost || bytes.Contains(b, []byte("HQ-Record"))
					// sync read the policies list as well as the hosts.
					sawPolicy = sawPolicy || bytes.Contains(b, []byte("HQ-Policy"))
					sawLearning = sawLearning || bytes.Contains(b, []byte("rotated"))
					// fleet sync stored the child's rows under its CID.
					sawFleet = sawFleet || bytes.Contains(b, []byte(bbChild))
				}
				sawFeedback = sawFeedback || bytes.Contains(b, []byte("an ordinary note"))
				return nil
			})
			if !sawHost || !sawPolicy {
				t.Errorf("no local store holds the fetched records (host %v, policy %v)", sawHost, sawPolicy)
			}
			if !sawLearning {
				t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
			}
			if !sawFeedback {
				t.Errorf("no file holds the feedback note")
			}
			if mode.hasPair() && !sawFleet {
				t.Errorf("%s: no local store holds the child CID's fleet rows; the fleet store scan did not run", mode)
			}
		})
	}
}

// Class secret-in-output, with an environment pair over a stored token (the
// stored token held and unused), with the client pair minting in every
// process, and with the client pair on a cached minted token.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credPairOverStored, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			outputs := map[string]string{}
			for _, c := range battery(t, e, deliverFile, mode.held()) {
				out := e.runIn(t, c.stdin, c.args...)
				outputs[strings.Join(c.args, " ")] = out
				assertNoEcho(t, fmt.Sprintf("output of %v", c.args), out)
			}
			// The unit that repeats a credential is withheld whole, with the kit's notice.
			if out := outputs["devices combined-by-filter --json"]; !strings.Contains(out, withheldNotice) {
				t.Errorf("devices combined-by-filter --json repeats a credential and printed no notice:\n%.1500s", out)
			}
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			assertJSONLines(t, out)
			// The fake echoed the header it really received, so the scans above
			// covered it; no other credential source was sent.
			checkMode(t, sv, mode)
		})
	}
}

// The control for TestNoCredentialInAnyOutput: a Falcon API that repeats no
// credential has its records printed, on the CLI and through the MCP server
// (in the server and in a CLI child), JSON output stays JSON, and the fleet
// views read what fleet sync wrote.
func TestOrdinaryRecordsStillPrint(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := quietServer(t)
			e := newEnv(t, sv, mode)
			cmd := exec.Command(bbCLI, "devices", "combined-by-filter", "--json")
			cmd.Env, cmd.Dir = e.env, e.home
			stdout, err := cmd.Output()
			if err != nil {
				t.Fatalf("devices combined-by-filter --json: %v\n%.1500s", err, stdout)
			}
			var doc any
			if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
				t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
			}
			for _, args := range [][]string{
				{"devices", "get-online-state-v1", "--ids", bbHost, "--json"},
				{"policy", "get-prevention-policies", "--ids", bbPolicy, "--json"},
			} {
				if out := e.run(t, args...); strings.Contains(out, withheldNotice) || (!strings.Contains(out, "dev-1042") && !strings.Contains(out, "HQ-Policy")) {
					t.Errorf("%v lost its record:\n%.1500s", args, out)
				}
			}
			if mode.hasPair() {
				if out := e.run(t, "fleet", "sync", "--all-cids", "--json"); !strings.Contains(out, `"cids_synced": 1`) {
					t.Errorf("fleet sync synced no child CID:\n%.1500s", out)
				}
				if out := e.run(t, "fleet", "stale", "--days", "1", "--json"); !strings.Contains(out, "HQ-Record") || !strings.Contains(out, bbChild) {
					t.Errorf("fleet stale printed no synced child host:\n%.1500s", out)
				}
			}
			out := e.mcpCalls(t, mcpReads())
			if strings.Contains(out, withheldNotice) || strings.Count(out, "HQ-Record") < 2 {
				t.Fatalf("MCP results lost the record:\n%.3000s", out)
			}
			assertJSONLines(t, out)
		})
	}
}

// The oracle is not vacuous: the fake echoes a value that is not a credential
// (bbControl, sent as a host filter and a host id) in every shape it echoes
// the credentials it is sent, and the output prints it, so the echo shapes
// teach the holder nothing. (The quiet fake echoes only the control: a unit
// that also repeats a credential is withheld whole.)
func TestEchoTeachesTheHolderNothing(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := quietServer(t)
			e := newEnv(t, sv, mode)
			out := e.run(t, "devices", "combined-by-filter", "--filter", bbControl, "--json")
			out += e.run(t, "devices", "get-details-v2", "--ids", bbControl, "--json")
			out += e.mcpCalls(t, []map[string]any{{"name": "crowdstrike_read", "arguments": map[string]any{
				"endpoint_id": "devices.combined-by-filter", "params": map[string]any{"filter": bbControl}}}})
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

// Class credential-sent-to-another-host: a redirect off the configured cloud
// is refused before it is followed, from the CLI, sync, fleet sync's child
// reads and MCP alike, and in the client-credentials mode a redirected token
// endpoint never receives the client credentials either (the parent mint and
// the member_cid mint).
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	t.Run("API requests", func(t *testing.T) {
		for _, mode := range []credMode{credStored, credPair} {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			redirect := e.run(t, "devices", "combined-hidden-by-filter", "--json")
			sv.mu.Lock()
			sv.redirectHosts = true
			sv.mu.Unlock()
			syncOut := e.run(t, "sync", "--resources", "devices", "--json")
			outs := map[string]string{"hidden hosts": redirect, "sync": syncOut}
			want := 4
			if mode.hasPair() {
				outs["fleet sync"] = e.run(t, "fleet", "sync", "--cids", bbChild, "--kinds", "hosts", "--json")
				want = 5
			}
			e.mcpCalls(t, []map[string]any{
				{"name": "crowdstrike_read", "arguments": map[string]any{"endpoint_id": "devices.combined-hidden-by-filter"}},
				{"name": "crowdstrike_read", "arguments": map[string]any{"endpoint_id": "devices.combined-by-filter"}},
			})
			sv.mu.Lock()
			n := sv.offHost
			var redirected int
			for _, m := range sv.methods {
				if strings.HasSuffix(m, "/devices/combined/devices-hidden/v1") || strings.HasSuffix(m, "/devices/combined/devices/v1") {
					redirected++
				}
			}
			sv.mu.Unlock()
			if n != 0 {
				t.Fatalf("%s: the other host was contacted %d time(s)", mode, n)
			}
			if redirected < want {
				t.Fatalf("%s: only %d request(s) met a redirect; the CLI, sync, fleet sync and MCP calls did not all run", mode, redirected)
			}
			for where, out := range outs {
				if !strings.Contains(out, "leave the configured CrowdStrike host") {
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
		out := e.run(t, "devices", "combined-by-filter", "--json")
		out += e.run(t, "fleet", "sync", "--cids", bbChild, "--kinds", "hosts", "--json")
		out += e.mcpCalls(t, []map[string]any{{"name": "crowdstrike_read", "arguments": map[string]any{"endpoint_id": "devices.combined-by-filter"}}})
		methods, mints, _, _ := sv.counts()
		memberMints, _, _ := sv.memberCounts()
		if mints < 2 || memberMints < 1 {
			t.Fatalf("the token endpoint saw %d parent and %d member_cid request(s); the CLI, fleet sync and MCP mints did not all run:\n%.1500s", mints, memberMints, out)
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
		if !strings.Contains(out, "leave the configured CrowdStrike host") && !strings.Contains(out, "refused cross-origin redirect") && !strings.Contains(out, "HTTP 307") {
			t.Fatalf("want the redirect refused, got:\n%.1500s", out)
		}
		assertNoEcho(t, "output", out)
	})
}

// A rejected mint (a wrong secret, the token endpoint's refusal echoing what
// it was sent and the client it read from the form) returns promptly and
// prints nothing it was sent, from the CLI, sync, fleet sync's member_cid mint
// and MCP.
func TestRejectedMintPrintsNoSecret(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credPair)
	const wrong = "wrongClientSecret-6d2a9f1c4e"
	e.env = append(withoutEnv(e.env, "FALCON_CLIENT_SECRET"), "FALCON_CLIENT_SECRET="+wrong)
	out := e.run(t, "devices", "combined-by-filter", "--json")
	out += e.run(t, "sync", "--resources", "devices", "--json")
	out += e.run(t, "fleet", "sync", "--cids", bbChild, "--kinds", "hosts", "--json")
	out += e.mcpCalls(t, []map[string]any{{"name": "crowdstrike_read", "arguments": map[string]any{"endpoint_id": "devices.combined-by-filter"}}})
	_, mints, minted, bad := sv.counts()
	memberMints, memberOK, _ := sv.memberCounts()
	if mints == 0 || memberMints == 0 || minted != 0 || memberOK != 0 || len(bad) == 0 {
		t.Fatalf("want only refused mints (parent and member_cid), got %d+%d requests, %d+%d accepted (refused %v)", mints, memberMints, minted, memberOK, bad)
	}
	for _, why := range bad {
		if why != "wrong client secret" {
			t.Fatalf("a mint was refused for %q, not the wrong secret: the request was malformed", why)
		}
	}
	if !strings.Contains(out, "401") && !strings.Contains(strings.ToLower(out), "invalid client") && !strings.Contains(strings.ToLower(out), "access denied") {
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
// call sets confirm, though the battery calls every kind of write tool (host
// containment, host-group and policy create and delete, an MSSP group edit,
// import, fleet sync); each write answers with a preview. In the minted mode
// the token endpoint is the only POST, and it is not a write.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			out := e.mcpCalls(t, mcpBattery())
			// A record, or the kit's notice for one that repeats a credential.
			if !strings.Contains(out, "HQ-Record") && !strings.Contains(out, withheldNotice) {
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
			if memberMints, _, _ := sv.memberCounts(); memberMints != 0 {
				t.Errorf("fleet sync without confirm minted %d member_cid token(s)", memberMints)
			}
			checkMode(t, sv, mode)
		})
	}
}

// Control for the test above: with confirm set to boolean true the same write
// (host containment) reaches the API, so the gate previews rather than drops;
// the string "true" still previews and sends nothing at all (in the minted
// mode, not even a mint).
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			contain := func(confirm any) []map[string]any {
				return []map[string]any{{"name": "crowdstrike_execute", "arguments": map[string]any{
					"endpoint_id": "devices.perform-action-v2", "params": map[string]any{"action_name": "contain", "ids": []any{bbHost}}, "confirm": confirm}}}
			}
			preview := e.mcpPhases(t, contain("true"))
			if !strings.Contains(preview, "confirmation_required") {
				t.Fatalf("confirm \"true\" (a string) returned no preview:\n%.1500s", preview)
			}
			if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
				t.Errorf("confirm \"true\" (a string) sent %v (%d token requests)", methods, mints)
			}
			e.mcpPhases(t, contain(true))
			methods, _, _, _ := sv.counts()
			var sent bool
			for _, m := range methods {
				sent = sent || m == "POST /devices/entities/devices-actions/v2"
			}
			if !sent {
				t.Fatalf("confirm true did not send the containment POST; requests: %v", methods)
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
			for _, c := range []call{
				{args: []string{"devices", "perform-action-v2", "--action-name", "contain", "--stdin", "--dry-run"}, stdin: `{"ids":["` + bbHost + `"]}`},
				{args: []string{"devices", "create-host-groups", "--stdin", "--dry-run"}, stdin: `{"resources":[{"name":"Dry Group","group_type":"static"}]}`},
				{args: []string{"devices", "delete-host-groups", "--ids", "hg-1042", "--dry-run"}},
				{args: []string{"devices", "update-tags", "--stdin", "--dry-run"}, stdin: `{"action":"add","device_ids":["` + bbHost + `"],"tags":["FalconGroupingTags/Dry"]}`},
				{args: []string{"policy", "delete-prevention-policies", "--ids", bbPolicy, "--dry-run"}},
				{args: []string{"auth", "login", "--client-id", bbClientID, "--client-secret", bbClientSecret, "--dry-run"}},
				{args: []string{"import", "devices", "--input", groups, "--dry-run"}},
				{args: []string{"fleet", "sync", "--all-cids", "--dry-run"}},
			} {
				// Each run reaches its preview, so the count below is not vacuous
				// (a flag error would send nothing either). auth login refuses
				// outright under CROWDSTRIKE_NO_CONFIG_WRITE, sending nothing. A
				// preview that prints the --stdin body is the kit's notice: the kit
				// holds every line of a --stdin body (credwire StdinFlags).
				out := e.runIn(t, c.stdin, c.args...)
				stdinPreview := c.stdin != "" && strings.HasPrefix(out, withheldNotice)
				if !strings.Contains(out, "dry_run") && !strings.Contains(out, "would ") && !strings.Contains(out, "dry run") && !stdinPreview &&
					!(mode == credPair && c.args[0] == "auth" && strings.Contains(out, "no credential may be written")) {
					t.Errorf("%v (%s) printed no preview:\n%.1500s", c.args, mode, out)
				}
				assertNoEcho(t, fmt.Sprintf("preview of %v", c.args), out)
			}
			if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
				t.Errorf("--dry-run (%s) sent %v and %d token request(s)", mode, methods, mints)
			}
			if memberMints, _, _ := sv.memberCounts(); memberMints != 0 {
				t.Errorf("--dry-run (%s) minted %d member_cid token(s)", mode, memberMints)
			}
			// Control: the same write without --dry-run reaches the fake.
			e.runIn(t, `{"ids":["`+bbHost+`"]}`, "devices", "perform-action-v2", "--action-name", "contain", "--stdin")
			if methods, _, _, _ := sv.counts(); len(methods) != 1 || methods[0] != "POST /devices/entities/devices-actions/v2" {
				t.Errorf("control (%s): want the one containment POST, got %v", mode, methods)
			}
			checkMode(t, sv, mode)
		})
	}
}

// per-cloud-token-url + api-origin-pinned + token-request-body-credentials, on
// the built binary: with only CROWDSTRIKE_BASE_URL set to a Falcon cloud other
// than US-1 (the fake stands for EU-1; no CROWDSTRIKE_TOKEN_URL), auth login,
// the automatic mint a read triggers, and fleet sync's member_cid mint all
// post the form to that cloud's /oauth2/token and nowhere else; a base URL
// carrying userinfo, or a plaintext one off loopback, posts nothing anywhere.
// Every non-loopback request the
// binary makes goes through a recording proxy (HTTPS_PROXY, HTTP_PROXY), which
// answers nothing, so a mint aimed at US-1 (api.crowdstrike.com) is caught
// without reaching the network.
func TestEveryMintStaysOnItsCloud(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var escaped []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		escaped = append(escaped, r.Method+" "+r.Host+r.URL.String())
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(proxy.Close)
	left := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), escaped...)
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credPair) // the pair in the environment; login must be allowed to save
	e.env = append(withoutEnv(e.env, "CROWDSTRIKE_NO_CONFIG_WRITE"),
		"HTTPS_PROXY="+proxy.URL, "HTTP_PROXY="+proxy.URL, "https_proxy="+proxy.URL, "http_proxy="+proxy.URL, "NO_PROXY=", "no_proxy=")
	out := e.run(t, "auth", "login")
	if l := left(); len(l) != 0 {
		t.Fatalf("auth login sent %d request(s) to another host: %v\n%.1500s", len(l), l, out)
	}
	_, mints, minted, bad := sv.counts()
	if mints != 1 || minted != 1 {
		t.Fatalf("auth login: want one accepted mint at the configured cloud, got %d requests, %d accepted (refused %v):\n%.1500s", mints, minted, bad, out)
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
	// The token it saved authenticates the next run against the same cloud
	// without another mint, and still nothing leaves.
	sv.reset()
	stored := withoutEnv(withoutEnv(e.env, "FALCON_CLIENT_ID"), "FALCON_CLIENT_SECRET")
	pairEnv := e.env
	e.env = stored
	// The fake echoes the token it was sent, so the record arrives as the kit's notice.
	if read := e.run(t, "devices", "combined-by-filter", "--json"); !strings.Contains(read, "HQ-Record") && !strings.Contains(read, withheldNotice) {
		t.Fatalf("the read after login fetched nothing:\n%.1500s", read)
	}
	if _, mints, _, _ := sv.counts(); mints != 0 {
		t.Errorf("the read after login minted %d time(s); the saved token was not used", mints)
	}
	if n, _ := sv.sawHeader("Bearer " + bbMintAccess); n == 0 {
		t.Errorf("the read after login did not carry the token login saved")
	}
	// The automatic mint (the environment pair the files do not hold mints its
	// own token) and fleet sync's member_cid mint post to the same cloud.
	sv.reset()
	e.env = append(append([]string{}, pairEnv...), "CROWDSTRIKE_NO_CONFIG_WRITE=1")
	read := e.run(t, "devices", "query-by-filter", "--json")
	fleet := e.run(t, "fleet", "sync", "--cids", bbChild, "--kinds", "hosts", "--json")
	_, mints, minted, bad = sv.counts()
	memberMints, memberOK, memberReads := sv.memberCounts()
	if mints != 1 || minted != 1 || memberMints != 1 || memberOK != 1 || len(memberReads) == 0 || len(bad) != 0 {
		t.Fatalf("want the read's mint and the member_cid mint accepted at the configured cloud and the child read with its token; got parent %d/%d, member_cid %d/%d, child reads %v, refused %v:\n%.1500s\n%.1500s",
			minted, mints, memberOK, memberMints, memberReads, bad, read, fleet)
	}
	// A base URL carrying userinfo names no token URL: nothing is posted, to
	// the fake or anywhere else.
	sv.reset()
	u, _ := url.Parse(sv.url)
	unsafe := "https://ro:unsafePw-5b2e@" + u.Host
	e.env = append(withoutEnv(append([]string{}, pairEnv...), "CROWDSTRIKE_BASE_URL"), "CROWDSTRIKE_BASE_URL="+unsafe)
	var unsafeOut string
	for _, args := range [][]string{{"auth", "login"}, {"devices", "query-by-filter", "--json"}, {"fleet", "sync", "--cids", bbChild, "--kinds", "hosts", "--json"}} {
		unsafeOut += e.run(t, args...)
	}
	methods, mints, _, _ := sv.counts()
	memberMints, _, _ = sv.memberCounts()
	if len(methods) != 0 || mints != 0 || memberMints != 0 {
		t.Errorf("with a base URL carrying userinfo the fake received %v and %d+%d token request(s):\n%.1500s", methods, mints, memberMints, unsafeOut)
	}
	if !strings.Contains(unsafeOut, "token URL") {
		t.Errorf("with a base URL carrying userinfo no command said why it sent nothing:\n%.1500s", unsafeOut)
	}
	if strings.Contains(unsafeOut, "unsafePw-5b2e") {
		t.Errorf("the base URL's password was printed:\n%.1500s", unsafeOut)
	}
	// A plaintext base URL off loopback (http://, the EU-1 host spelled
	// without TLS) may not carry the client secret either: every mint path
	// refuses before a byte leaves (the proxy would receive the form in clear).
	e.env = append(withoutEnv(append([]string{}, pairEnv...), "CROWDSTRIKE_BASE_URL"), "CROWDSTRIKE_BASE_URL=http://api.eu-1.crowdstrike.com")
	var plainOut string
	for _, args := range [][]string{{"auth", "login"}, {"devices", "query-by-filter", "--json"}, {"fleet", "sync", "--cids", bbChild, "--kinds", "hosts", "--json"}} {
		plainOut += e.run(t, args...)
	}
	assertNoEcho(t, "output", read+fleet+unsafeOut+plainOut)
	if l := left(); len(l) != 0 {
		t.Fatalf("a request left for another host: %v\n%.1500s", l, plainOut)
	}
}
