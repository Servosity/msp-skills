// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package rocketcyberblackbox runs the built CLI and MCP binaries against an
// in-process RocketCyber fake and checks what a reviewer cannot read off the
// code: that the shared credential kit is wired into every sink (rocketcyber
// handfixes credkit-wiring, rocketcyber-blackbox-suite,
// mcp-endpoint-write-confirm, mcp-mirror-write-confirm), judged by
// docs/credential-leak-contract.md.
//
// The fake is the RocketCyber Customer API v3 on one origin
// (ROCKETCYBER_BASE_URL points the CLI at <fake>/v3), authenticated with the
// one header Authorization: Bearer <ROCKETCYBER_API_TOKEN>. The API is
// read-only (GET only; anything else answers 405). Its lists answer
// {"totalCount":N,"currentPage":P,"totalPages":T,"dataCount":K,"data":[...]},
// paged by the 1-based page and pageSize (the vendor's documented default and
// maximum are 1000); /account, /defender, /office, /events/summary and
// /suppression/rule/{ruleId} answer objects, and /reportApi answers a CSV
// report. The record shapes are the vendor's documented samples
// (research rocketcyber-postman-collection.json, rocketcyber-endpoints-detail.txt).
//
// RocketCyber documents no credential in any answer. The fake echoes every
// credential the process holds back in every encoding it can, whole and
// split across lines, adjacent values, members, records and pages, in a
// field value, an error body, a response header, a CSV report cell and an
// identifier: what the request carried (the API token from any source, a
// configured header value). It also answers generic credential-named members
// (every agent's password in a gzip-encoded list, an incident's api_key and
// credentials container in a BOM-prefixed list, a firewall's secret and
// token, an app's access_token, an account integration's OAuth tokens,
// client secret, encryption key and username) and echoes a credential-named
// query value; the contract holds none of those, since RocketCyber declares
// no response field, flag or MCP argument (outOfContract). The suite checks
// that:
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the token from ROCKETCYBER_API_TOKEN, from the
//     credential store (auth set-token) or from a config file's api_token,
//     access_token or auth_header, a credential the configuration holds but
//     does not send (a stored token the environment overrides, a config
//     file's access_token, refresh_token, client_id and client_secret), a
//     configured header value, the MCP HTTP token, a credential-named query
//     value given on the command line or as an MCP argument (tracked, see
//     outOfContract), and the credential-named members a response carries
//     (tracked the same way);
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     feedback log, the learn journal, profiles, teach logs);
//   - no credential leaves for a host other than the configured one: a
//     redirect off the API host (a homoglyph host included) is refused before
//     it is followed, and nothing given later (a path argument, an MCP
//     parameter) can move the pinned origin;
//   - the MCP server sends only reads (every vendor tool is read-only; the
//     tools that write do so only to local files), and --dry-run sends
//     nothing.
//
// RocketCyber data under names that read as credentials or carry identities
// (an Office report's secureScoreProgress, secureScorePercentage,
// secureScoreToDo and a monitored account's mfaStatus; an event's
// tokenIssuerType and its additionalInfo Key/Value pair; a suppression
// rule's user; account, agent, incident, app and rule ids and names) is never
// withheld when the answer repeats no held credential.
//
// Every child process runs with HTTPS_PROXY and HTTP_PROXY pointing at a trap
// (loopback hosts, the fakes, are never proxied), so a request that got past
// the pin to any real host would reach the trap instead; TestMain fails the
// run when the trap saw anything.
//
// The oracle is the exact set of strings the fake sent, so a pass does not
// depend on the program's own decoder.
package rocketcyberblackbox

import (
	"bytes"
	"compress/gzip"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/csv"
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
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bbToken         = "bbtok-mzwmmwmzzwzwwzqzqzzqqqqzqmzk"  // ROCKETCYBER_API_TOKEN (Authorization: Bearer <token>)
	bbStored        = "storedToken-71c3e9b05d2a4f68e1"      // auth set-token (stdin), the credential store
	bbFileToken     = "cfgtok-kkwzwqwkwzmwwwwz"             // api_token in a 0600 config file named by ROCKETCYBER_CONFIG
	bbFileAccess    = "cfgacc-mqzkmkqkzqmwzk"               // access_token in a 0600 config file (sent as Bearer when no api_token is set; held unused otherwise)
	bbFileHeader    = "cfgFlHdr-9a4e1c7b3d58"               // auth_header = "Bearer <this>" in a 0600 config file (sent as given)
	bbFileRefresh   = "cfgFlRfr-0d6b2f8e4a71"               // refresh_token in a 0600 config file (held unused)
	bbFileSecret    = "cfgsec-wzkqmmmqqmwmqm"               // client_secret in a 0600 config file (held unused)
	bbFileClientID  = "cfgcid-wkwwzkwmqqwm"                 // client_id in a 0600 config file (held unused)
	bbFileHdrVal    = "cfgFlHval-4a7c2e9b51"                // headers."X-HQ-Trace" in a 0600 config file (sent as its own header)
	bbArgvSec       = "-argvSecret-91c3e7a5"                // a credential-named query value on the command line (sync --param password=...; starts with "-")
	bbMCPArgSec     = "mcpArgSecret-81d3c6f0"               // a credential-named query value an MCP sync call sends (param password=...)
	bbShortResp     = "Sh0rtPw"                             // a 7-character password every agent record carries (gzip-encoded list)
	bbBOMResp       = "BomRespSecret-6a1c"                  // an incident's api_key, BOM-prefixed list
	bbCredUser      = "integrationUser-5c1e9a27"            // an incident's credentials.username (inside a credential container)
	bbGzipResp      = "GzipRespToken-4f2a9c1e"              // the account integration's OAuth access_token, gzip-encoded response
	bbRefreshResp   = "refreshRespToken-8e2d4b6f0a"         // the account integration's OAuth refresh_token (gzip)
	bbClientSecResp = "clsec-mmwmmmwmmzkwmk"                // the account integration's OAuth client_secret (gzip)
	bbEncKey        = "enckey-qwzwqkwmzmmqmmwz"             // the account integration's encryption_key (a qualified key name)
	bbPairUser      = "ssoUser-3e8a51c0"                    // the account integration's username (the identity half of the pair)
	bbFwSecret      = "fwsec-zqqqmzqzmqzmmwq"               // a firewall record's secret
	bbFwToken       = "fwToken-8c3e1a6f0b2d57"              // a firewall record's token
	bbAppAccess     = "appacc-zzkzkzkzqzzqm"                // an app record's access_token
	bbHTTPToken     = "mcpht-kzkkqqqwzwmkzwmzmzqkqwwkkwwzw" // ROCKETCYBER_MCP_HTTP_TOKEN
	bbAccountID     = "2"                                   // the provider account
	bbCustomerID    = "2311414"                             // the client account (customerId) every agent and incident belongs to
	bbAgentID       = "asda12341aasdafg1ag1llk412a"         // an online agent
	bbAgent2        = "1b9926c916a2ce004dd1f3ba9f4eb140"    // an offline agent, silent 20 days
	bbIncidentID    = "123"                                 // an open incident, 2 days old
	bbIncident2     = "124"                                 // a resolved incident (36 hours to resolve)
	bbRuleID        = "42"                                  // a suppression rule, active and untouched for 200 days
	bbAppID         = "60"                                  // the detection app the events belong to
	bbRedirect      = "9911"                                // a rule id the fake redirects off-host
	bbHomoglyph     = "9922"                                // a rule id the fake redirects to a homoglyph of the API host
	bbDenied        = "40199"                               // a rule id (or a query value) the fake answers 401
	// RocketCyber data under names that read as credentials or identities:
	// never withheld (rocketcyber-credential-names-exact).
	bbAccountName = "HQ-Record Provider"                  // the provider account's accountName and name
	bbClientName  = "HQ-Record Client Co"                 // the client account's name
	bbHostname    = "HQ-Record-PROBE02"                   // bbAgentID's hostname
	bbHostname2   = "HQ-Record-LT-149"                    // bbAgent2's hostname, Defender's riskiest device
	bbIncTitle    = "HQ-Record Webroot Detection"         // bbIncidentID's title
	bbRuleName    = "HQ-Record suppress printer noise"    // bbRuleID's ruleName
	bbRuleUser    = "HQ-Record Brett"                     // the user who created bbRuleID
	bbUPN         = "pat.user@hq-record.example.com"      // an event's userPrincipalName
	bbMonitored   = "monitored@hq-record.example.com"     // an Office monitored account's id
	bbControl     = "AdminMFAV2"                          // an Office secure-score to-do control
	bbReportRow   = "HQ-Record report row"                // a CSV report cell
	bbUserAgentUA = "Mozilla/5.0 (HQ-Record Workstation)" // an event's additionalInfo Value (Key userAgent)
)

// secrets are every credential the suite gives the process that the
// credential leak contract holds (section 1: the declared secret
// environment, the config file's credential fields and [headers] values, the
// credential store), in the order the fake splits them (the first one a
// request carried is the one split across pages). Each one in any output,
// file or request off its origin fails the test.
func secrets() []string {
	return []string{bbToken, bbStored, bbFileToken, bbFileAccess, bbFileHeader, bbFileRefresh, bbFileSecret, bbFileClientID, bbFileHdrVal,
		bbHTTPToken}
}

// outOfContract are the suite's other credential-shaped values: a
// credential-named query value given with sync --param or as an MCP sync
// argument, and the credential-named members the fake's answers carry. The
// contract holds a flag, an MCP argument or a response field only when the
// connector declares it (section 1), and RocketCyber declares none: its
// Customer API is read-only, takes no credential in a query and answers
// none. The fake still echoes them, and an output that carries one is counted
// (outOfContractSeen, reported by TestMain), never failed: the old
// connector-side layer withheld them by name, the kit does not
// (handfixes.json credkit-wiring).
func outOfContract() []string {
	return []string{bbArgvSec, bbMCPArgSec, bbShortResp, bbBOMResp, bbCredUser, bbGzipResp, bbRefreshResp, bbClientSecResp, bbEncKey, bbPairUser,
		bbFwSecret, bbFwToken, bbAppAccess}
}

// kitClosedMCP is what the MCP server logs when the kit withholds a
// JSON-RPC message the tool layer did not answer (a protocol error that
// repeats a held tool or prompt name): the kit fails the session's output
// closed, so that request and every later one get no reply.
const kitClosedMCP = "credkit: the MCP response failed closed"

// kitNotice is the fixed notice the kit writes in place of a unit it
// withheld from sink (credkit.Notice).
func kitNotice(sink string) string {
	return "[credkit: withheld from " + sink + ": it carried a credential this program holds]"
}

// outOfContractSeen counts the outputs that carried an outOfContract value.
var outOfContractSeen struct {
	sync.Mutex
	n int
}

var bbCLI, bbMCP string

// trap stands in for every host but the loopback fakes: each child process
// sends its non-loopback requests through it (HTTPS_PROXY, HTTP_PROXY), so a
// request that got past the pin is recorded here, never sent to a real host.
var trap struct {
	sync.Mutex
	url   string
	dials []string
}

func trapDials() []string {
	trap.Lock()
	defer trap.Unlock()
	return append([]string{}, trap.dials...)
}

// trapEnv routes every non-loopback request of a child through the trap.
func trapEnv() []string {
	return []string{"HTTPS_PROXY=" + trap.url, "HTTP_PROXY=" + trap.url, "ALL_PROXY=" + trap.url, "NO_PROXY=", "no_proxy="}
}

// bbMinTimeout is what a full run needs (about 40 minutes: 971 CLI runs and
// 435 MCP calls). Under a shorter go test timeout (CI's default 10m) the
// suite says it is skipped instead of dying mid-run as a false failure; the
// factory runs it with -timeout 60m.
const bbMinTimeout = 60 * time.Minute

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	if f := flag.Lookup("test.timeout"); f != nil {
		if d, ok := f.Value.(flag.Getter).Get().(time.Duration); ok && d > 0 && d < bbMinTimeout {
			fmt.Fprintf(os.Stderr, "rocketcyberblackbox: SKIPPED: the suite needs -timeout %v (have %v): go test -timeout 60m ./internal/rocketcyberblackbox/\n", bbMinTimeout, d)
			os.Exit(0)
		}
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trap.Lock()
		trap.dials = append(trap.dials, r.Method+" "+r.Host+r.URL.Path)
		trap.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	trap.url = ts.URL
	dir, err := os.MkdirTemp("", "rocketcyber-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "rocketcyber-cli"+exe)
	bbMCP = filepath.Join(dir, "rocketcyber-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/rocketcyber-cli", bbMCP: "../../cmd/rocketcyber-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	ts.Close()
	if d := trapDials(); len(d) != 0 {
		fmt.Fprintf(os.Stderr, "FAIL: %d request(s) left for a non-loopback host (caught by the proxy trap): %v\n", len(d), d)
		code = 1
	}
	fmt.Fprintf(os.Stderr, "rocketcyberblackbox: %d CLI runs, %d MCP tools/call requests\n", cliRuns.n, mcpCallsSent.n)
	fmt.Fprintf(os.Stderr, "rocketcyberblackbox: %d checked output(s) carried an out-of-contract value (outOfContract; tracked, not a leak)\n", outOfContractSeen.n)
	os.Exit(code)
}

// echoForms is every encoding of s the server sends back whole: as written,
// inside other text, as the header line, base64 (standard and URL-safe), hex,
// base32, ASCII85, percent-encoded, PEM-armored, and gzip-compressed then
// base64-encoded (the way a JSON string carries compressed bytes).
func echoForms(s string) []string {
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return []string{
		s,
		"x" + s + "x",
		"Authorization: Bearer " + s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
		string(pem.EncodeToMemory(&pem.Block{Type: "ROCKETCYBER API TOKEN", Bytes: []byte(s)})),
		gzipBase64(s),
	}
}

// gzipBase64 is s gzip-compressed (no header name or time, so the bytes are
// the same every run) and base64-encoded.
func gzipBase64(s string) string {
	var z bytes.Buffer
	zw := gzip.NewWriter(&z)
	_, _ = zw.Write([]byte(s))
	_ = zw.Close()
	return base64.StdEncoding.EncodeToString(z.Bytes())
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

// echoObject repeats each of sent whole, in every form.
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

// requestText is everything a request carried: its headers, its URL and its
// body (which it restores for the handler).
func requestText(r *http.Request) string {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	return fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
}

// sentSecrets is every known secret the request carried (its headers, the
// URL or the body): a real API can echo what it was sent.
func sentSecrets(r *http.Request) []string {
	seen := requestText(r)
	var out []string
	for _, s := range append(secrets(), outOfContract()...) {
		if strings.Contains(seen, s) {
			out = append(out, s)
		}
	}
	return out
}

// firstSent is the first credential a request carried, or a public
// placeholder when it carried none.
func firstSent(sent []string) string {
	if len(sent) == 0 {
		return "no-credential-was-sent"
	}
	return sent[0]
}

// union is a followed by every value of b not already in it.
func union(a []string, b ...string) []string {
	out := append([]string{}, a...)
	for _, s := range b {
		dup := false
		for _, x := range out {
			dup = dup || x == s
		}
		if !dup {
			out = append(out, s)
		}
	}
	return out
}

type server struct {
	url     string // ROCKETCYBER_BASE_URL: the fake's origin + /v3
	caFile  string
	mu      sync.Mutex
	offHost int
	methods []string        // API requests, "METHOD /v3/path"
	keys    map[string]int  // tokens the API received as the one header Authorization: Bearer <token>
	headers map[string]int  // every Authorization header value the API received, as sent
	texts   strings.Builder // everything every request carried (headers, URL, body)
	// otherAuth counts requests whose credential came any other way: a second
	// Authorization header, one that is not "Bearer <token>" (a doubled
	// scheme included), an X-Api-Key header, or a token in the query
	// (RocketCyber takes only the Authorization header).
	otherAuth int
	// otherAuthSeen records what each such request carried, for the report.
	otherAuthSeen []string
	// pages counts each list page the fake served, by "/v3/path?page=N".
	pages map[string]int
	// redirectAgents sends the agent list (a syncable resource the novels
	// read) to the other host too, so sync meets a redirect.
	redirectAgents bool
	// denied paths (/v3 prefix stripped) answer 401 (an export or sync that
	// meets an API error).
	denied map[string]bool
	// denials counts the requests the fake answered 401.
	denials int
}

func newServer() *server {
	return &server{keys: map[string]int{}, headers: map[string]int{}, pages: map[string]int{}, denied: map[string]bool{}}
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

// caFileFor writes srv's certificate as a PEM CA bundle (ROCKETCYBER_CA_CERT).
func caFileFor(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return ca
}

// isWrite reports a request that changes data (HEAD, a count, does not).
func isWrite(m string) bool {
	method, _, _ := strings.Cut(m, " ")
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

// apiPath cuts the /v3 prefix off a request path; ok is false for a path
// outside the API.
func apiPath(p string) (string, bool) {
	rest, ok := strings.CutPrefix(p, "/v3")
	return rest, ok && (rest == "" || strings.HasPrefix(rest, "/"))
}

// pageQuery is a RocketCyber list request's page: pageSize (def when absent,
// at most most) and the 1-based page (a page below 1 reads as the first).
func pageQuery(q url.Values, def, most int) (size, number int) {
	size, number = def, 1
	if n, err := strconv.Atoi(q.Get("pageSize")); err == nil && n > 0 {
		size = min(n, most)
	}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n >= 1 {
		number = n
	}
	return size, number
}

// pagedList is one page of a RocketCyber list, items[(page-1)*pageSize:...],
// in the documented envelope (totalCount, currentPage, totalPages, dataCount,
// data); extra members follow data.
func pagedList(q url.Values, def, most int, items []string, extra string) string {
	size, number := pageQuery(q, def, most)
	start := min((number-1)*size, len(items))
	page := items[start:min(start+size, len(items))]
	total := max(1, (len(items)+size-1)/size)
	return fmt.Sprintf(`{"totalCount":%d,"currentPage":%d,"totalPages":%d,"dataCount":%d,"data":[%s]%s}`,
		len(items), number, total, len(page), strings.Join(page, ","), extra)
}

func gzipped(w http.ResponseWriter, doc string) {
	var z bytes.Buffer
	zw := gzip.NewWriter(&z)
	_, _ = io.WriteString(zw, doc)
	_ = zw.Close()
	w.Header().Set("Content-Encoding", "gzip")
	_, _ = w.Write(z.Bytes())
}

// countAuth records how a request authenticated: the one Authorization:
// Bearer <token> header counts the token; anything else counts as other
// auth. Call with sv.mu held.
func (sv *server) countAuth(r *http.Request) {
	auth := r.Header.Values("Authorization")
	h := r.Header.Get("Authorization")
	for _, v := range auth {
		sv.headers[v]++
	}
	scheme, tok, _ := strings.Cut(h, " ")
	switch {
	case len(auth) == 1 && scheme == "Bearer" && tok != "" && !strings.ContainsAny(tok, " \t"):
		sv.keys[tok]++
	case len(auth) > 0:
		sv.otherAuth++
		sv.otherAuthSeen = append(sv.otherAuthSeen, fmt.Sprintf("%d Authorization header(s), scheme %q, %d-character credential", len(auth), scheme, len(tok)))
	}
	if len(r.Header.Values("X-Api-Key")) > 0 {
		sv.otherAuth++
		sv.otherAuthSeen = append(sv.otherAuthSeen, "X-Api-Key")
	}
	q := r.URL.Query()
	for _, k := range []string{"api_key", "apikey", "api_token", "x-api-key", "token", "access_token", "key"} {
		if q.Has(k) {
			sv.otherAuth++
			sv.otherAuthSeen = append(sv.otherAuthSeen, "query "+k)
		}
	}
}

// receivedKeys are the distinct tokens this fake received, sorted.
func (sv *server) receivedKeys() []string {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var all []string
	for k := range sv.keys {
		all = append(all, k)
	}
	sort.Strings(all)
	return all
}

var reRule = regexp.MustCompile(`^/suppression/rule/[^/]+$`)

// csvReport is the /reportApi answer: a CSV report whose cells repeat each of
// sent in every form, and split across two adjacent cells and across a
// quoted line break, after one row of public report data.
func csvReport(sent []string, detected string) string {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"appId", "deviceId", "verdict", "detectionDate", "value", "valueType", "note", "extra"})
	_ = w.Write([]string{bbAppID, bbAgentID, "malicious", detected, "HQ-Record signin from 192.0.2.44", "signin", bbReportRow, ""})
	for _, s := range sent {
		for _, f := range append(echoForms(s), jsonUnicode(s)) {
			_ = w.Write([]string{bbAppID, bbAgentID, "informational", detected, f, "echo", "x", ""})
		}
		a, c := halves(s)
		_ = w.Write([]string{bbAppID, bbAgentID, "informational", detected, a, c, a + "\n" + c, a + " " + c})
	}
	w.Flush()
	return b.String()
}

// echoServer starts the RocketCyber fake (TLS).
func echoServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, false)
}

// quietServer is the fake answering as RocketCyber does: the same records,
// repeating nothing a request carried. The kit withholds a unit whole
// (credential leak contract section 4, no partial redaction), so an answer
// that echoes the held token never prints or persists any of its public
// data; the quiet fake is what proves a sink really writes and that no
// RocketCyber field is withheld by its name.
func quietServer(t *testing.T) *server {
	t.Helper()
	return fakeServer(t, true)
}

// shapeServer starts a RocketCyber fake (TLS) answering what docs returns for
// a path (a document, and whether it has one: 404 otherwise). docs sees the
// path under /v3 ("/agents", "/suppression/rule/42"); the answer to /account
// goes gzip-encoded. It records the requests and the tokens it received.
func shapeServer(t *testing.T, docs func(p string, r *http.Request) (string, bool)) *server {
	t.Helper()
	sv := newServer()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		sv.texts.WriteString(requestText(r) + "\n")
		sv.countAuth(r)
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		p, onAPI := apiPath(r.URL.Path)
		doc, ok := docs(p, r)
		switch {
		case !onAPI || !ok:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"statusCode":404,"message":"Not Found"}`)
		case p == "/account":
			gzipped(w, doc)
		default:
			fmt.Fprint(w, doc)
		}
	}))
	sv.url = srv.URL + "/v3"
	sv.caFile = caFileFor(t, srv)
	return sv
}

func fakeServer(t *testing.T, quiet bool) *server {
	t.Helper()
	sv := newServer()
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`{"totalCount":0,"data":[]}`))
	}))
	now := time.Now().UTC()
	ago := func(d time.Duration) string { return now.Add(-d).Format("2006-01-02T15:04:05.000Z") }
	day := 24 * time.Hour
	agent := func(id, hostname, conn string, last time.Duration, extra string) string {
		return fmt.Sprintf(`{"id":%q,"customerId":%s,"hostname":%q,"ipv4Address":"192.0.2.10","macAddress":"C4:34:64:54:F4:D4","createdAt":%q,`+
			`"platform":"Microsoft","family":"Windows","version":"10","edition":"Pro","architecture":"64-bit","build":"19041","release":"2009",`+
			`"accountPath":"2.1123.%s","agentVersion":"v1.5 \"Release\" Build (22460)","connectivity":%q,"lastConnected":%q,"password":%q%s}`,
			id, bbCustomerID, hostname, ago(300*day), bbCustomerID, conn, ago(last), bbShortResp, extra)
	}
	incident := func(id, title, status string, created, resolvedAfter time.Duration, extra string) string {
		resolved := "null"
		if resolvedAfter > 0 {
			resolved = strconv.Quote(now.Add(-created + resolvedAfter).Format("2006-01-02T15:04:05.000Z"))
		}
		return fmt.Sprintf(`{"id":%q,"title":%q,"description":"HQ-Record detected and remediated a file on the agent","remediation":"Review the detection.\r\nRun a full AV scan of the system.",`+
			`"resolvedAt":%s,"publishedAt":%q,"createdAt":%q,"updatedAt":%q,"status":%q,"accountId":%s,"customerId":%s%s}`,
			id, title, resolved, ago(created), ago(created), ago(created/2), status, bbAccountID, bbCustomerID, extra)
	}
	event := func(verdict, extra string) string {
		return fmt.Sprintf(`{"appId":%s,"deviceId":null,"verdict":%q,"detectionDate":%q,"value":"HQ-Record signin from 192.0.2.44","valueType":"signin",`+
			`"details":{"time":%q,"result":null,"source":"IdentityProtection","activity":"signin","location":{"city":"New York","state":"New York",`+
			`"geoCoordinates":{"latitude":40.720392,"longitude":-24.030888},"countryOrRegion":"US"},"ipAddress":"192.0.2.44","riskLevel":"medium",`+
			`"riskState":"atRisk","riskDetail":"none","description":"none","resultReason":null,"riskEventType":"anonymizedIPAddress",`+
			`"additionalInfo":{"Key":"userAgent","Value":%q},"loggingService":null,"tokenIssuerType":"AzureAD","userDisplayName":"HQ-Record Pat User",`+
			`"detectedDateTime":%q,"userPrincipalName":%q,"detectionTimingType":"realtime"},"createdAt":%q,"updatedAt":%q,"customerId":%s,`+
			`"accountPath":"2.4.%s","incidentId":null,"archived":false%s}`,
			bbAppID, verdict, ago(day), ago(day), bbUserAgentUA, ago(day), bbUPN, ago(day), ago(day), bbCustomerID, bbCustomerID, extra)
	}
	rule := func(extra string) string {
		return fmt.Sprintf(`{"ruleId":%s,"ruleName":%q,"status":"active","accountId":%s,"userId":335,"user":{"id":335,"name":%q},`+
			`"conditions":[{"field":"hostname","operator":"equals","value":"HQ-PRINTER-01"}],"comment":"HQ-Record printer spooler noise",`+
			`"createdAt":%q,"updatedAt":%q,"startDate":%q,"stopDate":null%s}`,
			bbRuleID, bbRuleName, bbAccountID, bbRuleUser, ago(400*day), ago(200*day), ago(400*day), extra)
	}
	firewall := func(id, extra string) string {
		return fmt.Sprintf(`{"id":%q,"accountPath":"2.4.%s","details":{"mac":"11:b1:11:11:3a:ec","type":"sonicwall"},"createdAt":%q,"instanceType":"firewall",`+
			`"secret":%q,"token":%q%s}`, id, bbCustomerID, ago(day), bbFwSecret, bbFwToken, extra)
	}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sent []string
		if !quiet {
			sent = sentSecrets(r)
		}
		echo := echoObject(sent)
		p, onAPI := apiPath(r.URL.Path)
		q := r.URL.Query()
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		sv.texts.WriteString(requestText(r) + "\n")
		sv.countAuth(r)
		pg := q.Get("page")
		if pg == "" {
			pg = "1"
		}
		if r.Method == http.MethodGet {
			sv.pages[r.URL.Path+"?page="+pg]++
		}
		redirectAgents := sv.redirectAgents
		denied := sv.denied[p]
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		deniedQuery := false
		for _, vs := range q {
			for _, v := range vs {
				deniedQuery = deniedQuery || v == bbDenied
			}
		}
		list := func(items ...string) string { return pagedList(q, 1000, 1000, items, "") }
		switch {
		case !onAPI:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"statusCode":404,"message":"Not Found"}`)
		case denied || deniedQuery || strings.Contains(p, "/"+bbDenied):
			// Rejected (a denied rule, a sync or export the test denies): the
			// error repeats what it was sent.
			sv.mu.Lock()
			sv.denials++
			sv.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"statusCode":401,"error":"Unauthorized","message":"invalid token %s","errors":[{"message":"bad token %s"}],"echo":%s}`,
				strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case strings.Contains(p, "/"+bbRedirect) || (redirectAgents && p == "/agents"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case strings.Contains(p, "/"+bbHomoglyph):
			// A redirect to the API host spelled with a Cyrillic "е".
			w.Header().Set("Location", "https://api-us.rocketcyb\u0435r.com"+r.URL.Path+"?token="+url.QueryEscape(firstSent(sent)))
			w.WriteHeader(http.StatusTemporaryRedirect)
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNoContent)
		case r.Method != http.MethodGet:
			// The Customer API is read-only.
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintf(w, `{"statusCode":405,"message":"Method Not Allowed","echo":%s}`, echo)
		case p == "/account":
			// The provider account, gzip-encoded, with the OAuth credentials
			// of an integration: access and refresh tokens, a client secret,
			// an encryption key and a username.
			gzipped(w, fmt.Sprintf(`{"id":%s,"accountId":%s,"accountName":%q,"name":%q,"hierarchy":"HQ-Record MSP -> %s","path":"9578.%s",`+
				`"address":{"street1":"1 HQ Way","street2":"Suite E","city":"Austin","state":"TX","country":"United States of America","zipCode":"75001"},`+
				`"type":"Provider","status":"Active","emails":null,"customers":[%s],"customerAccounts":[{"customerId":%s,"accountName":%q}],`+
				`"integration":{"username":%q,"access_token":%q,"refresh_token":%q,"client_secret":%q,"encryption_key":%q},"echo":%s}`,
				bbAccountID, bbAccountID, bbAccountName, bbAccountName, bbAccountName, bbCustomerID, bbCustomerID, bbCustomerID, bbClientName,
				bbPairUser, bbGzipResp, bbRefreshResp, bbClientSecResp, bbEncKey,
				echoObject(union(sent, bbGzipResp, bbRefreshResp, bbClientSecResp, bbEncKey, bbPairUser))))
		case p == "/agents":
			// The agents, gzip-encoded and paged, each with a password: the
			// first carries the echo.
			gzipped(w, list(agent(bbAgentID, bbHostname, "online", time.Hour, `,"echo":`+echo), agent(bbAgent2, bbHostname2, "offline", 20*day, "")))
		case p == "/incidents":
			// Incidents (list, sync, export, triage, mttr): BOM-prefixed, an
			// api_key and a credentials container, the echo, and a record
			// whose identifier repeats the credential it was sent.
			fmt.Fprint(w, "\xef\xbb\xbf"+list(
				incident(bbIncidentID, bbIncTitle, "open", 2*day, 0, fmt.Sprintf(`,"api_key":%q,"credentials":{"username":%q},"echo":%s`, bbBOMResp, bbCredUser, echo)),
				incident(bbIncident2, "HQ-Record Sophos cleanup", "resolved", 40*day, 36*time.Hour, ""),
				incident(firstSent(sent), "Echo-ID", "open", 3*day, 0, "")))
		case p == "/events":
			if q.Get("appId") == "" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"statusCode":400,"message":"appId is required"}`)
				break
			}
			fmt.Fprint(w, list(event("malicious", `,"echo":`+echo), event("suspicious", "")))
		case p == "/events/summary":
			// Per-app verdict counts: the echo split across values, lines and
			// spaces.
			fmt.Fprintf(w, `{"accountId":%s,"data":[{"appId":%s,"accountId":%s,"malicious":1,"suspicious":2,"informational":4}],"split":%s}`,
				bbAccountID, bbAppID, bbAccountID, splitObject(sent))
		case p == "/apps":
			// The apps: an app's access_token, the echo, and the sent
			// credentials in response headers.
			w.Header().Set("X-Hq-Echo", strings.Join(sent, " "))
			w.Header().Set("Content-Location", "https://api-us.rocketcyber.com/v3/apps?token="+url.QueryEscape(firstSent(sent)))
			fmt.Fprint(w, list(
				fmt.Sprintf(`{"id":2,"name":"Advanced Breach Detection","description":"HQ-Record: computers compromised past their defenses","status":"active","access_token":%q,"echo":%s}`, bbAppAccess, echo),
				fmt.Sprintf(`{"id":%s,"name":"HQ-Record Identity Protection","description":"HQ-Record: risky Microsoft 365 sign-ins","status":"active"}`, bbAppID)))
		case p == "/defender":
			fmt.Fprintf(w, `{"totalDevices":66,"totalHealthy":34,"totalUnhealthy":12,"totalUnknown":20,`+
				`"detectionSummary":{"totalEvents":3,"totalMalicious":2,"totalSuspicious":1,"totalInformational":0},`+
				`"devicesAtRisk":{"total":2,"data":[{"deviceId":%q,"hostname":%q,"ipAddress":"192.0.2.11",`+
				`"operatingSystem":{"platform":"Microsoft","family":"Windows","version":"10","edition":"Enterprise"},"detections":{"malicious":60,"suspicious":0,"informational":0}},`+
				`{"deviceId":"1646af2edb210fcec7b85b3adbb76692","hostname":"HQ-Record-LT-174","ipAddress":"192.0.2.12",`+
				`"operatingSystem":{"platform":"Microsoft","family":"Windows","version":"10","edition":"Pro"},"detections":{"malicious":2,"suspicious":180,"informational":0}}]},`+
				`"devicesWithPoorHealth":{"total":1,"data":[{"deviceId":"1a967806748f650335e42b6c66bc8826","hostname":"HQ-Record-LT-29","ipAddress":"192.0.2.13",`+
				`"operatingSystem":{"platform":"Microsoft","family":"Windows","version":"10","edition":"Enterprise"}}]},`+
				`"devicesWithUnknownHealth":{"total":0,"data":[]},"echo":%s}`, bbAgent2, bbHostname2, echo)
		case p == "/office":
			// The documented Microsoft 365 report: the secure-score series,
			// the monitored accounts and their MFA status, and the to-do
			// controls.
			fmt.Fprintf(w, `{"secureScoreProgress":{"startDate":%q,"endDate":%q,"totalDays":3,"minScore":36.76,"maxScore":48.92,"averageScore":42.29,`+
				`"data":[{"detectionDate":%q,"secureScorePercentage":36.76},{"detectionDate":%q,"secureScorePercentage":41.18},{"detectionDate":%q,"secureScorePercentage":48.92}]},`+
				`"monitoredAccounts":{"total":1,"data":[{"id":%q,"mfaStatus":"unknown","licenses":["TEAMS_EXPLORATORY"]}]},`+
				`"secureScoreToDo":{"total":2,"data":[{"maxScore":50,"control":%q,"description":"HQ-Record: require MFA for administrative roles",`+
				`"remediation":"HQ-Record: enable MFA for every admin"},{"maxScore":1,"control":"meeting_restrictanonymousjoin_v1",`+
				`"description":"HQ-Record: restrict anonymous meeting joins","remediation":"HQ-Record: turn anonymous join off"}]},"echo":%s}`,
				ago(3*day), ago(day), ago(3*day), ago(2*day), ago(day), bbMonitored, bbControl, echo)
		case p == "/firewalls":
			// The first credential it was sent, split across two pages of the
			// firewalls (pageSize, default 1000, full pages until the last;
			// 1-based page): the first firewall of page 1 carries one half as
			// its id, the first firewall of page 2 the other, so every pager
			// that walks the pages meets both halves as records of their own.
			a, b := halves(firstSent(sent))
			size, _ := pageQuery(q, 1000, 1000)
			items := []string{firewall(a, "")}
			for i := 1; i < size; i++ {
				items = append(items, fmt.Sprintf(`{"id":"fw-%05d","instanceType":"firewall"}`, i+1))
			}
			items = append(items, firewall(b, ""))
			counters := ""
			if q.Get("counters") == "true" {
				counters = `,"counters":{"received":26075587,"parsed":26075587,"filtered":26074536,"reported":1051}`
			}
			fmt.Fprint(w, pagedList(q, 1000, 1000, items, counters))
		case p == "/suppression/rules":
			fmt.Fprint(w, list(rule(`,"echo":`+echo)))
		case reRule.MatchString(p):
			// One rule: the echo split across values, lines and spaces.
			fmt.Fprint(w, rule(`,"split":`+splitObject(sent)))
		case p == "/reportApi":
			// A CSV report: text/csv for events, a download
			// (application/octet-stream) for incidents.
			ct := "text/csv; charset=utf-8"
			if q.Get("itemType") == "incidents" {
				ct = "application/octet-stream"
				w.Header().Set("Content-Disposition", `attachment; filename="report.csv"`)
			}
			w.Header().Set("Content-Type", ct)
			fmt.Fprint(w, csvReport(sent, ago(day)))
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"statusCode":404,"message":"Not Found"}`)
		}
	}))
	sv.url = srv.URL + "/v3"
	sv.caFile = caFileFor(t, srv)
	return sv
}

// sawHeader reports how many requests carried the Authorization header h,
// and the distinct header values the fake received.
func (sv *server) sawHeader(h string) (int, []string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var all []string
	for k := range sv.headers {
		all = append(all, k)
	}
	return sv.headers[h], all
}

// received reports whether any request carried v anywhere.
func (sv *server) received(v string) bool {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return strings.Contains(sv.texts.String(), v)
}

// sawKey reports how many requests carried the token v in the Authorization
// header, and the distinct tokens the fake received.
func (sv *server) sawKey(v string) (int, []string) {
	all := sv.receivedKeys()
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.keys[v], all
}

func (sv *server) resetMethods() {
	sv.mu.Lock()
	sv.methods = nil
	sv.mu.Unlock()
}

// allMethods is every request the fake received.
func (sv *server) allMethods() []string {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return append([]string{}, sv.methods...)
}

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its API token.
type credMode int

const (
	credStored        credMode = iota // none in the environment: the battery runs on a token stored with auth set-token
	credEnv                           // ROCKETCYBER_API_TOKEN
	credEnvOverStored                 // a stored token and ROCKETCYBER_API_TOKEN: the environment wins, the stored token is held unused
	credConfigFile                    // api_token in a 0600 config file named by ROCKETCYBER_CONFIG
	credConfigAccess                  // access_token alone in a 0600 config file: sent as Bearer
	credConfigBoth                    // api_token, access_token, refresh_token, client_id, client_secret and a header: api_token is sent, the header as its own header, the rest held unused
	credConfigHeader                  // auth_header = "Bearer <token>" in a 0600 config file: sent as given
	credEnvPrefixed                   // ROCKETCYBER_API_TOKEN="Bearer <token>": already prefixed (held whole and without the scheme)
)

func (m credMode) String() string {
	return [...]string{"stored", "env ROCKETCYBER_API_TOKEN", "env over stored", "config api_token", "config access_token", "config every key",
		"config auth_header", "env prefixed Bearer"}[m]
}

// sent is the token the mode sends; held are the credentials it holds but
// never sends as the Authorization token.
func (m credMode) sent() string {
	return [...]string{bbStored, bbToken, bbToken, bbFileToken, bbFileAccess, bbFileToken, bbFileHeader, bbToken}[m]
}

// sentHeader is the Authorization header value the mode sends.
func (m credMode) sentHeader() string {
	if m == credEnvPrefixed {
		return knownPrefixedHeader
	}
	return "Bearer " + m.sent()
}

func (m credMode) held() []string {
	switch m {
	case credEnvOverStored:
		return []string{bbStored, bbHTTPToken}
	case credConfigBoth:
		return []string{bbFileAccess, bbFileRefresh, bbFileSecret, bbFileClientID, bbFileHdrVal, bbHTTPToken}
	}
	return []string{bbHTTPToken}
}

// knownPrefixedHeader is what ROCKETCYBER_API_TOKEN="Bearer <token>" sends:
// the generated config.go applies its "Bearer {token}" format to the value
// as given, so the scheme is doubled (P2 env-token-prefixed-twice: RocketCyber
// answers 401; the request still goes only to the pinned host and the token
// never prints).
const knownPrefixedHeader = "Bearer Bearer " + bbToken

// newEnv builds a clean environment against the fake with the credentials
// mode names; the stored modes run auth set-token first.
func newEnv(t *testing.T, sv *server, mode credMode) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"ROCKETCYBER_BASE_URL=" + sv.url, "ROCKETCYBER_CA_CERT=" + sv.caFile,
		"ROCKETCYBER_CLI_PATH=" + bbCLI, "ROCKETCYBER_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}, trapEnv()...)
	e := &bbEnv{home: home, env: env}
	switch mode {
	case credStored:
		e.storeToken(t, bbStored)
	case credEnv:
		e.env = append(e.env, "ROCKETCYBER_API_TOKEN="+bbToken)
	case credEnvOverStored:
		e.storeToken(t, bbStored)
		e.env = append(e.env, "ROCKETCYBER_API_TOKEN="+bbToken)
	case credConfigFile:
		e.writeConfig(t, fmt.Sprintf("api_token = %q\n", bbFileToken))
	case credConfigAccess:
		e.writeConfig(t, fmt.Sprintf("access_token = %q\n", bbFileAccess))
	case credConfigBoth:
		e.writeConfig(t, fmt.Sprintf("api_token = %q\naccess_token = %q\nrefresh_token = %q\nclient_id = %q\nclient_secret = %q\n\n[headers]\nX-HQ-Trace = %q\n",
			bbFileToken, bbFileAccess, bbFileRefresh, bbFileClientID, bbFileSecret, bbFileHdrVal))
	case credConfigHeader:
		e.writeConfig(t, fmt.Sprintf("auth_header = %q\n", "Bearer "+bbFileHeader))
	case credEnvPrefixed:
		e.env = append(e.env, "ROCKETCYBER_API_TOKEN=Bearer "+bbToken)
	}
	return e
}

// writeConfig writes body as a 0600 config file and names it with
// ROCKETCYBER_CONFIG.
func (e *bbEnv) writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := filepath.Join(e.home, "cfg")
	_ = os.MkdirAll(dir, 0o700)
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "ROCKETCYBER_CONFIG"), "ROCKETCYBER_CONFIG="+cfg)
}

// storeToken saves tok with auth set-token (stdin) and checks it landed in
// the credential store.
func (e *bbEnv) storeToken(t *testing.T, tok string) {
	t.Helper()
	out := e.runIn(t, tok+"\n", "auth", "set-token")
	var stored bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "credentials") {
			b, _ := os.ReadFile(path)
			stored = stored || bytes.Contains(b, []byte(tok))
		}
		return nil
	})
	if !stored {
		t.Fatalf("auth set-token wrote no credential store:\n%s", out)
	}
}

func (e *bbEnv) run(t *testing.T, args ...string) string {
	t.Helper()
	return e.runIn(t, "", args...)
}

// cliRuns counts every CLI process the suite starts (the report's battery
// size); mcpCallsSent counts every tools/call it sends.
var cliRuns, mcpCallsSent struct {
	sync.Mutex
	n int
}

func count(c *struct {
	sync.Mutex
	n int
}, n int) {
	c.Lock()
	c.n += n
	c.Unlock()
}

func (e *bbEnv) runIn(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	count(&cliRuns, 1)
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
	case <-time.After(600 * time.Second): // a hang guard; the kit checks each SQLite statement, and a 2500-record sync took 160 s at load 80 and over 300 s at load 115
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

// mcpSessionDeadline bounds one MCP session. P2 chokepoint-latency-under-echo
// (seen/mcp-call-timing.log): a result that echoes held values has
// WithholdText re-run the whole HeldEchoes check on each JSON string literal,
// so with every config key held (credConfigBoth) and the fake's split-echo
// records in the store, the full-store sql read alone takes over 120s and
// small results 6s each late in the session; the 43-call battery did not
// finish in 300s (about 500s alone, more beside the other modes). Every
// result still fails closed; documented RocketCyber responses carry no
// credential, so that loop never runs on them.
const mcpSessionDeadline = 1500 * time.Second

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
	// A server that exits early (the kit refuses to start on a credential
	// that occurs in its public text, exit 78), or whose output the kit has
	// closed (a protocol message that carried a held value: kitClosedMCP),
	// answers nothing more: stop waiting for replies then.
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	exited := func() bool {
		select {
		case <-waitDone:
			return true
		default:
			return strings.Contains(errb.String(), kitClosedMCP)
		}
	}
	enc := json.NewEncoder(stdin)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	id := 0
	deadline := time.Now().Add(mcpSessionDeadline)
	for _, calls := range phases {
		count(&mcpCallsSent, len(calls))
		for _, c := range calls {
			id++
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		}
		for time.Now().Before(deadline) && len(mcpReplies(out.String())) < id+1 && !exited() {
			time.Sleep(100 * time.Millisecond)
		}
	}
	_ = stdin.Close()
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

// foldedAlnum is s reduced to its ASCII letters and digits, lowercased: the
// form a normalizer that lowercases text and turns separators into spaces
// (the learning store's query family) leaves of a credential.
func foldedAlnum(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case 'A' <= c && c <= 'Z':
			b.WriteByte(c + ('a' - 'A'))
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9':
			b.WriteByte(c)
		}
	}
	return b.String()
}

// foldedMinAlnum is the shortest folded credential the case-folded check
// matches (the chokepoint's joined-view floor); shorter ones match by chance.
const foldedMinAlnum = 16

// assertNoEcho fails when text holds a secret in any form the server sent,
// both halves of one (a split the reader could rejoin), or one case-folded
// or re-separated (a normalizer's lowercased copy).
func assertNoEcho(t *testing.T, where, text string) {
	t.Helper()
	assertNoEchoIn(t, where, text, true)
}

// isStoreFile names the local SQLite store and its side files.
func isStoreFile(name string) bool {
	for _, suf := range []string{".db", ".db-wal", ".db-journal", ".db-shm"} {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}

// assertNoEchoIn is assertNoEcho; halves false skips the both-halves check
// (the local store's file only: a held value split across two stored records
// is the work order's ruled P2 credential-split-across-sql-bindings, proven
// by TestStoreSplitAcrossRecordsKnownP2; one record is checked whole by the
// store's own write path, so a split inside a record still fails here).
func assertNoEchoIn(t *testing.T, where, text string, halvesToo bool) {
	t.Helper()
	folded := foldedAlnum(text)
	for _, s := range secrets() {
		for _, f := range echoForms(s) {
			if strings.Contains(text, f) {
				t.Errorf("%s holds a credential as %q", where, f)
			}
		}
		if a, b := halves(s); halvesToo && strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
		if n := foldedAlnum(s); len(n) >= foldedMinAlnum && strings.Contains(folded, n) {
			t.Errorf("%s holds %q case-folded or re-separated", where, s)
		}
	}
	for _, s := range outOfContract() {
		if strings.Contains(text, s) {
			count(&outOfContractSeen, 1)
			break
		}
	}
}

// cliCall is one CLI run: its arguments and what it reads on stdin.
type cliCall struct {
	stdin string
	args  []string
}

func calls(argss ...[]string) []cliCall {
	out := make([]cliCall, len(argss))
	for i, a := range argss {
		out[i] = cliCall{args: a}
	}
	return out
}

// novels is every novel command: incidents mttr and agents stale read the
// local store a sync filled; triage, defender riskiest, office trend and
// suppression audit read the API live.
func novels() [][]string {
	return [][]string{
		{"triage", "--json"},
		{"triage"},
		{"triage", "--since", "72h", "--account-id", bbAccountID, "--json"},
		{"incidents", "mttr", "--json"},
		{"incidents", "mttr", "--since", "30d"},
		{"agents", "stale", "--json"},
		{"agents", "stale", "--since", "1d"},
		{"defender", "riskiest", "--json"},
		{"defender", "riskiest", "--top", "1"},
		{"office", "trend", "--json"},
		{"office", "trend"},
		{"suppression", "audit", "--json"},
		{"suppression", "audit", "--stale-after", "30d"},
		{"analytics", "--type", "agents", "--json"},
		{"analytics", "--type", "incidents", "--group-by", "status"},
	}
}

// battery runs every output path; tok is the token the process holds (the
// one it sends: ROCKETCYBER_API_TOKEN, the stored token, or a config file's
// api_token or access_token).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) []cliCall {
	b := calls(
		// Every output mode.
		[]string{"agents", "--json"},
		[]string{"agents"},
		[]string{"agents", "--csv"},
		[]string{"agents", "--plain"},
		[]string{"agents", "--agent"},
		[]string{"agents", "--json", "--receipt"},
		[]string{"agents", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		[]string{"agents", "--json", "--deliver", "file:" + deliverFile},
		// Every response shape: the account integration's OAuth credentials
		// (gzip), every agent's short password (gzip-encoded list), an
		// incident's api_key and credentials (BOM-prefixed) and an echoed
		// identifier, a firewall's secret and token and the firewalls split
		// across two pages, an app's access_token and the response headers,
		// the documented events, Defender and Office reports, a rule's split
		// echo, a 401 echo, refused redirects (off-host and to a homoglyph
		// host), the CSV report (text/csv and a download), RocketCyber data
		// under credential-like names.
		[]string{"account", "--json"},
		[]string{"account"},
		[]string{"account", "--csv"},
		[]string{"account", "--details", "--account-id", bbAccountID, "--json"},
		[]string{"incidents", "--json"},
		[]string{"incidents"},
		[]string{"incidents", "--csv"},
		[]string{"incidents", "--status", "open", "--json"},
		[]string{"incidents", "--title", bbDenied, "--json"},
		[]string{"events", "list", "--app-id", bbAppID, "--json"},
		[]string{"events", "list", "--app-id", bbAppID},
		[]string{"events", "summary", "--json"},
		[]string{"events", "summary"},
		[]string{"apps", "--json"},
		[]string{"apps"},
		[]string{"defender", "--json"},
		[]string{"defender"},
		[]string{"office", "--json"},
		[]string{"office"},
		[]string{"office", "--csv"},
		[]string{"firewalls", "--json"},
		[]string{"firewalls", "--counters", "--json"},
		[]string{"firewalls", "--page", "2", "--page-size", "1", "--json"},
		[]string{"suppression", "rules", "--json"},
		[]string{"suppression", "rules"},
		[]string{"suppression", "rule", bbRuleID, "--json"},
		[]string{"suppression", "rule", bbRuleID},
		[]string{"suppression", "rule", bbDenied, "--json"},
		[]string{"suppression", "rule", bbRedirect, "--json"},
		[]string{"suppression", "rule", bbHomoglyph, "--json"},
		[]string{"reports", "--item-type", "events", "--account-id", bbAccountID, "--app-id", "5", "--user-id", "3389"},
		[]string{"reports", "--item-type", "events", "--json"},
		[]string{"reports", "--item-type", "incidents", "--json"},
		[]string{"tail", "agents", "--follow=false"},
		[]string{"api"},
		[]string{"api", "suppression"},
		// The local store, exports and the novel commands.
		[]string{"sync", "--json"},
		[]string{"sync", "--resources", "suppression", "--json"},
		[]string{"workflow", "archive", "--json"},
		// incidents, not agents: agents is the export TestExportWalksEveryPage
		// pages through.
		[]string{"export", "incidents", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		[]string{"export", "agents", "--format", "json"},
		[]string{"export", "firewalls", "--format", "json"},
		[]string{"export", "suppression", bbRuleID, "--format", "json"},
		[]string{"search", "HQ", "--json"},
		[]string{"search", "Echo", "--json", "--data-source", "local"},
		[]string{"workflow", "status", "--json"},
	)
	b = append(b, calls(novels()...)...)
	return append(b, calls(
		// A sync that meets a 401 (a filtered walk into its own store, after
		// the novels: the main store stays whole).
		[]string{"sync", "--db", filepath.Join(e.home, "denied.db"), "--resources", "incidents", "--param", "title=" + bbDenied, "--json"},
		// A credential on the command line: a credential-named query value
		// (sent, and echoed back), the same in a dry run, an unknown flag, a
		// positional, feedback text.
		[]string{"sync", "--db", filepath.Join(e.home, "param.db"), "--resources", "apps", "--param", "password=" + bbArgvSec, "--json"},
		[]string{"sync", "--db", filepath.Join(e.home, "param.db"), "--resources", "apps", "--global-param", "password=" + bbArgvSec, "--dry-run"},
		[]string{"agents", "--json", "--" + tok},
		[]string{"suppression", "rule", tok, "--json"},
		[]string{"incidents", "--no-such-flag", tok},
		[]string{"feedback", "noted the key " + tok + " in a ticket"},
		// Previews: a dry run sends nothing.
		[]string{"incidents", "--title", "Echo " + tok, "--dry-run"},
		[]string{"suppression", "rule", tok, "--dry-run"},
		[]string{"agents", "--hostname", tok, "--dry-run"},
		[]string{"reports", "--item-type", "events", "--user-id", tok, "--dry-run"},
		// Learnings, playbooks, teach logs and profiles land in local files.
		[]string{"teach", "--query", "find agents " + tok, "--resource", bbAgentID, "--resource-type", "agents",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		[]string{"teach", "--query", "find " + tok, "--resource", tok, "--resource-type", "agents"}, // a held resource id: refused, logged to teach.log
		[]string{"teach-playbook", "--query", "agents with " + tok, "--notes", "use " + tok},
		[]string{"profile", "save", "held", "--description", "uses " + tok, "--select", "id"},
		[]string{"profile", "show", "held"},
		[]string{"profile", "list"},
		[]string{"feedback", "list"},
		[]string{"learnings", "list", "--json"},
		[]string{"recall", "--query", "find agents " + tok},
		[]string{"doctor"},
		[]string{"auth", "status"},
		[]string{"agent-context"},
	)...)
}

func tool(name string, args map[string]any) map[string]any {
	return map[string]any{"name": name, "arguments": args}
}

// confirmed is call with confirm set to v.
func confirmed(call map[string]any, v any) map[string]any {
	call["arguments"].(map[string]any)["confirm"] = v
	return call
}

// mcpReads are the read-only tools across every response shape the fake has,
// and every novel tool. The firewalls (the first half of a split credential,
// page 1 of two) run in their own session (TestMCPSplitAcrossPagesWithheld).
// In one session, an answer that repeats credentials can make later answers
// withheld whole (the P2 TestMCPSessionEchoWithholdsALaterAnswerKnownP2), so
// tests that check what a read returns run each read in its own session.
func mcpReads() []map[string]any {
	none := map[string]any{}
	return []map[string]any{
		tool("account_get", none),
		tool("account_get", map[string]any{"account-id": 2, "details": true}),
		tool("agents_list", none),
		tool("agents_list", map[string]any{"connectivity": "offline", "page-size": 50}),
		tool("incidents_list", none),
		tool("incidents_list", map[string]any{"title": bbDenied}),
		tool("events_list", map[string]any{"app-id": 60}),
		tool("events_summary", none),
		tool("apps_list", none),
		tool("defender_get", none),
		tool("office_get", none),
		tool("suppression_rules", none),
		tool("suppression_rule", map[string]any{"ruleId": 42}),
		tool("suppression_rule", map[string]any{"ruleId": 40199}),
		tool("reports_export", map[string]any{"item-type": "events"}),
		tool("reports_export", map[string]any{"item-type": "incidents"}),
		tool("context", none),
		// The novels run only with a flag (with none they print help: the
		// P2 TestMCPNovelWithoutArgumentsPrintsHelpKnownP2).
		tool("triage", map[string]any{"since": "72h"}),
		tool("incidents_mttr", map[string]any{"since": "90d"}),
		tool("agents_stale", map[string]any{"since": "7d"}),
		tool("defender_riskiest", map[string]any{"top": 5}),
		tool("office_trend", map[string]any{"account-id": 2}),
		tool("suppression_audit", map[string]any{"stale-after": "90d"}),
		tool("analytics", map[string]any{"type": "agents"}),
		tool("tail", map[string]any{"resource": "agents", "follow": false}),
		tool("search", map[string]any{"query": "HQ"}),
		tool("sql", map[string]any{"query": "SELECT data FROM resources"}),
		tool("sql", map[string]any{"query": "SELECT data FROM resources WHERE resource_type = 'incidents'"}),
		tool("recall", map[string]any{"query": "HQ"}),
		tool("learnings_list", none),
		tool("learnings_candidates", none),
		tool("learnings_stats", none),
		tool("playbook_list", none),
		tool("workflow_status", none),
	}
}

// mcpBypasses try to turn a read into something else: a write method, a
// confirm or yes, a dry run turned off, a raw path. Each must be refused or
// stay a read, and send nothing but GETs.
func mcpBypasses() []map[string]any {
	return []map[string]any{
		tool("agents_list", map[string]any{"method": "DELETE"}),
		confirmed(tool("suppression_rule", map[string]any{"ruleId": 42}), true),
		tool("suppression_rule", map[string]any{"ruleId": 42, "path": "/suppression/rule/42/delete"}),
		tool("sync", map[string]any{"resources": []any{"apps"}, "dry-run": false}),
		tool("sync", map[string]any{"resources": []any{"apps"}, "yes": true}),
		confirmed(tool("export", map[string]any{"resource": "apps", "format": "json"}), true),
	}
}

// mcpLocalWrites are mirrored tools that write only local files (no gate by
// design): their child CLI output passes the server's chokepoint too. The
// sync's credential-named query value is sent, and echoed back.
func mcpLocalWrites() []map[string]any {
	return []map[string]any{
		tool("export", map[string]any{"resource": "incidents", "format": "json"}),
		tool("sync", map[string]any{"resources": []any{"incidents", "suppression"}}),
		tool("sync", map[string]any{"resources": []any{"apps"}, "param": []any{"password=" + bbMCPArgSec}}),
	}
}

func mcpBattery() []map[string]any {
	return append(append(mcpReads(), mcpBypasses()...), mcpLocalWrites()...)
}

// scanFiles runs fn over every regular file under root but the credential
// store, the suite's own inputs excluded.
func scanFiles(root string, fn func(rel string, data []byte)) (scanned int) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), "credentials") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, "cfg"+string(filepath.Separator)) {
			return nil // the test's own inputs
		}
		scanned++
		fn(rel, data)
		return nil
	})
	return scanned
}

// checkOneAuth fails when a request authenticated any way but the one
// Authorization: Bearer header the mode sends.
func checkOneAuth(t *testing.T, sv *server, mode credMode) {
	t.Helper()
	sv.mu.Lock()
	defer sv.mu.Unlock()
	other := sv.otherAuth
	if mode == credEnvPrefixed {
		// P2 env-token-prefixed-twice: every request carries the doubled
		// scheme (counted as other auth by its shape); nothing else may.
		other = 0
		for _, s := range sv.otherAuthSeen {
			if !strings.HasPrefix(s, `1 Authorization header(s), scheme "Bearer"`) {
				other++
			}
		}
	}
	if other != 0 {
		t.Errorf("%d request(s) carried a credential another way (a second Authorization header, one not \"Bearer <token>\", X-Api-Key, a query token); RocketCyber takes only the Authorization header: %v", other, sv.otherAuthSeen)
	}
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, c := range battery(t, e, deliverFile, bbStored) {
		// credkit-wiring: operator text that repeats a held credential
		// reaches a file only as the kit's withhold notice (the file scan
		// below proves no credential is written; sawFeedbackWithheld that the
		// feedback entries really reached the sink).
		e.runIn(t, c.stdin, c.args...)
	}
	// Controls without a credential: the same sinks write, so the scans
	// below are not vacuous. Every battery answer echoes the held token, so
	// the kit withheld each of its store, export and delivery writes whole;
	// the data controls read the quiet fake instead, into the same home.
	quiet := quietServer(t)
	echoEnv := e.env
	e.env = append(withoutEnv(withoutEnv(e.env, "ROCKETCYBER_BASE_URL"), "ROCKETCYBER_CA_CERT"),
		"ROCKETCYBER_BASE_URL="+quiet.url, "ROCKETCYBER_CA_CERT="+quiet.caFile)
	e.run(t, "sync", "--json")
	e.run(t, "sync", "--resources", "suppression", "--json")
	e.run(t, "export", "incidents", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl"))
	e.run(t, "agents", "--json", "--deliver", "file:"+deliverFile)
	e.env = echoEnv
	e.run(t, "feedback", "control note HQ-Feedback")
	e.run(t, "teach", "--query", "find agents control", "--resource", bbAgentID, "--resource-type", "agents",
		"--notes", "rotated control", "--no-validate")
	e.run(t, "teach", "--resource", bbAgentID, "--resource-type", "agents") // no --query: teach.log records the refusal
	e.run(t, "profile", "save", "control", "--description", "HQ-Profile control", "--select", "id")
	e.mcpCalls(t, mcpBattery())
	if n, all := sv.sawKey(bbStored); n == 0 {
		t.Fatalf("no request carried the stored token as Authorization: Bearer (saw %d distinct values); the battery ran unauthenticated", len(all))
	}
	checkOneAuth(t, sv, credStored)
	var sawCreds bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "credentials") {
			return nil
		}
		data, _ := os.ReadFile(path)
		// The credential store: the one file allowed to hold a credential.
		sawCreds = sawCreds || bytes.Contains(data, []byte(bbStored))
		if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			rel, _ := filepath.Rel(e.home, path)
			t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
		}
		return nil
	})
	scanned := scanFiles(e.home, func(rel string, data []byte) {
		assertNoEchoIn(t, "file "+rel, string(data), !isStoreFile(rel))
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-token wrote no credential store")
	}
	// The battery really wrote data, so the scan above is not vacuous.
	if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte(bbHostname)) {
		t.Errorf("%s: want the delivered agents (err %v)", filepath.Base(deliverFile), err)
	}
	if b, err := os.ReadFile(filepath.Join(e.home, "export.jsonl")); err != nil || !bytes.Contains(b, []byte(bbIncTitle)) {
		t.Errorf("export.jsonl: want the exported incident (err %v):\n%.1500s", err, b)
	}
	var sawStore, sawAgents, sawIncident, sawRule, sawLearning, sawFeedback, sawFeedbackWithheld, sawProfile, sawTeachLog bool
	scanFiles(e.home, func(rel string, b []byte) {
		name := filepath.Base(rel)
		if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") {
			sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
			// The quiet control's synced records were stored.
			sawAgents = sawAgents || (bytes.Contains(b, []byte(bbHostname)) && bytes.Contains(b, []byte(bbHostname2)))
			sawIncident = sawIncident || bytes.Contains(b, []byte(bbIncTitle))
			// The opt-in suppression sync ran.
			sawRule = sawRule || bytes.Contains(b, []byte(bbRuleName))
			// The control teach run's learning row was written.
			sawLearning = sawLearning || bytes.Contains(b, []byte("rotated control"))
		}
		// The control feedback entry was written.
		sawFeedback = sawFeedback || (name == "feedback.jsonl" && bytes.Contains(b, []byte("HQ-Feedback")))
		sawFeedbackWithheld = sawFeedbackWithheld || (name == "feedback.jsonl" && bytes.Contains(b, []byte("[credkit: withheld from file")))
		sawProfile = sawProfile || bytes.Contains(b, []byte("HQ-Profile control"))
		sawTeachLog = sawTeachLog || (name == "teach.log" && bytes.Contains(b, []byte("missing --query")))
	})
	for what, ok := range map[string]bool{
		"no local store holds a fetched record":                                                            sawStore,
		"no local store holds both synced agents":                                                          sawAgents,
		"no local store holds the synced incident":                                                         sawIncident,
		"no local store holds the suppression rule; the opt-in suppression sync did not run":               sawRule,
		"no local store holds the taught learning; the teach run wrote nothing":                            sawLearning,
		"no feedback.jsonl holds the control feedback entry; the feedback run wrote nothing":               sawFeedback,
		"no feedback.jsonl holds a withheld entry; the credential-bearing feedback never reached the sink": sawFeedbackWithheld,
		"no profile file holds the control profile; profile save wrote nothing":                            sawProfile,
		"no teach.log holds the control refusal; teach wrote no log":                                       sawTeachLog,
	} {
		if !ok {
			t.Error(what)
		}
	}
	// Some pager walked every page: the split-across-pages echo really ran;
	// and the 401 echoes really ran (incidents --title, suppression rule, the
	// denied sync, MCP suppression_rule and incidents_list).
	sv.mu.Lock()
	page2 := sv.pages["/v3/firewalls?page=2"]
	denials := sv.denials
	sv.mu.Unlock()
	if denials < 5 {
		t.Errorf("only %d request(s) met a 401; the error-echo runs did not all reach the fake", denials)
	}
	if page2 == 0 {
		t.Errorf("no pager asked for the second page of the firewalls; the split-across-pages echo did not run")
	}
}

// Class secret-in-output, once per credential source the token can come from
// that is not the stored token alone: ROCKETCYBER_API_TOKEN over a stored
// token (held unused), a config file's api_token, a config file's
// access_token (sent as Bearer) and every config key (access_token,
// refresh_token, client_id, client_secret and a header held, the header sent
// as its own header). (The stored token as the sent value runs in
// TestNoCredentialInAnyWrittenFile; TestEveryCredentialSourceSendsOneKey
// covers every source with a short battery.)
func TestNoCredentialInAnyOutput(t *testing.T) {
	for _, mode := range []credMode{credEnvOverStored, credConfigFile, credConfigAccess, credConfigBoth} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, c := range battery(t, e, deliverFile, mode.sent()) {
				assertNoEcho(t, fmt.Sprintf("output of %v", c.args), e.runIn(t, c.stdin, c.args...))
			}
			// A held value no API receives, typed where the program repeats
			// it: a path the fake echoes, and previews.
			for _, held := range mode.held() {
				for _, args := range [][]string{
					{"suppression", "rule", held, "--json"},
					{"incidents", "--title", "HQ " + held, "--dry-run"},
					{"suppression", "rule", held, "--dry-run"},
					{"agents", "--hostname", held, "--dry-run", "--json"},
				} {
					assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
				}
			}
			// Every answer here echoes the held token, so the kit withholds
			// what carries it whole (credential leak contract section 4: no
			// partial redaction) and says so with its notice: the scans above
			// were not vacuous. The same reads print their data from the quiet
			// fake (TestRocketCyberPublicFieldsAreNotWithheld).
			for _, args := range [][]string{{"agents", "--json"}, {"incidents", "--json"}} {
				if out := e.run(t, args...); !strings.Contains(out, kitNotice("stdout")) {
					t.Fatalf("%v: want the kit's stdout notice for the echoed token, got:\n%.1500s", args, out)
				}
			}
			out := e.mcpCalls(t, mcpBattery())
			if replies := mcpReplies(out); len(replies) < len(mcpBattery())+1 {
				var missing []int
				for id := 1; id <= len(mcpBattery()); id++ {
					if _, ok := replies[id]; !ok {
						missing = append(missing, id)
					}
				}
				t.Fatalf("MCP session did not answer calls %v of %d (%v session deadline):\n%.2000s", missing, len(mcpBattery()), mcpSessionDeadline, out)
			}
			assertNoEcho(t, "MCP session", out)
			if !strings.Contains(out, kitNotice("mcp")) {
				t.Fatalf("the MCP session withheld nothing; the echoes did not reach it:\n%.2000s", out)
			}
			// The fake echoed the token it really received, so the scans
			// above covered it; no other credential source was sent.
			n, all := sv.sawKey(mode.sent())
			if n == 0 || len(all) != 1 {
				t.Fatalf("want every request to carry the %s token, got %d of it and %d distinct values", mode, n, len(all))
			}
			checkOneAuth(t, sv, mode)
		})
	}
}

// publicField matches name and value in JSON output, raw or inside a JSON
// string (an MCP result escapes its quotes).
func publicField(name, value string) *regexp.Regexp {
	return regexp.MustCompile(`\\*"` + regexp.QuoteMeta(name) + `\\*":\s*\\*"?` + regexp.QuoteMeta(value))
}

// publicReads are the reads whose answers carry RocketCyber data under names
// that read as credentials or identities.
func publicReads() ([][]string, []map[string]any) {
	return [][]string{
			{"account", "--json"},
			{"agents", "--json"},
			{"incidents", "--json"},
			{"events", "list", "--app-id", bbAppID, "--json"},
			{"apps", "--json"},
			{"defender", "--json"},
			{"office", "--json"},
			{"suppression", "rules", "--json"},
			{"suppression", "rule", bbRuleID, "--json"},
		}, []map[string]any{
			tool("account_get", map[string]any{}),
			tool("agents_list", map[string]any{}),
			tool("incidents_list", map[string]any{}),
			tool("events_list", map[string]any{"app-id": 60}),
			tool("apps_list", map[string]any{}),
			tool("defender_get", map[string]any{}),
			tool("office_get", map[string]any{}),
			tool("suppression_rules", map[string]any{}),
			tool("suppression_rule", map[string]any{"ruleId": 42}),
		}
}

// publicWant is the RocketCyber data publicReads must print.
func publicWant() []*regexp.Regexp {
	return []*regexp.Regexp{
		// account ids and names
		publicField("accountName", bbAccountName),
		publicField("name", bbAccountName),
		publicField("accountId", bbAccountID),
		publicField("customerId", bbCustomerID),
		publicField("accountName", bbClientName),
		// agents
		publicField("hostname", bbHostname),
		publicField("ipv4Address", "192.0.2.10"),
		publicField("platform", "Microsoft"),
		publicField("version", "10"),
		publicField("connectivity", "online"),
		publicField("id", bbAgentID),
		// incidents
		publicField("title", bbIncTitle),
		publicField("description", "HQ-Record detected and remediated a file on the agent"),
		publicField("remediation", "Review the detection."),
		publicField("status", "open"),
		publicField("id", bbIncidentID),
		// events, the documented details included
		publicField("verdict", "malicious"),
		publicField("value", "HQ-Record signin from 192.0.2.44"),
		publicField("valueType", "signin"),
		publicField("userPrincipalName", bbUPN),
		publicField("tokenIssuerType", "AzureAD"),
		publicField("Key", "userAgent"),
		publicField("Value", bbUserAgentUA),
		// apps
		publicField("name", "Advanced Breach Detection"),
		publicField("status", "active"),
		// Defender and Office posture
		publicField("hostname", bbHostname2),
		publicField("totalDevices", "66"),
		publicField("malicious", "60"),
		regexp.MustCompile(`\\*"secureScoreProgress\\*":\s*\{`),
		publicField("secureScorePercentage", "48.92"),
		publicField("averageScore", "42.29"),
		publicField("mfaStatus", "unknown"),
		publicField("id", bbMonitored),
		regexp.MustCompile(`\\*"secureScoreToDo\\*":\s*\{`),
		publicField("control", bbControl),
		publicField("maxScore", "50"),
		publicField("remediation", "HQ-Record: enable MFA for every admin"),
		// suppression rules
		publicField("ruleId", bbRuleID),
		publicField("ruleName", bbRuleName),
		publicField("comment", "HQ-Record printer spooler noise"),
		regexp.MustCompile(`\\*"conditions\\*":\s*\[`),
		regexp.MustCompile(`\\*"user\\*":\s*\{[^{}]*` + regexp.QuoteMeta(bbRuleUser)),
	}
}

// RocketCyber data under names that read as credentials or carry identities
// (the retired rocketcyber-credential-names-exact; the kit withholds by value,
// never by name) prints as the API returned it, from the
// CLI and MCP alike, and the store keeps it (sync walks the agents, apps,
// firewalls and incidents, and the opt-in suppression rules): account ids and
// names, agent inventory, incidents, the documented event details (a
// tokenIssuerType, the additionalInfo Key/Value pair, a userPrincipalName),
// app names, the Defender posture and the Office posture (secureScoreProgress,
// secureScorePercentage, secureScoreToDo, a monitored account's mfaStatus), a
// suppression rule's conditions, comment and user.
func TestRocketCyberPublicFieldsAreNotWithheld(t *testing.T) {
	// The quiet fake: RocketCyber's answers, repeating nothing they were sent
	// (an answer that echoes the held token is withheld whole by contract).
	sv := quietServer(t)
	e := newEnv(t, sv, credEnv)
	cliReads, mcpReadCalls := publicReads()
	results := map[string]string{}
	for _, args := range cliReads {
		results["CLI "+strings.Join(args, " ")] = e.run(t, args...)
	}
	// One session per read: in one session an answer that repeats
	// credentials can make later answers withheld whole (the P2
	// TestMCPSessionEchoWithholdsALaterAnswerKnownP2).
	for _, call := range mcpReadCalls {
		results[fmt.Sprintf("MCP %s %v", call["name"], call["arguments"])] = mcpReplies(e.mcpCalls(t, []map[string]any{call}))[1]
	}
	var cli, mcp strings.Builder
	for where, got := range results {
		if strings.HasPrefix(where, "CLI") {
			cli.WriteString(got)
		} else {
			mcp.WriteString(got)
		}
		assertNoEcho(t, where, got)
	}
	for where, got := range map[string]string{"CLI": cli.String(), "MCP": mcp.String()} {
		for _, want := range publicWant() {
			if !want.MatchString(got) {
				t.Errorf("%s withheld RocketCyber data (want %s)", where, want)
			}
		}
	}
	if t.Failed() {
		for where, got := range results {
			t.Logf("%s:\n%.2500s", where, got)
		}
	}
	// The store keeps it too: the agents, the incidents and the rule a sync
	// stored.
	e.run(t, "sync", "--json")
	e.run(t, "sync", "--resources", "suppression", "--json")
	var agents, incidents, rules bool
	scanFiles(e.home, func(rel string, b []byte) {
		if isStoreFile(rel) {
			agents = agents || (bytes.Contains(b, []byte(bbHostname)) && bytes.Contains(b, []byte("192.0.2.10")))
			incidents = incidents || bytes.Contains(b, []byte(bbIncTitle))
			rules = rules || (bytes.Contains(b, []byte(bbRuleName)) && bytes.Contains(b, []byte(bbRuleUser)))
		}
	})
	if !agents {
		t.Errorf("the local store dropped the agents' hostnames and addresses")
	}
	if !incidents {
		t.Errorf("the local store dropped the incident titles")
	}
	if !rules {
		t.Errorf("the local store dropped the suppression rule or its user")
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host, or to a homoglyph spelling of it, is refused before it is followed,
// from the CLI, sync and MCP alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	before := len(trapDials())
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	redirect := e.run(t, "suppression", "rule", bbRedirect, "--json")
	homoglyph := e.run(t, "suppression", "rule", bbHomoglyph, "--json")
	sv.mu.Lock()
	sv.redirectAgents = true
	sv.mu.Unlock()
	syncOut := e.run(t, "sync", "--resources", "agents", "--json")
	mcp := e.mcpCalls(t, []map[string]any{
		tool("suppression_rule", map[string]any{"ruleId": 9911}),
		tool("suppression_rule", map[string]any{"ruleId": 9922}),
		tool("agents_list", map[string]any{}),
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		if strings.HasSuffix(m, "/v3/suppression/rule/"+bbRedirect) || strings.HasSuffix(m, "/v3/suppression/rule/"+bbHomoglyph) ||
			strings.HasSuffix(m, "GET /v3/agents") {
			redirected++
		}
	}
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if d := trapDials(); len(d) != before {
		t.Fatalf("a redirect dialed out (through the proxy trap): %v", d[before:])
	}
	if redirected < 6 {
		t.Fatalf("only %d request(s) met a redirect; the CLI, sync and MCP calls did not all run", redirected)
	}
	// Each run says the redirect was refused: the client's redirect policy
	// (refusing redirect) or the kit's transport (a held credential carried
	// to an origin it is not allowed).
	for where, out := range map[string]string{"suppression rule (off-host)": redirect, "suppression rule (homoglyph)": homoglyph, "sync": syncOut} {
		if c := redirectRefusals(out); c == 0 {
			t.Fatalf("%s: want the redirect refused, got:\n%.1500s", where, out)
		}
	}
	if c := redirectRefusals(mcp); c < 3 {
		t.Fatalf("MCP: want the redirect refused three times, got %d:\n%.3000s", c, mcp)
	}
	assertNoEcho(t, "redirect runs", redirect+homoglyph+syncOut+mcp)
}

// redirectRefusals counts the redirect refusals in out: the client's redirect
// policy says "refusing redirect" (once or twice in one message), the kit's
// transport writes its outbound notice.
func redirectRefusals(out string) int {
	n := strings.Count(out, kitNotice("outbound request"))
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "refusing redirect") {
			n++
		}
	}
	return n
}

// Class credential-sent-to-another-host, origin form (api-origin-pinned):
// the token goes only to the origin of the configured base URL, whatever a
// later input says. A path argument or an MCP parameter shaped like a URL
// (absolute, scheme-relative, userinfo, a dot segment) stays a path on that
// origin; and a configured base URL that does not resolve to one origin (an
// unresolved placeholder, a homoglyph host) pins nothing, so every request
// is refused before any dial. The proxy trap stands in for the network: a
// request that got past the pin would reach it, never a real host.
func TestOriginPinNeverMovesTheKey(t *testing.T) {
	before := len(trapDials())
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	hostile := []string{"https://evil.example.com/x", "//evil.example.com/x", "@evil.example.com", "..%2F..%2F..%2Fevil", "evil.example.com:443",
		"../../../evil", "%2F%2Fevil.example.com"}
	var out strings.Builder
	for _, id := range hostile {
		out.WriteString(e.run(t, "suppression", "rule", id, "--json"))
		out.WriteString(e.run(t, "export", "suppression", id, "--format", "json"))
		out.WriteString(e.run(t, "agents", "--hostname", id, "--json"))
	}
	var mcpCalls []map[string]any
	for _, id := range hostile {
		mcpCalls = append(mcpCalls, tool("suppression_rule", map[string]any{"ruleId": id}),
			tool("agents_list", map[string]any{"hostname": id, "id": id}))
	}
	out.WriteString(e.mcpCalls(t, mcpCalls))
	// Every request reached the fake under the configured /v3 path.
	var api int
	for _, m := range sv.allMethods() {
		if strings.Contains(m, " /v3/") {
			api++
		}
	}
	if api == 0 {
		t.Errorf("no request reached the configured origin; requests: %v", sv.allMethods())
	}
	// A base URL holding an unresolved placeholder, or a homoglyph of the
	// API host, pins nothing: every request is refused before any dial (by
	// the kit's transport, which withholds a held credential bound for an
	// origin it is not allowed, or earlier, by the request builder that
	// cannot read the host). The proxy trap below proves no dial.
	refused := regexp.MustCompile(regexp.QuoteMeta(kitNotice("outbound request")) + `|invalid character \\?"\{\\?" in host name|must be an absolute URL with a host|invalid URL|non-ASCII`)
	for _, base := range []string{"https://{tenant}.api-us.rocketcyber.com/v3", "https://api-us.rocketcyb\u0435r.com/v3"} {
		home := t.TempDir()
		cfg := filepath.Join(home, "pinned.toml")
		if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
			t.Fatal(err)
		}
		pe := &bbEnv{home: home, env: cleanEnv(home, "ROCKETCYBER_API_TOKEN="+bbToken, "ROCKETCYBER_CONFIG="+cfg, "ROCKETCYBER_CLI_PATH="+bbCLI,
			"ROCKETCYBER_MCP_HTTP_TOKEN="+bbHTTPToken)}
		for _, args := range [][]string{
			{"agents", "--json"},
			{"suppression", "rule", bbRuleID, "--json"},
			{"office", "trend", "--json"},
		} {
			got := pe.run(t, args...)
			if !refused.MatchString(got) {
				t.Errorf("%v with base URL %q: want a refusal before any dial, got:\n%.1000s", args, base, got)
			}
			out.WriteString(got)
		}
		mcp := pe.mcpCalls(t, []map[string]any{
			tool("agents_list", map[string]any{}),
			tool("suppression_rule", map[string]any{"ruleId": 42}),
		})
		if len(refused.FindAllString(mcp, -1)) < 2 {
			t.Errorf("MCP reads with base URL %q: want a refusal twice, got:\n%.2000s", base, mcp)
		}
		out.WriteString(mcp)
	}
	if d := trapDials(); len(d) != before {
		t.Errorf("a request got past the pin and dialed out (through the proxy trap): %v", d[before:])
	}
	sv.mu.Lock()
	if sv.offHost != 0 {
		t.Errorf("the other host was contacted %d time(s)", sv.offHost)
	}
	sv.mu.Unlock()
	assertNoEcho(t, "origin-pin runs", out.String())
	checkOneAuth(t, sv, credEnv)
}

// mcpSplitHalf is the page-1 read of the firewalls that split the first
// credential across two pages.
func mcpSplitHalf() map[string]any { return tool("firewalls_list", map[string]any{}) }

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm, read-only API:
// every request the MCP server and its mirrored CLI children send to the API
// is a read (a GET), across every tool and every attempt to turn a read into
// something else (a method argument, confirm or yes, a dry run turned off,
// a raw path). The bypass attempts are refused or stay reads; the reads and
// the novels answer from the fake and the synced store (a refusal would
// leave the leak checks vacuous for them).
func TestMCPSendsOnlyReads(t *testing.T) {
	// The quiet fake: the reads must answer their data, which an answer
	// echoing the held token never does (withheld whole by contract); the
	// MCP leak checks run against the echoing fake in TestNoCredentialInAnyOutput.
	sv := quietServer(t)
	e := newEnv(t, sv, credEnv)
	// The store holds the synced account first, so the store novels have
	// something to read.
	e.run(t, "sync", "--json")
	sv.resetMethods()
	// Each read in its own session: an answer that echoes credentials makes
	// the session withhold some later answers whole (the P2
	// TestMCPSessionEchoWithholdsALaterAnswerKnownP2), which would leave
	// this check reading notices.
	for _, call := range mcpReads() {
		want := map[any]string{
			"account_get": bbAccountName, "agents_list": bbHostname2, "incidents_list": bbIncTitle, "events_list": bbUPN,
			"apps_list": "Advanced Breach Detection", "defender_get": bbHostname2, "office_get": "secureScorePercentage",
			"suppression_rules": bbRuleName, "suppression_rule": bbRuleName, "triage": bbIncTitle, "incidents_mttr": "mttr_hours_mean",
			"agents_stale": bbHostname2, "defender_riskiest": bbHostname2, "office_trend": "improving", "suppression_audit": bbRuleName,
			"context": "rocketcyber", "analytics": "agents", "search": "HQ-Record", "sql": "HQ-Record",
		}[call["name"]]
		// A denied read answers an error; a CSV report that repeats a
		// credential is withheld whole (it has no member to redact; the clean
		// report is TestReportsCSVPrintsItsRows); so is the whole store, which
		// keeps the firewalls' two halves of the token in two records (the
		// ruled P2 TestStoreSplitAcrossRecordsKnownP2).
		args, _ := call["arguments"].(map[string]any)
		if args["title"] == bbDenied || args["ruleId"] == 40199 || call["name"] == "reports_export" || args["query"] == "SELECT data FROM resources" {
			want = ""
		}
		r := mcpReplies(e.mcpCalls(t, []map[string]any{call}))[1]
		if r == "" || (want != "" && !strings.Contains(r, want)) {
			t.Errorf("%s %v read nothing (want %q):\n%.1500s", call["name"], call["arguments"], want, r)
		}
	}
	rest := append(mcpBypasses(), mcpLocalWrites()...)
	replies := mcpReplies(e.mcpCalls(t, rest))
	for i, call := range rest {
		if replies[i+1] == "" {
			t.Errorf("%s %v was not answered", call["name"], call["arguments"])
		}
	}
	for _, m := range sv.allMethods() {
		if isWrite(m) {
			t.Errorf("the MCP server sent %s", m)
		}
	}
	checkOneAuth(t, sv, credEnv)
}

// localWriteTools are the tools that write only the server's own local files
// (the store, learnings, playbooks): no RocketCyber data changes, so they
// carry no confirm gate by design.
var localWriteTools = map[string]bool{
	"export": true, "sync": true, "workflow_archive": true, "learnings_confirm": true, "learnings_forget": true, "learnings_reject": true,
	"playbook_amend": true, "teach": true, "teach_lookup": true, "teach_pattern": true, "teach_playbook": true,
}

// vendorTools are the tools that read the RocketCyber Customer API.
var vendorTools = []string{"account_get", "agents_list", "apps_list", "defender_get", "events_list", "events_summary", "firewalls_list",
	"incidents_list", "office_get", "reports_export", "suppression_rule", "suppression_rules"}

// mcp-endpoint-write-confirm, fail-closed form for a read-only API: every
// tool the server lists either carries the read-only hint or is one of the
// local-write tools above, every vendor tool is read-only, and no tool sends
// anything but a GET. A write a later print adds (a tool without the
// read-only hint that is not a local write) fails here until it carries a
// confirm gate. Control: each local-write tool really runs (not refused) and
// writes only local files.
func TestMCPToolsAreReadOnlyOrLocal(t *testing.T) {
	// The quiet fake: the controls need the export to carry its data, which an
	// answer echoing the held token never does (withheld whole by contract).
	sv := quietServer(t)
	e := newEnv(t, sv, credEnv)
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}})
	cmd := exec.Command(bbMCP)
	cmd.Env, cmd.Dir, cmd.Stdin = e.env, e.home, &in
	raw, _ := cmd.Output()
	var list struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Annotations struct {
					ReadOnly *bool `json:"readOnlyHint"`
				} `json:"annotations"`
				InputSchema struct {
					Properties map[string]any `json:"properties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, `"id":1`) {
			_ = json.Unmarshal([]byte(line), &list)
		}
	}
	if len(list.Result.Tools) < 20 {
		t.Fatalf("tools/list returned %d tools:\n%.2000s", len(list.Result.Tools), raw)
	}
	listed := map[string]bool{}
	for _, tl := range list.Result.Tools {
		listed[tl.Name] = true
		ro := tl.Annotations.ReadOnly != nil && *tl.Annotations.ReadOnly
		_, gated := tl.InputSchema.Properties["confirm"]
		if !ro && !localWriteTools[tl.Name] && !gated {
			t.Errorf("tool %s is neither read-only, a local write, nor confirm-gated", tl.Name)
		}
		if ro && localWriteTools[tl.Name] {
			t.Errorf("local-write tool %s claims the read-only hint", tl.Name)
		}
	}
	for _, name := range vendorTools {
		if !listed[name] {
			t.Errorf("vendor tool %s is not listed", name)
		}
	}
	for _, tl := range list.Result.Tools {
		for _, name := range vendorTools {
			if tl.Name == name && (tl.Annotations.ReadOnly == nil || !*tl.Annotations.ReadOnly) {
				t.Errorf("vendor tool %s lacks the read-only hint", name)
			}
		}
	}
	// Control: every local-write tool runs, writes only local files and
	// sends only reads.
	sv.resetMethods()
	out := e.mcpPhases(t,
		[]map[string]any{tool("sync", map[string]any{"resources": []any{"agents", "incidents"}})},
		[]map[string]any{
			tool("teach", map[string]any{"query": "find stale agents", "resource-type": "agents", "resource": []any{bbAgent2}, "notes": "HQ-Record control", "no-validate": true}),
			tool("teach_playbook", map[string]any{"query": "find stale agents", "notes": "HQ-Record playbook"}),
			tool("teach_lookup", map[string]any{"kind": "agent", "canonical": bbHostname2, "value": bbAgent2}),
			tool("teach_pattern", map[string]any{"query-template": "agents on {host}", "resource-template": "{host}", "entity-kind": "agent", "resource-type": "agents"}),
			tool("workflow_archive", map[string]any{}),
			tool("export", map[string]any{"resource": "apps", "format": "json"}),
		},
		[]map[string]any{
			tool("playbook_amend", map[string]any{"query": "find stale agents", "add-note": "HQ-Record amended"}),
			tool("learnings_forget", map[string]any{"query": "find nothing at all"}),
			tool("learnings_confirm", map[string]any{"id": "1"}),
			tool("learnings_reject", map[string]any{"id": "2"}),
		})
	replies := mcpReplies(out)
	if r := replies[7]; !strings.Contains(r, "Advanced Breach Detection") {
		t.Errorf("the export tool exported nothing:\n%.1500s", r)
	}
	var sawLearning bool
	scanFiles(e.home, func(rel string, b []byte) {
		sawLearning = sawLearning || (isStoreFile(rel) && bytes.Contains(b, []byte("HQ-Record control")))
	})
	if !sawLearning {
		t.Errorf("the teach tool wrote no learning; the local writes did not run:\n%.3000s", out)
	}
	for _, m := range sv.allMethods() {
		if isWrite(m) {
			t.Errorf("a local-write tool sent %s", m)
		}
	}
	assertNoEcho(t, "MCP local writes", out)
}

// dry-run-sends-nothing: a --dry-run never reaches the API, from a cold start
// with the token in the environment, the credential store or a config file
// (api_token, or access_token alone).
func TestDryRunSendsNothing(t *testing.T) {
	for _, mode := range []credMode{credEnv, credStored, credConfigFile, credConfigAccess} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		for _, args := range [][]string{
			{"agents", "--dry-run"},
			{"agents", "--hostname", "dry", "--dry-run", "--json"},
			{"account", "--dry-run"},
			{"incidents", "--status", "open", "--dry-run"},
			{"events", "list", "--app-id", bbAppID, "--dry-run"},
			{"events", "summary", "--dry-run"},
			{"apps", "--dry-run"},
			{"defender", "--dry-run"},
			{"office", "--dry-run"},
			{"firewalls", "--page", "2", "--dry-run"},
			{"suppression", "rules", "--dry-run"},
			{"suppression", "rule", bbRuleID, "--dry-run"},
			{"reports", "--item-type", "events", "--dry-run"},
			{"sync", "--resources", "incidents", "--dry-run"},
			{"export", "agents", "--format", "json", "--dry-run"},
			{"triage", "--dry-run"},
			{"defender", "riskiest", "--dry-run"},
			{"office", "trend", "--dry-run"},
			{"suppression", "audit", "--dry-run"},
		} {
			// Each run reaches its preview, so the count below is not vacuous
			// (a flag error would send nothing either).
			if out := e.run(t, args...); !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") && !strings.Contains(out, "would ") &&
				!strings.Contains(out, "--dry-run") {
				t.Errorf("%v (mode %s) printed no preview:\n%.1500s", args, mode, out)
			}
		}
		if m := sv.allMethods(); len(m) != 0 {
			t.Errorf("--dry-run (mode %s) sent %v", mode, m)
		}
		// Control: the same reads without --dry-run reach the fake.
		e.run(t, "suppression", "rule", bbRuleID, "--json")
		e.run(t, "agents", "--json")
		// (A list read first refreshes the head of its synced resource:
		// auto_refresh.go, one more GET of the same list.)
		m := sv.allMethods()
		ok := len(m) >= 2 && m[0] == "GET /v3/suppression/rule/"+bbRuleID
		for _, x := range m[min(1, len(m)):] {
			ok = ok && x == "GET /v3/agents"
		}
		if !ok {
			t.Errorf("control (mode %s): want the rule GET then the agents GETs, got %v", mode, m)
		}
	}
}

// credential-split-across-output-records over MCP: the fake splits the first
// credential it was sent across the two pages of the firewalls, one half
// each. One session reads both pages; the halves must never both reach the
// client.
func TestMCPSplitAcrossPagesWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{mcpSplitHalf()},
		[]map[string]any{tool("firewalls_list", map[string]any{"page": 2})})
	if !strings.Contains(out, `"id":2`) {
		t.Fatalf("MCP session did not answer both pages:\n%.2000s", out)
	}
	sv.mu.Lock()
	page2 := sv.pages["/v3/firewalls?page=2"]
	sv.mu.Unlock()
	if page2 == 0 {
		t.Fatalf("the second page was never asked for; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
}

// P2 (adversarial response shape, over-withholding, not a leak): once one MCP
// result carrying half a held credential is delivered (a firewall whose id is
// half the API token), the history check spells the other half from chance
// fragments of later output, so later unrelated results of the same session
// (here the apps, after the agents) are withheld whole. Runs only with
// ROCKETCYBER_BB_P2=1; fails while the P2 stands. On this print it does not
// stand (seen/bb-p2-standing.log in the run dir: it passes); it stays as the
// donor's guard.
func TestMCPSessionHalfWithholdsLaterResultsKnownP2(t *testing.T) {
	if os.Getenv("ROCKETCYBER_BB_P2") == "" {
		t.Skip("P2 session-wide withholding after a delivered half; set ROCKETCYBER_BB_P2=1 to reproduce")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{mcpSplitHalf()},
		[]map[string]any{tool("agents_list", map[string]any{})},
		[]map[string]any{tool("apps_list", map[string]any{})})
	if r := mcpReplies(out)[3]; !strings.Contains(r, "Advanced Breach Detection") {
		t.Errorf("the apps after a delivered half were withheld:\n%.1500s", r)
	}
}

// P2 (adversarial response shape, over-withholding, not a leak): an MCP
// result is compact one-line JSON, so when a record splits a held token
// across two adjacent values (the fake's "split" member), the whitespace
// token that holds both halves runs from the last space before them to the
// first space after, and the public fields between them are withheld with
// it. The CLI's indented output keeps them. Runs only with
// ROCKETCYBER_BB_P2=1; fails while the P2 stands.
func TestMCPSplitRecordWithholdsItsPublicFieldsKnownP2(t *testing.T) {
	if os.Getenv("ROCKETCYBER_BB_P2") == "" {
		t.Skip("P2 compact MCP result loses public fields beside a split; set ROCKETCYBER_BB_P2=1 to reproduce")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.mcpCalls(t, []map[string]any{tool("suppression_rule", map[string]any{"ruleId": 42})})
	for _, want := range []*regexp.Regexp{publicField("ruleName", bbRuleName), publicField("comment", "HQ-Record printer spooler noise")} {
		if !want.MatchString(out) {
			t.Errorf("suppression_rule withheld %s beside a split token:\n%.2000s", want, out)
		}
	}
	assertNoEcho(t, "MCP suppression_rule", out)
}

// The work order's ruled P2 credential-split-across-sql-bindings, kept
// reproducible: a paginated walk writes each page through to the local store
// as its own record, and the store checks each record alone (output streams
// and file sinks check a write with what they already delivered; the SQLite
// store keeps no such history), so the two halves of a held token that
// arrive on two pages land in two rows of the store file. The output
// withholds both pages. Runs only with ROCKETCYBER_BB_P2=1; fails while the
// P2 stands.
func TestStoreSplitAcrossRecordsKnownP2(t *testing.T) {
	if os.Getenv("ROCKETCYBER_BB_P2") == "" {
		t.Skip("ruled P2 credential-split-across-sql-bindings; set ROCKETCYBER_BB_P2=1 to reproduce")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	out := e.run(t, "sync", "--resources", "firewalls", "--json")
	assertNoEcho(t, "sync output", out)
	var stores int
	scanFiles(e.home, func(rel string, data []byte) {
		if isStoreFile(rel) {
			stores++
			assertNoEcho(t, "file "+rel, string(data))
		}
	})
	if stores == 0 {
		t.Fatal("the walk wrote no local store; the test did not run")
	}
}

// P2 (adversarial response shape, over-withholding, not a leak): in one MCP
// session, after the incident list (an api_key, a credentials container and
// an identifier that each repeat a credential) is delivered, the account
// answer (OAuth credentials and the echo, gzip-encoded) is withheld whole
// "as an authentication error", though alone, or after any other read, its
// public data prints with the credentials withheld in place. RocketCyber
// documents no credential in either answer. Runs only with
// ROCKETCYBER_BB_P2=1; fails while the P2 stands.
func TestMCPSessionEchoWithholdsALaterAnswerKnownP2(t *testing.T) {
	if os.Getenv("ROCKETCYBER_BB_P2") == "" {
		t.Skip("P2 session withholds the account answer whole after the incident list; set ROCKETCYBER_BB_P2=1 to reproduce")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{tool("incidents_list", map[string]any{})},
		[]map[string]any{tool("account_get", map[string]any{})})
	if r := mcpReplies(out)[2]; !strings.Contains(r, bbAccountName) {
		t.Errorf("the account answer after the incident list was withheld whole:\n%.1500s", r)
	}
	assertNoEcho(t, "MCP session", out)
}

// P2 (MCP usability, not a credential class): the novels triage, incidents
// mttr, agents stale, defender riskiest and suppression audit print their
// help when run with no flag at all (their RunE: len(args) == 0 &&
// NFlag() == 0 -> Help), and the MCP mirror passes no flag for a call with
// no arguments, so the tool answers "command printed help instead of
// running" instead of the board. The fix is in the novels (or the mirror
// adding --agent), outside this suite's files. Runs only with
// ROCKETCYBER_BB_P2=1; fails while the P2 stands.
func TestMCPNovelWithoutArgumentsPrintsHelpKnownP2(t *testing.T) {
	if os.Getenv("ROCKETCYBER_BB_P2") == "" {
		t.Skip("P2 novels print help for an MCP call with no arguments; set ROCKETCYBER_BB_P2=1 to reproduce")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	e.run(t, "sync", "--json")
	novels := []string{"triage", "incidents_mttr", "agents_stale", "defender_riskiest", "suppression_audit"}
	var calls []map[string]any
	for _, n := range novels {
		calls = append(calls, tool(n, map[string]any{}))
	}
	replies := mcpReplies(e.mcpCalls(t, calls))
	for i, n := range novels {
		if r := replies[i+1]; strings.Contains(r, "printed help instead of running") {
			t.Errorf("%s with no arguments printed help:\n%.300s", n, r)
		}
	}
}
