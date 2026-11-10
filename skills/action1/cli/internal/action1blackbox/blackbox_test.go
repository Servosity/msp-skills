// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package action1blackbox runs the built CLI and MCP binaries against an
// Action1 fake that echoes every credential it was sent back in every
// encoding it can, whole and split across lines and adjacent values, and
// checks what a reviewer cannot read off the code (action1 handfixes
// credkit-wiring, no-response-cache, config-file-holds-no-credential,
// api-origin-pinned, dry-run-sends-nothing, mcp-endpoint-write-confirm,
// client-credentials-mint). A unit that repeats a held credential is withheld
// whole by the shared credential kit, with its notice (kitNotice); the
// public-data controls read a quiet fake that echoes nothing (quietServer):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the static ACTION1_OAUTH2 token, the token stored
//     with auth set-token, the ACTION1_CLIENT_ID + ACTION1_CLIENT_SECRET pair,
//     the access and refresh tokens the fake's token endpoint mints for that
//     pair (fresh each run, or cached by an earlier run), the MCP HTTP token,
//     a credential-named flag value, and the credentials a response carries
//     under the fields Declarations.ResponseFields names (a password, an
//     access_token);
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     learn journal, config.toml), and the credential store never holds an
//     environment credential;
//   - no credential leaves for a host other than the configured one by
//     redirect, the token mint included;
//   - the MCP server sends a write only when the call sets confirm to the
//     boolean true, and --dry-run sends nothing at all (no mint either).
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the kit's decoder. The fake teaches the holder nothing: it echoes
// a credential only under names that are not declared response fields, so a
// value the program failed to hold from its real source (the environment, the
// store, the mint) is printed, and the scan fails.
// TestEchoTeachesTheHolderNothing proves it.
package action1blackbox

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
	bbToken        = "bbtok-kkwwzwmzqzzkkwwmwzmmqzmwwqmzkzzqq" // ACTION1_OAUTH2 (sent as Authorization: Bearer <v>)
	bbClientID     = "bbcid-kzqzmqzqmmkkqmmkzwzkmwk"           // ACTION1_CLIENT_ID (no api-key- form, so holdClientID holds it whole)
	bbClientSecret = "bbsec-kqqmqmmwwzqwqkqzkzwmmkkmqk"        // ACTION1_CLIENT_SECRET
	bbMintAccess   = "bbacc-wzkqkwmzzmqkwqqzmzwk"              // the access_token the fake's token endpoint mints for the pair
	bbMintRefresh  = "bbref-mwqzkzkkmqwwzqkmzzqw"              // the refresh_token it mints with it
	bbStored       = "bbsto-qmkwqkmqwzwkzwqqkzzmkkwq"          // auth set-token (stdin), the credential store
	bbArgvSec      = "-bbarg-wkqqmqwqkwmqq"                    // --client-secret on the command line (starts with "-")
	bbShortResp    = "Sh0rtPw"                                 // a 7-character credential a response carries
	bbBOMResp      = "bbbom-mzqzwwmwzkqz"                      // a credential in a BOM-prefixed response
	bbGzipResp     = "bbgzp-zkkmkwmwmmqqkkzz"                  // a credential in a gzip-encoded response
	bbHTTPToken    = "bbmht-mkzkzzkzzwmzmwkwzzmkqkkmzkkww"     // ACTION1_MCP_HTTP_TOKEN
	bbOrg          = "org-1042"                                // an organization id
	bbEndpoint     = "ep-7731"                                 // a managed endpoint id
	bbDenied       = "ep-denied"                               // an endpoint id the fake answers 401
	bbControl      = "bbctl-zqwmkqqzwmzkmm"                    // not a credential: the fake echoes it like one (TestEchoTeachesTheHolderNothing)
)

// secrets are every credential the suite gives the process that the credential
// leak contract holds: the declared secret environment, the client ID's unique
// part, the credential store, the minted tokens, the MCP HTTP token and the
// declared response fields; and the command-line value. Each one in any output,
// file or request off its origin fails the test.
func secrets() []string {
	return []string{bbToken, bbClientID, bbClientSecret, bbMintAccess, bbMintRefresh, bbStored, bbArgvSec,
		bbShortResp, bbBOMResp, bbGzipResp, bbHTTPToken}
}

// echoed is every value the fake echoes when a request carries it: the
// secrets and the control value.
func echoed() []string { return append(secrets(), bbControl) }

// kitNotice is the fixed notice the kit writes in place of a unit it
// withheld from sink (credkit.Notice).
func kitNotice(sink string) string {
	return "[credkit: withheld from " + sink + ": it carried a credential this program holds]"
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	// The tests wait on the CLI and MCP processes they start, so GOMAXPROCS
	// (go test's -parallel default) is not their concurrency: run twice as
	// many unless -parallel was given. Serial, the package passed go test's
	// 10 minute limit on the Linux and Windows CI rows (PR #598).
	explicit := false
	flag.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "test.parallel" })
	if !explicit {
		_ = flag.Set("test.parallel", fmt.Sprint(max(8, 2*runtime.GOMAXPROCS(0))))
	}
	dir, err := os.MkdirTemp("", "action1-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "action1-cli"+exe)
	bbMCP = filepath.Join(dir, "action1-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/action1-cli", bbMCP: "../../cmd/action1-mcp"} {
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
// auth-scheme form ("Bearer <s>"): the raw form already finds a printed
// "Bearer <s>".
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
	url     string
	caFile  string
	mu      sync.Mutex
	offHost int
	methods []string       // API requests but the token endpoint, "METHOD /path"
	headers map[string]int // Authorization header values those requests carried
	mints   int            // token endpoint requests (POST <base>/oauth2/token)
	minted  int            // ... that the fake answered with the minted pair
	badMint []string       // ... the fake refused, and why
	// redirectOrganizations sends the organizations list (sync's root) to
	// the other host too, so sync meets a redirect.
	redirectOrganizations bool
	// redirectToken sends the token endpoint to the other host (a 307 would
	// resend the posted client secret).
	redirectToken bool
	// echoOnly, when set, is all the fake echoes of what a request carried
	// (TestEchoTeachesTheHolderNothing).
	echoOnly []string
}

// setEchoOnly sets echoOnly while the fake is running.
func (sv *server) setEchoOnly(vals []string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.echoOnly = vals
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

// Records shaped so the store, the fleet views (patch-posture, vuln-triage,
// stale, health-score, reboot-pending, software-rollup, automation-health,
// org-scorecard, patch-drift) and the exports have something real to read.
const (
	orgRecord      = `"id":"org-1042","name":"HQ-Record","description":"HQ organization","status":"active","created_at":"2026-10-20T10:00:00Z"`
	endpointRecord = `"id":"ep-7731","name":"HQ-Endpoint","organization_id":"org-1042","OS":"Windows 11","status":"Connected","online_status":"SUCCESS","reboot_required":"Yes","last_seen":"2026-10-21T10:00:00Z","missing_updates":{"critical":2,"other":1}`
	vulnRecord     = `"id":"CVE-2026-1042","cve_id":"CVE-2026-1042","name":"HQ-CVE","cvss_score":"9.8","cisa_kev":"Yes","endpoints_count":3,"organization_id":"org-1042"`
	appRecord      = `"id":"app-1042","name":"HQ-App","version":"120.0","organization_id":"org-1042"`
	instanceRecord = `"id":"inst-1042","name":"HQ-Automation","status":"Failed","percent_completed":50,"organization_id":"org-1042"`
)

// page wraps items the way Action1 answers a list: a ResultPage.
func page(items ...string) string {
	return `{"type":"ResultPage","items":[` + strings.Join(items, ",") + `],"total_items":` + fmt.Sprint(len(items)) + `,"next_page":"","prev_page":""}`
}

func echoServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, false)
}

// quietServer is the fake answering as Action1 does: the same records,
// repeating nothing a request carried. The kit withholds a unit whole
// (credential leak contract section 4, no partial redaction), so an answer
// that echoes the held token never prints or persists any of its public data;
// the quiet fake is what proves a sink really writes.
func quietServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, true)
}

func fakeServer(t *testing.T, quiet bool) *server {
	t.Helper()
	sv := &server{headers: map[string]int{}}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(page()))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sent := sentValues(r, body)
		if quiet {
			sent = nil
		}
		sv.mu.Lock()
		if sv.echoOnly != nil {
			sent = slices.DeleteFunc(sent, func(v string) bool { return !slices.Contains(sv.echoOnly, v) })
		}
		sv.mu.Unlock()
		echo := echoObject(sent)
		p := strings.TrimPrefix(r.URL.Path, "/api/3.0")
		get := r.Method == http.MethodGet
		w.Header().Set("Content-Type", "application/json")
		if p == "/oauth2/token" && r.Method == http.MethodPost {
			sv.token(w, r, body, other.URL)
			return
		}
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		if h := r.Header.Get("Authorization"); h != "" {
			sv.headers[h]++
		}
		redirectOrganizations := sv.redirectOrganizations
		sv.mu.Unlock()
		echoID := ""
		if len(sent) > 0 {
			// A record whose identifier or name repeats the first value it was sent.
			echoID = fmt.Sprintf(`{"id":%q,"name":"Echo-ID","organization_id":"org-1042"}`, sent[0])
		}
		switch {
		case p == "/users" || strings.Contains(p, "/"+bbDenied):
			// Rejected: the error repeats what it was sent.
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"status":401,"error":"invalid request %s","message":"bad request %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case strings.HasPrefix(p, "/logs/") || (redirectOrganizations && p == "/organizations"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && strings.HasPrefix(p, "/endpoints/managed/") && strings.Count(p, "/") == 4:
			// endpoints managed-id: the echo split across values, lines and spaces.
			fmt.Fprintf(w, `{%s,"split":%s}`, endpointRecord, splitObject(sent))
		case get && strings.HasPrefix(p, "/endpoints/managed/"):
			// endpoints managed and the sync fan-out: the endpoint, the echo,
			// and an endpoint whose id repeats the value it was sent (the fleet
			// views print endpoint ids and names).
			items := []string{fmt.Sprintf(`{%s,"echo":%s}`, endpointRecord, echo)}
			if echoID != "" {
				items = append(items, echoID)
			}
			fmt.Fprint(w, page(items...))
		case get && p == "/me" && quiet:
			fmt.Fprint(w, `{"id":"user-1","email":"hq@example.com","name":"HQ-Me"}`)
		case get && p == "/me":
			// me: BOM-prefixed, with a password.
			fmt.Fprint(w, "\xef\xbb\xbf"+fmt.Sprintf(`{"id":"user-1","email":"hq@example.com","name":"HQ-Me","note":%q,"password":%q}`, bbBOMResp, bbBOMResp))
		case get && p == "/enterprise" && quiet:
			fmt.Fprint(w, `{"id":"ent-1","name":"HQ-Enterprise","type":"Enterprise"}`)
		case get && p == "/enterprise":
			// enterprise get: a short credential.
			fmt.Fprintf(w, `{"id":"ent-1","name":"HQ-Enterprise","type":"Enterprise","note":%q,"password":%q}`, bbShortResp, bbShortResp)
		case get && p == "/organizations":
			// organizations get, sync, export: gzip-encoded, with an access
			// token, the echo, and a record whose identifier repeats the value
			// it was sent.
			items := []string{fmt.Sprintf(`{%s,"access_token":%q,"echo":%s}`, orgRecord, bbGzipResp, echoObject(append(sent, bbGzipResp)))}
			if quiet {
				items = []string{"{" + orgRecord + "}"}
			}
			if echoID != "" {
				items = append(items, echoID)
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprint(zw, page(items...))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case get && strings.HasPrefix(p, "/vulnerabilities/"):
			fmt.Fprint(w, page(fmt.Sprintf(`{%s,"echo":%s}`, vulnRecord, echo)))
		case get && strings.HasPrefix(p, "/installed-software/"):
			fmt.Fprint(w, page(fmt.Sprintf(`{%s,"echo":%s}`, appRecord, echo)))
		case get && strings.HasPrefix(p, "/automations/instances/"):
			fmt.Fprint(w, page(fmt.Sprintf(`{%s,"echo":%s}`, instanceRecord, echo)))
		case !get:
			// Every write echoes what it was sent.
			fmt.Fprintf(w, `{%s,"echo":%s}`, orgRecord, echo)
		default:
			fmt.Fprint(w, page(fmt.Sprintf(`{"id":"rec-1","name":"HQ-Other","echo":%s}`, echo)))
		}
	}))
	sv.url = srv.URL + "/api/3.0"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// token emulates Action1's token endpoint: POST <base>/oauth2/token on the
// API origin, a JSON body {"client_id","client_secret"} (refresh:
// {"client_id","refresh_token","grant_type":"refresh_token"}), answered with
// {"access_token","refresh_token","expires_in","token_type"}. It answers only
// the configured pair, and never echoes a request on success. A refusal
// repeats what it was sent under names that are not credentials.
func (sv *server) token(w http.ResponseWriter, r *http.Request, body []byte, otherURL string) {
	sv.mu.Lock()
	sv.mints++
	redirect := sv.redirectToken
	sv.mu.Unlock()
	if redirect {
		http.Redirect(w, r, otherURL+r.URL.Path, http.StatusTemporaryRedirect)
		return
	}
	var req map[string]any
	why := ""
	switch {
	case !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json"):
		why = "content type " + r.Header.Get("Content-Type")
	case json.Unmarshal(body, &req) != nil:
		why = "body is not JSON"
	case req["client_id"] != bbClientID:
		why = "unknown client"
	case req["grant_type"] == "refresh_token" && req["refresh_token"] != bbMintRefresh:
		why = "unknown refresh token"
	case req["grant_type"] != "refresh_token" && req["client_secret"] != bbClientSecret:
		why = "wrong client secret"
	}
	sv.mu.Lock()
	if why != "" {
		sv.badMint = append(sv.badMint, why)
	} else {
		sv.minted++
	}
	sv.mu.Unlock()
	if why != "" {
		sent := sentValues(r, body)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"invalid_client","error_description":"rejected %s","echo":%s}`, strings.Join(sent, " "), echoObject(sent))
		return
	}
	fmt.Fprintf(w, `{"access_token":%q,"refresh_token":%q,"expires_in":3600,"token_type":"Bearer"}`, bbMintAccess, bbMintRefresh)
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
	sv.methods, sv.headers, sv.mints, sv.minted, sv.badMint, sv.offHost = nil, map[string]int{}, 0, 0, nil, 0
}

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its Authorization.
type credMode int

const (
	credStored     credMode = iota // none in the environment: the battery runs on a token stored with auth set-token
	credEnvToken                   // ACTION1_OAUTH2 (a static bearer token)
	credEnvBoth                    // ACTION1_OAUTH2 and the client pair: the static token wins, the pair is held unused and never mints
	credPair                       // the client pair, ACTION1_NO_CONFIG_WRITE=1: every process mints its own token, nothing is cached
	credPairCached                 // the client pair stored in the credential store, and the token an earlier run minted for it cached beside it
)

func (m credMode) String() string {
	return [...]string{"stored token", "env token", "env token and pair", "client pair minted", "client pair cached token"}[m]
}

// header is the Authorization value every API request must carry in mode m.
func (m credMode) header() string {
	switch m {
	case credStored:
		return "Bearer " + bbStored
	case credEnvToken, credEnvBoth:
		return "Bearer " + bbToken
	default:
		return "Bearer " + bbMintAccess
	}
}

// held is a credential the process holds from its start in mode m (the one a
// test types on the command line or into a URL).
func (m credMode) held() string {
	switch m {
	case credStored:
		return bbStored
	case credEnvToken, credEnvBoth:
		return bbToken
	case credPair:
		return bbClientSecret
	default:
		return bbMintAccess // loaded from the credential store
	}
}

// newEnv builds a clean environment against the fake with the credentials
// mode names. Every user directory (ACTION1_HOME, HOME, the XDG and Windows
// dirs, TMPDIR) is inside a fresh t.TempDir. credPairCached first runs one
// read so the client pair mints a token and caches it in the credential
// store, then clears the fake's counters.
func newEnv(t *testing.T, sv *server, mode credMode) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "ACTION1_HOME=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"ACTION1_BASE_URL=" + sv.url, "ACTION1_CA_CERT=" + sv.caFile,
		"ACTION1_CLI_PATH=" + bbCLI, "ACTION1_MCP_HTTP_TOKEN=" + bbHTTPToken,
		"ACTION1_ORG_ID=" + bbOrg, // a default org filter, not a credential
	}
	switch mode {
	case credEnvToken:
		env = append(env, "ACTION1_OAUTH2="+bbToken)
	case credEnvBoth:
		env = append(env, "ACTION1_OAUTH2="+bbToken, "ACTION1_CLIENT_ID="+bbClientID, "ACTION1_CLIENT_SECRET="+bbClientSecret)
	case credPair:
		env = append(env, "ACTION1_CLIENT_ID="+bbClientID, "ACTION1_CLIENT_SECRET="+bbClientSecret, "ACTION1_NO_CONFIG_WRITE=1")
	case credPairCached:
		// A token is cached only beside the pair it was minted for: the pair
		// lives in the credential store (a token minted for an environment
		// pair is kept in its process, minted-token-bound-to-its-client).
		store := filepath.Join(home, "data", "credentials.toml")
		if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
			t.Fatal(err)
		}
		pair := "base_url = " + strconv.Quote(sv.url) + "\nclient_id = " + strconv.Quote(bbClientID) + "\nclient_secret = " + strconv.Quote(bbClientSecret) + "\n"
		if err := os.WriteFile(store, []byte(pair), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e := &bbEnv{home: home, env: env}
	if mode == credPairCached {
		out := e.run(t, "organizations", "get", "--json")
		if _, mints, minted, bad := sv.counts(); mints != 1 || minted != 1 {
			t.Fatalf("priming the cached token: want one accepted mint, got %d requests, %d accepted (refused: %v):\n%.1500s", mints, minted, bad, out)
		}
		if !e.credentialStoreHolds(bbMintAccess) {
			t.Fatalf("priming the cached token: the minted token was not cached in the credential store:\n%.1500s", out)
		}
		sv.reset()
	}
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

// importFile writes an import input: one organization whose name repeats
// tok, so the create request carries it and the fake echoes it. It lives
// outside the home: it is the test's input, not a file the program wrote.
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "orgs.jsonl")
	if err := os.WriteFile(p, []byte(`{"name":"Imported `+tok+`","description":"d"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// tokenBody is the oauth2 command's body: the client pair, typed on the
// command line.
const tokenBody = `{"client_id":"` + bbClientID + `","client_secret":"` + bbClientSecret + `"}`

// battery runs every output path; tok is a credential the process holds
// (credMode.held).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) [][]string {
	orgs := e.importFile(t, tok)
	return [][]string{
		// Every output mode.
		{"organizations", "get", "--json"},
		{"organizations", "get"},
		{"organizations", "get", "--csv"},
		{"organizations", "get", "--plain"},
		{"organizations", "get", "--json", "--receipt"},
		{"organizations", "get", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"organizations", "get", "--json", "--deliver", "file:" + deliverFile},
		// Every response shape: BOM-prefixed, short credential, split echo,
		// 401 echo, refused redirect, an id that repeats a held value.
		{"me", "me", "--json"},
		{"enterprise", "get", "--json"},
		{"endpoints", "managed-id", bbOrg, bbEndpoint, "--json"},
		{"users", "get", "--json"},
		{"logs", bbOrg, "--json"},
		{"endpoints", "managed", bbOrg, "--json"},
		// Writes the fake echoes, a rejected write, the token endpoint as a
		// command (the posted pair and the minted response), and import.
		{"organizations", "org-id-patch", bbOrg, "--name", "HQ", "--json"},
		{"endpoints", "managed-id-patch", bbOrg, bbEndpoint, "--comment", "HQ " + tok, "--json"},
		{"endpoints", "managed-id-patch", bbOrg, bbDenied, "--comment", "HQ", "--json"},
		{"oauth2", "--body-json", tokenBody, "--json"},
		{"import", "organizations", "--input", orgs, "--json"},
		// The local store, exports and the nine novel fleet views.
		{"sync", "--json"},
		{"export", "organizations", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"export", "organizations", "--format", "json"},
		{"fleet", "patch-posture", "--json"},
		{"fleet", "vuln-triage", "--json"},
		{"fleet", "stale", "--days", "1", "--json"},
		{"fleet", "patch-drift", "--json"},
		{"fleet", "software-rollup", "--json"},
		{"fleet", "health-score", "--json"},
		{"fleet", "automation-health", "--json"},
		{"fleet", "org-scorecard", "--json"},
		{"fleet", "reboot-pending", "--json"},
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
		// A credential on the command line: a credential-named flag, an unknown
		// flag, a positional, feedback text.
		{"organizations", "get", "--client-secret", bbArgvSec},
		{"organizations", "get", "--json", "--" + tok},
		{"endpoints", "managed-id", bbOrg, tok, "--json"},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Previews: a dry run sends nothing.
		{"organizations", "org-id-patch", bbOrg, "--name", "Echo " + tok, "--dry-run"},
		{"organizations", "post", "--name", "Echo " + tok, "--description", "d", "--dry-run"},
		{"oauth2", "--body-json", tokenBody, "--dry-run"},
		{"import", "organizations", "--input", orgs, "--dry-run"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find organizations " + tok, "--resource", bbOrg, "--resource-type", "organizations",
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
		return map[string]any{"name": "action1_read", "arguments": args}
	}
	none := map[string]any{}
	return []map[string]any{
		read("organizations.get", nil),
		read("me.me", nil),
		read("enterprise.get", nil),
		read("endpoints.managed-id", map[string]any{"orgId": bbOrg, "endpointId": bbEndpoint}),
		read("endpoints.managed", map[string]any{"orgId": bbOrg}),
		read("users.get", nil),
		{"name": "action1_search", "arguments": map[string]any{"query": "organizations"}},
		{"name": "action1_get", "arguments": map[string]any{"endpoint_id": "organizations.get"}},
		{"name": "fleet_patch_posture", "arguments": none},
		{"name": "fleet_vuln_triage", "arguments": none},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// mcpWrites is every kind of write tool, called without confirm: each must
// answer with a preview and send nothing. action1_execute previews as a
// confirmation_required object; the mirrored import previews as the
// command's --dry-run plan followed by the gate's "Preview only: nothing was
// sent" note.
func mcpWrites() []map[string]any {
	exec := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "action1_execute", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		exec("organizations.org-id-patch", map[string]any{"orgId": bbOrg, "name": "Preview Co"}),
		exec("organizations.post", map[string]any{"name": "Preview Co", "description": "d"}),
		exec("organizations.org-id-delete", map[string]any{"orgId": bbOrg}),
		exec("endpoints.managed-id-patch", map[string]any{"orgId": bbOrg, "endpointId": bbEndpoint, "comment": "Preview"}),
		exec("endpoints.managed-id-delete", map[string]any{"orgId": bbOrg, "endpointId": bbEndpoint}),
		exec("oauth2.token", map[string]any{"client_id": bbClientID, "client_secret": bbClientSecret}),
		{"name": "import", "arguments": map[string]any{"resource": "organizations"}},
	}
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
}

// checkMode asserts the fake saw the credential mode the test set up: every
// API request carried the mode's header and no other, and the token endpoint
// was used exactly as the mode says (never for a static or stored token or a
// cached one; for every process in the minted mode, with no refusal).
func checkMode(t *testing.T, sv *server, mode credMode) {
	t.Helper()
	n, all := sv.sawHeader(mode.header())
	if n == 0 || len(all) != 1 {
		t.Errorf("%s: want every API request to carry %q, got %d of it and %d distinct values", mode, mode.header(), n, len(all))
	}
	_, mints, minted, bad := sv.counts()
	switch mode {
	case credPair:
		if minted == 0 || len(bad) != 0 {
			t.Errorf("%s: want the pair to mint and no refused mint, got %d accepted, refused %v", mode, minted, bad)
		}
	default:
		// The battery's own oauth2 command posts the pair once; nothing else may mint.
		if mints > 1 || len(bad) != 0 {
			t.Errorf("%s: want no mint but the oauth2 command's, got %d token requests (refused %v)", mode, mints, bad)
		}
	}
}

// quietEnv is e pointed at the quiet fake, in the same home and with the same
// credentials.
func (e *bbEnv) quietEnv(quiet *server) *bbEnv {
	return &bbEnv{home: e.home, env: append(withoutEnv(withoutEnv(e.env, "ACTION1_BASE_URL"), "ACTION1_CA_CERT"),
		"ACTION1_BASE_URL="+quiet.url, "ACTION1_CA_CERT="+quiet.caFile)}
}

// Class credential-persisted-in-files, in every credential mode.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credStored, credEnvBoth, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			if mode == credStored {
				e.runIn(t, bbStored+"\n", "auth", "set-token")
			}
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, args := range battery(t, e, deliverFile, mode.held()) {
				e.run(t, args...)
			}
			e.mcpCalls(t, mcpBattery())
			checkMode(t, sv, mode)
			if _, _, minted, _ := sv.counts(); mode == credPair && minted < 5 {
				// ACTION1_NO_CONFIG_WRITE: no process can reuse another's token.
				t.Errorf("%s: only %d mints across the battery; a token was reused from disk", mode, minted)
			}
			// Controls without a credential: the same sinks write, so the scans
			// below are not vacuous. Every battery answer echoes the held token,
			// so the kit withheld each of its store, delivery and export writes
			// whole (credential leak contract section 4, no partial redaction);
			// the data controls read the quiet fake instead, into the same home,
			// and into files of their own, so the battery's files stay scanned.
			qe := e.quietEnv(quietServer(t))
			controlDeliver := filepath.Join(e.home, "deliver-control.json")
			controlExport := filepath.Join(e.home, "export-control.jsonl")
			qe.run(t, "sync", "--json")
			qe.run(t, "organizations", "get", "--json", "--deliver", "file:"+controlDeliver)
			qe.run(t, "export", "organizations", "--format", "jsonl", "--output", controlExport)
			e.run(t, "teach", "--query", "find organizations control", "--resource", bbOrg, "--resource-type", "organizations",
				"--notes", "rotated control", "--no-validate")
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
					// credential, the token it was given (set-token or a
					// mint), never an environment credential or one a
					// response or the command line carried.
					sawCreds = sawCreds || bytes.Contains(data, []byte(bbStored)) || bytes.Contains(data, []byte(bbMintAccess))
					if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
						t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
					}
					never := []string{bbToken, bbClientID, bbClientSecret, bbArgvSec, bbShortResp, bbBOMResp, bbGzipResp, bbHTTPToken}
					if mode == credPairCached {
						never = never[3:] // the pair was stored there (newEnv), beside the token minted for it
						never = append(never, bbToken)
					}
					for _, s := range never {
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
			switch mode {
			case credStored, credPairCached:
				if !sawCreds {
					t.Errorf("no credential store holds the %s", mode)
				}
			default:
				// An environment credential is never persisted, and
				// ACTION1_NO_CONFIG_WRITE keeps a minted token off disk.
				if sawCreds || e.credentialStoreHolds(bbMintRefresh) {
					t.Errorf("%s: a credential store holds a token", mode)
				}
			}
			// The battery's delivery is an answer that echoes the held token, so
			// the kit refused that write whole (credkit.WriteFile writes nothing);
			// a file there would have been scanned above. The controls really
			// wrote data, so the sinks the scan covers work.
			if b, err := os.ReadFile(controlDeliver); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
				t.Errorf("%s: want the delivered record (err %v):\n%.1500s", filepath.Base(controlDeliver), err, b)
			}
			if b, err := os.ReadFile(controlExport); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
				t.Errorf("%s: want the exported record (err %v):\n%.1500s", filepath.Base(controlExport), err, b)
			}
			var sawStore, sawFanOut, sawLearning bool
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".db") {
					if b, rerr := os.ReadFile(path); rerr == nil {
						sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
						// sync walked the organizations into their endpoints.
						sawFanOut = sawFanOut || bytes.Contains(b, []byte("HQ-Endpoint"))
						// The control teach run's learning row was written.
						sawLearning = sawLearning || bytes.Contains(b, []byte("rotated control"))
					}
				}
				return nil
			})
			if !sawStore || !sawFanOut {
				t.Errorf("no local store holds the fetched records (organization %v, endpoint %v)", sawStore, sawFanOut)
			}
			if !sawLearning {
				t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
			}
		})
	}
}

// Class secret-in-output, with the static token (the client pair set too, and
// unused: ACTION1_OAUTH2 wins), with the client pair minting in every
// process, and with the client pair on a cached minted token.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credEnvBoth, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, args := range battery(t, e, deliverFile, mode.held()) {
				assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
			}
			// Every answer here echoes the held token, so the kit withholds
			// what carries it whole (credential leak contract section 4: no
			// partial redaction) and says so with its notice: the scans above
			// were not vacuous.
			if out := e.run(t, "organizations", "get", "--json"); !strings.Contains(out, kitNotice("stdout")) {
				t.Fatalf("organizations get --json: want the kit's stdout notice for the echoed token, got:\n%.1500s", out)
			}
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			if !strings.Contains(out, kitNotice("mcp")) {
				t.Fatalf("the MCP session withheld nothing; the echoes did not reach it:\n%.2000s", out)
			}
			// The fake echoed the header it really received, so the scans above
			// covered it; no other credential source was sent.
			checkMode(t, sv, mode)
			// The same reads print their data, as valid JSON, from the quiet
			// fake, and the fleet views print what sync wrote.
			qe := e.quietEnv(quietServer(t))
			cmd := exec.Command(bbCLI, "organizations", "get", "--json")
			cmd.Env, cmd.Dir = qe.env, qe.home
			stdout, err := cmd.Output()
			if err != nil {
				t.Fatalf("organizations get --json (quiet fake): %v", err)
			}
			var doc any
			if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
				t.Fatalf("quiet fake: want valid JSON with the record, got:\n%.1500s", stdout)
			}
			qe.run(t, "sync", "--json")
			// The same home still holds what the echoing fake returned (synccpu:
			// the local store is not checked), so a fleet view may be withheld
			// whole, and it carries no credential either way.
			fleet := qe.run(t, "fleet", "patch-posture", "--json")
			assertNoEcho(t, "quiet fleet patch-posture", fleet)
			if !strings.Contains(fleet, "HQ-Endpoint") && !strings.Contains(fleet, bbEndpoint) && !strings.Contains(fleet, kitNotice("stdout")) {
				t.Errorf("quiet fake: fleet patch-posture printed no synced endpoint:\n%.1500s", fleet)
			}
			if qout := qe.mcpCalls(t, mcpReads()); !strings.Contains(qout, "HQ-Record") {
				t.Fatalf("quiet fake: MCP results lost the record itself:\n%.2000s", qout)
			}
		})
	}
}

// The oracle is not vacuous: the fake echoes a value that is not a credential
// (bbControl, sent as an organization id) in exactly the shapes it echoes the
// credentials it is sent, and the output prints it, so the echo shapes teach
// the holder nothing. The same answers echoing the held token beside it are
// withheld whole.
func TestEchoTeachesTheHolderNothing(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credEnvToken, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			echoes := func() string {
				out := e.run(t, "endpoints", "managed", bbControl, "--json")
				out += e.run(t, "endpoints", "managed-id", bbControl, bbEndpoint, "--json")
				return out + e.mcpCalls(t, []map[string]any{{"name": "action1_read", "arguments": map[string]any{
					"endpoint_id": "endpoints.managed", "params": map[string]any{"orgId": bbControl}}}})
			}
			sv.setEchoOnly([]string{bbControl})
			out := echoes()
			for _, f := range echoForms(bbControl) {
				if !holdsForm(out, f) {
					t.Errorf("the echo of the control value as %q was withheld or never sent: the fake teaches the holder, so the credential scans are vacuous:\n%.2000s", f, out)
				}
			}
			if a, b := halves(bbControl); !strings.Contains(out, a+" "+b) {
				t.Errorf("the split echo of the control value was withheld:\n%.2000s", out)
			}
			assertNoEcho(t, "output", out)
			// The same shapes, now also echoing the Authorization header the
			// request carried: withheld whole, with the notice.
			sv.setEchoOnly(nil)
			out = echoes()
			if !strings.Contains(out, kitNotice("stdout")) || !strings.Contains(out, kitNotice("mcp")) {
				t.Errorf("the echo of the held token was not withheld with the kit's notice:\n%.2000s", out)
			}
			assertNoEcho(t, "output", out)
			checkMode(t, sv, mode)
		})
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before it is followed, from the CLI, sync and MCP alike, and
// in the client-credentials mode a redirected token endpoint never receives
// the posted secret either.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	t.Run("API requests", func(t *testing.T) {
		for _, mode := range []credMode{credEnvToken, credPair} {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			redirect := e.run(t, "logs", bbOrg, "--json")
			sv.mu.Lock()
			sv.redirectOrganizations = true
			sv.mu.Unlock()
			syncOut := e.run(t, "sync", "--resources", "organizations", "--json")
			e.mcpCalls(t, []map[string]any{
				{"name": "action1_read", "arguments": map[string]any{"endpoint_id": "logs.org-id-get", "params": map[string]any{"orgId": bbOrg}}},
				{"name": "action1_read", "arguments": map[string]any{"endpoint_id": "organizations.get"}},
			})
			sv.mu.Lock()
			n := sv.offHost
			var redirected int
			for _, m := range sv.methods {
				if strings.Contains(m, "/logs/") || strings.HasSuffix(m, "/organizations") {
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
			for where, out := range map[string]string{"logs": redirect, "sync": syncOut} {
				if !strings.Contains(out, "leave the configured Action1 host") {
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
		out := e.run(t, "organizations", "get", "--json")
		out += e.mcpCalls(t, []map[string]any{{"name": "action1_read", "arguments": map[string]any{"endpoint_id": "organizations.get"}}})
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
		if !strings.Contains(out, "leave the configured Action1 host") && !strings.Contains(out, "returned HTTP 307") {
			t.Fatalf("want the redirect refused, got:\n%.1500s", out)
		}
		assertNoEcho(t, "output", out)
	})
}

// A rejected mint (a wrong secret, the token endpoint's refusal echoing what
// it was posted) returns promptly, names the variables, and prints nothing it
// was posted, from the CLI and from MCP.
func TestRejectedMintPrintsNoSecret(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credPair)
	const wrong = "bbwrong-zmqkwzzqkmwqzk"
	e.env = append(withoutEnv(e.env, "ACTION1_CLIENT_SECRET"), "ACTION1_CLIENT_SECRET="+wrong)
	out := e.run(t, "organizations", "get", "--json")
	out += e.run(t, "sync", "--resources", "organizations", "--json")
	out += e.mcpCalls(t, []map[string]any{{"name": "action1_read", "arguments": map[string]any{"endpoint_id": "organizations.get"}}})
	_, mints, minted, bad := sv.counts()
	if mints == 0 || minted != 0 || len(bad) == 0 {
		t.Fatalf("want only refused mints, got %d requests, %d accepted (refused %v)", mints, minted, bad)
	}
	if !strings.Contains(out, "ACTION1_CLIENT_SECRET") {
		t.Errorf("the refusal does not name the variables:\n%.2000s", out)
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
	t.Parallel()
	for _, mode := range []credMode{credEnvToken, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			// The quiet fake: the reads must answer their data, which an answer
			// that echoes the held token never does (withheld whole by contract).
			sv := quietServer(t)
			e := newEnv(t, sv, mode)
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, "HQ-Record") {
				t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
			}
			replies := mcpReplies(out)
			for i, call := range mcpWrites() {
				id := len(mcpReads()) + i + 1
				r := replies[id]
				// The token call's arguments carry the client secret, which the kit
				// holds from the call (contract section 1, MCP arguments), so its
				// preview, repeating them, is withheld whole; the methods check
				// below still proves it sent nothing.
				if call["arguments"].(map[string]any)["endpoint_id"] == "oauth2.token" && strings.Contains(r, kitNotice("mcp")) {
					continue
				}
				if !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") {
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
	t.Parallel()
	for _, mode := range []credMode{credEnvToken, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			update := func(confirm any) []map[string]any {
				return []map[string]any{{"name": "action1_execute", "arguments": map[string]any{
					"endpoint_id": "organizations.org-id-patch", "params": map[string]any{"orgId": bbOrg, "name": "HQ map"}, "confirm": confirm}}}
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
				sent = sent || m == "PATCH /api/3.0/organizations/"+bbOrg
			}
			if !sent {
				t.Fatalf("confirm true did not send the PATCH; requests: %v", methods)
			}
			checkMode(t, sv, mode)
		})
	}
}

// Finding (.review/kit/test-helper-findings.md): with only the static token
// in the environment, the client secret an operator types into the token
// request's body is held by nothing, and the --dry-run preview prints it. The
// request it previews would hold it (contract section 1: JSON credential fields
// of an outgoing request; Declarations.CredentialFields names client_secret).
// The client ID beside it is an identifier (Declarations.Identifiers), so only
// the secret is checked. The control is the same preview with the pair in the
// environment, where the kit already holds it.
func TestDryRunPreviewWithholdsATypedClientSecret(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvToken)
	out := e.run(t, "oauth2", "--body-json", tokenBody, "--dry-run")
	if !strings.Contains(out, "dry run") && !strings.Contains(out, "dry_run") {
		t.Fatalf("the dry run printed no preview, so this test proves nothing:\n%.1500s", out)
	}
	for i := 0; i+8 <= len(bbClientSecret); i++ {
		if strings.Contains(out, bbClientSecret[i:i+8]) {
			t.Errorf("the --dry-run preview prints the typed client secret (piece %q):\n%.1500s", bbClientSecret[i:i+8], out)
			break
		}
	}
	if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
		t.Errorf("--dry-run sent %v and %d token request(s)", methods, mints)
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, and never
// mints, from a cold start with the static token, the client pair, or the
// client pair on a cached token.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credEnvToken, credPair, credPairCached} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			orgs := e.importFile(t, "Dry")
			for _, args := range [][]string{
				{"organizations", "org-id-patch", bbOrg, "--name", "Dry Co", "--dry-run"},
				{"organizations", "post", "--name", "Dry Co", "--description", "d", "--dry-run"},
				{"organizations", "org-id-delete", bbOrg, "--dry-run"},
				{"endpoints", "managed-id-patch", bbOrg, bbEndpoint, "--comment", "Dry", "--dry-run"},
				{"endpoints", "managed-id-delete", bbOrg, bbEndpoint, "--dry-run"},
				{"oauth2", "--body-json", tokenBody, "--dry-run"},
				{"import", "organizations", "--input", orgs, "--dry-run"},
			} {
				// Each run reaches its preview, so the count below is not vacuous
				// (a flag error would send nothing either).
				out := e.run(t, args...)
				if !strings.Contains(out, "dry_run") && !strings.Contains(out, "would ") && !strings.Contains(out, "dry run") {
					t.Errorf("%v (%s) printed no preview:\n%.1500s", args, mode, out)
				}
				if mode == credEnvToken && args[0] == "oauth2" {
					// The pair typed into the body is held by nothing in this mode:
					// TestDryRunPreviewWithholdsATypedClientSecret (a recorded
					// finding) owns this preview's leak check.
					continue
				}
				assertNoEcho(t, fmt.Sprintf("preview of %v", args), out)
			}
			if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
				t.Errorf("--dry-run (%s) sent %v and %d token request(s)", mode, methods, mints)
			}
			// Control: the same write without --dry-run reaches the fake.
			e.run(t, "organizations", "org-id-patch", bbOrg, "--name", "Dry Co")
			if methods, _, _, _ := sv.counts(); len(methods) != 1 || methods[0] != "PATCH /api/3.0/organizations/"+bbOrg {
				t.Errorf("control (%s): want the one PATCH, got %v", mode, methods)
			}
			checkMode(t, sv, mode)
		})
	}
}
