// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package knowbe4blackbox runs the built CLI and MCP binaries against two
// in-process KnowBe4 fakes and checks what a reviewer cannot read off the
// code (knowbe4 handfixes output-chokepoint, store-never-keeps-secrets,
// no-response-cache, file-sinks-withheld, response-credentials-held,
// api-origin-pinned, region-origin-pinned, user-event-origin-pinned,
// dry-run-sends-nothing, mcp-endpoint-write-confirm, mcp-mirror-write-confirm,
// knowbe4-credential-names-exact):
//
//   - the Reporting API (/v1, read-only: every endpoint is a GET; lists are
//     bare JSON arrays paged by the 1-based page and per_page, gets and
//     /account are bare objects, integer ids), authenticated with
//     Authorization: Bearer <KNOWBE4_API_KEY>;
//   - the User Event API (its own host; GET/POST /events, GET/DELETE
//     /events/{id}, GET /event_types, GET /statuses, GET /statuses/{id};
//     every answer a {"data": ..., "meta": {"count": N}} envelope),
//     authenticated with Authorization: Bearer <KNOWBE4_USER_EVENT_API_KEY>.
//
// Both fakes serve TLS with one test certificate, trusted through
// KNOWBE4_CA_CERT (the credential kit refuses a held value over plain http).
// Each fake echoes every credential the process holds back in every encoding
// it can, whole and split across lines, adjacent values, members and pages:
// what the request carried, and the other API's key (both keys are held from
// the first byte of output); the kit withholds the whole unit that repeats
// one. A quiet fake (quietServer, setQuiet) echoes nothing, for the checks
// that the data itself prints. The Reporting API documents no
// credential-bearing response field, so the kit holds none; the
// credential-named members the fakes add are data, not checked. The suite
// checks that:
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the Reporting key from KNOWBE4_API_KEY, from the
//     credential store (auth set-token) or from a config file's api_key or
//     access_token, a key the configuration holds but does not send (a stored
//     key the environment overrides, a config file's access_token beside its
//     api_key), the User Event key, the MCP HTTP token, a credential-named
//     query value given on the command line or as an MCP argument;
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     feedback log, the learn journal, profiles, teach logs);
//   - no credential leaves for a host other than the one it belongs to: a
//     redirect off either API host is refused before it is followed, a region
//     (KNOWBE4_REGION, sync --path-context region=) cannot move the resolved
//     https://{region}.api.knowbe4.com origin, a hostile region cannot move
//     the User Event host, and neither key ever reaches the other API's host;
//   - the MCP server sends a write (events_create, events_delete) only when
//     the call sets confirm to the boolean true, and --dry-run sends nothing.
//
// KnowBe4 data under names that read as credentials or carry identities (a
// phishing recipient's user object and its email, a user's last_sign_in and
// manager_email, the account admins' emails, an event's risk_level,
// event_type and target_user) is never withheld.
//
// The oracle is the exact set of strings the fakes sent, so a pass does not
// depend on the program's own decoder.
package knowbe4blackbox

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
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"knowbe4-pp-cli/internal/userevents"
)

const (
	bbReportHeld = "Qz7vXk2mWp9rTn4bLc8dHs3fJy6g"         // KNOWBE4_API_KEY (Reporting API: Authorization: Bearer <key>)
	bbEventHeld  = "Rw3nVb8xKq5tZm2pLd7cHy4sGf9j"         // KNOWBE4_USER_EVENT_API_KEY (User Event API host only: Authorization: Bearer <key>)
	bbStored     = "Tp6kMz9wQx2vBn5rYc8hLs3dFj7g"         // auth set-token (stdin), the credential store
	bbFileHeldA  = "Wm4qZr7xNv2kTb9pHc5yLd8sGj3f"         // api_key in a 0600 config file named by KNOWBE4_CONFIG
	bbFileHeldB  = "Yx8tKp3vMz6wQn2rBc9hLf5sDj7g"         // access_token in a 0600 config file (sent alone; held unused beside api_key)
	bbArgvSec    = "-Vk5rQz8wNm3tYp6xBh2cJ"               // a credential-named query value on the command line (sync --param password=...; starts with "-")
	bbMCPArgSec  = "Zn7wQk4vXm9rTp2bLh6cYd"               // a credential-named query value an MCP sync call sends (param password=...)
	bbShortResp  = "Sh0rtPw"                              // a 7-character password every user record carries (gzip-encoded list)
	bbBOMResp    = "BomRespSecret-6a1c"                   // a store purchase's api_key, BOM-prefixed response
	bbGzipResp   = "GzipRespToken-4f2a9c1e"               // the account integration's token, gzip-encoded response
	bbNewSecret  = "newEventSecret-0b7e3d5a9c1f"          // the secret a POST /events answer carries
	bbEncHeld    = "cQ7vZr2k-6d0a2e8c4f71Wm"              // the account integration's encryption_key (a qualified key name)
	bbPairUser   = "ssoUser-3e8a51c0"                     // the account integration's username (the identity half of the pair)
	bbCredUser   = "purchaseUser-5c1e9a27"                // a store purchase's credentials.username (inside a credential container)
	bbHTTPHeld   = "Hc2vQz8kXm5wTr9pNb3yLd6sGf4jKq7x"     // KNOWBE4_MCP_HTTP_TOKEN
	bbUserID     = "667542"                               // a user who clicked in both phishing tests and passed no training
	bbUser2      = "667543"                               // a user who reported both phishing tests and passed training
	bbGroupID    = "3264"                                 // a group; its members split the first credential across two pages
	bbGroup2     = "3265"                                 // a second group, no members
	bbCampaignID = "3423"                                 // a phishing campaign
	bbPSTID      = "16142"                                // a phishing security test of bbCampaignID
	bbPST2       = "16143"                                // a second phishing security test of bbCampaignID
	bbRecipient  = "3077742"                              // bbUserID's recipient row in bbPSTID
	bbTrainingID = "4501"                                 // a training campaign
	bbEnrollID   = "1425526"                              // a training enrollment (bbUser2, Passed)
	bbPurchaseID = "11"                                   // a store purchase
	bbPolicyID   = "1"                                    // a policy
	bbEventID    = "513f46ac-c3d7-4682-ad0d-0c149c0728a2" // a user event
	bbRequestID  = "abcdefgh-843c-4fc8-bb2f-decf89876f7b" // a user-event request status
	bbRedirect   = "9911"                                 // a user ID and an event ID the fakes redirect off-host
	bbDenied     = "40199"                                // an ID (or a query value) the fakes answer 401
	// KnowBe4 data under names that read as credentials or identities: never
	// withheld (knowbe4-credential-names-exact).
	bbClickEmail  = "pat.clicker@hq-record.example.com"    // bbUserID's email (a recipient's user.email, an event's target_user)
	bbReportEmail = "sam.reporter@hq-record.example.com"   // bbUser2's email
	bbMgrEmail    = "morgan.manager@hq-record.example.com" // a user's manager_email
	bbAdminEmail  = "alex.admin@hq-record.example.com"     // the account admin's email
	bbLastSignIn  = "2026-10-27T08:15:42.000Z"             // a user's last_sign_in
	bbEventType   = "PhishRIP Message Found"               // an event's event_type
)

// secrets are every credential the suite gives the process, in the order the
// fakes split them (the first one a request carried is the one split across
// pages).
//
// Every one is random-looking and shares no 8-character piece with the
// program's own text (the kit withholds any unit that repeats a piece of a
// held value). The credential-named response fields the fakes add (bbShortResp
// through bbCredUser) are data the Reporting API does not document: the kit
// declares no response field, so they are not held and not checked here. The
// credential-named query values typed on the command line or as an MCP
// argument (bbArgvSec, bbMCPArgSec) are not held either (credwire declares
// no credential flag and no MCP argument): the battery still sends them, but
// a dry run may print them back, so they are not checked.
func secrets() []string {
	return []string{bbReportHeld, bbEventHeld, bbStored, bbFileHeldA, bbFileHeldB, bbHTTPHeld}
}

// reportingKeys are the keys that belong to the Reporting API host; the User
// Event host must never receive one.
var reportingKeys = []string{bbReportHeld, bbStored, bbFileHeldA, bbFileHeldB}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "knowbe4-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "knowbe4-cli"+exe)
	bbMCP = filepath.Join(dir, "knowbe4-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/knowbe4-cli", bbMCP: "../../cmd/knowbe4-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	fmt.Fprintf(os.Stderr, "knowbe4blackbox: %d CLI runs, %d MCP tools/call requests\n", cliRuns.n, mcpCallsSent.n)
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
		string(pem.EncodeToMemory(&pem.Block{Type: "KNOWBE4 API KEY", Bytes: []byte(s)})),
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
	url     string
	caFile  string
	mu      sync.Mutex
	offHost int
	methods []string       // API requests, "METHOD /path"
	keys    map[string]int // keys the API received as the one header Authorization: Bearer <key>
	// otherAuth counts requests whose credential came any other way: a second
	// Authorization header, one that is not "Bearer <key>", an X-Api-Key
	// header, or a key in the query (KnowBe4 takes only Authorization:
	// Bearer <key>).
	otherAuth int
	// foreign counts requests that carried the OTHER API's key anywhere (a
	// header, the URL or the body): the User Event key at the Reporting host,
	// a Reporting key at the User Event host.
	foreign int
	// pages counts each list page the fake served, by "/path?page=N".
	pages map[string]int
	// redirectUsers sends the user list (the syncable resource the novels
	// read) to the other host too, so sync meets a redirect.
	redirectUsers bool
	// denied paths answer 401 (an export or sync that meets an API error).
	denied map[string]bool
	// denials counts the requests the fake answered 401.
	denials int
	// ev is the User Event API fake beside this Reporting API fake.
	ev *server
	// rep is the Reporting API fake beside this User Event API fake.
	rep *server
	// quiet stops the echo: the fake repeats no credential it was sent or
	// holds, so a check of the data itself is not withheld whole by the kit.
	quiet bool
}

func (sv *server) isQuiet() bool {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.quiet
}

// setQuiet turns the echo off (or on) at both fakes.
func (sv *server) setQuiet(q bool) {
	for _, s := range []*server{sv, sv.ev} {
		if s != nil {
			s.mu.Lock()
			s.quiet = q
			s.mu.Unlock()
		}
	}
}

// quietServer is echoServer with the echo off at both fakes.
func quietServer(t *testing.T) *server {
	t.Helper()
	sv := echoServer(t)
	sv.setQuiet(true)
	return sv
}

func newServer() *server {
	return &server{keys: map[string]int{}, pages: map[string]int{}, denied: map[string]bool{}}
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

// isWrite reports a request that changes data (HEAD, a count, does not).
func isWrite(m string) bool {
	method, _, _ := strings.Cut(strings.TrimPrefix(m, "events "), " ")
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

// pageQuery is a KnowBe4 list request's page: per_page (def when absent, at
// most 500) and the 1-based page (a page below 1 reads as the first).
func pageQuery(q url.Values, def int) (size, number int) {
	size, number = def, 1
	if n, err := strconv.Atoi(q.Get("per_page")); err == nil && n > 0 {
		size = min(n, 500)
	}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n >= 1 {
		number = n
	}
	return size, number
}

// pagedArray is one page of a KnowBe4 list: items[(page-1)*per_page:...] as a
// bare JSON array.
func pagedArray(q url.Values, def int, items []string) string {
	size, number := pageQuery(q, def)
	start := min((number-1)*size, len(items))
	return "[" + strings.Join(items[start:min(start+size, len(items))], ",") + "]"
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
func (sv *server) countAuth(r *http.Request) {
	auth := r.Header.Values("Authorization")
	if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && len(auth) == 1 && tok != "" && !strings.ContainsAny(tok, " \t") {
		sv.keys[tok]++
	} else if len(auth) > 0 {
		sv.otherAuth++
	}
	if len(r.Header.Values("X-Api-Key")) > 0 {
		sv.otherAuth++
	}
	q := r.URL.Query()
	for _, k := range []string{"api_key", "apikey", "x-api-key", "token", "access_token", "key"} {
		if q.Has(k) {
			sv.otherAuth++
		}
	}
}

// receivedKeys are the distinct bearer keys this fake received, sorted.
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

var (
	reUser       = regexp.MustCompile(`^/users/[^/]+$`)
	reRiskHist   = regexp.MustCompile(`^/(users|groups)/[^/]+/risk_score_history$`)
	reGroup      = regexp.MustCompile(`^/groups/[^/]+$`)
	reMembers    = regexp.MustCompile(`^/groups/[^/]+/members$`)
	reCampaign   = regexp.MustCompile(`^/phishing/campaigns/[^/]+$`)
	reCampaignPS = regexp.MustCompile(`^/phishing/campaigns/[^/]+/security_tests$`)
	rePST        = regexp.MustCompile(`^/phishing/security_tests/[^/]+$`)
	reRecipients = regexp.MustCompile(`^/phishing/security_tests/([^/]+)/recipients$`)
	reRecipient  = regexp.MustCompile(`^/phishing/security_tests/([^/]+)/recipients/([^/]+)$`)
	reTraining   = regexp.MustCompile(`^/training/campaigns/[^/]+$`)
	reEnrollment = regexp.MustCompile(`^/training/enrollments/[^/]+$`)
	rePurchase   = regexp.MustCompile(`^/training/store_purchases/[^/]+$`)
	rePolicy     = regexp.MustCompile(`^/policies/[^/]+$`)
	reEvent      = regexp.MustCompile(`^/events/[^/]+$`)
	reStatus     = regexp.MustCompile(`^/statuses/[^/]+$`)
)

// last is the last path segment of p.
func last(p string) string { return p[strings.LastIndex(p, "/")+1:] }

// kb4User is a user as GET /users, GET /users/{id} and GET
// /groups/{id}/members return it, with the password every record carries.
func kb4User(id, first, last, email, title string, risk float64, signIn, extra string) string {
	return fmt.Sprintf(`{"id":%s,"employee_number":"E-%s","first_name":%q,"last_name":%q,"job_title":%q,"email":%q,`+
		`"phish_prone_percentage":%.1f,"phone_number":"555-0100","location":"HQ","division":"Ops","manager_name":"Morgan Manager",`+
		`"manager_email":%q,"groups":[%s],"current_risk_score":%.1f,"joined_on":"2025-01-15T00:00:00.000Z","last_sign_in":%q,`+
		`"status":"active","organization":"HQ-Record","department":"Finance","language":"English","password":%q%s}`,
		id, id, first, last, title, email, risk/2, bbMgrEmail, bbGroupID, risk, signIn, bbShortResp, extra)
}

// echoServer starts the Reporting API fake (TLS) and the User Event API fake
// (plain HTTP: its client takes no CA bundle) beside it.
func echoServer(t *testing.T) *server {
	t.Helper()
	sv := newServer()
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	now := time.Now().UTC()
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	earlier := now.Add(-20 * 24 * time.Hour).Format(time.RFC3339)
	user1 := func(extra string) string {
		return kb4User(bbUserID, "HQ-Record Pat", "Clicker", bbClickEmail, "Analyst", 72.5, bbLastSignIn, extra)
	}
	user2 := func(extra string) string {
		return kb4User(bbUser2, "HQ-Record Sam", "Reporter", bbReportEmail, "Controller", 12.0, bbLastSignIn, extra)
	}
	group := func(id, name, extra string) string {
		return fmt.Sprintf(`{"id":%s,"name":%q,"group_type":"console_group","adi_guid":null,"member_count":2,"current_risk_score":44.1,"status":"active"%s}`,
			id, name, extra)
	}
	groupsRef := fmt.Sprintf(`[{"group_id":%s,"name":"HQ-Record Finance"}]`, bbGroupID)
	campaign := func(extra string) string {
		return fmt.Sprintf(`{"campaign_id":%s,"name":"HQ-Record Quarterly Phish","groups":%s,"last_phish_prone_percentage":50.0,"last_run":%q,`+
			`"status":"Active","hidden":false,"send_duration":"3 Business Days","track_duration":"3 Days","frequency":"Monthly",`+
			`"difficulty_filter":[1,2],"create_date":%q,"psts_count":2%s}`, bbCampaignID, groupsRef, recent, earlier, extra)
	}
	pst := func(id, name, started, extra string) string {
		return fmt.Sprintf(`{"campaign_id":%s,"pst_id":%s,"status":"Closed","name":%q,"groups":%s,"phish_prone_percentage":50.0,`+
			`"started_at":%q,"duration":3,"categories":[{"category_id":4,"name":"HQ"}],"template":{"id":1,"name":"HQ invoice"},`+
			`"landing_page":{"id":1,"name":"HQ"},"scheduled_count":2,"delivered_count":2,"opened_count":1,"clicked_count":1,`+
			`"replied_count":0,"attachment_open_count":0,"macro_enabled_count":0,"data_entered_count":0,"qr_code_scanned_count":0,`+
			`"reported_count":1,"bounced_count":0%s}`, bbCampaignID, id, name, groupsRef, started, extra)
	}
	// recipient is a phishing test recipient row: the clicker clicked, the
	// reporter reported.
	recipient := func(rid, pstID, uid, email string, clicked bool, at, extra string) string {
		clickedAt, reportedAt := "null", strconv.Quote(at)
		if clicked {
			clickedAt, reportedAt = strconv.Quote(at), "null"
		}
		return fmt.Sprintf(`{"recipient_id":%s,"pst_id":%s,"user":{"id":%s,"active_directory_guid":null,"first_name":"HQ-Record","last_name":"Recipient","email":%q},`+
			`"template":{"id":1,"name":"HQ invoice"},"scheduled_at":%q,"delivered_at":%q,"opened_at":%q,"clicked_at":%s,"replied_at":null,`+
			`"attachment_opened_at":null,"macro_enabled_at":null,"data_entered_at":null,"qr_code_scanned_at":null,"reported_at":%s,`+
			`"bounced_at":null,"ip":"203.0.113.7","ip_location":"HQ","browser":"Chrome","browser_version":"118","os":"Windows"%s}`,
			rid, pstID, uid, email, at, at, at, clickedAt, reportedAt, extra)
	}
	recipients := func(pstID, extra string) []string {
		base, _ := strconv.Atoi(bbRecipient)
		if pstID == bbPST2 {
			base += 2
		}
		return []string{
			recipient(strconv.Itoa(base), pstID, bbUserID, bbClickEmail, true, recent, extra),
			recipient(strconv.Itoa(base+1), pstID, bbUser2, bbReportEmail, false, recent, ""),
		}
	}
	enrollment := func(id, uid, email, status, extra string) string {
		return fmt.Sprintf(`{"enrollment_id":%s,"content_type":"Training Module","module_name":"HQ-Record Phishing 101",`+
			`"user":{"id":%s,"first_name":"HQ-Record","last_name":"Learner","email":%q},"campaign_name":"HQ-Record Training",`+
			`"enrollment_date":%q,"start_date":%q,"completion_date":%q,"status":%q,"time_spent":1200,"policy_acknowledged":false%s}`,
			id, uid, email, earlier, earlier, recent, status, extra)
	}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreign := strings.Contains(requestText(r), bbEventHeld)
		// What the request carried, and the User Event key the process holds
		// (both keys are held from the first byte of output).
		sent := union(sentSecrets(r), bbEventHeld)
		if sv.isQuiet() {
			sent = nil
		}
		echo := echoObject(sent)
		p := strings.TrimPrefix(r.URL.Path, "/v1")
		q := r.URL.Query()
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		sv.countAuth(r)
		if foreign {
			sv.foreign++
		}
		pg := q.Get("page")
		if pg == "" {
			pg = "1"
		}
		if r.Method == http.MethodGet {
			sv.pages[p+"?page="+pg]++
		}
		redirectUsers := sv.redirectUsers
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
			// Rejected (a denied user, group or test, a sync or export the
			// test denies): the error repeats what it was sent.
			sv.mu.Lock()
			sv.denials++
			sv.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"message":"invalid api key %s","detail":[{"msg":"bad key %s","type":"auth"}],"echo":%s}`,
				strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/users/"+bbRedirect || (redirectUsers && p == "/users"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNoContent)
		case !get:
			// The Reporting API is read-only.
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprint(w, `{"message":"method not allowed"}`)
		case p == "/account":
			// The account, gzip-encoded: its admins (public), and an
			// integration's username, token and encryption key.
			gzipped(w, fmt.Sprintf(`{"name":"HQ-Record MSP","type":"paid","domains":["hq-record.example.com"],`+
				`"admins":[{"id":974278,"first_name":"HQ-Record","last_name":"Admin","email":%q}],"subscription_level":"Diamond",`+
				`"subscription_end_date":"2027-06-30","number_of_seats":250,"current_risk_score":37.4,`+
				`"integration":{"username":%q,"token":%q,"encryption_key":%q},"echo":%s}`, bbAdminEmail, bbPairUser, bbGzipResp, bbEncHeld,
				echoObject(union(sent, bbGzipResp, bbEncHeld, bbPairUser))))
		case p == "/account/risk_score_history" || reRiskHist.MatchString(p):
			fmt.Fprintf(w, `[{"risk_score":37.4,"date":"2026-10-01","echo":%s},{"risk_score":35.1,"date":"2026-09-01"}]`, echo)
		case p == "/users":
			// The users, gzip-encoded and paged, each with its password: the
			// clicker carries the echo.
			gzipped(w, pagedArray(q, 100, []string{user1(`,"echo":` + echo), user2("")}))
		case reUser.MatchString(p):
			// One user: the echo split across values, lines and spaces.
			u := user1
			if last(p) == bbUser2 {
				u = user2
			}
			fmt.Fprint(w, u(`,"split":`+splitObject(sent)))
		case p == "/groups":
			fmt.Fprint(w, pagedArray(q, 100, []string{group(bbGroupID, "HQ-Record Finance", `,"echo":`+echo), group(bbGroup2, "HQ-Record Ops", "")}))
		case reMembers.MatchString(p) && strings.TrimSuffix(strings.TrimPrefix(p, "/groups/"), "/members") == bbGroupID:
			// The first credential it was sent, split across two pages of the
			// group's members (per_page, default 100, full pages until the
			// last; 1-based page): the first member of page 1 carries one
			// half as its job title, the one member of page 2 the other, so
			// every pager that walks the pages meets both halves as records
			// of their own.
			first := "HQ-Record quiet member title"
			if len(sent) > 0 {
				first = sent[0]
			}
			a, b := halves(first)
			size, _ := pageQuery(q, 100)
			items := []string{kb4User("700001", "HQ-Record Member", "One", "member1@hq-record.example.com", a, 30, recent, "")}
			for i := 1; i < size; i++ {
				id := strconv.Itoa(700001 + i)
				items = append(items, kb4User(id, "HQ-Record Member", "N"+id, "member"+id+"@hq-record.example.com", "Staff", 30, recent, ""))
			}
			id := strconv.Itoa(700001 + size)
			items = append(items, kb4User(id, "HQ-Record Member", "Last", "member-last@hq-record.example.com", b, 30, recent, ""))
			fmt.Fprint(w, pagedArray(q, 100, items))
		case reMembers.MatchString(p):
			fmt.Fprint(w, `[]`)
		case reGroup.MatchString(p):
			fmt.Fprint(w, group(last(p), "HQ-Record Finance", `,"echo":`+echo))
		case p == "/phishing/campaigns":
			fmt.Fprint(w, pagedArray(q, 100, []string{campaign(`,"echo":` + echo)}))
		case reCampaignPS.MatchString(p):
			fmt.Fprintf(w, "[%s,%s]", pst(bbPSTID, "HQ-Record Phish Oct", recent, `,"echo":`+echo), pst(bbPST2, "HQ-Record Phish Sep", earlier, ""))
		case reCampaign.MatchString(p):
			fmt.Fprint(w, campaign(`,"echo":`+echo))
		case p == "/phishing/security_tests":
			fmt.Fprint(w, pagedArray(q, 100, []string{pst(bbPSTID, "HQ-Record Phish Oct", recent, `,"echo":`+echo), pst(bbPST2, "HQ-Record Phish Sep", earlier, "")}))
		case reRecipients.MatchString(p):
			id := reRecipients.FindStringSubmatch(p)[1]
			if id != bbPSTID && id != bbPST2 {
				fmt.Fprint(w, `[]`)
				break
			}
			fmt.Fprint(w, pagedArray(q, 100, recipients(id, `,"echo":`+echo)))
		case reRecipient.MatchString(p):
			m := reRecipient.FindStringSubmatch(p)
			fmt.Fprint(w, recipient(m[2], m[1], bbUserID, bbClickEmail, true, recent, `,"split":`+splitObject(sent)))
		case rePST.MatchString(p):
			fmt.Fprint(w, pst(last(p), "HQ-Record Phish Oct", recent, `,"echo":`+echo))
		case p == "/training/campaigns" || reTraining.MatchString(p):
			tc := fmt.Sprintf(`{"campaign_id":%s,"name":"HQ-Record Training","groups":%s,"status":"In Progress","content":[{"store_purchase_id":%s,"name":"HQ-Record Phishing 101"}],`+
				`"duration_type":"Specific End Date","start_date":%q,"end_date":%q,"relative_duration":"","auto_enroll":true,`+
				`"allow_multiple_enrollments":false,"completion_percentage":50,"echo":%s}`, bbTrainingID, groupsRef, bbPurchaseID, earlier, recent, echo)
			if p == "/training/campaigns" {
				tc = pagedArray(q, 100, []string{tc})
			}
			fmt.Fprint(w, tc)
		case p == "/training/enrollments":
			fmt.Fprint(w, pagedArray(q, 100, []string{
				enrollment(bbEnrollID, bbUser2, bbReportEmail, "Passed", `,"echo":`+echo),
				enrollment("1425527", bbUserID, bbClickEmail, "In Progress", ""),
			}))
		case reEnrollment.MatchString(p):
			fmt.Fprint(w, enrollment(last(p), bbUser2, bbReportEmail, "Passed", `,"echo":`+echo))
		case p == "/training/store_purchases" || rePurchase.MatchString(p):
			// Store purchases (list and get, sync, export): BOM-prefixed, with
			// an api_key and a credentials container, the echo, and a record
			// whose identifier repeats the credential it was sent.
			purchase := fmt.Sprintf(`{"store_purchase_id":%s,"content_type":"Store Purchase","name":"HQ-Record Phishing 101","description":"HQ-Record module",`+
				`"type":"Training Module","duration":15,"retired":false,"retirement_date":null,"publish_date":%q,"publisher":"KnowBe4",`+
				`"purchase_date":%q,"api_key":%q,"credentials":{"username":%q},"echo":%s}`, bbPurchaseID, earlier, earlier, bbBOMResp, bbCredUser, echo)
			doc := purchase
			if p == "/training/store_purchases" {
				items := []string{purchase}
				if len(sent) > 0 {
					items = append(items, fmt.Sprintf(`{"store_purchase_id":%q,"name":"Echo-ID","retired":true}`, sent[0]))
				}
				doc = pagedArray(q, 100, items)
			}
			fmt.Fprint(w, "\xef\xbb\xbf"+doc)
		case p == "/policies" || rePolicy.MatchString(p):
			pol := fmt.Sprintf(`{"policy_id":%s,"content_type":"Uploaded Policy","name":"HQ-Record AUP","minimum_time":3,"default_language":"en-us","published":true,"echo":%s}`,
				bbPolicyID, echo)
			if p == "/policies" {
				pol = pagedArray(q, 100, []string{pol})
			}
			fmt.Fprint(w, pol)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"not found"}`)
		}
	}))
	sv.url = srv.URL + "/v1"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	sv.ev = eventServer(t, sv)
	return sv
}

// eventServer starts a User Event API fake (TLS, the same test certificate
// as the Reporting fake, so its CA file trusts both) beside the Reporting
// fake rep: every answer a {"data":...,"meta":{"count":N}} envelope; it
// echoes what it was sent and every key rep received (the process holds
// both).
func eventServer(t *testing.T, rep *server) *server {
	t.Helper()
	ev := newServer()
	ev.rep = rep
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ev.mu.Lock()
		ev.offHost++
		ev.mu.Unlock()
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	now := time.Now().UTC()
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	event := func(id, extra string) string {
		return fmt.Sprintf(`{"id":%q,"user":{"id":%s,"email":%q},"target_user":%q,"external_id":"HQ-EXT-1","source":"HQ-Record SOC",`+
			`"description":"HQ-Record PhishRIP hit","occurred_date":%q,"risk_level":25,"risk_decay_mode":"linear",`+
			`"risk_expire_date":"2027-01-01T00:00:00.000Z","event_type":%q%s}`, id, bbUserID, bbClickEmail, bbClickEmail, recent, bbEventType, extra)
	}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		text := requestText(r)
		foreign := false
		for _, k := range reportingKeys {
			foreign = foreign || strings.Contains(text, k)
		}
		var repKeys []string
		if rep != nil {
			repKeys = rep.receivedKeys()
		}
		sent := union(sentSecrets(r), repKeys...)
		if ev.isQuiet() {
			sent = nil
		}
		echo := echoObject(sent)
		p, q := r.URL.Path, r.URL.Query()
		ev.mu.Lock()
		ev.methods = append(ev.methods, r.Method+" "+r.URL.Path)
		ev.countAuth(r)
		if foreign {
			ev.foreign++
		}
		pg := q.Get("page")
		if pg == "" {
			pg = "1"
		}
		if r.Method == http.MethodGet {
			ev.pages[p+"?page="+pg]++
		}
		denied := ev.denied[p]
		ev.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		get := r.Method == http.MethodGet
		deniedQuery := false
		for _, vs := range q {
			for _, v := range vs {
				deniedQuery = deniedQuery || v == bbDenied
			}
		}
		list := func(items ...string) string {
			return fmt.Sprintf(`{"data":%s,"meta":{"count":%d}}`, pagedArray(q, 100, items), len(items))
		}
		switch {
		case denied || deniedQuery || strings.Contains(p, "/"+bbDenied):
			ev.mu.Lock()
			ev.denials++
			ev.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"message":"invalid api key %s","errors":[{"detail":"bad key %s"}],"echo":%s}`,
				strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/events/"+bbRedirect:
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && p == "/events":
			fmt.Fprint(w, list(event(bbEventID, `,"echo":`+echo), event("7c1f2e3d-0000-4000-8000-000000000002", "")))
		case get && reEvent.MatchString(p):
			fmt.Fprintf(w, `{"data":%s,"meta":{"count":1}}`, event(last(p), `,"split":`+splitObject(sent)))
		case get && p == "/event_types":
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":7,"name":%q,"description":"HQ-Record PhishRIP","echo":%s}`, bbEventType, echo)))
		case get && p == "/statuses":
			fmt.Fprint(w, list(fmt.Sprintf(`{"id":%q,"details":{"events":[%q],"failures":[]},"processed":%q,"echo":%s}`, bbRequestID, bbEventID, recent, echo)))
		case get && reStatus.MatchString(p):
			fmt.Fprintf(w, `{"data":{"id":%q,"details":{"events":[%q],"failures":[]},"processed":%q,"echo":%s},"meta":{"count":1}}`, last(p), bbEventID, recent, echo)
		case r.Method == http.MethodPost && p == "/events":
			// A new event: its request id, and a secret the layer must hold.
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"data":{"id":%q,"secret":%q,"echo":%s},"meta":{"count":1}}`, bbRequestID, bbNewSecret,
				echoObject(union(sent, bbNewSecret)))
		case r.Method == http.MethodDelete && reEvent.MatchString(p):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"not found"}`)
		}
	}))
	ev.url = srv.URL
	return ev
}

// sawKey reports how many requests carried Authorization: Bearer v, and the
// distinct keys the fake received.
func (sv *server) sawKey(v string) (int, []string) {
	all := sv.receivedKeys()
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.keys[v], all
}

func (sv *server) resetMethods() {
	for _, s := range []*server{sv, sv.ev} {
		if s == nil {
			continue
		}
		s.mu.Lock()
		s.methods = nil
		s.mu.Unlock()
	}
}

// allMethods is every request both fakes received, the User Event ones
// prefixed "events ".
func (sv *server) allMethods() []string {
	sv.mu.Lock()
	out := append([]string{}, sv.methods...)
	sv.mu.Unlock()
	if sv.ev != nil {
		sv.ev.mu.Lock()
		for _, m := range sv.ev.methods {
			out = append(out, "events "+m)
		}
		sv.ev.mu.Unlock()
	}
	return out
}

type bbEnv struct {
	home string
	env  []string
}

// credMode is where the process gets its Reporting API key (the User Event
// key comes from the environment in every mode).
type credMode int

const (
	credStored        credMode = iota // none in the environment: the battery runs on a key stored with auth set-token
	credEnv                           // KNOWBE4_API_KEY
	credEnvOverStored                 // a stored key and KNOWBE4_API_KEY: the environment wins, the stored key is held unused
	credConfigFile                    // api_key in a 0600 config file named by KNOWBE4_CONFIG
	credConfigAccess                  // access_token alone in a 0600 config file: sent as the bearer key
	credConfigBoth                    // both config keys: api_key is sent, access_token is held unused
)

func (m credMode) String() string {
	return [...]string{"stored", "env KNOWBE4_API_KEY", "env over stored", "config api_key", "config access_token", "config both keys"}[m]
}

// sent is the Reporting key the mode sends; held are the credentials it holds
// but never sends anywhere.
func (m credMode) sent() string {
	return [...]string{bbStored, bbReportHeld, bbReportHeld, bbFileHeldA, bbFileHeldB, bbFileHeldA}[m]
}

func (m credMode) held() []string {
	switch m {
	case credEnvOverStored:
		return []string{bbStored, bbHTTPHeld}
	case credConfigBoth:
		return []string{bbFileHeldB, bbHTTPHeld}
	}
	return []string{bbHTTPHeld}
}

// newEnv builds a clean environment against the fakes with the credentials
// mode names; the stored modes run auth set-token first. The User Event key
// and its fake are set in every mode.
func newEnv(t *testing.T, sv *server, mode credMode) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	if sv.ev == nil {
		sv.ev = eventServer(t, sv)
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"KNOWBE4_BASE_URL=" + sv.url, "KNOWBE4_CA_CERT=" + sv.caFile, "SSL_CERT_FILE=" + sv.caFile,
		"KNOWBE4_USER_EVENT_API_KEY=" + bbEventHeld, "KNOWBE4_USER_EVENT_BASE_URL=" + sv.ev.url,
		"KNOWBE4_CLI_PATH=" + bbCLI, "KNOWBE4_MCP_HTTP_TOKEN=" + bbHTTPHeld,
	}
	e := &bbEnv{home: home, env: env}
	switch mode {
	case credStored:
		e.storeToken(t, bbStored)
	case credEnv:
		e.env = append(e.env, "KNOWBE4_API_KEY="+bbReportHeld)
	case credEnvOverStored:
		e.storeToken(t, bbStored)
		e.env = append(e.env, "KNOWBE4_API_KEY="+bbReportHeld)
	case credConfigFile:
		e.writeConfig(t, fmt.Sprintf("api_key = %q\n", bbFileHeldA))
	case credConfigAccess:
		e.writeConfig(t, fmt.Sprintf("access_token = %q\n", bbFileHeldB))
	case credConfigBoth:
		e.writeConfig(t, fmt.Sprintf("api_key = %q\naccess_token = %q\n", bbFileHeldA, bbFileHeldB))
	}
	return e
}

// writeConfig writes body as a 0600 config file and names it with
// KNOWBE4_CONFIG.
func (e *bbEnv) writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := filepath.Join(e.home, "cfg")
	_ = os.MkdirAll(dir, 0o700)
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "KNOWBE4_CONFIG"), "KNOWBE4_CONFIG="+cfg)
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
// is the kit contract's cross-record split, out of scope; one record is
// checked whole by the store's own write path, so a split inside a record
// still fails here).
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

// novels is every novel command; each reads the local store a sync filled
// (the recipients walk and the risk snapshots included).
func novels() [][]string {
	return [][]string{
		{"repeat-clickers", "--min-clicks", "2", "--json"},
		{"repeat-clickers"},
		{"risk-drift", "--json"},
		{"risk-drift"},
		{"untrained-clickers", "--json"},
		{"untrained-clickers"},
		{"coverage-gaps", "--json"},
		{"coverage-gaps", "--type", "training"},
		{"phish-prone-trend", "--json"},
		{"phish-prone-trend", "--group", "Finance"},
		{"risk-leaderboard", "--json"},
		{"risk-leaderboard", "--csv"},
		{"group-risk-contribution", "--json"},
		{"qbr", "--json"},
		{"qbr", "--format", "md"},
		{"report-rate", "--json"},
		{"report-rate", "--by-group", "--json"},
		{"freshness", "--json"},
		{"freshness"},
		{"analytics", "--type", "users", "--json"},
	}
}

// battery runs every output path; tok is the Reporting key the process holds
// (the key it sends: KNOWBE4_API_KEY, the stored key or a config file's
// api_key or access_token). The User Event key goes only into events
// commands, the Reporting key only into Reporting commands and previews, so
// neither key the user types reaches the other API's host.
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) []cliCall {
	b := calls(
		// Every output mode.
		[]string{"users", "list", "--json"},
		[]string{"users", "list"},
		[]string{"users", "list", "--csv"},
		[]string{"users", "list", "--plain"},
		[]string{"users", "list", "--agent"},
		[]string{"users", "list", "--json", "--receipt"},
		[]string{"users", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		[]string{"users", "list", "--json", "--deliver", "file:" + deliverFile},
		// Every response shape: a user's short password (gzip-encoded list),
		// split echo, the account integration's token, encryption key and
		// username (gzip), a store purchase's api_key and credentials
		// (BOM-prefixed), an echoed identifier, 401 echo, refused redirect,
		// a group's members split across two pages, KnowBe4 identities
		// (recipients' users, managers, admins).
		[]string{"users", "get", bbUserID, "--json"},
		[]string{"users", "get", bbUserID},
		[]string{"users", "get", bbDenied, "--json"},
		[]string{"users", "get", bbRedirect, "--json"},
		[]string{"users", "list", "--group-id", bbDenied, "--json"},
		[]string{"users", "risk-score-history", bbUserID, "--json"},
		[]string{"account", "info", "--json"},
		[]string{"account", "info"},
		[]string{"account", "risk-score-history", "--json"},
		[]string{"groups", "list", "--json"},
		[]string{"groups", "get", bbGroupID, "--json"},
		[]string{"groups", "members", bbGroupID, "--json"},
		[]string{"groups", "members", bbGroupID, "--all", "--json"},
		[]string{"groups", "members", bbGroupID, "--page", "2", "--per-page", "1", "--json"},
		[]string{"groups", "risk-score-history", bbGroupID, "--json"},
		[]string{"phishing-campaigns", "list", "--json"},
		[]string{"phishing-campaigns", "get", bbCampaignID, "--json"},
		[]string{"phishing-campaigns", "security-tests", bbCampaignID, "--json"},
		[]string{"phishing-tests", "list", "--json"},
		[]string{"phishing-tests", "list"},
		[]string{"phishing-tests", "get", bbPSTID, "--json"},
		[]string{"phishing-tests", "recipients", bbPSTID, "--json"},
		[]string{"phishing-tests", "recipients", bbPSTID, "--csv"},
		[]string{"phishing-tests", "recipient", bbPSTID, bbRecipient, "--json"},
		[]string{"training-campaigns", "list", "--json"},
		[]string{"training-campaigns", "get", bbTrainingID, "--json"},
		[]string{"training-enrollments", "list", "--json"},
		[]string{"training-enrollments", "get", bbEnrollID, "--json"},
		[]string{"store-purchases", "list", "--json"},
		[]string{"store-purchases", "list"},
		[]string{"store-purchases", "get", bbPurchaseID, "--json"},
		[]string{"policies", "list", "--json"},
		[]string{"policies", "get", bbPolicyID, "--json"},
		[]string{"tail", "users", "--follow=false"},
		// The User Event API (the second key): every read, its 401 echo and
		// refused redirect.
		[]string{"events", "list", "--json"},
		[]string{"events", "list"},
		[]string{"events", "list", "--csv"},
		[]string{"events", "get", bbEventID, "--json"},
		[]string{"events", "get", bbEventID},
		[]string{"events", "get", bbDenied, "--json"},
		[]string{"events", "get", bbRedirect, "--json"},
		[]string{"events", "types", "--json"},
		[]string{"events", "statuses", "--json"},
		[]string{"events", "status", bbRequestID, "--json"},
		// Writes the fake echoes (each with and without --yes; a write
		// without --yes is the work order's P2, but it must never leak
		// either). The User Event key typed into the description reaches only
		// its own host.
		[]string{"events", "create", "--target-user", bbClickEmail, "--event-type", bbEventType, "--source", "HQ-Record SOC",
			"--description", "HQ " + bbEventHeld, "--risk-level", "25", "--yes", "--json"},
		[]string{"events", "create", "--target-user", bbClickEmail, "--event-type", bbEventType, "--description", "HQ " + bbEventHeld},
		[]string{"events", "delete", bbEventID, "--yes", "--json"},
		[]string{"events", "delete", bbEventID},
		[]string{"events", "delete", bbDenied, "--yes", "--json"},
		// The local store, exports and the novel commands.
		[]string{"sync", "--json"},
		[]string{"workflow", "archive", "--json"},
		// store-purchases, not users: users is the export TestExportWalksEveryPage
		// pages through.
		[]string{"export", "store-purchases", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		[]string{"export", "users", "--format", "json"},
		[]string{"export", "phishing-tests", "--format", "json"},
		[]string{"search", "HQ", "--json"},
		[]string{"search", "Echo", "--json", "--data-source", "local"},
		[]string{"workflow", "status", "--json"},
	)
	b = append(b, calls(novels()...)...)
	return append(b, calls(
		// A sync that meets a 401 (a filtered walk into its own store, after
		// the novels: the main store stays whole).
		[]string{"sync", "--db", filepath.Join(e.home, "denied.db"), "--resources", "users", "--param", "status=" + bbDenied, "--json"},
		// A credential on the command line: a credential-named query value
		// (sent, and echoed back), the same in a dry run, an unknown flag, a
		// positional, feedback text.
		[]string{"sync", "--db", filepath.Join(e.home, "param.db"), "--resources", "policies", "--param", "password=" + bbArgvSec, "--json"},
		[]string{"sync", "--db", filepath.Join(e.home, "param.db"), "--resources", "policies", "--global-param", "password=" + bbArgvSec, "--dry-run"},
		[]string{"users", "list", "--json", "--" + tok},
		[]string{"users", "get", tok, "--json"},
		[]string{"events", "get", "--no-such-flag", bbEventHeld},
		[]string{"feedback", "noted the key " + tok + " in a ticket"},
		[]string{"feedback", "noted the event key " + bbEventHeld + " in a ticket"},
		// Previews: a dry run sends nothing.
		[]string{"events", "create", "--target-user", bbClickEmail, "--event-type", bbEventType, "--description", "Echo " + tok, "--dry-run"},
		[]string{"events", "delete", tok, "--dry-run"},
		[]string{"users", "get", tok, "--dry-run"},
		// Learnings, playbooks, teach logs and profiles land in local files.
		[]string{"teach", "--query", "find users " + tok, "--resource", bbUserID, "--resource-type", "users",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		[]string{"teach", "--query", "find " + tok, "--resource", tok, "--resource-type", "users"}, // a held resource id: refused, logged to teach.log
		[]string{"teach", "--query", "find events " + bbEventHeld, "--resource", bbUserID, "--resource-type", "users",
			"--notes", "rotated " + bbEventHeld, "--no-validate"},
		[]string{"teach-playbook", "--query", "users with " + tok, "--notes", "use " + tok},
		[]string{"profile", "save", "held", "--description", "uses " + tok + " and " + bbEventHeld, "--select", "id"},
		[]string{"profile", "show", "held"},
		[]string{"profile", "list"},
		[]string{"feedback", "list"},
		[]string{"learnings", "list", "--json"},
		[]string{"recall", "--query", "find users " + tok},
		[]string{"doctor"},
		[]string{"auth", "status"},
		[]string{"agent-context"},
	)...)
}

func tool(name string, args map[string]any) map[string]any {
	return map[string]any{"name": name, "arguments": args}
}

// pageCursor is the opaque MCP cursor a typed list tool hands back for the
// API's page n (the bound package's endpointCursor: {"v":1,"u":"<page>"}).
func pageCursor(n int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"v":1,"u":"%d"}`, n)))
}

// mcpReads are read-only tools across every response shape the fakes have,
// and every novel tool. The group members (the first half of a split
// credential, page 1 of two) run in their own session
// (TestMCPSplitAcrossPagesWithheld): a delivered half makes the history check
// withhold later results of the session whole (the P2
// TestMCPSessionHalfWithholdsLaterResultsKnownP2), which would hide every
// preview this battery checks.
func mcpReads() []map[string]any {
	none := map[string]any{}
	return []map[string]any{
		tool("account_info", none),
		tool("account_risk-score-history", none),
		tool("users_list", none),
		tool("users_get", map[string]any{"id": bbUserID}),
		tool("users_get", map[string]any{"id": bbDenied}),
		tool("users_risk-score-history", map[string]any{"id": bbUserID}),
		tool("groups_list", none),
		tool("groups_get", map[string]any{"id": bbGroupID}),
		tool("groups_members", map[string]any{"id": bbGroup2}),
		tool("groups_risk-score-history", map[string]any{"id": bbGroupID}),
		tool("phishing-campaigns_list", none),
		tool("phishing-campaigns_get", map[string]any{"id": bbCampaignID}),
		tool("phishing-campaigns_security-tests", map[string]any{"id": bbCampaignID}),
		tool("phishing-tests_list", none),
		tool("phishing-tests_get", map[string]any{"id": bbPSTID}),
		tool("phishing-tests_recipients", map[string]any{"id": bbPSTID}),
		tool("phishing-tests_recipient", map[string]any{"pst-id": bbPSTID, "recipient-id": bbRecipient}),
		tool("training-campaigns_list", none),
		tool("training-campaigns_get", map[string]any{"id": bbTrainingID}),
		tool("training-enrollments_list", none),
		tool("training-enrollments_get", map[string]any{"id": bbEnrollID}),
		tool("store-purchases_list", none),
		tool("store-purchases_get", map[string]any{"id": bbPurchaseID}),
		tool("policies_list", none),
		tool("policies_get", map[string]any{"id": bbPolicyID}),
		tool("events_list", none),
		tool("events_get", map[string]any{"event-id": bbEventID}),
		tool("events_get", map[string]any{"event-id": bbDenied}),
		tool("events_types", none),
		tool("events_statuses", none),
		tool("events_status", map[string]any{"request-id": bbRequestID}),
		tool("context", none),
		tool("repeat_clickers", none),
		tool("risk_drift", none),
		tool("untrained_clickers", none),
		tool("coverage_gaps", none),
		tool("phish_prone_trend", none),
		tool("risk_leaderboard", none),
		tool("group_risk_contribution", none),
		tool("qbr", none),
		tool("report_rate", none),
		tool("freshness", none),
		tool("analytics", map[string]any{"type": "users"}),
		tool("tail", map[string]any{"resource": "users", "follow": false}),
		tool("search", map[string]any{"query": "HQ"}),
		tool("sql", map[string]any{"query": "SELECT data FROM resources"}),
		tool("recall", map[string]any{"query": "HQ"}),
		tool("learnings_list", none),
		tool("learnings_candidates", none),
		tool("learnings_stats", none),
		tool("playbook_list", none),
		tool("workflow_status", none),
	}
}

// mcpWrites is every write that takes confirm, called without it: each must
// answer with a preview and send nothing. The mirrored writes (events_create,
// events_delete and the events parent) preview as the command's --dry-run
// output followed by the gate's "Preview only: nothing was sent" note. The
// create's description repeats the User Event key, which its preview must
// withhold.
func mcpWrites() []map[string]any {
	return []map[string]any{
		tool("events_create", map[string]any{"target-user": bbClickEmail, "event-type": bbEventType, "source": "HQ-Record SOC",
			"description": "Preview HQ-Record", "risk-level": 25}),
		tool("events_delete", map[string]any{"event-id": bbEventID}),
		tool("events", map[string]any{}),
	}
}

// mcpBypasses try to turn a write preview into a send without confirm true:
// each must preview or be refused, and send nothing.
func mcpBypasses() []map[string]any {
	return []map[string]any{
		tool("events_delete", map[string]any{"event-id": bbEventID, "dry-run": false}),
		tool("events_create", map[string]any{"target-user": bbClickEmail, "event-type": bbEventType, "dry-run": "false"}),
		tool("events_delete", map[string]any{"event-id": bbEventID, "yes": true}),
		// A write whose argument repeats the held event key: refused (a
		// credential is never passed to a tool) or previewed, never sent.
		tool("events_create", map[string]any{"target-user": bbClickEmail, "event-type": bbEventType, "source": "HQ-Record SOC",
			"description": "Preview " + bbEventHeld, "risk-level": 25}),
	}
}

// mcpLocalWrites are mirrored tools that write only local files (no gate by
// design): their child CLI output passes the server's chokepoint too. The
// sync's credential-named query value is sent, and echoed back.
func mcpLocalWrites() []map[string]any {
	return []map[string]any{
		tool("export", map[string]any{"resource": "store-purchases", "format": "json"}),
		tool("sync", map[string]any{"resources": []any{"users", "phishing-tests"}}),
		tool("sync", map[string]any{"resources": []any{"policies"}, "param": []any{"password=" + bbMCPArgSec}}),
	}
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
		if strings.HasPrefix(rel, "cfg"+string(filepath.Separator)) {
			return nil // the test's own inputs
		}
		scanned++
		fn(rel, data)
		return nil
	})
	return scanned
}

// checkKeysStayHome fails when a key reached the other API's host, or a
// request authenticated any way but Authorization: Bearer <key>.
func checkKeysStayHome(t *testing.T, sv *server) {
	t.Helper()
	for name, s := range map[string]*server{"the Reporting API": sv, "the User Event API": sv.ev} {
		s.mu.Lock()
		if s.foreign != 0 {
			t.Errorf("%s received the other API's key in %d request(s)", name, s.foreign)
		}
		if s.otherAuth != 0 {
			t.Errorf("%d request(s) to %s carried a credential another way (a second or non-Bearer Authorization header, X-Api-Key, a query key); KnowBe4 takes only Authorization: Bearer <key>", s.otherAuth, name)
		}
		s.mu.Unlock()
	}
	if n, all := sv.ev.sawKey(bbEventHeld); len(all) > 0 && (n == 0 || len(all) != 1) {
		t.Errorf("the User Event API received %d request(s) with its key and distinct keys %q", n, all)
	}
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, c := range battery(t, e, deliverFile, bbStored) {
		e.runIn(t, c.stdin, c.args...)
	}
	// Controls without a credential: the same sinks write, so the scans
	// below are not vacuous.
	e.run(t, "feedback", "control note HQ-Feedback")
	e.run(t, "teach", "--query", "find users control", "--resource", bbUserID, "--resource-type", "users",
		"--notes", "rotated control", "--no-validate")
	e.run(t, "teach", "--resource", bbUserID, "--resource-type", "users") // no --query: teach.log records the refusal
	e.run(t, "profile", "save", "control", "--description", "HQ-Profile control", "--select", "id")
	e.mcpCalls(t, mcpBattery())
	// The kit withholds a whole unit that repeats a held key, so the echoing
	// runs above may leave a sink without its record. Non-echoing runs write
	// the data the checks below look for (and the scan covers them too).
	sv.setQuiet(true)
	for _, args := range [][]string{
		{"sync", "--json"},
		{"users", "list", "--json", "--deliver", "file:" + deliverFile},
		{"export", "store-purchases", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
	} {
		e.run(t, args...)
	}
	if n, all := sv.sawKey(bbStored); n == 0 {
		t.Fatalf("no request carried the stored key as Authorization: Bearer (saw %d distinct values); the battery ran unauthenticated", len(all))
	}
	if n, _ := sv.ev.sawKey(bbEventHeld); n == 0 {
		t.Fatalf("no request carried the User Event key to its host; the events battery did not run")
	}
	checkKeysStayHome(t, sv)
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
		if !synccpuStoreFile(rel) { // synccpu: the local store is not checked
			assertNoEchoIn(t, "file "+rel, string(data), !isStoreFile(rel))
		}
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-token wrote no credential store")
	}
	// The battery really wrote data, so the scan above is not vacuous.
	if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
	}
	if b, err := os.ReadFile(filepath.Join(e.home, "export.jsonl")); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("export.jsonl: want the exported record (err %v):\n%.1500s", err, b)
	}
	var sawStore, sawUser, sawRecipient, sawLearning, sawFeedback, sawProfile, sawTeachLog bool
	scanFiles(e.home, func(rel string, b []byte) {
		name := filepath.Base(rel)
		if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") {
			sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
			// The user record was stored (its password withheld).
			sawUser = sawUser || bytes.Contains(b, []byte("HQ-Record Pat"))
			// The recipients walk stored a recipient row.
			sawRecipient = sawRecipient || bytes.Contains(b, []byte(bbClickEmail))
			// The control teach run's learning row was written.
			sawLearning = sawLearning || bytes.Contains(b, []byte("rotated control"))
		}
		// The control feedback entry was written.
		sawFeedback = sawFeedback || (name == "feedback.jsonl" && bytes.Contains(b, []byte("HQ-Feedback")))
		sawProfile = sawProfile || bytes.Contains(b, []byte("HQ-Profile control"))
		sawTeachLog = sawTeachLog || (name == "teach.log" && bytes.Contains(b, []byte("missing --query")))
	})
	for what, ok := range map[string]bool{
		"no local store holds a fetched record":                                              sawStore,
		"no local store holds the synced user":                                               sawUser,
		"no local store holds a phishing recipient; the insights walk did not run":           sawRecipient,
		"no local store holds the taught learning; the teach run wrote nothing":              sawLearning,
		"no feedback.jsonl holds the control feedback entry; the feedback run wrote nothing": sawFeedback,
		"no profile file holds the control profile; profile save wrote nothing":              sawProfile,
		"no teach.log holds the control refusal; teach wrote no log":                         sawTeachLog,
	} {
		if !ok {
			t.Error(what)
		}
	}
	// Some pager walked every page: the split-across-pages echo really ran;
	// and the 401 echoes really ran (users get, users list --group-id, the
	// denied sync, MCP users_get, events get and delete, MCP events_get).
	sv.mu.Lock()
	page2 := sv.pages["/groups/"+bbGroupID+"/members?page=2"]
	denials := sv.denials
	sv.mu.Unlock()
	sv.ev.mu.Lock()
	denials += sv.ev.denials
	sv.ev.mu.Unlock()
	if denials < 6 {
		t.Errorf("only %d request(s) met a 401; the error-echo runs did not all reach the fakes", denials)
	}
	if page2 == 0 {
		t.Errorf("no pager asked for the second page of the group's members; the split-across-pages echo did not run")
	}
}

// Class secret-in-output, once per credential source the Reporting key can
// come from that is not the stored key alone: the KNOWBE4_API_KEY variable, over a stored
// key (held unused), a config file's api_key, a config file's access_token
// (sent alone) and both config keys (access_token held unused); the User
// Event key comes from the environment in each. (The stored key as the sent
// value runs in TestNoCredentialInAnyWrittenFile;
// TestEveryCredentialSourceSendsOneKey covers every source with a short
// battery.)
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	// One source keeps the package fast: it holds a key it does not send.
	// Every source runs in TestEveryCredentialSourceSendsOneKey.
	for _, mode := range []credMode{credConfigBoth} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			for _, c := range battery(t, e, deliverFile, mode.sent()) {
				assertNoEcho(t, fmt.Sprintf("output of %v", c.args), e.runIn(t, c.stdin, c.args...))
			}
			// A held value no API receives, typed where the program repeats
			// it: a path the Reporting fake echoes, and previews.
			for _, held := range mode.held() {
				for _, c := range []cliCall{
					{args: []string{"users", "get", held, "--json"}},
					{args: []string{"events", "create", "--target-user", bbClickEmail, "--event-type", bbEventType, "--description", "HQ " + held, "--dry-run"}},
					{args: []string{"events", "delete", held, "--dry-run"}},
				} {
					assertNoEcho(t, fmt.Sprintf("output of %v", c.args), e.runIn(t, c.stdin, c.args...))
				}
			}
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			// With the echo off the data prints, JSON output is valid JSON, and
			// KnowBe4 data under an identity name (the user's last_sign_in and
			// manager_email) is not withheld.
			sv.setQuiet(true)
			for _, args := range [][]string{{"users", "list", "--json"}, {"events", "list", "--json"}} {
				cmd := exec.Command(bbCLI, args...)
				cmd.Env, cmd.Dir = e.env, e.home
				stdout, err := cmd.Output()
				if err != nil {
					t.Fatalf("%v: %v", args, err)
				}
				want := []*regexp.Regexp{regexp.MustCompile(`HQ-Record Pat`), publicField("last_sign_in", bbLastSignIn), publicField("manager_email", bbMgrEmail)}
				if args[0] == "events" {
					want = []*regexp.Regexp{publicField("target_user", bbClickEmail), publicField("event_type", bbEventType)}
				}
				var doc any
				ok := json.Unmarshal(stdout, &doc) == nil
				for _, re := range want {
					ok = ok && re.Match(stdout)
				}
				if !ok {
					t.Fatalf("%v: want valid JSON with the record and its public fields, got:\n%.1500s", args, stdout)
				}
				assertNoEcho(t, fmt.Sprintf("output of %v", args), string(stdout))
			}
			// The fakes echoed the keys they really received, so the scans
			// above covered them; no other credential source was sent, and
			// neither key reached the other API's host.
			if n, all := sv.sawKey(mode.sent()); n == 0 || len(all) != 1 {
				t.Fatalf("want every Reporting request to carry the %s key, got %d of it and distinct values %q", mode, n, all)
			}
			checkKeysStayHome(t, sv)
		})
	}
}

// publicField matches name and value in JSON output, raw or inside a JSON
// string (an MCP result escapes its quotes).
func publicField(name, value string) *regexp.Regexp {
	return regexp.MustCompile(`\\*"` + regexp.QuoteMeta(name) + `\\*":\s*\\*"?` + regexp.QuoteMeta(value))
}

// knowbe4-credential-names-exact: KnowBe4 data under names that read as
// credentials or carry identities prints as the API returned it, from the CLI
// and MCP alike, and the store keeps it (the recipients walk reads the
// recipient's user): a phishing recipient's user object and its email, a
// user's last_sign_in and manager_email, the account admins' emails, an
// event's risk_level, event_type and target_user.
func TestKnowBe4PublicFieldsAreNotWithheld(t *testing.T) {
	t.Parallel()
	sv := quietServer(t)
	e := newEnv(t, sv, credEnv)
	var cli strings.Builder
	for _, args := range [][]string{
		{"phishing-tests", "recipients", bbPSTID, "--json"},
		{"phishing-tests", "recipient", bbPSTID, bbRecipient, "--json"},
		{"users", "get", bbUserID, "--json"},
		{"users", "list", "--json"},
		{"account", "info", "--json"},
		{"events", "list", "--json"},
		{"events", "get", bbEventID, "--json"},
	} {
		cli.WriteString(e.run(t, args...))
	}
	// users_list carries the user's public fields over MCP: users_get and
	// phishing-tests_recipient answer records that also split a held key
	// across adjacent values, and a compact one-line result loses its
	// neighbouring public fields to that split (the P2
	// TestMCPSplitRecordWithholdsItsPublicFieldsKnownP2); both still run for
	// the no-echo check.
	mcp := e.mcpCalls(t, []map[string]any{
		tool("phishing-tests_recipients", map[string]any{"id": bbPSTID}),
		tool("phishing-tests_recipient", map[string]any{"pst-id": bbPSTID, "recipient-id": bbRecipient}),
		tool("users_get", map[string]any{"id": bbUserID}),
		tool("users_list", map[string]any{}),
		tool("account_info", map[string]any{}),
		tool("events_list", map[string]any{}),
		tool("events_get", map[string]any{"event-id": bbEventID}),
	})
	for where, got := range map[string]string{"CLI": cli.String(), "MCP": mcp} {
		for _, want := range []*regexp.Regexp{
			regexp.MustCompile(`\\*"user\\*":\s*\{[^{}]*` + regexp.QuoteMeta(bbClickEmail)),
			publicField("email", bbClickEmail),
			publicField("last_sign_in", bbLastSignIn),
			publicField("manager_email", bbMgrEmail),
			publicField("email", bbAdminEmail),
			publicField("risk_level", "25"),
			publicField("event_type", bbEventType),
			publicField("target_user", bbClickEmail),
		} {
			if !want.MatchString(got) {
				t.Errorf("%s withheld KnowBe4 data (want %s):\n%.3000s", where, want, got)
			}
		}
		assertNoEcho(t, where, got)
	}
	// The store keeps it too: the synced users' managers and the recipients
	// walk's user emails.
	e.run(t, "sync", "--resources", "users,phishing-tests", "--json")
	var manager, recipient bool
	scanFiles(e.home, func(rel string, b []byte) {
		if isStoreFile(rel) {
			manager = manager || publicField("manager_email", bbMgrEmail).Match(b)
			recipient = recipient || bytes.Contains(b, []byte(bbReportEmail))
		}
	})
	if !manager {
		t.Errorf("the local store dropped the users' manager_email")
	}
	if !recipient {
		t.Errorf("the local store dropped the phishing recipients' user emails")
	}
}

// Class credential-sent-to-another-host: a redirect off either API host is
// refused before it is followed, from the CLI, sync and MCP alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	sv := quietServer(t)
	e := newEnv(t, sv, credEnv)
	redirect := e.run(t, "users", "get", bbRedirect, "--json")
	eventRedirect := e.run(t, "events", "get", bbRedirect, "--json")
	sv.mu.Lock()
	sv.redirectUsers = true
	sv.mu.Unlock()
	syncOut := e.run(t, "sync", "--resources", "users", "--json")
	e.mcpCalls(t, []map[string]any{
		tool("users_get", map[string]any{"id": bbRedirect}),
		tool("users_list", map[string]any{}),
		tool("events_get", map[string]any{"event-id": bbRedirect}),
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		if strings.HasSuffix(m, "/v1/users/"+bbRedirect) || strings.HasSuffix(m, "GET /v1/users") {
			redirected++
		}
	}
	sv.mu.Unlock()
	sv.ev.mu.Lock()
	n += sv.ev.offHost
	for _, m := range sv.ev.methods {
		if strings.HasSuffix(m, "/events/"+bbRedirect) {
			redirected++
		}
	}
	sv.ev.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if redirected < 6 {
		t.Fatalf("only %d request(s) met a redirect; the CLI, sync and MCP calls did not all run", redirected)
	}
	for where, out := range map[string]string{"users get": redirect, "events get": eventRedirect, "sync": syncOut} {
		if !strings.Contains(out, "leave the configured KnowBe4 host") {
			t.Fatalf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
	}
}

// Class credential-sent-to-another-host, region form (region-origin-pinned,
// user-event-origin-pinned): with KNOWBE4_BASE_URL unset the Reporting host
// is https://{region}.api.knowbe4.com, the region from KNOWBE4_REGION
// (default us). A region that is not one DNS label (it would move the host:
// "evil.example.com/x?", "x.attacker") and a region a call supplies later
// (sync --path-context region=..., the MCP sync tool's argument) are refused
// before any dial. A proxy trap stands in for the network: the transport
// sends every dial through HTTPS_PROXY, so a request that got past the pin
// would reach the trap, never a real host. The same hostile regions cannot
// move the User Event host: its client falls back to
// https://api.events.knowbe4.com, asserted on the base URL the client
// resolves (never dialled), and an explicit KNOWBE4_USER_EVENT_BASE_URL that
// would carry the key in userinfo, a query or a fragment is refused.
func TestRegionNeverMovesTheKey(t *testing.T) {
	var mu sync.Mutex
	var dials []string
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		dials = append(dials, r.Method+" "+r.Host)
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(trap.Close)
	home := t.TempDir()
	base := cleanEnv(home, "KNOWBE4_API_KEY="+bbReportHeld, "KNOWBE4_USER_EVENT_API_KEY="+bbEventHeld, "KNOWBE4_CLI_PATH="+bbCLI,
		"KNOWBE4_MCP_HTTP_TOKEN="+bbHTTPHeld, "HTTPS_PROXY="+trap.URL, "HTTP_PROXY="+trap.URL, "NO_PROXY=", "no_proxy=")
	hostile := []string{"evil.example.com/x?", "x.attacker", "a@b", "evil.example#"}
	var out strings.Builder
	for _, region := range hostile {
		env := append(append([]string{}, base...), "KNOWBE4_REGION="+region)
		got := runWith(t, env, "users", "list", "--json")
		if !strings.Contains(got, "leave the configured KnowBe4 host") {
			t.Errorf("KNOWBE4_REGION=%q: want the pin refusal, got:\n%.1000s", region, got)
		}
		out.WriteString(got)
	}
	for _, region := range []string{"evil", "evil.example.com/x?"} {
		got := runWith(t, base, "sync", "--resources", "users", "--path-context", "region="+region, "--json")
		if !strings.Contains(got, "leave the configured KnowBe4 host") {
			t.Errorf("sync --path-context region=%q: want the pin refusal, got:\n%.1000s", region, got)
		}
		out.WriteString(got)
	}
	e := &bbEnv{home: home, env: base}
	mcp := e.mcpCalls(t, []map[string]any{
		tool("sync", map[string]any{"resources": []any{"users"}, "path-context": []any{"region=evil"}}),
		tool("sync", map[string]any{"resources": []any{"users"}, "path-context": []any{"region=evil.example.com/x?"}}),
	})
	if strings.Count(mcp, "leave the configured KnowBe4 host") < 2 {
		t.Errorf("the MCP sync tool with a region argument: want the pin refusal twice, got:\n%.2000s", mcp)
	}
	out.WriteString(mcp)
	// The User Event host for the documented regions and every hostile one
	// (the hostile ones fall back to the US host).
	want := map[string]string{"": "https://api.events.knowbe4.com", "us": "https://api.events.knowbe4.com",
		"eu": "https://api-eu.events.knowbe4.com", "ca": "https://api-ca.events.knowbe4.com",
		"uk": "https://api-uk.events.knowbe4.com", "de": "https://api-de.events.knowbe4.com"}
	for _, region := range append(hostile, "api.evil.example", "eu/../x", "EU.evil") {
		if _, ok := want[region]; !ok {
			want[region] = "https://api.events.knowbe4.com"
		}
	}
	for region, w := range want {
		if got := userEventBaseURL(t, region, ""); got != w {
			t.Errorf("KNOWBE4_REGION=%q: the User Event client resolves %q, want %q", region, got, w)
		}
	}
	for _, override := range []string{"https://ro:" + bbStored + "@events.example.com", "https://events.example.com/?token=" + bbStored,
		"https://events.example.com/#" + bbStored, "ftp://events.example.com"} {
		if got := userEventBaseURL(t, "", override); got != "" {
			t.Errorf("KNOWBE4_USER_EVENT_BASE_URL=%q was accepted (%q)", override, got)
		}
	}
	// End to end, the hostile region with the event key: the events command
	// refuses an override that would carry a credential, printing neither.
	for _, override := range []string{"https://ro:" + bbStored + "@events.example.com", "https://events.example.com/?token=" + bbStored} {
		env := append(append([]string{}, base...), "KNOWBE4_REGION=evil.example.com/x?", "KNOWBE4_USER_EVENT_BASE_URL="+override)
		got := runWith(t, env, "events", "list", "--json")
		if !strings.Contains(got, "KNOWBE4_USER_EVENT_BASE_URL") {
			t.Errorf("KNOWBE4_USER_EVENT_BASE_URL=%q: want the override refusal, got:\n%.1000s", override, got)
		}
		out.WriteString(got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(dials) != 0 {
		t.Errorf("a request got past the pin and dialed out (through the proxy trap): %v", dials)
	}
	assertNoEcho(t, "region refusals", out.String())
}

// userEventBaseURL is the base URL the User Event client resolves for a
// region and a KNOWBE4_USER_EVENT_BASE_URL override ("" when the client
// refuses the configuration). It builds the client in process and never
// sends a request.
func userEventBaseURL(t *testing.T, region, override string) string {
	t.Helper()
	t.Setenv("KNOWBE4_USER_EVENT_API_KEY", bbEventHeld)
	t.Setenv("KNOWBE4_REGION", region)
	t.Setenv("KNOWBE4_USER_EVENT_BASE_URL", override)
	c, err := userevents.New(time.Second)
	if err != nil {
		return ""
	}
	return c.BaseURL()
}

// mcpSplitHalf is the page-1 read of the group whose members split the first
// credential across two pages.
func mcpSplitHalf() map[string]any { return tool("groups_members", map[string]any{"id": bbGroupID}) }

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to either API is a read (a
// GET) while no call sets confirm, though the battery calls every write
// (events_create, events_delete, the events parent) and tries to bypass the
// gate (dry-run false, a blocked --yes); each write answers with a preview.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	t.Parallel()
	sv := quietServer(t)
	e := newEnv(t, sv, credEnv)
	// The store holds the synced tenant first, so the novels have something
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
		if r := replies[id]; !strings.Contains(r, "Preview only: nothing was sent") {
			t.Errorf("%s %v without confirm did not return a preview:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	for i, call := range mcpBypasses() {
		id := len(mcpReads()) + len(mcpWrites()) + i + 1
		if r := replies[id]; r == "" || (!strings.Contains(r, "Preview only: nothing was sent") && !strings.Contains(r, `"isError":true`)) {
			t.Errorf("%s %v was neither previewed nor refused:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	// The novels answered from the synced store and the reads from the fakes
	// (a refusal would leave the leak checks vacuous for them).
	for i, call := range mcpReads() {
		want := map[any]string{
			"users_list": "HQ-Record Sam", "phishing-tests_recipients": bbReportEmail, "events_list": bbEventType,
			"repeat_clickers": bbClickEmail, "untrained_clickers": bbClickEmail, "risk_leaderboard": bbClickEmail,
			"store-purchases_list": "HQ-Record Phishing 101", "account_info": bbAdminEmail,
		}[call["name"]]
		if r := replies[i+1]; want != "" && !strings.Contains(r, want) {
			t.Errorf("%s %v read nothing (want %q):\n%.1500s", call["name"], call["arguments"], want, r)
		}
	}
	for _, m := range sv.allMethods() {
		if isWrite(m) || strings.Contains(m, "POST") || strings.Contains(m, "DELETE") {
			t.Errorf("the MCP server sent %s without confirm", m)
		}
	}
	checkKeysStayHome(t, sv)
}

// Control for the test above: with confirm set to boolean true the same write
// reaches the User Event API exactly once, so the gate previews rather than
// drops; the string "true" and the number 1 still preview.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	write := func(confirm any) []map[string]any {
		return []map[string]any{
			tool("events_delete", map[string]any{"event-id": bbEventID, "confirm": confirm}),
			tool("events_create", map[string]any{"target-user": bbClickEmail, "event-type": bbEventType, "confirm": confirm}),
		}
	}
	previews := e.mcpPhases(t, write("true"), write(1))
	for _, m := range sv.allMethods() {
		if isWrite(m) || strings.Contains(m, "POST") || strings.Contains(m, "DELETE") {
			t.Errorf("confirm \"true\" or 1 sent %s", m)
		}
	}
	replies := mcpReplies(previews)
	for id := 1; id <= 4; id++ {
		if !strings.Contains(replies[id], "Preview only: nothing was sent") {
			t.Errorf("call %d did not preview:\n%.1000s", id, replies[id])
		}
	}
	out := e.mcpPhases(t, write(true))
	var del, create int
	for _, m := range sv.allMethods() {
		if m == "events DELETE /events/"+bbEventID {
			del++
		}
		if m == "events POST /events" {
			create++
		}
	}
	if del != 1 || create != 1 {
		t.Errorf("confirm true: want exactly one DELETE and one POST at the User Event API, got %d and %d; requests: %v\n%.1500s", del, create, sv.allMethods(), out)
	}
	assertNoEcho(t, "MCP confirmed writes", out)
	checkKeysStayHome(t, sv)
}

// dry-run-sends-nothing: a --dry-run never reaches either API, from a cold
// start with the key in the environment, the credential store or a config
// file (api_key, or access_token alone).
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	for _, mode := range []credMode{credEnv, credStored, credConfigFile, credConfigAccess} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		for _, args := range [][]string{
			{"events", "create", "--target-user", bbClickEmail, "--event-type", bbEventType, "--source", "Dry", "--risk-level", "25", "--dry-run"},
			{"events", "create", "--target-user", bbClickEmail, "--event-type", bbEventType, "--yes", "--json", "--dry-run"},
			{"events", "delete", bbEventID, "--dry-run"},
			{"events", "delete", bbEventID, "--yes", "--dry-run"},
			{"users", "list", "--dry-run"},
			{"users", "get", bbUserID, "--dry-run"},
			{"groups", "members", bbGroupID, "--all", "--dry-run"},
			{"phishing-tests", "recipients", bbPSTID, "--dry-run"},
			{"account", "info", "--dry-run"},
			{"sync", "--resources", "users", "--dry-run"},
		} {
			// Each run reaches its preview, so the count below is not vacuous
			// (a flag error would send nothing either).
			if out := e.run(t, args...); !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") && !strings.Contains(out, "would ") {
				t.Errorf("%v (mode %s) printed no preview:\n%.1500s", args, mode, out)
			}
		}
		// P2 (not a leak): the User Event reads print nothing at all under
		// --dry-run (events.go returns before any output). They must still
		// send nothing, and print no error either (a flag error would also
		// send nothing).
		for _, args := range [][]string{{"events", "list", "--dry-run"}, {"events", "get", bbEventID, "--dry-run"}} {
			if out := strings.TrimSpace(e.run(t, args...)); out != "" && !strings.Contains(out, "dry") {
				t.Errorf("%v (mode %s) printed something other than a preview:\n%.1500s", args, mode, out)
			}
		}
		if m := sv.allMethods(); len(m) != 0 {
			t.Errorf("--dry-run (mode %s) sent %v", mode, m)
		}
		// Control: the same write and read without --dry-run reach the fakes.
		e.run(t, "events", "delete", bbEventID, "--yes")
		e.run(t, "users", "get", bbUserID, "--json")
		if m := sv.allMethods(); len(m) != 2 || m[0] != "GET /v1/users/"+bbUserID || m[1] != "events DELETE /events/"+bbEventID {
			t.Errorf("control (mode %s): want the one GET and the one DELETE, got %v", mode, m)
		}
	}
}

// credential-split-across-output-records over MCP: the fake splits the first
// credential it was sent across the two pages of a group's members, one half
// each. One session reads both pages; the halves must never both reach the
// client.
func TestMCPSplitAcrossPagesWithheld(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{mcpSplitHalf()},
		[]map[string]any{tool("groups_members", map[string]any{"id": bbGroupID, "cursor": pageCursor(2)})})
	if !strings.Contains(out, `"id":2`) {
		t.Fatalf("MCP session did not answer both pages:\n%.2000s", out)
	}
	sv.mu.Lock()
	page2 := sv.pages["/groups/"+bbGroupID+"/members?page=2"]
	sv.mu.Unlock()
	if page2 == 0 {
		t.Fatalf("the second page was never asked for; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
}
