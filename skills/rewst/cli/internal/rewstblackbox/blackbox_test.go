// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package rewstblackbox runs the built CLI and MCP binaries against an
// in-process Rewst fake and checks what a reviewer cannot read off the
// code (rewst handfixes output-chokepoint, store-never-keeps-secrets,
// no-response-cache, file-sinks-withheld, response-credentials-held,
// api-origin-pinned, dry-run-sends-nothing, mcp-endpoint-write-confirm,
// mcp-mirror-write-confirm, rewst-credential-names-exact,
// org-variable-secret-category, graphql-errors-are-failures).
//
// The fake is the Rewst GraphQL gateway on one origin (REWST_BASE_URL
// points the CLI there), authenticated with the one header Authorization:
// Bearer <REWST_API_TOKEN>. Every operation is /graphql. It answers both
// shapes the CLI sends there: a GraphQL POST ({"query": "...", "variables":
// {...}}: the top-level fields the document names, each answered by name,
// {"data": {<field>: ...}}), and the REST-shaped requests the 152 generated
// typed commands send (a known press limitation, recorded for a press
// follow-up: a read is GET /graphql?<arg>=<value> and reads data.<field>, a
// write is POST, PATCH or DELETE /graphql with its flag values as JSON body
// fields). A REST-shaped read is answered with every field the battery's
// typed reads name, so each command finds its own response path; lists are
// bare arrays under data.<field>, paged by limit and offset. A failed
// operation comes back as the vendor's own Apollo shapes: HTTP 200 with an
// AUTH_ERR errors array (not authorized), HTTP 400 with the CSRF-prevention
// BAD_REQUEST errors, or HTTP 401.
//
// The fake echoes every credential the process holds back in every encoding
// it can, whole and split across lines, adjacent values, members and pages:
// what the request carried, and the credentials Rewst documents in its
// answers (spec.graphql: ApiClientWithSecret and ApiClientSecretRotation's
// auth0ClientSecret and auth0ClientId from createOrganizationApiClient and
// rotateApiClientSecret; an ApiClient's auth0ClientId; the string answers of
// getCannyToken, getHaloLiveChatToken and getSkilljarLoginToken; an
// OrgVariable's value when its category is secret), plus generic
// credential-named members a pack config's config JSON carries (an
// integration's access_token, refresh_token, client_secret, encryption_key,
// password, auth_username and a credentials container) that the layer must
// still withhold. The suite checks that:
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the token from REWST_API_TOKEN, from the credential
//     store (auth set-token) or from a config file's api_token, access_token
//     or auth_header, a credential the configuration holds but does not send
//     (a stored token the environment overrides, a config file's
//     access_token beside its api_token, refresh_token, client_id and
//     client_secret), the MCP HTTP token, a credential-named query value
//     given on the command line or as an MCP argument, and the credentials a
//     response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, delivered files, the feedback
//     log, the learn journal, profiles, teach logs);
//   - no credential leaves for a host other than the configured one: a
//     redirect off the API host (an ASCII host or a homoglyph one) is refused
//     before it is followed, and nothing given later (an argument, an MCP
//     parameter, a GraphQL variable) can move the pinned origin;
//   - the MCP server sends a write (rewst_execute, the mirrored import and
//     jobs prune) only when the call sets confirm to the boolean true, and
//     --dry-run sends nothing.
//
// Rewst data under names that read as credentials or carry identities (org
// ids and names, workflow and execution ids, names and counts, a username,
// a user object, the Login page object, CrateToken lists under tokens, a
// tokenId, a zedtoken, secretRotatedAt, authorizedForOrganizations lists, a
// tokenApiClient but its auth0ClientId, an OrgVariable's value when its
// category is general, contact or system) is never withheld.
//
// Every child process runs with HTTPS_PROXY and HTTP_PROXY pointing at a trap
// (loopback hosts, the fakes, are never proxied), so a request that got past
// the pin to any real host would reach the trap instead; TestMain fails the
// run when the trap saw anything. No request ever reaches a Rewst host.
//
// The oracle is the exact set of strings the fake sent, so a pass does not
// depend on the program's own decoder.
package rewstblackbox

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
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
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"rewst-pp-cli/internal/credwire"
)

const (
	bbToken  = "rewstTOKEN7fb3189bQ6u3sZxW4o1n7R8v52" // REWST_API_TOKEN (Authorization: Bearer <token>)
	bbStored = "storedToken-71c3e9b05d2a4f68e1"       // auth set-token (stdin), the credential store
	// bbFileToken: api_token in a 0600 config file named by REWST_CONFIG. Its
	// tail was 5e2a8c0d4b6f19 until 2026-11-05: any text holding
	// e2a8c0d4b6f19 runs the kit's decoder past its depth limit, so the kit
	// withheld every request that carried it (.review/kit-findings.md).
	bbFileToken     = "configFileToken-7c1e9b3d5a2f48"
	bbFileAccess    = "configFileAccess-2b9d7e4a1c63"       // access_token in a 0600 config file (sent as Bearer alone; held unused beside an api_token)
	bbFileHeader    = "configFileHeader-9a4e1c7b3d58"       // auth_header = "Bearer <this>" in a 0600 config file (sent as given)
	bbFileRefresh   = "configFileRefresh-0d6b2f8e4a71"      // refresh_token in a 0600 config file (held unused)
	bbFileSecret    = "configFileSecret-3c8a5e1f7b29"       // client_secret in a 0600 config file (held unused)
	bbFileClient    = "configFileClientId-6e1d9b3a7c"       // client_id in a 0600 config file (held unused)
	bbArgvSec       = "-argvSecret-91c3e7a5"                // a credential-named query value on the command line (sync --param password=...; starts with "-")
	bbMCPArgSec     = "mcpArgSecret-81d3c6f0"               // a credential-named query value an MCP sync call sends (param password=...)
	bbShortResp     = "Sh0rtPw"                             // a 7-character password a pack config's config carries (gzip-encoded answer)
	bbBOMResp       = "BomRespSecret-6a1c"                  // an api_key a secret org variable's pack config names, BOM-prefixed answer
	bbGzipResp      = "GzipRespToken-4f2a9c1e"              // a pack config's integration access_token (gzip-encoded answer)
	bbRefreshResp   = "refreshRespToken-8e2d4b6f0a"         // a pack config's integration refresh_token (gzip)
	bbClientSecResp = "clientSecretResp-5f1b3d7c9e"         // a pack config's integration client_secret (gzip)
	bbEncKey        = "cQ7vZr2k-" + "6d0a2e8c4f71Wm"        // a pack config's encryption_key (a qualified key name)
	bbPairUser      = "sso-q8Wm3Tz1Kd5f"                    // a pack config's auth_username (the identity half of the pair)
	bbCredUser      = "jdoe-zx9Kq2wPm4Rb"                   // a pack config's credentials.username (inside a credential container)
	bbClientSecret  = "auth0Secret-9QwErTy7uIoP3aSdF6gH"    // createOrganizationApiClient (ApiClientWithSecret auth0ClientSecret)
	bbRotated       = "auth0Rotated-2ZxCvBn8mLkJ4hGfD1sA"   // rotateApiClientSecret (ApiClientSecretRotation auth0ClientSecret)
	bbClientID      = "auth0Cid-7Hq2Lk9Xw3Rt5Vb8Nm"         // an ApiClient's auth0ClientId (held by name)
	bbCannyTok      = "cannyTok-5f8a1c3e7b9d2046"           // getCannyToken (a String answer)
	bbHaloTok       = "haloTok-3b7d9f1a5c8e2064"            // getHaloLiveChatToken (a String answer)
	bbSkilljarTok   = "skilljarTok-8e2a4c6f1b3d5079"        // getSkilljarLoginToken (a String answer)
	bbOrgSecret     = "orgVarSecret-" + "4c9e2a7f1d3b"      // an OrgVariable's value of category secret (orgVariables, visibleOrgVariables, orgVariable)
	bbHTTPToken     = "mcpHttpToken-0123456789abcdefABCDEF" // REWST_MCP_HTTP_TOKEN
	// Rewst data the fake answers (public: never withheld).
	bbOrgID         = "6f1c2a9e-3b7d-4e8a-9c0f-1d2e3f4a5b6c" // the MSP's parent organization
	bbOrgName       = "HQ-Record MSP"
	bbSubOrgID      = "7a2d3b0f-4c8e-4f9b-8d1a-2e3f4a5b6c7d" // a managed client organization
	bbSubOrgName    = "HQ-Record Client Co"
	bbSubOrg2ID     = "1b9e8d7c-6f5a-4b3c-8d2e-1f0a9b8c7d6e" // a second managed client, missing a pack
	bbWorkflowID    = "8b3e4c1a-5d9f-4a0c-9e2b-3f4a5b6c7d8e" // a workflow with a recent failed execution
	bbWorkflowName  = "HQ-Record Onboard User"
	bbDormantID     = "9c4f5d2b-6e0a-4b1d-8f3c-4a5b6c7d8e9f" // a workflow with no execution (dormant)
	bbDormantName   = "HQ-Record Legacy Offboard"
	bbExecID        = "ac5a6e3c-7f1b-4c2e-9a4d-5b6c7d8e9fa0" // bbWorkflowID's failed execution
	bbPageID        = "bd6b7f4d-8a2c-4d3f-8b5e-6c7d8e9fa0b1" // the synced page (sync's default resource)
	bbPageName      = "HQ-Record Portal page"
	bbAPIClientID   = "ce7c8a5e-9b3d-" + "4e4a-9c6f-7d8e9fa0b1c2" // an API client
	bbAPIClientName = "HQ-Record API client"
	bbUserID        = "d1e2f3a4-b5c6-4d7e-8f9a-0b1c2d3e4f5a" // the token user
	bbUsername      = "pat.engineer@hq-record.example.com"   // its username (public: Rewst has no password)
	bbPackID        = "e09eac7a-bd5f-4a6c-9e8b-9fa0b1c2d3e4" // an integration pack
	bbPackConfigID  = "f1aebd8b-ce6a-4b7d-8f9c-a0b1c2d3e4f5"
	bbPackConfName  = "HQ-Record M365 pack config"
	bbMissingPack   = "HQ-Record Halo PSA" // installed on bbSubOrgID only
	bbOrgVarID      = "0a1b2c3d-4e5f-4a6b-9c8d-7e6f5a4b3c2d"
	bbGeneralValue  = "HQ-Record general value"        // an OrgVariable of category general
	bbContactValue  = "helpdesk@hq-record.example.com" // an OrgVariable of category contact
	bbSystemValue   = "HQ-Record system value"         // an OrgVariable of category system
	bbTemplateID    = "2c3d4e5f-6a7b-4c8d-9e0f-1a2b3c4d5e6f"
	bbFormID        = "3d4e5f6a-7b8c-4d9e-8f0a-2b3c4d5e6f7a"
	bbTokenID       = "4e5f" + "6a7b-8c9d-4e0f-9a1b-3c4d5e6f7a8b" // a CrateToken (tokens lists, tokenId)
	bbTokenName     = "ticket_board_id"                           // that CrateToken's name
	bbZedtoken      = "GhUKEzE3NjE3" + "NjU0MzIxMDAwMDAwMDA"      // a SpiceDB consistency token
	bbRotatedAt     = "2026-10-01T12:00:00.000Z"                  // secretRotatedAt
	bbLoginDomain   = "hq-record.rewst.example.com"               // the Login page's domain
	// A CrateToken object answered under the generic name token: Rewst data,
	// never held by name (TestRewstPublicFieldsAreNotWithheld).
	bbTokenObjName = "Zq7CrateTok" + "Obj4Wv"
	// Request values the fake answers specially.
	bbDenied   = "40199" // answered HTTP 200 + AUTH_ERR errors (Rewst's not-authorized shape)
	bbUnauth   = "40100" // answered HTTP 401
	bbCSRF     = "40000" // answered HTTP 400, Apollo's CSRF-prevention BAD_REQUEST
	bbRedirect = "9911"  // redirected to another host (its Location carries the credential)
	bbRedirIDN = "9922"  // redirected to a homoglyph of api.rewst.io (Cyrillic s)
)

// secrets are every credential the suite gives the process, in the order the
// fake splits them (the first one a request carried is the one split across
// pages). Two fixture values are not here: a pack config's usernames
// (bbPairUser, bbCredUser) are identities, not secrets, and the kit holds
// response fields only by declared name; an org variable's secret value
// (bbOrgSecret) is the P2 TestOrgVariableSecretValueHeldKnownP2.
func secrets() []string {
	return []string{bbToken, bbStored, bbFileToken, bbFileAccess, bbFileHeader, bbFileRefresh, bbFileSecret, bbFileClient, bbArgvSec,
		bbMCPArgSec, bbShortResp, bbBOMResp, bbGzipResp, bbRefreshResp, bbClientSecResp, bbEncKey, bbClientSecret,
		bbRotated, bbClientID, bbCannyTok, bbHaloTok, bbSkilljarTok, bbHTTPToken}
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

// homoglyphConnect is the one line the trap may record: the fake redirects
// bbRedirIDN to a homoglyph of api.rewst.io, and the hop is followed without
// the credential (the client drops Authorization on an origin change and the
// credential kit judges the hop strictly: leak contract 2(c)). The trap
// reads the request inside that tunnel (tunnel_test.go), and TestMain fails
// the run if one carried a held value.
const homoglyphConnect = "CONNECT " + homoglyphHost + ":443"

// trapEnv routes every non-loopback request of a child through the trap.
func trapEnv() []string {
	return []string{"HTTPS_PROXY=" + trap.url, "HTTP_PROXY=" + trap.url, "ALL_PROXY=" + trap.url, "NO_PROXY=", "no_proxy="}
}

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	ts := httptest.NewServer(http.HandlerFunc(trapHandler))
	trap.url = ts.URL
	dir, err := os.MkdirTemp("", "rewst-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "rewst-cli"+exe)
	bbMCP = filepath.Join(dir, "rewst-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/rewst-cli", bbMCP: "../../cmd/rewst-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	ts.Close()
	var left []string
	for _, d := range trapDials() {
		if d != homoglyphConnect {
			left = append(left, d)
		}
	}
	if len(left) != 0 {
		fmt.Fprintf(os.Stderr, "FAIL: %d request(s) left for a non-loopback host (caught by the proxy trap): %v\n", len(left), left)
		code = 1
	}
	if l := tunnelLeaks(); len(l) != 0 {
		fmt.Fprintf(os.Stderr, "FAIL: a request through the trap's tunnel to %s carried a held value: %v\n", homoglyphHost, l)
		code = 1
	}
	fmt.Fprintf(os.Stderr, "rewstblackbox: %d CLI runs, %d MCP tools/call requests\n", cliRuns.n, mcpCallsSent.n)
	os.Exit(code)
}

// echoForms is every encoding of s the server sends back whole: as written,
// inside other text, as the header line, base64 (standard and URL-safe), hex,
// base32, ASCII85, percent-encoded, PEM-armored, gzip-compressed then
// base64-encoded (the way a JSON string carries compressed bytes), and a
// Zstandard frame the program cannot inflate, base64-encoded (the class
// secret-in-unreadable-compressed-stream).
func echoForms(s string) []string {
	if f, ok := echoFormCache.Load(s); ok {
		return f.([]string)
	}
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	forms := []string{
		s,
		"x" + s + "x",
		"Authorization: Bearer " + s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
		string(pem.EncodeToMemory(&pem.Block{Type: "REWST API TOKEN", Bytes: []byte(s)})),
		gzipBase64(s),
		base64.StdEncoding.EncodeToString(zstdRLEFrame(s)),
	}
	echoFormCache.Store(s, forms)
	return forms
}

// echoFormCache: every check and every request the fake answers asks for every
// secret's forms, and each gzip writer allocates about a megabyte.
var echoFormCache sync.Map

// gzipBase64 is s gzip-compressed (no header name or time, so the bytes are
// the same every run) and base64-encoded.
func gzipBase64(s string) string {
	var z bytes.Buffer
	zw := gzip.NewWriter(&z)
	_, _ = zw.Write([]byte(s))
	_ = zw.Close()
	return base64.StdEncoding.EncodeToString(z.Bytes())
}

// zstdRLEFrame is s (at most 255 bytes) as one Zstandard frame (RFC 8878):
// a single-segment frame header with its content size, then one RLE block
// per byte of s (block type 1, size 1, the byte), the last one flagged. A
// Zstandard decoder reads s back, while no run of the frame's bytes spells it
// (every byte of s sits behind its own three-byte block header), and the
// standard library has no reader for it: the shape of the Codex input
// KLUv/SAgAwEAWg== (pandadoc 4.34.2, an RLE frame of 32 Z's).
func zstdRLEFrame(s string) []byte {
	b := []byte{0x28, 0xb5, 0x2f, 0xfd, 0x20, byte(len(s))}
	for i := 0; i < len(s); i++ {
		hdr := uint32(1<<1 | 1<<3) // RLE, block size 1
		if i == len(s)-1 {
			hdr |= 1 // last block
		}
		b = append(b, byte(hdr), byte(hdr>>8), byte(hdr>>16), s[i])
	}
	return b
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
	for _, s := range secrets() {
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

// js is v as JSON, HTML characters as written.
func js(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}

// raw is a JSON document carried as is inside a fixture.
func raw(doc string) json.RawMessage { return json.RawMessage(doc) }

type server struct {
	url     string // REWST_BASE_URL: the fake's origin
	caFile  string
	mu      sync.Mutex
	offHost int
	// methods are the requests the fake received: "METHOD /path" and, on
	// /graphql, what the request was: "query:<fields>" or "mutation:<fields>"
	// for a GraphQL document, "rest" for a REST-shaped request (a typed
	// command's GET with query arguments, or POST, PATCH or DELETE with its
	// flag values as body fields).
	methods []string
	keys    map[string]int // tokens the API received as the one header Authorization: Bearer <token>
	// otherAuth counts requests whose credential came any other way: a second
	// Authorization header, one that is not "Bearer <token>" (another scheme,
	// a doubled prefix), an X-Api-Key header, or a token in the query (Rewst
	// takes only the Authorization header).
	otherAuth int
	// pages counts each page of the split page list the fake served, by
	// "offset=N" (sync's walk of the default resource, and the paged reads).
	pages map[string]int
	// redirectPages sends the paged page list (sync's walk) to the other
	// host too, so sync meets a redirect.
	redirectPages bool
	// denials counts the requests the fake answered as failed (AUTH_ERR,
	// 401, CSRF).
	denials int
	// watch, when set, counts the requests whose text carries it.
	watch     string
	watchHits int
	// The credential kit withholds a whole unit (an MCP result, a stdout
	// write, a store record) that carries a held value or a piece of 8 or
	// more characters of one, so a check of what else a unit carries runs
	// against answers that carry none. quiet, when set, echoes nothing a
	// request carried; whole splits nothing (the paged page list and the API
	// client by id split the credential they were sent); bare answers no
	// member the connector declares a response credential, and no echo
	// member, which repeats the credentials a fixture answers
	// (stripDeclared).
	quiet, whole, bare bool
}

// stripDeclared is v without any member named in the connector's declared
// response credentials, and without an echo member, at any depth.
func stripDeclared(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if k != "echo" && !slices.Contains(credwire.Declarations.ResponseFields, k) {
				out[k] = stripDeclared(e)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = stripDeclared(e)
		}
		return out
	}
	return v
}

func newServer() *server {
	return &server{keys: map[string]int{}, pages: map[string]int{}}
}

// tlsServer starts an HTTP/2 TLS fake.
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{suiteCert()}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// caFileFor writes srv's certificate as a PEM CA bundle (REWST_CA_CERT).
func caFileFor(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return ca
}

// isWrite reports a request that changes data: a GraphQL mutation, or a
// REST-shaped POST, PATCH, PUT or DELETE (a GraphQL query POSTed, a GET, a
// HEAD, does not).
func isWrite(m string) bool {
	method, _, _ := strings.Cut(m, " ")
	switch {
	case strings.Contains(m, " mutation:"):
		return true
	case method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions:
		return false
	case strings.Contains(m, " query:"):
		return false
	}
	return true
}

// gqlField is one top-level field of a GraphQL document: the response key
// (its alias, or the name) and the field name.
type gqlField struct{ key, name string }

// gqlTokens splits a GraphQL document into punctuators, names, numbers and
// strings, dropping white space, commas and comments.
func gqlTokens(doc string) []string {
	var toks []string
	for i := 0; i < len(doc); {
		c := doc[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',' || c == 0xef || c == 0xbb || c == 0xbf:
			i++
		case c == '#':
			for i < len(doc) && doc[i] != '\n' {
				i++
			}
		case strings.HasPrefix(doc[i:], `"""`):
			end := strings.Index(doc[i+3:], `"""`)
			if end < 0 {
				end = len(doc) - i - 3
			}
			toks = append(toks, doc[i:min(len(doc), i+3+end+3)])
			i += 3 + end + 3
		case c == '"':
			j := i + 1
			for j < len(doc) && doc[j] != '"' {
				if doc[j] == '\\' {
					j++
				}
				j++
			}
			toks = append(toks, doc[i:min(len(doc), j+1)])
			i = j + 1
		case strings.HasPrefix(doc[i:], "..."):
			toks = append(toks, "...")
			i += 3
		case c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c == '-':
			j := i
			for j < len(doc) && (doc[j] == '_' || doc[j] == '.' || doc[j] == '-' || ('a' <= doc[j] && doc[j] <= 'z') || ('A' <= doc[j] && doc[j] <= 'Z') || ('0' <= doc[j] && doc[j] <= '9')) {
				j++
			}
			toks = append(toks, doc[i:j])
			i = j
		default:
			toks = append(toks, string(c))
			i++
		}
	}
	return toks
}

func isGQLName(tok string) bool {
	return tok != "" && (tok[0] == '_' || ('a' <= tok[0] && tok[0] <= 'z') || ('A' <= tok[0] && tok[0] <= 'Z'))
}

// gqlOperation reads a GraphQL document's operation type (query, mutation
// or subscription; a shorthand { ... } is a query) and its top-level fields.
func gqlOperation(doc string) (op string, fields []gqlField) {
	toks := gqlTokens(doc)
	op = "query"
	i := 0
	for paren := 0; i < len(toks); i++ {
		switch t := toks[i]; {
		case t == "(":
			paren++
		case t == ")":
			paren--
		case t == "{" && paren == 0:
			goto body
		case paren == 0 && (t == "query" || t == "mutation" || t == "subscription") && len(fields) == 0 && i == 0:
			op = t
		}
	}
	return op, nil
body:
	depth, paren := 0, 0
	for ; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t == "{":
			depth++
		case t == "}":
			depth--
			if depth == 0 {
				return op, fields
			}
		case t == "(":
			paren++
		case t == ")":
			paren--
		case depth == 1 && paren == 0 && (t == "@" || t == "..."):
			i++ // a directive's or a fragment spread's name
			if i+1 < len(toks) && toks[i] == "on" {
				i++
			}
		case depth == 1 && paren == 0 && isGQLName(t):
			f := gqlField{key: t, name: t}
			if i+2 < len(toks) && toks[i+1] == ":" && isGQLName(toks[i+2]) {
				f.name = toks[i+2]
				i += 2
			}
			fields = append(fields, f)
		}
	}
	return op, fields
}

// gqlRequest is what one request to /graphql asked: a GraphQL document (its
// operation, fields and variables), or a REST-shaped request (its body
// members, decoded).
type gqlRequest struct {
	doc    string
	op     string
	fields []gqlField
	vars   map[string]any
	body   map[string]any // the decoded JSON body (any request)
}

func (g gqlRequest) isDoc() bool { return g.doc != "" }

// readGQL reads a request body sent to /graphql.
func readGQL(body []byte) gqlRequest {
	var g gqlRequest
	_ = json.Unmarshal(body, &g.body)
	if q, ok := g.body["query"].(string); ok && strings.TrimSpace(q) != "" {
		g.doc = q
		g.op, g.fields = gqlOperation(q)
		g.vars, _ = g.body["variables"].(map[string]any)
	}
	return g
}

// label is the fake's record of a /graphql request (server.methods).
func (g gqlRequest) label() string {
	if !g.isDoc() {
		return "rest"
	}
	names := make([]string, 0, len(g.fields))
	for _, f := range g.fields {
		names = append(names, f.name)
	}
	return g.op + ":" + strings.Join(names, ",")
}

// stringLeaves is every string value inside v.
func stringLeaves(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		*out = append(*out, x)
	case map[string]any:
		for _, y := range x {
			stringLeaves(y, out)
		}
	case []any:
		for _, y := range x {
			stringLeaves(y, out)
		}
	}
}

// special is the first of the values the fake answers specially that the
// request carries: a query value, a body or variable string (whole), or a
// string literal in the GraphQL document.
func special(q url.Values, g gqlRequest) string {
	var leaves []string
	for _, vs := range q {
		leaves = append(leaves, vs...)
	}
	stringLeaves(g.body, &leaves)
	for _, v := range []string{bbDenied, bbUnauth, bbCSRF, bbRedirect, bbRedirIDN} {
		for _, s := range leaves {
			if s == v {
				return v
			}
		}
		if strings.Contains(g.doc, `"`+v+`"`) {
			return v
		}
	}
	return ""
}

// orgOf is the organization a GraphQL request names: its orgId variable, or
// an org id written in the document.
func orgOf(g gqlRequest) string {
	if s, ok := g.vars["orgId"].(string); ok {
		return s
	}
	for _, id := range []string{bbOrgID, bbSubOrgID, bbSubOrg2ID} {
		if strings.Contains(g.doc, id) {
			return id
		}
	}
	return ""
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
	scheme, tok, _ := strings.Cut(h, " ")
	if len(auth) == 1 && scheme == "Bearer" && tok != "" && !strings.ContainsAny(tok, " \t") {
		sv.keys[tok]++
	} else if len(auth) > 0 {
		sv.otherAuth++
	}
	if len(r.Header.Values("X-Api-Key")) > 0 {
		sv.otherAuth++
	}
	q := r.URL.Query()
	for _, k := range []string{"api_key", "apikey", "x-api-key", "token", "access_token", "api_token"} {
		if q.Has(k) {
			sv.otherAuth++
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

// pagesSplit reports a REST-shaped read of the page list that pages (a
// limit, no selector): sync's walk of its default resource, and the paged
// reads.
func pagesSplit(q url.Values) bool {
	return q.Has("limit") && !q.Has("where") && !q.Has("orgId") && !q.Has("search") && !q.Has("visibleForOrgId") && !q.Has("id")
}

// apolloAuthError is Rewst's own not-authorized answer (HTTP 200, observed
// from the vendor's Apollo server), repeating what it was sent.
func apolloAuthError(field string, sent []string, echo string) string {
	joined := strings.Join(sent, " ")
	return js(map[string]any{"data": nil, "errors": []any{map[string]any{
		"message":   "Not Authorized - User not found - " + field + " " + joined,
		"locations": []any{map[string]any{"line": 1, "column": 3}},
		"path":      []any{field},
		"extensions": map[string]any{"code": "AUTH_ERR", "stacktrace": []any{
			"Error: Not Authorized - User not found - " + field, "    at authorize (/app/dist/auth.js:41:11) " + joined}, "echo": raw(echo)},
	}}})
}

// apolloCSRFError is Apollo's CSRF-prevention answer to a GET /graphql
// without a content type (HTTP 400, observed from the vendor's server).
func apolloCSRFError(sent []string, echo string) string {
	return js(map[string]any{"errors": []any{map[string]any{
		"message": "This operation has been blocked as a potential Cross-Site Request Forgery (CSRF). Please either specify a " +
			"'content-type' header (with a type that is not one of application/x-www-form-urlencoded, multipart/form-data, " +
			"text/plain) or provide a non-empty value for one of the following headers: x-apollo-operation-name, apollo-require-preflight " +
			strings.Join(sent, " "),
		"extensions": map[string]any{"code": "BAD_REQUEST", "stacktrace": []any{"BadRequestError: " + strings.Join(sent, " ")}, "echo": raw(echo)},
	}}})
}

// echoServer starts the Rewst fake (TLS).
func echoServer(t *testing.T) *server {
	t.Helper()
	sv := newServer()
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	now := time.Now().UTC()
	ago := func(d time.Duration) string { return now.Add(-d).Format("2006-01-02T15:04:05.000Z") }
	day := 24 * time.Hour
	org := func(id, name, parent string) map[string]any {
		var p any
		if parent != "" {
			p = parent
		}
		return map[string]any{"id": id, "name": name, "domain": "hq-record.example.com", "isEnabled": true, "parentOrgId": p,
			"managingOrgId": p, "createdAt": ago(400 * day), "updatedAt": ago(10 * day)}
	}
	withEcho := func(m map[string]any, echo string) map[string]any {
		m["echo"] = raw(echo)
		return m
	}
	orgVar := func(id, name, category, value string) map[string]any {
		return map[string]any{"id": id, "name": name, "category": category, "value": value, "cascade": false, "orgId": bbOrgID,
			"packConfigId": nil, "createdAt": ago(90 * day), "updatedAt": ago(9 * day), "organization": map[string]any{"id": bbOrgID, "name": bbOrgName}}
	}
	// orgVars are an organization's variables, one of each category: the
	// secret one's value is a credential (pair label category), the others
	// are Rewst data. maskSecrets masks the secret value, as Rewst does.
	orgVars := func(sent []string, masked bool) []any {
		secret := bbOrgSecret
		if masked {
			secret = "********"
		}
		return []any{
			withEcho(orgVar(bbOrgVarID, "halo_tenant_value", "secret", secret), echoObject(union(sent, bbOrgSecret))),
			orgVar("1b2c3d4e-5f6a-4b7c-8d9e-0f1a2b3c4d5e", "ticket_board_url", "general", bbGeneralValue),
			orgVar("2c3d4e5f-6a7b-4c8d-9e0f-1a2b3c4d5e6f", "helpdesk_email", "contact", bbContactValue),
			orgVar("3d4e5f6a-7b8c-4d9e-8f0a-1b2c3d4e5f6a", "rewst_region", "system", bbSystemValue),
		}
	}
	apiClient := func(extra map[string]any) map[string]any {
		m := map[string]any{"id": bbAPIClientID, "name": bbAPIClientName, "description": "HQ-Record automation client", "clientType": "SERVICE",
			"orgId": bbOrgID, "organization": map[string]any{"id": bbOrgID, "name": bbOrgName}, "auth0ClientId": bbClientID,
			"impersonationUserId": bbUserID, "impersonationUser": map[string]any{"id": bbUserID, "username": bbUsername},
			"createdAt": ago(120 * day), "updatedAt": ago(30 * day), "secretRotatedAt": bbRotatedAt}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	page := func(id, name string) map[string]any {
		return map[string]any{"id": id, "name": name, "path": "/hq", "orgId": bbOrgID, "siteId": "5f6a7b8c-9d0e-4f1a-8b2c-3d4e5f6a7b8c",
			"type": "page", "isCanvas": false, "createdAt": ago(60 * day), "updatedAt": ago(2 * day)}
	}
	// pagesFor is the page list a REST-shaped read answers. A paged read
	// (pagesSplit) meets the first credential it was sent split across two
	// pages (limit, default 100, full pages until the last; offset): the
	// first page of page 1 carries one half as its name, the record after
	// the last full page the other, so every pager that walks the pages
	// meets both halves as records of their own.
	pagesFor := func(q url.Values, sent []string, echo string) []any {
		main := withEcho(page(bbPageID, bbPageName), echo)
		if !pagesSplit(q) {
			return []any{main}
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 100
		}
		offset, _ := strconv.Atoi(q.Get("offset"))
		a, b := halves(firstSent(sent))
		items := []any{page("a0000000-0000-4000-8000-000000000001", a), main}
		for i := len(items); i < limit; i++ {
			items = append(items, page(fmt.Sprintf("f%07d-0000-4000-8000-000000000000", i), fmt.Sprintf("HQ-Record filler page %d", i)))
		}
		items = append(items, page("b0000000-0000-4000-8000-000000000002", b))
		start := min(max(offset, 0), len(items))
		return items[start:min(start+limit, len(items))]
	}
	// packConfig is an integration pack's configuration: its config JSON
	// holds the integration's own credentials under generic names.
	packConfig := func(sent []string) map[string]any {
		return map[string]any{"id": bbPackConfigID, "name": bbPackConfName, "packId": bbPackID, "orgId": bbOrgID, "default": true,
			"description": "HQ-Record Microsoft 365 integration", "metadata": map[string]any{"region": "us"},
			"config": map[string]any{"tenant_domain": "hq-record.onmicrosoft.com", "access_token": bbGzipResp, "refresh_token": bbRefreshResp,
				"client_secret": bbClientSecResp, "encryption_key": bbEncKey, "auth_username": bbPairUser, "password": bbShortResp,
				"credentials": map[string]any{"username": bbCredUser}},
			"echo": raw(echoObject(union(sent, bbGzipResp, bbRefreshResp, bbClientSecResp, bbEncKey, bbPairUser)))}
	}
	executions := func(sent []string, echo string) []any {
		return []any{
			withEcho(map[string]any{"id": bbExecID, "status": "FAILED", "createdAt": ago(time.Hour), "numSuccessfulTasks": 3,
				"numAwaitingResponseTasks": 0, "orgId": bbOrgID, "workflow": map[string]any{"id": bbWorkflowID, "name": bbWorkflowName}}, echo),
			map[string]any{"id": "bc6b7f4d-0000-4d3f-8b5e-000000000002", "status": "SUCCEEDED", "createdAt": ago(2 * time.Hour),
				"numSuccessfulTasks": 7, "numAwaitingResponseTasks": 0, "orgId": bbOrgID, "workflow": map[string]any{"id": bbWorkflowID, "name": bbWorkflowName}},
			// An execution of a workflow whose name repeats the credential
			// the request carried (an echoed identifier).
			map[string]any{"id": "bc6b7f4d-0000-4d3f-8b5e-000000000003", "status": "FAILED", "createdAt": ago(3 * time.Hour),
				"numSuccessfulTasks": 0, "numAwaitingResponseTasks": 0, "orgId": bbOrgID,
				"workflow": map[string]any{"id": "e0e0e0e0-0000-4000-8000-000000000003", "name": "Echo " + firstSent(sent)}},
		}
	}
	user := func(echo string) map[string]any {
		return withEcho(map[string]any{"id": bbUserID, "username": bbUsername, "orgId": bbOrgID, "sub": "auth0|hq-record-pat", "isApiUser": false,
			"isTokenUser": true, "isSuperuser": false, "roleIds": []any{"admin"}, "organization": map[string]any{"id": bbOrgID, "name": bbOrgName},
			"managedOrgs":    []any{map[string]any{"id": bbSubOrgID, "name": bbSubOrgName}},
			"tokenApiClient": map[string]any{"id": bbAPIClientID, "name": bbAPIClientName, "auth0ClientId": bbClientID}}, echo)
	}
	// rest is the data a REST-shaped read answers: every field the battery's
	// typed reads name, each command reading its own response path.
	rest := func(q url.Values, sent, split []string, echo string) map[string]any {
		client := apiClient(nil)
		if q.Has("id") {
			// One API client by id: the echo split across values, lines and
			// spaces.
			client = apiClient(map[string]any{"split": raw(splitObject(split))})
		}
		return map[string]any{
			"pages":                     pagesFor(q, split, echo),
			"myAccessibleOrganizations": []any{withEcho(org(bbOrgID, bbOrgName, ""), echo), org(bbSubOrgID, bbSubOrgName, bbOrgID)},
			"visibleOrgVariables":       orgVars(sent, false),
			"apiClient":                 client,
			"organizationApiClients":    map[string]any{"apiClients": []any{apiClient(nil)}, "totalCount": 1, "hasMore": false},
			"packConfigsForOrg":         []any{packConfig(sent)},
			"workflowExecutions":        executions(sent, echo),
			"login":                     map[string]any{"domain": bbLoginDomain, "customDomain": nil, "useCustomDomain": false, "layout": "centered", "theme": "dark"},
			"getTestUserSession":        user(echo),
			"formPermissionState": map[string]any{"formId": bbFormID, "zedtoken": bbZedtoken, "userTotalCount": 1, "roles": []any{}, "triggers": []any{},
				"users": []any{map[string]any{"id": bbUserID, "label": bbUsername, "type": "USER", "grants": []any{}}}},
			"cratesForTemplate": []any{map[string]any{"id": "6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d", "name": "HQ-Record Ticket crate", "tokenId": bbTokenID,
				"tokens": []any{map[string]any{"id": bbTokenID, "name": bbTokenName, "index": 0, "isMultiselect": false}},
				// A CrateToken object under the generic name token.
				"token": map[string]any{"id": "c7e2a9f4-3b1d-4e6a-8f5c-2d9b7e4a1c63", "name": bbTokenObjName}}},
			"permissionAuditLog": map[string]any{"totalCount": 1, "hasMore": false, "entries": []any{withEcho(map[string]any{
				"id": "7b8c9d0e-1f2a-4b3c-8d4e-5f6a7b8c9d0e", "createdAt": ago(day), "orgId": bbOrgID, "orgName": "HQ-Record audit org",
				"actorUserId": bbUserID, "actorUserEmail": bbUsername, "itemType": "role", "action": "update", "actionDetails": map[string]any{"role": "admin"}}, echo)}},
			"permissions": []any{map[string]any{"id": "8c9d0e1f-2a3b-4c4d-9e5f-6a7b8c9d0e1f", "orgId": bbOrgID, "relation": "viewer", "objectType": "workflow",
				"authorizedForOrganizations": []any{map[string]any{"id": bbSubOrgID, "name": bbSubOrgName}}}},
			"conversationMessageVotes": []any{map[string]any{"id": "9d0e1f2a-3b4c-4d5e-8f6a-7b8c9d0e1f2a", "vote": "UP", "reason": "helpful",
				"userId": bbUserID, "user": map[string]any{"id": bbUserID, "username": bbUsername, "orgId": bbOrgID}}},
		}
	}
	// field answers one top-level field of a GraphQL document by name.
	field := func(name string, g gqlRequest, sent []string, echo string) any {
		orgID := orgOf(g)
		masked := regexp.MustCompile(`maskSecrets\s*:\s*true`).MatchString(g.doc)
		switch name {
		case "createOrganizationApiClient":
			return apiClient(map[string]any{"auth0ClientSecret": bbClientSecret, "echo": raw(echoObject(union(sent, bbClientSecret, bbClientID)))})
		case "rotateApiClientSecret":
			return map[string]any{"id": bbAPIClientID, "auth0ClientId": bbClientID, "auth0ClientSecret": bbRotated, "secretRotatedAt": bbRotatedAt,
				"echo": raw(echoObject(union(sent, bbRotated, bbClientID)))}
		case "getCannyToken":
			return bbCannyTok
		case "getHaloLiveChatToken":
			return bbHaloTok
		case "getSkilljarLoginToken":
			return bbSkilljarTok
		case "orgVariable":
			return orgVars(sent, masked)[0]
		case "orgVariables", "visibleOrgVariables":
			vars := orgVars(sent, masked)
			// The variable only one organization has (drift reads the
			// names), and on the parent one named after the credential the
			// request carried.
			if orgID == bbSubOrgID {
				return append(vars, orgVar("4e5f6a7b-0000-4d9e-8f0a-000000000001", "only_in_client_var", "general", "HQ-Record client only"))
			}
			return append(vars, orgVar("4e5f6a7b-0000-4d9e-8f0a-000000000002", "only_in_parent_var", "general", "HQ-Record parent only"),
				orgVar("4e5f6a7b-0000-4d9e-8f0a-000000000003", "Echo "+firstSent(sent), "general", "echoVal-r4Tq9Lz2"))
		case "packsForOrg":
			packs := []any{map[string]any{"name": "Microsoft 365", "ref": "microsoft_graph"}}
			if orgID != bbSubOrgID {
				packs = append(packs, map[string]any{"name": bbMissingPack, "ref": "halo_psa"}, map[string]any{"name": "Echo " + firstSent(sent), "ref": "echo"})
			}
			return packs
		case "workflowExecutionStats":
			return map[string]any{"succeeded": 38, "failed": 2, "running": 1, "pending": 0, "delayed": 0, "paused": 0, "humanSecondsSaved": 36000, "echo": raw(echo)}
		case "timeSavedGroupByWorkflow":
			return []any{
				map[string]any{"workflowId": bbWorkflowID, "workflowName": bbWorkflowName, "secondsSaved": 36000, "totalExecutions": 40, "successfulExecutions": 38, "failedExecutions": 2},
				map[string]any{"workflowId": "e0e0e0e0-0000-4000-8000-000000000004", "workflowName": "Echo " + firstSent(sent), "secondsSaved": 60, "totalExecutions": 1, "successfulExecutions": 1},
			}
		case "workflowExecutions":
			return executions(sent, echo)
		case "workflows":
			return []any{
				withEcho(map[string]any{"id": bbWorkflowID, "name": bbWorkflowName, "updatedAt": ago(5 * day), "humanSecondsSaved": 900}, echo),
				map[string]any{"id": bbDormantID, "name": bbDormantName, "updatedAt": ago(200 * day), "humanSecondsSaved": 300},
				map[string]any{"id": "e0e0e0e0-0000-4000-8000-000000000005", "name": "Echo " + firstSent(sent), "updatedAt": ago(200 * day), "humanSecondsSaved": 1},
			}
		case "managedAndSubOrganizations":
			return []any{
				map[string]any{"id": bbSubOrgID, "name": bbSubOrgName, "installedPacks": []any{map[string]any{"name": "Microsoft 365", "ref": "microsoft_graph"},
					map[string]any{"name": bbMissingPack, "ref": "halo_psa"}}},
				map[string]any{"id": bbSubOrg2ID, "name": "HQ-Record Second Client", "installedPacks": []any{map[string]any{"name": "Microsoft 365", "ref": "microsoft_graph"}}},
				map[string]any{"id": "e0e0e0e0-0000-4000-8000-000000000006", "name": "Echo " + firstSent(sent), "installedPacks": []any{}},
			}
		case "userOrganization":
			return withEcho(org(bbOrgID, bbOrgName, ""), echo)
		}
		if v, ok := rest(url.Values{}, sent, nil, echo)[name]; ok {
			return v
		}
		return map[string]any{"id": "hq-" + name, "echo": raw(echo)}
	}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent := sentSecrets(r)
		split := sent // what the split fixtures cut in two
		sv.mu.Lock()
		if sv.quiet {
			sent = nil
		}
		if sv.whole {
			split = nil
		}
		bare := sv.bare
		sv.mu.Unlock()
		answer := func(data any) string {
			if bare {
				data = stripDeclared(data)
			}
			return js(map[string]any{"data": data})
		}
		echo := echoObject(sent)
		text := requestText(r)
		body, _ := io.ReadAll(r.Body)
		g := readGQL(body)
		q := r.URL.Query()
		sv.mu.Lock()
		m := r.Method + " " + r.URL.Path
		if r.URL.Path == "/graphql" {
			m += " " + g.label()
		}
		sv.methods = append(sv.methods, m)
		sv.countAuth(r)
		get := r.Method == http.MethodGet
		if r.URL.Path == "/graphql" && get && pagesSplit(q) {
			sv.pages["offset="+q.Get("offset")]++
		}
		if sv.watch != "" && strings.Contains(text, sv.watch) {
			sv.watchHits++
		}
		redirectPages := sv.redirectPages
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		sp := special(q, g)
		first := "userOrganization"
		if len(g.fields) > 0 {
			first = g.fields[0].name
		}
		deny := func(code int, doc string) {
			sv.mu.Lock()
			sv.denials++
			sv.mu.Unlock()
			w.WriteHeader(code)
			fmt.Fprint(w, doc)
		}
		switch {
		case r.URL.Path == "/" && (get || r.Method == http.MethodHead):
			// doctor's reachability check of the origin.
			fmt.Fprint(w, `{"status":"ok"}`)
		case r.URL.Path != "/graphql":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"errors":[{"message":"Not Found"}]}`)
		case sp == bbDenied:
			deny(http.StatusOK, apolloAuthError(first, sent, echo))
		case sp == bbUnauth:
			deny(http.StatusUnauthorized, js(map[string]any{"errors": []any{map[string]any{"message": "invalid token " + strings.Join(sent, " "),
				"extensions": map[string]any{"code": "UNAUTHENTICATED", "echo": raw(echo)}}}}))
		case sp == bbCSRF:
			deny(http.StatusBadRequest, apolloCSRFError(sent, echo))
		case sp == bbRedirect || (redirectPages && get && pagesSplit(q)):
			// Off-host, its Location carrying the credential it was sent.
			http.Redirect(w, r, other.URL+r.URL.Path+"?"+r.URL.RawQuery+"&hq_token="+url.QueryEscape(firstSent(sent)), http.StatusTemporaryRedirect)
		case sp == bbRedirIDN:
			// Set as written (http.Redirect would percent-escape the host): a
			// Cyrillic s (U+0455) in rewst.
			w.Header().Set("Location", "https://api.rew\u0455t.io"+r.URL.RequestURI())
			w.WriteHeader(http.StatusTemporaryRedirect)
		case g.isDoc():
			data := map[string]any{}
			for _, f := range g.fields {
				data[f.key] = field(f.name, g, sent, echo)
			}
			fmt.Fprint(w, answer(data))
		case get:
			doc := answer(rest(q, sent, split, echo))
			switch {
			case q.Has("orgId"):
				gzipped(w, doc)
			case q.Has("visibleForOrgId"):
				fmt.Fprint(w, "\xef\xbb\xbf"+doc)
			default:
				fmt.Fprint(w, doc)
			}
		case r.Method == http.MethodDelete:
			fmt.Fprint(w, js(map[string]any{"data": map[string]any{"result": map[string]any{"deleted": true, "id": bbPackID, "echo": raw(echo)}}}))
		default:
			// A REST-shaped write: the record it would make, its body members
			// echoed back, under every mutation name the battery's writes
			// read.
			rec := map[string]any{}
			for k, v := range g.body {
				rec[k] = v
			}
			rec["id"], rec["echo"] = "5a6b7c8d-0000-4e0f-9a1b-00000000beef", raw(echo)
			if _, ok := rec["name"]; !ok {
				rec["name"] = "HQ-Record written record"
			}
			data := map[string]any{}
			for _, k := range []string{"createOrgVariable", "createTag", "createUserInvite", "updateWorkflow", "createWorkflow", "createPage"} {
				data[k] = rec
			}
			data["updateOrgVariables"], data["updateTags"] = []any{rec}, []any{rec}
			fmt.Fprint(w, js(map[string]any{"data": data}))
		}
	}))
	sv.url = srv.URL
	sv.caFile = caFileFor(t, srv)
	return sv
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

// credMode is where the process gets its token.
type credMode int

const (
	credStored        credMode = iota // none in the environment: the battery runs on a token stored with auth set-token
	credEnv                           // REWST_API_TOKEN
	credEnvOverStored                 // a stored token and REWST_API_TOKEN: the environment wins, the stored token is held unused
	credConfigFile                    // api_token in a 0600 config file named by REWST_CONFIG
	credConfigAccess                  // access_token alone in a 0600 config file: sent as Bearer
	credConfigBoth                    // api_token, access_token, refresh_token, client_id and client_secret: api_token is sent, the rest held unused
	credConfigHeader                  // auth_header = "Bearer <token>" in a 0600 config file: sent as given
	credEnvPrefixed                   // REWST_API_TOKEN="Bearer <token>" (the P2 TestPrefixedEnvTokenDoublePrefixedKnownP2)
)

func (m credMode) String() string {
	return [...]string{"stored", "env REWST_API_TOKEN", "env over stored", "config api_token", "config access_token", "config every key",
		"config auth_header", "env prefixed Bearer"}[m]
}

// sent is the token the mode sends; held are the credentials it holds but
// never sends anywhere.
func (m credMode) sent() string {
	return [...]string{bbStored, bbToken, bbToken, bbFileToken, bbFileAccess, bbFileToken, bbFileHeader, bbToken}[m]
}

func (m credMode) held() []string {
	switch m {
	case credEnvOverStored:
		return []string{bbStored, bbHTTPToken}
	case credConfigBoth:
		return []string{bbFileAccess, bbFileRefresh, bbFileSecret, bbFileClient, bbHTTPToken}
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
		"HOME=" + home, "REWST_HOME=" + filepath.Join(home, ".rewst-home"), "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"REWST_BASE_URL=" + sv.url, "REWST_CA_CERT=" + sv.caFile,
		"REWST_CLI_PATH=" + bbCLI, "REWST_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}, trapEnv()...)
	e := &bbEnv{home: home, env: env}
	switch mode {
	case credStored:
		e.storeToken(t, bbStored)
	case credEnv:
		e.env = append(e.env, "REWST_API_TOKEN="+bbToken)
	case credEnvOverStored:
		e.storeToken(t, bbStored)
		e.env = append(e.env, "REWST_API_TOKEN="+bbToken)
	case credConfigFile:
		e.writeConfig(t, fmt.Sprintf("api_token = %q\n", bbFileToken))
	case credConfigAccess:
		e.writeConfig(t, fmt.Sprintf("access_token = %q\n", bbFileAccess))
	case credConfigBoth:
		e.writeConfig(t, fmt.Sprintf("api_token = %q\naccess_token = %q\nrefresh_token = %q\nclient_id = %q\nclient_secret = %q\n",
			bbFileToken, bbFileAccess, bbFileRefresh, bbFileClient, bbFileSecret))
	case credConfigHeader:
		e.writeConfig(t, fmt.Sprintf("auth_header = %q\n", "Bearer "+bbFileHeader))
	case credEnvPrefixed:
		e.env = append(e.env, "REWST_API_TOKEN=Bearer "+bbToken)
	}
	return e
}

// writeConfig writes body as a 0600 config file and names it with
// REWST_CONFIG.
func (e *bbEnv) writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := filepath.Join(e.home, "cfg")
	_ = os.MkdirAll(dir, 0o700)
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "REWST_CONFIG"), "REWST_CONFIG="+cfg)
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

// importFile is a pages import file (one JSONL record) whose name repeats
// tok.
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(e.home, "import.jsonl")
	if err := os.WriteFile(p, []byte(`{"name":"Imported `+tok+`","path":"/imported","orgId":"`+bbOrgID+`"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// orgVarBody is an org-variables create --stdin body (a secret variable)
// whose name repeats tok.
func orgVarBody(tok string) string {
	b, _ := json.Marshal(map[string]any{"name": "Stdin " + tok, "value": "HQ-Record stdin value", "category": "general", "orgId": bbOrgID, "cascade": false})
	return string(b)
}

// gqlBody is a request body carrying one GraphQL document (a typed write's
// --stdin, or an MCP rewst_execute params object, sends it to /graphql as
// written).
func gqlBody(doc string) string {
	b, _ := json.Marshal(map[string]any{"query": doc})
	return string(b)
}

// The GraphQL documents the battery sends through a typed write's --stdin
// (and MCP rewst_execute): the operations whose answers carry credentials.
const (
	mutCreateClient = `mutation CreateClient { createOrganizationApiClient(input: {name: "HQ-Record API client", orgId: "` + bbOrgID + `"}) { id name orgId auth0ClientId auth0ClientSecret secretRotatedAt } }`
	mutRotate       = `mutation Rotate { rotateApiClientSecret(id: "` + bbAPIClientID + `") { id auth0ClientId auth0ClientSecret secretRotatedAt } }`
	qThirdParty     = `query ThirdParty { getCannyToken getHaloLiveChatToken getSkilljarLoginToken }`
	qCanny          = `query { getCannyToken }`
	qHalo           = `query { getHaloLiveChatToken }`
	qSkilljar       = `query { getSkilljarLoginToken }`
	qOrgVars        = `query Vars { orgVariables(where: {orgId: "` + bbOrgID + `"}) { id name category value orgId } }`
	qOrgVar         = `query OneVar { variable: orgVariable(where: {id: "` + bbOrgVarID + `"}) { id name category value } }`
)

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
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%v: timed out", args)
	}
	return out.String() + "\n" + errb.String()
}

// mcpWait bounds one MCP session's wait for its replies.
const mcpWait = 180 * time.Second

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
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	id := 0
	deadline := time.Now().Add(mcpWait)
	for _, calls := range phases {
		count(&mcpCallsSent, len(calls))
		for _, c := range calls {
			id++
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		}
	waiting:
		for time.Now().Before(deadline) && len(mcpReplies(out.String())) < id+1 {
			select {
			case <-waitDone: // the server exited (a refusal to start): nothing more answers
				break waiting
			case <-time.After(100 * time.Millisecond):
			}
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

// kitWithheld reports whether text carries the credential kit's withhold
// notice (the whole unit replaced).
func kitWithheld(text string) bool { return strings.Contains(text, "[credkit: withheld from ") }

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
// (no caller does today: the work order's ruled P2
// credential-split-across-sql-bindings does not stand in this print, so the
// local store's files are checked for both halves too;
// TestStoreSplitAcrossRecordsWithheld).
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

// novels is every novel command: live GraphQL aggregators (POST /graphql
// queries), each in JSON and in its human form.
// fullSweep is REWST_BLACKBOX_FULL=1: every case of the exhaustive sweeps. The
// default run keeps every heavy test and its assertions on a representative
// sample, so the package fits go test's default 10-minute timeout, which is
// what CI runs it under. Run the full sweep once after a change to the
// credential layer or the kit.
var fullSweep = os.Getenv("REWST_BLACKBOX_FULL") == "1"

// lean is the default run's cheaper process for a test whose subject is neither
// the local store nor learn capture: a read goes live (opening the store for
// writing runs its 316 CREATE statements, about 2s a process at load 85), and
// nothing is captured for learning (about 3.5s a write). The tests of those
// sinks stay off it: TestNoCredentialInAnyWrittenFile, TestStoreSplitAcrossRecordsWithheld,
// TestMCPChildNeverReceivesAHeldValue.
func (e *bbEnv) lean() *bbEnv {
	if !fullSweep {
		e.env = append(e.env, "REWST_DATA_SOURCE=live", "REWST_NO_LEARN=1")
	}
	return e
}

// sampleCalls is all under fullSweep; otherwise, in order, the first call whose
// arguments start with each keep entry. An entry that matches nothing fails the
// test, so a sample cannot thin out unnoticed when the list changes.
func sampleCalls(t *testing.T, all []cliCall, keep ...string) []cliCall {
	t.Helper()
	if fullSweep {
		return all
	}
	taken := map[string]bool{}
	var out []cliCall
	for _, c := range all {
		line := strings.Join(c.args, " ")
		for _, k := range keep {
			if !taken[k] && strings.HasPrefix(line, k) {
				taken[k] = true
				out = append(out, c)
				break
			}
		}
	}
	for _, k := range keep {
		if !taken[k] {
			t.Fatalf("the sample names %q, which the list no longer has", k)
		}
	}
	return out
}

// outputSample is the default run's sample of battery for the first credential
// source of TestNoCredentialInAnyOutput: one call per output path and sink.
func outputSample(t *testing.T, all []cliCall, tok string) []cliCall {
	t.Helper()
	return sampleCalls(t, all,
		"organizations get --agent", // the agent envelope
		"api-clients "+bbDenied,     // the AUTH_ERR, 401 and CSRF echoes, both refused redirects
		"api-clients "+bbUnauth,
		"api-clients "+bbCSRF,
		"api-clients "+bbRedirect,
		"api-clients "+bbRedirIDN,
		"pack-delete-responses --pack-id "+bbDenied+" --yes", // a REST-shaped write
		"sync --json",                                        // the store, then a local read of it
		"organizations create --stdin --yes --json",          // a GraphQL document through --stdin
		"health --org "+bbOrgID+" --json",                    // novels, JSON and table
		"coverage --parent "+bbOrgID+" --json",
		"coverage --parent "+bbOrgID+" --pack",
		"pages get --data-source local",
		"health --org "+bbDenied, // novels that meet each failure
		"roi --org "+bbUnauth,
		"failures --org "+bbRedirect,
		"organizations get --json --"+tok, // the command line: flag, unknown command, feedback text
		base64.StdEncoding.EncodeToString([]byte(tok)),
		"feedback noted",
		"org-variables create --name Echo", // previews
		"health --org "+tok,
		"org-variables create --stdin --dry-run",
		"profile save", // local files, and the commands that read them back
		"profile show",
		"recall --query",
		"auth status",
	)
}

// mcpCore is the default run's MCP session for a credential source past the
// first: a read, a mirrored novel, a write preview.
func mcpCore() []map[string]any {
	return []map[string]any{
		read("organizations.get", map[string]any{}),
		tool("failures", map[string]any{"org": bbOrgID}),
		gqlExecute(mutCreateClient),
	}
}

// withoutTools is all under fullSweep, otherwise all less the named tools (the
// mirrored ones cost a CLI child each).
func withoutTools(all []map[string]any, names ...string) []map[string]any {
	if fullSweep {
		return all
	}
	var out []map[string]any
	for _, c := range all {
		if !slices.Contains(names, c["name"].(string)) {
			out = append(out, c)
		}
	}
	return out
}

func novels() [][]string {
	return [][]string{
		{"health", "--org", bbOrgID, "--json"},
		{"health", "--org", bbOrgID},
		{"failures", "--org", bbOrgID, "--since", "7d", "--json"},
		{"failures", "--org", bbOrgID, "--since", "7d"},
		{"roi", "--org", bbOrgID, "--json"},
		{"roi", "--org", bbOrgID},
		{"dormant", "--org", bbOrgID, "--days", "30", "--json"},
		{"dormant", "--org", bbOrgID},
		{"drift", "--org", bbOrgID, "--against", bbSubOrgID, "--json"},
		{"drift", "--org", bbOrgID, "--against", bbSubOrgID},
		{"coverage", "--parent", bbOrgID, "--json"},
		{"coverage", "--parent", bbOrgID, "--pack", "halo"},
	}
}

// localReads are the commands that read only the local store a sync filled.
func localReads() [][]string {
	return [][]string{
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
		{"stale", "--days", "1", "--json"},
		{"orphans", "--json"},
		{"load", "--json"},
		{"analytics", "--type", "pages", "--json"},
		{"workflow", "status", "--json"},
		{"pages", "get", "--data-source", "local", "--json"},
	}
}

// battery runs every output path; tok is the token the process holds (the
// token it sends: REWST_API_TOKEN, the stored token or a config file's
// api_token or access_token).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) []cliCall {
	imp := e.importFile(t, tok)
	b := calls(
		// Every output mode.
		[]string{"organizations", "get", "--json"},
		[]string{"organizations", "get"},
		[]string{"organizations", "get", "--csv"},
		[]string{"organizations", "get", "--plain"},
		[]string{"organizations", "get", "--agent"},
		[]string{"organizations", "get", "--json", "--select", "id,name,echo"},
		[]string{"organizations", "get", "--json", "--receipt"},
		[]string{"organizations", "get", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		[]string{"organizations", "get", "--json", "--deliver", "file:" + deliverFile},
		// Every response shape: the page list (split across two pages when
		// paged), an API client's auth0ClientId and a split echo (one client
		// by id), a BOM-prefixed answer of org variables (a secret one), a
		// gzip-encoded pack config holding an integration's credentials
		// (access and refresh tokens, a client secret, an encryption key, a
		// password, an auth username, a credentials container), the AUTH_ERR,
		// 401 and CSRF echoes, refused redirects (an ASCII host, a homoglyph
		// host), Rewst data under credential-like names.
		[]string{"pages", "get", "--json"},
		[]string{"pages", "get", "--limit", "100", "--json"},
		[]string{"pages", "get", "--limit", "100", "--offset", "100", "--json"},
		[]string{"api-clients", bbAPIClientID, "--json"},
		[]string{"api-clients", bbAPIClientID},
		[]string{"api-clients", bbDenied, "--json"},
		[]string{"api-clients", bbUnauth, "--json"},
		[]string{"api-clients", bbCSRF, "--json"},
		[]string{"api-clients", bbRedirect, "--json"},
		[]string{"api-clients", bbRedirIDN, "--json"},
		[]string{"api-client-lists", "--org-id", bbOrgID, "--json"},
		[]string{"api-client-lists", "--org-id", bbOrgID, "--csv"},
		[]string{"org-variables", "get", "--visible-for-org-id", bbOrgID, "--json"},
		[]string{"org-variables", "get", "--visible-for-org-id", bbOrgID},
		[]string{"org-variables", "get", "--visible-for-org-id", bbOrgID, "--csv"},
		[]string{"org-variables", "get", "--visible-for-org-id", bbDenied, "--json"},
		[]string{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID, "--json"},
		[]string{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID},
		[]string{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID, "--plain"},
		[]string{"users", "get", "--json"},
		[]string{"users", "get"},
		[]string{"logins", "--domain", bbLoginDomain, "--json"},
		[]string{"crates", "get", "--template-id", bbTemplateID, "--json"},
		[]string{"form-permission-states", "--form-id", bbFormID, "--org-id", bbOrgID, "--json"},
		[]string{"workflow-executions", "--where", `{"orgId":"` + bbOrgID + `"}`, "--json"},
		[]string{"permission-audit-log-lists", "--org-id", bbOrgID, "--json"},
		[]string{"permissions", "get", "--where", `{"orgId":"` + bbOrgID + `"}`, "--json"},
		// REST-shaped writes, each with and without --yes where it changes
		// data (a write without --yes is the work order's P2, but it must
		// never leak either): a secret org variable whose value repeats the
		// held token, a general one, a tag, an invite, a delete.
		[]string{"org-variables", "create", "--name", "hq_secret_var", "--value", "HQ " + tok, "--category", "secret", "--org-id", bbOrgID, "--cascade", "false", "--yes", "--json"},
		[]string{"org-variables", "create", "--name", "hq_secret_var", "--value", "HQ " + tok, "--category", "secret", "--org-id", bbOrgID, "--cascade", "false"},
		[]string{"org-variables", "create", "--name", "hq_general_var", "--value", "HQ-Record general", "--category", "general", "--org-id", bbOrgID, "--cascade", "false", "--yes"},
		[]string{"org-variables", "update", "--id", bbOrgVarID, "--name", "hq_secret_var", "--value", "HQ " + tok, "--category", "secret", "--yes", "--json"},
		[]string{"tags", "create", "--org-id", bbOrgID, "--name", "HQ " + tok, "--yes", "--json"},
		[]string{"user-invites", "create", "--email", "invite@hq-record.example.com", "--org-id", bbOrgID, "--role-ids", "admin", "--yes", "--json"},
		[]string{"workflows", "update", "--id", bbWorkflowID, "--name", "HQ " + tok, "--yes", "--json"},
		[]string{"pack-delete-responses", "--pack-id", bbPackID, "--yes", "--json"},
		[]string{"pack-delete-responses", "--pack-id", bbPackID},
		[]string{"pack-delete-responses", "--pack-id", bbDenied, "--yes", "--json"},
		[]string{"delete-org-interpreter-responses", bbOrgID, "--yes", "--json"},
		[]string{"import", "pages", "--input", imp, "--json"},
		// The local store.
		[]string{"sync", "--json"},
		[]string{"sync", "--resources", "permission-audit-log-lists", "--param", "orgId=" + bbOrgID, "--json"},
		[]string{"workflow", "archive", "--json"},
	)
	// GraphQL documents through a typed write's --stdin: the answers that
	// carry Rewst's documented credentials, and a request body whose name
	// repeats the held token.
	for _, c := range []struct {
		doc  string
		args []string
	}{
		{mutCreateClient, []string{"organizations", "create", "--stdin", "--yes", "--json"}},
		{mutCreateClient, []string{"organizations", "create", "--stdin", "--yes"}},
		{mutRotate, []string{"organizations", "create", "--stdin", "--yes", "--json"}},
		{qThirdParty, []string{"organizations", "create", "--stdin", "--yes", "--json"}},
		{qOrgVars, []string{"org-variables", "create", "--stdin", "--yes", "--json"}},
		{qOrgVar, []string{"org-variables", "create", "--stdin", "--yes"}},
	} {
		b = append(b, cliCall{stdin: gqlBody(c.doc), args: c.args})
	}
	b = append(b, cliCall{stdin: orgVarBody(tok), args: []string{"org-variables", "create", "--stdin", "--yes", "--json"}})
	b = append(b, calls(novels()...)...)
	b = append(b, calls(localReads()...)...)
	return append(append(b, calls(
		// Novels that meet an AUTH_ERR, a 401 and a redirect.
		[]string{"health", "--org", bbDenied, "--json"},
		[]string{"roi", "--org", bbUnauth, "--json"},
		[]string{"failures", "--org", bbRedirect, "--json"},
		// A sync that meets an AUTH_ERR (into its own store, after the local
		// reads: the main store stays whole).
		[]string{"sync", "--db", filepath.Join(e.home, "denied.db"), "--resources", "pages", "--param", "search=" + bbDenied, "--json"},
		// A credential on the command line: a credential-named query value
		// (sent, and echoed back), the same in a dry run, an unknown flag, a
		// positional, an unknown command spelled as its base64, feedback text.
		// (Spelled as a Zstandard frame: the P2 TestZstdFrameOfHeldValueKnownP2.)
		[]string{"sync", "--db", filepath.Join(e.home, "param.db"), "--resources", "pages", "--param", "password=" + bbArgvSec, "--json"},
		[]string{"sync", "--db", filepath.Join(e.home, "param.db"), "--resources", "pages", "--global-param", "password=" + bbArgvSec, "--dry-run"},
		[]string{"organizations", "get", "--json", "--" + tok},
		[]string{"api-clients", tok, "--json"},
		[]string{"api-clients", "--no-such-flag", tok},
		[]string{base64.StdEncoding.EncodeToString([]byte(tok))},
		[]string{"feedback", "noted the key " + tok + " in a ticket"},
		// Previews: a dry run sends nothing.
		[]string{"org-variables", "create", "--name", "Echo " + tok, "--value", "x", "--category", "general", "--org-id", bbOrgID, "--cascade", "false", "--dry-run"},
		[]string{"org-variables", "create", "--name", "dry_secret", "--value", tok, "--category", "secret", "--org-id", bbOrgID, "--cascade", "false", "--dry-run"},
		[]string{"pack-delete-responses", "--pack-id", tok, "--dry-run"},
		[]string{"api-clients", tok, "--dry-run"},
		[]string{"health", "--org", tok, "--dry-run"},
		[]string{"import", "pages", "--input", imp, "--dry-run"},
		// Learnings, playbooks, teach logs and profiles land in local files.
		[]string{"teach", "--query", "find pages " + tok, "--resource", bbPageID, "--resource-type", "pages",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		[]string{"teach", "--query", "find " + tok, "--resource", tok, "--resource-type", "pages"}, // a held resource id: refused, logged to teach.log
		[]string{"teach-playbook", "--query", "pages with " + tok, "--notes", "use " + tok},
		[]string{"profile", "save", "held", "--description", "uses " + tok, "--select", "id"},
		[]string{"profile", "show", "held"},
		[]string{"profile", "list"},
		[]string{"feedback", "list"},
		[]string{"learnings", "list", "--json"},
		[]string{"recall", "--query", "find pages " + tok},
		[]string{"jobs", "list", "--json"},
		[]string{"doctor"},
		[]string{"auth", "status"},
		[]string{"agent-context"},
	)...), cliCall{stdin: orgVarBody(tok), args: []string{"org-variables", "create", "--stdin", "--dry-run"}},
		cliCall{stdin: gqlBody(mutCreateClient), args: []string{"organizations", "create", "--stdin", "--dry-run"}})
}

func tool(name string, args map[string]any) map[string]any {
	return map[string]any{"name": name, "arguments": args}
}

// read is a rewst_read call (a read-only endpoint by its id).
func read(endpoint string, params map[string]any) map[string]any {
	return tool("rewst_read", map[string]any{"endpoint_id": endpoint, "params": params})
}

// execute is a rewst_execute call (a state-changing endpoint by its id),
// without confirm.
func execute(endpoint string, params map[string]any) map[string]any {
	return tool("rewst_execute", map[string]any{"endpoint_id": endpoint, "params": params})
}

// gqlExecute is a rewst_execute call that POSTs one GraphQL document to
// /graphql (organizations.create sends its params as the request body).
func gqlExecute(doc string) map[string]any {
	return execute("organizations.create", map[string]any{"query": doc})
}

// confirmed is call with confirm set to v.
func confirmed(call map[string]any, v any) map[string]any {
	call["arguments"].(map[string]any)["confirm"] = v
	return call
}

// mcpReads are read-only tools across every response shape the fake has,
// and every novel tool. The paged page list (the first half of a split
// credential, page 1 of two) runs in its own sessions
// (TestMCPSplitAcrossPagesWithheld, TestMCPSessionHalfKeepsLaterResults).
func mcpReads() []map[string]any {
	none := map[string]any{}
	org := map[string]any{"org": bbOrgID}
	return withoutTools([]map[string]any{
		tool("rewst_search", map[string]any{"query": "create an api client for an organization"}),
		tool("rewst_get", map[string]any{"endpoint_id": "org-variables.get"}),
		read("pages.get", none),
		read("organizations.get", none),
		read("org-variables.get", map[string]any{"visibleForOrgId": bbOrgID}),
		read("org-variables.get", map[string]any{"visibleForOrgId": bbDenied}),
		read("api-clients.get", map[string]any{"id": bbAPIClientID}),
		read("api-client-lists.get", map[string]any{"orgId": bbOrgID}),
		read("pack-configs.get", map[string]any{"orgId": bbOrgID}),
		read("workflow-executions.get", map[string]any{"where": `{"orgId":"` + bbOrgID + `"}`}),
		read("logins.get", map[string]any{"domain": bbLoginDomain}),
		read("users.get", none),
		read("form-permission-states.get", map[string]any{"formId": bbFormID, "orgId": bbOrgID}),
		read("crates.get", map[string]any{"templateId": bbTemplateID}),
		read("permissions.get", map[string]any{"where": `{"orgId":"` + bbOrgID + `"}`}),
		read("conversation-message-votes.get", map[string]any{"where": `{"userId":"` + bbUserID + `"}`}),
		tool("context", none),
		tool("health", org),
		tool("failures", map[string]any{"org": bbOrgID, "since": "7d"}),
		tool("roi", org),
		tool("dormant", org),
		tool("drift", map[string]any{"org": bbOrgID, "against": bbSubOrgID}),
		tool("coverage", map[string]any{"parent": bbOrgID}),
		tool("stale", map[string]any{"days": 1}),
		tool("orphans", none),
		tool("load", none),
		tool("analytics", map[string]any{"type": "pages"}),
		tool("search", map[string]any{"query": "HQ"}),
		tool("sql", map[string]any{"query": "SELECT data FROM resources WHERE resource_type = 'pages' AND id = '" + bbPageID + "'"}),
		tool("recall", map[string]any{"query": "HQ"}),
		tool("learnings_list", none),
		tool("learnings_candidates", none),
		tool("learnings_stats", none),
		tool("playbook_list", none),
		tool("workflow_status", none),
		tool("jobs_list", none),
	}, "roi", "dormant", "drift", "coverage", "orphans", "load", "analytics", "recall", "learnings_list",
		"learnings_candidates", "learnings_stats", "playbook_list", "workflow_status", "jobs_list")
}

// mcpWrites is every kind of write that takes confirm, called without it:
// each must answer with a preview and send nothing. A rewst_execute call
// previews as a confirmation_required object; the mirrored CLI writes
// (import, jobs prune) preview as the command's --dry-run output followed by
// the gate's "Preview only: nothing was sent" note. The GraphQL mutations
// whose answers carry credentials (createOrganizationApiClient,
// rotateApiClientSecret) are among them. A secret org variable's value that no
// other source holds is the P2 TestOrgVariableSecretValueHeldKnownP2.
func mcpWrites() []map[string]any {
	return withoutTools([]map[string]any{
		tool("import", map[string]any{"resource": "pages"}),
		tool("jobs_prune", map[string]any{}),
		gqlExecute(mutCreateClient),
		gqlExecute(mutRotate),
		execute("org-variables.create", map[string]any{"name": "hq_preview_secret", "value": "HQ-Record preview", "category": "secret", "orgId": bbOrgID, "cascade": false}),
		execute("org-variables.update", map[string]any{"id": bbOrgVarID, "name": "hq_preview", "value": "HQ-Record preview"}),
		execute("pages.create", map[string]any{"name": "HQ-Record preview page", "path": "/preview", "orgId": bbOrgID}),
		execute("pages.update", map[string]any{"id": bbPageID, "name": "HQ-Record preview"}),
		execute("pack-delete-responses.delete", map[string]any{"packId": bbPackID}),
		execute("delete-org-interpreter-responses.delete", map[string]any{"id": bbOrgID}),
		execute("user-invites.create", map[string]any{"email": "preview@hq-record.example.com", "orgId": bbOrgID, "roleIds": []any{"admin"}}),
		execute("workflows.update", map[string]any{"id": bbWorkflowID, "name": "HQ-Record preview"}),
		execute("tags.create", map[string]any{"name": "HQ-Record preview tag", "orgId": bbOrgID}),
		execute("permissions.update", map[string]any{"objectId": bbWorkflowID, "relation": "viewer"}),
	}, "jobs_prune")
}

// mcpBypasses try to turn a write preview into a send without confirm true:
// rewst_read asked for a write (refused), confirm inside params, and the
// mirrored import told dry-run false or yes. Each must preview or be
// refused, and send nothing.
func mcpBypasses() []map[string]any {
	return []map[string]any{
		read("pack-delete-responses.delete", map[string]any{"packId": bbPackID}),
		read("organizations.create", map[string]any{"query": mutCreateClient}),
		read("org-variables.create", map[string]any{"name": "hq_bypass", "value": "x", "orgId": bbOrgID}),
		execute("org-variables.create", map[string]any{"name": "hq_bypass", "value": "x", "orgId": bbOrgID, "confirm": true}),
		tool("import", map[string]any{"resource": "pages", "dry-run": false}),
		tool("import", map[string]any{"resource": "pages", "yes": true}),
	}
}

// mcpLocalWrites are mirrored tools that write only local files (no gate by
// design): their child CLI output passes the server's chokepoint too. The
// sync's credential-named query value is sent, and echoed back.
func mcpLocalWrites() []map[string]any {
	all := []map[string]any{
		tool("sync", map[string]any{"resources": []any{"permission-audit-log-lists"}, "param": []any{"orgId=" + bbOrgID}}),
		tool("sync", map[string]any{"resources": []any{"pages"}, "param": []any{"password=" + bbMCPArgSec}}),
	}
	if !fullSweep {
		return all[1:]
	}
	return all
}

func mcpBattery() []map[string]any {
	return append(append(append(mcpReads(), mcpWrites()...), mcpBypasses()...), mcpLocalWrites()...)
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
		if rel == "import.jsonl" || strings.HasPrefix(rel, "cfg"+string(filepath.Separator)) {
			return nil // the test's own inputs
		}
		scanned++
		fn(rel, data)
		return nil
	})
	return scanned
}

// checkOneAuth fails when a request authenticated any way but the one
// Authorization: Bearer <token> header.
func checkOneAuth(t *testing.T, sv *server, mode credMode) {
	t.Helper()
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.otherAuth != 0 {
		t.Errorf("%d request(s) carried a credential another way (the %s mode: a second Authorization header, one not \"Bearer <token>\", X-Api-Key, a query token); Rewst takes only the Authorization header", sv.otherAuth, mode)
	}
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	if !fullSweep {
		e.env = append(e.env, "REWST_DATA_SOURCE=live") // the store's own walk is sync's; learn capture stays on
	}
	deliverFile := filepath.Join(e.home, "deliver.json")
	// The default run: one call per file sink, and the failure answers the
	// check below counts. (The store with an echoing answer: the mirrored sync
	// below; the learn store with a held value:
	// TestMCPChildNeverReceivesAHeldValue; every write site:
	// TestEveryFileWritePassesTheChokepoint.)
	for _, c := range sampleCalls(t, battery(t, e, deliverFile, bbStored),
		"organizations get --json --audit-dir", "organizations get --json --deliver",
		"api-clients "+bbDenied, "api-clients "+bbUnauth, "api-clients "+bbCSRF,
		"health --org "+bbDenied, "roi --org "+bbUnauth,
		"feedback noted", "profile save") {
		e.runIn(t, c.stdin, c.args...)
	}
	// Controls without a credential: the same sinks write, so the scans
	// below are not vacuous.
	e.run(t, "feedback", "control note HQ-Feedback")
	if fullSweep {
		e.run(t, "teach", "--query", "find pages control", "--resource", bbPageID, "--resource-type", "pages",
			"--notes", "rotated control", "--no-validate")
	}
	e.run(t, "teach", "--resource", bbPageID, "--resource-type", "pages") // no --query: teach.log records the refusal
	e.run(t, "profile", "save", "control", "--description", "HQ-Profile control", "--select", "id")
	mcp := mcpBattery()
	if !fullSweep {
		mcp = append(append(mcpCore(), mcpLocalWrites()...),
			read("org-variables.get", map[string]any{"visibleForOrgId": bbDenied}),
			read("api-clients.get", map[string]any{"id": bbDenied}),
			read("api-clients.get", map[string]any{"id": bbUnauth}),
			read("pages.get", map[string]any{"limit": 100, "offset": 100}))
	}
	assertNoEcho(t, "MCP session", e.mcpCalls(t, mcp))
	if n, all := sv.sawKey(bbStored); n == 0 {
		t.Fatalf("no request carried the stored token as Authorization: Bearer (saw %d distinct values); the battery ran unauthenticated", len(all))
	}
	checkOneAuth(t, sv, credStored)
	// The fake echoes the token, so the kit withholds every record that
	// repeats it (a store row, a delivered file); the same sinks write from a
	// quiet fake, so the scans below are not vacuous.
	sv.mu.Lock()
	sv.quiet, sv.whole, sv.bare = true, true, true
	sv.mu.Unlock()
	deliverControl := filepath.Join(e.home, "deliver-control.json")
	e.run(t, "organizations", "get", "--json", "--deliver", "file:"+deliverControl)
	if fullSweep {
		e.run(t, "sync", "--resources", "pages", "--json")
	} else {
		e.run(t, "sync", "--resources", "pages", "--max-pages", "1", "--json")
	}
	if fullSweep {
		e.run(t, "sync", "--resources", "permission-audit-log-lists", "--param", "orgId="+bbOrgID, "--json")
	}
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
		// Every file but the SQLite store, which the kit no longer checks (synccpu).
		if !synccpuStoreFile(rel) { // synccpu: the local store is not checked
			assertNoEchoIn(t, "file "+rel, string(data), true)
		}
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-token wrote no credential store")
	}
	// The battery really wrote data, so the scan above is not vacuous.
	if b, err := os.ReadFile(deliverControl); err != nil || !bytes.Contains(b, []byte(bbOrgName)) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverControl), err)
	}
	var sawStore, sawPage, sawAudit, sawLearning, sawFeedback, sawFeedbackWithheld, sawProfile, sawTeachLog bool
	scanFiles(e.home, func(rel string, b []byte) {
		name := filepath.Base(rel)
		if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") {
			sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
			// The page record sync's default resource walked was stored, its
			// public orgId with it.
			sawPage = sawPage || (bytes.Contains(b, []byte(bbPageName)) && bytes.Contains(b, []byte(bbOrgID)))
			// The opt-in permission audit log walk ran.
			sawAudit = sawAudit || bytes.Contains(b, []byte("HQ-Record audit org"))
			// The control teach run's learning row was written.
			sawLearning = sawLearning || bytes.Contains(b, []byte("rotated control"))
		}
		// The control feedback entry was written, and the entries that
		// repeat a held credential were replaced by the kit's notice.
		sawFeedback = sawFeedback || (name == "feedback.jsonl" && bytes.Contains(b, []byte("HQ-Feedback")))
		sawFeedbackWithheld = sawFeedbackWithheld || (name == "feedback.jsonl" && kitWithheld(string(b)))
		sawProfile = sawProfile || bytes.Contains(b, []byte("HQ-Profile control"))
		sawTeachLog = sawTeachLog || (name == "teach.log" && bytes.Contains(b, []byte("missing --query")))
	})
	for what, ok := range map[string]bool{
		"no local store holds a fetched record":                                              sawStore,
		"no local store holds the synced page":                                               sawPage,
		"no local store holds the permission audit log entry; the opt-in walk did not run":   sawAudit || !fullSweep,
		"no local store holds the taught learning; the teach run wrote nothing":              sawLearning || !fullSweep,
		"no feedback.jsonl holds the control feedback entry; the feedback run wrote nothing": sawFeedback,
		"no feedback.jsonl holds the kit's notice for an entry that repeats a credential":    sawFeedbackWithheld,
		"no profile file holds the control profile; profile save wrote nothing":              sawProfile,
		"no teach.log holds the control refusal; teach wrote no log":                         sawTeachLog,
	} {
		if !ok {
			t.Error(what)
		}
	}
	// Some pager walked every page: the split-across-pages echo really ran;
	// and the failure echoes really ran (api-clients and org-variables with a
	// denied value, the 401 and the CSRF, a denied delete, the novels, the
	// denied sync, MCP org-variables.get).
	sv.mu.Lock()
	page2 := sv.pages["offset=100"]
	denials := sv.denials
	sv.mu.Unlock()
	if denials < 8 {
		t.Errorf("only %d request(s) met a failure answer; the error-echo runs did not all reach the fake", denials)
	}
	if page2 == 0 {
		t.Errorf("no pager asked for the second page of the page list; the split-across-pages echo did not run")
	}
}

// Class secret-in-output, once per credential source the token can come from
// that is not the stored token alone: REWST_API_TOKEN over a stored token
// (held unused), a config file's api_token, a config file's access_token
// (sent as Bearer) and every config key (access_token, refresh_token,
// client_id and client_secret held unused beside the api_token). (The stored
// token as the sent value runs in TestNoCredentialInAnyWrittenFile;
// TestEveryCredentialSourceSendsOneKey covers every source with a short
// battery.)
func TestNoCredentialInAnyOutput(t *testing.T) {
	budget(t)
	for i, mode := range []credMode{credEnvOverStored, credConfigFile, credConfigAccess, credConfigBoth} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode).lean()
			deliverFile := filepath.Join(e.home, "deliver.json")
			tok := mode.sent()
			// The default run: the first source walks one call per output path
			// and sink; the others send their token once and repeat what they hold.
			b := battery(t, e, deliverFile, tok)
			if i == 0 {
				b = outputSample(t, b, tok)
			} else {
				b = sampleCalls(t, b, "failures --org "+bbOrgID, "auth status")
			}
			for _, c := range b {
				assertNoEcho(t, fmt.Sprintf("output of %v", c.args), e.runIn(t, c.stdin, c.args...))
			}
			// A held value no API receives, typed where the program repeats
			// it: an argument the fake echoes, and previews.
			for _, held := range mode.held() {
				for _, c := range sampleCalls(t, []cliCall{
					{args: []string{"api-clients", held, "--json"}},
					{args: []string{"org-variables", "create", "--name", "HQ " + held, "--value", "x", "--category", "general", "--org-id", bbOrgID, "--cascade", "false", "--dry-run"}},
					{args: []string{"pack-delete-responses", "--pack-id", held, "--dry-run"}},
					{args: []string{"import", "pages", "--input", e.importFile(t, held), "--dry-run"}},
					{stdin: orgVarBody(held), args: []string{"org-variables", "create", "--stdin", "--dry-run"}},
				}, "org-variables create --name") {
					assertNoEcho(t, fmt.Sprintf("output of %v", c.args), e.runIn(t, c.stdin, c.args...))
				}
			}
			// An answer that echoes the token is withheld whole: stdout is the
			// kit's notice (the public data of a clean answer prints:
			// TestRewstPublicFieldsAreNotWithheld).
			echoing := [][]string{{"organizations", "get", "--json"}, {"org-variables", "get", "--visible-for-org-id", bbOrgID, "--json"}}
			if !fullSweep {
				echoing = [][]string{{"organizations", "get", "--json", "--data-source", "auto"}} // the store's read path
				if i > 0 {
					echoing = [][]string{{"failures", "--org", bbOrgID, "--json"}}
				}
			}
			for _, args := range echoing {
				cmd := exec.Command(bbCLI, args...)
				cmd.Env, cmd.Dir = e.env, e.home
				stdout, _ := cmd.Output()
				if !kitWithheld(string(stdout)) {
					t.Fatalf("%v: want the kit's withhold notice, got:\n%.1500s", args, stdout)
				}
			}
			mcp := mcpBattery()
			if !fullSweep {
				mcp = mcp[:len(mcp)-len(mcpLocalWrites())] // the mirrored sync: TestNoCredentialInAnyWrittenFile
				if i > 0 {
					mcp = mcpCore()
				}
			}
			out := e.mcpCalls(t, mcp)
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcp))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			if !strings.Contains(out, "HQ-Record") && !kitWithheld(out) {
				t.Fatalf("MCP results answered nothing:\n%.2000s", out)
			}
			// The fake echoed the token it really received, so the scans
			// above covered it; no other credential source was sent.
			if n, all := sv.sawKey(mode.sent()); n == 0 || len(all) != 1 {
				t.Fatalf("want every request to carry the %s token, got %d of it and distinct values %q", mode, n, all)
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

// rewst-credential-names-exact: Rewst data under names that read as
// credentials or carry identities prints as the API returned it, from the
// CLI and MCP alike, and the store keeps it: org ids and names, workflow and
// execution ids and names, a username, a user object, the Login page object,
// CrateToken lists under tokens and a tokenId, a zedtoken, secretRotatedAt,
// authorizedForOrganizations, a tokenApiClient's id and name, an org
// variable's value when its category is general, contact or system, and the
// CrateToken object under the generic name token. The credential kit holds a
// response field only by a name credwire declares, and none is a Rewst
// product name (TestDeclaredResponseFieldsAreNoRewstProductName). The kit
// withholds a whole unit that carries a held value, so the fake runs quiet
// here: no echo, no split, no declared credential member; the same reads
// run loud in TestNoCredentialInAnyOutput.
func TestRewstPublicFieldsAreNotWithheld(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	sv.quiet, sv.whole, sv.bare = true, true, true // public data in answers that carry no held value (TestNoCredentialInAnyOutput runs these reads loud)
	e := newEnv(t, sv, credEnv).lean()
	var cli strings.Builder
	// The default run: every read through MCP; the CLI's one read is held to
	// what that read answers.
	cliReads := [][]string{
		{"organizations", "get", "--json"},
		{"org-variables", "get", "--visible-for-org-id", bbOrgID, "--json"},
		{"api-client-lists", "--org-id", bbOrgID, "--json"},
		{"users", "get", "--json"},
		{"logins", "--domain", bbLoginDomain, "--json"},
		{"crates", "get", "--template-id", bbTemplateID, "--json"},
		{"form-permission-states", "--form-id", bbFormID, "--org-id", bbOrgID, "--json"},
		{"workflow-executions", "--where", `{"orgId":"` + bbOrgID + `"}`, "--json"},
		{"permissions", "get", "--where", `{"orgId":"` + bbOrgID + `"}`, "--json"},
		{"conversation-message-votes", "get", "--where", `{"userId":"` + bbUserID + `"}`, "--json"},
		{"failures", "--org", bbOrgID, "--since", "7d", "--json"},
	}
	if !fullSweep {
		cliReads = cliReads[:1]
	}
	for _, args := range cliReads {
		cli.WriteString(e.run(t, args...))
	}
	if fullSweep {
		cli.WriteString(e.runIn(t, gqlBody(mutRotate), "organizations", "create", "--stdin", "--yes", "--json"))
	}
	mcp := e.mcpCalls(t, []map[string]any{
		read("organizations.get", map[string]any{}),
		read("org-variables.get", map[string]any{"visibleForOrgId": bbOrgID}),
		read("api-client-lists.get", map[string]any{"orgId": bbOrgID}),
		read("users.get", map[string]any{}),
		read("logins.get", map[string]any{"domain": bbLoginDomain}),
		read("crates.get", map[string]any{"templateId": bbTemplateID}),
		read("form-permission-states.get", map[string]any{"formId": bbFormID, "orgId": bbOrgID}),
		read("workflow-executions.get", map[string]any{"where": `{"orgId":"` + bbOrgID + `"}`}),
		read("permissions.get", map[string]any{"where": `{"orgId":"` + bbOrgID + `"}`}),
		read("conversation-message-votes.get", map[string]any{"where": `{"userId":"` + bbUserID + `"}`}),
		tool("failures", map[string]any{"org": bbOrgID, "since": "7d"}),
		confirmed(gqlExecute(mutRotate), true),
	})
	for where, got := range map[string]string{"CLI": cli.String(), "MCP": mcp} {
		wants := []*regexp.Regexp{
			publicField("id", bbOrgID),
			publicField("name", bbOrgName),
			publicField("orgId", bbOrgID),
			publicField("parentOrgId", bbOrgID),
			publicField("name", bbSubOrgName),
			publicField("value", bbGeneralValue),
			publicField("value", bbContactValue),
			publicField("value", bbSystemValue),
			publicField("username", bbUsername),
			publicField("name", bbAPIClientName),
			publicField("secretRotatedAt", bbRotatedAt),
			publicField("zedtoken", bbZedtoken),
			publicField("tokenId", bbTokenID),
			publicField("domain", bbLoginDomain),
			publicField("name", bbWorkflowName),
			regexp.MustCompile(`\\*"tokens\\*":\s*\[\s*\{[^\]]*` + regexp.QuoteMeta(bbTokenName)),
			regexp.MustCompile(`\\*"login\\*":\s*\{[^{}]*` + regexp.QuoteMeta(bbLoginDomain) + `|\\*"domain\\*":\s*\\*"` + regexp.QuoteMeta(bbLoginDomain)),
			regexp.MustCompile(`\\*"user\\*":\s*\{[^{}]*` + regexp.QuoteMeta(bbUsername)),
			regexp.MustCompile(`\\*"authorizedForOrganizations\\*":\s*\[\s*\{[^\]]*` + regexp.QuoteMeta(bbSubOrgName)),
			regexp.MustCompile(`\\*"tokenApiClient\\*":\s*\{[^{}]*` + regexp.QuoteMeta(bbAPIClientName)),
			regexp.MustCompile(regexp.QuoteMeta(bbExecID)),
		}
		sampled := where == "CLI" && !fullSweep
		if sampled {
			wants = wants[:2]
		}
		for _, want := range wants {
			if !want.MatchString(got) {
				t.Errorf("%s withheld Rewst data (want %s):\n%.3000s", where, want, got)
			}
		}
		// Rewst data under the generic name token: a CrateToken object.
		if !sampled && !strings.Contains(got, bbTokenObjName) {
			t.Errorf("%s withheld the CrateToken object under token (rewst-credential-names-exact):\n%.3000s", where, got)
		}
		assertNoEcho(t, where, got)
	}
	if !fullSweep {
		return // the store's half runs in TestNoCredentialInAnyWrittenFile (sawPage)
	}
	// The store keeps it too: the page record sync's default resource walked.
	e.run(t, "sync", "--resources", "pages", "--json")
	var stored bool
	scanFiles(e.home, func(rel string, b []byte) {
		if isStoreFile(rel) {
			stored = stored || (bytes.Contains(b, []byte(bbPageName)) && bytes.Contains(b, []byte(bbOrgID)))
		}
	})
	if !stored {
		t.Errorf("the local store dropped the synced page or its orgId")
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host never carries the credential there, for a REST-shaped read and a
// GraphQL POST, from the CLI, sync and MCP alike. The redirect to another
// host (whose Location repeats the token) is refused before it is followed;
// the hop to a homoglyph of api.rewst.io is followed without the credential:
// the client drops Authorization on an origin change and the credential kit
// judges the hop strictly (leak contract 2(c)). The proxy trap opens that
// hop's tunnel (tunnel_test.go), reads the request and refuses it; the
// request carries no credential.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	budget(t)
	before, tunnelsBefore := len(trapDials()), len(tunnelRequests())
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	cliOut := map[string]string{
		"api-clients":           e.run(t, "api-clients", bbRedirect, "--json"),
		"api-clients homoglyph": e.run(t, "api-clients", bbRedirIDN, "--json"),
		"failures (POST)":       e.run(t, "failures", "--org", bbRedirect, "--json"),
		"health homoglyph":      e.run(t, "health", "--org", bbRedirIDN, "--json"),
	}
	sv.mu.Lock()
	sv.redirectPages = true
	sv.mu.Unlock()
	cliOut["sync"] = e.run(t, "sync", "--resources", "pages", "--json")
	mcp := e.mcpCalls(t, []map[string]any{
		read("api-clients.get", map[string]any{"id": bbRedirect}),
		read("pages.get", map[string]any{"limit": 100}),
		tool("health", map[string]any{"org": bbRedirIDN}),
		confirmed(gqlExecute(`query { orgVariables(where: {orgId: "`+bbRedirect+`"}) { id } }`), true),
	})
	sv.mu.Lock()
	n := sv.offHost
	redirected := len(sv.methods)
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if redirected < 9 {
		t.Fatalf("only %d request(s) reached the fake; the CLI, sync and MCP calls did not all run: %v", redirected, sv.allMethods())
	}
	failed := regexp.MustCompile(`refusing redirect|proxyconnect|Forbidden|403`)
	for where, out := range cliOut {
		if !failed.MatchString(out) {
			t.Errorf("%s: want the redirect refused or failed, got:\n%.1500s", where, out)
		}
	}
	if c := len(failed.FindAllString(mcp, -1)); c < 4 {
		t.Errorf("MCP: want each of the 4 calls refused or failed, got %d:\n%.3000s", c, mcp)
	}
	for _, d := range trapDials()[before:] {
		if d != homoglyphConnect {
			t.Errorf("a redirect dialed out (through the proxy trap): %s", d)
		}
	}
	// The hop to the homoglyph host carried no credential: the trap read the
	// request inside its tunnel.
	hops := tunnelRequests()[tunnelsBefore:]
	if len(hops) == 0 {
		t.Errorf("no redirect to %s reached the trap's tunnel; the homoglyph case did not run", homoglyphHost)
	}
	for _, h := range hops {
		if strings.Contains(strings.ToLower(h), "authorization") {
			t.Errorf("the redirected hop to %s carried an Authorization header:\n%.500s", homoglyphHost, h)
		}
		assertNoEcho(t, "the redirected hop to "+homoglyphHost, h)
	}
	for where, out := range cliOut {
		assertNoEcho(t, where, out)
	}
	assertNoEcho(t, "MCP", mcp)
}

// Class credential-sent-to-another-host, origin form: the token goes only to
// the origin of the configured base URL (the credential kit's declared
// origin), whatever a later input says. An argument, a GraphQL variable or an
// MCP parameter shaped like a URL (absolute, scheme-relative, userinfo, a dot
// segment, a host spelled with a Cyrillic homoglyph) stays a value sent to
// that origin; and a configured base URL that does not resolve to one ASCII
// origin (an unresolved placeholder, a homoglyph host, from the config file
// or the environment) is no allowed origin, so every request is refused
// before any dial: by the request builder that cannot read the host, or by
// the kit, which withholds a request carrying a held value to a host it
// cannot spell in ASCII. The proxy trap stands in for the network: a request
// that got past would reach it, never a real host.
func TestOriginPinNeverMovesTheKey(t *testing.T) {
	budget(t)
	before := len(trapDials())
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv).lean()
	const homoglyph = "https://api.rew\u0455t.io/x" // Cyrillic s (U+0455) in place of the Latin one
	hostile := []string{"https://evil.example.com/x", "//evil.example.com/x", "@evil.example.com", "..%2F..%2F..%2Fevil", "evil.example.com:443", homoglyph}
	if !fullSweep {
		hostile = []string{hostile[0], homoglyph}
	}
	var out strings.Builder
	for _, id := range hostile {
		out.WriteString(e.run(t, "api-clients", id, "--json"))
		if fullSweep {
			out.WriteString(e.run(t, "org-variables", "get", "--visible-for-org-id", id, "--json"))
		}
		out.WriteString(e.run(t, "health", "--org", id, "--json"))
	}
	var mcpCalls []map[string]any
	for _, id := range hostile {
		mcpCalls = append(mcpCalls, read("api-clients.get", map[string]any{"id": id}),
			read("org-variables.get", map[string]any{"visibleForOrgId": id}),
			tool("roi", map[string]any{"org": id}),
			execute("pack-delete-responses.delete", map[string]any{"packId": id}))
	}
	out.WriteString(e.mcpCalls(t, mcpCalls))
	// Every request reached the fake: the REST-shaped reads and the GraphQL
	// POSTs alike.
	var gets, posts int
	for _, m := range sv.allMethods() {
		gets += strings.Count(m, "GET /graphql rest")
		posts += strings.Count(m, "POST /graphql query:")
	}
	if gets < 2*len(hostile) || posts < len(hostile) {
		t.Errorf("the hostile values never reached the configured origin (GET %d, GraphQL POST %d); requests: %v", gets, posts, sv.allMethods())
	}
	// A base URL that is no allowed origin: every request, a REST-shaped read
	// and a GraphQL POST, CLI and MCP, is refused before any dial (by the
	// request builder that cannot read the host, or by the kit).
	refused := regexp.MustCompile(`\[credkit: withheld from |invalid character \\?"\{\\?" in host name|must be an absolute URL with a host`)
	for name, base := range map[string]string{
		"placeholder": "https://{tenant}.api.rewst.io",
		"homoglyph":   "https://api.rew\u0455t.io",
	} {
		home := t.TempDir()
		cfg := filepath.Join(home, "pin.toml")
		if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
			t.Fatal(err)
		}
		pe := &bbEnv{home: home, env: cleanEnv(home, "REWST_API_TOKEN="+bbToken, "REWST_CONFIG="+cfg, "REWST_CLI_PATH="+bbCLI,
			"REWST_MCP_HTTP_TOKEN="+bbHTTPToken)}
		envBase := &bbEnv{home: home, env: append(withoutEnv(pe.env, "REWST_CONFIG"), "REWST_BASE_URL="+base)}
		envs := []*bbEnv{pe, envBase}
		if !fullSweep {
			envs = envs[:1] // a placeholder from the config file
			if name == "homoglyph" {
				envs = []*bbEnv{envBase} // a homoglyph from the environment
			}
		}
		for _, env := range envs {
			reads := [][]string{
				{"organizations", "get", "--json"},
				{"api-clients", bbAPIClientID, "--json"},
				{"health", "--org", bbOrgID, "--json"},
			}
			if !fullSweep {
				reads = reads[1:]
			}
			for _, args := range reads {
				got := env.run(t, args...)
				if !refused.MatchString(got) {
					t.Errorf("%v with a %s base URL: want a refusal before any dial, got:\n%.1000s", args, name, got)
				}
				out.WriteString(got)
			}
			mcp := env.mcpCalls(t, []map[string]any{
				read("organizations.get", map[string]any{}),
				tool("health", map[string]any{"org": bbOrgID}),
			})
			if len(refused.FindAllString(mcp, -1)) < 2 {
				t.Errorf("MCP calls with a %s base URL: want a refusal twice, got:\n%.2000s", name, mcp)
			}
			out.WriteString(mcp)
		}
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

// mcpSplitHalf is the page-1 read of the page list that splits the first
// credential across two pages.
func mcpSplitHalf() map[string]any { return read("pages.get", map[string]any{"limit": 100}) }

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a read (a GET,
// or a GraphQL query) while no call sets confirm, though the battery calls
// every kind of write (rewst_execute, a GraphQL mutation through it, and the
// mirrored import and jobs prune) and tries to bypass the gate (rewst_read
// for a write, confirm inside params, dry-run false, a blocked --yes); each
// write answers with a preview, and rewst_read refuses a write outright.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	// Quiet: the kit withholds a whole result that repeats a held value, and
	// this test reads what each call answered (TestNoCredentialInAnyOutput
	// runs the same battery loud).
	sv.quiet, sv.whole, sv.bare = true, true, true
	e := newEnv(t, sv, credEnv).lean()
	// The store holds the synced pages first, so the local tools have
	// something to read while the writes preview.
	// The default run skips that walk and the tools that read it (the store is
	// read back through MCP in TestSyncWalksEveryPage).
	readCalls, mcp := mcpReads(), mcpBattery()
	if fullSweep {
		e.run(t, "sync", "--json")
	} else {
		readCalls = withoutTools(readCalls, "search", "sql", "stale")
		mcp = append(append(slices.Clone(readCalls), mcpWrites()...), mcpBypasses()...)
	}
	sv.resetMethods()
	out := e.mcpCalls(t, mcp)
	if !strings.Contains(out, "HQ-Record") {
		t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
	}
	replies := mcpReplies(out)
	for i, call := range mcpWrites() {
		id := len(readCalls) + i + 1
		if r := replies[id]; !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") {
			t.Errorf("%s %v without confirm did not return a preview:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	for i, call := range mcpBypasses() {
		id := len(readCalls) + len(mcpWrites()) + i + 1
		r := replies[id]
		if r == "" || (!strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") &&
			!strings.Contains(r, `"isError":true`)) {
			t.Errorf("%s %v was neither previewed nor refused:\n%.1500s", call["name"], call["arguments"], r)
		}
		if args, _ := call["arguments"].(map[string]any); call["name"] == "rewst_read" && !strings.Contains(r, "changes state") {
			t.Errorf("rewst_read %v was not refused:\n%.1500s", args, r)
		}
	}
	// The novels and the reads answered from the fake, and the local tools
	// from the synced store (a refusal would leave the leak checks vacuous
	// for them).
	for i, call := range readCalls {
		want := map[any]string{
			"health": "verdict", "failures": bbWorkflowName, "roi": bbWorkflowName, "dormant": bbDormantName,
			"drift": "only_in_parent_var", "coverage": "halo_psa", "sql": bbPageName, "search": bbPageName,
		}[call["name"]]
		if args, _ := call["arguments"].(map[string]any); call["name"] == "rewst_read" {
			want = map[any]string{"organizations.get": bbOrgName, "users.get": bbUsername, "pack-configs.get": bbPackConfName,
				"api-client-lists.get": bbAPIClientName, "workflow-executions.get": bbExecID, "logins.get": bbLoginDomain,
				"pages.get": bbPageName}[args["endpoint_id"]]
		}
		if r := replies[i+1]; want != "" && !strings.Contains(r, want) {
			t.Errorf("%s %v read nothing (want %q):\n%.1500s", call["name"], call["arguments"], want, r)
		}
	}
	var reads int
	for _, m := range sv.allMethods() {
		if isWrite(m) {
			t.Errorf("the MCP server sent %s without confirm", m)
		} else {
			reads++
		}
	}
	if reads < 15 {
		t.Errorf("only %d read(s) reached the fake; the reads did not run: %v", reads, sv.allMethods())
	}
	checkOneAuth(t, sv, credEnv)
}

// Control for the test above: with confirm set to boolean true the same
// write reaches the API exactly once, so the gate previews rather than drops;
// the string "true" and the number 1 still preview. Both gates:
// rewst_execute (a REST-shaped org variable create, a REST-shaped pack
// delete, and createOrganizationApiClient as a GraphQL mutation, whose answer
// carries an auth0ClientSecret, each endpoint id as rewst_search names it)
// and the mirrored write (import). The mirror blocks import's required
// --input (a file the server would read for the caller), so a mirrored
// import can never send a record: its control is that confirm true runs the
// child as given (its own answer, without --dry-run or the gate's preview
// note) while every other value previews.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv).lean()
	search := e.mcpCalls(t, []map[string]any{
		tool("rewst_search", map[string]any{"query": "create an orgvariable"}),
		tool("rewst_search", map[string]any{"query": "delete a packdeleteresponse"}),
		tool("rewst_search", map[string]any{"query": "create an organization"}),
	})
	for _, id := range []string{"org-variables.create", "pack-delete-responses.delete", "organizations.create"} {
		if !strings.Contains(search, id) {
			t.Errorf("rewst_search did not name %s:\n%.2000s", id, search)
		}
	}
	write := func(confirm any) []map[string]any {
		return []map[string]any{
			confirmed(execute("org-variables.create", map[string]any{"name": "hq_confirmed", "value": "HQ-Record confirmed", "category": "general",
				"orgId": bbOrgID, "cascade": false}), confirm),
			confirmed(execute("pack-delete-responses.delete", map[string]any{"packId": bbPackID}), confirm),
			confirmed(gqlExecute(mutCreateClient), confirm),
			tool("import", map[string]any{"resource": "pages", "confirm": confirm}),
		}
	}
	previews := e.mcpPhases(t, write("true"), write(1))
	for _, m := range sv.allMethods() {
		if isWrite(m) {
			t.Errorf("confirm \"true\" or 1 sent %s", m)
		}
	}
	replies := mcpReplies(previews)
	for _, id := range []int{1, 2, 3, 5, 6, 7} {
		if !strings.Contains(replies[id], "confirmation_required") {
			t.Errorf("call %d did not preview:\n%.1000s", id, replies[id])
		}
	}
	for _, id := range []int{4, 8} {
		if !strings.Contains(replies[id], "Preview only: nothing was sent") {
			t.Errorf("mirrored import call %d did not preview:\n%.1000s", id, replies[id])
		}
	}
	out := e.mcpPhases(t, write(true))
	var create, del, mutation int
	for _, m := range sv.allMethods() {
		switch m {
		case "POST /graphql rest":
			create++
		case "DELETE /graphql rest":
			del++
		case "POST /graphql mutation:createOrganizationApiClient":
			mutation++
		}
	}
	if create != 1 || del != 1 || mutation != 1 {
		t.Errorf("confirm true: want exactly one REST-shaped POST, one DELETE and one createOrganizationApiClient mutation, got %d, %d and %d; requests: %v\n%.1500s",
			create, del, mutation, sv.allMethods(), out)
	}
	// The mutation answered (its answer carries an auth0ClientSecret, a
	// declared response credential, so the kit withholds it whole).
	if r := mcpReplies(out)[3]; !kitWithheld(r) && !strings.Contains(r, bbAPIClientName) {
		t.Errorf("confirm true: the mutation answered nothing:\n%.1500s", r)
	}
	if r := mcpReplies(out)[4]; r == "" || strings.Contains(r, "Preview only") || strings.Contains(r, "dry run") || !strings.Contains(r, "input") {
		t.Errorf("confirm true did not run the mirrored import as given:\n%.1500s", r)
	}
	assertNoEcho(t, "MCP confirmed writes", out)
	checkOneAuth(t, sv, credEnv)
}

// dry-run-sends-nothing: a --dry-run never reaches the API, from a cold start
// with the token in the environment, the credential store or a config file
// (api_token, or access_token alone).
func TestDryRunSendsNothing(t *testing.T) {
	budget(t)
	for i, mode := range []credMode{credEnv, credStored, credConfigFile, credConfigAccess} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode).lean()
		imp := e.importFile(t, "dry")
		all := append(calls(
			[]string{"org-variables", "create", "--name", "dry", "--value", "Dry", "--category", "general", "--org-id", bbOrgID, "--cascade", "false", "--dry-run"},
			[]string{"org-variables", "create", "--name", "dry", "--value", "ColdPass7c1", "--category", "secret", "--org-id", bbOrgID, "--cascade", "false", "--yes", "--dry-run"},
			[]string{"org-variables", "update", "--id", bbOrgVarID, "--name", "dry", "--value", "Dry", "--dry-run"},
			[]string{"tags", "create", "--org-id", bbOrgID, "--name", "dry", "--dry-run"},
			[]string{"workflows", "update", "--id", bbWorkflowID, "--name", "Dry", "--dry-run"},
			[]string{"pack-delete-responses", "--pack-id", bbPackID, "--yes", "--dry-run"},
			[]string{"delete-org-interpreter-responses", bbOrgID, "--dry-run"},
			[]string{"import", "pages", "--input", imp, "--dry-run"},
			[]string{"organizations", "get", "--dry-run"},
			[]string{"pages", "get", "--limit", "100", "--dry-run"},
			[]string{"api-clients", bbAPIClientID, "--dry-run"},
			[]string{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID, "--dry-run"},
			[]string{"health", "--org", bbOrgID, "--dry-run"},
			[]string{"failures", "--org", bbOrgID, "--dry-run"},
			[]string{"drift", "--org", bbOrgID, "--against", bbSubOrgID, "--dry-run"},
			[]string{"coverage", "--parent", bbOrgID, "--dry-run"},
			[]string{"sync", "--resources", "pages", "--dry-run"},
		), cliCall{stdin: orgVarBody("Cold-Stdin-7c4"), args: []string{"org-variables", "create", "--stdin", "--dry-run"}},
			cliCall{stdin: gqlBody(mutCreateClient), args: []string{"organizations", "create", "--stdin", "--yes", "--dry-run"}})
		// The default run: one preview per kind for the first source (a write,
		// a delete, an import, a read, a novel, --stdin), a write for the others,
		// and the control once. (A sync's preview: the numeric and whitespace
		// tests.)
		if i == 0 {
			all = sampleCalls(t, all, "org-variables create --name dry --value Dry", "org-variables create --name dry --value ColdPass7c1",
				"pack-delete-responses", "import pages", "organizations get", "health --org", "org-variables create --stdin")
		} else {
			all = sampleCalls(t, all, "tags create")
		}
		for _, c := range all {
			// Each run reaches its preview, so the count below is not vacuous
			// (a flag error would send nothing either). A --stdin preview
			// repeats the body, which the kit holds (credentials read from
			// stdin, leak contract section 1), so it is withheld whole.
			out := e.runIn(t, c.stdin, c.args...)
			if c.stdin != "" && kitWithheld(out) {
				continue
			}
			if !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") &&
				!strings.Contains(out, "dry-run") && !strings.Contains(out, "would ") {
				t.Errorf("%v (mode %s) printed no preview:\n%.1500s", c.args, mode, out)
			}
		}
		if m := sv.allMethods(); len(m) != 0 {
			t.Errorf("--dry-run (mode %s) sent %v", mode, m)
		}
		if !fullSweep && i > 0 {
			continue
		}
		// Control: the same write and read without --dry-run reach the fake.
		e.run(t, "pack-delete-responses", "--pack-id", bbPackID, "--yes")
		e.run(t, "organizations", "get", "--json")
		if m := sv.allMethods(); len(m) != 2 || m[0] != "DELETE /graphql rest" || m[1] != "GET /graphql rest" {
			t.Errorf("control (mode %s): want the one DELETE and the one GET, got %v", mode, m)
		}
	}
}

// credential-split-across-output-records over MCP: the fake splits the first
// credential it was sent across the two pages of the page list, one half
// each. One session reads both pages; the halves must never both reach the
// client.
func TestMCPSplitAcrossPagesWithheld(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{mcpSplitHalf()},
		[]map[string]any{read("pages.get", map[string]any{"limit": 100, "offset": 100})})
	if !strings.Contains(out, `"id":2`) {
		t.Fatalf("MCP session did not answer both pages:\n%.2000s", out)
	}
	sv.mu.Lock()
	page2 := sv.pages["offset=100"]
	sv.mu.Unlock()
	if page2 == 0 {
		t.Fatalf("the second page was never asked for; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
}

// Over-withholding control: the MCP result carrying half a held credential
// (a page named with half the API token, 18 characters, a piece the leak
// contract counts: section 2(b)) is withheld whole, and later unrelated
// results of the same session (an organizations read, then a write preview)
// are still delivered. The fake runs quiet and bare, so only the split
// carries a held value.
func TestMCPSessionHalfKeepsLaterResults(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	sv.quiet, sv.bare = true, true
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{mcpSplitHalf()},
		[]map[string]any{read("organizations.get", map[string]any{})},
		[]map[string]any{execute("tags.create", map[string]any{"name": "HQ-Record preview tag", "orgId": bbOrgID})})
	replies := mcpReplies(out)
	if r := replies[1]; !kitWithheld(r) {
		t.Errorf("the page carrying half the token was not withheld:\n%.1500s", r)
	}
	if r := replies[2]; !strings.Contains(r, bbOrgName) {
		t.Errorf("the organizations read after a delivered half was withheld:\n%.1500s", r)
	}
	if r := replies[3]; !strings.Contains(r, "confirmation_required") {
		t.Errorf("the preview after a delivered half was withheld:\n%.1500s", r)
	}
	assertNoEcho(t, "MCP session", out)
}

// A record that splits a held token across two adjacent values, a line
// break and a space (the fake's "split" member on one API client) is
// withheld whole: each half is a piece of 8 or more characters (leak
// contract section 2(b)), and the kit withholds the whole MCP result that
// carries one. The fake runs quiet and bare, so only the split carries a
// held value.
func TestMCPSplitRecordWithheldWhole(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	sv.quiet, sv.bare = true, true
	e := newEnv(t, sv, credEnv)
	out := e.mcpCalls(t, []map[string]any{read("api-clients.get", map[string]any{"id": bbAPIClientID})})
	if r := mcpReplies(out)[1]; !kitWithheld(r) {
		t.Errorf("api-clients.get with a split token was not withheld:\n%.2000s", r)
	}
	assertNoEcho(t, "MCP api-clients.get", out)
}

// P2 (a wrong request, not a leak; config.go AuthHeader, a generated file):
// REWST_API_TOKEN given already prefixed ("Bearer <token>", the way the
// header reads in Rewst's docs) is sent as "Authorization: Bearer Bearer
// <token>", which the gateway rejects; the auth_header config key is sent as
// given. The doubled prefix never leaks the token (the chokepoint holds it
// with and without the scheme word). Runs only with REWST_BB_P2=1; fails
// while the P2 stands.
func TestPrefixedEnvTokenDoublePrefixedKnownP2(t *testing.T) {
	budget(t)
	if os.Getenv("REWST_BB_P2") == "" {
		t.Skip("P2 REWST_API_TOKEN=\"Bearer x\" is sent double-prefixed; set REWST_BB_P2=1 to reproduce")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvPrefixed)
	assertNoEcho(t, "output", e.run(t, "organizations", "get", "--json"))
	if n, all := sv.sawKey(bbToken); n == 0 || len(all) != 1 {
		t.Errorf("want Authorization: Bearer %s, got %d of it and distinct values %q", bbToken, n, all)
	}
	checkOneAuth(t, sv, credEnvPrefixed)
}
