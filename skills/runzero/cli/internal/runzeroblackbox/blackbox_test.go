// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package runzeroblackbox runs the built CLI and MCP binaries against an
// in-process runZero API fake and checks what a reviewer cannot read
// off the code (runzero handfixes output-chokepoint,
// store-never-keeps-secrets, no-response-cache, file-sinks-withheld,
// response-credentials-held, api-origin-pinned, dry-run-sends-nothing,
// mcp-endpoint-write-confirm, mcp-mirror-write-confirm,
// runzero-credential-names-exact):
//
//   - the runZero API v1.0 (the /api/v1.0 path prefix is part of the base
//     URL, so RUNZERO_BASE_URL is the fake's origin plus /api/v1.0; spec.yaml,
//     the vendor's OpenAPI 3.0.0): lists are bare JSON arrays (/org/assets,
//     /org/services, /org/sites, /org/tasks, /account/keys, /account/orgs,
//     /account/users, /account/credentials, ...); the exports
//     (/export/org/{assets,services,software,vulnerabilities,certificates,
//     wireless,sites,...}.json) and the event log (/account/events.json) are
//     bare arrays, and with page_size a page {"assets"|"events":[...],
//     "next_key":"..."} the next start_key continues;
//     service, software and vulnerability rows embed their asset (`id` is the
//     asset's id, the row key is service_id, software_id, vulnerability_id);
//     PUT /org/sites/{site_id}/scan answers the scan Task, GET
//     /org/tasks/{task_id} its state; authenticated with the one header
//     Authorization: Bearer <key> (an account CT, organization OT or export
//     ET key, all sent the same way). /health and /releases/* declare
//     security: [] in the spec.
//
// runZero, unlike most vendors, returns credentials, and the fake answers
// every one it documents: an API key's token (GET /account/keys,
// /account/keys/{key_id}, /org/key, PATCH /account/keys/{key_id}/rotate),
// an organization's export_token and download_token (GET /account/orgs,
// /account/orgs/{org_id}), an export token's token
// (/account/orgs/{org_id}/exportTokens), the access_token POST
// /account/api/token mints and the api_client_secret POST
// /account/api/rotate returns. It also echoes every credential the process
// holds back in every encoding it can, whole and split across lines, adjacent
// values, members and pages: what the request carried (the key and its
// "Bearer <key>" header value), plus generic credential-named members (a
// user's password in a gzip-encoded list, an api_key in a gzip-encoded
// export, a token in a BOM-prefixed site list, an asset whose name repeats the
// credential), in a response header, a WWW-Authenticate challenge and every
// redirect's Location. The suite checks that:
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the key from RUNZERO_API_KEY, from the credential
//     store (auth set-token, stdin; a key typed as its argument by mistake),
//     from a config file's api_key, access_token or auth_header ("Bearer
//     <key>"), a key the configuration holds but does not send (a stored key
//     or a config file's api_key the environment overrides, a config file's
//     access_token beside its api_key), the MCP HTTP token, a
//     credential-named query value given on the command line or as an MCP
//     argument, and the credentials a response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     feedback log, the learn journal, profiles, teach logs);
//   - no credential leaves for a host other than the configured one: a
//     redirect to another host, another port, plain http or a non-ASCII
//     homograph of console.runzero.com is refused before it is followed, and
//     an off-origin absolute URL in an export page's next_key is sent to the
//     pinned host as a start_key, never dialled;
//   - the MCP server sends a write (runzero_execute, import, scan_watch, the
//     cve_blast_radius_during_an_incident recipe) only when the call sets
//     confirm to the boolean true, and --dry-run sends nothing.
//
// runZero's asset, service, site, task, credential and key IDs, addresses,
// MACs, hostnames, OS, ports, protocols, CVE ids, severities, scores, the
// certificate subject, cn, issuer, validity_end, fp_sha256 and
// signature_algorithm, token_type, export_token_last_used_by and
// last_used_ip (IPs), timestamps, counters, Wireless.authentication
// ("wpa2-psk") and a credential's insecure flag are never withheld: an MSP
// acts on them.
//
// Every request any process makes to a non-loopback host goes through a
// proxy trap (HTTPS_PROXY), so no test can reach a real runZero host; the
// suite fails if the trap ever sees one.
//
// The oracle is the exact set of strings the fake sent, so a pass does not
// depend on the program's own decoder.
package runzeroblackbox

import (
	"bytes"
	"compress/gzip"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"runzero-pp-cli/internal/credkit"
)

// apiBase is the path prefix of every runZero API path (part of the base URL).
const apiBase = "/api/v1.0"

const (
	// The credentials the process is given, one per source.
	bbKey        = "CT0lyurIer8NpgQjFrecCy0ArZsoJGyx"   // gitleaks:allow synthetic test fixture; RUNZERO_API_KEY (Authorization: Bearer <key>)
	bbStored     = "OTYrUmoJkWyzxyb15LQgva4wcSVh0z"     // auth set-token (stdin), the credential store
	bbFileKey    = "CT42PmbJgxQkBga07AsFHNcoVZ3dqe"     // gitleaks:allow synthetic test fixture; api_key in a 0600 config file named by RUNZERO_CONFIG
	bbFileAccess = "cqNtfXlTEZAe3TGca7k2Li6auH0M"       // gitleaks:allow synthetic test fixture; access_token beside api_key in a config file (held, never sent)
	bbFileOnly   = "ETic0qUbK6fmV15c5IemsidhN6MYhy"     // access_token alone in a config file (sent as Bearer)
	bbFileHeader = "ETqnoZg1wBaW1eQrDTWa3OOzty1VXI"     // auth_header = "Bearer <key>" in a config file (sent as written)
	bbArgvSec    = "-ePzI9SJ6q0rqWffTwuB"               // a credential-named query value on the command line (sync --param password=...; starts with "-")
	bbMCPArgSec  = "kcTco7HbLtCxJxh6X9JG"               // a credential-named query value an MCP sync call sends (param password=...)
	bbHTTPToken  = "wBoIFpg9YLzVxopyW3mFzo3OjNVEGIQPqq" // gitleaks:allow synthetic test fixture; RUNZERO_MCP_HTTP_TOKEN
	// Generic credential-named members a response carries.
	bbShortResp = "Sh0rtPw"             // a 7-character password every user carries (GET /account/users, gzip-encoded)
	bbGzipResp  = "irKuZGaqsKgtRFqyMDg" // an api_key a service row carries (GET /export/org/services.json, gzip-encoded)
	bbBOMResp   = "dn4fQyu91G0En3HIS"   // a token a site carries (GET /org/sites, BOM-prefixed)
	// The credentials runZero documents in its responses.
	bbKeyToken      = "CTxUoCDrIGEwgor2EA3OuFXVBgZCa8fC"   // gitleaks:allow synthetic test fixture; OrganizationAPIKey.token (GET /account/keys, /account/keys/{key_id})
	bbKey2Token     = "CTPT2rsXGgDSecSJsEyyRUW3mFoSK8"     // gitleaks:allow synthetic test fixture; the second listed key's token
	bbOrgKeyToken   = "OTRoFhd7qbWhqTXlltvOJwMMheH3PZ"     // gitleaks:allow synthetic test fixture; the organization key's token (GET /org/key)
	bbRotatedKey    = "CT5tUWh9FzrDWN8sy9Fd0Qxjba6BqOkR"   // gitleaks:allow synthetic test fixture; the token PATCH /account/keys/{key_id}/rotate returns
	bbExportTok     = "ET16atWzOjkIT3fE52rlm0lFYy0WivuO"   // Organization.export_token (GET /account/orgs, /account/orgs/{org_id})
	bbDownloadTok   = "DTrUWKxAtSUyzcRVq3pwPnTlhWaI8tQ3"   // Organization.download_token
	bbETToken       = "ETrJAzbyhVUpyXhLhPA0JusnvtZyorGi"   // ExportToken.token (GET /account/orgs/{org_id}/exportTokens[/{key_id}])
	bbAccessResp    = "xkPANEpwdfDFMesBrnkeO7uo6L3jn6J0"   // gitleaks:allow synthetic test fixture; AccessToken.access_token (POST /account/api/token)
	bbClientSecResp = "CSavpHGOYn9GLv4QO7foksC18E3EiiuhkE" // gitleaks:allow synthetic test fixture; api_client_secret (POST /account/api/rotate)
	// Identifiers: public, an MSP acts on each.
	bbOrgID      = "a284ec35-d455-4ea6-bb2e-251e6a208d2d"
	bbSiteID     = "5429d241-e1c5-4644-a90f-23dfdbf92327"
	bbAssetID    = "e092b4ba-896f-4ec8-9048-3a66e28e1cc6"
	bbAsset2ID   = "d77b63ee-9886-42c6-bdc0-bdbb089c4fda"
	bbServiceID  = "f75bf1a2-1cec-4a04-9b3d-b1004a3ac170"
	bbService2ID = "8d57941d-3cc0-4aac-beb6-aa42c9edad7f"
	bbSoftwareID = "b02c6d96-6935-4cfd-9a02-822944bfb0a2"
	bbVulnID     = "4ca0b0d9-0f9d-40e4-9c92-70fe3613ff50"
	bbCertID     = "4151e4d1-e4c3-438e-88f4-343a4c10780b"
	bbWirelessID = "6ed806d3-0907-43fc-b7cb-1182e2fc1c90"
	bbTaskID     = "192f12ac-42aa-472c-8644-b209a099ac18"
	bbKeyID      = "adc1d2c2-3d09-4f8b-a1df-a09729aee686" // gitleaks:allow synthetic test fixture
	bbKey2ID     = "9890b67c-d365-4373-a735-d94df9e6774b" // gitleaks:allow synthetic test fixture
	bbCredID     = "e79de830-ffbe-4471-b711-2ad98db46357"
	bbAgentID    = "dcc74931-a00c-4261-bd5e-0dee39fc46f0"
	bbETID       = "96229afb-429f-4e77-b5ab-4ad6b71c52e9"
	bbClientID   = "b8aca517-8dbf-495d-b341-1a4d8dc86b2a" // gitleaks:allow synthetic test fixture; an API client id (client_id reads as a credential by the client-pair rule)
	bbRedirect   = "redirect-other-host"                  // an asset the fake redirects to another host (localhost)
	bbRedirPort  = "redirect-other-port"                  // an asset the fake redirects to another port of the same host
	bbRedirHTTP  = "redirect-plain-http"                  // an asset the fake redirects to plain http (a scheme downgrade)
	bbRedirIDN   = "redirect-homograph"                   // an asset the fake redirects to a non-ASCII homograph of console.runzero.com
	bbDenied     = "PDENY01"                              // an ID (or a query value) the fake answers 401
	// Asset, service, vulnerability, software, certificate and key data: never withheld.
	bbHost         = "hq-record-dc01"
	bbHost2        = "hq-record-web02"
	bbAddr         = "10.20.30.40"
	bbAddr2        = "10.20.30.41"
	bbCIDR         = "10.20.30.0/24"
	bbMAC          = "00:50:56:9a:4b:2c"
	bbOS           = "Windows Server 2016"
	bbCVE          = "CVE-2024-38063"
	bbVulnName     = "HQ-Record IPv6 Remote Code Execution"
	bbSWVendor     = "OpenSSL Project"
	bbSWProduct    = "OpenSSL"
	bbSWVersion    = "1.0.2k"
	bbCertSubject  = "CN=hq-record.example.com,O=HQ Record"
	bbCertCN       = "hq-record.example.com"
	bbCertIssuer   = "CN=HQ-Record Issuing CA,O=HQ Record"
	bbCertFP256    = "8f2e4d6c0b1a3958e7f6d5c4b3a29180f7e6d5c4b3a2918f0e1d2c3b4a596877"
	bbCertSigAlg   = "SHA1-RSA"
	bbLastUsedIP   = "203.0.113.45"
	bbExportUsedBy = "198.51.100.23"
	bbUsageToday   = "4242"
	bbCreatedAt    = "1576300370"
	bbESSID        = "HQ-Record-Corp"
	bbWifiAuth     = "wpa2-psk"
	bbKeyComment   = "HQ-Record Splunk integration"
)

// bbCertEnd is the certificate's validity_end: ten days from now, so
// certs-expiring lists it.
var bbCertEnd = strconv.FormatInt(time.Now().Add(10*24*time.Hour).Unix(), 10)

// bearer is the Authorization header value runZero takes for key.
func bearer(key string) string { return "Bearer " + key }

// keyForms are the forms of one key a request carries: the key itself, then
// the header value.
func keyForms(key string) []string { return []string{key, bearer(key)} }

// secrets are every credential the suite gives the process or the fake
// answers, in the order the fake splits them (the first one a request
// carried is the one split across pages).
func secrets() []string {
	var out []string
	for _, k := range []string{bbKey, bbStored, bbFileKey, bbFileHeader, bbFileOnly} {
		out = union(out, keyForms(k)...)
	}
	return union(out, bbFileAccess, bbArgvSec, bbMCPArgSec, bbShortResp, bbGzipResp, bbBOMResp, bbHTTPToken,
		bbKeyToken, bbKey2Token, bbOrgKeyToken, bbRotatedKey, bbExportTok, bbDownloadTok, bbETToken, bbAccessResp, bbClientSecResp)
}

var bbCLI, bbMCP string

// underRosetta is true in a linux/amd64 container on Apple silicon, where
// verify_all predicts CI's Linux row: each CLI start costs 1 to 3 seconds and
// the package took 262 to 1408 s with host load, against go test's ten-minute
// limit. CI's native Linux and Windows rows and macOS run all of it;
// RUNZERO_BB_FULL=1 forces it here.
var underRosetta = func() bool {
	b, _ := os.ReadFile("/proc/cpuinfo")
	return bytes.Contains(b, []byte("VirtualApple"))
}()

// trap stands in for the network: every process the suite starts has
// HTTPS_PROXY and HTTP_PROXY pointing at it, so a request to any host that
// is not loopback (a real runZero host, a homograph a redirect named)
// reaches it instead of the network. It must never see a request.
var trap struct {
	sync.Mutex
	url  string
	seen []string
}

func trapEnv() []string {
	return []string{"HTTPS_PROXY=" + trap.url, "HTTP_PROXY=" + trap.url, "https_proxy=" + trap.url, "http_proxy=" + trap.url,
		"NO_PROXY=", "no_proxy="}
}

func trapSeen() []string {
	trap.Lock()
	defer trap.Unlock()
	return append([]string{}, trap.seen...)
}

func TestMain(m *testing.M) {
	if underRosetta && os.Getenv("RUNZERO_BB_FULL") != "1" {
		fmt.Fprintln(os.Stderr, "runzeroblackbox: skipped under Rosetta emulation; CI's native rows and macOS run it")
		os.Exit(0)
	}
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	// Not more than go test's -parallel default (GOMAXPROCS): every CLI
	// start costs the credential kit CPU, so the package is CPU-bound, and
	// running twice as many tests at once on a 4-vCPU runner starved each
	// process past its own timeout (PR #616, run 42177394104).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trap.Lock()
		trap.seen = append(trap.seen, r.Method+" "+r.Host+r.URL.RequestURI())
		trap.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	trap.url = ts.URL
	dir, err := os.MkdirTemp("", "runzero-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "runzero-cli"+exe)
	bbMCP = filepath.Join(dir, "runzero-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/runzero-cli", bbMCP: "../../cmd/runzero-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	ts.Close()
	if seen := trapSeen(); len(seen) != 0 {
		fmt.Fprintf(os.Stderr, "runzeroblackbox: FAIL: %d request(s) left for a non-loopback host (the proxy trap caught them): %v\n", len(seen), seen)
		code = 1
	}
	fmt.Fprintf(os.Stderr, "runzeroblackbox: %d CLI runs, %d MCP tools/call requests\n", cliRuns.n, mcpCallsSent.n)
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
		string(pem.EncodeToMemory(&pem.Block{Type: "RUNZERO API KEY", Bytes: []byte(s)})),
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

// bearerKey is the key of a request's one Authorization: Bearer <key>
// header, "" when it has none (or another scheme).
func bearerKey(r *http.Request) string {
	key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return ""
	}
	return key
}

// requestText is everything a request carried: its headers, its URL (raw
// and decoded) and its body (which it restores for the handler).
func requestText(r *http.Request) string {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	decoded, _ := url.QueryUnescape(r.URL.RawQuery)
	form, _ := url.QueryUnescape(string(body))
	return fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + decoded + "\n" + string(body) + "\n" + form
}

// sentSecrets is every known secret the request carried (its headers, the
// URL or the body): a real API can echo what it was sent.
func sentSecrets(r *http.Request) []string {
	seen := requestText(r)
	var out []string
	for _, s := range secrets() {
		if strings.Contains(seen, s) {
			out = append(out, s)
		}
	}
	return out
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
	url     string // the fake's origin; the CLI's base URL is url + apiBase
	caFile  string
	mu      sync.Mutex
	offHost int            // connections another host (or port) accepted: a credential must never reach one
	methods []string       // API requests, "METHOD /path" with the /api/v1.0 prefix removed ("METHOD !/path" without it)
	keys    map[string]int // keys the API received as the one header Authorization: Bearer <key>
	// otherAuth counts requests whose credential came any other way: a second
	// Authorization header, one that is not Bearer over a key (Basic, Token,
	// a doubled "Bearer Bearer"), an X-Api-Key header, or a key in the query
	// (runZero takes only the Bearer header).
	otherAuth int
	// openAuth and openNoAuth count the requests to the endpoints the spec
	// declares security: [] (/health, /releases/*) that carried the key and
	// that carried none.
	openAuth, openNoAuth int
	// pages counts each export page the fake served, by
	// "/path?start_key=<key>" ("" for the first page).
	pages map[string]int
	// writes are the bodies of the writes the fake received, by "METHOD /path".
	writes map[string][]string
	// redirectSync sends /account/agents (the account resource sync walks) to
	// another host too, so sync meets a redirect.
	redirectSync bool
	// denied paths answer 401 (an export or sync that meets an API error).
	denied map[string]bool
	// denials counts the requests the fake answered 401.
	denials int
	// offOriginNext names the next export page with an absolute URL on
	// another host (runZero documents an opaque next_key), and the scan task
	// a site starts with an absolute URL on another host as its id.
	offOriginNext bool
	// pageFill is how many filler records the asset export and the event
	// log carry besides their named records and split halves (default 21: 25
	// rows on page 1 of the event log at page_size 25).
	pageFill int
	// splitAssets splits the first two credentials a request carried across
	// the asset export's records too (TestStoreSplitAcrossRecordsKnownP2).
	splitAssets bool
	// quiet, when set, names the paths (below /api/v1.0) whose answers
	// repeat nothing the request carried (setQuiet).
	quiet func(p string) bool
}

// setQuiet makes the paths quiet names (none when nil) answer without
// repeating what they were sent: the credential kit withholds a whole answer
// that repeats a held value, so a check that the data itself prints reads it
// from such an answer.
func (sv *server) setQuiet(quiet func(p string) bool) {
	sv.mu.Lock()
	sv.quiet = quiet
	sv.mu.Unlock()
}

// everyPath is setQuiet's argument for every path.
func everyPath(string) bool { return true }

// quietServer is echoServer with setQuiet(everyPath).
func quietServer(t *testing.T) *server {
	t.Helper()
	sv := echoServer(t)
	sv.setQuiet(everyPath)
	return sv
}

// kitNotice is the credential kit's notice in place of a unit it withheld
// from sink (credkit.SinkStdout, SinkStderr, SinkMCP, SinkFile).
func kitNotice(sink string) string { return credkit.Notice(sink) }

func newServer() *server {
	return &server{keys: map[string]int{}, pages: map[string]int{}, writes: map[string][]string{}, denied: map[string]bool{}, pageFill: 21}
}

// base is the base URL the CLI is given.
func (sv *server) base() string { return sv.url + apiBase }

// tlsServer starts an HTTP/2 TLS fake.
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// tlsFake starts h as a TLS fake and writes its certificate to the CA file
// the CLI is given (RUNZERO_CA_CERT): the credential kit sends no held value
// over plain http.
func tlsFake(t *testing.T, h http.Handler) (origin, caFile string) {
	t.Helper()
	srv := tlsServer(t, h)
	caFile = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return srv.URL, caFile
}

// isWrite reports a request that changes data.
func isWrite(m string) bool {
	method, _, _ := strings.Cut(m, " ")
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
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
// Bearer <key> header counts the key; anything else counts as other auth.
// Call with sv.mu held.
func (sv *server) countAuth(r *http.Request, p string) {
	auth := r.Header.Values("Authorization")
	key := bearerKey(r)
	if len(auth) == 1 && key != "" && !strings.ContainsAny(key, " \t") {
		sv.keys[key]++
	} else if len(auth) > 0 {
		sv.otherAuth++
	}
	if len(r.Header.Values("X-Api-Key")) > 0 {
		sv.otherAuth++
	}
	q := r.URL.Query()
	for _, k := range []string{"api_key", "apikey", "x-api-key", "token", "access_token", "key", "bearer", "_token"} {
		if q.Has(k) {
			sv.otherAuth++
		}
	}
	if p == "/health" || strings.HasPrefix(p, "/releases/") {
		if len(auth) > 0 {
			sv.openAuth++
		} else {
			sv.openNoAuth++
		}
	}
}

// receivedKeys are the distinct keys this fake received, sorted.
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

// laterPages counts the requests for p that asked for a page after the
// first (a start_key).
func (sv *server) laterPages(p string) int {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	n := 0
	for k, c := range sv.pages {
		path, key, _ := strings.Cut(k, "?")
		if path == p && key != "start_key=" {
			n += c
		}
	}
	return n
}

var (
	reAsset     = regexp.MustCompile(`^/org/assets/([^/]+)$`)
	reService   = regexp.MustCompile(`^/org/services/([^/]+)$`)
	reSite      = regexp.MustCompile(`^/org/sites/([^/]+)$`)
	reSiteScan  = regexp.MustCompile(`^/org/sites/([^/]+)/scan$`)
	reTask      = regexp.MustCompile(`^/org/tasks/([^/]+)$`)
	reTaskData  = regexp.MustCompile(`^/org/tasks/([^/]+)/(log|data|changes)$`)
	reKey       = regexp.MustCompile(`^/account/keys/([^/]+)$`)
	reKeyRotate = regexp.MustCompile(`^/account/keys/([^/]+)/rotate$`)
	reOrg       = regexp.MustCompile(`^/account/orgs/([^/]+)$`)
	reETs       = regexp.MustCompile(`^/account/orgs/([^/]+)/exportTokens$`)
	reET        = regexp.MustCompile(`^/account/orgs/([^/]+)/exportTokens/([^/]+)$`)
	reCred      = regexp.MustCompile(`^/account/credentials/([^/]+)$`)
)

// last is the last path segment of p.
func last(p string) string { return p[strings.LastIndex(p, "/")+1:] }

// assetDoc is an Asset as GET /org/assets and the asset export return it:
// its credentials member maps a credential UUID to the probes it ran.
func assetDoc(id, name, addr, extra string) string {
	return fmt.Sprintf(`{"id":%q,"organization_id":%q,"site_id":%q,"site_name":"HQ-Record Main","names":[%q],"addresses":[%q],`+
		`"macs":[%q],"os":%q,"os_vendor":"Microsoft","type":"Server","hw":"VMware Virtual Platform","alive":true,"criticality":"high",`+
		`"criticality_rank":3,"risk":"critical","risk_rank":4,"eol_os":1736899200,"tags":{"owner":"hq-record"},"service_count":2,`+
		`"vulnerability_count":1,"first_seen":%s,"last_seen":%s,"created_at":%s,"updated_at":%s,`+
		`"credentials":{%q:{"snmpv2_community":true}}%s}`,
		id, bbOrgID, bbSiteID, name, addr, bbMAC, bbOS, bbCreatedAt, bbCreatedAt, bbCreatedAt, bbCreatedAt, bbCredID, extra)
}

// serviceDoc is a Service row (GET /org/services, the services export): it
// embeds its asset, so `id` is the asset's id and service_id the row's.
func serviceDoc(serviceID string, port int, protocol, product, extra string) string {
	return fmt.Sprintf(`{"id":%q,"names":[%q],"addresses":[%q],"criticality":"high","service_id":%q,"service_asset_id":%q,`+
		`"service_address":%q,"service_transport":"tcp","service_port":%d,"service_protocol":%q,"service_product":%q,`+
		`"service_created_at":%s%s}`, bbAssetID, bbHost, bbAddr, serviceID, bbAssetID, bbAddr, port, protocol, product, bbCreatedAt, extra)
}

// vulnDoc is a Vulnerability row (the vulnerability export): it embeds its
// asset.
func vulnDoc(extra string) string {
	return fmt.Sprintf(`{"id":%q,"names":[%q],"addresses":[%q],"criticality":"high","vulnerability_id":%q,"vulnerability_asset_id":%q,`+
		`"vulnerability_name":%q,"vulnerability_cve":%q,"vulnerability_severity":"critical","vulnerability_risk":"critical",`+
		`"vulnerability_cvss3_base_score":9.8,"vulnerability_first_detected_at":%s%s}`,
		bbAssetID, bbHost, bbAddr, bbVulnID, bbAssetID, bbVulnName, bbCVE, bbCreatedAt, extra)
}

// softwareDoc is a Software row (the software export).
func softwareDoc(extra string) string {
	return fmt.Sprintf(`{"id":%q,"names":[%q],"addresses":[%q],"software_id":%q,"software_asset_id":%q,"software_vendor":%q,`+
		`"software_product":%q,"software_version":%q%s}`, bbAssetID, bbHost, bbAddr, bbSoftwareID, bbAssetID, bbSWVendor, bbSWProduct, bbSWVersion, extra)
}

// certDoc is a Certificate (the certificate export): its signature member
// reads as a credential, signature_algorithm does not.
func certDoc(extra string) string {
	return fmt.Sprintf(`{"id":%q,"organization_id":%q,"asset_id":%q,"subject":%q,"cn":%q,"issuer":%q,"validity_start":%s,`+
		`"validity_end":%s,"self_signed":false,"signature":"3a5f9c1e7b2d4a6f8e0c","signature_algorithm":%q,`+
		`"fp_sha1":"0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c","fp_sha256":%q,"san_dns_names":[%q,"www.hq-record.example.com"]%s}`,
		bbCertID, bbOrgID, bbAssetID, bbCertSubject, bbCertCN, bbCertIssuer, bbCreatedAt, bbCertEnd, bbCertSigAlg, bbCertFP256, bbCertCN, extra)
}

// wirelessDoc is a Wireless network (GET /org/wireless, the wireless export).
func wirelessDoc(extra string) string {
	return fmt.Sprintf(`{"id":%q,"organization_id":%q,"site_id":%q,"bssid":"00:1a:2b:3c:4d:5e","essid":%q,"authentication":%q,`+
		`"encryption":"aes","channels":"11","family":"Cisco","last_seen":%s%s}`, bbWirelessID, bbOrgID, bbSiteID, bbESSID, bbWifiAuth, bbCreatedAt, extra)
}

// siteDoc is a Site.
func siteDoc(extra string) string {
	return fmt.Sprintf(`{"id":%q,"organization_id":%q,"name":"HQ-Record Main","description":"HQ-Record headquarters","scope":%q,`+
		`"subnets":{%q:{"tags":{}}},"asset_count":2,"created_at":%s%s}`, bbSiteID, bbOrgID, bbCIDR, bbCIDR, bbCreatedAt, extra)
}

// taskDoc is a Task (GET /org/tasks/{task_id}; the scan PUT answers one).
func taskDoc(id, status, extra string) string {
	return fmt.Sprintf(`{"id":%q,"organization_id":%q,"site_id":%q,"agent_id":%q,"name":"HQ-Record scan","type":"scan","status":%q,`+
		`"params":{"targets":%q},"stats":{"hosts":2},"created_at":%s%s}`, id, bbOrgID, bbSiteID, bbAgentID, status, bbCIDR, bbCreatedAt, extra)
}

// keyDoc is an OrganizationAPIKey as /account/keys and /org/key return it.
func keyDoc(id, comment, token, extra string) string {
	return fmt.Sprintf(`{"id":%q,"client_id":%q,"organization_id":%q,"created_at":%s,"created_by":"pat@hq-record.example.com",`+
		`"comment":%q,"last_used_at":%s,"last_used_ip":%q,"last_used_ua":"curl/7.44.1","counter":7,"usage_today":%s,`+
		`"usage_limit":10000,"token":%q,"inactive":false,"type":"org"%s}`,
		id, bbClientID, bbOrgID, bbCreatedAt, comment, bbCreatedAt, bbLastUsedIP, bbUsageToday, token, extra)
}

// orgDoc is an Organization: its export_token and download_token are
// credentials, export_token_last_used_by an IP.
func orgDoc(extra string) string {
	return fmt.Sprintf(`{"id":%q,"created_at":%s,"updated_at":%s,"client_id":%q,"download_token":%q,"download_token_created_at":%s,`+
		`"name":"HQ-Record","description":"HQ-Record subdivisions","inactive":false,"export_token":%q,"export_token_created_at":%s,`+
		`"export_token_last_used_at":%s,"export_token_last_used_by":%q,"export_token_counter":3%s}`,
		bbOrgID, bbCreatedAt, bbCreatedAt, bbClientID, bbDownloadTok, bbCreatedAt, bbExportTok, bbCreatedAt, bbCreatedAt, bbExportUsedBy, extra)
}

// exportTokenDoc is an ExportToken.
func exportTokenDoc(extra string) string {
	return fmt.Sprintf(`{"id":%q,"token":%q,"comment":"HQ-Record export for Splunk","created_at":%s,"last_used_at":%s,`+
		`"last_used_by":%q,"counter":3%s}`, bbETID, bbETToken, bbCreatedAt, bbCreatedAt, bbExportUsedBy, extra)
}

// credentialDoc is a Credential as GET /account/credentials lists it (the
// secret itself is never returned).
func credentialDoc(extra string) string {
	return fmt.Sprintf(`{"id":%q,"client_id":%q,"name":"HQ-Record SNMP","type":"snmpv2_community","created_at":%s,`+
		`"created_by_email":"pat@hq-record.example.com","global":false,"cidrs":[%q],"insecure":"true","last_used_at":%s%s}`,
		bbCredID, bbClientID, bbCreatedAt, bbCIDR, bbCreatedAt, extra)
}

// userDoc is a User as GET /account/users lists it, with a generic password
// member every record carries.
func userDoc(id, email, first, extra string) string {
	return fmt.Sprintf(`{"id":%q,"client_id":%q,"email":%q,"first_name":%q,"last_name":"HQ-Record","sso_only":false,`+
		`"login_failures":0,"org_default_role":"admin","created_at":%s,"password":%q%s}`, id, bbClientID, email, first, bbCreatedAt, bbShortResp, extra)
}

// exportPage answers a paged export of items: without page_size one bare
// array (runZero's documented default); with page_size a page
// {"<field>":[...],"next_key":"..."} the next start_key continues
// (spec.yaml AssetPage, EventPage). next names the page after start as a
// start_key; offOriginNext names it with an absolute URL on another host
// instead. A start_key that is not page-N (an absolute URL a naive pager sent
// back) reads as page 2.
func exportPage(q url.Values, field, path string, items []string, otherPort string, offOriginNext bool) string {
	size, err := strconv.Atoi(q.Get("page_size"))
	if err != nil || size <= 0 {
		return "[" + strings.Join(items, ",") + "]"
	}
	start := 0
	if k := q.Get("start_key"); k != "" {
		start = size
		if n, err := strconv.Atoi(strings.TrimPrefix(k, "page-")); err == nil && n > 1 {
			start = (n - 1) * size
		}
	}
	start = min(start, len(items))
	end := min(start+size, len(items))
	next := ""
	if end < len(items) {
		next = fmt.Sprintf("page-%d", end/size+1)
		if offOriginNext {
			next = fmt.Sprintf("https://localhost:%s%s%s?page_size=%d&start_key=%s", otherPort, apiBase, path, size, next)
		}
	}
	return fmt.Sprintf(`{%q:[%s],"next_key":%q}`, field, strings.Join(items[start:end], ","), next)
}

// fillers are n filler records of the shape doc builds for index i.
func fillers(n int, doc func(i int) string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = doc(i)
	}
	return out
}

// exportAssets is the asset export (GET /export/org/assets.json): the named
// assets (the first carries the echo) and the fillers. With splitAssets the
// first two credentials the request carried are split across the export's
// records too (the ruled P2 credential-split-across-sql-bindings: a sync
// stores them as rows of their own).
func (sv *server) exportAssets(q url.Values, first, second, echo, otherPort string, offOriginNext bool) string {
	sv.mu.Lock()
	fill, split := sv.pageFill, sv.splitAssets
	sv.mu.Unlock()
	items := []string{assetDoc(bbAssetID, bbHost, bbAddr, `,"echo":`+echo), assetDoc(bbAsset2ID, bbHost2, bbAddr2, "")}
	items = append(items, fillers(fill, func(i int) string {
		return assetDoc(fmt.Sprintf("asset-fill-%04d", i), fmt.Sprintf("hq-record-fill-%d", i), fmt.Sprintf("10.20.32.%d", i%250), "")
	})...)
	if split {
		a1, b1 := halves(first)
		a2, b2 := halves(second)
		items = append(append([]string{assetDoc("asset-split-a1", a1, "10.20.31.1", ""), assetDoc("asset-split-a2", a2, "10.20.31.2", "")}, items...),
			assetDoc("asset-split-b1", b1, "10.20.31.3", ""), assetDoc("asset-split-b2", b2, "10.20.31.4", ""))
	}
	return exportPage(q, "assets", "/export/org/assets.json", items, otherPort, offOriginNext)
}

// eventDoc is an Event of the system event log (GET /account/events.json).
func eventDoc(id, target, extra string) string {
	return fmt.Sprintf(`{"id":%q,"created_at":%s,"organization_id":%q,"site_id":%q,"action":"api-key-used","source_type":"api",`+
		`"source_name":"HQ-Record automation","target_type":"asset","target_name":%q,"state":"success","success":true%s}`,
		id, bbCreatedAt, bbOrgID, bbSiteID, target, extra)
}

// exportEvents is the system event log (GET /account/events.json, paged like
// the asset export): the first two credentials the request carried, split
// across two pages (one half each as an event's target_name: the halves a1
// and a2 head page 1 at page_size 25, b1 and b2 are page 2), an event with
// the echo, and the fillers between.
func (sv *server) exportEvents(q url.Values, first, second, echo string) string {
	sv.mu.Lock()
	fill := sv.pageFill
	sv.mu.Unlock()
	a1, b1 := halves(first)
	a2, b2 := halves(second)
	items := []string{eventDoc("event-split-a1", a1, ""), eventDoc("event-split-a2", a2, ""),
		eventDoc("event-hq-0001", bbHost, `,"echo":`+echo), eventDoc("event-hq-0002", bbHost2, "")}
	items = append(items, fillers(fill, func(i int) string {
		return eventDoc(fmt.Sprintf("event-fill-%04d", i), fmt.Sprintf("hq-record-fill-%d", i), "")
	})...)
	items = append(items, eventDoc("event-split-b1", b1, ""), eventDoc("event-split-b2", b2, ""))
	return exportPage(q, "events", "/account/events.json", items, "", false)
}

// echoServer starts the runZero API fake (TLS) and another host it
// redirects to (TLS, its connections counted).
func echoServer(t *testing.T) *server {
	t.Helper()
	sv := newServer()
	// Another host (and port) the API redirects to: a credential must never
	// reach it. Every connection it accepts counts, so a hop refused only by
	// the TLS name check still counts.
	other := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	other.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			sv.mu.Lock()
			sv.offHost++
			sv.mu.Unlock()
		}
	}
	other.EnableHTTP2 = true
	other.StartTLS()
	t.Cleanup(other.Close)
	_, otherPort, _ := net.SplitHostPort(strings.TrimPrefix(other.URL, "https://"))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// What the request carried.
		sent := sentSecrets(r)
		sv.mu.Lock()
		quiet := sv.quiet
		sv.mu.Unlock()
		if quiet != nil && quiet(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, apiBase), "/")) {
			sent = nil
		}
		echo := echoObject(sent)
		split := splitObject(sent)
		q := r.URL.Query()
		p, onBase := strings.CutPrefix(r.URL.Path, apiBase)
		p = strings.TrimSuffix(p, "/")
		body, _ := io.ReadAll(r.Body)
		sv.mu.Lock()
		if onBase {
			sv.methods = append(sv.methods, r.Method+" "+p)
		} else {
			sv.methods = append(sv.methods, r.Method+" !"+r.URL.Path)
		}
		sv.countAuth(r, p)
		if r.Method == http.MethodGet && (strings.HasPrefix(p, "/export/") || p == "/account/events.json") {
			sv.pages[p+"?start_key="+q.Get("start_key")]++
		}
		if isWrite(r.Method + " ") {
			sv.writes[r.Method+" "+p] = append(sv.writes[r.Method+" "+p], string(body))
		}
		redirectSync := sv.redirectSync
		denied := sv.denied[p]
		offOriginNext := sv.offOriginNext
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		deniedQuery := false
		for _, vs := range q {
			for _, v := range vs {
				deniedQuery = deniedQuery || strings.Contains(v, bbDenied)
			}
		}
		first, second := "none", "none"
		if len(sent) > 0 {
			first = sent[0]
		}
		if len(sent) > 1 {
			second = sent[1]
		}
		get := r.Method == http.MethodGet
		// Every answer repeats the first credential the request carried in a
		// response header, and every redirect in its Location query.
		w.Header().Set("X-Request-Echo", first)
		echoQuery := "echo=" + url.QueryEscape(first)
		switch {
		case !onBase:
			// The documented base path is /api/v1.0; a client that drops it
			// is answered as runZero would, not found.
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
		case denied || deniedQuery || strings.Contains(p, "/"+bbDenied):
			// Rejected (a denied asset or list, a sync or export the test
			// denies): the error repeats what it was sent.
			sv.mu.Lock()
			sv.denials++
			sv.mu.Unlock()
			w.Header().Set("WWW-Authenticate", `Bearer realm="runZero", error="invalid_token", error_description="`+strings.Join(sent, " ")+`"`)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid API key: %s","detail":"bad credentials %s","echo":%s}`,
				strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/org/assets/"+bbRedirect || (redirectSync && p == "/account/agents"):
			http.Redirect(w, r, "https://localhost:"+otherPort+r.URL.Path+"?"+echoQuery, http.StatusTemporaryRedirect)
		case p == "/org/assets/"+bbRedirPort:
			http.Redirect(w, r, other.URL+r.URL.Path+"?"+echoQuery, http.StatusTemporaryRedirect)
		case p == "/org/assets/"+bbRedirHTTP:
			http.Redirect(w, r, "http://"+r.Host+r.URL.Path+"?"+echoQuery, http.StatusTemporaryRedirect)
		case p == "/org/assets/"+bbRedirIDN:
			// Set as written (http.Redirect would percent-escape the host): a
			// Cyrillic o in runzero.
			w.Header().Set("Location", "https://console.runzer\u043e.com"+apiBase+"/org/assets/"+bbAssetID+"?"+echoQuery)
			w.WriteHeader(http.StatusTemporaryRedirect)
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNoContent)
		case p == "/health":
			fmt.Fprint(w, `{"status":"ok"}`)
		case strings.HasPrefix(p, "/releases/"):
			fmt.Fprintf(w, `{"id":%q,"version":"4.0.250101"}`, strings.TrimSuffix(strings.TrimPrefix(p, "/releases/"), "/version"))
		case r.Method == http.MethodPut && reSiteScan.MatchString(p):
			// The scan task the site starts (scan-watch then polls it).
			id := bbTaskID
			if offOriginNext {
				id = "https://localhost:" + otherPort + apiBase + "/org/tasks/" + bbTaskID
			}
			fmt.Fprint(w, taskDoc(id, "new", ""))
		case reTaskData.MatchString(p):
			// Documented: 303 to the task's data (spec.yaml
			// /org/tasks/{task_id}/log, /data, /changes: "Redirection to scan
			// data"), a temporary URL on another host.
			http.Redirect(w, r, "https://localhost:"+otherPort+"/scan-data/"+bbTaskID+"?sig=temporary&"+echoQuery, http.StatusSeeOther)
		case r.Method == http.MethodPost && p == "/account/api/token":
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":3600,"echo":%s}`, bbAccessResp, echo)
		case r.Method == http.MethodPost && p == "/account/api/rotate":
			fmt.Fprintf(w, `{"api_client_id":%q,"api_client_secret":%q}`, bbClientID, bbClientSecResp)
		case r.Method == http.MethodPatch && reKeyRotate.MatchString(p):
			fmt.Fprint(w, keyDoc(reKeyRotate.FindStringSubmatch(p)[1], bbKeyComment, bbRotatedKey, ""))
		case r.Method == http.MethodDelete && reKey.MatchString(p):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && (strings.HasSuffix(p, "/hide") || strings.HasSuffix(p, "/stop")):
			fmt.Fprint(w, `{}`)
		case !get:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
		case p == "/account/keys":
			// A key with its token and the echo, and a key whose comment
			// repeats the credential it was sent.
			fmt.Fprintf(w, "[%s,%s]", keyDoc(bbKeyID, bbKeyComment, bbKeyToken, `,"echo":`+echo),
				keyDoc(bbKey2ID, first, bbKey2Token, ""))
		case reKey.MatchString(p):
			fmt.Fprint(w, keyDoc(last(p), bbKeyComment, bbKeyToken, `,"split":`+split))
		case p == "/org/key":
			fmt.Fprint(w, keyDoc(bbKeyID, bbKeyComment, bbOrgKeyToken, ""))
		case p == "/account/orgs":
			// gzip-encoded: an organization's tokens and the echo.
			gzipped(w, "["+orgDoc(`,"echo":`+echo)+"]")
		case reOrg.MatchString(p):
			fmt.Fprint(w, orgDoc(`,"split":`+split))
		case reETs.MatchString(p):
			fmt.Fprint(w, "["+exportTokenDoc(`,"echo":`+echo)+"]")
		case reET.MatchString(p):
			fmt.Fprint(w, exportTokenDoc(""))
		case p == "/account/users":
			// gzip-encoded, each user with its password; the first carries the
			// echo.
			gzipped(w, "["+userDoc("user-pat-0001", "pat@hq-record.example.com", "Pat", `,"echo":`+echo)+","+
				userDoc("user-sam-0002", "sam@hq-record.example.com", "Sam", "")+"]")
		case p == "/account/credentials":
			fmt.Fprint(w, "["+credentialDoc(`,"echo":`+echo)+"]")
		case reCred.MatchString(p):
			fmt.Fprint(w, credentialDoc(""))
		case p == "/org/assets":
			// An asset with the echo, and an asset whose name repeats the
			// credential it was sent.
			fmt.Fprint(w, "["+assetDoc(bbAssetID, bbHost, bbAddr, `,"echo":`+echo)+","+assetDoc(bbAsset2ID, first, bbAddr2, "")+"]")
		case reAsset.MatchString(p):
			fmt.Fprint(w, assetDoc(last(p), bbHost, bbAddr, `,"split":`+split))
		case p == "/org/services":
			fmt.Fprint(w, "["+serviceDoc(bbServiceID, 445, "smb", "Microsoft SMB", `,"echo":`+echo)+","+
				serviceDoc(bbService2ID, 3389, "rdp", "Microsoft Terminal Services", "")+"]")
		case reService.MatchString(p):
			fmt.Fprint(w, serviceDoc(last(p), 445, "smb", "Microsoft SMB", ""))
		case p == "/org/sites" || p == "/account/sites" || p == "/export/org/sites.json":
			// BOM-prefixed: a site carrying a token and the echo.
			fmt.Fprint(w, "\xef\xbb\xbf["+siteDoc(fmt.Sprintf(`,"token":%q,"echo":%s`, bbBOMResp, echo))+"]")
		case reSite.MatchString(p):
			fmt.Fprint(w, siteDoc(""))
		case p == "/org/tasks" || p == "/account/tasks" || p == "/export/org/tasks.json":
			fmt.Fprint(w, "["+taskDoc(bbTaskID, "processed", `,"echo":`+echo)+"]")
		case reTask.MatchString(p):
			// Every task has finished, so scan-watch sees it complete.
			fmt.Fprint(w, taskDoc(last(p), "processed", ""))
		case p == "/org/wireless" || p == "/export/org/wireless.json":
			fmt.Fprint(w, "["+wirelessDoc("")+"]")
		case p == "/org/agents" || p == "/org/explorers" || p == "/account/agents":
			fmt.Fprintf(w, `[{"id":%q,"organization_id":%q,"site_id":%q,"name":"HQ-RECORD-AGENT","host_id":"6f9e6fe52271da70962e007183c5c9c9",`+
				`"internal_ip":%q,"os":%q,"version":"4.0.250101","connected":true,"created_at":%s}]`, bbAgentID, bbOrgID, bbSiteID, bbAddr2, bbOS, bbCreatedAt)
		case p == "/org/metrics":
			fmt.Fprintf(w, `{"assets":2,"services":2,"vulnerabilities":1,"updated_at":%s}`, bbCreatedAt)
		case p == "/org":
			fmt.Fprint(w, orgDoc(""))
		case p == "/export/org/assets.json":
			fmt.Fprint(w, sv.exportAssets(q, first, second, echo, otherPort, offOriginNext))
		case p == "/account/events.json":
			fmt.Fprint(w, sv.exportEvents(q, first, second, echo))
		case p == "/export/org/services.json":
			// gzip-encoded: a service row carrying an api_key and the echo.
			gzipped(w, "["+serviceDoc(bbServiceID, 445, "smb", "Microsoft SMB", fmt.Sprintf(`,"api_key":%q,"echo":%s`, bbGzipResp, echo))+","+
				serviceDoc(bbService2ID, 3389, "rdp", "Microsoft Terminal Services", "")+"]")
		case p == "/export/org/vulnerabilities.json":
			fmt.Fprint(w, "["+vulnDoc(`,"echo":`+echo)+"]")
		case p == "/export/org/software.json":
			fmt.Fprint(w, "["+softwareDoc("")+"]")
		case p == "/export/org/certificates.json":
			fmt.Fprint(w, "["+certDoc(`,"echo":`+echo)+"]")
		case strings.HasSuffix(p, ".json") || strings.HasPrefix(p, "/account/") || strings.HasPrefix(p, "/org/"):
			// Every other documented list (users and groups exports, findings,
			// templates, ownership types, hosted zones, ...): an empty array.
			fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
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

// sawKey reports how many requests carried Authorization: Bearer <key>,
// and the distinct keys the fake received.
func (sv *server) sawKey(key string) (int, []string) {
	all := sv.receivedKeys()
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.keys[key], all
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

// credMode is where the process gets its key.
type credMode int

const (
	credStored        credMode = iota // none in the environment: the battery runs on a key stored with auth set-token (stdin)
	credEnv                           // RUNZERO_API_KEY
	credEnvOverStored                 // a stored key and RUNZERO_API_KEY: the environment wins, the stored key is held unused
	credEnvOverFile                   // a config file's api_key and RUNZERO_API_KEY: the environment wins, the file's key is held unused
	credConfigFile                    // api_key in a 0600 config file named by RUNZERO_CONFIG
	credConfigHeader                  // auth_header = "Bearer <key>" in a 0600 config file: sent as written
	credConfigBoth                    // api_key and access_token in a 0600 config file: api_key is sent, access_token is held unused
	credConfigAccess                  // access_token alone in a 0600 config file: sent as Bearer
)

func (m credMode) String() string {
	return [...]string{"stored", "env key", "env over stored", "env over config api_key", "config api_key", "config auth_header",
		"config api_key and access_token", "config access_token"}[m]
}

// sent is the key the mode sends; held are the credentials it holds but
// never sends anywhere.
func (m credMode) sent() string {
	return [...]string{bbStored, bbKey, bbKey, bbKey, bbFileKey, bbFileHeader, bbFileKey, bbFileOnly}[m]
}

func (m credMode) held() []string {
	switch m {
	case credEnvOverStored:
		return []string{bbStored, bbHTTPToken}
	case credEnvOverFile:
		return []string{bbFileKey, bbHTTPToken}
	case credConfigBoth:
		return []string{bbFileAccess, bbHTTPToken}
	}
	return []string{bbHTTPToken}
}

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
		"RUNZERO_BASE_URL=" + sv.base(), "RUNZERO_CA_CERT=" + sv.caFile,
		"RUNZERO_CLI_PATH=" + bbCLI, "RUNZERO_MCP_HTTP_TOKEN=" + bbHTTPToken,
		// The credential kit allocates heavily at start; a wider GC target
		// takes about a tenth off each child's CPU and changes no behavior.
		"GOGC=200",
	}, trapEnv()...)
	e := &bbEnv{home: home, env: env}
	envKey := "RUNZERO_API_KEY=" + bbKey
	switch mode {
	case credStored:
		e.storeKey(t, bbStored)
	case credEnv:
		e.env = append(e.env, envKey)
	case credEnvOverStored:
		e.storeKey(t, bbStored)
		e.env = append(e.env, envKey)
	case credEnvOverFile:
		e.writeConfig(t, fmt.Sprintf("api_key = %q\n", bbFileKey))
		e.env = append(e.env, envKey)
	case credConfigFile:
		e.writeConfig(t, fmt.Sprintf("api_key = %q\n", bbFileKey))
	case credConfigHeader:
		e.writeConfig(t, fmt.Sprintf("auth_header = %q\n", bearer(bbFileHeader)))
	case credConfigBoth:
		e.writeConfig(t, fmt.Sprintf("api_key = %q\naccess_token = %q\n", bbFileKey, bbFileAccess))
	case credConfigAccess:
		e.writeConfig(t, fmt.Sprintf("access_token = %q\n", bbFileOnly))
	}
	return e
}

// writeConfig writes body as a 0600 config file and names it with
// RUNZERO_CONFIG.
func (e *bbEnv) writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := filepath.Join(e.home, "cfg")
	_ = os.MkdirAll(dir, 0o700)
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "RUNZERO_CONFIG"), "RUNZERO_CONFIG="+cfg)
}

// storeKey saves key with auth set-token (read from stdin) and checks it
// landed in the credential store.
func (e *bbEnv) storeKey(t *testing.T, key string) {
	t.Helper()
	out := e.runIn(t, key+"\n", "auth", "set-token")
	assertNoEcho(t, "auth set-token", out)
	var stored bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "credentials") {
			b, _ := os.ReadFile(path)
			stored = stored || bytes.Contains(b, []byte(key))
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
	out, err := e.runErr(stdin, args...)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out
}

// runErr is runIn for a goroutine other than the test's own: it returns
// what would fail the test instead of failing it.
func (e *bbEnv) runErr(stdin string, args ...string) (string, error) {
	count(&cliRuns, 1)
	cmd := exec.Command(bbCLI, args...)
	cmd.Env = e.env
	cmd.Dir = e.home
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		return "", errors.New("timed out")
	}
	return out.String() + "\n" + errb.String(), nil
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
	// A server that exits (the credential kit refuses to start on a
	// credential that occurs in the program's own text) answers nothing
	// more, so the wait below stops there instead of at the deadline.
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	exited := func() bool {
		select {
		case <-waitDone:
			return true
		default:
			return false
		}
	}
	enc := json.NewEncoder(stdin)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	id := 0
	deadline := time.Now().Add(420 * time.Second) // a battery session runs syncs and novels in mirrored children
	for _, calls := range phases {
		count(&mcpCallsSent, len(calls))
		for _, c := range calls {
			id++
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		}
		for time.Now().Before(deadline) && !exited() && len(mcpReplies(out.String())) < id+1 {
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
		// A half shorter than 8 characters is advisory (contract section 6
		// item 6: the kit holds a piece of 8 or more), so the check runs when
		// either half is that long.
		if a, b := halves(s); halvesToo && max(len(a), len(b)) >= 8 && strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
		if n := foldedAlnum(s); len(n) >= foldedMinAlnum && strings.Contains(folded, n) {
			t.Errorf("%s holds %q case-folded or re-separated", where, s)
		}
	}
	for _, view := range append([]string{text}, decodedRuns(text)...) {
		for _, s := range secrets() {
			for _, core := range credentialCores(s) {
				if strings.Contains(view, core) {
					t.Errorf("%s holds all of %q but its edges (%q), raw or in a base64 or hex run cut mid-value", where, s, core)
				}
			}
		}
	}
}

// credentialCores are s with up to two characters cut from each end, the
// whole value excluded (echoForms has it), at least coreMinLen long: what a
// base64 or hex run keeps of a value it carries when the run is cut inside
// the value (a decoder at the right alignment loses at most two bytes at a
// cut).
func credentialCores(s string) []string {
	var out []string
	for i := 0; i <= 2; i++ {
		for j := 0; j <= 2; j++ {
			if i+j == 0 || len(s)-i-j < coreMinLen {
				continue
			}
			out = append(out, s[i:len(s)-j])
		}
	}
	return out
}

// coreMinLen is the shortest credential core the check matches (shorter
// ones would match by chance).
const coreMinLen = 12

var (
	base64Run = regexp.MustCompile(`[A-Za-z0-9+/_-]{16,}={0,2}`)
	hexRun    = regexp.MustCompile(`[0-9a-fA-F]{24,}`)
)

// decodedRuns decodes every base64 (both alphabets) and hex run of text at
// every alignment, so a run cut inside a credential is read the way a
// decoder at the right offset reads it.
func decodedRuns(text string) []string {
	var out []string
	for _, run := range base64Run.FindAllString(text, -1) {
		run = strings.TrimRight(run, "=")
		for a := 0; a < 4 && a < len(run); a++ {
			sub := run[a:]
			sub = sub[:len(sub)-len(sub)%4]
			for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
				if b, err := enc.DecodeString(sub); err == nil {
					out = append(out, string(b))
				}
			}
		}
	}
	for _, run := range hexRun.FindAllString(text, -1) {
		for a := 0; a < 2; a++ {
			sub := run[a:]
			sub = sub[:len(sub)-len(sub)%2]
			if b, err := hex.DecodeString(sub); err == nil {
				out = append(out, string(b))
			}
		}
	}
	return out
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

// novels is every novel command, in the order they run: each reads the
// local store an inventory sync filled (two syncs, so diff and
// exposure-delta have a baseline).
func novels() [][]string {
	return [][]string{
		{"inventory", "sync", "--json"},
		{"triage", "--json"},
		{"triage"},
		{"triage", "--internet-facing", "--json"},
		{"affected", bbCVE, "--json"},
		{"affected", bbCVE},
		{"software", "rollup", "--json"},
		{"software", "rollup", "openssl"},
		{"certs-expiring", "--days", "3650", "--json"},
		{"certs-expiring", "--weak", "--json"},
		{"certs-expiring"},
		{"stale", "--days", "1", "--json"},
		{"exposure-map", bbCIDR, "--json"},
		{"exposure-map", bbCIDR},
		{"inventory", "sync", "--json"},
		{"diff", "--json"},
		{"exposure-delta", "--json"},
		{"inventory", "list", "assets", "--json"},
		{"inventory", "list", "vulnerabilities", "--match", "cve", "--json"},
		{"inventory", "list", "certificates"},
		{"inventory", "status", "--json"},
	}
}

// battery runs every output path; key is the key the process sends (the
// stored key, RUNZERO_API_KEY, or a config file's api_key, access_token or
// auth_header key).
func battery(t *testing.T, e *bbEnv, deliverFile string, key string) []cliCall {
	b := calls(
		// Every output mode.
		[]string{"org", "get-assets", "--json"},
		[]string{"org", "get-assets"},
		[]string{"org", "get-assets", "--csv"},
		[]string{"org", "get-assets", "--plain"},
		[]string{"org", "get-assets", "--agent"},
		[]string{"org", "get-assets", "--json", "--receipt"},
		[]string{"org", "get-assets", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		[]string{"org", "get-assets", "--json", "--deliver", "file:" + deliverFile},
		// Every response shape: the documented credentials (an API key's
		// token, an organization's export and download tokens, an export
		// token, a minted access token, a rotated client secret and key), the
		// echo in a record, an api_key in a gzip-encoded export, a token in a
		// BOM-prefixed list, a user's short password (gzip), a record whose
		// name or comment repeats the credential, the echo split across
		// values, lines and spaces (an asset, a key, an organization), a 401
		// echo, a refused redirect, the first two credentials split across
		// two pages of the event log (account export-events-json).
		[]string{"account", "get-keys", "--json"},
		[]string{"account", "get-keys"},
		[]string{"account", "get-keys", "--csv"},
		[]string{"account", "get-key", bbKeyID, "--json"},
		[]string{"account", "get-key", bbKeyID},
		[]string{"org", "get-key", "--json"},
		[]string{"account", "get-organizations", "--json"},
		[]string{"account", "get-organizations", "--plain"},
		[]string{"account", "get-organization", bbOrgID, "--json"},
		[]string{"account", "get-organization-export-tokens", bbOrgID, "--json"},
		[]string{"account", "get-organization-export-tokens", bbOrgID},
		[]string{"account", "get-organization-export-token", bbOrgID, bbETID, "--json"},
		[]string{"account", "get-users", "--json"},
		[]string{"account", "get-users", "--csv"},
		[]string{"account", "get-credentials", "--json"},
		[]string{"org", "get-asset", bbAssetID, "--json"},
		[]string{"org", "get-asset", bbAssetID},
		[]string{"org", "get-asset", bbDenied, "--json"},
		[]string{"org", "get-asset", bbRedirect, "--json"},
		[]string{"org", "get-assets", "--search", bbDenied, "--json"},
		[]string{"org", "get-services", "--json"},
		[]string{"org", "get-sites", "--json"},
		[]string{"org", "get-sites"},
		[]string{"org", "get-tasks", "--json"},
		[]string{"org", "get-task", bbTaskID, "--json"},
		[]string{"org", "get-wireless-lans", "--json"},
		[]string{"runzero-export", "assets-json", "--json"},
		[]string{"runzero-export", "assets-json", "--page-size", "25", "--json"},
		[]string{"runzero-export", "assets-json", "--page-size", "25", "--all", "--json"},
		[]string{"account", "export-events-json", "--page-size", "25", "--json"},
		[]string{"account", "export-events-json", "--page-size", "25", "--all", "--json"},
		[]string{"account", "export-events-json", "--page-size", "25", "--start-key", "page-2", "--json"},
		[]string{"account", "export-events-json", "--json"},
		[]string{"runzero-export", "services-json", "--json"},
		[]string{"runzero-export", "services-json", "--csv"},
		[]string{"runzero-export", "vulnerabilities-json", "--json"},
		[]string{"runzero-export", "certificates-json", "--json"},
		[]string{"health", "--json"},
		[]string{"releases", "get-latest-agent-version", "--json"},
		// The documented token mints: a client secret on the command line,
		// an access token and a rotated secret in the answer.
		[]string{"account", "get-apitoken", "--grant-type", "client_credentials", "--client-id", bbClientID, "--client-secret", bbArgvSec, "--json"},
		[]string{"account", "rotate-apitoken", "--grant-type", "client_credentials", "--client-id", bbClientID, "--client-secret", bbArgvSec, "--yes", "--json"},
		// Writes, with and without --yes (a CLI write without --yes is the
		// work order's P2, but it must never leak either): a scan, a key
		// rotation, a key removal; a scan-watch that follows its task to
		// completion.
		[]string{"org", "create-scan", bbSiteID, "--agent-id", bbAgentID, "--targets", bbCIDR, "--yes", "--json"},
		[]string{"org", "create-scan", bbSiteID, "--agent-id", bbAgentID, "--targets", bbCIDR},
		[]string{"account", "rotate-key", bbKeyID, "--yes", "--json"},
		[]string{"account", "rotate-key", bbKeyID},
		[]string{"account", "remove-key", bbKey2ID, "--yes", "--json"},
		[]string{"account", "remove-key", bbKey2ID},
		[]string{"scan-watch", bbSiteID, "--targets", bbCIDR, "--interval", "1s", "--timeout", "60s", "--json"},
		// The local store, exports and the novel commands.
		[]string{"sync", "--json"},
		[]string{"workflow", "archive", "--json"},
		[]string{"export", "runzero-export", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		[]string{"export", "runzero-export", "--format", "json"},
		[]string{"export", "account", "--format", "json"},
		[]string{"search", "HQ", "--json"},
		[]string{"search", "Echo", "--json", "--data-source", "local"},
		[]string{"workflow", "status", "--json"},
	)
	b = append(b, calls(novels()...)...)
	return append(b, calls(
		// A sync that meets a 401 (a filtered walk into its own store, after
		// the novels: the main store stays whole).
		[]string{"sync", "--db", filepath.Join(e.home, "denied.db"), "--resources", "org-assets", "--param", "search=" + bbDenied, "--json"},
		// A credential on the command line: a credential-named query value
		// (sent, and echoed back), the same in a dry run, an unknown flag, a
		// positional, a key typed as auth set-token's argument, feedback text;
		// the key and its header value alike.
		[]string{"sync", "--db", filepath.Join(e.home, "param.db"), "--resources", "org-assets", "--param", "password=" + bbArgvSec, "--json"},
		[]string{"sync", "--db", filepath.Join(e.home, "param.db"), "--resources", "org-assets", "--global-param", "password=" + bbArgvSec, "--dry-run"},
		[]string{"org", "get-assets", "--json", "--" + key},
		[]string{"org", "get-asset", key, "--json"},
		[]string{"account", "get-key", "--no-such-flag", key},
		[]string{"auth", "set-token", key},
		[]string{"feedback", "noted the key " + key + " in a ticket"},
		[]string{"feedback", "noted the header " + bearer(key) + " in a ticket"},
		// Previews: a dry run sends nothing.
		[]string{"org", "create-scan", bbSiteID, "--agent-id", bbAgentID, "--targets", key, "--dry-run"},
		[]string{"org", "get-asset", key, "--dry-run"},
		[]string{"account", "remove-key", key, "--dry-run"},
		// Learnings, playbooks, teach logs and profiles land in local files.
		[]string{"teach", "--query", "find assets " + key, "--resource", bbAssetID, "--resource-type", "org",
			"--notes", "rotated " + key, "--playbook-notes", "use " + bearer(key), "--no-validate"},
		[]string{"teach", "--query", "find " + key, "--resource", key, "--resource-type", "org"}, // a held resource id: refused, logged to teach.log
		[]string{"teach-playbook", "--query", "assets with " + key, "--notes", "use " + bearer(key)},
		[]string{"profile", "save", "held", "--description", "uses " + key, "--select", "id"},
		[]string{"profile", "show", "held"},
		[]string{"profile", "list"},
		[]string{"feedback", "list"},
		[]string{"learnings", "list", "--json"},
		[]string{"recall", "find assets " + key},
		[]string{"doctor"},
		[]string{"auth", "status"},
		[]string{"agent-context"},
	)...)
}

func tool(name string, args map[string]any) map[string]any {
	return map[string]any{"name": name, "arguments": args}
}

// read calls runzero_read on endpoint with params.
func read(endpoint string, params map[string]any) map[string]any {
	args := map[string]any{"endpoint_id": endpoint}
	if params != nil {
		args["params"] = params
	}
	return tool("runzero_read", args)
}

// execute calls runzero_execute on endpoint with params; extra sets the
// call's other arguments (confirm).
func execute(endpoint string, params, extra map[string]any) map[string]any {
	args := map[string]any{"endpoint_id": endpoint}
	if params != nil {
		args["params"] = params
	}
	for k, v := range extra {
		args[k] = v
	}
	return tool("runzero_execute", args)
}

// scanParams are a scan's parameters (PUT /org/sites/{site_id}/scan).
func scanParams(targets string) map[string]any {
	return map[string]any{"site_id": bbSiteID, "agent": bbAgentID, "targets": targets}
}

// callName is the tool a call names.
func callName(call map[string]any) string {
	name, _ := call["name"].(string)
	return name
}

// endpointOf is the endpoint a runzero_read or runzero_execute call names
// ("" for any other tool).
func endpointOf(call map[string]any) string {
	args, _ := call["arguments"].(map[string]any)
	id, _ := args["endpoint_id"].(string)
	return id
}

// mcpReads are read-only tools across every response shape the fake has,
// and every novel tool; the reads whose record splits the key across adjacent
// values (mcpSplitReads) come last. The event log's pages (the first halves
// of two split credentials, page 1 of two) run in their own sessions
// (TestMCPSplitAcrossPagesWithheld, TestMCPSessionHalfKeepsLaterResults).
func mcpReads() []map[string]any {
	none := map[string]any{}
	return append([]map[string]any{
		read("account.get-keys", nil),
		read("org.get-key", nil),
		read("account.get-organizations", nil),
		read("account.get-organization-export-tokens", map[string]any{"org_id": bbOrgID}),
		read("account.get-organization-export-token", map[string]any{"org_id": bbOrgID, "key_id": bbETID}),
		read("account.get-users", nil),
		read("account.get-credentials", nil),
		read("org.get-assets", nil),
		read("org.get-asset", map[string]any{"asset_id": bbDenied}),
		read("org.get-assets", map[string]any{"search": bbDenied}),
		read("org.get-services", nil),
		read("org.get-sites", nil),
		read("org.get-task", map[string]any{"task_id": bbTaskID}),
		read("runzero-export.services-json", nil),
		read("runzero-export.certificates-json", nil),
		execute("account.get-apitoken", map[string]any{"grant_type": "client_credentials", "client_id": bbClientID, "client_secret": bbMCPArgSec}, map[string]any{"confirm": true}),
		tool("runzero_search", map[string]any{"query": "api keys"}),
		tool("runzero_get", map[string]any{"endpoint_id": "account.get-keys"}),
		tool("context", none),
		tool("triage", none),
		tool("affected", map[string]any{"cve": bbCVE}),
		tool("software_rollup", none),
		tool("certs_expiring", map[string]any{"days": 3650}),
		tool("stale", map[string]any{"days": 1}),
		tool("exposure_map", map[string]any{"cidr": bbCIDR}),
		tool("exposure_delta", none),
		tool("diff", none),
		tool("inventory_list", map[string]any{"entity": "assets"}),
		tool("inventory_status", none),
		tool("search", map[string]any{"query": "HQ"}),
		tool("sql", map[string]any{"query": "SELECT data FROM resources"}),
		tool("recall", map[string]any{"query": "HQ"}),
		tool("learnings_list", none),
		tool("learnings_candidates", none),
		tool("learnings_stats", none),
		tool("playbook_list", none),
		tool("workflow_status", none),
	}, mcpSplitReads()...)
}

// mcpSplitReads are the reads whose record splits the credentials the request
// carried across adjacent values, lines and spaces (the fake's "split"
// member): once the second half of a split printed, the session's spelling
// record withholds whole any later result that echoes the key in another
// form, so they run after the other reads.
func mcpSplitReads() []map[string]any {
	return []map[string]any{
		read("account.get-key", map[string]any{"key_id": bbKeyID}),
		read("account.get-organization", map[string]any{"org_id": bbOrgID}),
		read("org.get-asset", map[string]any{"asset_id": bbAssetID}),
	}
}

// mcpWrites is every write that takes confirm, called without it: each must
// answer with a preview and send nothing (runzero_execute's and the endpoint
// gate's {"preview":true,"confirmation_required":true,...}; a mirrored
// command's "Preview only: nothing was sent" note): a PUT, a POST and a
// DELETE through runzero_execute, import, scan_watch and the recipe intent.
func mcpWrites() []map[string]any {
	return []map[string]any{
		execute("org.create-scan", scanParams(bbCIDR), nil),
		execute("account.rotate-apitoken", map[string]any{"grant_type": "client_credentials", "client_id": bbClientID, "client_secret": "not-a-held-secret-1"}, nil),
		execute("account.get-apitoken", map[string]any{"grant_type": "client_credentials", "client_id": bbClientID, "client_secret": "not-a-held-secret-1"}, nil),
		execute("account.remove-key", map[string]any{"key_id": bbKey2ID}, nil),
		tool("import", map[string]any{"resource": "account"}),
		tool("scan_watch", map[string]any{"site_id": bbSiteID, "targets": bbCIDR, "interval": "1s"}),
		tool("cve_blast_radius_during_an_incident", map[string]any{"slug": bbCVE}),
	}
}

// mcpBypasses try to turn a write preview into a send without confirm true:
// each must preview or be refused, and send nothing. key is the key the
// process sends; home is where a flag-shaped recipe argument would write.
func mcpBypasses(key, home string) []map[string]any {
	return []map[string]any{
		execute("org.create-scan", scanParams(bbCIDR), map[string]any{"confirm": "true"}),
		execute("org.create-scan", scanParams(bbCIDR), map[string]any{"confirm": 1}),
		execute("org.create-scan", scanParams(bbCIDR), map[string]any{"confirm": []any{true}}),
		execute("org.create-scan", scanParams(bbCIDR), map[string]any{"dry-run": false, "yes": true}),
		execute("account.remove-key", map[string]any{"key_id": bbKey2ID, "confirm": true}, nil),
		// A read tool on a write endpoint is refused.
		read("account.remove-key", map[string]any{"key_id": bbKey2ID}),
		read("org.create-scan", scanParams(bbCIDR)),
		tool("import", map[string]any{"resource": "account", "dry-run": false}),
		tool("import", map[string]any{"resource": "account", "yes": true}),
		// import's --input names a host file: it is not an MCP argument, so
		// even with confirm true no host file is read or sent.
		tool("import", map[string]any{"resource": "account", "input": filepath.Join(home, "host-file.jsonl"), "confirm": true}),
		tool("scan_watch", map[string]any{"site_id": bbSiteID, "targets": bbCIDR, "confirm": "true"}),
		// A flag-shaped recipe argument stays a positional (after "--"),
		// never a --db that opens or creates a host file.
		tool("cve_blast_radius_during_an_incident", map[string]any{"slug": "--db=" + filepath.Join(home, "recipe-flag.db")}),
		// A write whose argument repeats the held key: refused (a
		// credential is never passed to a tool) or previewed, never sent.
		execute("org.create-scan", scanParams(key), nil),
		execute("account.remove-key", map[string]any{"key_id": key}, nil),
	}
}

// mcpLocalWrites are mirrored tools that write only local files (no gate by
// design): their child CLI output passes the server's chokepoint too. The
// sync's credential-named query value is sent, and echoed back.
func mcpLocalWrites() []map[string]any {
	return []map[string]any{
		tool("export", map[string]any{"resource": "runzero-export", "format": "json"}),
		tool("sync", map[string]any{"resources": []any{"account-keys", "account-orgs", "runzero-export-services-json"}}),
		tool("sync", map[string]any{"resources": []any{"org-assets"}, "param": []any{"password=" + bbMCPArgSec}}),
		tool("inventory_sync", map[string]any{"only": "assets,services,certificates"}),
	}
}

// mcpBattery is every MCP call the suite makes; key is the key the process
// sends.
func mcpBattery(key, home string) []map[string]any {
	return append(append(append(mcpReads(), mcpWrites()...), mcpBypasses(key, home)...), mcpLocalWrites()...)
}

// previewed reports an MCP reply that is a write preview.
var previewed = regexp.MustCompile(`Preview only: nothing was sent|confirmation_required\\*"\s*:\s*true`)

// previewOrWithheld reports an unconfirmed write's answer as its preview, or,
// for a call whose arguments carry a client_secret (an MCP argument the
// credential kit holds), as the kit's notice: the preview repeats the
// argument, so it is withheld whole.
func previewOrWithheld(call map[string]any, reply string) bool {
	return previewed.MatchString(reply) ||
		strings.Contains(fmt.Sprint(call["arguments"]), "client_secret:") && strings.Contains(reply, kitNotice("mcp"))
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

// checkOneAuthWay fails when a request authenticated any way but the one
// header Authorization: Bearer <key>.
func checkOneAuthWay(t *testing.T, sv *server) {
	t.Helper()
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.otherAuth != 0 {
		t.Errorf("%d request(s) carried a credential another way (a second Authorization header, one that is not Bearer over the key, X-Api-Key, a query key); runZero takes only Authorization: Bearer", sv.otherAuth)
	}
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	deliverFile := filepath.Join(e.home, "deliver.json")
	// Every battery call's output is checked here too, so each call meets
	// the stored key and one more source (TestNoCredentialInAnyOutput).
	// This is the package's longest chain of processes, so the calls before
	// the first sync (API calls that read nothing local) run beside the rest,
	// in a home of their own with the same stored key and fake runZero; both
	// homes are scanned below.
	b := battery(t, e, deliverFile, bbStored)
	split := slices.IndexFunc(b, func(c cliCall) bool { return c.args[0] == "sync" })
	if split <= 0 {
		t.Fatalf("the battery has no sync after its API calls (index %d)", split)
	}
	api := newEnv(t, sv, credStored)
	apiOut, apiErr := make([]string, split), make([]error, split)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i, c := range b[:split] {
			apiOut[i], apiErr[i] = api.runErr(c.stdin, c.args...)
		}
	}()
	for _, c := range b[split:] {
		assertNoEcho(t, fmt.Sprintf("output of %v", c.args), e.runIn(t, c.stdin, c.args...))
	}
	wg.Wait()
	for i, c := range b[:split] {
		if apiErr[i] != nil {
			t.Fatalf("%v: %v", c.args, apiErr[i])
		}
		assertNoEcho(t, fmt.Sprintf("output of %v", c.args), apiOut[i])
	}
	// Controls without a credential: the same sinks write, so the scans
	// below are not vacuous. The credential kit withholds a whole record
	// that repeats a held value (a delivered file writes nothing, an export
	// line or a stored row is withheld), so the delivered file, the export
	// and the store are also written from answers that repeat nothing.
	sv.setQuiet(everyPath)
	controlDeliver, controlExport := filepath.Join(e.home, "deliver-control.json"), filepath.Join(e.home, "export-control.jsonl")
	e.run(t, "org", "get-assets", "--json", "--deliver", "file:"+controlDeliver)
	e.run(t, "export", "runzero-export", "--format", "jsonl", "--output", controlExport)
	e.run(t, "sync", "--json")
	e.run(t, "inventory", "sync", "--json")
	sv.setQuiet(nil)
	e.run(t, "feedback", "control note HQ-Feedback")
	e.run(t, "teach", "--query", "find assets control", "--resource", bbAssetID, "--resource-type", "org",
		"--notes", "rotated control", "--no-validate")
	e.run(t, "teach", "--resource", bbAssetID, "--resource-type", "org") // no --query: teach.log records the refusal
	e.run(t, "profile", "save", "control", "--description", "HQ-Profile control", "--select", "id")
	// The whole MCP battery's session output too (TestNoCredentialInAnyOutput
	// runs its reads and local writes in one more mode each).
	mcpOut := e.mcpCalls(t, mcpBattery(bbStored, e.home))
	if !strings.Contains(mcpOut, fmt.Sprintf(`"id":%d`, len(mcpBattery(bbStored, e.home)))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", mcpOut)
	}
	assertNoEcho(t, "MCP session", mcpOut)
	if n, all := sv.sawKey(bbStored); n == 0 {
		t.Fatalf("no request carried the stored key as Authorization: Bearer (saw %d distinct values); the battery ran unauthenticated", len(all))
	}
	checkOneAuthWay(t, sv)
	var sawCreds bool
	scanned := 0
	for _, home := range []string{e.home, api.home} {
		_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "credentials") {
				return nil
			}
			data, _ := os.ReadFile(path)
			// The credential store: the one file allowed to hold a credential.
			sawCreds = sawCreds || bytes.Contains(data, []byte(bbStored))
			if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				rel, _ := filepath.Rel(home, path)
				t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
			}
			return nil
		})
		scanned += scanFiles(home, func(rel string, data []byte) {
			assertNoEchoIn(t, "file "+rel, string(data), !isStoreFile(rel))
		})
	}
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-token wrote no credential store")
	}
	if _, err := os.Stat(filepath.Join(e.home, "recipe-flag.db")); err == nil {
		t.Errorf("an MCP recipe argument shaped like --db created a host file")
	}
	// The battery really wrote data, so the scan above is not vacuous: the
	// controls' delivered record and export, and the battery's export,
	// whose records that repeat no credential are written whole.
	if b, err := os.ReadFile(controlDeliver); err != nil || !bytes.Contains(b, []byte(bbHost)) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(controlDeliver), err)
	}
	for _, f := range []string{controlExport, filepath.Join(e.home, "export.jsonl")} {
		if b, err := os.ReadFile(f); err != nil || !bytes.Contains(b, []byte(bbHost2)) {
			t.Errorf("%s: want the exported record (err %v):\n%.1500s", filepath.Base(f), err, b)
		}
	}
	// The battery's held-credential feedback is withheld whole in the
	// ledger: the kit's notice stands in its place (teach is silent by
	// design; the file scan above covers both).
	var sawStore, sawKeyRow, sawOrgRow, sawService, sawInventory, sawLearning, sawFeedback, sawFeedbackNotice, sawProfile, sawTeachLog bool
	scanFiles(e.home, func(rel string, b []byte) {
		name := filepath.Base(rel)
		if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") {
			sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
			// The synced API key and organization rows were stored (their
			// tokens withheld: the scan above proves it).
			sawKeyRow = sawKeyRow || bytes.Contains(b, []byte(bbKeyID))
			sawOrgRow = sawOrgRow || bytes.Contains(b, []byte(bbOrgID))
			// The services export was stored (its api_key withheld).
			sawService = sawService || bytes.Contains(b, []byte(bbServiceID))
			// inventory sync stored the vulnerability the novels read.
			sawInventory = sawInventory || bytes.Contains(b, []byte(bbCVE))
			// The control teach run's learning row was written.
			sawLearning = sawLearning || bytes.Contains(b, []byte("rotated control"))
		}
		// The control feedback entry was written.
		sawFeedback = sawFeedback || (name == "feedback.jsonl" && bytes.Contains(b, []byte("HQ-Feedback")))
		sawFeedbackNotice = sawFeedbackNotice || (name == "feedback.jsonl" && bytes.Contains(b, []byte(kitNotice("file"))))
		sawProfile = sawProfile || bytes.Contains(b, []byte("HQ-Profile control"))
		sawTeachLog = sawTeachLog || (name == "teach.log" && bytes.Contains(b, []byte("missing --query")))
	})
	for what, ok := range map[string]bool{
		"no local store holds a fetched record":                                              sawStore,
		"no local store holds the synced API key row":                                        sawKeyRow,
		"no local store holds the synced organization row":                                   sawOrgRow,
		"no local store holds the synced service":                                            sawService,
		"no local store holds the inventory's vulnerability":                                 sawInventory,
		"no local store holds the taught learning; the teach run wrote nothing":              sawLearning,
		"no feedback.jsonl holds the control feedback entry; the feedback run wrote nothing": sawFeedback,
		"no feedback.jsonl holds the kit's notice for the held-credential feedback":          sawFeedbackNotice,
		"no profile file holds the control profile; profile save wrote nothing":              sawProfile,
		"no teach.log holds the control refusal; teach wrote no log":                         sawTeachLog,
	} {
		if !ok {
			t.Error(what)
		}
	}
	// Some reader asked for the second export page: the split-across-pages
	// echo really ran; and the 401 echoes really ran (org get-asset, org
	// get-assets --search, the denied sync, MCP get-asset and get-assets).
	sv.mu.Lock()
	denials := sv.denials
	sv.mu.Unlock()
	if denials < 5 {
		t.Errorf("only %d request(s) met a 401; the error-echo runs did not all reach the fake", denials)
	}
	if sv.laterPages("/account/events.json") == 0 {
		t.Errorf("no reader asked for the event log's second page; the split-across-pages echo did not run")
	}
}

// Class secret-in-output, once per credential source that is not the stored
// key alone: the environment's key over a stored key (held unused) or over a
// config file's api_key (held unused), a config file's api_key, a config
// file's auth_header (sent as written), its api_key beside access_token (held
// unused) and its access_token alone. (The stored key as the sent value runs
// in TestNoCredentialInAnyWrittenFile; TestEveryCredentialSourceSendsOneKey
// covers every source with a short battery.)
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	// Every mode is its own fake runZero and home, so the modes run side by
	// side. By default call i runs in mode i mod 6 only: every call still
	// meets two credential sources, since TestNoCredentialInAnyWrittenFile
	// checks every call's output with the stored key, and every mode runs
	// about fifteen calls. Each CLI start costs the credential kit about two
	// CPU seconds, and the package has to fit go test's ten-minute limit on a
	// 4-vCPU CI runner; RUNZERO_BB_FULL=1 runs every call in every mode.
	full := os.Getenv("RUNZERO_BB_FULL") == "1"
	for mi, mode := range []credMode{credEnvOverStored, credEnvOverFile, credConfigFile, credConfigHeader, credConfigBoth, credConfigAccess} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for i, c := range battery(t, e, deliverFile, mode.sent()) {
				if !full && i%6 != mi {
					continue
				}
				assertNoEcho(t, fmt.Sprintf("output of %v", c.args), e.runIn(t, c.stdin, c.args...))
			}
			// A held value no API receives, typed where the program repeats
			// it: a path the fake echoes, and previews.
			for _, held := range mode.held() {
				for _, c := range []cliCall{
					{args: []string{"org", "get-asset", held, "--json"}},
					{args: []string{"org", "create-scan", bbSiteID, "--agent-id", bbAgentID, "--targets", held, "--dry-run"}},
					{args: []string{"account", "remove-key", held, "--dry-run"}},
				} {
					assertNoEcho(t, fmt.Sprintf("output of %v", c.args), e.runIn(t, c.stdin, c.args...))
				}
			}
			// An answer that repeats a held value is withheld whole: stdout is
			// the credential kit's notice alone. The same answer repeating
			// nothing prints as valid JSON with runZero's public data, and an
			// API key's token is masked by name.
			for _, args := range [][]string{{"org", "get-assets", "--json"}, {"account", "get-keys", "--json"}} {
				stdoutOf := func() []byte {
					cmd := exec.Command(bbCLI, args...)
					cmd.Env, cmd.Dir = e.env, e.home
					stdout, err := cmd.Output()
					if err != nil {
						t.Fatalf("%v: %v", args, err)
					}
					return stdout
				}
				if got := strings.TrimSpace(string(stdoutOf())); got != kitNotice("stdout") {
					t.Errorf("%v: want the kit's notice alone for an answer that repeats the key, got:\n%.1500s", args, got)
				}
				sv.setQuiet(everyPath)
				stdout := stdoutOf()
				sv.setQuiet(nil)
				want := []*regexp.Regexp{publicField("id", bbAssetID), publicValue(bbAddr), publicField("os", bbOS)}
				if args[0] == "account" {
					want = []*regexp.Regexp{publicField("id", bbKeyID), publicField("last_used_ip", bbLastUsedIP), publicField("usage_today", bbUsageToday),
						publicField("token", "[credential withheld]")}
				}
				var doc any
				ok := json.Unmarshal(stdout, &doc) == nil
				for _, re := range want {
					ok = ok && re.Match(stdout)
				}
				if !ok {
					t.Fatalf("%v: want valid JSON with the record and its public fields, got:\n%.1500s", args, stdout)
				}
				assertNoEcho(t, fmt.Sprintf("quiet output of %v", args), string(stdout))
			}
			// The MCP battery the same way: its API reads and local writes
			// (about twenty CPU seconds a mode) run in one mode each, and
			// TestNoCredentialInAnyWrittenFile checks the whole battery's
			// session with the stored key. The writes, the bypasses and the
			// reads of this mode's own store (search, sql, recall) run in
			// every mode.
			var mcp []map[string]any
			for i, c := range mcpReads() {
				if local := c["name"] == "search" || c["name"] == "sql" || c["name"] == "recall"; full || local || i%6 == mi {
					mcp = append(mcp, c)
				}
			}
			mcp = append(append(mcp, mcpWrites()...), mcpBypasses(mode.sent(), e.home)...)
			for i, c := range mcpLocalWrites() {
				if full || i%6 == mi {
					mcp = append(mcp, c)
				}
			}
			out := e.mcpCalls(t, mcp)
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcp))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			if !strings.Contains(out, bbHost) {
				t.Fatalf("MCP results lost the record itself:\n%.2000s", out)
			}
			// The fake echoed the key it really received, so the scans above
			// covered it; no other credential source was sent.
			if n, all := sv.sawKey(mode.sent()); n == 0 || len(all) != 1 {
				t.Fatalf("want every request to carry the %s key, got %d of it and distinct values %q", mode, n, all)
			}
			checkOneAuthWay(t, sv)
		})
	}
}

// publicField matches name and value in JSON output, raw or inside a JSON
// string (an MCP result escapes its quotes), with <, > and & as written or
// as the \u003c, \u003e and \u0026 escapes Go's JSON encoder writes.
func publicField(name, value string) *regexp.Regexp {
	return regexp.MustCompile(`\\*"` + regexp.QuoteMeta(name) + `\\*":\s*\\*"?` + jsonEscapedValue(value))
}

// jsonEscapedValue is a pattern for value as a JSON encoder may write it.
func jsonEscapedValue(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '<':
			b.WriteString(`(?:<|\\+u003c)`)
		case '>':
			b.WriteString(`(?:>|\\+u003e)`)
		case '&':
			b.WriteString(`(?:&|\\+u0026)`)
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}

// publicValue matches value anywhere (a table cell, a CSV field, a JSON
// string, an MCP result).
func publicValue(value string) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(value))
}

// runzeroPublic is runZero data an MSP acts on, by the name the check
// reports: none may be withheld. Several read like credentials by name
// (export_token_last_used_by, last_used_ip, token_type, authentication,
// signature_algorithm, insecure, a credential's id).
func runzeroPublic() map[string]*regexp.Regexp {
	return map[string]*regexp.Regexp{
		"asset id":                  publicField("id", bbAssetID),
		"hostname":                  publicValue(bbHost),
		"address":                   publicValue(bbAddr),
		"mac":                       publicValue(bbMAC),
		"os":                        publicField("os", bbOS),
		"site id":                   publicField("site_id", bbSiteID),
		"service id":                publicField("service_id", bbServiceID),
		"service port":              publicField("service_port", "445"),
		"service protocol":          publicField("service_protocol", "smb"),
		"key id":                    publicField("id", bbKeyID),
		"key organization_id":       publicField("organization_id", bbOrgID),
		"key comment":               publicField("comment", bbKeyComment),
		"last_used_ip":              publicField("last_used_ip", bbLastUsedIP),
		"usage_today":               publicField("usage_today", bbUsageToday),
		"created_at":                publicField("created_at", bbCreatedAt),
		"export_token_last_used_by": publicField("export_token_last_used_by", bbExportUsedBy),
		"export token id":           publicField("id", bbETID),
		"export token last_used_by": publicField("last_used_by", bbExportUsedBy),
		"credential id":             publicField("id", bbCredID),
		"credential insecure":       publicField("insecure", "true"),
		"task id":                   publicField("id", bbTaskID),
		"cve":                       publicField("vulnerability_cve", bbCVE),
		"severity":                  publicField("vulnerability_severity", "critical"),
		"cvss":                      publicField("vulnerability_cvss3_base_score", "9.8"),
		"cert subject":              publicField("subject", bbCertSubject),
		"cert cn":                   publicField("cn", bbCertCN),
		"cert issuer":               publicField("issuer", bbCertIssuer),
		"cert validity_end":         publicField("validity_end", bbCertEnd),
		"cert fp_sha256":            publicField("fp_sha256", bbCertFP256),
		"cert signature_algorithm":  publicField("signature_algorithm", bbCertSigAlg),
		"wireless authentication":   publicField("authentication", bbWifiAuth),
		"wireless essid":            publicField("essid", bbESSID),
		"token_type":                publicField("token_type", "bearer"),
	}
}

// runzero-credential-names-exact: runZero's asset, service, key,
// organization, credential, vulnerability, certificate and wireless data
// print as the API returned them, from the CLI and MCP alike, and the store
// keeps them. After an inventory sync the novels that join them (triage,
// affected, software rollup, certs-expiring, inventory list) print them
// whole.
func TestRunzeroPublicFieldsAreNotWithheld(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	// The fake repeats nothing it was sent: the credential kit withholds a
	// whole answer that repeats a held value (TestNoCredentialInAnyOutput).
	sv := quietServer(t)
	e := newEnv(t, sv, credEnv)
	var cli strings.Builder
	for _, args := range [][]string{
		{"org", "get-assets", "--json"},
		{"org", "get-services", "--json"},
		{"org", "get-tasks", "--json"},
		{"account", "get-keys", "--json"},
		{"account", "get-organizations", "--json"},
		{"account", "get-organization-export-tokens", bbOrgID, "--json"},
		{"account", "get-credentials", "--json"},
		{"account", "get-apitoken", "--grant-type", "client_credentials", "--client-id", bbClientID, "--client-secret", bbArgvSec, "--json"},
		{"runzero-export", "vulnerabilities-json", "--json"},
		{"runzero-export", "certificates-json", "--json"},
		{"runzero-export", "wireless-json", "--json"},
	} {
		cli.WriteString(e.run(t, args...))
	}
	mcp := e.mcpCalls(t, []map[string]any{
		read("org.get-assets", nil),
		read("org.get-services", nil),
		read("org.get-tasks", nil),
		read("account.get-keys", nil),
		read("account.get-organizations", nil),
		read("account.get-organization-export-tokens", map[string]any{"org_id": bbOrgID}),
		read("account.get-credentials", nil),
		execute("account.get-apitoken", map[string]any{"grant_type": "client_credentials", "client_id": bbClientID, "client_secret": bbMCPArgSec}, map[string]any{"confirm": true}),
		read("runzero-export.vulnerabilities-json", nil),
		read("runzero-export.certificates-json", nil),
		read("runzero-export.wireless-json", nil),
	})
	for where, got := range map[string]string{"CLI": cli.String(), "MCP": mcp} {
		for name, want := range runzeroPublic() {
			if !want.MatchString(got) {
				t.Errorf("%s withheld runZero's %s (want %s):\n%.3000s", where, name, want, got)
			}
		}
		assertNoEcho(t, where, got)
	}
	// The novels print them whole after an inventory sync.
	e.run(t, "inventory", "sync", "--json")
	novel := map[string]string{
		"triage":                      e.run(t, "triage", "--json"),
		"affected":                    e.run(t, "affected", bbCVE, "--json"),
		"software rollup":             e.run(t, "software", "rollup", "--json"),
		"certs-expiring":              e.run(t, "certs-expiring", "--json"),
		"inventory list":              e.run(t, "inventory", "list", "assets", "--json"),
		"inventory list certificates": e.run(t, "inventory", "list", "certificates", "--json"),
		"inventory list wireless":     e.run(t, "inventory", "list", "wireless", "--json"),
	}
	novel["MCP novels"] = e.mcpCalls(t, []map[string]any{
		tool("triage", map[string]any{}),
		tool("affected", map[string]any{"cve": bbCVE}),
		tool("software_rollup", map[string]any{}),
		tool("certs_expiring", map[string]any{}),
		tool("inventory_list", map[string]any{"entity": "assets"}),
	})
	wants := map[string][]string{
		"triage":                      {bbAssetID, bbHost, bbAddr, bbOS, bbCVE, "critical"},
		"affected":                    {bbAssetID, bbHost, bbAddr, bbCVE, "critical", "445"},
		"software rollup":             {bbSWProduct, bbSWVersion},
		"certs-expiring":              {bbCertID, bbCertSubject, bbCertIssuer, bbAssetID},
		"inventory list":              {bbAssetID, bbHost, bbAddr, bbMAC, bbOS},
		"inventory list certificates": {bbCertSubject, bbCertFP256, bbCertSigAlg, bbCertEnd},
		"inventory list wireless":     {bbESSID, bbWifiAuth},
		"MCP novels":                  {bbAssetID, bbHost, bbAddr, bbCVE, bbSWProduct, bbCertSubject, bbMAC},
	}
	for where, out := range novel {
		for _, v := range wants[where] {
			if !strings.Contains(out, v) {
				t.Errorf("%s withheld %q:\n%.3000s", where, v, out)
			}
		}
		assertNoEcho(t, where, out)
	}
	// The store keeps them too.
	var host, cve, fp bool
	scanFiles(e.home, func(rel string, b []byte) {
		if isStoreFile(rel) {
			host = host || bytes.Contains(b, []byte(bbHost))
			cve = cve || bytes.Contains(b, []byte(bbCVE))
			fp = fp || bytes.Contains(b, []byte(bbCertFP256))
		}
	})
	if !host || !cve || !fp {
		t.Errorf("the local store dropped an asset's hostname (%v), a CVE (%v) or a certificate's fp_sha256 (%v)", host, cve, fp)
	}
}

// Class name-reads-as-a-credential: runZero names that read like credentials
// are data an MSP acts on (a wireless network's authentication mode, a
// certificate's signature_algorithm, the IP that last used an export token, a
// key's last_used_ip, an access token's token_type, a credential's insecure
// flag and id). They print and are stored as runZero returned them (in JSON
// and in a --plain cell), and nothing in them is held: a held "wpa2-psk" or
// "SHA1-RSA" would withhold later MCP results of the session whole.
func TestPublicNamesThatReadAsCredentialsAreNotHeld(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	// The fake repeats nothing it was sent: the credential kit withholds a
	// whole answer that repeats a held value (TestNoCredentialInAnyOutput).
	sv := quietServer(t)
	e := newEnv(t, sv, credEnv)
	for _, format := range []string{"--json", "--plain"} {
		cli := e.run(t, "runzero-export", "wireless-json", format)
		cli += e.run(t, "runzero-export", "certificates-json", format)
		for _, v := range []string{bbWifiAuth, bbESSID, bbCertSigAlg} {
			if !strings.Contains(cli, v) {
				t.Errorf("CLI %s withheld %q:\n%.3000s", format, v, cli)
			}
		}
		assertNoEcho(t, "CLI "+format, cli)
	}
	mcp := e.mcpPhases(t,
		[]map[string]any{read("runzero-export.wireless-json", nil), read("runzero-export.certificates-json", nil),
			read("account.get-keys", nil), read("account.get-credentials", nil)},
		[]map[string]any{read("org.get-sites", nil), read("org.get-task", map[string]any{"task_id": bbTaskID})})
	for _, v := range []string{bbWifiAuth, bbCertSigAlg, bbLastUsedIP, bbCredID} {
		if !strings.Contains(mcp, v) {
			t.Errorf("MCP withheld %q:\n%.3000s", v, mcp)
		}
	}
	replies := mcpReplies(mcp)
	if !strings.Contains(replies[5], "HQ-Record headquarters") || !strings.Contains(replies[6], bbTaskID) {
		t.Errorf("MCP withheld the reads after the wireless, certificate, key and credential data:\n%.1500s\n%.1500s", replies[5], replies[6])
	}
	assertNoEcho(t, "MCP", mcp)
	e.run(t, "inventory", "sync", "--json")
	certs := e.run(t, "certs-expiring", "--weak", "--plain")
	if !strings.Contains(certs, bbCertSubject) {
		t.Errorf("certs-expiring --weak --plain withheld the weak certificate:\n%.3000s", certs)
	}
	assertNoEcho(t, "certs-expiring", certs)
}

// Class credential-sent-to-another-host: a redirect off the configured origin
// (to another host, another port, plain http, or a non-ASCII homograph of
// console.runzero.com) is refused before it is followed, from the CLI,
// sync and MCP alike; no connection reaches another host or port, and nothing
// reaches the proxy trap that stands in for the network.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	cli := map[string]string{}
	for _, id := range []string{bbRedirect, bbRedirPort, bbRedirHTTP, bbRedirIDN} {
		cli["org get-asset "+id] = e.run(t, "org", "get-asset", id, "--json")
	}
	sv.mu.Lock()
	sv.redirectSync = true
	sv.mu.Unlock()
	cli["sync"] = e.run(t, "sync", "--resources", "account", "--json")
	cli["account get-agents"] = e.run(t, "account", "get-agents", "--json")
	// runZero's documented redirect: a task's log, data and change report
	// answer 303 to a temporary URL on another host.
	cli["org get-task-log"] = e.run(t, "org", "get-task-log", bbTaskID, "--json")
	cli["org get-task-scan-data"] = e.run(t, "org", "get-task-scan-data", bbTaskID)
	mcp := e.mcpCalls(t, []map[string]any{
		read("org.get-asset", map[string]any{"asset_id": bbRedirect}),
		read("org.get-asset", map[string]any{"asset_id": bbRedirPort}),
		read("org.get-asset", map[string]any{"asset_id": bbRedirHTTP}),
		read("org.get-asset", map[string]any{"asset_id": bbRedirIDN}),
		read("account.get-agents", nil),
		read("org.get-task-change-report", map[string]any{"task_id": bbTaskID}),
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		for _, id := range []string{bbRedirect, bbRedirPort, bbRedirHTTP, bbRedirIDN} {
			if strings.HasSuffix(m, "/org/assets/"+id) {
				redirected++
			}
		}
		if m == "GET /account/agents" || reTaskData.MatchString(strings.TrimPrefix(m, "GET ")) {
			redirected++
		}
	}
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("another host or port was contacted %d time(s)", n)
	}
	if redirected < 14 {
		t.Fatalf("only %d request(s) met a redirect; the CLI, sync and MCP calls did not all run", redirected)
	}
	for where, out := range cli {
		if !strings.Contains(out, "leave the configured Runzero host") {
			t.Errorf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
		assertNoEcho(t, where, out)
	}
	if c := strings.Count(mcp, "leave the configured Runzero host"); c < 6 {
		t.Errorf("MCP: want the pin refusal for each of the 6 redirects, got %d:\n%.3000s", c, mcp)
	}
	assertNoEcho(t, "MCP redirects", mcp)
	if seen := trapSeen(); len(seen) != 0 {
		t.Errorf("a request got past the pin and reached the proxy trap: %v", seen)
	}
}

// Class credential-sent-to-another-host, next-link form: an export page whose
// next_key is an absolute URL on another host never sends a request there,
// from the CLI's --all, sync and MCP alike (runZero documents next_key as an
// opaque key for start_key, so an absolute URL is an adversarial shape); the
// key stays on the configured host. The fake repeats nothing it was sent, so
// the first page prints (the credential kit withholds a whole page that
// repeats a held value) and its next link can be read back.
func TestOffOriginNextLinkNeverMovesTheKey(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	sv := quietServer(t)
	sv.offOriginNext = true
	e := newEnv(t, sv, credEnv)
	out := e.run(t, "runzero-export", "assets-json", "--page-size", "2", "--json")
	next := regexp.MustCompile(`https://localhost:[0-9]+[^"\\]*`).FindString(out)
	if next == "" {
		t.Fatalf("the first page printed no off-origin next_key; the test did not run:\n%.2000s", out)
	}
	out += e.run(t, "runzero-export", "assets-json", "--page-size", "2", "--all", "--json")
	out += e.run(t, "runzero-export", "assets-json", "--page-size", "2", "--start-key", next, "--json")
	out += e.run(t, "sync", "--resources", "runzero-export", "--param", "page_size=2", "--json")
	// A scan whose task id is an absolute URL on another host: scan-watch
	// polls it on the pinned host only.
	out += e.run(t, "scan-watch", bbSiteID, "--targets", bbCIDR, "--interval", "1s", "--timeout", "5s", "--json")
	out += e.mcpPhases(t, []map[string]any{read("runzero-export.assets-json", map[string]any{"page_size": 2})},
		[]map[string]any{read("runzero-export.assets-json", map[string]any{"page_size": 2, "start_key": next})})
	sv.mu.Lock()
	n, page1 := sv.offHost, sv.pages["/export/org/assets.json?start_key="]
	sv.mu.Unlock()
	if n != 0 {
		t.Errorf("the next link's host was contacted %d time(s)", n)
	}
	if page1 < 3 || sv.laterPages("/export/org/assets.json") < 2 {
		t.Errorf("only %d request(s) read the first page and %d a later one; the test did not run:\n%.2000s", page1, sv.laterPages("/export/org/assets.json"), out)
	}
	assertNoEcho(t, "next-link output", out)
	if seen := trapSeen(); len(seen) != 0 {
		t.Errorf("a request reached the proxy trap: %v", seen)
	}
	checkOneAuthWay(t, sv)
}

// mcpSplitHalf is the page-1 read of the event log, whose events split the
// first two credentials across two pages.
func mcpSplitHalf() map[string]any {
	return read("account.export-events-json", map[string]any{"page_size": 25})
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send is a read while no call sets
// confirm, though the battery calls every write (runzero_execute's PUT, POST
// and DELETE, import, scan_watch, the recipe intent) and tries to bypass the
// gate (confirm "true", 1 or [true], confirm inside params, dry-run false, a
// blocked yes, a read tool on a write endpoint); each write answers with a
// preview.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	// The store holds the synced data first, so the novels have something
	// to read while the writes preview: synced from answers that repeat
	// nothing (the credential kit withholds a record that repeats a held
	// value from the store).
	sv.setQuiet(everyPath)
	e.run(t, "sync", "--json")
	e.run(t, "inventory", "sync", "--json")
	e.run(t, "inventory", "sync", "--json")
	sv.setQuiet(nil)
	// A host file an import could read: were it read, its record would be
	// POSTed (and fail the no-write check below).
	if err := os.WriteFile(filepath.Join(e.home, "host-file.jsonl"), []byte(`{"name":"HQ-Record host file"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.resetMethods()
	// One session, in phases (numbered as mcpBattery numbers them): the reads
	// that split no credential, then the split reads, then every write.
	reads := mcpReads()
	plain := len(reads) - len(mcpSplitReads())
	writes := append(append(mcpWrites(), mcpBypasses(bbKey, e.home)...), mcpLocalWrites()...)
	out := e.mcpPhases(t, reads[:plain], reads[plain:], writes)
	if !strings.Contains(out, bbHost) {
		t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
	}
	replies := mcpReplies(out)
	for i, call := range mcpWrites() {
		id := len(mcpReads()) + i + 1
		if r := replies[id]; !previewOrWithheld(call, r) {
			t.Errorf("%s %v without confirm did not return a preview:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	for i, call := range mcpBypasses(bbKey, e.home) {
		id := len(mcpReads()) + len(mcpWrites()) + i + 1
		if r := replies[id]; r == "" || (!previewed.MatchString(r) && !strings.Contains(r, `"isError":true`) && !strings.Contains(r, `"error":{`)) {
			t.Errorf("%s %v was neither previewed nor refused:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	// The novels answered from the synced store and the reads from the fake
	// (a refusal would leave the leak checks vacuous for them).
	for i, call := range reads[:plain] {
		if strings.Contains(fmt.Sprint(call["arguments"]), bbDenied) {
			continue // answered 401 on purpose
		}
		want := map[string]string{
			"runzero_read account.get-keys": bbKeyID, "runzero_read account.get-organizations": bbExportUsedBy,
			"runzero_read account.get-organization-export-tokens": bbETID, "runzero_read account.get-users": "user-sam-0002",
			"runzero_read org.get-assets": bbHost, "runzero_read org.get-sites": bbSiteID, "runzero_read org.get-services": bbServiceID,
			"runzero_read runzero-export.certificates-json": bbCertSubject, "runzero_execute account.get-apitoken": `bearer`,
			"triage": bbAssetID, "affected": bbHost, "software_rollup": bbSWProduct, "certs_expiring": bbCertSubject,
			"exposure_map": "smb", "inventory_list": bbMAC, "runzero_search": "account.get-keys",
		}[strings.TrimSpace(callName(call)+" "+endpointOf(call))]
		// The fake repeats the key it was sent, so an answer withheld whole
		// by the credential kit (its notice) is an answer that arrived.
		if r := replies[i+1]; want != "" && !strings.Contains(r, want) && !strings.Contains(r, kitNotice("mcp")) {
			t.Errorf("%s %v read nothing (want %q):\n%.1500s", call["name"], call["arguments"], want, r)
		}
	}
	mints := 0
	for _, m := range sv.allMethods() {
		if m == "POST /account/api/token" {
			mints++ // mcpReads' one confirmed mint; mcpWrites' unconfirmed one must preview
			continue
		}
		if isWrite(m) {
			t.Errorf("the MCP server sent %s without confirm", m)
		}
	}
	if mints != 1 {
		t.Errorf("want exactly the one confirmed token mint, got %d", mints)
	}
	if _, err := os.Stat(filepath.Join(e.home, "recipe-flag.db")); err == nil {
		t.Errorf("an MCP recipe argument shaped like --db created a host file")
	}
	checkOneAuthWay(t, sv)
}

// Control for the test above: with confirm set to boolean true the same
// writes reach the API exactly once each (a PUT, a POST and a DELETE), so
// the gate previews rather than drops; the string "true" and the number 1
// still preview. The POST's answer is a documented credential (a rotated
// client secret): withheld.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	writes := func(confirm any) []map[string]any {
		c := map[string]any{"confirm": confirm}
		return []map[string]any{
			execute("org.create-scan", scanParams(bbCIDR), c),
			execute("account.rotate-apitoken", map[string]any{"grant_type": "client_credentials", "client_id": bbClientID, "client_secret": "not-a-held-secret-1"}, c),
			execute("account.remove-key", map[string]any{"key_id": bbKey2ID}, c),
		}
	}
	previews := e.mcpPhases(t, writes("true"), writes(1))
	for _, m := range sv.allMethods() {
		if isWrite(m) {
			t.Errorf("confirm \"true\" or 1 sent %s", m)
		}
	}
	replies := mcpReplies(previews)
	unconfirmed := append(writes("true"), writes(1)...)
	for id := 1; id <= 6; id++ {
		if !previewOrWithheld(unconfirmed[id-1], replies[id]) {
			t.Errorf("call %d did not preview:\n%.1000s", id, replies[id])
		}
	}
	out := e.mcpPhases(t, writes(true))
	got := map[string]int{}
	for _, m := range sv.allMethods() {
		if isWrite(m) {
			got[m]++
		}
	}
	for _, m := range []string{"PUT /org/sites/" + bbSiteID + "/scan", "POST /account/api/rotate", "DELETE /account/keys/" + bbKey2ID} {
		if got[m] != 1 {
			t.Errorf("confirm true: want exactly one %s, got %d; requests: %v\n%.1500s", m, got[m], sv.allMethods(), out)
		}
	}
	sv.mu.Lock()
	bodies := append([]string{}, sv.writes["PUT /org/sites/"+bbSiteID+"/scan"]...)
	sv.mu.Unlock()
	if len(bodies) != 1 || !strings.Contains(bodies[0], bbCIDR) {
		t.Errorf("confirm true: the one scan PUT did not carry its targets: %q", bodies)
	}
	if !strings.Contains(out, bbTaskID) {
		t.Errorf("confirm true: the scan's task did not print:\n%.1500s", out)
	}
	assertNoEcho(t, "MCP confirmed writes", out)
	checkOneAuthWay(t, sv)
}

// dry-run-sends-nothing: a --dry-run never reaches the API, from a cold start
// with the key in the environment, the credential store or a config file
// (api_key, or auth_header).
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	for _, mode := range []credMode{credEnv, credStored, credConfigFile, credConfigHeader} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		for _, args := range [][]string{
			{"org", "create-scan", bbSiteID, "--agent-id", bbAgentID, "--targets", bbCIDR, "--dry-run"},
			{"org", "create-scan", bbSiteID, "--agent-id", bbAgentID, "--targets", bbCIDR, "--yes", "--json", "--dry-run"},
			{"account", "remove-key", bbKey2ID, "--yes", "--dry-run"},
			{"account", "rotate-key", bbKeyID, "--yes", "--dry-run"},
			{"org", "get-asset", bbAssetID, "--dry-run"},
			{"account", "get-keys", "--dry-run"},
			{"runzero-export", "assets-json", "--page-size", "2", "--all", "--dry-run"},
			{"sync", "--resources", "org-assets", "--dry-run"},
			{"scan-watch", bbSiteID, "--targets", bbCIDR, "--dry-run"},
		} {
			// Each run reaches its preview, so the count below is not vacuous
			// (a flag error would send nothing either).
			if out := e.run(t, args...); !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") && !strings.Contains(out, "would ") {
				t.Errorf("%v (mode %s) printed no preview:\n%.1500s", args, mode, out)
			}
		}
		if m := sv.allMethods(); len(m) != 0 {
			t.Errorf("--dry-run (mode %s) sent %v", mode, m)
		}
		// Control: the same write and read without --dry-run reach the fake.
		e.run(t, "org", "create-scan", bbSiteID, "--agent-id", bbAgentID, "--targets", bbCIDR, "--yes")
		e.run(t, "org", "get-asset", bbAssetID, "--json")
		if m := sv.allMethods(); len(m) != 2 || m[0] != "PUT /org/sites/"+bbSiteID+"/scan" || m[1] != "GET /org/assets/"+bbAssetID {
			t.Errorf("control (mode %s): want the one PUT and the one GET, got %v", mode, m)
		}
	}
}

// credential-split-across-output-records over MCP: the fake splits the first
// two credentials it was sent across the two pages of the asset export, one
// half each. One session reads both pages (page_size, then the next_key as
// start_key); the halves must never both reach the client.
func TestMCPSplitAcrossPagesWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{mcpSplitHalf()},
		[]map[string]any{read("account.export-events-json", map[string]any{"page_size": 25, "start_key": "page-2"})})
	if !strings.Contains(out, `"id":2`) {
		t.Fatalf("MCP session did not answer both pages:\n%.2000s", out)
	}
	if sv.laterPages("/account/events.json") == 0 {
		t.Fatalf("the second page was never asked for; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
}

// The pagerduty suite's P2 session-wide withholding after a delivered half
// does not stand here, so it runs as a regression check: once one MCP result
// carrying half a held credential has answered (an event log page whose
// event names half the key; the credential kit withholds it whole, each half
// being a held piece of 8 or more characters), later unrelated results of the
// same session (the sites, then a write preview) still print. Only the event
// log repeats what it was sent: the kit withholds any answer that does.
func TestMCPSessionHalfKeepsLaterResults(t *testing.T) {
	sv := echoServer(t)
	sv.setQuiet(func(p string) bool { return p != "/account/events.json" })
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{mcpSplitHalf()},
		[]map[string]any{read("org.get-sites", nil)},
		[]map[string]any{execute("org.create-scan", scanParams(bbCIDR), nil)})
	replies := mcpReplies(out)
	if !strings.Contains(replies[2], "HQ-Record headquarters") {
		t.Errorf("the site list after a delivered half was withheld:\n%.1500s", replies[2])
	}
	if r := replies[3]; !previewed.MatchString(r) {
		t.Errorf("the preview after a delivered half was withheld:\n%.1500s", r)
	}
	assertNoEcho(t, "MCP session", out)
}

// An MCP read of a record that splits a held credential across two adjacent
// values (the fake's "split" member) is withheld whole by the credential kit
// (contract 2(b): each half is a held piece of 8 or more characters), and
// the same reads of records that repeat nothing keep their public fields.
func TestMCPSplitRecordKeepsItsPublicFields(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	reads := []map[string]any{read("org.get-asset", map[string]any{"asset_id": bbAssetID}),
		read("account.get-key", map[string]any{"key_id": bbKeyID})}
	out := e.mcpCalls(t, reads)
	replies := mcpReplies(out)
	for id := 1; id <= len(reads); id++ {
		if !strings.Contains(replies[id], kitNotice("mcp")) {
			t.Errorf("read %d of a split credential was not withheld whole:\n%.2000s", id, out)
		}
	}
	assertNoEcho(t, "MCP split records", out)
	sv.setQuiet(everyPath)
	quiet := e.mcpCalls(t, reads)
	for _, want := range []*regexp.Regexp{publicField("os", bbOS), publicField("site_id", bbSiteID), publicField("last_used_ip", bbLastUsedIP)} {
		if !want.MatchString(quiet) {
			t.Errorf("a read withheld %s:\n%.2000s", want, quiet)
		}
	}
	assertNoEcho(t, "MCP records", quiet)
}

// The work order's ruled P2 credential-split-across-sql-bindings, kept
// reproducible (the fake's splitAssets): a sync writes each record of the
// asset export to the local store as its own row, and the store checks each
// record alone (output streams and file sinks check a write with what they
// already delivered; the SQLite store keeps no such history), so the two
// halves of a held credential that arrive in two records land in two rows of
// the store file. The output withholds both. Runs only with RUNZERO_BB_P2=1;
// fails while the P2 stands.
func TestStoreSplitAcrossRecordsKnownP2(t *testing.T) {
	if os.Getenv("RUNZERO_BB_P2") == "" {
		t.Skip("ruled P2 credential-split-across-sql-bindings; set RUNZERO_BB_P2=1 to reproduce")
	}
	sv := echoServer(t)
	sv.splitAssets = true
	e := newEnv(t, sv, credStored)
	out := e.run(t, "sync", "--resources", "runzero-export", "--json")
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

// The endpoints the spec declares security: [] (/health, /releases/*): the
// client may send them the key (they are on the pinned origin, over TLS:
// the credential's own allowed origin, so not a leak), but only as the one
// Bearer header, and never anywhere else. What it sends is recorded in the
// log for the report.
func TestOpenEndpointsCarryOnlyThePinnedKey(t *testing.T) {
	t.Parallel() // its own fake runZero and home
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.run(t, "health", "--json")
	out += e.run(t, "releases", "get-latest-agent-version", "--json")
	out += e.run(t, "releases", "get-latest-scanner-version")
	out += e.mcpCalls(t, []map[string]any{read("health.get-check", nil), read("releases.get-latest-platform-version", nil)})
	sv.mu.Lock()
	withKey, without, off := sv.openAuth, sv.openNoAuth, sv.offHost
	sv.mu.Unlock()
	if withKey+without < 5 {
		t.Fatalf("only %d request(s) reached /health or /releases; the test did not run:\n%.2000s", withKey+without, out)
	}
	t.Logf("security: [] endpoints: %d request(s) carried Authorization: Bearer <key>, %d carried none", withKey, without)
	if off != 0 {
		t.Errorf("another host was contacted %d time(s)", off)
	}
	assertNoEcho(t, "open endpoints", out)
	checkOneAuthWay(t, sv)
}
