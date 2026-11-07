// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package domotzblackbox runs the built CLI and MCP binaries against a
// Domotz Public API fake (/public-api/v1: bare JSON arrays paged by
// page_size / page_number with an X-Entities-Count header, bare objects, 201
// creates, 204 updates and deletes) that echoes every credential it knows
// back in every encoding it can, whole and split across lines, adjacent
// values and pages, and checks what a reviewer cannot read off the code
// (domotz handfixes output-chokepoint, store-never-keeps-secrets,
// no-response-cache, file-sinks-withheld, response-credentials-held,
// api-origin-pinned, region-origin-pinned, dry-run-sends-nothing,
// mcp-endpoint-write-confirm, mcp-mirror-write-confirm,
// domotz-credential-names-exact):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the API key from DOMOTZ_API_KEY, from the
//     credential store (auth set-token) or from a config file's api_key, a
//     key the configuration holds but does not send (a stored key the
//     environment overrides, a config file's access_token, which Domotz
//     never takes), the MCP HTTP token, a credential-named flag or MCP
//     argument value (a device's credentials password, an SNMP key), and the
//     credentials a response carries (a device's SNMP read community, a
//     device's SNMP v3 username, authentication key and encryption key, a
//     custom-driver association's credentials, a new team leader's
//     password);
//   - no file the program writes, except the 0600 credential store, holds
//     one either (the local store, receipts, exports, delivered files, the
//     feedback log, the learn journal, profiles, teach logs);
//   - no credential leaves for a host other than the configured one, by
//     redirect or by a region (DOMOTZ_REGION, sync --path-context region=)
//     that would move the resolved https://api-{region}.domotz.com origin;
//   - the MCP server sends a write (a POST, PUT or DELETE) only when the call
//     sets confirm to the boolean true, and --dry-run sends nothing at all.
//
// Domotz names that read as credentials but are product data (an inventory
// field's key, an activity log's user, a VPN session id, a device's
// authentication_status, a user's authentication_type and
// required_authentication_type, the SNMP authentication_protocol, a custom
// driver's requires_credentials) are never withheld.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package domotzblackbox

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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bbToken      = "domotzTOKEN7fb3189bQ6u3sZxW4o1n7R8v52" // DOMOTZ_API_KEY (sent raw as X-Api-Key: <key>)
	bbStored     = "storedToken-71c3e9b05d2a4f68e1"        // auth set-token (stdin), the credential store
	bbFileToken  = "configFileToken-9d4c2b7e1a5f38"        // api_key in a 0600 config file named by DOMOTZ_CONFIG
	bbFileAccess = "configFileAccess-2b9d7e4a1c63"         // access_token in a 0600 config file (held; Domotz takes only the API key)
	bbArgvSec    = "-argvSecret-91c3e7a5"                  // --password on the command line (set-credentials; starts with "-")
	bbMCPArgSec  = "mcpArgSecret-81d3c6f0"                 // a credential-named argument an MCP call previews (never sent)
	bbBOMResp    = "BomRespSecret-6a1c"                    // a custom-driver association's credentials password, BOM-prefixed response
	bbNewSecret  = "newLeaderPassword-0b7e3d5a9c1f"        // the team leader password POST /area/{area_id}/team returns
	bbDriverUser = "driverUser-5c1e9a27"                   // a custom-driver association's credentials username
	bbHTTPToken  = "mcpHttpToken-0123456789abcdefABCDEF"   // DOMOTZ_MCP_HTTP_TOKEN
	bbAgentID    = "12345"                                 // a collector (agent), online
	bbAgent2     = "12346"                                 // a second collector, offline, no devices
	bbDeviceID   = "67890"                                 // a device of bbAgentID, online, authentication REQUIRED
	bbDevice2    = "67891"                                 // a second device of bbAgentID, offline
	bbDriverID   = "4321"                                  // a custom driver
	bbAssocID    = "77"                                    // a custom-driver association
	bbAreaID     = "5"                                     // an area
	bbTeamID     = "9"                                     // a team
	bbTagID      = "88"                                    // a custom tag
	bbMemberID   = "501"                                   // an RBAC user
	bbRedirect   = "9911"                                  // a device ID the fake redirects off-host
	bbDenied     = "40199"                                 // an agent or device ID (or a query value) the fake answers 401
	// Domotz product data under names that read as credentials: never
	// withheld (domotz-credential-names-exact).
	bbInvKey      = "hq-rack-slot7"                  // an inventory field's key (its name)
	bbLogUser     = "noc-tech@hq-record.example.com" // an activity log's user
	bbVPNSession  = "730519"                         // a VPN session id
	bbAuthType    = "HQ-SSO"                         // a user's authentication_type
	bbReqAuthType = "HQ-2FA"                         // a user's required_authentication_type
)

func secrets() []string {
	return []string{bbToken, bbStored, bbFileToken, bbFileAccess, bbArgvSec, bbMCPArgSec,
		bbBOMResp, bbNewSecret, bbDriverUser, bbHTTPToken}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "domotz-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "domotz-cli"+exe)
	bbMCP = filepath.Join(dir, "domotz-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/domotz-cli", bbMCP: "../../cmd/domotz-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	fmt.Fprintf(os.Stderr, "domotzblackbox: %d CLI runs, %d MCP tools/call requests\n", cliRuns.n, mcpCallsSent.n)
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
		"X-Api-Key: " + s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
		string(pem.EncodeToMemory(&pem.Block{Type: "DOMOTZ API KEY", Bytes: []byte(s)})),
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
	keys    map[string]int // keys the API received as the one header X-Api-Key: <key>
	// otherAuth counts requests whose credential came any other way: an
	// Authorization header, a second X-Api-Key value, a key with a scheme
	// word ("Bearer <key>"), or a key in the query (Domotz takes only the raw
	// X-Api-Key header).
	otherAuth int
	// pages counts each list page the fake served, by "/path?page_number=N".
	pages map[string]int
	// redirectAgents sends the collector list (the syncable resource every
	// novel reads) to the other host too, so sync meets a redirect.
	redirectAgents bool
	// splitGroups names the two variables pages' records with the halves of
	// the first credential the fake was sent. Under the credential kit a store
	// write carrying a half is refused, so only the test of that split turns
	// it on.
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

// isWrite reports a request that changes data (HEAD, a count, does not).
func isWrite(m string) bool {
	method, _, _ := strings.Cut(m, " ")
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

// pageQuery is a Domotz list request's page: page_size (def when absent,
// at most 1000) and the 0-indexed page_number.
func pageQuery(q url.Values, def int) (size, number int) {
	size, number = def, 0
	if n, err := strconv.Atoi(q.Get("page_size")); err == nil && n > 0 {
		size = min(n, 1000)
	}
	if n, err := strconv.Atoi(q.Get("page_number")); err == nil && n >= 0 {
		number = n
	}
	return size, number
}

// pagedArray is one page of a Domotz list: items[number*size:...] as a bare
// JSON array, with the total in X-Entities-Count.
func pagedArray(w http.ResponseWriter, q url.Values, def int, items []string) string {
	size, number := pageQuery(q, def)
	w.Header().Set("X-Entities-Count", strconv.Itoa(len(items)))
	start := min(number*size, len(items))
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

var (
	reAgent      = regexp.MustCompile(`^/agent/[^/]+$`)
	reDevices    = regexp.MustCompile(`^/agent/[^/]+/device$`)
	reDevice     = regexp.MustCompile(`^/agent/[^/]+/device/[^/]+$`)
	reVariables  = regexp.MustCompile(`^/agent/[^/]+/variable$`)
	reTeams      = regexp.MustCompile(`^/area/[^/]+/team$`)
	reDriver     = regexp.MustCompile(`^/custom-driver/[0-9]+$`)
	reObjectRead = regexp.MustCompile(`(/uptime|/eye-statistics|/metric-statistics|-policy|/connection/consumption|^/user|^/meta/usage|^/rbac/(member|user-group|role)/[^/]+)$`)
)

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{keys: map[string]int{}, pages: map[string]int{}, denied: map[string]bool{}}
	// A second host the API redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	now := time.Now().UTC()
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	firstSeen := now.Add(-2 * time.Hour).Format(time.RFC3339)
	// agent is a collector as GET /agent and GET /agent/{agent_id} return it.
	agent := func(id, name, status, extra string) string {
		return fmt.Sprintf(`{"id":%s,"display_name":%q,"status":{"value":%q,"last_change":%q},"creation_time":%q,"timezone":"UTC",`+
			`"version":{"agent":"5.9.4","package":"5.9.4-1"},"team":{"id":%s,"name":"HQ-Record team"},`+
			`"location":{"latitude":"45.07","longitude":"7.68"},"organization":{"id":3,"name":"HQ-Record Org"},`+
			`"installation_info":{"os":{"name":"Linux","version":"6.1"},"platform":"debian"},`+
			`"wan_info":{"ip":"203.0.113.7","hostname":"hq-record.example.com"},"tags":[]%s}`,
			id, name, status, recent, recent, bbTeamID, extra)
	}
	// device is a LocalIpDevice as GET /agent/{agent_id}/device and GET
	// /agent/{agent_id}/device/{device_id} return it.
	device := func(id, name, status, importance, extra string) string {
		return fmt.Sprintf(`{"id":%s,"display_name":%q,"protocol":"IP","ip_addresses":["192.168.10.%s"],"hw_address":"00:1a:2b:3c:4d:%s",`+
			`"status":%q,"importance":%q,"authentication_status":"REQUIRED","snmp_status":"CHECKING","first_seen_on":%q,`+
			`"last_status_change":%q,"vendor":"HQ-Record Networks","model":"HQ-9000","main_id":%s,`+
			`"type":{"id":2,"label":"Switch","detected_id":2},"os":{"name":"HQ-OS","version":"1.2"},`+
			`"user_data":{"name":%q,"vendor":"HQ-Record","model":"HQ-9000","type":2},`+
			`"details":{"zone":"HQ","room":"rack 7","serial":"SN-HQ-%s"},"is_excluded":false,"agent_reachable":true%s}`,
			id, name, id[len(id)-2:], id[len(id)-2:], status, importance, firstSeen, recent, id, name, id, extra)
	}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		// Under the credential kit a response that repeats a held credential is
		// withheld whole, so the records the tests read carry no echo of what
		// the request sent (internal/credwire/leak_test.go proves the echoes).
		echo := "null"
		p := strings.TrimPrefix(r.URL.Path, "/public-api/v1")
		q := r.URL.Query()
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		keys := r.Header.Values("X-Api-Key")
		if len(keys) == 1 && keys[0] != "" && !strings.ContainsAny(keys[0], " \t") {
			sv.keys[keys[0]]++
		} else if len(keys) > 0 {
			sv.otherAuth++
		}
		if len(r.Header.Values("Authorization")) > 0 {
			sv.otherAuth++
		}
		for _, k := range []string{"api_key", "apikey", "x-api-key", "token", "access_token"} {
			if q.Has(k) {
				sv.otherAuth++
			}
		}
		pn := q.Get("page_number")
		if pn == "" {
			pn = "0"
		}
		if r.Method == http.MethodGet {
			sv.pages[p+"?page_number="+pn]++
		}
		redirectAgents := sv.redirectAgents
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
			// Rejected (a denied collector or device, a sync or export the
			// test denies): the error repeats what it was sent.
			sv.mu.Lock()
			sv.denials++
			sv.mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"message":"invalid api key %s","detail":[{"msg":"bad key %s","type":"auth"}],"echo":%s}`,
				strings.Join(sent, " "), strings.Join(sent, " "), echo)
		case p == "/agent/"+bbAgentID+"/device/"+bbRedirect || (redirectAgents && p == "/agent"):
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case r.Method == http.MethodHead:
			w.Header().Set("X-Entities-Count", "2")
			w.WriteHeader(http.StatusNoContent)
		case get && p == "/agent":
			// The fleet the novels read: an online collector carrying the echo
			// and an offline one, paged as Domotz pages (page_size, default
			// 10; 0-indexed page_number).
			fmt.Fprint(w, pagedArray(w, q, 10, []string{
				agent(bbAgentID, "HQ-Record HQ", "ONLINE", `,"echo":`+echo),
				agent(bbAgent2, "HQ-Record Branch", "OFFLINE", ""),
			}))
		case get && reAgent.MatchString(p):
			fmt.Fprint(w, agent(strings.TrimPrefix(p, "/agent/"), "HQ-Record HQ", "ONLINE", `,"listen_on":"eth0","echo":`+echo))
		case get && p == "/agent/"+bbAgentID+"/device":
			// The devices, gzip-encoded: the online one needs credentials (fleet
			// unmonitored), the printer is offline (fleet offline).
			gzipped(w, "["+device(bbDeviceID, "HQ-Record core switch", "ONLINE", "VITAL", `,"echo":`+echo)+","+
				device(bbDevice2, "HQ-Record printer", "OFFLINE", "FLOATING", "")+"]")
		case get && reDevices.MatchString(p):
			fmt.Fprint(w, `[]`)
		case get && p == "/agent/"+bbAgentID+"/device/"+bbDeviceID+"/snmp-authentication":
			// A device's SNMP v3 authentication, gzip-encoded, without its
			// username and keys (the kit would withhold the record whole).
			gzipped(w, `{"version":"V3_AUTH_PRIV","authentication_protocol":"SHA-256","encryption_protocol":"AES","echo":null}`)
		case get && reDevice.MatchString(p):
			fmt.Fprint(w, device(p[strings.LastIndex(p, "/")+1:], "HQ-Record core switch", "ONLINE", "VITAL", ""))
		case get && reVariables.MatchString(p):
			// The first credential it was sent, split across two pages of
			// collector variables, paged as Domotz pages (page_size, default
			// 100, full pages until the last; 0-indexed page_number): the
			// first variable of page 0 is one half, the one variable of page
			// 1 the other, so every pager that walks the pages meets both
			// halves as records of their own.
			a, b := "variable-a", "variable-b"
			sv.mu.Lock()
			split := sv.splitGroups
			sv.mu.Unlock()
			if split && len(sent) > 0 {
				a, b = halves(sent[0])
			}
			size, _ := pageQuery(q, 100)
			variable := func(id int, label string) string {
				return fmt.Sprintf(`{"id":%d,"path":"hq/v%d","label":%q,"value":"42","metric":"hq","has_history":false,"value_update_time":%q}`, id, id, label, recent)
			}
			items := []string{variable(1, a)}
			for i := 1; i < size; i++ {
				items = append(items, variable(1+i, fmt.Sprintf("HQ-Record variable %d", i)))
			}
			items = append(items, variable(1+size, b))
			fmt.Fprint(w, pagedArray(w, q, 100, items))
		case get && p == "/custom-driver/agent/"+bbAgentID+"/association":
			// Custom-driver associations: BOM-prefixed, with the credentials
			// the association was created with.
			fmt.Fprint(w, "\xef\xbb\xbf"+fmt.Sprintf(`[{"id":%s,"custom_driver_id":%s,"device_id":%s,"status":"ENABLED","sample_period":300,`+
				`"last_inspection_time":%q,"used_variables":3,"credentials":{"username":%q,"password":%q},`+
				`"parameters":[{"custom_driver_parameter_id":1,"name":"ups_model","value":"HQ-Record UPS","value_type":"STRING"}],"echo":%s}]`,
				bbAssocID, bbDriverID, bbDeviceID, recent, bbDriverUser, bbBOMResp, echo))
		case get && (p == "/custom-driver" || reDriver.MatchString(p)):
			// Custom drivers (list and get, sync, export): gzip-encoded.
			driver := fmt.Sprintf(`{"id":%s,"name":"HQ-Record UPS driver","type":"GENERIC","description":"HQ-Record UPS","minimal_sample_period":300,`+
				`"is_valid":true,"requires_credentials":true,"echo":%s}`, bbDriverID, echo)
			doc := driver
			if p == "/custom-driver" {
				doc = "[" + driver + "]"
			}
			gzipped(w, doc)
		case get && strings.HasSuffix(p, "/activity-log"):
			fmt.Fprintf(w, `[{"type":"vpn_session_start","description":"HQ-Record VPN opened","user":%q,"timestamp":%q,"device_id":%s,"details":{},"echo":%s}]`,
				bbLogUser, recent, bbDeviceID, echo)
		case get && strings.HasSuffix(p, "/inventory") && strings.HasPrefix(p, "/agent/"):
			fmt.Fprintf(w, `[{"key":%q,"value":"HQ-Record rack 7","creation_time":%q,"echo":%s}]`, bbInvKey, recent, echo)
		case get && p == "/inventory":
			fmt.Fprintf(w, `[{"key":%q,"label":"HQ-Record rack slot","defined_by_user":1,"creation_time":%q}]`, bbInvKey, recent)
		case get && strings.HasSuffix(p, "/connection/vpn-session"):
			fmt.Fprintf(w, `[{"id":%s,"status":"ACTIVE","name":"HQ-Record tech","bytes":1024,"creation_time":%q,"expiration_time":%q,"echo":%s}]`,
				bbVPNSession, recent, recent, echo)
		case get && p == "/rbac/member":
			fmt.Fprintf(w, `{"users":[{"id":%s,"email":"tech@hq-record.example.com","display_name":"HQ-Record tech","is_external":false,`+
				`"authentication_type":%q,"required_authentication_type":%q,"user_groups":[],"echo":%s}]}`, bbMemberID, bbAuthType, bbReqAuthType, echo)
		case get && p == "/rbac/role":
			fmt.Fprintf(w, `{"roles":[{"id":1,"name":"HQ-Record admin","description":"HQ-Record","echo":%s}]}`, echo)
		case get && p == "/rbac/user-group":
			fmt.Fprintf(w, `{"user_groups":[{"id":3,"name":"HQ-Record NOC","echo":%s}]}`, echo)
		case get && p == "/custom-tag":
			fmt.Fprintf(w, `{"tags":[{"id":%s,"name":"HQ-Record","color":"red","echo":%s}]}`, bbTagID, echo)
		case get && p == "/alert-profile":
			fmt.Fprintf(w, `[{"id":21,"name":"HQ-Record alerts","is_enabled":true,"description":"HQ-Record","events":["device_status_down"],"echo":%s}]`, echo)
		case get && p == "/area":
			fmt.Fprintf(w, `[{"id":%s,"name":"HQ-Record area","echo":%s}]`, bbAreaID, echo)
		case get && reTeams.MatchString(p):
			fmt.Fprintf(w, `[{"id":%s,"name":"HQ-Record team","echo":%s}]`, bbTeamID, echo)
		case get && strings.HasSuffix(p, "/history/network/speed"):
			fmt.Fprintf(w, `[{"timestamp":%q,"values":[94000000,18000000]}]`, recent)
		case get && strings.HasSuffix(p, "/ip-conflict"):
			fmt.Fprintf(w, `[{"ip":"192.168.10.90","conflicting_devices":[{"id":%s,"hw_address":"00:1a:2b:3c:4d:90"}],"echo":%s}]`, bbDeviceID, echo)
		case get && strings.HasSuffix(p, "/eye/tcp"):
			fmt.Fprintf(w, `[{"id":31,"port":443,"status":"DOWN","last_update":%q,"device_id":%s,"echo":%s}]`, recent, bbDeviceID, echo)
		case get && strings.HasSuffix(p, "/network-topology"):
			fmt.Fprintf(w, `{"edges":[{"from":%s,"to":%s}],"echo":%s}`, bbDeviceID, bbDevice2, echo)
		case get && reObjectRead.MatchString(p):
			fmt.Fprintf(w, `{"id":1,"name":"HQ-Record","echo":%s}`, echo)
		case get:
			// Every other read is a list.
			fmt.Fprintf(w, `[{"id":1,"name":"HQ-Record","echo":%s}]`, echo)
		case r.Method == http.MethodPost && reTeams.MatchString(p):
			// A new team comes back with its leader's password.
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"id":%s,"name":"HQ-Record team","leader":{"name":"HQ-Record lead","password":%q},"echo":%s}`, bbTeamID, bbNewSecret, echo)
		case r.Method == http.MethodPost:
			// Every other create echoes what it was sent.
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"id":7,"name":"HQ-Record","status":"ok","echo":%s}`, echo)
		default:
			// PUT and DELETE: 204, as Domotz answers them.
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	sv.url = srv.URL + "/public-api/v1"
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

// sawKey reports how many API requests carried X-Api-Key: v, and the
// distinct keys the fake received.
func (sv *server) sawKey(v string) (int, []string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var all []string
	for h := range sv.keys {
		all = append(all, h)
	}
	return sv.keys[v], all
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

// credMode is where the process gets its API key.
type credMode int

const (
	credStored        credMode = iota // none in the environment: the battery runs on a key stored with auth set-token
	credEnv                           // DOMOTZ_API_KEY
	credEnvOverStored                 // a stored key and DOMOTZ_API_KEY: the environment wins, the stored key is held unused
	credConfigFile                    // api_key in a 0600 config file named by DOMOTZ_CONFIG
	credConfigAccess                  // access_token alone in a 0600 config file: held, never sent (Domotz takes only the API key)
	credConfigBoth                    // both config keys: api_key is sent, access_token is held unused
)

func (m credMode) String() string {
	return [...]string{"stored", "env DOMOTZ_API_KEY", "env over stored", "config api_key", "config access_token", "config both keys"}[m]
}

// sent is the key the mode sends ("" when it sends none); held are the
// credentials it holds but never sends.
func (m credMode) sent() string {
	return [...]string{bbStored, bbToken, bbToken, bbFileToken, "", bbFileToken}[m]
}

func (m credMode) held() []string {
	switch m {
	case credEnvOverStored:
		return []string{bbStored, bbHTTPToken}
	case credConfigAccess, credConfigBoth:
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
		"DOMOTZ_BASE_URL=" + sv.url, "DOMOTZ_CA_CERT=" + sv.caFile,
		"DOMOTZ_CLI_PATH=" + bbCLI, "DOMOTZ_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	e := &bbEnv{home: home, env: env}
	switch mode {
	case credStored:
		e.storeToken(t, bbStored)
	case credEnv:
		e.env = append(e.env, "DOMOTZ_API_KEY="+bbToken)
	case credEnvOverStored:
		e.storeToken(t, bbStored)
		e.env = append(e.env, "DOMOTZ_API_KEY="+bbToken)
	case credConfigFile:
		e.writeConfig(t, fmt.Sprintf("api_key = %q\n", bbFileToken))
	case credConfigAccess:
		e.writeConfig(t, fmt.Sprintf("access_token = %q\n", bbFileAccess))
	case credConfigBoth:
		e.writeConfig(t, fmt.Sprintf("api_key = %q\naccess_token = %q\n", bbFileToken, bbFileAccess))
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
// DOMOTZ_CONFIG.
func (e *bbEnv) writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := filepath.Join(e.home, "cfg")
	_ = os.MkdirAll(dir, 0o700)
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "DOMOTZ_CONFIG"), "DOMOTZ_CONFIG="+cfg)
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
// import write path (POST /custom-tag per record).
func (e *bbEnv) importFile(t *testing.T, tok string) string {
	t.Helper()
	p := filepath.Join(e.home, "import.jsonl")
	if err := os.WriteFile(p, []byte(`{"name":"Imported `+tok+`","color":"red"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// associationBody is a custom-driver association create --stdin body: the
// credentials the driver runs with (username and password), the password pw.
func associationBody(pw string) string {
	b, _ := json.Marshal(map[string]any{
		"sample_period": 300,
		"credentials":   map[string]any{"username": "hq-driver", "password": pw},
		"parameters":    []any{map[string]any{"custom_driver_parameter_id": 1, "value": "HQ-Record UPS"}},
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

// novels is every novel command; the store-backed ones read the local store
// after sync, the live ones fan out over the synced or listed collectors.
func novels() [][]string {
	return [][]string{
		{"fleet", "health", "--json"},
		{"fleet", "health"},
		{"fleet", "agents", "--json"},
		{"fleet", "agents"},
		{"fleet", "offline", "--json"},
		{"fleet", "offline"},
		{"fleet", "new", "--since", "24h", "--json"},
		{"fleet", "inventory", "--json"},
		{"fleet", "inventory", "--csv"},
		{"fleet", "breakdown", "--by", "vendor", "--json"},
		{"fleet", "alerts", "--json"},
		{"fleet", "unmonitored", "--json"},
		{"fleet", "unmonitored"},
		{"fleet", "stale", "--max-age", "0", "--json"},
		{"fleet", "ip-conflicts", "--json"},
		{"fleet", "speedtest", "--json"},
		{"fleet", "triggers", "--json"},
		{"fleet", "events", "--since", "24h", "--json"},
		{"topology", bbAgentID, "--json"},
		{"topology", bbAgentID, "--cached", "--json"},
		{"drift", bbAgentID, "--json"},
		{"drift", bbAgentID},
		{"analytics", "--type", "agent", "--json"},
	}
}

// battery runs every output path; tok is a credential the process holds
// (the key it sends: DOMOTZ_API_KEY, the stored key or a config file's
// api_key, or a held access_token).
func battery(t *testing.T, e *bbEnv, deliverFile, tok string) []cliCall {
	imp := e.importFile(t, tok)
	b := calls(
		// Every output mode.
		[]string{"agent", "list", "--json"},
		[]string{"agent", "list"},
		[]string{"agent", "list", "--csv"},
		[]string{"agent", "list", "--plain"},
		[]string{"agent", "list", "--agent"},
		[]string{"agent", "list", "--json", "--receipt"},
		[]string{"agent", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		[]string{"agent", "list", "--json", "--deliver", "file:" + deliverFile},
		// Every response shape: a device's short SNMP read community
		// (gzip-encoded list), split echo, the SNMP v3 keys (gzip), a
		// custom-driver association's credentials (BOM-prefixed), an echoed
		// identifier (gzip), 401 echo, refused redirect, product data under
		// credential-like names, a new team leader's password.
		[]string{"agent", "device", "list", bbAgentID, "--json"},
		[]string{"agent", "device", "list", bbAgentID},
		[]string{"agent", "device", "get", bbAgentID, bbDeviceID, "--json"},
		[]string{"agent", "device", "get", bbAgentID, bbDeviceID},
		[]string{"agent", "device", "get", bbAgentID, bbDenied, "--json"},
		[]string{"agent", "device", "get", bbAgentID, bbRedirect, "--json"},
		[]string{"agent", "list", "--display-name", bbDenied, "--json"},
		[]string{"agent", "get", bbDenied, "--json"},
		[]string{"agent", "get", bbAgentID, "--json"},
		[]string{"agent", "device", "get-snmpauthentication", bbAgentID, bbDeviceID, "--json"},
		[]string{"agent", "device", "get-snmpauthentication", bbAgentID, bbDeviceID},
		[]string{"custom-driver", "list-associations", bbAgentID, "--json"},
		[]string{"custom-driver", "list-associations", bbAgentID},
		[]string{"custom-driver", "list", "--json"},
		[]string{"custom-driver", "list"},
		[]string{"custom-driver", "get", bbDriverID, "--json"},
		[]string{"agent", "activity-log", "get-agent", bbAgentID, "--json"},
		[]string{"agent", "device", "get-inventory", bbAgentID, bbDeviceID, "--json"},
		[]string{"agent", "connection", "get-agent-vpnactive", bbAgentID, "--json"},
		[]string{"rbac", "get-users", "--json"},
		[]string{"inventory", "get", "--json"},
		[]string{"custom-tag", "get", "--json"},
		[]string{"alert-profile", "get-alert-profiles2", "--json"},
		[]string{"area", "list", "--json"},
		[]string{"area", "team", "list", bbAreaID, "--json"},
		[]string{"agent", "variable", "list-agent", bbAgentID, "--json"},
		[]string{"agent", "variable", "list-agent", bbAgentID, "--all", "--json"},
		[]string{"agent", "variable", "list-agent", bbAgentID, "--page-number", "1", "--page-size", "1", "--json"},
		[]string{"agent", "history", "get-speed-test", bbAgentID, "--json"},
		[]string{"agent", "network-topology", "get", bbAgentID, "--json"},
		[]string{"tail", "agent", "--follow=false"},
		// Writes the fake echoes (each with and without --yes where it
		// changes data; a write without --yes is the work order's P2, but it
		// must never leak either).
		[]string{"custom-tag", "create", "--name", "HQ " + tok, "--color", "red", "--json"},
		[]string{"custom-tag", "edit", bbTagID, "--name", "HQ " + tok, "--yes", "--json"},
		[]string{"custom-tag", "edit", bbTagID, "--name", "HQ " + tok},
		[]string{"custom-tag", "delete", bbTagID, "--yes", "--json"},
		[]string{"custom-tag", "delete", bbTagID},
		[]string{"agent", "device", "delete", bbAgentID, bbDevice2, "--yes", "--json"},
		[]string{"agent", "device", "set-credentials", bbAgentID, bbDeviceID, "--username", "hq-admin", "--password", tok, "--yes", "--json"},
		[]string{"agent", "device", "set-credentials", bbAgentID, bbDenied, "--username", "hq-admin", "--password", tok, "--yes", "--json"},
		[]string{"agent", "device", "set-snmpauthentication", bbAgentID, bbDeviceID, "--version", "V3_AUTH_PRIV", "--username", "hq-snmp",
			"--authentication-protocol", "SHA", "--authentication-key", tok, "--encryption-protocol", "AES", "--encryption-key", tok, "--yes", "--json"},
		[]string{"agent", "device", "set-snmp-community", bbAgentID, bbDeviceID, "--read", tok, "--json"},
		[]string{"area", "team", "create", bbAreaID, "--name", "HQ team", "--leader-name", "HQ lead", "--leader-password", "HQ-Lead-Pass-1",
			"--leader-details-display-name", "HQ Lead", "--yes", "--json"},
		[]string{"area", "team", "create", bbAreaID, "--name", "HQ " + tok, "--leader-name", "HQ lead", "--leader-password", tok,
			"--leader-details-display-name", "HQ Lead"},
		[]string{"custom-driver", "agent", "create-custom-driver-association", bbDriverID, bbAgentID, bbDeviceID,
			"--credentials-username", "hq-driver", "--credentials-password", tok, "--sample-period", "300", "--json"},
		[]string{"agent", "connection", "delete-agent-vpnconnection", bbAgentID, bbVPNSession, "--yes", "--json"},
		[]string{"rbac", "create-user", "--email", "tech@hq-record.example.com", "--name", "HQ " + tok, "--user-group-ids", "[3]", "--json"},
		[]string{"import", "custom-tag", "--input", imp, "--json"},
		// The local store, exports and the novel commands.
		[]string{"sync", "--json"},
		[]string{"workflow", "archive", "--json"},
		// custom-driver, not agent: agent is the one page_number-paged export
		// and its first page is TestExportWalksEveryPage's finding.
		[]string{"export", "custom-driver", "--format", "jsonl", "--output", filepath.Join(e.home, "export.jsonl")},
		[]string{"export", "agent", "--format", "json"},
		[]string{"export", "custom-tag", "--format", "json"},
		[]string{"search", "HQ", "--json"},
		[]string{"search", "Echo", "--json", "--data-source", "local"},
		[]string{"workflow", "status", "--json"},
	)
	// A request body on stdin: a custom-driver association's credentials
	// (the held key itself as its password).
	b = append(b, cliCall{stdin: associationBody(tok), args: []string{"custom-driver", "agent", "create-custom-driver-association",
		bbDriverID, bbAgentID, bbDeviceID, "--stdin", "--json"}})
	b = append(b, calls(novels()...)...)
	return append(append(b, calls(
		// A sync that meets a 401 (a filtered walk into its own store, after
		// the novels: the main store stays whole).
		[]string{"sync", "--db", filepath.Join(e.home, "denied.db"), "--resources", "agent", "--param", "display_name=" + bbDenied, "--json"},
		// A credential on the command line: a credential-named flag, an unknown
		// flag, a positional, feedback text.
		[]string{"agent", "device", "set-credentials", bbAgentID, bbDeviceID, "--username", "argv", "--password", bbArgvSec},
		[]string{"agent", "device", "set-snmp-community", bbAgentID, bbDeviceID, "--read", bbArgvSec, "--write", bbArgvSec, "--dry-run"},
		[]string{"agent", "device", "set-snmp-community", bbAgentID, bbDeviceID, "--read", bbArgvSec, "--json"},
		[]string{"agent", "list", "--json", "--" + tok},
		[]string{"agent", "get", tok, "--json"},
		[]string{"feedback", "noted the key " + tok + " in a ticket"},
		// Previews: a dry run sends nothing.
		[]string{"custom-tag", "edit", bbTagID, "--name", "Echo " + tok, "--dry-run"},
		[]string{"custom-tag", "create", "--name", "Echo " + tok, "--dry-run"},
		[]string{"agent", "device", "set-credentials", bbAgentID, bbDeviceID, "--username", "hq-admin", "--password", tok, "--dry-run"},
		[]string{"agent", "device", "set-snmpauthentication", bbAgentID, bbDeviceID, "--version", "V3_AUTH_PRIV", "--authentication-key", tok,
			"--encryption-key", tok, "--dry-run"},
		[]string{"agent", "device", "delete", bbAgentID, bbDeviceID, "--dry-run"},
		[]string{"import", "custom-tag", "--input", imp, "--dry-run"},
		// Learnings, playbooks, teach logs and profiles land in local files.
		[]string{"teach", "--query", "find devices " + tok, "--resource", bbAgentID, "--resource-type", "agent",
			"--notes", "rotated " + tok, "--playbook-notes", "use " + tok, "--no-validate"},
		[]string{"teach", "--query", "find " + tok, "--resource", tok, "--resource-type", "agent"}, // a held resource id: refused, logged to teach.log
		[]string{"teach-playbook", "--query", "devices with " + tok, "--notes", "use " + tok},
		[]string{"profile", "save", "held", "--description", "uses " + tok, "--select", "id"},
		[]string{"profile", "show", "held"},
		[]string{"profile", "list"},
		[]string{"feedback", "list"},
		[]string{"learnings", "list", "--json"},
		[]string{"recall", "--query", "find devices " + tok},
		[]string{"doctor"},
		[]string{"auth", "status"},
		[]string{"agent-context"},
	)...), cliCall{stdin: associationBody(tok), args: []string{"custom-driver", "agent", "create-custom-driver-association",
		bbDriverID, bbAgentID, bbDeviceID, "--stdin", "--dry-run"}})
}

func read(endpoint string, params map[string]any) map[string]any {
	return map[string]any{"name": "domotz_read", "arguments": map[string]any{"endpoint_id": endpoint, "params": params}}
}

func execute(endpoint string, params map[string]any) map[string]any {
	return map[string]any{"name": "domotz_execute", "arguments": map[string]any{"endpoint_id": endpoint, "params": params}}
}

func tool(name string, args map[string]any) map[string]any {
	return map[string]any{"name": name, "arguments": args}
}

// onDevice is the params of a call on bbDeviceID.
func onDevice(extra map[string]any) map[string]any {
	p := map[string]any{"agent_id": bbAgentID, "device_id": bbDeviceID}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

// mcpReads are read-only tools across every response shape the fake has, and
// every novel tool. The collector variables (the first half of a split
// credential, page 0 of two) run in their own session
// (TestMCPSplitAcrossPagesWithheld): a delivered half makes the history check
// withhold later results of the session whole (the P2
// TestMCPSessionHalfWithholdsLaterResultsKnownP2), which would hide every
// preview this battery checks.
func mcpReads() []map[string]any {
	none := map[string]any{}
	agentOnly := map[string]any{"agent_id": bbAgentID}
	return []map[string]any{
		tool("domotz_search", map[string]any{"query": "device snmp authentication credentials"}),
		tool("domotz_get", map[string]any{"endpoint_id": "agent.device.get-snmpauthentication"}),
		read("agent.list", none),
		read("agent.get", agentOnly),
		read("agent.get", map[string]any{"agent_id": bbDenied}),
		read("agent.device.list", agentOnly),
		read("agent.device.get", onDevice(nil)),
		read("agent.device.get-snmpauthentication", onDevice(nil)),
		read("custom-driver.list-associations", agentOnly),
		read("custom-driver.list", none),
		read("custom-driver.get", map[string]any{"custom_driver_id": bbDriverID}),
		read("agent.activity-log.get-agent", agentOnly),
		read("agent.device.get-inventory", onDevice(nil)),
		read("agent.connection.get-agent-vpnactive", agentOnly),
		read("rbac.get-users", none),
		read("inventory.get", none),
		read("custom-tag.get", none),
		read("area.team.list", map[string]any{"area_id": bbAreaID}),
		read("agent.history.get-speed-test", agentOnly),
		read("agent.network-topology.get", agentOnly),
		tool("context", none),
		tool("fleet", none),
		tool("fleet_health", none),
		tool("fleet_agents", none),
		tool("fleet_offline", none),
		tool("fleet_new", map[string]any{"since": "24h"}),
		tool("fleet_inventory", none),
		tool("fleet_breakdown", none),
		tool("fleet_alerts", none),
		tool("fleet_unmonitored", none),
		tool("fleet_stale", none),
		tool("fleet_ip_conflicts", none),
		tool("fleet_speedtest", none),
		tool("fleet_triggers", none),
		tool("fleet_events", none),
		tool("topology", map[string]any{"agent-id": bbAgentID}),
		tool("drift", map[string]any{"agent-id": bbAgentID}),
		tool("analytics", map[string]any{"type": "agent"}),
		tool("tail", map[string]any{"resource": "agent", "follow": false}),
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
// answer with a preview and send nothing. A domotz_execute call previews as a
// confirmation_required object; the mirrored CLI write (import) previews as
// the command's --dry-run output followed by the gate's "Preview only:
// nothing was sent" note. The credential-carrying endpoints (device
// credentials, SNMP v3 keys, a team leader's password, a custom driver's
// credentials) carry an argument value no other source holds, which their
// previews mask (the credential kit holds it).
func mcpWrites() []map[string]any {
	return []map[string]any{
		tool("import", map[string]any{"resource": "custom-tag"}),
		execute("agent.device.set-credentials", onDevice(map[string]any{"username": "hq-admin", "password": bbMCPArgSec})),
		execute("agent.device.set-snmpauthentication", onDevice(map[string]any{"version": "V3_AUTH_PRIV", "username": "hq-snmp",
			"authentication_protocol": "SHA", "authentication_key": bbMCPArgSec, "encryption_protocol": "AES", "encryption_key": bbMCPArgSec})),
		execute("agent.device.set-snmp-community", onDevice(map[string]any{"read": bbMCPArgSec})),
		execute("area.team.create", map[string]any{"area_id": bbAreaID, "name": "Preview team",
			"leader": map[string]any{"name": "Preview lead", "password": bbMCPArgSec, "details": map[string]any{"display_name": "Lead"}}}),
		execute("custom-driver.agent.create-custom-driver-association", map[string]any{"custom_driver_id": bbDriverID, "agent_id": bbAgentID,
			"device_id": bbDeviceID, "credentials": map[string]any{"username": "hq-driver", "password": bbMCPArgSec}}),
		execute("agent.device.delete", onDevice(nil)),
		execute("agent.device.edit", onDevice(map[string]any{"field": "importance", "value": "VITAL"})),
		execute("agent.device.hide", onDevice(nil)),
		execute("agent.device.delete-down", map[string]any{"agent_id": bbAgentID}),
		execute("agent.device.power-action-on", onDevice(map[string]any{"field": "power_cycle"})),
		execute("agent.device.connect-to", onDevice(map[string]any{"port": 443, "protocol": "https"})),
		execute("agent.delete", map[string]any{"agent_id": bbAgentID}),
		execute("agent.ownership.move-agent", map[string]any{"agent_id": bbAgentID, "team_id": bbTeamID}),
		execute("agent.connection.create-agent-vpnconnection", map[string]any{"agent_id": bbAgentID}),
		execute("agent.connection.delete-agent-vpnconnection", map[string]any{"agent_id": bbAgentID, "vpn_session_id": bbVPNSession}),
		execute("custom-tag.create", map[string]any{"name": "Preview tag", "color": "red"}),
		execute("custom-tag.edit", map[string]any{"custom_tag_id": bbTagID, "name": "Preview"}),
		execute("custom-tag.delete", map[string]any{"custom_tag_id": bbTagID}),
		execute("rbac.create-user", map[string]any{"email": "preview@hq-record.example.com", "name": "Preview"}),
		execute("rbac.delete-user", map[string]any{"member_id": bbMemberID}),
		execute("inventory.delete", map[string]any{}),
		execute("custom-driver.re-enable-associations", map[string]any{}),
	}
}

// mcpRefusedReads ask domotz_read for a write: each must be refused with an
// error and send nothing.
func mcpRefusedReads() []map[string]any {
	return []map[string]any{
		read("agent.device.delete", onDevice(nil)),
		read("custom-tag.create", map[string]any{"name": "Refused tag"}),
		read("agent.device.set-credentials", onDevice(map[string]any{"username": "hq-admin", "password": bbMCPArgSec})),
	}
}

// mcpLocalWrites are mirrored tools that write only local files (no gate by
// design): their child CLI output passes the server's chokepoint too.
func mcpLocalWrites() []map[string]any {
	return []map[string]any{
		tool("export", map[string]any{"resource": "custom-driver", "format": "json"}),
		tool("sync", map[string]any{"resources": "agent,custom-driver"}),
	}
}

func mcpBattery() []map[string]any {
	return append(append(append(mcpReads(), mcpWrites()...), mcpRefusedReads()...), mcpLocalWrites()...)
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

// publicField matches name and value in JSON output, raw or inside a JSON
// string (an MCP result escapes its quotes).
func publicField(name, value string) *regexp.Regexp {
	return regexp.MustCompile(`\\*"` + regexp.QuoteMeta(name) + `\\*":\s*\\*"?` + regexp.QuoteMeta(value))
}

// domotz-credential-names-exact: Domotz product data under names the
// credential rule would read as credentials prints as the API returned it,
// from the CLI and MCP alike, and the store keeps it (fleet unmonitored reads
// authentication_status): an inventory field's key, an activity log's user,
// a VPN session id, a device's authentication_status, a user's
// authentication_type and required_authentication_type, the SNMP
// authentication_protocol, a custom driver's requires_credentials.
func TestDomotzPublicFieldsAreNotWithheld(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	var cli strings.Builder
	for _, args := range [][]string{
		{"agent", "device", "get", bbAgentID, bbDeviceID, "--json"},
		{"agent", "device", "get-snmpauthentication", bbAgentID, bbDeviceID, "--json"},
		{"agent", "device", "get-inventory", bbAgentID, bbDeviceID, "--json"},
		{"inventory", "get", "--json"},
		{"agent", "activity-log", "get-agent", bbAgentID, "--json"},
		{"agent", "connection", "get-agent-vpnactive", bbAgentID, "--json"},
		{"agent", "connection", "delete-agent-vpnconnection", bbAgentID, bbVPNSession, "--dry-run"},
		{"rbac", "get-users", "--json"},
		{"custom-driver", "list", "--json"},
		{"rbac", "create-user", "--email", "tech@hq-record.example.com", "--name", "HQ", "--user-group-ids", "[3]", "--required-authentication-type", bbReqAuthType, "--dry-run"},
	} {
		cli.WriteString(e.run(t, args...))
	}
	mcp := e.mcpCalls(t, []map[string]any{
		read("agent.device.get", onDevice(nil)),
		read("agent.device.get-snmpauthentication", onDevice(nil)),
		read("agent.device.get-inventory", onDevice(nil)),
		read("agent.activity-log.get-agent", map[string]any{"agent_id": bbAgentID}),
		read("agent.connection.get-agent-vpnactive", map[string]any{"agent_id": bbAgentID}),
		execute("agent.connection.delete-agent-vpnconnection", map[string]any{"agent_id": bbAgentID, "vpn_session_id": bbVPNSession}),
		read("rbac.get-users", map[string]any{}),
		read("custom-driver.list", map[string]any{}),
	})
	for where, got := range map[string]string{"CLI": cli.String(), "MCP": mcp} {
		for _, want := range []*regexp.Regexp{
			publicField("authentication_status", "REQUIRED"),
			publicField("authentication_protocol", "SHA-256"),
			publicField("key", bbInvKey),
			publicField("user", bbLogUser),
			publicField("authentication_type", bbAuthType),
			publicField("required_authentication_type", bbReqAuthType),
			publicField("requires_credentials", "true"),
			regexp.MustCompile(`/vpn-session/` + bbVPNSession),
		} {
			if !want.MatchString(got) {
				t.Errorf("%s withheld product data (want %s):\n%.3000s", where, want, got)
			}
		}
		assertNoEcho(t, where, got)
	}
	// The store keeps it too.
	e.run(t, "sync", "--resources", "agent", "--json")
	var stored bool
	scanFiles(e.home, func(rel string, b []byte) {
		stored = stored || (isStoreFile(rel) && publicField("authentication_status", "REQUIRED").Match(b))
	})
	if !stored {
		t.Errorf("the local store dropped the devices' authentication_status")
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before it is followed, from the CLI, sync and MCP alike.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	redirect := e.run(t, "agent", "device", "get", bbAgentID, bbRedirect, "--json")
	sv.mu.Lock()
	sv.redirectAgents = true
	sv.mu.Unlock()
	syncOut := e.run(t, "sync", "--resources", "agent", "--json")
	e.mcpCalls(t, []map[string]any{
		read("agent.device.get", map[string]any{"agent_id": bbAgentID, "device_id": bbRedirect}),
		read("agent.list", map[string]any{}),
	})
	sv.mu.Lock()
	n := sv.offHost
	var redirected int
	for _, m := range sv.methods {
		if strings.HasSuffix(m, "/public-api/v1/agent/"+bbAgentID+"/device/"+bbRedirect) || strings.HasSuffix(m, "GET /public-api/v1/agent") {
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
	for where, out := range map[string]string{"agent device get": redirect, "sync": syncOut} {
		if !strings.Contains(out, "leave the configured Domotz host") {
			t.Fatalf("%s: want the pin refusal, got:\n%.1500s", where, out)
		}
	}
}

// Class credential-sent-to-another-host, region form (region-origin-pinned):
// with DOMOTZ_BASE_URL unset the API host is
// https://api-{region}.domotz.com, the region from DOMOTZ_REGION (default
// us-east-1-cell-1). A region that is not one DNS label (it would move the
// host: "evil.example.com/x?", "x.attacker") and a region a call supplies
// later (sync --path-context region=..., the MCP sync tool's argument) are
// refused before any dial. A proxy trap stands in for the network: the
// transport sends every dial through HTTPS_PROXY, so a request that got
// past the pin would reach the trap, never a real host.
func TestRegionNeverMovesTheKey(t *testing.T) {
	t.Parallel()
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
	base := cleanEnv(home, "DOMOTZ_API_KEY="+bbToken, "DOMOTZ_CLI_PATH="+bbCLI, "DOMOTZ_MCP_HTTP_TOKEN="+bbHTTPToken,
		"HTTPS_PROXY="+trap.URL, "HTTP_PROXY="+trap.URL, "NO_PROXY=", "no_proxy=")
	var out strings.Builder
	for _, region := range []string{"evil.example.com/x?", "x.attacker", "a@b", "evil.example#"} {
		env := append(append([]string{}, base...), "DOMOTZ_REGION="+region)
		got := runWith(t, env, "agent", "list", "--json")
		if !strings.Contains(got, "leave the configured Domotz host") {
			t.Errorf("DOMOTZ_REGION=%q: want the pin refusal, got:\n%.1000s", region, got)
		}
		out.WriteString(got)
	}
	for _, region := range []string{"evil", "evil.example.com/x?"} {
		got := runWith(t, base, "sync", "--resources", "agent", "--path-context", "region="+region, "--json")
		if !strings.Contains(got, "leave the configured Domotz host") {
			t.Errorf("sync --path-context region=%q: want the pin refusal, got:\n%.1000s", region, got)
		}
		out.WriteString(got)
	}
	e := &bbEnv{home: home, env: base}
	mcp := e.mcpPhases(t,
		[]map[string]any{tool("sync", map[string]any{"resources": "agent", "path-context": []any{"region=evil"}})},
		[]map[string]any{tool("sync", map[string]any{"resources": "agent", "path-context": "region=evil.example.com/x?"})},
	)
	if strings.Count(mcp, "leave the configured Domotz host") < 2 {
		t.Errorf("the MCP sync tool with a region argument: want the pin refusal twice, got:\n%.2000s", mcp)
	}
	out.WriteString(mcp)
	mu.Lock()
	defer mu.Unlock()
	if len(dials) != 0 {
		t.Errorf("a request got past the pin and dialed out (through the proxy trap): %v", dials)
	}
	assertNoEcho(t, "region refusals", out.String())
}

// mcp-endpoint-write-confirm and mcp-mirror-write-confirm: every request the
// MCP server and its mirrored CLI children send to the API is a read (a GET)
// while no call sets confirm, though the battery calls every kind of write
// (domotz_execute and the mirrored import); each write answers with a
// preview, and domotz_read refuses a write outright.
func TestMCPSendsOnlyReadsWithoutConfirm(t *testing.T) {
	t.Parallel()
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
	for i, call := range mcpRefusedReads() {
		id := len(mcpReads()) + len(mcpWrites()) + i + 1
		if r := replies[id]; !strings.Contains(r, "changes state") {
			t.Errorf("domotz_read %v was not refused:\n%.1500s", call["arguments"], r)
		}
	}
	// The novels answered from the synced store or the live fan-out (a
	// refusal would leave the leak checks vacuous for them).
	for i, call := range mcpReads() {
		want := map[any]string{"fleet_health": "HQ-Record Branch", "fleet_agents": "HQ-Record Branch"}[call["name"]]
		if args, _ := call["arguments"].(map[string]any); call["name"] == "domotz_read" {
			want = map[any]string{"agent.list": "HQ-Record Branch", "agent.device.list": "HQ-Record printer",
				"rbac.get-users": "HQ-Record tech", "custom-driver.list": "HQ-Record UPS driver"}[args["endpoint_id"]]
		}
		if r := replies[i+1]; want != "" && !strings.Contains(r, want) {
			t.Errorf("%s %v read nothing (want %q):\n%.1500s", call["name"], call["arguments"], want, r)
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
// and the number 1 still preview. Both gates: domotz_execute (a device
// delete, a tag create) and the mirrored write (import). The mirror blocks
// import's required --input (a file the server would read for the caller),
// so a mirrored import can never send a record: its control is that confirm
// true runs the child as given (its own answer, without --dry-run or the
// gate's preview note) while every other value previews.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	write := func(confirm any) []map[string]any {
		del := execute("agent.device.delete", onDevice(nil))
		del["arguments"].(map[string]any)["confirm"] = confirm
		create := execute("custom-tag.create", map[string]any{"name": "Confirmed tag", "color": "red"})
		create["arguments"].(map[string]any)["confirm"] = confirm
		return []map[string]any{del, create, tool("import", map[string]any{"resource": "custom-tag", "confirm": confirm})}
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
	var del, create bool
	for _, m := range sv.methods {
		del = del || m == "DELETE /public-api/v1/agent/"+bbAgentID+"/device/"+bbDeviceID
		create = create || m == "POST /public-api/v1/custom-tag"
	}
	if !del || !create {
		t.Errorf("confirm true did not send the device delete and the tag create; requests: %v", sv.methods)
	}
	if r := mcpReplies(out)[3]; r == "" || strings.Contains(r, "Preview only") || strings.Contains(r, "dry run") || !strings.Contains(r, "input") {
		t.Errorf("confirm true did not run the mirrored import as given:\n%.1500s", r)
	}
}

// dry-run-sends-nothing: a --dry-run write never reaches the API, from a cold
// start with the key in the environment, the credential store or a config
// file (api_key, or access_token alone: nothing to send either way).
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	// The key's source does not change what --dry-run sends: one environment
	// key and one config-file key.
	for _, mode := range []credMode{credEnv, credConfigFile} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			imp := e.importFile(t, "dry")
			for _, c := range append(calls(
				[]string{"custom-tag", "create", "--name", "Dry", "--color", "red", "--dry-run"},
				[]string{"custom-tag", "edit", bbTagID, "--name", "Dry", "--dry-run"},
				[]string{"custom-tag", "delete", bbTagID, "--yes", "--dry-run"},
				[]string{"agent", "delete", bbAgentID, "--dry-run"},
				[]string{"agent", "device", "delete", bbAgentID, bbDeviceID, "--dry-run"},
				[]string{"agent", "device", "delete-down", bbAgentID, "--dry-run"},
				[]string{"agent", "device", "hide", bbAgentID, bbDeviceID, "--dry-run"},
				[]string{"agent", "device", "edit", bbAgentID, bbDeviceID, "--field", "importance", "--dry-run"},
				[]string{"agent", "device", "set-credentials", bbAgentID, bbDeviceID, "--username", "hq-cold-admin", "--password", "Cold-Pass-7c1", "--dry-run"},
				[]string{"agent", "device", "set-snmpauthentication", bbAgentID, bbDeviceID, "--version", "V2", "--snmp-read-community", "cold-community-7", "--dry-run"},
				// Not "...community...": a held community sharing a word with
				// the path (snmp-community) withholds the preview (fail closed).
				[]string{"agent", "device", "set-snmp-community", bbAgentID, bbDeviceID, "--read", "Cold-Ro-7c8", "--dry-run"},
				[]string{"agent", "connection", "create-agent-vpnconnection", bbAgentID, "--allowed-ip", "203.0.113.9", "--routing-policy", "global", "--dry-run"},
				[]string{"agent", "connection", "delete-agent-vpnconnection", bbAgentID, bbVPNSession, "--dry-run"},
				[]string{"area", "team", "create", bbAreaID, "--name", "Dry", "--leader-name", "Dry", "--leader-password", "Cold-Pass-7c2",
					"--leader-details-display-name", "Dry", "--dry-run"},
				[]string{"custom-driver", "agent", "create-custom-driver-association", bbDriverID, bbAgentID, bbDeviceID,
					"--credentials-username", "hq-cold-driver", "--credentials-password", "Cold-Pass-7c3", "--dry-run"},
				[]string{"rbac", "create-user", "--email", "dry@hq-record.example.com", "--name", "Dry", "--user-group-ids", "[3]", "--dry-run"},
				[]string{"rbac", "delete-user", bbMemberID, "--dry-run"},
				[]string{"inventory", "delete", "--dry-run"},
				[]string{"import", "custom-tag", "--input", imp, "--dry-run"},
				[]string{"agent", "list", "--dry-run"},
			), cliCall{stdin: associationBody("Cold-Pass-7c4"), args: []string{"custom-driver", "agent", "create-custom-driver-association",
				bbDriverID, bbAgentID, bbDeviceID, "--stdin", "--dry-run"}}) {
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
			e.run(t, "custom-tag", "edit", bbTagID, "--name", "Dry Co")
			sv.mu.Lock()
			if len(sv.methods) != 1 || sv.methods[0] != "PUT /public-api/v1/custom-tag/"+bbTagID {
				t.Errorf("control (mode %s): want the one PUT, got %v", mode, sv.methods)
			}
			sv.mu.Unlock()
		})
	}
}

// credential-split-across-output-records over MCP: the fake splits the first
// credential it was sent across the two pages of a collector's variables,
// one half each. One session reads both pages; the halves must never both
// reach the client.
func TestMCPSplitAcrossPagesWithheld(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	sv.splitGroups = true
	e := newEnv(t, sv, credEnv)
	out := e.mcpPhases(t,
		[]map[string]any{read("agent.variable.list-agent", map[string]any{"agent_id": bbAgentID})},
		[]map[string]any{read("agent.variable.list-agent", map[string]any{"agent_id": bbAgentID, "page_number": 1})})
	if !strings.Contains(out, `"id":2`) {
		t.Fatalf("MCP session did not answer both pages:\n%.2000s", out)
	}
	sv.mu.Lock()
	page1 := sv.pages["/agent/"+bbAgentID+"/variable?page_number=1"]
	sv.mu.Unlock()
	if page1 == 0 {
		t.Fatalf("the second page was never asked for; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
}
