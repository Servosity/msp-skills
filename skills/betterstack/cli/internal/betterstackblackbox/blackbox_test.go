// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package betterstackblackbox runs the built CLI and MCP binaries against a
// Better Stack Uptime fake (JSON:API, page-number paging with absolute links)
// and checks what a reviewer cannot read off the code, end to end under the
// credential kit (betterstack handfixes api-origin-pinned, no-response-cache,
// dry-run-sends-nothing, mcp-endpoint-write-confirm, mcp-mirror-write-confirm,
// export-file-closed-on-error, config-file-holds-no-credential):
//
//   - no credential leaves for a host other than the configured one by
//     redirect;
//   - the MCP server sends a write (a POST, PATCH or DELETE) only when the
//     call sets confirm to the boolean true, and --dry-run sends nothing at
//     all;
//   - what the program prints and writes on those paths holds no credential.
//
// The credential classes themselves are proven on the same binaries by
// internal/credwire/leak_test.go.
//
// A heartbeat's url (its push URL) is product data the operator needs, not a
// credential this program holds: it is never echoed as one.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package betterstackblackbox

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
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bbToken      = "uptimeTOKEN7fb3189bQ6u3sZxW4o1n7R8v52" // BETTERSTACK_API_TOKEN (sent as Authorization: Bearer <token>)
	bbStored     = "storedToken-71c3e9b05d2a4f68e1"        // auth set-token (stdin), the credential store
	bbFileToken  = "configFileToken-9d4c2b7e1a5f38"        // api_token in a 0600 config file named by BETTERSTACK_CONFIG
	bbFileAccess = "configFileAccess-2b9d7e4a1c63"         // access_token in a 0600 config file named by BETTERSTACK_CONFIG
	bbArgvSec    = "-argvSecret-91c3e7a5"                  // --auth-password on the command line (starts with "-")
	bbMCPArgSec  = "mcpArgSecret-81d3c6f0"                 // a credential-named argument an MCP call previews (never sent)
	bbShortResp  = "Sh0rtPw"                               // a 7-character auth_password a monitor record carries
	bbBOMResp    = "BomRespSecret-6a1c"                    // a status page password in a BOM-prefixed response
	bbGzipResp   = "GzipRespToken-4f2a9c1e"                // a policy's incident_token in a gzip-encoded response
	bbNewSecret  = "newPolicyIncidentToken-0b7e3d5a9c1f"   // the incident_token POST /policies returns
	bbHeaderSec  = "cQ7vZr2k-6d0a2e8c4f71Wm"               // a monitor's request header value under a credential name (X-Api-Key)
	bbHTTPToken  = "mcpHttpToken-0123456789abcdefABCDEF"   // BETTERSTACK_MCP_HTTP_TOKEN
	bbMonitorID  = "2085431"                               // a monitor ID
	bbHeartbeat  = "3141"                                  // a heartbeat ID
	bbIncidentID = "90321"                                 // an incident ID
	bbPolicyID   = "77"                                    // an escalation policy ID
	bbStatusPage = "184620"                                // a status page ID
	bbSectionID  = "5150"                                  // a status page section ID
	bbOnCallID   = "4242"                                  // an on-call calendar ID
	bbRedirect   = "9911"                                  // a monitor ID the fake redirects off-host
	bbDenied     = "40199"                                 // a monitor or incident ID (or a query value) the fake answers 401
	// bbPushURL is a heartbeat's push URL: operator data, never withheld.
	bbPushURL = "https://uptime.betterstack.com/api/v1/heartbeat/hqPushK3y9xQ2v"
	// bbTenantHeader is a monitor request header value under a name that is
	// not a credential's: operator data, never withheld.
	bbTenantHeader = "HQ-Tenant-7"
)

func secrets() []string {
	return []string{bbToken, bbStored, bbFileToken, bbFileAccess, bbArgvSec, bbMCPArgSec, bbShortResp,
		bbBOMResp, bbGzipResp, bbNewSecret, bbHeaderSec, bbHTTPToken}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "betterstack-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "betterstack-cli"+exe)
	bbMCP = filepath.Join(dir, "betterstack-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/betterstack-cli", bbMCP: "../../cmd/betterstack-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	fmt.Fprintf(os.Stderr, "betterstackblackbox: %d CLI runs, %d MCP tools/call requests\n", cliRuns.n, mcpCallsSent.n)
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
		string(pem.EncodeToMemory(&pem.Block{Type: "BETTERSTACK TOKEN", Bytes: []byte(s)})),
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

// sentSecrets is every known secret the request carried (its headers, the
// URL or the body): a real API can echo only what it was sent.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	seen := fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
	var out []string
	for _, s := range secrets() {
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
	methods []string       // API requests, "METHOD /path"
	bearers map[string]int // tokens the API received as the one header Authorization: Bearer <token>
	// otherAuth counts requests whose credential came any other way: an
	// X-API-KEY header, a second Authorization header, or one that is not a
	// bearer token (Better Stack takes only Authorization: Bearer <token>).
	otherAuth int
	// pages counts each list page the fake served, by "/path?page=N".
	pages map[string]int
	// redirectIncidents sends the incidents list (the syncable resource
	// triage ranks) to the other host too, so sync meets a redirect.
	redirectIncidents bool
	// splitGroups names the two monitor-groups pages' groups with the halves
	// of the first credential the fake was sent. Under the credential kit a
	// store write carrying a half is refused, so only the test of that split
	// turns it on.
	splitGroups bool
	// denied paths answer 401 (an export or sync that meets an API error).
	denied map[string]bool
	// denials counts the requests the fake answered 401.
	denials int
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

func isWrite(m string) bool {
	method, _, _ := strings.Cut(m, " ")
	return method != http.MethodGet
}

// jsonItem is one JSON:API resource object; attrs is the body of its
// attributes object.
func jsonItem(id, typ, attrs string) string {
	return fmt.Sprintf(`{"id":%q,"type":%q,"attributes":{%s}}`, id, typ, attrs)
}

// single is the Better Stack get/create/update envelope.
func single(id, typ, attrs string) string { return `{"data":` + jsonItem(id, typ, attrs) + `}` }

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{bearers: map[string]int{}, pages: map[string]int{}, denied: map[string]bool{}}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"first":null,"last":null,"prev":null,"next":null}}`))
	}))
	now := time.Now().UTC()
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	started := now.Add(-3 * time.Hour).Format(time.RFC3339)
	acked := now.Add(-170 * time.Minute).Format(time.RFC3339)
	resolved := now.Add(-2 * time.Hour).Format(time.RFC3339)
	origin := ""
	// listBody is a Better Stack list envelope of one page: JSON:API data and
	// page-number pagination with absolute links. Every page after the first
	// is empty, so every pager stops.
	listBody := func(r *http.Request, items ...string) string {
		self := origin + r.URL.Path
		link := func(n int) string { return fmt.Sprintf("%q", fmt.Sprintf("%s?page=%d", self, n)) }
		if pg := r.URL.Query().Get("page"); pg != "" && pg != "1" {
			return fmt.Sprintf(`{"data":[],"pagination":{"first":%s,"last":%s,"prev":%s,"next":null}}`, link(1), link(1), link(1))
		}
		return fmt.Sprintf(`{"data":[%s],"pagination":{"first":%s,"last":%s,"prev":null,"next":null}}`, strings.Join(items, ","), link(1), link(1))
	}
	// requestHeaders is a monitor's request_headers: one under a credential
	// name, one under a name that is not.
	requestHeaders := fmt.Sprintf(`"request_headers":[{"id":"406511","name":"X-Api-Key","value":%q},{"id":"406512","name":"X-Tenant","value":%q}]`, bbHeaderSec, bbTenantHeader)
	monitor := func(id, name, status, policy, group string, extra string) string {
		return jsonItem(id, "monitor", fmt.Sprintf(`"url":"https://hq-record.example.com/health","pronounceable_name":%q,"monitor_type":"status",`+
			`"status":%q,"paused":false,"policy_id":%s,"monitor_group_id":%s,"email":true,"sms":false,"call":false,"push":false,`+
			`"check_frequency":180,"verify_ssl":true,"auth_username":"hq-probe","auth_password":%q,%s,"last_checked_at":%q,"created_at":%q,"updated_at":%q,%s`,
			name, status, policy, group, bbShortResp, requestHeaders, recent, recent, recent, extra))
	}
	policy := func(id, name, token string, extra string) string {
		tok := "null"
		if token != "" {
			tok = fmt.Sprintf("%q", token)
		}
		return jsonItem(id, "policy", fmt.Sprintf(`"name":%q,"repeat_count":3,"repeat_delay":60,"incident_token":%s,`+
			`"steps":[{"type":"escalation","wait_before":0,"urgency_id":1}],"created_at":%q,"updated_at":%q%s`, name, tok, recent, recent, extra))
	}
	statusPage := func(echo string) string {
		return jsonItem(bbStatusPage, "status_page", fmt.Sprintf(`"company_name":"HQ-Record","subdomain":"hq-record","custom_domain":null,`+
			`"aggregate_state":"operational","subscribable":true,"timezone":"UTC","password_enabled":true,"password":%q,"announcement":%q,`+
			`"created_at":%q,"updated_at":%q,"echo":%s`, bbBOMResp, bbBOMResp, recent, recent, echo))
	}
	heartbeat := func(echo string) string {
		return jsonItem(bbHeartbeat, "heartbeat", fmt.Sprintf(`"url":%q,"name":"HQ-Record nightly backup","period":86400,"grace":0,`+
			`"status":"down","heartbeat_group_id":"71","policy_id":null,"sort_index":0,"paused":false,"created_at":%q,"updated_at":%q,"echo":%s`,
			bbPushURL, recent, recent, echo))
	}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		// Under the credential kit a response that repeats a held credential is
		// withheld whole, so the records the tests read carry no echo of what
		// the request sent (internal/credwire/leak_test.go proves the echoes).
		echo := "null"
		p := strings.TrimPrefix(r.URL.Path, "/api/v2")
		q := r.URL.Query()
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		auth := r.Header.Values("Authorization")
		if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && len(auth) == 1 && tok != "" && !strings.ContainsAny(tok, " \t") {
			sv.bearers[tok]++
		} else if len(auth) > 0 {
			sv.otherAuth++
		}
		if r.Header.Get("X-API-KEY") != "" {
			sv.otherAuth++
		}
		pg := q.Get("page")
		if pg == "" {
			pg = "1"
		}
		if r.Method == http.MethodGet {
			sv.pages[p+"?page="+pg]++
		}
		redirectIncidents := sv.redirectIncidents
		denied := sv.denied[p]
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		get := r.Method == http.MethodGet
		deniedQuery := false
		for _, vs := range q {
			for _, v := range vs {
				deniedQuery = deniedQuery || v == bbDenied
			}
		}
		switch {
		case denied || deniedQuery || strings.Contains(p, "/"+bbDenied):
			// Rejected (a denied monitor or incident, a sync or export the test
			// denies): the error repeats what it was sent.
			sv.mu.Lock()
			sv.denials++
			sv.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"errors":[{"status":"401","title":"Unauthorized","detail":"invalid token %s"}],"message":"bad token %s","echo":%s}`,
				strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/monitors/"+bbRedirect || (redirectIncidents && p == "/incidents"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && p == "/monitors":
			// The fleet the novels read: a down monitor with no escalation
			// policy, and an up one. Each carries its basic-auth password (a
			// short credential) and its request headers.
			fmt.Fprint(w, listBody(r,
				monitor(bbMonitorID, "HQ-Record site", "down", "null", `"61"`, `"echo":`+echo),
				monitor("2085432", "HQ-Record api", "up", `"`+bbPolicyID+`"`, `"61"`, `"note":"HQ"`)))
		case get && strings.HasPrefix(p, "/monitors/"):
			// One monitor: the echo split across values, lines and spaces.
			fmt.Fprint(w, `{"data":`+monitor(path.Base(p), "HQ-Record site", "down", "null", `"61"`, `"split":`+splitObject(sent))+`}`)
		case get && p == "/monitor-groups":
			// The first credential it was sent, split across two pages, paged
			// as Better Stack pages (per_page records, default 50, full pages
			// until the last): the first group of page 1 is one half, the one
			// group of page 2 the other, so every pager that walks the pages
			// meets both halves as records of their own.
			a, b := "group-a", "group-b"
			sv.mu.Lock()
			split := sv.splitGroups
			sv.mu.Unlock()
			if split && len(sent) > 0 {
				a, b = halves(sent[0])
			}
			per := 50
			if n, err := strconv.Atoi(q.Get("per_page")); err == nil && n > 0 {
				per = min(n, 250)
			}
			self := origin + r.URL.Path
			link := func(n int) string { return fmt.Sprintf("%q", fmt.Sprintf("%s?page=%d&per_page=%d", self, n, per)) }
			group := func(id int, name string) string {
				return jsonItem(strconv.Itoa(id), "monitor_group", fmt.Sprintf(`"name":%q,"sort_index":%d,"paused":false`, name, id))
			}
			switch pg {
			case "1":
				items := []string{group(61, a)}
				for i := 1; i < per; i++ {
					items = append(items, group(61+i, fmt.Sprintf("HQ-Record group %d", i)))
				}
				fmt.Fprintf(w, `{"data":[%s],"pagination":{"first":%s,"last":%s,"prev":null,"next":%s}}`, strings.Join(items, ","), link(1), link(2), link(2))
			case "2":
				fmt.Fprintf(w, `{"data":[%s],"pagination":{"first":%s,"last":%s,"prev":%s,"next":null}}`, group(61+per, b), link(1), link(2), link(1))
			default:
				fmt.Fprintf(w, `{"data":[],"pagination":{"first":%s,"last":%s,"prev":%s,"next":null}}`, link(1), link(2), link(2))
			}
		case get && p == "/monitor-groups/61":
			fmt.Fprint(w, single("61", "monitor_group", `"name":"HQ-Record group","sort_index":0,"paused":false,"echo":`+echo))
		case get && p == "/heartbeats":
			fmt.Fprint(w, listBody(r, heartbeat(echo)))
		case get && strings.HasPrefix(p, "/heartbeats/"):
			fmt.Fprint(w, `{"data":`+heartbeat(echo)+`}`)
		case get && p == "/heartbeat-groups":
			fmt.Fprint(w, listBody(r, jsonItem("71", "heartbeat_group", `"name":"HQ-Record heartbeats","sort_index":0,"paused":false,"echo":`+echo)))
		case get && strings.HasPrefix(p, "/heartbeat-groups/"):
			fmt.Fprint(w, single(path.Base(p), "heartbeat_group", `"name":"HQ-Record heartbeats","sort_index":0,"paused":false,"echo":`+echo))
		case get && p == "/incidents":
			// One open incident nobody acknowledged (triage, down), one
			// acknowledged and resolved (mttr, flapping).
			fmt.Fprint(w, listBody(r,
				jsonItem(bbIncidentID, "incident", fmt.Sprintf(`"name":"HQ-Record outage","url":"https://hq-record.example.com/health","cause":"Status 503",`+
					`"status":"Started","started_at":%q,"acknowledged_at":null,"resolved_at":null,"monitor_id":%q,"response_content":"HQ-Record down","echo":%s`,
					recent, bbMonitorID, echo)),
				jsonItem("90320", "incident", fmt.Sprintf(`"name":"HQ-Record blip","url":"https://hq-record.example.com/health","cause":"Timeout",`+
					`"status":"Resolved","started_at":%q,"acknowledged_at":%q,"acknowledged_by":"tech@example.com","resolved_at":%q,"resolved_by":"tech@example.com","monitor_id":%q`,
					started, acked, resolved, bbMonitorID))))
		case get && strings.HasPrefix(p, "/incidents/"):
			fmt.Fprint(w, single(path.Base(p), "incident", fmt.Sprintf(`"name":"HQ-Record outage","status":"Started","started_at":%q,"monitor_id":%q,"echo":%s`,
				recent, bbMonitorID, echo)))
		case get && p == "/on-calls":
			// A calendar with nobody on call (oncall-gaps).
			fmt.Fprint(w, listBody(r, jsonItem(bbOnCallID, "on_call_calendar", `"name":"HQ-Record rota","default_calendar":true,"on_call_users":[],"echo":`+echo)))
		case get && strings.HasPrefix(p, "/on-calls/"):
			fmt.Fprint(w, single(path.Base(p), "on_call_calendar", `"name":"HQ-Record rota","default_calendar":true,"on_call_users":[],"echo":`+echo))
		case get && (p == "/policies" || strings.HasPrefix(p, "/policies/")):
			// Policies (list and get, sync, export): gzip-encoded, each with
			// its incident_token.
			doc := `{"data":` + policy(path.Base(p), "HQ-Record escalation", bbGzipResp, `,"echo":null`) + `}`
			if p == "/policies" {
				doc = listBody(r, policy(bbPolicyID, "HQ-Record escalation", bbGzipResp, `,"echo":null`))
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			_, _ = io.WriteString(zw, doc)
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case get && p == "/status-pages":
			// Status pages: BOM-prefixed, with the page's password.
			fmt.Fprint(w, "\xef\xbb\xbf"+listBody(r, statusPage(echo)))
		case get && strings.HasSuffix(p, "/sections"):
			fmt.Fprint(w, listBody(r, jsonItem(bbSectionID, "status_page_section", `"name":"HQ-Record core","position":0,"echo":`+echo)))
		case get && strings.HasSuffix(p, "/resources"):
			fmt.Fprint(w, listBody(r, jsonItem("1", "status_page_resource", fmt.Sprintf(`"public_name":"HQ-Record site","resource_type":"Monitor",`+
				`"resource_id":%q,"status":"down","availability":0.9921,"position":0,"echo":%s`, bbMonitorID, echo))))
		case get && strings.Contains(p, "/sections/"):
			fmt.Fprint(w, single(path.Base(p), "status_page_section", `"name":"HQ-Record core","position":0,"echo":`+echo))
		case get && strings.Contains(p, "/resources/"):
			fmt.Fprint(w, single(path.Base(p), "status_page_resource", `"public_name":"HQ-Record site","resource_type":"Monitor","echo":`+echo))
		case get && strings.HasPrefix(p, "/status-pages/"):
			fmt.Fprint(w, "\xef\xbb\xbf"+`{"data":`+statusPage(echo)+`}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && p == "/policies":
			// A new policy comes back with its incident_token.
			fmt.Fprint(w, single("78", "policy", fmt.Sprintf(`"name":"HQ-Record","incident_token":%q,"echo":%s`, bbNewSecret, echo)))
		case !get:
			// Every other write echoes what it was sent.
			fmt.Fprint(w, single("7", strings.Trim(strings.SplitN(strings.TrimPrefix(p, "/"), "/", 2)[0], "s"),
				`"name":"HQ-Record","status":"ok","echo":`+echo))
		default:
			fmt.Fprint(w, single("1", "unknown", `"name":"HQ-Record","echo":`+echo))
		}
	}))
	origin = srv.URL
	sv.url = srv.URL + "/api/v2"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// sawBearer reports how many API requests carried Authorization: Bearer v,
// and the distinct bearer tokens the fake received.
func (sv *server) sawBearer(v string) (int, []string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var all []string
	for h := range sv.bearers {
		all = append(all, h)
	}
	return sv.bearers[v], all
}

func (sv *server) resetMethods() {
	sv.mu.Lock()
	sv.methods = nil
	sv.mu.Unlock()
}

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its bearer token.
type credMode int

const (
	credStored        credMode = iota // none in the environment: the battery runs on a token stored with auth set-token
	credEnv                           // BETTERSTACK_API_TOKEN
	credEnvOverStored                 // a stored token and BETTERSTACK_API_TOKEN: the environment wins, the stored token is held unused
	credConfigFile                    // api_token in a 0600 config file named by BETTERSTACK_CONFIG
	credConfigAccess                  // access_token in a 0600 config file named by BETTERSTACK_CONFIG
	credConfigBoth                    // both config keys: api_token wins, access_token is held unused
)

func (m credMode) String() string {
	return [...]string{"stored", "env BETTERSTACK_API_TOKEN", "env over stored", "config api_token", "config access_token", "config both keys"}[m]
}

// sent is the token the mode sends; held are the tokens it holds but never sends.
func (m credMode) sent() string {
	return [...]string{bbStored, bbToken, bbToken, bbFileToken, bbFileAccess, bbFileToken}[m]
}

func (m credMode) held() []string {
	switch m {
	case credEnvOverStored:
		return []string{bbStored, bbHTTPToken}
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
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"BETTERSTACK_BASE_URL=" + sv.url, "BETTERSTACK_CA_CERT=" + sv.caFile,
		"BETTERSTACK_CLI_PATH=" + bbCLI, "BETTERSTACK_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	e := &bbEnv{home: home, env: env}
	switch mode {
	case credStored:
		e.storeToken(t, bbStored)
	case credEnv:
		e.env = append(e.env, "BETTERSTACK_API_TOKEN="+bbToken)
	case credEnvOverStored:
		e.storeToken(t, bbStored)
		e.env = append(e.env, "BETTERSTACK_API_TOKEN="+bbToken)
	case credConfigFile:
		e.writeConfig(t, fmt.Sprintf("api_token = %q\n", bbFileToken))
	case credConfigAccess:
		e.writeConfig(t, fmt.Sprintf("access_token = %q\n", bbFileAccess))
	case credConfigBoth:
		e.writeConfig(t, fmt.Sprintf("api_token = %q\naccess_token = %q\n", bbFileToken, bbFileAccess))
	}
	return e
}

// withoutEnv is env without key.
func withoutEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}

// writeConfig writes body as a 0600 config file and names it with
// BETTERSTACK_CONFIG.
func (e *bbEnv) writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := filepath.Join(e.home, "cfg")
	_ = os.MkdirAll(dir, 0o700)
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "BETTERSTACK_CONFIG"), "BETTERSTACK_CONFIG="+cfg)
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
	deadline := time.Now().Add(180 * time.Second)
	for _, calls := range phases {
		count(&mcpCallsSent, len(calls))
		for _, c := range calls {
			id++
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		}
		for time.Now().Before(deadline) && len(mcpReplies(out.String())) < id+1 {
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
}

// importFile writes an import input whose one record repeats tok, for the
// import write path (POST /monitors per record).
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(e.home, "import.jsonl")
	if err := os.WriteFile(p, []byte(`{"monitor_type":"status","url":"https://import.example.com","pronounceable_name":"Imported `+tok+`"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// monitorBody is a monitors create --stdin body: a monitored site's basic-auth
// password and a request header under a credential name, both pw.
func monitorBody(name, pw string) string {
	b, _ := json.Marshal(map[string]any{
		"monitor_type": "status", "url": "https://hq-record.example.com/health", "pronounceable_name": name,
		"auth_username": "hq-probe", "auth_password": pw,
		"request_headers": []any{map[string]any{"name": "X-Api-Key", "value": pw}, map[string]any{"name": "X-Tenant", "value": bbTenantHeader}},
	})
	return string(b)
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

// novels is every novel command, each reading the local store after sync.
func novels() [][]string {
	return [][]string{
		{"fleet", "--json"},
		{"fleet"},
		{"coverage", "--json"},
		{"coverage", "--include-paused"},
		{"mttr", "--days", "30", "--by-monitor", "--json"},
		{"mttr"},
		{"flapping", "--days", "30", "--json"},
		{"oncall-gaps", "--json"},
		{"heartbeat-risk", "--json"},
		{"heartbeat-risk"},
		{"down", "--json"},
		{"down"},
		{"triage", "--json"},
		{"triage"},
		{"statuspage-audit", "--json"},
		{"group-health", "--json"},
		{"group-health"},
	}
}

// battery runs every output path; tok is a credential the process holds
// (the token it sends: BETTERSTACK_API_TOKEN, the stored token or a config
// file's api_token or access_token).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) []cliCall {
	imp := e.importFile(t, tok)
	b := calls(
		// Every output mode.
		[]string{"monitors", "list", "--json"},
		[]string{"monitors", "list"},
		[]string{"monitors", "list", "--csv"},
		[]string{"monitors", "list", "--plain"},
		[]string{"monitors", "list", "--agent"},
		[]string{"monitors", "list", "--json", "--receipt"},
		[]string{"monitors", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		[]string{"monitors", "list", "--json", "--deliver", "file:" + deliverFile},
		// Every response shape: a monitor's request headers and short
		// basic-auth password, split echo, BOM-prefixed (a status page
		// password), gzip-encoded (a policy's incident token, an echoed
		// identifier), 401 echo, refused redirect, a heartbeat's push URL,
		// the nested status page resources, a new policy's incident token.
		[]string{"monitors", "get", bbMonitorID, "--json"},
		[]string{"monitors", "get", bbMonitorID},
		[]string{"monitors", "get", bbDenied, "--json"},
		[]string{"monitors", "get", bbRedirect, "--json"},
		[]string{"monitors", "list", "--name", bbDenied, "--json"},
		[]string{"incidents", "get", bbDenied, "--json"},
		[]string{"incidents", "list", "--json"},
		[]string{"incidents", "get", bbIncidentID, "--json"},
		[]string{"status-pages", "list", "--json"},
		[]string{"status-pages", "list"},
		[]string{"status-pages", "get", bbStatusPage, "--json"},
		[]string{"status-page-sections", "list", bbStatusPage, "--json"},
		[]string{"status-page-sections", "get", bbStatusPage, bbSectionID, "--json"},
		[]string{"status-page-resources", "list", bbStatusPage, "--json"},
		[]string{"status-page-resources", "get", bbStatusPage, "1", "--json"},
		[]string{"policies", "list", "--json"},
		[]string{"policies", "list"},
		[]string{"policies", "get", bbPolicyID, "--json"},
		[]string{"heartbeats", "list", "--json"},
		[]string{"heartbeats", "get", bbHeartbeat, "--json"},
		[]string{"heartbeat-groups", "list", "--json"},
		[]string{"monitor-groups", "list", "--json"},
		[]string{"monitor-groups", "list", "--page", "2", "--per-page", "1", "--json"},
		[]string{"on-calls", "list", "--json"},
		[]string{"on-calls", "get", bbOnCallID, "--json"},
		// Writes the fake echoes (each with and without --yes where it
		// changes data; a write without --yes is the work order's P2, but it
		// must never leak either).
		[]string{"monitors", "create", "--type", "status", "--url", "https://hq-record.example.com", "--name", "HQ " + tok, "--json"},
		[]string{"monitors", "update", bbMonitorID, "--name", "HQ " + tok, "--yes", "--json"},
		[]string{"monitors", "update", bbMonitorID, "--name", "HQ " + tok},
		[]string{"monitors", "delete", bbMonitorID, "--yes", "--json"},
		[]string{"monitors", "delete", bbMonitorID},
		[]string{"heartbeats", "create", "--name", "HQ " + tok, "--period", "60", "--grace", "30", "--json"},
		[]string{"heartbeats", "update", bbHeartbeat, "--name", "HQ " + tok, "--yes", "--json"},
		[]string{"incidents", "acknowledge", bbIncidentID, "--by", "tech@example.com", "--yes", "--json"},
		[]string{"incidents", "resolve", bbIncidentID, "--json"},
		[]string{"policies", "create", "--name", "HQ policy", "--yes", "--json"},
		[]string{"policies", "create", "--name", "HQ " + tok},
		[]string{"status-page-sections", "create", bbStatusPage, "--name", "HQ " + tok, "--json"},
		[]string{"status-page-resources", "delete", bbStatusPage, "1", "--yes", "--json"},
		[]string{"import", "monitors", "--input", imp, "--json"},
		// The local store, exports and the novel commands.
		[]string{"sync", "--json"},
		[]string{"workflow", "archive", "--json"},
		[]string{"export", "monitors", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		[]string{"export", "policies", "--format", "json"},
		[]string{"export", "monitor-groups", "--format", "json"},
		[]string{"search", "HQ", "--json"},
		[]string{"search", "Echo", "--json", "--data-source", "local"},
		[]string{"workflow", "status", "--json"},
	)
	// A request body on stdin: a monitored site's basic-auth password and a
	// request header under a credential name (the held token itself).
	b = append(b, cliCall{stdin: monitorBody("HQ stdin", tok), args: []string{"monitors", "create", "--stdin", "--json"}})
	b = append(b, calls(novels()...)...)
	return append(append(b, calls(
		// A sync that meets a 401 (a filtered walk into its own store, after
		// the novels: the main store stays whole).
		[]string{"sync", "--db", filepath.Join(e.home, "denied.db"), "--resources", "on-calls", "--param", "team=" + bbDenied, "--json"},
		// A credential on the command line: a credential-named flag, an unknown
		// flag, a positional, feedback text.
		[]string{"monitors", "create", "--auth-password", bbArgvSec, "--type", "status", "--name", "argv"},
		[]string{"monitors", "list", "--json", "--" + tok},
		[]string{"monitors", "get", tok, "--json"},
		[]string{"feedback", "noted the key " + tok + " in a ticket"},
		// Previews: a dry run sends nothing.
		[]string{"monitors", "update", bbMonitorID, "--name", "Echo " + tok, "--dry-run"},
		[]string{"monitors", "create", "--type", "status", "--name", "Echo " + tok, "--dry-run"},
		[]string{"monitors", "create", "--auth-password", tok, "--type", "status", "--dry-run"},
		[]string{"incidents", "resolve", bbIncidentID, "--dry-run"},
		[]string{"import", "monitors", "--input", imp, "--dry-run"},
		// Learnings, playbooks, teach logs and profiles land in local files.
		[]string{"teach", "--query", "find monitors " + tok, "--resource", bbMonitorID, "--resource-type", "monitors",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		[]string{"teach", "--query", "find " + tok, "--resource", tok, "--resource-type", "monitors"}, // a held resource id: refused, logged to teach.log
		[]string{"teach-playbook", "--query", "monitors with " + tok, "--notes", "use " + tok},
		[]string{"profile", "save", "held", "--description", "uses " + tok, "--select", "id"},
		[]string{"profile", "show", "held"},
		[]string{"profile", "list"},
		[]string{"feedback", "list"},
		[]string{"learnings", "list", "--json"},
		[]string{"recall", "--query", "find monitors " + tok},
		[]string{"doctor"},
		[]string{"auth", "status"},
		[]string{"agent-context"},
	)...), cliCall{stdin: monitorBody("Echo stdin", tok), args: []string{"monitors", "create", "--stdin", "--dry-run"}})
}

// mcpReads are read-only tools across every response shape the fake has, and
// every novel tool. The monitor groups list (the first half of a split
// credential, page 1 of two) runs in its own session
// (TestMCPSplitAcrossPagesWithheld): a delivered half makes the history check
// withhold later results of the session whole (the P2
// TestMCPSessionHalfWithholdsLaterResultsKnownP2), which would hide every
// preview this battery checks.
func mcpReads() []map[string]any {
	none := map[string]any{}
	tool := func(name string, args map[string]any) map[string]any {
		return map[string]any{"name": name, "arguments": args}
	}
	return []map[string]any{
		tool("monitors_list", none),
		tool("monitors_get", map[string]any{"id": bbMonitorID}),
		tool("monitors_get", map[string]any{"id": bbDenied}),
		tool("monitor-groups_get", map[string]any{"id": "61"}),
		tool("heartbeats_list", none),
		tool("heartbeats_get", map[string]any{"id": bbHeartbeat}),
		tool("heartbeat-groups_list", none),
		tool("incidents_list", none),
		tool("incidents_get", map[string]any{"id": bbIncidentID}),
		tool("on-calls_list", none),
		tool("on-calls_get", map[string]any{"id": bbOnCallID}),
		tool("policies_list", none),
		tool("policies_get", map[string]any{"id": bbPolicyID}),
		tool("status-pages_list", none),
		tool("status-pages_get", map[string]any{"id": bbStatusPage}),
		tool("status-page-sections_list", map[string]any{"status-page-id": bbStatusPage}),
		tool("status-page-resources_list", map[string]any{"status-page-id": bbStatusPage}),
		tool("context", none),
		tool("fleet", none),
		tool("coverage", none),
		tool("mttr", map[string]any{"days": 30}),
		tool("flapping", map[string]any{"days": 30}),
		tool("oncall_gaps", none),
		tool("heartbeat_risk", none),
		tool("down", none),
		tool("triage", none),
		tool("statuspage_audit", none),
		tool("group_health", none),
		tool("search", map[string]any{"query": "HQ"}),
		tool("sql", map[string]any{"query": "SELECT data FROM resources"}),
		tool("recall", map[string]any{"query": "HQ"}),
		tool("learnings_list", none),
		tool("workflow_status", none),
	}
}

// mcpWrites is every write tool that takes confirm, called without it: each
// must answer with a preview and send nothing. An endpoint tool previews as a
// confirmation_required object; the mirrored CLI write (import) previews as
// the command's --dry-run output followed by the gate's "Preview only:
// nothing was sent" note. monitors_create also carries credential-named
// arguments its preview repeats (a basic-auth password and a request header).
func mcpWrites() []map[string]any {
	tool := func(name string, args map[string]any) map[string]any {
		return map[string]any{"name": name, "arguments": args}
	}
	return []map[string]any{
		tool("import", map[string]any{"resource": "monitors"}),
		tool("monitors_create", map[string]any{"type": "status", "url": "https://preview.example.com", "name": "Preview monitor",
			"auth_password": bbMCPArgSec, "request_headers": []any{map[string]any{"name": "X-Api-Key", "value": bbMCPArgSec}}}),
		tool("monitors_update", map[string]any{"id": bbMonitorID, "name": "Preview"}),
		tool("monitors_delete", map[string]any{"id": bbMonitorID}),
		tool("monitor-groups_create", map[string]any{"name": "Preview group"}),
		tool("monitor-groups_delete", map[string]any{"id": "61"}),
		tool("heartbeats_create", map[string]any{"name": "Preview heartbeat", "period": 60}),
		tool("heartbeats_update", map[string]any{"id": bbHeartbeat, "name": "Preview"}),
		tool("heartbeats_delete", map[string]any{"id": bbHeartbeat}),
		tool("heartbeat-groups_create", map[string]any{"name": "Preview group"}),
		tool("heartbeat-groups_delete", map[string]any{"id": "71"}),
		tool("incidents_acknowledge", map[string]any{"id": bbIncidentID}),
		tool("incidents_resolve", map[string]any{"id": bbIncidentID}),
		tool("incidents_delete", map[string]any{"id": bbIncidentID}),
		tool("policies_create", map[string]any{"name": "Preview policy"}),
		tool("policies_delete", map[string]any{"id": bbPolicyID}),
		tool("status-pages_create", map[string]any{"company-name": "Preview Co", "subdomain": "preview"}),
		tool("status-pages_delete", map[string]any{"id": bbStatusPage}),
		tool("status-page-sections_create", map[string]any{"status-page-id": bbStatusPage, "name": "Preview"}),
		tool("status-page-sections_delete", map[string]any{"status-page-id": bbStatusPage, "id": bbSectionID}),
		tool("status-page-resources_delete", map[string]any{"status-page-id": bbStatusPage, "id": "1"}),
	}
}

// mcpLocalWrites are mirrored tools that write only local files (no gate by
// design): their child CLI output passes the server's chokepoint too.
func mcpLocalWrites() []map[string]any {
	return []map[string]any{
		{"name": "export", "arguments": map[string]any{"resource": "policies", "format": "json"}},
		{"name": "sync", "arguments": map[string]any{"resources": "policies,incidents"}},
	}
}

func mcpBattery() []map[string]any {
	return append(append(mcpReads(), mcpWrites()...), mcpLocalWrites()...)
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
		if rel == "import.jsonl" || rel == "numeric.jsonl" || strings.HasPrefix(rel, "cfg"+string(filepath.Separator)) {
			return nil // the test's own inputs
		}
		scanned++
		fn(rel, data)
		return nil
	})
	return scanned
}

// A heartbeat's push URL is operator data, not a credential this program
// holds: it prints as the API returned it, from the CLI and MCP alike.
func TestHeartbeatPushURLIsNotWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.run(t, "heartbeats", "get", bbHeartbeat, "--json")
	out += e.run(t, "heartbeats", "list", "--json")
	mcp := e.mcpCalls(t, []map[string]any{{"name": "heartbeats_get", "arguments": map[string]any{"id": bbHeartbeat}}})
	for where, got := range map[string]string{"CLI": out, "MCP": mcp} {
		if !strings.Contains(got, bbPushURL) {
			t.Errorf("%s withheld the heartbeat push URL:\n%.1500s", where, got)
		}
		assertNoEcho(t, where, got)
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before it is followed, from the CLI, sync and MCP alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	redirect := e.run(t, "monitors", "get", bbRedirect, "--json")
	sv.mu.Lock()
	sv.redirectIncidents = true
	sv.mu.Unlock()
	syncOut := e.run(t, "sync", "--resources", "incidents", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "monitors_get", "arguments": map[string]any{"id": bbRedirect}},
		{"name": "incidents_list", "arguments": map[string]any{}},
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		if strings.HasSuffix(m, "/api/v2/monitors/"+bbRedirect) || strings.HasSuffix(m, "/api/v2/incidents") {
			redirected++
		}
	}
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if redirected < 4 {
		t.Fatalf("only %d request(s) met a redirect; the CLI, sync and MCP calls did not all run", redirected)
	}
	for where, out := range map[string]string{"monitors get": redirect, "sync": syncOut} {
		if !strings.Contains(out, "leave the configured Betterstack host") {
			t.Fatalf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
	}
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a read (a GET)
// while no call sets confirm, though the battery calls every write tool; each
// write answers with a preview.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	// The store holds the synced fleet first, so the novels have something to
	// read while the writes preview.
	e.run(t, "sync", "--json")
	sv.resetMethods()
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, "HQ-Record") {
		t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
	}
	replies := mcpReplies(out)
	for i, call := range mcpWrites() {
		id := len(mcpReads()) + i + 1
		r := replies[id]
		if !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") {
			t.Errorf("%s %v without confirm did not return a preview:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	// The novels answered from the synced store (a refusal would leave the
	// leak checks vacuous for them). group_health is left out: its rollup
	// names both monitor groups, the two halves of the split credential, so
	// it is withheld whole by design.
	for i, call := range mcpReads() {
		want := map[any]string{"fleet": "2 total", "triage": "HQ-Record outage", "down": "HQ-Record site",
			"coverage": "HQ-Record site", "oncall_gaps": "HQ-Record rota", "heartbeat_risk": "HQ-Record nightly backup"}[call["name"]]
		if r := replies[i+1]; want != "" && !strings.Contains(r, want) {
			t.Errorf("%s read nothing from the synced store (want %q):\n%.1500s", call["name"], want, r)
		}
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	for _, m := range sv.methods {
		if isWrite(m) {
			t.Errorf("the MCP server sent %s without confirm", m)
		}
	}
}

// Control for the test above: with confirm set to boolean true the same write
// reaches the API, so the gate previews rather than drops; the string "true"
// and the number 1 still preview. Both gates: an endpoint tool
// (incidents_resolve, monitors_delete) and the mirrored write (import). The
// mirror blocks import's required --input (a file the server would read for
// the caller), so a mirrored import can never send a record: its control is
// that confirm true runs the child as given (its own answer, without --dry-run
// or the gate's preview note) while every other value previews.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	write := func(confirm any) []map[string]any {
		return []map[string]any{
			{"name": "incidents_resolve", "arguments": map[string]any{"id": bbIncidentID, "confirm": confirm}},
			{"name": "monitors_delete", "arguments": map[string]any{"id": bbMonitorID, "confirm": confirm}},
			{"name": "import", "arguments": map[string]any{"resource": "monitors", "confirm": confirm}},
		}
	}
	previews := e.mcpPhases(t, write("true"), write(1))
	sv.mu.Lock()
	for _, m := range sv.methods {
		if isWrite(m) {
			t.Errorf("confirm \"true\" or 1 sent %s", m)
		}
	}
	sv.mu.Unlock()
	replies := mcpReplies(previews)
	for _, id := range []int{1, 2, 4, 5} {
		if !strings.Contains(replies[id], "confirmation_required") {
			t.Errorf("call %d did not preview:\n%.1000s", id, replies[id])
		}
	}
	for _, id := range []int{3, 6} {
		if !strings.Contains(replies[id], "Preview only: nothing was sent") {
			t.Errorf("mirrored import call %d did not preview:\n%.1000s", id, replies[id])
		}
	}
	out := e.mcpPhases(t, write(true))
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var resolve, del bool
	for _, m := range sv.methods {
		resolve = resolve || m == "POST /api/v2/incidents/"+bbIncidentID+"/resolve"
		del = del || m == "DELETE /api/v2/monitors/"+bbMonitorID
	}
	if !resolve || !del {
		t.Errorf("confirm true did not send the incident resolve and the monitor delete; requests: %v", sv.methods)
	}
	if r := mcpReplies(out)[3]; r == "" || strings.Contains(r, "Preview only") || strings.Contains(r, "dry run") || !strings.Contains(r, "input") {
		t.Errorf("confirm true did not run the mirrored import as given:\n%.1500s", r)
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, from a cold
// start with the token in the environment, the credential store or either
// config file key.
func TestDryRunSendsNothing(t *testing.T) {
	for _, mode := range []credMode{credEnv, credStored, credConfigFile, credConfigAccess} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		imp := e.importFile(t, "dry")
		for _, c := range append(calls(
			[]string{"monitors", "create", "--type", "status", "--url", "https://dry.example.com", "--name", "Dry", "--dry-run"},
			[]string{"monitors", "update", bbMonitorID, "--name", "Dry", "--dry-run"},
			[]string{"monitors", "delete", bbMonitorID, "--yes", "--dry-run"},
			[]string{"monitor-groups", "create", "--name", "Dry", "--dry-run"},
			[]string{"monitor-groups", "delete", "61", "--dry-run"},
			[]string{"heartbeats", "create", "--name", "Dry", "--period", "60", "--dry-run"},
			[]string{"heartbeats", "update", bbHeartbeat, "--name", "Dry", "--dry-run"},
			[]string{"heartbeats", "delete", bbHeartbeat, "--dry-run"},
			[]string{"heartbeat-groups", "create", "--name", "Dry", "--dry-run"},
			[]string{"heartbeat-groups", "delete", "71", "--dry-run"},
			[]string{"incidents", "acknowledge", bbIncidentID, "--by", "dry@example.com", "--dry-run"},
			[]string{"incidents", "resolve", bbIncidentID, "--dry-run"},
			[]string{"incidents", "delete", bbIncidentID, "--dry-run"},
			[]string{"policies", "create", "--name", "Dry", "--dry-run"},
			[]string{"policies", "delete", bbPolicyID, "--dry-run"},
			[]string{"status-pages", "create", "--company-name", "Dry", "--subdomain", "dry", "--dry-run"},
			[]string{"status-pages", "delete", bbStatusPage, "--dry-run"},
			[]string{"status-page-sections", "create", bbStatusPage, "--name", "Dry", "--dry-run"},
			[]string{"status-page-sections", "delete", bbStatusPage, bbSectionID, "--dry-run"},
			[]string{"status-page-resources", "delete", bbStatusPage, "1", "--dry-run"},
			[]string{"import", "monitors", "--input", imp, "--dry-run"},
			[]string{"monitors", "list", "--dry-run"},
		), cliCall{stdin: monitorBody("Dry stdin", "dry-auth"), args: []string{"monitors", "create", "--stdin", "--dry-run"}}) {
			// Each run reaches its preview, so the count below is not vacuous
			// (a flag error would send nothing either).
			// A --stdin body is held whole (stdin-body-declared), so its preview
			// is withheld by the kit: that notice is the preview's output.
			out := e.runIn(t, c.stdin, c.args...)
			if c.stdin != "" && strings.Contains(out, "[credkit: withheld from stdout") {
				continue
			}
			if !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") && !strings.Contains(out, "would ") {
				t.Errorf("%v (mode %s) printed no preview:\n%.1500s", c.args, mode, out)
			}
		}
		sv.mu.Lock()
		if len(sv.methods) != 0 {
			t.Errorf("--dry-run (mode %s) sent %v", mode, sv.methods)
		}
		sv.mu.Unlock()
		// Control: the same write without --dry-run reaches the fake.
		e.run(t, "monitors", "update", bbMonitorID, "--name", "Dry Co")
		sv.mu.Lock()
		if len(sv.methods) != 1 || sv.methods[0] != "PATCH /api/v2/monitors/"+bbMonitorID {
			t.Errorf("control (mode %s): want the one PATCH, got %v", mode, sv.methods)
		}
		sv.mu.Unlock()
	}
}

// credential-split-across-output-records over MCP: the fake splits the first
// credential it was sent across the two monitor-groups pages, one half each.
// One session reads both pages; the halves must never both reach the client.
func TestMCPSplitAcrossPagesWithheld(t *testing.T) {
	sv := echoServer(t)
	sv.splitGroups = true
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{{"name": "monitor-groups_list", "arguments": map[string]any{}}},
		[]map[string]any{{"name": "monitor-groups_list", "arguments": map[string]any{"page": "2"}}})
	if !strings.Contains(out, `"id":2`) {
		t.Fatalf("MCP session did not answer both pages:\n%.2000s", out)
	}
	sv.mu.Lock()
	page2 := sv.pages["/monitor-groups?page=2"]
	sv.mu.Unlock()
	if page2 == 0 {
		t.Fatalf("the second page was never asked for; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
}
