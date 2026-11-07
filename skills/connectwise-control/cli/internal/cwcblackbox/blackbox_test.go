// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package cwcblackbox runs the built CLI and MCP binaries against a
// ConnectWise Control (ScreenConnect) fake over TLS and checks the connector
// behavior a reviewer cannot read off the code (connectwise-control handfixes
// api-origin-pinned, dry-run-sends-nothing, mcp-endpoint-write-confirm,
// mcp-mirror-write-confirm, export-file-closed-on-error):
//
//   - no credential leaves for a host other than the configured instance by
//     redirect, and nothing at all leaves for a non-loopback host (every
//     process runs behind a recording proxy that answers 502);
//   - the MCP server sends a write (any POST but the documented reads:
//     GetLiveData, GetSessionDetails, GetSecurityConfigurationInfo,
//     GetAuditInfo, QueryAuditLog) only when the call sets confirm to the
//     boolean true, and --dry-run sends nothing at all;
//   - the generated commands run on the configured credentials, export and
//     tail read the records, and the session nouns print.
//
// What the program prints, writes and sends is held by the shared credential
// kit (handfixes.json credkit-wiring); its leak tests are internal/credwire's.
package cwcblackbox

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

// The credentials are distinct and long enough to be found by value; the
// stored password is shaped like a generated key (uppercase letters and
// digits), so ordinary output never spells it by chance.
const (
	bbUser       = "apiuser-3c9e1b7a5d2f80aa4e@nodomain.invalid"   // CONNECTWISE_CONTROL_USERNAME (the Basic user name)
	bbPass       = "Zq7Kx2Lr9Wm4Tk8Pv3Hs6N"                        // CONNECTWISE_CONTROL_PASSWORD (the Basic password)
	bbStoredUser = "stored-user-71c3e9b05d2a4f@cwc-api.example"    // auth set-credentials <user>: the credential store
	bbStoredPass = "ST71C3E9B05D2A4F68E1QKZW7"                     // auth set-credentials <password>: the credential store
	bbFileUser   = "config-file-user-2b9d7e4a1c63@cwc-api.example" // control_username in a 0600 config file named by CONNECTWISE_CONTROL_CONFIG
	bbFilePass   = "Fs9A4c1E7b3D50Qw8Rt2Yx"                        // control_password in that file
	bbArgvUser   = "argv-user-5e2d9c1a7b04@cwc-api.example"        // auth set-credentials --dry-run <user> on the command line
	bbArgvPass   = "-argvPassw0rd-6b2e8f14"                        // auth set-credentials --dry-run <user> <password>: a password that starts with "-"
	bbArgvSec    = "-argvSecret-91c3e7a5"                          // security save-user --password and an unknown --secret on the command line (starts with "-")
	bbMCPArgSec  = "mcpArgSecret-81d3c6f0"                         // a credential-named argument an MCP call previews (never sent)
	bbShortResp  = "Sh0rtPw"                                       // a 7-character user password a response carries
	bbBOMResp    = "BomRespSecret-6a1c"                            // a credential in a BOM-prefixed response
	bbGzipResp   = "GzipRespToken-4f2a9c1e"                        // a credential nested in a record of a gzip-encoded response
	bbNewSecret  = "newUserPassword-0b7e3d5a9c1f"                  // the initial password a save-user response carries
	bbAccessTok  = "accessTok-cQ7vZr2k6d0a2e8c4f71Wm"              // the access grant GetAccessToken issues (accessToken, and Token= in its join url)
	bbHTTPToken  = "mcpHttpToken-0123456789abcdefABCDEF"           // CONNECTWISE_CONTROL_MCP_HTTP_TOKEN
	bbSessionID  = "6f9a2c41-3b7e-4d15-9a0c-2e8f71b4c5d3"          // a session (its id is public: it must print)
	bbRedirect   = "99110000-0000-4000-8000-00000000a1b2"          // a session the fake redirects off-host
	bbDenied     = "40199000-0000-4000-8000-00000000c3d4"          // a session id or session name the fake answers 401
	bbPlainUser  = "hq.tech2"                                      // a user name that is no credential: it must print
	bbGroup      = "HQ-Record group"                               // a session group (the sync key is its name)
)

// basicPairs are the (user, password) pairs a credential mode sends.
var basicPairs = []creds{{bbStoredUser, bbStoredPass}, {bbUser, bbPass}, {bbUser, bbStoredPass}, {bbFileUser, bbFilePass}, {bbFileUser, bbPass}}

// basicPayload is the Basic header's credential: base64 of user:password.
func basicPayload(c creds) string {
	return base64.StdEncoding.EncodeToString([]byte(c.user + ":" + c.pass))
}

func secrets() []string {
	s := []string{bbUser, bbPass, bbStoredUser, bbStoredPass, bbFileUser, bbFilePass, bbArgvUser, bbArgvPass, bbArgvSec,
		bbMCPArgSec, bbShortResp, bbBOMResp, bbGzipResp, bbNewSecret, bbAccessTok, bbHTTPToken}
	for _, c := range basicPairs {
		s = append(s, basicPayload(c))
	}
	return s
}

// The instance's RPC surface (spec.yaml): POSTs to /Services/<Service>.ashx/<Method>
// and one GET for the session groups.
const (
	pathLiveData   = "/Services/PageService.ashx/GetLiveData"
	pathDetail     = "/Services/PageService.ashx/GetSessionDetails"
	pathRunCommand = "/Services/PageService.ashx/AddSessionEvents"
	pathAddEvent   = "/Services/PageService.ashx/AddEventToSessions"
	pathRename     = "/Services/PageService.ashx/UpdateSessionName"
	pathCustomProp = "/Services/PageService.ashx/UpdateSessionCustomPropertyValue"
	pathAccess     = "/Services/PageService.ashx/GetAccessToken"
	pathGroups     = "/Services/SessionGroupService.ashx/session-groups"
	pathSecurity   = "/Services/SecurityService.ashx/GetSecurityConfigurationInfo"
	pathSaveUser   = "/Services/SecurityService.ashx/SaveUser"
	pathDeleteUser = "/Services/SecurityService.ashx/DeleteUser"
	pathAuditInfo  = "/Services/AuditService.ashx/GetAuditInfo"
	pathAuditLog   = "/Services/AuditService.ashx/QueryAuditLog"
)

var bbCLI, bbMCP string

// bbHeaderFlags is set when a generated endpoint command declares the Basic
// credential as a flag of its own (--authorization, --control-username,
// --control-password): a render that turns the spec's security scheme into
// per-operation parameters makes every command refuse to run on the
// configured credentials alone (TestGeneratedCommandsRunOnTheConfiguredCredentials).
// The battery then passes the Basic header on the command line, which is what
// an operator of such a tree has to do, so the argv path is covered too.
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
	dir, err := os.MkdirTemp("", "connectwise-control-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "connectwise-control-cli"+exe)
	bbMCP = filepath.Join(dir, "connectwise-control-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/connectwise-control-cli", bbMCP: "../../cmd/connectwise-control-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	help, _ := exec.Command(bbCLI, "sessions", "get-detail", "--help").CombinedOutput()
	bbHeaderFlags = bytes.Contains(help, []byte("--authorization")) || bytes.Contains(help, []byte("--control-password"))
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
	fmt.Fprintf(os.Stderr, "cwcblackbox: %d CLI runs, %d MCP tools/call requests, credential flags on argv: %v\n",
		cliRuns.n, mcpCallsSent.n, bbHeaderFlags)
	os.Exit(code)
}

// echoForms is every encoding of s the server sends back whole: as written,
// inside other text, as an Authorization header line (raw and as the Basic
// credential), base64 (standard and URL-safe), hex, base32, ASCII85,
// percent-encoded, PEM-armored, and gzip-compressed then base64-encoded (the
// way a JSON string carries compressed bytes).
func echoForms(s string) []string {
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return []string{
		s,
		"x" + s + "x",
		"Authorization: " + s,
		"Authorization: Basic " + s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
		string(pem.EncodeToMemory(&pem.Block{Type: "SCREENCONNECT KEY", Bytes: []byte(s)})),
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
// Basic credential decoded, the URL or the body): a real API can echo only
// what it was sent.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	seen := fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
	if u, p, ok := r.BasicAuth(); ok {
		seen += "\n" + u + "\n" + p
	}
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

// creds is the HTTP Basic pair one request carries.
type creds struct{ user, pass string }

func (c creds) String() string { return c.user + " | " + c.pass }

type server struct {
	url     string
	caFile  string
	mu      sync.Mutex
	offHost int
	methods []string      // API requests, "METHOD /path"
	headers map[creds]int // the Basic pairs the API received ({"", ""}: none)
	// nonBasic counts requests whose Authorization is missing or not one
	// Basic credential; elsewhere counts requests that carried their Basic
	// user name or password anywhere but the Authorization header.
	nonBasic, elsewhere int
	// redirectGroups sends the session-groups read (the one syncable,
	// exportable and tailable resource) to the other host too, so sync meets
	// a redirect; denyGroups answers it 401 (an export that meets an error).
	redirectGroups, denyGroups bool
	// splitGroups answers the session-groups read with two groups whose
	// names are the two halves of the first credential it was sent.
	splitGroups bool
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

// readPaths are the documented reads; every other request changes data.
var readPaths = map[string]bool{
	"POST " + pathLiveData:  true,
	"POST " + pathDetail:    true,
	"POST " + pathSecurity:  true,
	"POST " + pathAuditInfo: true,
	"POST " + pathAuditLog:  true,
	"GET " + pathGroups:     true,
}

// isWrite reports whether "METHOD /path" changes data: anything but a
// documented read (fail closed: an unknown method or path is a write).
func isWrite(m string) bool { return !readPaths[m] }

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{headers: map[creds]int{}}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	now := time.Now().UTC()
	stamp := func(d time.Duration) string { return now.Add(d).Format("2006-01-02T15:04:05.000Z") }
	recent, old := stamp(-time.Hour), stamp(-90*24*time.Hour)
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		// The fake echoes nothing it was sent: the kit withholds such an
		// answer whole, and these tests need the answer.
		var sent []string
		echo := echoObject(sent)
		u, p, basic := r.BasicAuth()
		var outside strings.Builder
		for k, vs := range r.Header {
			if !strings.EqualFold(k, "Authorization") {
				fmt.Fprintf(&outside, "%s: %v\n", k, vs)
			}
		}
		outside.WriteString(r.URL.String() + "\n" + string(body))
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		sv.headers[creds{u, p}]++
		if !basic || len(r.Header.Values("Authorization")) != 1 {
			sv.nonBasic++
		}
		if basic && ((u != "" && strings.Contains(outside.String(), u)) || (p != "" && strings.Contains(outside.String(), p))) {
			sv.elsewhere++
		}
		redirectGroups, denyGroups, splitGroups := sv.redirectGroups, sv.denyGroups, sv.splitGroups
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		str := func(k string) string { s, _ := req[k].(string); return s }
		p2 := r.URL.Path
		switch {
		case (p2 == pathGroups && denyGroups) || strings.Contains(string(body), bbDenied):
			// Rejected (a denied session id or session name, or the session
			// groups while denyGroups is set): the error repeats what it was
			// sent.
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"code":"Unauthorized","message":"invalid credentials %s","detail":"user %s","echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case (p2 == pathDetail && str("sessionID") == bbRedirect) || (p2 == pathGroups && redirectGroups):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case p2 == pathLiveData:
			// GetLiveData: the spec's bare array of camelCase sessions for a
			// Support list, the wire envelope (ResponseInfoMap.HostSessionInfo.Sessions,
			// PascalCase) otherwise. One session is stale, one recent, one
			// never connected; one carries a credential nested in its record
			// and one has an id that repeats the credential it was sent.
			fmt.Fprint(w, liveDataBody(str("sessionType"), sent, echo, old, recent))
		case p2 == pathDetail:
			// sessions get-detail: the echo split across values, lines and spaces.
			fmt.Fprintf(w, `{"sessionID":%q,"name":"HQ-Record kiosk","sessionType":"Access","host":"hq.tech2","guestMachineName":"HQ-KIOSK-01",`+
				`"lastConnectedEventTime":%q,"isPublic":false,"activeConnections":1,"customProperties":["HQ","Kiosk"],`+
				`"connections":[{"connectionID":"0b1c2d3e-0000-4000-8000-000000000001","participantName":"hq.tech2","connectedTime":%q}],`+
				`"events":[{"eventType":"RanCommand","sessionID":%q,"time":%q,"host":"hq.tech2","data":"HQ-Record output"}],"split":%s}`,
				str("sessionID"), recent, recent, str("sessionID"), recent, splitObject(sent))
		case p2 == pathGroups:
			// session-groups (session_groups list, sync, export, tail):
			// gzip-encoded, the echo, and a group carrying a credential
			// nested in its record.
			items := []string{
				fmt.Sprintf(`{"name":"All Machines","sessionType":"Access","sessionCount":3,"echo":%s}`, echoObject(append(sent, bbGzipResp))),
				fmt.Sprintf(`{"name":%q,"sessionType":"Support","sessionCount":1,"options":{"joinMode":"code","joinPassword":%q}}`, bbGroup, bbGzipResp),
			}
			if splitGroups {
				a, b := "grp-a", "grp-b"
				if len(sent) > 0 {
					a, b = halves(sent[0])
				}
				items = []string{fmt.Sprintf(`{"name":%q,"sessionType":"Access","sessionCount":1}`, a), fmt.Sprintf(`{"name":%q,"sessionType":"Access","sessionCount":1}`, b)}
			}
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			_, _ = io.WriteString(zw, "["+strings.Join(items, ",")+"]")
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case p2 == pathSecurity:
			// GetSecurityConfigurationInfo, BOM-prefixed: the API user (its
			// userName is the Basic user the request carried: held by value),
			// a user whose userName is no credential (it prints) with a short
			// password repeated under a field that is not credential-named,
			// and one with a credential block repeated the same way.
			fmt.Fprintf(w, "\xef\xbb\xbf"+`{"users":[`+
				`{"userName":%q,"displayName":"HQ API","email":"api@example.com","securityGroups":["Administrators"],"isPasswordExpired":false,"isDisabled":false,"echo":%s},`+
				`{"userName":%q,"displayName":"HQ-Record Tech","email":"tech2@example.com","securityGroups":[],"isPasswordExpired":true,"isDisabled":false,"password":%q,"notes":%q},`+
				`{"userName":"hq.vendor","displayName":"HQ-Record Vendor","email":"vendor@example.com","securityGroups":["Technicians"],"isPasswordExpired":false,"isDisabled":true,`+
				`"credentials":{"apiSecret":%q},"comment":%q}],"roles":["Administrators","Technicians"],"echo":%s}`,
				"hq.api", echo, bbPlainUser, bbShortResp, bbShortResp, bbBOMResp, bbBOMResp, echo)
		case p2 == pathAuditInfo:
			fmt.Fprintf(w, `{"eventTypes":["Login","RanCommand","CreatedSession"],"retentionDays":90,"echo":%s}`, echo)
		case p2 == pathAuditLog:
			// The API user's own login (its userName held by value) and a
			// plain user's command (it prints).
			fmt.Fprintf(w, `[{"time":%q,"eventType":"Login","userName":%q,"sessionName":"HQ-Record kiosk","networkAddress":"203.0.113.7","description":"HQ-Record login"},`+
				`{"time":%q,"eventType":"RanCommand","userName":%q,"sessionName":"HQ-Record kiosk","networkAddress":"203.0.113.8","description":"HQ-Record ran hostname","echo":%s}]`,
				recent, "hq.api", recent, bbPlainUser, echo)
		case p2 == pathAccess:
			// The access grant: a one-time token and the join url that repeats it.
			fmt.Fprintf(w, `{"accessToken":%q,"url":"https://instance.example/Host#Access/All%%20Machines/%s/Join?Token=%s","echo":%s}`,
				bbAccessTok, str("sessionID"), bbAccessTok, echo)
		case p2 == pathSaveUser:
			// The saved user, repeating the password it was sent and an
			// initial password beyond the documented User shape: a response
			// credential is held whatever the shape. It also carries an id
			// beyond that shape: no documented ConnectWise Control write
			// answer has one (User, Acknowledgement, the access grant), and
			// without an id the CLI stores no write answer, so this one
			// reaches the local store and the store scan covers the
			// mutation write-through.
			fmt.Fprintf(w, `{"id":"user-%s","userName":%q,"displayName":%q,"email":"new@example.com","securityGroups":[],"isPasswordExpired":false,"isDisabled":false,`+
				`"password":%q,"initialPassword":%q,"echo":%s}`, str("userName"), str("userName"), str("displayName"), str("password"), bbNewSecret, echo)
		case r.Method != http.MethodGet:
			// Every other write answers the Acknowledgement and echoes what it was sent.
			fmt.Fprintf(w, `{"success":true,"message":"HQ-Record queued","echo":%s}`, echo)
		default:
			fmt.Fprintf(w, `{"success":true,"echo":%s}`, echo)
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

// liveDataBody is a GetLiveData answer: the spec's bare camelCase array for a
// Support list, the PascalCase wire envelope for any other type.
func liveDataBody(sessionType string, sent []string, echo, old, recent string) string {
	type sess struct{ id, name, machine, last, extra string }
	list := []sess{
		{bbSessionID, "HQ-Record kiosk", "HQ-KIOSK-01", old, `"echo":` + echo},
		{"7a0b3d52-4c8f-4e26-8b1d-3f9082c5d6e4", "HQ-Record front desk", "HQ-DESK-02", recent, `"guestInfo":{"note":"nested","password":"` + bbGzipResp + `"}`},
		{"8b1c4e63-5d90-4f37-9c2e-40a193d6e7f5", "HQ-Record spare", "HQ-SPARE-03", "", ""},
	}
	if len(sent) > 0 {
		list = append(list, sess{sent[0], "Echo-ID", "HQ-ECHO", recent, ""})
	}
	var items []string
	for _, s := range list {
		extra := ""
		if s.extra != "" {
			extra = "," + s.extra
		}
		if sessionType == "Support" {
			items = append(items, fmt.Sprintf(`{"sessionID":%q,"name":%q,"sessionType":"Support","host":"hq.tech2","guestMachineName":%q,"guestLoggedOnUserName":"HQ\\kiosk",`+
				`"guestOperatingSystemName":"Windows 11","guestNetworkAddress":"203.0.113.9","lastConnectedEventTime":%q,"isPublic":false,"activeConnections":0,"customProperties":["HQ"]%s}`,
				s.id, s.name, s.machine, s.last, extra))
			continue
		}
		items = append(items, fmt.Sprintf(`{"SessionID":%q,"Name":%q,"SessionType":2,"Host":"hq.tech2","GuestMachineName":%q,"GuestLoggedOnUserName":"HQ\\kiosk",`+
			`"GuestOperatingSystemName":"Windows 11","GuestNetworkAddress":"203.0.113.9","LastConnectedEventTime":%q,"IsPublic":false,"ActiveConnections":0,"CustomProperties":["HQ"]%s}`,
			s.id, s.name, s.machine, s.last, extra))
	}
	if sessionType == "Support" {
		return "[" + strings.Join(items, ",") + "]"
	}
	return fmt.Sprintf(`{"ResponseInfoMap":{"HostSessionInfo":{"SessionGroupPathParts":["All Machines"],"SessionType":2,"Sessions":[%s]}},"Version":7,"echo":%s}`,
		strings.Join(items, ","), echo)
}

// sawHeader reports how many API requests carried exactly the pair c, and
// the distinct pairs the fake received.
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

// credMode is where the process gets its Basic user name and password.
type credMode int

const (
	credStored            credMode = iota // the pair stored with auth set-credentials; nothing in the environment
	credEnv                               // both from the environment
	credEnvUserOverStored                 // a stored pair and CONNECTWISE_CONTROL_USERNAME: the environment's user name with the stored password; the stored user name held unused
	credEnvOverStored                     // a stored pair and both environment names: the environment wins, the stored pair is held unused
	credConfigFile                        // both in a 0600 config file named by CONNECTWISE_CONTROL_CONFIG
	credEnvOverConfig                     // that config file and both environment names: each environment value wins, the file's are held unused
	credConfigMixed                       // that config file and CONNECTWISE_CONTROL_PASSWORD: the file's user name with the environment's password; the file's password held unused
)

func (m credMode) String() string {
	return [...]string{"stored pair", "env", "env user over stored", "env over stored", "config file", "env over config file",
		"config user with env password"}[m]
}

// sent is the pair the mode sends; held are the values it holds but never sends.
func (m credMode) sent() creds {
	switch m {
	case credStored:
		return creds{bbStoredUser, bbStoredPass}
	case credEnvUserOverStored:
		return creds{bbUser, bbStoredPass}
	case credConfigFile:
		return creds{bbFileUser, bbFilePass}
	case credConfigMixed:
		return creds{bbFileUser, bbPass}
	}
	return creds{bbUser, bbPass}
}

func (m credMode) held() []string {
	switch m {
	case credEnvUserOverStored:
		return []string{bbStoredUser, bbHTTPToken}
	case credEnvOverStored:
		return []string{bbStoredUser, bbStoredPass, bbHTTPToken}
	case credEnvOverConfig:
		return []string{bbFileUser, bbFilePass, bbHTTPToken}
	case credConfigMixed:
		return []string{bbFilePass, bbHTTPToken}
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
		"CONNECTWISE_CONTROL_BASE_URL=" + sv.url, "CONNECTWISE_CONTROL_CA_CERT=" + sv.caFile,
		"CONNECTWISE_CONTROL_CLI_PATH=" + bbCLI, "CONNECTWISE_CONTROL_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
}

// newEnv builds a clean environment against the fake with the credentials
// mode names; the stored modes run auth set-credentials first.
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
		e.env = append(e.env, "CONNECTWISE_CONTROL_CONFIG="+cfg)
	}
	envUser, envPass := "CONNECTWISE_CONTROL_USERNAME="+bbUser, "CONNECTWISE_CONTROL_PASSWORD="+bbPass
	fileBoth := fmt.Sprintf("control_username = %q\ncontrol_password = %q\n", bbFileUser, bbFilePass)
	switch mode {
	case credStored:
		e.storePair(t, bbStoredUser, bbStoredPass)
	case credEnv:
		e.env = append(e.env, envUser, envPass)
	case credEnvUserOverStored:
		e.storePair(t, bbStoredUser, bbStoredPass)
		e.env = append(e.env, envUser)
	case credEnvOverStored:
		e.storePair(t, bbStoredUser, bbStoredPass)
		e.env = append(e.env, envUser, envPass)
	case credConfigFile:
		writeConfig(fileBoth)
	case credEnvOverConfig:
		writeConfig(fileBoth)
		e.env = append(e.env, envUser, envPass)
	case credConfigMixed:
		writeConfig(fileBoth)
		e.env = append(e.env, envPass)
	}
	return e
}

// hdr is the credential flag with the Basic header the mode sends, when the
// generated endpoint commands require it (bbHeaderFlags); none otherwise.
func (e *bbEnv) hdr() []string {
	if !bbHeaderFlags {
		return nil
	}
	return []string{"--authorization", "Basic " + basicPayload(e.mode.sent())}
}

// api is a generated endpoint command line: args, then hdr.
func (e *bbEnv) api(args ...string) []string { return append(args, e.hdr()...) }

// storePair saves user and pass with auth set-credentials (positional
// arguments) and checks both landed in the credential store.
func (e *bbEnv) storePair(t *testing.T, user, pass string) {
	t.Helper()
	out := e.run(t, "auth", "set-credentials", user, pass)
	var stored bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "credentials") {
			b, _ := os.ReadFile(path)
			stored = stored || (bytes.Contains(b, []byte(user)) && bytes.Contains(b, []byte(pass)))
		}
		return nil
	})
	if !stored {
		t.Fatalf("auth set-credentials wrote no credential store:\n%s", out)
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
// import write path (POST AddEventToSessions per record).
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(e.home, "import.jsonl")
	if err := os.WriteFile(p, []byte(`{"group":"All Machines","sessionID":"`+bbSessionID+`","eventType":21,"data":"Imported `+tok+`"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// novels is every novel command and every local read over the synced store.
func novels() [][]string {
	return [][]string{
		{"sessions", "stale", "--json"},
		{"sessions", "stale"},
		{"sessions", "stale", "--session-type", "Support", "--days", "14", "--json"},
		{"sessions", "stale", "--agent"},
		{"security", "access-review", "--json"},
		{"security", "access-review"},
		{"security", "access-review", "--flagged-only", "--agent"},
		{"tail", "session_groups", "--follow=false", "--json"},
		{"workflow", "status", "--json"},
		{"learnings", "list", "--json"},
		{"recall", "find groups", "--json"},
	}
}

// battery runs every output path; tok is a credential the process holds (the
// password it sends: the environment's, the stored one or a config file's).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) [][]string {
	imp := e.importFile(t, tok)
	a := e.api
	b := [][]string{
		// Every output mode.
		a("sessions", "list", "--session-type", "Access", "--json"),
		a("sessions", "list", "--session-type", "Access"),
		a("sessions", "list", "--session-type", "Access", "--csv"),
		a("sessions", "list", "--session-type", "Access", "--plain"),
		a("sessions", "list", "--session-type", "Access", "--agent"),
		a("sessions", "list", "--session-type", "Access", "--json", "--receipt"),
		a("sessions", "list", "--session-type", "Access", "--json", "--audit-dir", filepath.Join(e.home, "audit")),
		a("sessions", "list", "--session-type", "Access", "--json", "--deliver", "file:"+deliverFile),
		// Every response shape: the bare session array, the split echo, a 401
		// echo, a refused redirect, gzip with a nested credential, BOM with a
		// short password and a credential block, the API user's own userName,
		// the audit log.
		a("sessions", "list", "--session-type", "Support", "--json"),
		a("sessions", "list", "--session-type", "Support", "--csv"),
		a("sessions", "get-detail", "--session-id", bbSessionID, "--json"),
		a("sessions", "get-detail", "--session-id", bbDenied, "--json"),
		a("sessions", "get-detail", "--session-id", bbRedirect, "--json"),
		a("session-groups", "--json"),
		a("session-groups"),
		a("security", "get-configuration", "--json"),
		a("security", "get-configuration"),
		a("security", "get-configuration", "--csv"),
		a("audit", "get-info", "--json"),
		a("audit", "query-log", "--json"),
		a("audit", "query-log", "--plain"),
		a("audit", "query-log", "--session-name", bbDenied, "--json"),
		// Writes the fake echoes, with and without --yes (a CLI write without
		// --yes is the work order's P2; it must never leak either way). The
		// access grant is a write too.
		a("sessions", "get-access-token", "--session-id", bbSessionID, "--json", "--yes"),
		a("sessions", "get-access-token", "--session-id", bbSessionID),
		a("sessions", "get-access-token", "--session-id", bbSessionID, "--csv", "--yes"),
		a("sessions", "run-command", "--session-id", bbSessionID, "--command", "echo "+tok, "--json", "--yes"),
		a("sessions", "run-command", "--session-id", bbSessionID, "--command", "hostname", "--json"),
		a("sessions", "add-event-to", "--session-id", bbSessionID, "--event-type", "21", "--data", "HQ "+tok, "--json", "--yes"),
		a("sessions", "update-name", "--session-id", bbSessionID, "--new-name", "HQ "+tok, "--json", "--yes"),
		a("sessions", "update-custom-property", "--session-id", bbSessionID, "--property-index", "0", "--value", "HQ "+tok, "--json"),
		a("security", "save-user", "--user-name", "hq.newuser", "--display-name", "HQ New", "--password", bbArgvSec, "--json", "--yes"),
		a("security", "save-user", "--user-name", "hq.newuser2", "--display-name", "HQ "+tok, "--json"),
		a("security", "delete-user", "--user-name", "hq.olduser", "--yes", "--json"),
		{"import", "sessions", "--input", imp, "--json"},
		// The local store, exports and the local reads.
		{"sync", "--json"},
		{"workflow", "archive", "--json"},
		{"export", "session_groups", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"export", "session_groups", "--format", "json"},
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
		{"analytics", "--type", "session_groups", "--group-by", "name", "--json"},
	}
	b = append(b, novels()...)
	return append(b, [][]string{
		// A credential on the command line: set-credentials' arguments (a
		// password that starts with "-", before and after "--"), a credential
		// flag whose value starts with "-", an unknown credential-named flag,
		// an unknown flag, a held value as an id, feedback text.
		{"auth", "set-credentials", "--dry-run", bbArgvUser, bbArgvPass},
		{"auth", "set-credentials", "--dry-run", "--", bbArgvUser, bbArgvPass},
		{"auth", "set-credentials", bbArgvUser, bbArgvPass, "extra-" + bbArgvPass},
		a("security", "save-user", "--user-name", "argv", "--password", bbArgvSec, "--dry-run"),
		a("sessions", "list", "--session-type", "Access", "--secret="+bbArgvSec, "--json"),
		append(a("sessions", "list", "--session-type", "Access", "--json"), "--"+tok),
		a("sessions", "get-detail", "--session-id", tok, "--json"),
		{"feedback", "noted the password " + tok + " in a session"},
		// Previews: a dry run sends nothing.
		a("sessions", "update-name", "--session-id", bbSessionID, "--new-name", "Echo "+tok, "--dry-run"),
		a("sessions", "run-command", "--session-id", bbSessionID, "--command", "Echo "+tok, "--dry-run"),
		a("security", "save-user", "--user-name", "hq.dry", "--password", tok, "--dry-run"),
		a("sessions", "get-access-token", "--session-id", bbSessionID, "--dry-run"),
		{"import", "sessions", "--input", imp, "--dry-run"},
		// Learnings, playbooks, teach logs and profiles land in local files.
		{"teach", "--query", "find groups " + tok, "--resource", bbGroup, "--resource-type", "session_groups",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		{"teach", "--query", "find " + tok, "--resource", tok, "--resource-type", "session_groups"}, // a held resource id: refused, logged to teach.log
		{"teach-playbook", "--query", "groups with " + tok, "--notes", "use " + tok},
		{"profile", "save", "held", "--description", "uses " + tok, "--select", "name"},
		{"doctor"},
		{"auth", "status"},
		{"auth", "status", "--json"},
		{"agent-context"},
	}...)
}

// mcpReads are read-only tools across every response shape the fake has, and
// every novel and local-read tool.
func mcpReads() []map[string]any {
	none := map[string]any{}
	return []map[string]any{
		{"name": "sessions_list", "arguments": map[string]any{"sessionType": "Access"}},
		{"name": "sessions_list", "arguments": map[string]any{"sessionType": "Support"}},
		{"name": "sessions_get-detail", "arguments": map[string]any{"sessionID": bbSessionID}},
		{"name": "sessions_get-detail", "arguments": map[string]any{"sessionID": bbDenied}},
		{"name": "session_groups_list", "arguments": none},
		{"name": "security_get-configuration", "arguments": none},
		{"name": "audit_get-info", "arguments": none},
		{"name": "audit_query-log", "arguments": map[string]any{"sessionName": "HQ-Record kiosk"}},
		{"name": "sessions_stale", "arguments": map[string]any{"days": 14}},
		{"name": "sessions_stale", "arguments": map[string]any{"session-type": "Support"}},
		{"name": "security_access_review", "arguments": none},
		{"name": "quarterly_user_access_review", "arguments": map[string]any{"flagged_only": true}},
		{"name": "tail", "arguments": map[string]any{"resource": "session_groups", "follow": false}},
		{"name": "analytics", "arguments": map[string]any{"type": "session_groups", "group-by": "name"}},
		{"name": "search", "arguments": map[string]any{"query": "HQ"}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM session_groups"}},
		{"name": "recall", "arguments": map[string]any{"query": "find groups"}},
		{"name": "workflow_status", "arguments": none},
		{"name": "context", "arguments": none},
	}
}

// mcpLocal are the local writes (no confirm gate by design): they read the
// API and write only the local store or the client's result.
func mcpLocal() []map[string]any {
	return []map[string]any{
		{"name": "sync", "arguments": map[string]any{}},
		{"name": "export", "arguments": map[string]any{"resource": "session_groups"}},
	}
}

// mcpWrites is every write tool, called without confirm: each must answer
// with a preview and send nothing. The endpoint writes preview as a
// confirmation_required object; the mirrored CLI write (import) previews as
// the command's --dry-run output followed by the gate's "Preview only:
// nothing was sent" note. The access grant (sessions_get-access-token) is a
// write. Credential-named arguments (a declared password, an undeclared
// secret) are held and withheld in the preview.
func mcpWrites() []map[string]any {
	return []map[string]any{
		{"name": "import", "arguments": map[string]any{"resource": "sessions"}},
		{"name": "sessions_run-command", "arguments": map[string]any{"sessionID": bbSessionID, "command": "hostname", "powershell": false}},
		{"name": "sessions_add-event-to", "arguments": map[string]any{"sessionID": bbSessionID, "eventType": 21, "data": "Preview"}},
		{"name": "sessions_update-name", "arguments": map[string]any{"sessionID": bbSessionID, "newName": "Preview"}},
		{"name": "sessions_update-custom-property", "arguments": map[string]any{"sessionID": bbSessionID, "propertyIndex": 0, "value": "Preview", "secret": bbMCPArgSec}},
		{"name": "sessions_get-access-token", "arguments": map[string]any{"sessionID": bbSessionID}},
		{"name": "security_save-user", "arguments": map[string]any{"userName": "hq.preview", "password": bbMCPArgSec}},
		{"name": "security_delete-user", "arguments": map[string]any{"userName": "hq.preview"}},
	}
}

func mcpBattery() []map[string]any {
	return append(append(mcpReads(), mcpLocal()...), mcpWrites()...)
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

// Class credential-sent-to-another-host: a redirect off the configured
// instance host is refused before it is followed, from the CLI, sync and MCP
// alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	redirect := e.run(t, e.api("sessions", "get-detail", "--session-id", bbRedirect, "--json")...)
	sv.mu.Lock()
	sv.redirectGroups = true
	sv.mu.Unlock()
	syncOut := e.run(t, "sync", "--resources", "session_groups", "--json")
	e.mcpCalls(t, []map[string]any{
		{"name": "sessions_get-detail", "arguments": map[string]any{"sessionID": bbRedirect}},
		{"name": "session_groups_list", "arguments": map[string]any{}},
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		if m == "POST "+pathDetail || m == "GET "+pathGroups {
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
	for where, out := range map[string]string{"sessions get-detail": redirect, "sync": syncOut} {
		if !strings.Contains(out, "leave the configured ConnectWise Control host") {
			t.Fatalf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
	}
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a documented
// read while no call sets confirm, though the battery calls every write tool;
// each write answers with a preview.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	// The store holds the synced groups first, so the local reads have
	// something to read while the writes preview.
	e.run(t, "sync", "--json")
	sv.resetMethods()
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, "HQ-Record") {
		t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
	}
	replies := mcpReplies(out)
	for i, call := range mcpWrites() {
		id := len(mcpReads()) + len(mcpLocal()) + i + 1
		r := replies[id]
		// A call whose arguments carry a credential gets the kit's withheld
		// notice in place of the preview that would repeat it; either way
		// nothing is sent (checked below).
		if !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") && !strings.Contains(r, "[credkit: withheld from mcp") {
			t.Errorf("%s %v without confirm did not return a preview:\n%.1500s", call["name"], call["arguments"], r)
		}
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var reads bool
	for _, m := range sv.methods {
		reads = reads || m == "POST "+pathLiveData
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
// and the number 1 still preview. Both gates: the endpoint tools and the
// mirrored write (import). The mirror blocks import's required --input (a
// file the server would read for the caller), so a mirrored import can never
// send a record: its control is that confirm true runs the child as given
// (its own answer, without --dry-run or the gate's preview note) while every
// other value previews.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	write := func(confirm any) []map[string]any {
		return []map[string]any{
			{"name": "sessions_update-name", "arguments": map[string]any{"sessionID": bbSessionID, "newName": "Renamed", "confirm": confirm}},
			{"name": "security_delete-user", "arguments": map[string]any{"userName": "hq.olduser", "confirm": confirm}},
			{"name": "import", "arguments": map[string]any{"resource": "sessions", "confirm": confirm}},
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
	var rename, del bool
	for _, m := range sv.methods {
		rename = rename || m == "POST "+pathRename
		del = del || m == "POST "+pathDeleteUser
	}
	if !rename || !del {
		t.Errorf("confirm true did not send the rename and the user delete; requests: %v", sv.methods)
	}
	if r := mcpReplies(out)[3]; r == "" || strings.Contains(r, "Preview only") || strings.Contains(r, "dry run") || !strings.Contains(r, "input") {
		t.Errorf("confirm true did not run the mirrored import as given:\n%.1500s", r)
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, from a cold
// start with the credentials in the environment, the credential store or a
// config file.
func TestDryRunSendsNothing(t *testing.T) {
	for _, mode := range []credMode{credEnv, credStored, credConfigFile, credConfigMixed} {
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		imp := e.importFile(t, "dry")
		for _, args := range [][]string{
			e.api("sessions", "run-command", "--session-id", bbSessionID, "--command", "hostname", "--dry-run"),
			e.api("sessions", "add-event-to", "--session-id", bbSessionID, "--event-type", "21", "--dry-run"),
			e.api("sessions", "update-name", "--session-id", bbSessionID, "--new-name", "Dry", "--dry-run"),
			e.api("sessions", "update-custom-property", "--session-id", bbSessionID, "--property-index", "0", "--value", "Dry", "--dry-run"),
			e.api("sessions", "get-access-token", "--session-id", bbSessionID, "--dry-run"),
			e.api("security", "save-user", "--user-name", "hq.dry", "--password", "DryPassw0rd-1x", "--dry-run"),
			e.api("security", "delete-user", "--user-name", "hq.dry", "--dry-run"),
			{"import", "sessions", "--input", imp, "--dry-run"},
			e.api("sessions", "list", "--session-type", "Access", "--dry-run"),
			e.api("sessions", "get-detail", "--session-id", bbSessionID, "--dry-run"),
			e.api("session-groups", "--dry-run"),
			{"sessions", "stale", "--dry-run"},
			{"security", "access-review", "--dry-run"},
			{"auth", "set-credentials", "dry-user", "DryPassw0rd-2x", "--dry-run"},
		} {
			// Each run reaches its preview, so the count below is not vacuous
			// (a flag error would send nothing either).
			if out := e.run(t, args...); !strings.Contains(out, "dry_run") && !strings.Contains(out, "dry run") && !strings.Contains(out, "dry-run") && !strings.Contains(out, "would ") {
				t.Errorf("%v (mode %s) printed no preview:\n%.1500s", args, mode, out)
			}
		}
		sv.mu.Lock()
		if len(sv.methods) != 0 {
			t.Errorf("--dry-run (mode %s) sent %v", mode, sv.methods)
		}
		sv.mu.Unlock()
		// Control: the same write without --dry-run reaches the fake.
		e.run(t, e.api("sessions", "update-name", "--session-id", bbSessionID, "--new-name", "Dry", "--yes")...)
		sv.mu.Lock()
		if len(sv.methods) != 1 || sv.methods[0] != "POST "+pathRename {
			t.Errorf("control (mode %s): want the one rename, got %v", mode, sv.methods)
		}
		sv.mu.Unlock()
	}
}
