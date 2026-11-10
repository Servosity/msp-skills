// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package autotaskblackbox runs the built CLI and MCP binaries against an
// Autotask fake that echoes every credential it knows back in every encoding
// it can, whole and split across lines, adjacent values and pages, and checks
// what a reviewer cannot read off the code: the shared credential kit's
// wiring (autotask handfixes credkit-wiring, credkit-sqlite-every-handle,
// credkit-mcphttp-outermost, credential-fields-withheld, no-response-cache,
// api-origin-pinned, dry-run-sends-nothing, mcp-endpoint-write-confirm,
// mcp-mirror-write-confirm, zone-discovery-pinned). The kit withholds a
// whole unit (an output write, a file write, an MCP message, a statement)
// that carries a held value, and prints its notice in its place, so a fake
// that echoes proves nothing leaks, and the same fake answering clean
// (server.clean) proves each sink still writes its data. Ported from the
// atera 4.34.2 reprint's aterablackbox; only what the vendor shape forces
// changed:
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the two Autotask credentials every request
//     carries (ApiIntegrationCode, Secret) from the environment
//     (AUTOTASK_API_INTEGRATION_CODE, AUTOTASK_PSA_SECRET), from the
//     credential store (auth set-token saves the integration code) or from a
//     0600 config file's api_integration_code and psa_secret; a value the
//     configuration holds but does not send (a stored code the environment
//     overrides, a config file the environment overrides); the MCP HTTP
//     token; an MCP argument value (a Secret or ApiIntegrationCode
//     parameter); and the credentials a response carries (a webhook
//     secretKey, a client portal user's password, an integration vendor's
//     secret). The third header, UserName (AUTOTASK_PSA_USER_NAME, a config
//     file's psa_user_name, and AUTOTASK_USERNAME or zone --user, which only
//     zone reads), is the API user's login name, an identifier the kit does
//     not hold (user_name_identifier_test.go): every request still carries
//     it, the API user's own Resource repeats it, and it prints;
//   - no file the program writes, except the 0600 credential store and the
//     config file a credential was read from (both credential sources), holds
//     one either (the local store, receipts, exports, delivered files, the
//     feedback log, the learn journal, profiles, teach logs);
//   - no credential leaves for a host other than the configured one by
//     redirect, and nothing at all leaves for a non-loopback host (every
//     process runs behind a recording proxy that answers 502);
//   - the MCP server sends a write (a POST that is not a /query, PATCH, PUT
//     or DELETE) only when the call sets confirm to the boolean true, and
//     --dry-run sends nothing at all.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package autotaskblackbox

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
	"strings"
	"sync"
	"testing"
	"time"
)

// The integration codes are shaped like Autotask's tracking identifiers
// (uppercase letters and digits): a code spelled from English words
// ("...IntegrationCode...") is spelled again by ordinary output across MCP
// results and withheld (P2 credential-spelled-from-output-vocabulary).
const (
	bbCode       = "AT7FB3189BQ6U3SZXW4O1NKD2Q"                      // AUTOTASK_API_INTEGRATION_CODE (sent as ApiIntegrationCode); tracking-identifier shaped
	bbUser       = "psa-api-user-3c9e1b7a5d2f80aa4e@psa-api.example" // AUTOTASK_PSA_USER_NAME (sent as UserName; an identifier, not held)
	bbSecret     = "Zq7Kx2Lr9" + "Wm4Tk8Pv3Hs6N"                     // AUTOTASK_PSA_SECRET (sent as Secret)
	bbStored     = "ST71C3E9B05D2A4F68E1QKZW7"                       // auth set-token (stdin): the integration code in the credential store
	bbFileCode   = "CF5E2A8C0D4B6F19RTYU3PLMX"                       // api_integration_code in a 0600 config file named by AUTOTASK_CONFIG
	bbFileUser   = "config-file-user-2b9d7e4a1c63@psa-api.example"   // psa_user_name in that file (an identifier, not held)
	bbFileSecret = "Fs9A4c1E7" + "b3D50Qw8Rt2Yx"                     // psa_secret in that file
	bbZoneUser   = "zone-user-6f1a0c3e9b72d4@psa-api.example"        // AUTOTASK_USERNAME (zone reads it; never sent to the API; an identifier, not held)
	bbArgvSec    = "-argvSecret-91c3e7a5"                            // an unknown --secret flag on the command line (starts with "-")
	bbMCPArgSec  = "mcpArgSecret-81d3c6f0"                           // a credential-named argument an MCP call previews (never sent)
	bbShortResp  = "Sh0rtPw"                                         // a 7-character client portal password a response carries
	bbBOMResp    = "BomRespSecret-6a1c"                              // an integration vendor secret in a BOM-prefixed response
	bbGzipResp   = "GzipRespToken-4f2a9c1e"                          // a webhook secretKey in a gzip-encoded response
	bbNewSecret  = "newWebhookSecretKey-0b7e3d5a9c1f"                // the secretKey a create response repeats
	bbWebhookKey = "whKey-cQ7" + "vZr2k6d0a2e8c4f71Wm"               // the secretKey a webhook-shaped record carries (no piece of the field name: kit finding 7)
	bbHTTPToken  = "mcpHttpToken-0123456789abcdefABCDEF"             // AUTOTASK_MCP_HTTP_TOKEN
	bbTicketID   = "5150"                                            // a ticket ID
	bbCompanyID  = "77"                                              // a company ID
	bbResourceID = "29683481"                                        // the API user's Resource ID
	bbTimeID     = "9"                                               // a time entry ID
	bbRedirect   = "9911"                                            // a ticket ID the fake redirects off-host
	bbDenied     = "40199"                                           // a ticket or contact ID the fake answers 401
	bbPlainUser  = "hq.tech2"                                        // a Resource userName that is no credential: it must print
	apiBase      = "/atservicesrest/V1.0"
)

func secrets() []string {
	return []string{bbCode, bbSecret, bbStored, bbFileCode, bbFileSecret, bbArgvSec,
		bbMCPArgSec, bbShortResp, bbBOMResp, bbGzipResp, bbNewSecret, bbWebhookKey, bbHTTPToken}
}

var bbCLI, bbMCP string

// bbHeaderFlags is set when every generated endpoint command declares the
// three auth headers as required flags (--api-integration-code,
// --user-name, --secret): the 4.34.2 render turned the spec's per-operation
// header parameters into required flags, so such a command refuses to run on
// the configured credentials alone (TestGeneratedCommandsRunOnTheConfiguredCredentials).
// The battery then passes the values its mode sends on the command line, which
// is what an operator of this tree has to do, so the argv path is covered too.
var bbHeaderFlags bool

// proxyURL is a recording proxy every process runs behind (HTTPS_PROXY,
// HTTP_PROXY): a request to any non-loopback host reaches it and is answered
// 502, so nothing leaves the machine.
var proxyURL string

var proxySeen struct {
	sync.Mutex
	reqs []string
}

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	// The tests wait on the CLI and MCP processes they start, so GOMAXPROCS
	// (go test's -parallel default) is not their concurrency: on a 4-vCPU
	// Windows runner, process starts and file writes left the CPU idle while
	// four groups ran and the rest waited 5 minutes in line (PR #565 passed
	// go test's 10 minute limit). Run twice as many unless -parallel was given.
	explicit := false
	flag.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "test.parallel" })
	if !explicit {
		_ = flag.Set("test.parallel", fmt.Sprint(max(8, 2*runtime.GOMAXPROCS(0))))
	}
	dir, err := os.MkdirTemp("", "autotask-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	// AUTOTASK_BLACKBOX_BIN_DIR runs the suite against binaries built elsewhere
	// (a seen-failing run with the kit's wiring disabled).
	bins := os.Getenv("AUTOTASK_BLACKBOX_BIN_DIR")
	if bins == "" {
		bins = dir
	}
	bbCLI = filepath.Join(bins, "autotask-cli"+exe)
	bbMCP = filepath.Join(bins, "autotask-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/autotask-cli", bbMCP: "../../cmd/autotask-mcp"} {
		if bins != dir {
			break
		}
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	help, _ := exec.Command(bbCLI, "tickets", "query-item", "--help").CombinedOutput()
	bbHeaderFlags = bytes.Contains(help, []byte("--api-integration-code")) && bytes.Contains(help, []byte("--user-name"))
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxySeen.Lock()
		proxySeen.reqs = append(proxySeen.reqs, r.Method+" "+r.Host+" "+r.RequestURI)
		proxySeen.Unlock()
		w.WriteHeader(http.StatusBadGateway)
	}))
	proxyURL = proxy.URL
	code := m.Run()
	proxy.Close()
	_ = os.RemoveAll(dir)
	fmt.Fprintf(os.Stderr, "autotaskblackbox: %d CLI runs, %d MCP tools/call requests, header flags on argv: %v\n",
		cliRuns.n, mcpCallsSent.n, bbHeaderFlags)
	os.Exit(code)
}

// echoForms is every encoding of s the server sends back whole: as written,
// inside other text, as each auth header line, base64 (standard and
// URL-safe), hex, base32, ASCII85, percent-encoded, PEM-armored, and
// gzip-compressed then base64-encoded (the way a JSON string carries
// compressed bytes).
func echoForms(s string) []string {
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return []string{
		s,
		"x" + s + "x",
		"ApiIntegrationCode: " + s,
		"UserName: " + s,
		"Secret: " + s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
		string(pem.EncodeToMemory(&pem.Block{Type: "AUTOTASK KEY", Bytes: []byte(s)})),
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
	if u, err := url.QueryUnescape(r.URL.RawQuery); err == nil {
		seen += "\n" + u
	}
	var out []string
	for _, s := range secrets() {
		if strings.Contains(seen, s) {
			out = append(out, s)
		}
	}
	return out
}

// creds is the three Autotask auth header values one request carries.
type creds struct{ code, user, secret string }

func (c creds) String() string { return c.code + " | " + c.user + " | " + c.secret }

type server struct {
	url     string
	caFile  string
	mu      sync.Mutex
	offHost int
	methods []string      // API requests, "METHOD /path"
	headers map[creds]int // the (ApiIntegrationCode, UserName, Secret) triples the API received
	auths   int           // requests that carried an Authorization header (Autotask never takes one)
	// redirectTickets sends the tickets query (the syncable resource every
	// ticket novel reads) to the other host too, so sync meets a redirect.
	redirectTickets bool
	// clean answers every route without repeating anything the request
	// carried, and without repeating a response credential outside its own
	// credential-named field: the controls that prove each sink writes data.
	clean bool
}

// setClean switches the fake between echoing and answering clean.
func (sv *server) setClean(clean bool) {
	sv.mu.Lock()
	sv.clean = clean
	sv.mu.Unlock()
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

// isWrite reports whether "METHOD /path" changes data: Autotask reads are
// GETs and POST /<Entity>/query (a filter in the body).
func isWrite(m string) bool {
	method, path, _ := strings.Cut(m, " ")
	if method == http.MethodGet {
		return false
	}
	return !(method == http.MethodPost && strings.HasSuffix(path, "/query"))
}

// queryFilters is the filter array of an Autotask query: the POST body's, or
// the GET form's search parameter.
func queryFilters(r *http.Request, body []byte) []map[string]any {
	var q struct {
		Filter []map[string]any `json:"filter"`
	}
	if r.Method == http.MethodGet {
		_ = json.Unmarshal([]byte(r.URL.Query().Get("search")), &q)
	} else {
		_ = json.Unmarshal(body, &q)
	}
	return q.Filter
}

// walkFrom is the id a later page of sync's id walk starts after (the
// filter's "gt" on id), or -1 on the first page.
func walkFrom(r *http.Request, body []byte) float64 {
	for _, f := range queryFilters(r, body) {
		if f["op"] == "gt" && strings.EqualFold(fmt.Sprint(f["field"]), "id") {
			switch v := f["value"].(type) {
			case float64:
				return v
			case string:
				var n float64
				if _, err := fmt.Sscan(v, &n); err == nil {
					return n
				}
			}
			return 0
		}
	}
	return -1
}

// listBody is an Autotask query envelope (spec.json QueryResultModel) of one
// page; sync's id walk asks for the next page with a "gt" filter, and every
// later page is empty, so every walk stops.
func listBody(r *http.Request, body []byte, items ...string) string {
	if walkFrom(r, body) >= 0 {
		items = nil
	}
	return fmt.Sprintf(`{"items":[%s],"pageDetails":{"count":%d,"requestCount":500,"prevPageUrl":null,"nextPageUrl":null}}`,
		strings.Join(items, ","), len(items))
}

// fieldsBody is an entityInformation/fields answer (picklist reads it): a
// status and a priority picklist, the first field carrying the echo.
func fieldsBody(echo string) string {
	pick := func(name string, vals ...string) string {
		var vs []string
		for i, v := range vals {
			vs = append(vs, fmt.Sprintf(`{"value":"%d","label":%q,"isActive":true,"isDefaultValue":%v,"sortOrder":%d,"parentValue":""}`, i+1, v, i == 0, i+1))
		}
		return fmt.Sprintf(`{"name":%q,"dataType":"integer","length":0,"isRequired":true,"isReadOnly":false,"isQueryable":true,"isReference":false,`+
			`"referenceEntityType":"","isPickList":true,"picklistValues":[%s],"picklistParentValueField":"","isSupportedWebhookField":false`, name, strings.Join(vs, ","))
	}
	return fmt.Sprintf(`{"fields":[%s,"label":"HQ-Record status","echo":%s},%s}],"echo":%s}`, pick("status", "New", "In Progress", "Waiting", "Escalated", "Complete"),
		echo, pick("priority", "High", "Medium", "Low"), echo)
}

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{headers: map[creds]int{}}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`{"items":[],"pageDetails":{"count":0,"requestCount":500,"prevPageUrl":null,"nextPageUrl":null}}`))
	}))
	now := time.Now().UTC()
	stamp := func(d time.Duration) string { return now.Add(d).Format("2006-01-02T15:04:05.000Z") }
	recent, old, soon, past := stamp(-time.Hour), stamp(-90*24*time.Hour), stamp(10*24*time.Hour), stamp(-300*24*time.Hour)
	overdue := stamp(-48 * time.Hour)
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		sv.mu.Lock()
		clean := sv.clean
		if clean {
			sent = nil
		}
		echo := echoObject(sent)
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		sv.headers[creds{r.Header.Get("ApiIntegrationCode"), r.Header.Get("UserName"), r.Header.Get("Secret")}]++
		if r.Header.Get("Authorization") != "" {
			sv.auths++
		}
		redirectTickets := sv.redirectTickets
		sv.mu.Unlock()
		// repeat is a response credential repeated under a field that is not
		// credential-named; the clean fake repeats nothing.
		repeat := func(v string) string {
			if clean {
				return "HQ note"
			}
			return v
		}
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimPrefix(r.URL.Path, apiBase)
		get := r.Method == http.MethodGet
		query := strings.HasSuffix(p, "/query")
		list := func(items ...string) string { return listBody(r, body, items...) }
		apiUser := r.Header.Get("UserName") // the API user's own Resource repeats its UserName, an identifier
		switch {
		case strings.HasPrefix(p, "/Invoices/") || strings.Contains(p, "/"+bbDenied):
			// Rejected (every invoice path, a denied ticket or contact): the
			// error repeats what it was sent. Invoices are the exportable,
			// syncable resource no novel reads, so sync and export meet a 401.
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"errors":["invalid credentials %s","user %s"],"echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/Tickets/"+bbRedirect || (redirectTickets && p == "/Tickets/query"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && p == "/Tickets/"+bbTicketID:
			// tickets query-item: the echo split across values, lines and spaces.
			fmt.Fprintf(w, `{"item":{"id":%s,"ticketNumber":"T20261028.0001","title":"HQ-Record printer","companyID":%s,"status":1,"split":%s}}`,
				bbTicketID, bbCompanyID, splitObject(sent))
		case query && p == "/Tickets/query":
			// tickets query, sync, export: gzip-encoded, with a webhook
			// secretKey, the echo, and a record whose identifier repeats the
			// credential it was sent. The ticket is open, unassigned, past its
			// due date and old: every ticket novel reports it.
			items := []string{fmt.Sprintf(`{"id":%s,"ticketNumber":"T20261028.0001","title":"HQ-Record printer down","status":1,"priority":1,`+
				`"queueID":8,"companyID":%s,"contactID":41,"contractID":null,"assignedResourceID":null,"createDate":%q,"lastActivityDate":%q,`+
				`"dueDateTime":%q,"firstResponseDueDateTime":%q,"firstResponseDateTime":null,"estimatedHours":2,"secretKey":%q,"echo":%s}`,
				bbTicketID, bbCompanyID, old, recent, overdue, overdue, bbGzipResp, echoObject(append(sent, repeat(bbGzipResp))))}
			if len(sent) > 0 {
				items = append(items, fmt.Sprintf(`{"id":%q,"title":"Echo-ID","status":1,"priority":2,"createDate":%q}`, sent[0], recent))
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			_, _ = io.WriteString(zw, list(items...))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case query && p == "/Companies/query":
			// companies query (and company-360): BOM-prefixed, carrying an
			// integration vendor secret, repeated under a field that is not
			// credential-named.
			fmt.Fprint(w, "\xef\xbb\xbf"+list(fmt.Sprintf(`{"id":%s,"companyName":"HQ-Record","companyType":1,"isActive":true,"createDate":%q,`+
				`"lastActivityDate":%q,"ownerResourceID":%s,"secret":%q,"additionalAddressInformation":%q}`, bbCompanyID, past, recent, bbResourceID, bbBOMResp, repeat(bbBOMResp))))
		case query && p == "/ConfigurationItems/query":
			// A webhook-shaped record (TicketWebhooks and its kin carry a
			// secretKey) with no company, so data-gaps reports it.
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":31,"companyID":null,"referenceTitle":"HQ-Record switch","serialNumber":"SN-31","isActive":true,`+
				`"secretKey":%q,"webhookUrl":"https://hooks.test/at","echo":%s}`, bbWebhookKey, echo)))
		case query && p == "/Contacts/query":
			// A client portal user's password (a short credential), repeated
			// under a field that is not credential-named.
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":41,"companyID":%s,"firstName":"HQ","lastName":"Record-Contact","emailAddress":"hq.contact@example.com",`+
				`"isActive":1,"password":%q,"note":%q,"echo":%s}`, bbCompanyID, bbShortResp, repeat(bbShortResp), echo)))
		case query && p == "/Resources/query":
			// The API user is a Resource: its userName is the UserName the
			// request carried, an identifier that prints. The other Resource's
			// userName is no credential either and must print.
			fmt.Fprint(w, list(
				fmt.Sprintf(`{"id":%s,"userName":%q,"firstName":"HQ","lastName":"Tech","email":"tech@example.com","isActive":true,"resourceType":"Employee","echo":%s}`,
					bbResourceID, apiUser, echo),
				fmt.Sprintf(`{"id":29683482,"userName":%q,"firstName":"HQ2","lastName":"Record-Tech","email":"tech2@example.com","isActive":true,"resourceType":"Employee"}`,
					bbPlainUser)))
		case query && p == "/Contracts/query":
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":12,"companyID":%s,"contractName":"HQ-Record retainer","contractType":6,"contractCategory":1,"status":1,`+
				`"startDate":%q,"endDate":%q,"estimatedHours":40,"echo":%s}`, bbCompanyID, past, soon, echo)))
		case query && p == "/ContractBlocks/query":
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":3,"contractID":12,"hours":40,"hoursApproved":36,"startDate":%q,"endDate":%q,"isPaid":true,"status":1,"echo":%s}`,
				past, soon, echo)))
		case query && p == "/TimeEntries/query":
			// Approved, uninvoiced time on the ticket, and an entry on no ticket.
			fmt.Fprint(w, list(
				fmt.Sprintf(`{"id":%s,"ticketID":%s,"taskID":null,"resourceID":%s,"contractID":12,"hoursWorked":1.5,"hoursToBill":1.5,"dateWorked":%q,`+
					`"startDateTime":%q,"isNonBillable":false,"billingApprovalDateTime":%q,"billingApprovalLevelMostRecent":1,"summaryNotes":"HQ-Record work","echo":%s}`,
					bbTimeID, bbTicketID, bbResourceID, recent, recent, recent, echo),
				fmt.Sprintf(`{"id":10,"ticketID":null,"taskID":null,"resourceID":%s,"hoursWorked":0.5,"hoursToBill":0.5,"dateWorked":%q,"summaryNotes":"HQ-Record unlinked"}`,
					bbResourceID, recent)))
		case query && p == "/Projects/query":
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":21,"projectName":"HQ-Record migration","companyID":%s,"status":1,"startDateTime":%q,"endDateTime":%q,`+
				`"lastActivityDateTime":%q,"projectLeadResourceID":%s,"echo":%s}`, bbCompanyID, past, old, old, bbResourceID, echo)))
		case query && p == "/Tasks/query":
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":22,"projectID":21,"title":"HQ-Record cutover","status":1,"startDateTime":%q,"endDateTime":%q,`+
				`"assignedResourceID":%s,"estimatedHours":4,"remainingHours":4,"lastActivityDateTime":%q,"echo":%s}`, past, old, bbResourceID, old, echo)))
		case query && p == "/Opportunities/query":
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":23,"companyID":%s,"title":"HQ-Record renewal","amount":1200,"probability":50,"status":1,"stage":1,`+
				`"createDate":%q,"lastActivity":%q,"projectedCloseDate":%q,"echo":%s}`, bbCompanyID, recent, recent, soon, echo)))
		case query && p == "/Holidays/query":
			// The first credential it was sent, split across two pages of
			// sync's id walk: each half is a record of its own, on its own page.
			a, b := "hol-a", "hol-b"
			if len(sent) > 0 {
				a, b = halves(sent[0])
			}
			switch from := walkFrom(r, body); {
			case from < 0:
				items := make([]string, 0, 100)
				for i := 1; i < 100; i++ {
					items = append(items, fmt.Sprintf(`{"id":%d,"holidayName":"HQ-Record day %d","holidayDate":%q}`, i, i, past))
				}
				items = append(items, fmt.Sprintf(`{"id":100,"holidayName":%q,"holidayDate":%q}`, a, past))
				fmt.Fprintf(w, `{"items":[%s],"pageDetails":{"count":100,"requestCount":100,"prevPageUrl":null,"nextPageUrl":null}}`, strings.Join(items, ","))
			case from == 100:
				fmt.Fprintf(w, `{"items":[{"id":101,"holidayName":%q,"holidayDate":%q}],"pageDetails":{"count":1,"requestCount":100,"prevPageUrl":null,"nextPageUrl":null}}`, b, past)
			default:
				fmt.Fprint(w, `{"items":[],"pageDetails":{"count":0,"requestCount":100,"prevPageUrl":null,"nextPageUrl":null}}`)
			}
		case get && strings.HasSuffix(p, "/entityInformation/fields"):
			fmt.Fprint(w, fieldsBody(echo))
		case get && strings.HasSuffix(p, "/entityInformation/userDefinedFields"):
			fmt.Fprintf(w, `{"fields":[],"echo":%s}`, echo)
		case get && strings.HasSuffix(p, "/entityInformation"):
			fmt.Fprintf(w, `{"info":{"name":"HQ-Record","canCreate":true,"canDelete":true,"canQuery":true,"canUpdate":true,"hasUserDefinedFields":false},"echo":%s}`, echo)
		case strings.HasSuffix(p, "/query/count"):
			fmt.Fprintf(w, `{"queryCount":1,"echo":%s}`, echo)
		case r.Method == http.MethodPost && p == "/ConfigurationItems":
			// A create answers {"itemId":N}; this one also repeats a
			// secretKey (a webhook created with its key), beyond the documented
			// shape: a response credential is held whatever the shape.
			fmt.Fprintf(w, `{"itemId":32,"secretKey":%q,"echo":%s}`, bbNewSecret, echo)
		case !get && !query:
			// Every other write (create, update, patch, delete) answers its
			// item id and echoes what it was sent.
			fmt.Fprintf(w, `{"itemId":123,"echo":%s}`, echo)
		case query:
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":1,"name":"HQ-Record","echo":%s}`, echo)))
		default:
			// GET /<Entity>/{id} and anything else: one item.
			fmt.Fprintf(w, `{"item":{"id":1,"name":"HQ-Record","echo":%s}}`, echo)
		}
	}))
	sv.url = srv.URL + apiBase
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// sawHeader reports how many API requests carried exactly the triple c, and
// the distinct triples the fake received.
func (sv *server) sawHeader(c creds) (int, []creds) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var all []creds
	for h := range sv.headers {
		all = append(all, h)
	}
	return sv.headers[c], all
}

func (sv *server) resetMethods() {
	sv.mu.Lock()
	sv.methods = nil
	sv.mu.Unlock()
}

type bbEnv struct {
	home string
	env  []string
	mode credMode
}

// credMode is where the process gets its three auth header values.
type credMode int

const (
	credStored        credMode = iota // the integration code stored with auth set-token; UserName and Secret from the environment
	credEnv                           // all three from the environment
	credEnvZoneUser                   // the environment, and AUTOTASK_USERNAME (zone's name for the user, an identifier) set to another user
	credEnvOverStored                 // a stored code and AUTOTASK_API_INTEGRATION_CODE: the environment wins, the stored code is held unused
	credConfigFile                    // all three in a 0600 config file named by AUTOTASK_CONFIG
	credEnvOverConfig                 // that config file and the environment: each environment value wins, the file's code and Secret are held unused
	credConfigMixed                   // the integration code in the config file, UserName and Secret from the environment
)

func (m credMode) String() string {
	return [...]string{"stored code", "env", "env and AUTOTASK_USERNAME", "env over stored", "config file", "env over config file",
		"config code with env user and secret"}[m]
}

// sent is the triple the mode sends; held are the values it holds but never sends.
func (m credMode) sent() creds {
	switch m {
	case credStored:
		return creds{bbStored, bbUser, bbSecret}
	case credConfigFile:
		return creds{bbFileCode, bbFileUser, bbFileSecret}
	case credConfigMixed:
		return creds{bbFileCode, bbUser, bbSecret}
	}
	return creds{bbCode, bbUser, bbSecret}
}

func (m credMode) held() []string {
	switch m {
	case credEnvOverStored:
		return []string{bbStored, bbHTTPToken}
	case credEnvOverConfig:
		return []string{bbFileCode, bbFileSecret, bbHTTPToken}
	}
	return []string{bbHTTPToken}
}

// baseEnv is a clean environment against the fake, behind the recording proxy.
func baseEnv(home string, sv *server) []string {
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"HTTPS_PROXY=" + proxyURL, "HTTP_PROXY=" + proxyURL, "https_proxy=" + proxyURL, "http_proxy=" + proxyURL, "NO_PROXY=", "no_proxy=",
		"AUTOTASK_BASE_URL=" + sv.url, "AUTOTASK_CA_CERT=" + sv.caFile,
		"AUTOTASK_CLI_PATH=" + bbCLI, "AUTOTASK_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
}

// newEnv builds a clean environment against the fake with the credentials
// mode names; the stored modes run auth set-token first.
func newEnv(t *testing.T, sv *server, mode credMode) *bbEnv {
	t.Helper()
	home := t.TempDir()
	e := &bbEnv{home: home, env: baseEnv(home, sv), mode: mode}
	writeConfig := func(body string) {
		dir := filepath.Join(home, "cfg")
		_ = os.MkdirAll(dir, 0o700)
		cfg := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		e.env = append(e.env, "AUTOTASK_CONFIG="+cfg)
	}
	userSecret := []string{"AUTOTASK_PSA_USER_NAME=" + bbUser, "AUTOTASK_PSA_SECRET=" + bbSecret}
	fileAll := fmt.Sprintf("api_integration_code = %q\npsa_user_name = %q\npsa_secret = %q\n", bbFileCode, bbFileUser, bbFileSecret)
	switch mode {
	case credStored:
		e.env = append(e.env, userSecret...)
		e.storeToken(t, bbStored)
	case credEnv:
		e.env = append(e.env, append(userSecret, "AUTOTASK_API_INTEGRATION_CODE="+bbCode)...)
	case credEnvZoneUser:
		e.env = append(e.env, append(userSecret, "AUTOTASK_API_INTEGRATION_CODE="+bbCode, "AUTOTASK_USERNAME="+bbZoneUser)...)
	case credEnvOverStored:
		e.env = append(e.env, userSecret...)
		e.storeToken(t, bbStored)
		e.env = append(e.env, "AUTOTASK_API_INTEGRATION_CODE="+bbCode)
	case credConfigFile:
		writeConfig(fileAll)
	case credEnvOverConfig:
		writeConfig(fileAll)
		e.env = append(e.env, append(userSecret, "AUTOTASK_API_INTEGRATION_CODE="+bbCode)...)
	case credConfigMixed:
		writeConfig(fmt.Sprintf("api_integration_code = %q\n", bbFileCode))
		e.env = append(e.env, userSecret...)
	}
	return e
}

// hdr is the three auth header flags with the values the mode sends, when
// the generated endpoint commands require them (bbHeaderFlags); none
// otherwise.
func (e *bbEnv) hdr() []string {
	if !bbHeaderFlags {
		return nil
	}
	c := e.mode.sent()
	return []string{"--api-integration-code", c.code, "--user-name", c.user, "--secret", c.secret}
}

// api is a generated endpoint command line: args, then hdr.
func (e *bbEnv) api(args ...string) []string { return append(args, e.hdr()...) }

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
	case <-time.After(300 * time.Second):
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
	deadline := time.Now().Add(240 * time.Second)
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
// (the local store's file only, whose pages hold unrelated text side by side;
// a held value split across two stored records is
// TestStoreSplitAcrossRecordsWithheld's).
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
// import write path (POST /Tickets per record).
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(e.home, "import.jsonl")
	if err := os.WriteFile(p, []byte(`{"title":"Imported `+tok+`","description":"HQ import","companyID":77,"status":1,"priority":2}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// allFilter is a query filter that matches every record.
const allFilter = `[{"op":"gte","field":"id","value":0}]`

// novels is every novel command, each reading the local store after sync
// (picklist reads the field definitions live).
func novels() [][]string {
	return [][]string{
		{"account-brief", bbCompanyID, "--since", "30d", "--json"},
		{"company-360", bbCompanyID, "--json"},
		{"company-360", bbCompanyID},
		{"contract-burn", "--json"},
		{"data-gaps", "--json"},
		{"orphans", "--json"},
		{"project-health", "--json"},
		{"reconcile", "--json"},
		{"retainer", "--json"},
		{"sla-breaches", "--json"},
		{"sla-breaches"},
		{"stale", "--days", "14", "--json"},
		{"stale", "--days", "30", "--entity", "projects", "--json"},
		{"ticket-aging", "--by", "priority", "--json"},
		{"triage", "--json"},
		{"triage"},
		{"unbilled", "--json"},
		{"workload", "--json"},
		{"load", "--json"},
		{"since", "7d", "--json"},
		{"picklist", "Tickets", "status", "--json"},
		{"tail", "tickets", "--follow=false", "--json"},
	}
}

// battery runs every output path; tok is a credential the process holds
// (the integration code it sends: the environment's, the stored one or a
// config file's). It comes in groups, each run in its own home against its
// own fake so the groups run in parallel; a group that reads the local store
// syncs first.
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) [][][]string {
	imp := e.importFile(t, tok)
	a := e.api
	sync := []string{"sync", "--json"}
	nv := novels()
	return [][][]string{{
		// Every output mode.
		a("tickets", "query", "--filter", allFilter, "--json"),
		a("tickets", "query", "--filter", allFilter),
		a("tickets", "query", "--filter", allFilter, "--csv"),
		a("tickets", "query", "--filter", allFilter, "--plain"),
		a("tickets", "query", "--filter", allFilter, "--agent"),
		a("tickets", "query", "--filter", allFilter, "--json", "--receipt"),
		a("tickets", "query", "--filter", allFilter, "--json", "--audit-dir", filepath.Join(e.home, "audit")),
		a("tickets", "query", "--filter", allFilter, "--json", "--deliver", "file:"+deliverFile),
		a("tickets", "url-parameter-query", "--search", `{"filter":`+allFilter+`}`, "--json"),
	}, {
		// Every response shape: BOM-prefixed (an integration vendor secret), a
		// webhook secretKey, a short client portal password, the API user's
		// Resource, split echo, 401 echo, refused redirect, picklist fields, a
		// count, a create that repeats a secretKey.
		a("companies", "query", "--filter", allFilter, "--json"),
		a("configuration-items", "query", "--filter", allFilter, "--json"),
		a("configuration-items", "query", "--filter", allFilter),
		a("contacts", "query", "--filter", allFilter, "--json"),
		a("resources", "query", "--filter", allFilter, "--json"),
		a("resources", "query", "--filter", allFilter),
		a("tickets", "query-item", bbTicketID, "--json"),
		a("tickets", "query-item", bbDenied, "--json"),
		a("contacts", "query-item", bbDenied, "--json"),
		a("invoices", "query", "--filter", allFilter, "--json"),
		a("tickets", "query-item", bbRedirect, "--json"),
		a("tickets", "query-field-definitions", "--json"),
		a("tickets", "query-count", "--filter", allFilter, "--json"),
		a("holidays", "query", "--filter", allFilter, "--json"),
		a("configuration-items", "create-entity", "--reference-title", "HQ switch", "--company-id", bbCompanyID, "--json", "--yes"),
	}, {
		// Writes the fake echoes, with and without --yes (a CLI write without
		// --yes is the work order's P2; it must never leak either way).
		a("tickets", "create-entity", "--title", "HQ "+tok, "--description", "d", "--company-id", bbCompanyID, "--status", "1", "--json", "--yes"),
		a("tickets", "create-entity", "--title", "HQ "+tok, "--description", "d", "--company-id", bbCompanyID, "--status", "1", "--json"),
		a("companies", "patch-entity", "--id", bbCompanyID, "--company-name", "HQ Co", "--additional-address-information", "HQ "+tok, "--json", "--yes"),
		a("tickets", "patch-entity", "--id", bbTicketID, "--status", "5", "--json"),
		a("tickets", "notes", "ticket-child-create-entity", bbTicketID, "--title", "HQ", "--description", "HQ "+tok, "--json", "--yes"),
		a("time-entries", "delete-entity", bbTimeID, "--yes", "--json"),
		a("companies", "alerts", "company-child-delete-entity", bbCompanyID, "3", "--json"),
		a("contracts", "update-entity", "--id", "12", "--contract-name", "HQ "+tok, "--json", "--yes"),
		{"import", "tickets", "--input", imp, "--json"},
	}, {
		// The local store and exports.
		sync,
		{"export", "tickets", bbTicketID, "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"export", "tickets", "--format", "json"},
		{"export", "resources", "--format", "json"},
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
		{"analytics", "--type", "tickets", "--group-by", "title", "--json"},
	},
		// The novel commands, each reading the local store after sync.
		append([][]string{sync}, nv[:len(nv)/2]...),
		append([][]string{sync}, nv[len(nv)/2:]...),
		{
			// A credential on the command line: an unknown credential-named
			// flag whose value starts with "-", an unknown flag, a positional,
			// feedback text.
			a("configuration-items", "create-entity", "--reference-title", "argv", "--secret="+bbArgvSec, "--json", "--yes"),
			append(a("tickets", "query", "--filter", allFilter, "--json"), "--"+tok),
			a("tickets", "query-item", tok, "--json"),
			{"feedback", "noted the key " + tok + " in a ticket"},
			// Previews: a dry run sends nothing.
			a("companies", "patch-entity", "--id", bbCompanyID, "--company-name", "Echo "+tok, "--dry-run"),
			a("tickets", "create-entity", "--title", "Echo "+tok, "--description", "d", "--dry-run"),
			a("configuration-items", "create-entity", "--reference-title", "Echo "+tok, "--dry-run"),
			a("tickets", "patch-entity", "--id", bbTicketID, "--status", "5", "--dry-run"),
			{"import", "tickets", "--input", imp, "--dry-run"},
		}, {
			// Learnings, playbooks, teach logs and profiles land in local files.
			{"teach", "--query", "find tickets " + tok, "--resource", bbTicketID, "--resource-type", "tickets",
				"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
			{"teach", "--query", "find " + tok, "--resource", tok, "--resource-type", "tickets"}, // a held resource id: refused, logged to teach.log
			{"teach-playbook", "--query", "tickets with " + tok, "--notes", "use " + tok},
			{"profile", "save", "held", "--description", "uses " + tok, "--select", "id"},
			// zone repeats the user it resolves for; its bootstrap host is reached
			// only through the recording proxy, which answers 502.
			{"zone", "--no-save", "--json"},
			{"doctor"},
			{"auth", "status"},
		}}
}

// eachGroup starts fn for each battery group as a parallel subtest of t, each
// in its own home against its own echoing fake; t's caller waits for them.
func eachGroup(t *testing.T, mode credMode, fn func(t *testing.T, sv *server, e *bbEnv, group [][]string)) {
	t.Helper()
	n := len(battery(t, &bbEnv{home: t.TempDir()}, "", "x"))
	for g := 0; g < n; g++ {
		t.Run(fmt.Sprintf("group-%d", g), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			fn(t, sv, e, battery(t, e, filepath.Join(e.home, "deliver.json"), mode.sent().code)[g])
		})
	}
}

// mcpReads are read-only tools across every response shape the fake has, and
// every novel tool.
func mcpReads() []map[string]any {
	none := map[string]any{}
	filter := []any{map[string]any{"op": "gte", "field": "id", "value": 0}}
	read := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "autotask_read", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		read("tickets.query", map[string]any{"filter": filter}),
		read("companies.query", map[string]any{"filter": filter}),
		read("configuration-items.query", map[string]any{"filter": filter}),
		read("contacts.query", map[string]any{"filter": filter}),
		read("resources.query", map[string]any{"filter": filter}),
		read("tickets.query-item", map[string]any{"id": bbTicketID}),
		read("tickets.query-item", map[string]any{"id": bbDenied}),
		read("tickets.query-field-definitions", none),
		read("tickets.url-parameter-query", map[string]any{"search": `{"filter":` + allFilter + `}`}),
		{"name": "autotask_search", "arguments": map[string]any{"query": "tickets"}},
		{"name": "autotask_get", "arguments": map[string]any{"endpoint_id": "tickets.query-item"}},
		{"name": "account_brief", "arguments": map[string]any{"companyID": bbCompanyID, "since": "30d"}},
		{"name": "company_360", "arguments": map[string]any{"id": bbCompanyID}},
		{"name": "contract_burn", "arguments": none},
		{"name": "data_gaps", "arguments": none},
		{"name": "orphans", "arguments": none},
		{"name": "project_health", "arguments": none},
		{"name": "reconcile", "arguments": none},
		{"name": "retainer", "arguments": none},
		{"name": "sla_breaches", "arguments": none},
		{"name": "stale", "arguments": map[string]any{"days": "14"}},
		{"name": "ticket_aging", "arguments": none},
		{"name": "triage", "arguments": none},
		{"name": "unbilled", "arguments": none},
		{"name": "workload", "arguments": none},
		{"name": "load", "arguments": none},
		{"name": "since", "arguments": map[string]any{"duration": "7d"}},
		{"name": "picklist", "arguments": map[string]any{"entity": "Tickets", "field": "status"}},
		{"name": "tail", "arguments": map[string]any{"resource": "tickets", "follow": false}},
		{"name": "analytics", "arguments": map[string]any{"type": "tickets", "group-by": "title"}},
		{"name": "search", "arguments": map[string]any{"query": "HQ"}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
		{"name": "context", "arguments": none},
	}
}

// mcpWrites is every write tool that takes confirm, called without it: each
// must answer with a preview and send nothing. autotask_execute previews as a
// confirmation_required object, account_review (the recipe, gated by the
// endpoint gate) the same; the mirrored CLI writes (import, zone) preview as
// the command's --dry-run (zone: --no-save) output followed by the gate's
// "Preview only: nothing was sent" note.
func mcpWrites() []map[string]any {
	execute := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "autotask_execute", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		{"name": "import", "arguments": map[string]any{"resource": "tickets"}},
		{"name": "zone", "arguments": map[string]any{"user": bbZoneUser}},
		{"name": "account_review", "arguments": map[string]any{"id": bbCompanyID}},
		execute("tickets.create-entity", map[string]any{"title": "Preview ticket", "description": "d", "companyID": 77, "status": 1, "priority": 1}),
		execute("companies.update-entity", map[string]any{"id": 77, "companyName": "Preview Co"}),
		execute("companies.patch-entity", map[string]any{"id": 77, "companyName": "Preview Co"}),
		execute("tickets.patch-entity", map[string]any{"id": 5150, "status": 5, "ApiIntegrationCode": bbMCPArgSec}),
		execute("tickets.notes.ticket-child-create-entity", map[string]any{"parentId": bbTicketID, "title": "Preview", "description": "d"}),
		execute("time-entries.delete-entity", map[string]any{"id": bbTimeID}),
		execute("companies.alerts.company-child-delete-entity", map[string]any{"parentId": bbCompanyID, "id": "3"}),
		execute("configuration-items.create-entity", map[string]any{"referenceTitle": "Preview switch", "Secret": bbMCPArgSec, "secretKey": bbMCPArgSec}),
		execute("contracts.update-entity", map[string]any{"id": 12, "contractName": "Preview"}),
		execute("resources.patch-entity", map[string]any{"id": 29683482, "email": "preview@example.com"}),
		execute("tickets.query", map[string]any{}),
	}
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
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

// Class credential-persisted-in-files. Every sink runs against the echoing
// fake and no file but the credential store holds a credential in any form
// the fake sent; then a clean fake and a control battery prove each sink
// really writes (the kit withholds a whole write that carries a held value,
// so the echoing run alone could pass by writing nothing). The battery
// groups, the MCP battery and the controls each run in their own home, in
// parallel.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	sent, scanned := 0, 0
	scanHome := func(t *testing.T, sv *server, e *bbEnv) {
		n, _ := sv.sawHeader(e.mode.sent())
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
		if !sawCreds {
			t.Errorf("auth set-token wrote no credential store")
		}
		k := scanFiles(e.home, func(rel string, data []byte) {
			if !synccpuStoreFile(rel) { // synccpu: the local store is not checked
				assertNoEchoIn(t, "file "+rel, string(data), !isStoreFile(rel))
			}
		})
		mu.Lock()
		sent, scanned = sent+n, scanned+k
		mu.Unlock()
	}
	t.Run("sinks", func(t *testing.T) {
		eachGroup(t, credStored, func(t *testing.T, sv *server, e *bbEnv, group [][]string) {
			for _, args := range group {
				e.run(t, args...)
			}
			scanHome(t, sv, e)
		})
		t.Run("mcp", func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, credStored)
			e.mcpCalls(t, mcpBattery())
			scanHome(t, sv, e)
		})
		t.Run("controls", func(t *testing.T) {
			t.Parallel()
			controls(t)
		})
	})
	if sent == 0 {
		t.Fatalf("no request carried the stored code with the environment's UserName and Secret; the battery ran unauthenticated")
	}
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
}

// controls runs the sinks without a credential in what they write: each
// writes, so the scans in TestNoCredentialInAnyWrittenFile are not vacuous.
func controls(t *testing.T) {
	sv := echoServer(t)
	sv.setClean(true)
	e := newEnv(t, sv, credStored)
	controlExport := filepath.Join(e.home, "control-export.jsonl")
	controlDeliver := filepath.Join(e.home, "control-deliver.json")
	for _, args := range [][]string{
		{"sync", "--json"},
		{"export", "tickets", bbTicketID, "--format", "jsonl", "--output", controlExport},
		e.api("tickets", "query", "--filter", allFilter, "--json", "--deliver", "file:"+controlDeliver),
		{"feedback", "control note HQ-Feedback"},
		{"teach", "--query", "find tickets control", "--resource", bbTicketID, "--resource-type", "tickets", "--notes", "rotated control", "--no-validate"},
		{"teach", "--resource", bbTicketID, "--resource-type", "tickets"}, // no --query: teach.log records the refusal
		{"profile", "save", "control", "--description", "HQ-Profile control", "--select", "id"},
	} {
		e.run(t, args...)
	}
	scanFiles(e.home, func(rel string, data []byte) {
		assertNoEchoIn(t, "file "+rel, string(data), !isStoreFile(rel))
	})
	for _, p := range []string{controlDeliver, controlExport} {
		if b, err := os.ReadFile(p); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
			t.Errorf("%s: want the record (err %v):\n%.1500s", filepath.Base(p), err, b)
		}
	}
	var sawStore, sawWebhook, sawMasked, sawResource, sawLearning, sawFeedback, sawProfile, sawTeachLog bool
	scanFiles(e.home, func(rel string, b []byte) {
		name := filepath.Base(rel)
		if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") {
			sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
			// The webhook-shaped record was stored, its secretKey replaced
			// (credential-fields-withheld).
			sawWebhook = sawWebhook || bytes.Contains(b, []byte("HQ-Record switch"))
			sawMasked = sawMasked || bytes.Contains(b, []byte("[withheld]"))
			// The Resource whose userName is no credential was stored as it came.
			sawResource = sawResource || bytes.Contains(b, []byte(bbPlainUser))
			// The control teach run's learning row was written.
			sawLearning = sawLearning || bytes.Contains(b, []byte("rotated control"))
		}
		sawFeedback = sawFeedback || (name == "feedback.jsonl" && bytes.Contains(b, []byte("HQ-Feedback")))
		sawProfile = sawProfile || bytes.Contains(b, []byte("HQ-Profile control"))
		sawTeachLog = sawTeachLog || (name == "teach.log" && bytes.Contains(b, []byte("missing --query")))
	})
	for what, ok := range map[string]bool{
		"no local store holds a fetched record":                                              sawStore,
		"no local store holds the synced webhook-shaped configuration item":                  sawWebhook,
		"no local store holds a secretKey replaced by [withheld]":                            sawMasked,
		"no local store holds the Resource whose userName is no credential":                  sawResource,
		"no local store holds the taught learning; the teach run wrote nothing":              sawLearning,
		"no feedback.jsonl holds the control feedback entry; the feedback run wrote nothing": sawFeedback,
		"no profile file holds the control profile; profile save wrote nothing":              sawProfile,
		"no teach.log holds the control refusal; teach wrote no log":                         sawTeachLog,
	} {
		if !ok {
			t.Error(what)
		}
	}
}

// Class secret-in-output, with the credentials in the environment over a
// config file (the file's code and Secret held unused) and AUTOTASK_USERNAME
// (an identifier) beside them: every command and MCP tool against the echoing fake, then a fake
// answering clean. Every other credential source runs a short battery in
// TestEveryCredentialSourceSendsOneKey; the stored code runs this battery in
// TestNoCredentialInAnyWrittenFile, whose file scan also reads what the
// battery wrote. Each part runs in its own home, in parallel.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	const mode = credEnvOverConfig
	env := func(t *testing.T, sv *server) *bbEnv {
		e := newEnv(t, sv, mode)
		e.env = append(e.env, "AUTOTASK_USERNAME="+bbZoneUser)
		return e
	}
	// The fake echoed the triple it really received, so the scans covered it;
	// no other credential source was sent.
	triple := func(t *testing.T, sv *server, needSent bool) {
		n, all := sv.sawHeader(mode.sent())
		if len(all) > 1 || (needSent && n == 0) {
			t.Errorf("want every request to carry the %s triple, got %d of it and distinct triples %v", mode, n, all)
		}
	}
	t.Run("parts", func(t *testing.T) {
		eachGroup(t, mode, func(t *testing.T, sv *server, e *bbEnv, group [][]string) {
			e.env = append(e.env, "AUTOTASK_USERNAME="+bbZoneUser)
			for _, args := range group {
				assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
			}
			triple(t, sv, false)
		})
		t.Run("typed-back", func(t *testing.T) {
			t.Parallel()
			// A held value the API never receives, typed where the program
			// repeats it: a path the fake echoes and a dry-run body.
			sv := echoServer(t)
			e := env(t, sv)
			for _, held := range mode.held() {
				for _, args := range [][]string{
					e.api("tickets", "query-item", held, "--json"),
					e.api("tickets", "create-entity", "--title", "HQ "+held, "--dry-run"),
					{"import", "tickets", "--input", e.importFile(t, held), "--dry-run"},
				} {
					assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
				}
			}
			// A response that echoes a credential is withheld whole, the kit's
			// notice in its place.
			cmd := exec.Command(bbCLI, e.api("tickets", "query", "--filter", allFilter, "--json")...)
			cmd.Env, cmd.Dir = e.env, e.home
			if stdout, _ := cmd.Output(); !bytes.Contains(stdout, []byte("credkit: withheld")) {
				t.Fatalf("tickets query: want the kit's notice for an echoing response, got:\n%.1500s", stdout)
			}
			triple(t, sv, true)
		})
		t.Run("mcp", func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := env(t, sv)
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			triple(t, sv, true)
		})
		t.Run("clean", func(t *testing.T) {
			t.Parallel()
			// Clean, the data prints as valid JSON, a credential-named field is
			// replaced by [withheld], and a Resource userName that is no
			// credential prints as it came, the API user's own UserName included.
			sv := echoServer(t)
			sv.setClean(true)
			e := env(t, sv)
			for _, c := range []struct {
				args []string
				want string
			}{
				{e.api("tickets", "query", "--filter", allFilter, "--json"), "[withheld]"},
				{e.api("resources", "query", "--filter", allFilter, "--json"), bbPlainUser},
				{e.api("resources", "query", "--filter", allFilter, "--json"), mode.sent().user},
				{e.api("contacts", "query", "--filter", allFilter, "--json"), "[withheld]"},
			} {
				cmd := exec.Command(bbCLI, c.args...)
				cmd.Env, cmd.Dir = e.env, e.home
				stdout, err := cmd.Output()
				if err != nil {
					t.Fatalf("%v: %v\n%s", c.args[:2], err, stdout)
				}
				var doc any
				if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ")) || !bytes.Contains(stdout, []byte(c.want)) {
					t.Fatalf("%v: want valid JSON with the record and %s, got:\n%.1500s", c.args[:2], c.want, stdout)
				}
				assertNoEcho(t, fmt.Sprintf("clean output of %v", c.args), string(stdout))
			}
			out := e.mcpCalls(t, mcpReads())
			if !strings.Contains(out, "HQ-Record") || !strings.Contains(out, bbPlainUser) {
				t.Fatalf("MCP results lost the record itself:\n%.2000s", out)
			}
			assertNoEcho(t, "clean MCP session", out)
			triple(t, sv, true)
		})
	})
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before it is followed, from the CLI, sync and MCP alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	redirect := e.run(t, e.api("tickets", "query-item", bbRedirect, "--json")...)
	sv.mu.Lock()
	sv.redirectTickets = true
	sv.mu.Unlock()
	syncOut := e.run(t, "sync", "--resources", "tickets", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "autotask_read", "arguments": map[string]any{"endpoint_id": "tickets.query-item", "params": map[string]any{"id": bbRedirect}}},
		{"name": "autotask_read", "arguments": map[string]any{"endpoint_id": "tickets.query", "params": map[string]any{
			"filter": []any{map[string]any{"op": "gte", "field": "id", "value": 0}}}}},
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		if strings.HasSuffix(m, apiBase+"/Tickets/"+bbRedirect) || strings.HasSuffix(m, apiBase+"/Tickets/query") {
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
	for where, out := range map[string]string{"tickets query-item": redirect, "sync": syncOut} {
		if !strings.Contains(out, "leave the configured Autotask host") {
			t.Fatalf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
	}
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a read (a GET
// or a POST /query) while no call sets confirm, though the battery calls
// every write tool; each write answers with a preview.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	sv.setClean(true) // the previews and the fetched record print (the kit withholds a result that echoes)
	e := newEnv(t, sv, credEnv)
	// The store holds the synced account first, so the novels have something
	// to read while the writes preview.
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
		if args, _ := call["arguments"].(map[string]any); args["endpoint_id"] == "tickets.query" {
			// A read named to the write tool is refused, never sent.
			if !strings.Contains(r, "is read-only; call autotask_read") {
				t.Errorf("autotask_execute tickets.query was not refused as read-only:\n%.1500s", r)
			}
			continue
		}
		// A preview that repeats a credential the call passed (Secret,
		// secretKey, ApiIntegrationCode: held as MCP arguments) is withheld
		// whole; the request count below proves it still sent nothing.
		if !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") && !strings.Contains(r, "credkit: withheld") {
			t.Errorf("%s %v without confirm did not return a preview:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var reads bool
	for _, m := range sv.methods {
		reads = reads || strings.HasSuffix(m, apiBase+"/Tickets/query")
		if isWrite(m) {
			t.Errorf("the MCP server sent %s without confirm", m)
		}
	}
	if !reads {
		t.Errorf("the MCP reads never reached the API; requests: %v", sv.methods)
	}
}

// Control for the test above: with confirm set to boolean true the same write
// reaches the API, so the gate previews rather than drops; the string "true"
// and the number 1 still preview. Both gates: the endpoint tool
// (autotask_execute) and the mirrored write (import). The mirror blocks
// import's required --input (a file the server would read for the caller), so
// a mirrored import can never send a record: its control is that confirm true
// runs the child as given (its own answer, without --dry-run or the gate's
// preview note) while every other value previews.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	sv.setClean(true) // the previews and the import's own answer print
	e := newEnv(t, sv, credEnv)
	write := func(confirm any) []map[string]any {
		return []map[string]any{
			{"name": "autotask_execute", "arguments": map[string]any{"endpoint_id": "tickets.patch-entity",
				"params": map[string]any{"id": 5150, "status": 5}, "confirm": confirm}},
			{"name": "autotask_execute", "arguments": map[string]any{"endpoint_id": "time-entries.delete-entity",
				"params": map[string]any{"id": bbTimeID}, "confirm": confirm}},
			{"name": "import", "arguments": map[string]any{"resource": "tickets", "confirm": confirm}},
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
	var patch, del bool
	for _, m := range sv.methods {
		patch = patch || m == "PATCH "+apiBase+"/Tickets"
		del = del || m == "DELETE "+apiBase+"/TimeEntries/"+bbTimeID
	}
	if !patch || !del {
		t.Errorf("confirm true did not send the ticket patch and the time entry delete; requests: %v", sv.methods)
	}
	if r := mcpReplies(out)[3]; r == "" || strings.Contains(r, "Preview only") || strings.Contains(r, "dry run") || !strings.Contains(r, "input") {
		t.Errorf("confirm true did not run the mirrored import as given:\n%.1500s", r)
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, from a cold
// start with the credentials in the environment, the credential store or a
// config file.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credEnv, credStored, credConfigFile, credConfigMixed} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		imp := e.importFile(t, "dry")
		for _, args := range [][]string{
			e.api("tickets", "create-entity", "--title", "Dry", "--description", "d", "--company-id", bbCompanyID, "--status", "1", "--dry-run"),
			e.api("companies", "patch-entity", "--id", bbCompanyID, "--company-name", "Dry Co", "--dry-run"),
			e.api("companies", "update-entity", "--id", bbCompanyID, "--company-name", "Dry Co", "--dry-run"),
			e.api("tickets", "patch-entity", "--id", bbTicketID, "--status", "5", "--dry-run"),
			e.api("time-entries", "delete-entity", bbTimeID, "--dry-run"),
			e.api("companies", "alerts", "company-child-delete-entity", bbCompanyID, "3", "--dry-run"),
			e.api("tickets", "notes", "ticket-child-create-entity", bbTicketID, "--title", "Dry", "--description", "d", "--dry-run"),
			e.api("configuration-items", "create-entity", "--reference-title", "Dry", "--dry-run"),
			e.api("contracts", "update-entity", "--id", "12", "--contract-name", "Dry", "--dry-run"),
			{"import", "tickets", "--input", imp, "--dry-run"},
			e.api("tickets", "query", "--filter", allFilter, "--dry-run"),
			e.api("tickets", "query-item", bbTicketID, "--dry-run"),
		} {
			// Each run reaches its preview, so the count below is not vacuous
			// (a flag error would send nothing either).
			if out := e.run(t, args...); !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") && !strings.Contains(out, "would ") {
				t.Errorf("%v (mode %s) printed no preview:\n%.1500s", args, mode, out)
			}
		}
		sv.mu.Lock()
		if len(sv.methods) != 0 {
			t.Errorf("--dry-run (mode %s) sent %v", mode, sv.methods)
		}
		sv.mu.Unlock()
		// Control: the same write without --dry-run reaches the fake.
		e.run(t, e.api("companies", "patch-entity", "--id", bbCompanyID, "--company-name", "Dry Co", "--yes")...)
		sv.mu.Lock()
		if len(sv.methods) != 1 || sv.methods[0] != "PATCH "+apiBase+"/Companies" {
			t.Errorf("control (mode %s): want the one PATCH, got %v", mode, sv.methods)
		}
		sv.mu.Unlock()
	}
}
