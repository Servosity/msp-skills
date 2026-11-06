// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package kbmsblackbox runs the built CLI and MCP binaries against a Kaseya BMS
// fake that echoes every credential it was sent back in every encoding it can,
// whole and split across lines and adjacent values, and checks what a reviewer
// cannot read off the code (kaseya-bms handfixes output-chokepoint,
// store-never-keeps-secrets, no-response-cache, no-credential-in-files,
// response-credentials-held, credential-endpoint-result-held,
// api-origin-pinned, dry-run-sends-nothing, mcp-endpoint-write-confirm,
// token-minted-per-client, minted-token-bound-to-login-and-host):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the KASEYA_BMS_PASSWORD of the login, the access and
//     refresh tokens the fake's login endpoint mints for it (fresh, or cached
//     in the credential store by an earlier run), a pre-minted
//     KASEYA_BMS_TOKEN, the token stored with auth set-token, the MCP HTTP
//     token, a credential-named flag value (admin post-webhook-configuration
//     --secret), and the credentials a response carries (an IT Glue password
//     returned as a bare Result string, the IT Glue and VSA access tokens, the
//     token pair security refresh-token returns);
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     learn journal, feedback, config.toml), and the credential store never
//     holds the password or an environment token;
//   - no credential leaves for a host other than the configured host by
//     redirect, the login included;
//   - the MCP server sends a write only when the call sets confirm to the
//     boolean true, and --dry-run sends nothing at all (no login either).
//
// The fake models Kaseya BMS: the API under <host>/v2, the login at POST
// <host>/v2/security/authenticate on the same origin (multipart/form-data
// UserName, Password, Tenant, GrantType=password, no Authorization header,
// answered {"Success":true,"Result":{"AccessToken","RefreshToken",
// "AccessTokenExpireOn"}}); every answer is the envelope
// {"Success","Error","Result",["TotalRecords"]}.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder. The fake teaches the holder nothing:
// it echoes a credential only under names that do not read as credentials and
// never in a shape the holder learns from (an auth-scheme value such as
// "Bearer <v>" is held under any name), so a value the program failed to hold
// from its real source (the environment, the store, the mint, the argv) is
// printed, and the scan fails. TestEchoTeachesTheHolderNothing proves it. The
// username and tenant are identifiers, not credentials: they print (the
// tenant is on every record as TenantName).
package kbmsblackbox

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
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bbUser        = "bb.api.user@example.com"                // KASEYA_BMS_USERNAME (an identifier, not held)
	bbTenant      = "bbtenant-hq"                            // KASEYA_BMS_TENANT (an identifier, not held)
	bbPassword    = "l0g1nPw" + "-3e9a7c" + "1d5b2f8a46c0de" // KASEYA_BMS_PASSWORD (no 8-character piece is a word a message prints: the kit holds every piece)
	bbMintAccess  = "mintedAccess-5c1e9a3f7b2d4e60c8"        // the AccessToken the fake's login mints for the login
	bbMintRefresh = "mintedRefresh-9d2b6f4a1c8e7305ab"       // the RefreshToken it mints with it
	bbEnvToken    = "envToken-4b8d2f6a0c9e1735bd"            // KASEYA_BMS_TOKEN, a pre-minted bearer
	bbStored      = "storedToken-71c3e9b05d2a4f68e1"         // auth set-token (stdin), the credential store
	bbArgvSec     = "-argvSecret-91c3e7a5"                   // --secret on the command line (starts with "-")
	bbShortResp   = "Sh0rtPw"                                // a 7-character IT Glue password, a bare Result string
	bbBOMResp     = "BomRespSecret-6a1c"                     // the VSA access token of a BOM-prefixed answer
	bbGzipResp    = "GzipRespToken-4f2a9c1e"                 // the IT Glue access token of a gzip-encoded answer
	bbResetKey    = "refreshedAccess-8c2e4a6f1b"             // the AccessToken security refresh-token returns
	bbResetSecret = "refreshedRefresh-3d5f7b9a2c4e"          // the RefreshToken it returns
	bbHTTPToken   = "mcpHttpToken-0123456789abcdefABCDEF"    // KASEYA_BMS_MCP_HTTP_TOKEN
	bbTicket      = "5150"                                   // a ticket id
	bbDenied      = "4040"                                   // a ticket id the fake answers 401
	bbControl     = "ctlEchoValue-7a1b9c2d3e"                // not a credential: the fake echoes it like one (TestEchoTeachesTheHolderNothing)
)

func secrets() []string {
	return []string{bbPassword, bbMintAccess, bbMintRefresh, bbEnvToken, bbStored, bbArgvSec,
		bbShortResp, bbBOMResp, bbGzipResp, bbResetKey, bbResetSecret, bbHTTPToken}
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
	dir, err := os.MkdirTemp("", "kbms-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "kaseya-bms-cli"+exe)
	bbMCP = filepath.Join(dir, "kaseya-bms-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/kaseya-bms-cli", bbMCP: "../../cmd/kaseya-bms-mcp"} {
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

// tlsServer starts an HTTP/2 TLS fake.
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

type server struct {
	url     string // the API base URL: <host>
	caFile  string
	mu      sync.Mutex
	offHost int
	methods []string       // API requests but the login, "METHOD /path"
	headers map[string]int // Authorization header values those requests carried
	mints   int            // login requests (POST <host>/v2/security/authenticate)
	minted  int            // ... that the fake answered with the minted token
	badMint []string       // ... the fake refused, and why
	denied  int            // API requests the fake answered 401
	// redirectTickets sends the tickets list (sync resource servicedesk) to
	// the other host too, so sync meets a redirect.
	redirectTickets bool
	// redirectToken sends the login to the other host (a 307 would resend the
	// request and its form, the password included).
	redirectToken bool
	// echo keeps the sent values the fake echoes (nil: every one). The
	// credential kit withholds a whole unit (a document, a row, a request)
	// that carries a held value, so a data pass sets it to keep none: the
	// records then land whole and the scans are shown not to be vacuous.
	echo func(string) bool
}

func (sv *server) setEcho(keep func(string) bool) {
	sv.mu.Lock()
	sv.echo = keep
	sv.mu.Unlock()
}

func echoNone(string) bool { return false }

// Records shaped the way spec.json documents them (TicketListDto,
// TimeLogOutputDto, OpportunityOutputDto, ClientContractSummaryOutputDto), so
// the store, the novels (queue-health and stale-tickets read tickets, unbilled
// reads time logs, pipeline reads opportunities, contract-burn reads
// contracts) and the exports have something real to read.
var (
	recent       = time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	old          = time.Now().Add(-90 * 24 * time.Hour).UTC().Format(time.RFC3339)
	future       = time.Now().Add(200 * 24 * time.Hour).UTC().Format(time.RFC3339)
	ticketRecord = `"Id":5150,"TicketNumber":"T20265150","Title":"HQ-Ticket","AccountName":"HQ-Record","QueueName":"Service Desk",` +
		`"PriorityName":"High","StatusName":"New","AssigneeName":"HQ-Tech","AssigneeId":7,"OpenDate":"` + old + `",` +
		`"LastActivityUpdate":"` + old + `","CreatedOn":"` + old + `","ContractName":"HQ-Contract","TenantName":"` + bbTenant + `"`
	timelogRecord = `"Id":8001,"TicketId":5150,"AccountName":"HQ-Record","UserId":7,"FirstName":"HQ","LastName":"Tech","Timespent":90,` +
		`"IsBillable":true,"IsApproved":true,"IsBilled":false,"StartDate":"` + recent + `"`
	oppRecord = `"Id":9001,"Subject":"HQ-Deal","AccountName":"HQ-Record","Status":"Open","Amount":5000,"Probability":50,` +
		`"CloseDate":"` + old + `","InPipeline":true`
	contractRecord = `"Id":701,"ContractName":"HQ-Contract","AccountName":"HQ-Record","ContractType":"Block Hours",` +
		`"StartDate":"` + old + `","EndDate":"` + future + `","BillingCycle":"Monthly","Status":"Active"`
)

// kbmsList wraps items the way every BMS list answers: one page.
func kbmsList(items ...string) string {
	return fmt.Sprintf(`{"Success":true,"Error":null,"Result":[%s],"TotalRecords":%d}`, strings.Join(items, ","), len(items))
}

func pageNumber(r *http.Request) string {
	if v := r.URL.Query().Get("PageNumber"); v != "" {
		return v
	}
	return "1"
}

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{headers: map[string]int{}}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(kbmsList()))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/v2/security/authenticate" {
			sv.token(w, r, body, other.URL)
			return
		}
		sent := sentValues(r, body)
		sv.mu.Lock()
		if keep := sv.echo; keep != nil {
			sent = slices.DeleteFunc(sent, func(s string) bool { return !keep(s) })
		}
		sv.mu.Unlock()
		echo := echoObject(sent)
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		get := r.Method == http.MethodGet
		seg := strings.Split(strings.Trim(p, "/"), "/")
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		if h := r.Header.Get("Authorization"); h != "" {
			sv.headers[h]++
		}
		redirectTickets := sv.redirectTickets
		sv.mu.Unlock()
		echoID := ""
		if len(sent) > 0 {
			// A record whose keys (Id, TicketNumber) repeat the first value it
			// was sent: the store keys rows by them.
			echoID = fmt.Sprintf(`{"Id":%q,"TicketNumber":%q,"Title":"Echo-ID","AccountName":"Echo-ID","StatusName":"New","TenantName":%q}`, sent[0], sent[0], bbTenant)
		}
		list := func(rec string) string {
			if pageNumber(r) != "1" {
				return kbmsList()
			}
			items := []string{fmt.Sprintf(`{%s,"echo":%s}`, rec, echo)}
			if echoID != "" {
				items = append(items, echoID)
			}
			return kbmsList(items...)
		}
		switch {
		case strings.Contains(p, "/"+bbDenied) || p == "/v2/servicedesk/templates/notes/lookup":
			// Rejected: the error repeats what it was sent.
			sv.mu.Lock()
			sv.denied++
			sv.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"Success":false,"Error":{"Code":978001,"Message":"bad request %s","Details":"invalid request %s"},"Result":null,"echo":%s}`, strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/v2/system/views" || (redirectTickets && p == "/v2/servicedesk/tickets"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case get && p == "/v2/servicedesk/tickets":
			fmt.Fprint(w, list(ticketRecord))
		case get && p == "/v2/timelogs":
			fmt.Fprint(w, list(timelogRecord))
		case get && p == "/v2/crm/opportunities":
			fmt.Fprint(w, list(oppRecord))
		case get && p == "/v2/finance/contracts/summary":
			fmt.Fprint(w, list(contractRecord))
		case get && len(seg) == 5 && seg[1] == "servicedesk" && seg[2] == "tickets" && seg[4] == "activities":
			// ticket activities: only the control value split across values,
			// lines and spaces (no credential beside it), so a split by itself
			// is shown not to be what the holder withholds.
			var ctl []string
			for _, v := range sent {
				if v == bbControl {
					ctl = append(ctl, v)
				}
			}
			fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":[{"Id":1,"ActivityType":"note","split":%s}],"TotalRecords":1}`, splitObject(ctl))
		case get && len(seg) == 4 && seg[1] == "servicedesk" && seg[2] == "tickets":
			// ticket get: the echo whole and split across values, lines and spaces.
			fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":{%s,"echo":%s,"split":%s}}`, ticketRecord, echo, splitObject(sent))
		case get && strings.HasPrefix(p, "/v2/integrations/itglue/passwords/"):
			// The IT Glue password value: a short credential as a bare Result
			// string (StringResponse), named nothing.
			fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":%q,"echo":%s}`, bbShortResp, echo)
		case get && p == "/v2/integrations/itglue/accessinfo":
			// IT Glue access info: gzip-encoded, an AccessToken, the echo.
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprintf(zw, `{"Success":true,"Error":null,"Result":{"IsConnected":true,"IsEnabled":true,"AccessToken":%q,"Subdomain":"hq","UrlLocation":"us"},"echo":%s}`,
				bbGzipResp, echoObject(append(sent, bbGzipResp)))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case get && strings.HasPrefix(p, "/v2/integrations/vsa/accessinfo/"):
			// VSA access info: BOM-prefixed, with its AccessToken.
			fmt.Fprint(w, "\xef\xbb\xbf"+fmt.Sprintf(`{"Success":true,"Error":null,"Result":{"AccessToken":%q,"Url":"https://vsa.example"},"note":%q}`, bbBOMResp, bbBOMResp))
		case get && len(seg) == 4 && seg[1] == "admin" && seg[2] == "webhooks":
			// A webhook configuration: HasSecret is a boolean, not a credential.
			fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":{"Id":3,"Name":"HQ-Hook","PayloadUrl":"https://hooks.example/x","HasSecret":true,"IsEnabled":true},"echo":%s}`, echo)
		case r.Method == http.MethodPost && p == "/v2/security/refreshtoken":
			// The token pair a refresh issues: a credential response to a write.
			fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":{"AccessToken":%q,"RefreshToken":%q,"AccessTokenExpireOn":"2099-01-01T00:00:00Z"},"echo":%s}`, bbResetKey, bbResetSecret, echo)
		case !get:
			// Every other write echoes what it was sent.
			fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":{%s,"echo":%s}}`, ticketRecord, echo)
		default:
			fmt.Fprint(w, list(`"Id":1,"Name":"HQ-Other"`))
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

// loginForm reads the multipart login form ("" fields when it is not one).
func loginForm(r *http.Request, body []byte) (map[string]string, error) {
	mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" {
		return nil, fmt.Errorf("content type %s", r.Header.Get("Content-Type"))
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	out := map[string]string{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(part)
		out[part.FormName()] = string(b)
	}
}

// token emulates the BMS login: POST <host>/v2/security/authenticate on the
// API origin, multipart/form-data UserName, Password, Tenant and
// GrantType=password, no Authorization header (never a bearer token), answered
// {"Success":true,"Result":{"AccessToken","RefreshToken","AccessTokenExpireOn"}}.
// It answers only the configured login, and never echoes a request on success.
// A refusal repeats what it was sent, the password it read from the form
// included, under names that are not credentials (a real login may name the
// user it refused).
func (sv *server) token(w http.ResponseWriter, r *http.Request, body []byte, otherURL string) {
	sv.mu.Lock()
	sv.mints++
	redirect := sv.redirectToken
	sv.mu.Unlock()
	if redirect {
		http.Redirect(w, r, otherURL+r.URL.Path, http.StatusTemporaryRedirect)
		return
	}
	form, ferr := loginForm(r, body)
	why := ""
	switch {
	case r.Method != http.MethodPost:
		why = "method " + r.Method
	case ferr != nil:
		why = "body is not a multipart form: " + ferr.Error()
	case r.Header.Get("Authorization") != "":
		why = "an Authorization header on the login"
	case form["GrantType"] != "password":
		why = "GrantType " + form["GrantType"]
	case form["UserName"] != bbUser:
		why = "unknown user"
	case form["Tenant"] != bbTenant:
		why = "unknown tenant"
	case form["Password"] != bbPassword:
		why = "wrong password"
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
		for _, k := range []string{"UserName", "Password", "Tenant"} {
			if v := form[k]; v != "" {
				posted = append(posted, v)
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"Success":false,"Error":{"Code":978001,"Message":"Security Error","Details":"rejected %s"},"Result":null,"echo":%s}`, strings.Join(posted, " "), echoObject(posted))
		return
	}
	fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":{"AccessToken":%q,"RefreshToken":%q,"AccessTokenExpireOn":"2099-01-01T00:00:00Z","RefreshTokenExpireOn":"2099-01-02T00:00:00Z"}}`, bbMintAccess, bbMintRefresh)
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

// counts reports the API requests and the login requests so far.
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

// credMode is where the process gets its Authorization.
type credMode int

const (
	credStored      credMode = iota // nothing in the environment: a token stored with auth set-token (no binding)
	credToken                       // a pre-minted KASEYA_BMS_TOKEN, with the login in the environment too: the token wins, nothing mints, the password is held and never sent
	credLogin                       // the login, nothing cached: the first process mints and caches the token (bound to the login and host), later ones reuse it
	credLoginCached                 // the login, and the token an earlier run minted for it cached in the credential store
)

func (m credMode) String() string {
	return [...]string{"stored token", "env token over login", "login minted", "login cached token"}[m]
}

// header is the Authorization value every API request must carry in mode m.
func (m credMode) header() string {
	switch m {
	case credStored:
		return "Bearer " + bbStored
	case credToken:
		return "Bearer " + bbEnvToken
	default:
		return "Bearer " + bbMintAccess
	}
}

// held is a credential the process holds from its start in mode m and never
// sends on its own (the one a test types on the command line or into a URL).
func (m credMode) held() string {
	switch m {
	case credStored:
		return bbStored
	case credLoginCached:
		return bbMintAccess // loaded from the credential store
	default:
		return bbPassword
	}
}

// mints reports whether mode m starts with no token, so its first process mints.
func (m credMode) mints() bool { return m == credLogin }

// keeps is what the credential store may hold in mode m.
func (m credMode) keeps() []string {
	switch m {
	case credStored:
		return []string{bbStored}
	case credLogin, credLoginCached:
		return []string{bbMintAccess, bbMintRefresh}
	}
	return nil
}

func loginEnv() []string {
	return []string{"KASEYA_BMS_USERNAME=" + bbUser, "KASEYA_BMS_PASSWORD=" + bbPassword, "KASEYA_BMS_TENANT=" + bbTenant}
}

// newEnv builds a clean environment against the fake with the credentials
// mode names. Every user directory (KASEYA_BMS_HOME, HOME, the XDG and Windows
// dirs, TMPDIR) is inside a fresh t.TempDir. credStored first stores bbStored
// with auth set-token; credLoginCached first runs one read, so the login mints
// a token and caches it in the credential store. The fake's counters are
// cleared after either.
func newEnv(t *testing.T, sv *server, mode credMode) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "KASEYA_BMS_HOME=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"KASEYA_BMS_BASE_URL=" + sv.url, "KASEYA_BMS_CA_CERT=" + sv.caFile,
		"KASEYA_BMS_CLI_PATH=" + bbCLI, "KASEYA_BMS_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	e := &bbEnv{home: home, env: env}
	switch mode {
	case credStored:
		if out := e.runIn(t, bbStored+"\n", "auth", "set-token"); !e.credentialStoreHolds(bbStored) {
			t.Fatalf("auth set-token stored no token:\n%.1500s", out)
		}
	case credToken:
		e.env = append(e.env, append(loginEnv(), "KASEYA_BMS_TOKEN="+bbEnvToken)...)
	case credLogin:
		e.env = append(e.env, loginEnv()...)
	case credLoginCached:
		e.env = append(e.env, loginEnv()...)
		out := e.run(t, "servicedesk", "get-tickets", "--json")
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

// importFile writes an import input: one account whose name repeats tok, so
// the request (import crm: POST /v2/crm/accounts per record) carries it and
// the fake echoes it. It lives outside the home: it is the test's input, not a
// file the program wrote.
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "accounts.jsonl")
	if err := os.WriteFile(p, []byte(`{"Name":"Imported `+tok+`","AccountType":1}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// ticketWrite is a ticket create or update with every body field BMS requires
// (TicketInputDto) besides the ones args set.
func ticketWrite(args ...string) []string {
	return append(args, "--account-id", "42", "--details", "HQ details", "--location-id", "1", "--open-date", "2026-10-01T00:00:00Z",
		"--priority-id", "1", "--source-id", "1", "--status-id", "1", "--type-id", "1")
}

// syncResources are the sync resources the novels and the exports read:
// tickets (servicedesk), time logs, opportunities (crm) and contracts.
const syncResources = "servicedesk,timelogs,crm,finance-contracts-summary"

// dataPass writes the records the file scan proves it covers, with the fake
// echoing nothing: a delivery, the local store, an export, a learning and a
// feedback entry.
func dataPass(deliverFile, exportFile string) [][]string {
	return [][]string{
		{"servicedesk", "get-tickets", "--json", "--deliver", "file:" + deliverFile},
		{"sync", "--resources", syncResources, "--json"},
		{"export", "timelogs", "--format", "jsonl", "--output", exportFile},
		{"teach", "--query", "find tickets", "--resource", bbTicket, "--resource-type", "servicedesk", "--notes", "rotated the queue", "--no-validate"},
		{"feedback", "data pass note"},
	}
}

// battery runs every output path; tok is a credential the process holds
// (credMode.held).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) [][]string {
	accounts := e.importFile(t, tok)
	list := []string{"servicedesk", "get-tickets"}
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
		// Every response shape: a ticket with the split echo, a bare-string
		// IT Glue password, a gzip IT Glue access token, a BOM-prefixed VSA
		// access token, a 401 echo, a refused redirect, a boolean named like a
		// credential, records whose keys repeat a held value.
		{"servicedesk", "get-ticket", bbTicket, "--json"},
		{"integrations", "get-itgpassword-value", "1001", "--json"},
		{"integrations", "get-itgaccess-info", "--json"},
		{"integrations", "get-vsa-access-info", "--integration-type-id", "2", "--json"},
		{"servicedesk", "get-ticket", bbDenied, "--json"},
		{"system", "get-views", "--json"},
		{"admin", "get-webhook-configuration", "3", "--json"},
		{"crm", "get-all-opportunities", "--json"},
		// Writes the fake echoes, a credential-named flag sent in a body, a
		// rejected write, the token refresh (a credential response), and import.
		ticketWrite("servicedesk", "put-ticket", bbTicket, "--title", "HQ", "--json"),
		ticketWrite("servicedesk", "post-ticket", "--title", "HQ "+tok, "--json"),
		{"admin", "post-webhook-configuration", "--name", "HQ", "--payload-url", "https://hooks.example/x", "--secret", bbArgvSec, "--json"},
		ticketWrite("servicedesk", "put-ticket", bbDenied, "--title", "HQ", "--json"),
		{"security", "refresh-token", "--access-token", bbArgvSec, "--refresh-token", bbArgvSec, "--json"},
		{"import", "crm", "--input", accounts, "--json"},
		// The local store, exports and the novels.
		{"sync", "--resources", syncResources, "--json"},
		{"export", "timelogs", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		{"export", "timelogs", "--format", "json"},
		{"queue-health", "--json"},
		{"stale-tickets", "--days", "1", "--json"},
		{"workload", "--json"},
		{"contract-burn", "--json"},
		{"unbilled", "--json"},
		{"pipeline", "--json"},
		{"search", "HQ", "--json"},
		{"search", "Echo", "--json", "--data-source", "local"},
		// A credential on the command line: a credential-named flag the
		// command lacks, an unknown flag, a positional, feedback text.
		with(list, "--password", bbArgvSec),
		with(list, "--json", "--"+tok),
		{"servicedesk", "get-ticket", tok, "--json"},
		{"feedback", "noted the token " + tok + " in a ticket"},
		// Previews: a dry run sends nothing (auth login neither mints nor saves).
		ticketWrite("servicedesk", "put-ticket", bbTicket, "--title", "Echo "+tok, "--dry-run"),
		{"admin", "post-webhook-configuration", "--name", "Echo " + tok, "--secret", bbArgvSec, "--dry-run"},
		{"auth", "login", "--dry-run"},
		{"import", "crm", "--input", accounts, "--dry-run"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find tickets " + tok, "--resource", bbTicket, "--resource-type", "servicedesk",
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
		return map[string]any{"name": "kaseya-bms_read", "arguments": args}
	}
	none := map[string]any{}
	return []map[string]any{
		read("servicedesk.get-tickets", nil),
		read("servicedesk.get-ticket", map[string]any{"ticketId": bbTicket}),
		read("integrations.get-itgpassword-value", map[string]any{"passwordId": "1001"}),
		read("integrations.get-itgaccess-info", nil),
		// (integrations.get-vsa-access-info is not callable over MCP: the 4.34.2
		// registry binds none of its path parameters; the CLI battery covers its
		// BOM-prefixed credential.)
		read("servicedesk.get-ticket", map[string]any{"ticketId": bbDenied}),
		{"name": "kaseya-bms_search", "arguments": map[string]any{"query": "tickets"}},
		{"name": "kaseya-bms_get", "arguments": map[string]any{"endpoint_id": "servicedesk.get-tickets"}},
		{"name": "queue_health", "arguments": none},
		{"name": "stale_tickets", "arguments": none},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// mcpWrites is every kind of write tool, called without confirm: each must
// answer with a preview and send nothing. kaseya-bms_execute previews as a
// confirmation_required object; the mirrored import previews as the command's
// --dry-run plan followed by the gate's "Preview only: nothing was sent" note.
func mcpWrites() []map[string]any {
	exec := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "kaseya-bms_execute", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	return []map[string]any{
		exec("servicedesk.put-ticket", map[string]any{"ticketId": bbTicket, "Title": "Preview Ticket"}),
		exec("servicedesk.post-ticket", map[string]any{"Title": "Preview Ticket"}),
		exec("servicedesk.delete-ticket", map[string]any{"ticketId": bbTicket}),
		exec("crm.post-opportunity", map[string]any{"Subject": "Preview Deal"}),
		exec("admin.post-webhook-configuration", map[string]any{"Name": "Preview Hook", "Secret": "previewSecretValue"}),
		exec("security.refresh-token", map[string]any{}),
		{"name": "import", "arguments": map[string]any{"resource": "crm"}},
	}
}

func mcpBattery() []map[string]any {
	return append(mcpReads(), mcpWrites()...)
}

// checkMode asserts the fake saw the credential mode the test set up: every
// API request carried the mode's header and no other, and the login was used
// exactly as the mode says (at least once and never refused for the login
// with nothing cached; never for a stored, cached or environment token).
func checkMode(t *testing.T, sv *server, mode credMode) {
	t.Helper()
	n, all := sv.sawHeader(mode.header())
	if n == 0 || len(all) != 1 {
		t.Errorf("%s: want every API request to carry %q, got %d of it and %d distinct values %v", mode, mode.header(), n, len(all), all)
	}
	_, mints, minted, bad := sv.counts()
	switch {
	case mode.mints():
		if minted == 0 || len(bad) != 0 {
			t.Errorf("%s: want the login to mint and no refused mint, got %d accepted, refused %v", mode, minted, bad)
		}
	case mints != 0:
		t.Errorf("%s: want no mint (the token is configured or cached), got %d login requests (refused %v)", mode, mints, bad)
	}
}

// Class credential-persisted-in-files, in every credential mode.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel() // each mode has its own server, home and env
	for _, mode := range []credMode{credStored, credToken, credLogin, credLoginCached} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			// The data pass: the fake echoes nothing, so each record lands
			// whole and the scan below is not vacuous. The echo pass after it
			// writes no record that carries a credential (the kit withholds the
			// unit whole), which its export shows.
			cleanDeliver, cleanExport := filepath.Join(e.home, "deliver-clean.json"), filepath.Join(e.home, "export-clean.jsonl")
			sv.setEcho(echoNone)
			for _, args := range dataPass(cleanDeliver, cleanExport) {
				e.run(t, args...)
			}
			sv.setEcho(nil)
			for _, args := range battery(t, e, deliverFile, mode.held()) {
				e.run(t, args...)
			}
			e.mcpCalls(t, mcpBattery())
			checkMode(t, sv, mode)
			if _, _, minted, _ := sv.counts(); mode.mints() && minted != 1 {
				// The first process mints and caches the token bound to the login;
				// every later process and the MCP server reuse it.
				t.Errorf("%s: %d mints across the battery; want exactly one (the cache bound to the login)", mode, minted)
			}
			keeps := mode.keeps()
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
					// credential, never the password, an environment token, or one
					// a response or the command line carried.
					for _, k := range keeps {
						sawCreds = sawCreds || bytes.Contains(data, []byte(k))
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
			// The data pass really wrote data, so the scan above is not vacuous,
			// and the echo pass reached the file sink (the kit's notice).
			if b, err := os.ReadFile(cleanDeliver); err != nil || !bytes.Contains(b, []byte("HQ-Ticket")) {
				t.Errorf("%s: want the delivered record (err %v)", filepath.Base(cleanDeliver), err)
			}
			if b, err := os.ReadFile(cleanExport); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
				t.Errorf("%s: want the exported record (err %v):\n%.1500s", filepath.Base(cleanExport), err, b)
			}
			if b, err := os.ReadFile(filepath.Join(e.home, "export.jsonl")); err != nil || !bytes.Contains(b, []byte("credkit: withheld")) {
				t.Errorf("export.jsonl: want the kit's notice for the echoed records (err %v):\n%.1500s", err, b)
			}
			var sawTicket, sawTimelog, sawDeal, sawLearning, sawFeedback bool
			_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					return nil
				}
				// The data pass's entry is written whole (the echo pass's, whose
				// text repeats a held value, is withheld).
				sawFeedback = sawFeedback || (d.Name() == "feedback.jsonl" && bytes.Contains(b, []byte(`"cli":"kaseya-bms-cli"`)))
				if strings.HasSuffix(d.Name(), ".db") {
					sawTicket = sawTicket || bytes.Contains(b, []byte("HQ-Ticket"))
					sawTimelog = sawTimelog || bytes.Contains(b, []byte("HQ-Tech"))
					sawDeal = sawDeal || bytes.Contains(b, []byte("HQ-Deal"))
					// The data pass's learning row was written.
					sawLearning = sawLearning || bytes.Contains(b, []byte("rotated"))
				}
				return nil
			})
			if !sawTicket || !sawTimelog || !sawDeal {
				t.Errorf("no local store holds the fetched records (ticket %v, time log %v, opportunity %v)", sawTicket, sawTimelog, sawDeal)
			}
			if !sawLearning {
				t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
			}
			if !sawFeedback {
				t.Errorf("feedback.jsonl holds no entry; the feedback scan did not run")
			}
		})
	}
}

// Class secret-in-output, with an environment token over the login (the
// password held and unused), with the login minting, and with the login on a
// cached minted token.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel() // each mode has its own server, home and env
	for _, mode := range []credMode{credToken, credLogin, credLoginCached} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			deliverFile := filepath.Join(e.home, "deliver.json")
			// The data pass: the fake echoes nothing, so the records print
			// whole (the kit withholds a whole document that carries a
			// credential) and the store the novels read is filled.
			sv.setEcho(echoNone)
			clean := map[string]string{}
			for _, args := range [][]string{
				{"sync", "--resources", syncResources, "--json"},
				{"admin", "get-webhook-configuration", "3", "--json"},
				{"servicedesk", "get-tickets", "--json"},
			} {
				out := e.run(t, args...)
				clean[strings.Join(args, " ")] = out
				assertNoEcho(t, fmt.Sprintf("output of %v", args), out)
			}
			cmd := exec.Command(bbCLI, "servicedesk", "get-tickets", "--json")
			cmd.Env, cmd.Dir = e.env, e.home
			stdout, err := cmd.Output()
			if err != nil {
				t.Fatalf("servicedesk get-tickets --json: %v", err)
			}
			var doc any
			if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Ticket")) {
				t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
			}
			cleanMCP := e.mcpCalls(t, mcpReads())
			if !strings.Contains(cleanMCP, "HQ-Ticket") {
				t.Fatalf("MCP results lost the record itself:\n%.2000s", cleanMCP)
			}
			assertNoEcho(t, "MCP session (data pass)", cleanMCP)
			sv.setEcho(nil)
			outputs := map[string]string{}
			for _, args := range battery(t, e, deliverFile, mode.held()) {
				out := e.run(t, args...)
				outputs[strings.Join(args, " ")] = out
				assertNoEcho(t, fmt.Sprintf("output of %v", args), out)
			}
			// The novels read what the data pass synced, so their scan is not vacuous.
			if out := outputs["queue-health --json"]; !strings.Contains(out, "Service Desk") {
				t.Errorf("queue-health printed no synced ticket:\n%.1500s", out)
			}
			if out := outputs["stale-tickets --days 1 --json"]; !strings.Contains(out, "HQ-Ticket") {
				t.Errorf("stale-tickets printed no synced ticket:\n%.1500s", out)
			}
			if out := outputs["pipeline --json"]; !strings.Contains(out, "HQ-Deal") {
				t.Errorf("pipeline printed no synced opportunity:\n%.1500s", out)
			}
			// Identifiers and booleans named like a credential are data: they print.
			if out := clean["admin get-webhook-configuration 3 --json"]; !strings.Contains(out, `"HasSecret": true`) && !strings.Contains(out, `"HasSecret":true`) {
				t.Errorf("HasSecret (a boolean, not a credential) was withheld:\n%.1500s", out)
			}
			if out := clean["servicedesk get-tickets --json"]; !strings.Contains(out, bbTenant) {
				t.Errorf("the tenant name (an identifier, not a credential) was withheld:\n%.1500s", out)
			}
			// The echo pass reached stdout: the document that carries the echo
			// is withheld whole.
			if out := outputs["servicedesk get-tickets --json"]; !strings.Contains(out, "credkit: withheld from stdout") {
				t.Errorf("the echoed records printed without the kit's notice:\n%.1500s", out)
			}
			out := e.mcpCalls(t, mcpBattery())
			if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
				t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
			}
			assertNoEcho(t, "MCP session", out)
			if !strings.Contains(out, "credkit: withheld from mcp") {
				t.Fatalf("the echoed MCP results carry no kit notice; the echo pass did not reach the MCP sink:\n%.2000s", out)
			}
			// The fake echoed the header it really received, so the scans above
			// covered it; no other credential source was sent.
			checkMode(t, sv, mode)
		})
	}
}

// The oracle is not vacuous: the fake echoes a value that is not a credential
// (bbControl, sent as a ticket filter and a ticket id) exactly as it echoes the
// credentials it is sent, and the output prints it, whole and split, so the echo shapes teach the holder nothing.
// The kit withholds a whole response that carries a credential, so the control
// runs echo the control alone; the same request with the credential echoed
// beside it is withheld.
func TestEchoTeachesTheHolderNothing(t *testing.T) {
	t.Parallel() // its own server, home and env: independent of every other test
	for _, mode := range []credMode{credStored, credLogin} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			sv.setEcho(func(s string) bool { return s == bbControl })
			out := e.run(t, "servicedesk", "get-tickets", "--filter-title", bbControl, "--json")
			out += e.run(t, "servicedesk", "get-ticket-activities", bbControl, "--json")
			out += e.mcpCalls(t, []map[string]any{{"name": "kaseya-bms_read", "arguments": map[string]any{
				"endpoint_id": "servicedesk.get-tickets", "params": map[string]any{"Filter.Title": bbControl}}}})
			for _, f := range echoForms(bbControl) {
				if !holdsForm(out, f) {
					t.Errorf("the echo of the control value as %q was withheld or never sent: the fake teaches the holder, so the credential scans are vacuous:\n%.2000s", f, out)
				}
			}
			if a, b := halves(bbControl); !strings.Contains(out, a+" "+b) {
				t.Errorf("the split echo of the control value was withheld:\n%.2000s", out)
			}
			assertNoEcho(t, "output", out)
			sv.setEcho(nil)
			withCred := e.run(t, "servicedesk", "get-tickets", "--filter-title", bbControl, "--json")
			if !strings.Contains(withCred, "credkit: withheld") {
				t.Errorf("the response that echoes the credential beside the control printed:\n%.2000s", withCred)
			}
			assertNoEcho(t, "output with the credential echoed", withCred)
			checkMode(t, sv, mode)
		})
	}
}

// Class credential-sent-to-another-host: a redirect off the configured host is
// refused before it is followed, from the CLI, sync and MCP alike, and a
// redirected login never receives the password either.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel() // its own server, home and env: independent of every other test
	t.Run("API requests", func(t *testing.T) {
		for _, mode := range []credMode{credStored, credLogin} {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			redirect := e.run(t, "system", "get-views", "--json")
			sv.mu.Lock()
			sv.redirectTickets = true
			sv.mu.Unlock()
			syncOut := e.run(t, "sync", "--resources", "servicedesk", "--json")
			e.mcpCalls(t, []map[string]any{
				{"name": "kaseya-bms_read", "arguments": map[string]any{"endpoint_id": "system.get-views"}},
				{"name": "kaseya-bms_read", "arguments": map[string]any{"endpoint_id": "servicedesk.get-tickets"}},
			})
			sv.mu.Lock()
			n := sv.offHost
			var redirected int
			for _, m := range sv.methods {
				if strings.HasSuffix(m, "/v2/system/views") || strings.HasSuffix(m, "/v2/servicedesk/tickets") {
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
			for where, out := range map[string]string{"system views": redirect, "sync": syncOut} {
				if !strings.Contains(out, "leave the configured Kaseya BMS host") {
					t.Fatalf("%s: %s: want the pin refusal, got:\n%.1500s", mode, where, out)
				}
				assertNoEcho(t, where, out)
			}
			checkMode(t, sv, mode)
		}
	})
	t.Run("login", func(t *testing.T) {
		sv := echoServer(t)
		sv.redirectToken = true
		e := newEnv(t, sv, credLogin)
		out := e.run(t, "servicedesk", "get-tickets", "--json")
		out += e.mcpCalls(t, []map[string]any{{"name": "kaseya-bms_read", "arguments": map[string]any{"endpoint_id": "servicedesk.get-tickets"}}})
		methods, mints, _, _ := sv.counts()
		if mints < 2 {
			t.Fatalf("the login saw %d request(s); the CLI and MCP mints did not both run:\n%.1500s", mints, out)
		}
		sv.mu.Lock()
		n := sv.offHost
		sv.mu.Unlock()
		if n != 0 {
			t.Fatalf("the redirected login reached the other host %d time(s)", n)
		}
		if len(methods) != 0 {
			t.Fatalf("API requests went out without a minted token: %v", methods)
		}
		// The login follows no redirect: the redirect is refused as the answer.
		if !strings.Contains(out, "leave the configured Kaseya BMS host") && !strings.Contains(out, "redirect") {
			t.Fatalf("want the redirect refused, got:\n%.1500s", out)
		}
		assertNoEcho(t, "output", out)
	})
}

// A rejected login (a wrong password, the refusal echoing what it was sent and
// the password it read) returns promptly and prints nothing it was sent, from
// the CLI, from sync and from MCP.
func TestRejectedMintPrintsNoSecret(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credLogin)
	const wrong = "wr0ngL0g1nPw-6d2a9f1c4e" // no 8-character piece is a word the refusal prints
	e.env = append(withoutEnv(e.env, "KASEYA_BMS_PASSWORD"), "KASEYA_BMS_PASSWORD="+wrong)
	start := time.Now()
	out := e.run(t, "servicedesk", "get-tickets", "--json")
	out += e.run(t, "sync", "--resources", "timelogs", "--json")
	out += e.mcpCalls(t, []map[string]any{{"name": "kaseya-bms_read", "arguments": map[string]any{"endpoint_id": "servicedesk.get-tickets"}}})
	if d := time.Since(start); d > 90*time.Second {
		t.Errorf("the rejected logins took %v to return", d)
	}
	methods, mints, minted, bad := sv.counts()
	if mints == 0 || minted != 0 || len(bad) == 0 {
		t.Fatalf("want only refused logins, got %d requests, %d accepted (refused %v)", mints, minted, bad)
	}
	for _, why := range bad {
		if why != "wrong password" {
			t.Fatalf("a login was refused for %q, not the wrong password: the request was malformed", why)
		}
	}
	if len(methods) != 0 {
		t.Errorf("API requests went out after a refused login: %v", methods)
	}
	if !strings.Contains(out, "401") && !strings.Contains(strings.ToLower(out), "login failed") {
		t.Errorf("the refusal was not reported:\n%.2000s", out)
	}
	for _, f := range echoForms(wrong) {
		if strings.Contains(out, f) {
			t.Errorf("output repeats the posted password as %q:\n%.2000s", f, out)
		}
	}
	assertNoEcho(t, "output", out)
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a GET while no
// call sets confirm, though the battery calls every kind of write tool; each
// write answers with a preview. In the minted mode the login is the only POST,
// and it is not a write.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	t.Parallel() // its own server, home and env: independent of every other test
	for _, mode := range []credMode{credStored, credLogin} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			sv.setEcho(echoNone) // the local tools read whole records
			e.run(t, "sync", "--resources", syncResources, "--json")
			sv.setEcho(nil)
			sv.reset()
			out := e.mcpCalls(t, mcpBattery())
			// A read whose answer echoes the credential is withheld whole (the
			// kit's notice); the local tools read the synced records.
			if !strings.Contains(out, "HQ-Ticket") || !strings.Contains(out, "credkit: withheld from mcp") {
				t.Fatalf("the MCP battery fetched nothing:\n%.2000s", out)
			}
			replies := mcpReplies(out)
			for i, call := range mcpWrites() {
				id := len(mcpReads()) + i + 1
				r := replies[id]
				// A preview that would repeat a credential-named argument (the
				// webhook Secret) is withheld whole; it sends nothing either, which
				// the GET-only check below proves.
				// So is the token refresh's: its body carries the stored refresh
				// token when the login minted one.
				credArg := strings.Contains(fmt.Sprint(call), "Secret:") || strings.Contains(fmt.Sprint(call), "security.refresh-token")
				if !strings.Contains(r, "confirmation_required") && !strings.Contains(r, "Preview only: nothing was sent") && !(credArg && strings.Contains(r, "credkit: withheld")) {
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
			if mode == credStored {
				checkMode(t, sv, mode)
			} else if n, all := sv.sawHeader(mode.header()); n == 0 || len(all) != 1 {
				// The token was minted and cached by the sync above.
				t.Errorf("%s: want every API request to carry %q, got %d of it and %v", mode, mode.header(), n, all)
			}
		})
	}
}

// Control for the test above: with confirm set to boolean true the same write
// reaches the API, so the gate previews rather than drops; the string "true"
// still previews and sends nothing at all, the login included (the execute
// preview builds a client that never mints: mcp-preview-never-mints).
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	for _, mode := range []credMode{credStored, credLogin} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			update := func(confirm any) []map[string]any {
				return []map[string]any{{"name": "kaseya-bms_execute", "arguments": map[string]any{
					"endpoint_id": "servicedesk.put-ticket", "params": map[string]any{"ticketId": bbTicket, "Title": "HQ map"}, "confirm": confirm}}}
			}
			preview := e.mcpPhases(t, update("true"))
			if !strings.Contains(preview, "confirmation_required") {
				t.Fatalf("confirm \"true\" (a string) returned no preview:\n%.1500s", preview)
			}
			if methods, mints, minted, bad := sv.counts(); len(methods) != 0 || len(bad) != 0 || mints != 0 {
				t.Errorf("confirm \"true\" (a string) sent %v (%d login requests, %d accepted, refused %v)", methods, mints, minted, bad)
			}
			sv.mu.Lock()
			off := sv.offHost
			sv.mu.Unlock()
			if off != 0 {
				t.Errorf("confirm \"true\" (a string) reached another host %d time(s)", off)
			}
			e.mcpPhases(t, update(true))
			methods, _, _, _ := sv.counts()
			var sent bool
			for _, m := range methods {
				sent = sent || m == "PUT /v2/servicedesk/tickets/"+bbTicket
			}
			if !sent {
				t.Fatalf("confirm true did not send the PUT; requests: %v", methods)
			}
			checkMode(t, sv, mode)
		})
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, and never
// mints, from a cold start with a stored token, the login, or the login on a
// cached token.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel() // its own server, home and env: independent of every other test
	for _, mode := range []credMode{credStored, credLogin, credLoginCached} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			accounts := e.importFile(t, "Dry")
			for _, args := range [][]string{
				ticketWrite("servicedesk", "put-ticket", bbTicket, "--title", "Dry Ticket", "--dry-run"),
				ticketWrite("servicedesk", "post-ticket", "--title", "Dry Ticket", "--dry-run"),
				{"servicedesk", "delete-ticket", bbTicket, "--dry-run"},
				{"admin", "post-webhook-configuration", "--name", "Dry Hook", "--secret", bbArgvSec, "--dry-run"},
				{"security", "refresh-token", "--access-token", bbArgvSec, "--refresh-token", bbArgvSec, "--dry-run"},
				{"auth", "login", "--dry-run"},
				{"import", "crm", "--input", accounts, "--dry-run"},
			} {
				// Each run reaches its preview, so the count below is not vacuous
				// (a flag error would send nothing either).
				out := e.run(t, args...)
				if !strings.Contains(out, "dry_run") && !strings.Contains(out, "would ") && !strings.Contains(out, "dry run") {
					t.Errorf("%v (%s) printed no preview:\n%.1500s", args, mode, out)
				}
				assertNoEcho(t, fmt.Sprintf("preview of %v", args), out)
			}
			if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
				t.Errorf("--dry-run (%s) sent %v and %d login request(s)", mode, methods, mints)
			}
			// Control: the same write without --dry-run reaches the fake.
			e.run(t, ticketWrite("servicedesk", "put-ticket", bbTicket, "--title", "Dry Ticket")...)
			if methods, _, _, _ := sv.counts(); len(methods) != 1 || methods[0] != "PUT /v2/servicedesk/tickets/"+bbTicket {
				t.Errorf("control (%s): want the one PUT, got %v", mode, methods)
			}
			checkMode(t, sv, mode)
		})
	}
}

// mint-pinned-to-api-host + api-origin-pinned, on the built binary: auth login
// with KASEYA_BMS_BASE_URL set to a regional host mints at that host's
// /v2/security/authenticate and nowhere else (never at the US default). Every
// non-loopback request the binary makes goes through a recording proxy
// (HTTPS_PROXY, HTTP_PROXY), which answers nothing, so a login aimed at another
// host is caught without reaching the network. The token login saved then
// authenticates the next run with no login at all.
func TestAuthLoginMintsOnlyAtItsHost(t *testing.T) {
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
	e := newEnv(t, sv, credLogin)
	e.env = append(e.env, "HTTPS_PROXY="+proxy.URL, "HTTP_PROXY="+proxy.URL, "https_proxy="+proxy.URL, "http_proxy="+proxy.URL, "NO_PROXY=", "no_proxy=")
	out := e.run(t, "auth", "login")
	mu.Lock()
	left := append([]string(nil), escaped...)
	mu.Unlock()
	if len(left) != 0 {
		t.Fatalf("auth login sent %d request(s) to another host: %v\n%.1500s", len(left), left, out)
	}
	_, mints, minted, bad := sv.counts()
	if mints != 1 || minted != 1 {
		t.Fatalf("auth login: want one accepted mint at the configured host, got %d requests, %d accepted (refused %v):\n%.1500s", mints, minted, bad, out)
	}
	if !strings.Contains(out, "Logged in to tenant "+bbTenant) {
		t.Fatalf("auth login did not report success:\n%.1500s", out)
	}
	assertNoEcho(t, "auth login output", out)
	if !e.credentialStoreHolds(bbMintAccess) {
		t.Errorf("auth login stored no token in the credential store")
	}
	if e.credentialStoreHolds(bbPassword) {
		t.Errorf("auth login persisted the environment password")
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
	// The token it saved authenticates the next run against the same host
	// without another mint and without the login, and still nothing leaves.
	sv.reset()
	sv.setEcho(echoNone) // the record prints whole (the kit withholds a document that echoes the token)
	e.env = withoutEnv(withoutEnv(withoutEnv(e.env, "KASEYA_BMS_USERNAME"), "KASEYA_BMS_PASSWORD"), "KASEYA_BMS_TENANT")
	if read := e.run(t, "servicedesk", "get-tickets", "--json"); !strings.Contains(read, "HQ-Ticket") {
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
