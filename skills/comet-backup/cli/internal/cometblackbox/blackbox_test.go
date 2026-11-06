// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package cometblackbox runs the built CLI and MCP binaries against a Comet
// Server that echoes every credential it receives back in every encoding it
// can, whole and split across lines and adjacent values, and checks what a
// reviewer cannot read off the code (comet-backup handfixes output-chokepoint,
// store-never-keeps-secrets, no-response-cache, response-credentials-held,
// api-origin-pinned, credential-tier-read-confirm):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the admin password and session key (sent in every
//     form body), the MCP HTTP token, a new or reset account password given as
//     a flag or on stdin, a profile's destination keys given as --profile-data,
//     and the password hash, TOTP key, destination keys, bucket key and
//     organization storage password the server returns;
//   - no file the program writes holds one either;
//   - no credential leaves for a host other than the configured one: not by a
//     redirect, and not in what the operator's own sinks (feedback --send,
//     --deliver webhook:) are sent.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package cometblackbox

import (
	"bytes"
	"compress/zlib"
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

const (
	bbPassword   = "CometAdminPassw0rd-5c1e9a7d3b"       // COMET_BACKUP_PASSWORD
	bbSessionKey = "cometSessionKey-71c3e9b05d2a4f68"    // COMET_BACKUP_SESSION_KEY
	bbHTTPToken  = "mcpHttpToken-0123456789abcdefABCDEF" // COMET_BACKUP_MCP_HTTP_TOKEN
	bbTargetPass = "NewAcctPassw0rd-2e8a4c6f1b9d"        // users add --target-password
	bbShortPass  = "Ab1c2D"                              // a 6-character --new-password
	bbDashPass   = "-dashPass-91c3e7a5"                  // a --target-password value that starts with "-"
	bbOldPass    = "OldAcctPass-8e2d4f6a1c"              // users reset-password --old-password
	bbStdinPass  = "StdinPass-4f2a9c1e"                  // TargetPassword in a --stdin request body
	bbProfileS3  = "profileS3Val-4d8b2e6f1a9c3e70"       // users set-profile --profile-data S3SecretKey
	bbPwHash     = "pwHashResp-9a7c5e3b1d2f4a6c"         // UserProfileConfig.PasswordHash
	bbTOTPKey    = "totpKeyResp-test-totp-key-one"       // UserProfileConfig.TOTPKey
	bbS3Secret   = "rS3vQ7Resp-test-s3-value-one"        // DestinationConfig.S3SecretKey
	bbSFTPPass   = "sfvPwResp-7b2d9e4f1a6c"              // DestinationConfig.SFTP.SFTPPassword
	bbRWKey      = "bucketRWKey-test-rw-key-one"         // BucketProperties.ReadWriteKey
	bbOrgPass    = "orgRemotePass-3h8v5n2b6c"            // Organization RemoteStorage Password
	bbShortResp  = "Sh0rtPw"                             // a 7-character credential a response carries
	bbBOMResp    = "BomRespSecret-6a1c"                  // a credential in a BOM-prefixed response
	bbTinyResp   = "Qz7"                                 // a 3-character SFTPPassword
	bbCleanSFTP  = "cleanSftpPw-5e2b8d1f4a"              // that SFTPPassword in the clean profile
	bbEightResp  = "Ei8htPw!"                            // an 8-character FTPPassword
	bbNineResp   = "Nine9Pw#x"                           // a 9-character LocalcopyWinSMBPassword
	bbMaskResp   = "[hidden]"                            // an S3SecretKey that reads like a mask
	// Adversarial r1 (publish): credentials under operator-chosen names, the
	// header maps and custom body of webhooks, PSA integrations and custom
	// remote storage (WebhookOption, PSAConfig, CustomRemoteBucketSettings).
	bbWebhookAuth = "whkSentinel-whkvalue"     // WebhookOptions[].CustomHeaders.Authorization (Bearer)
	bbPSABasic    = "cHNhU0VOVElORUxQU0EtOTkx" // PSAConfigs[].CustomHeaders.Authorization (Basic)
	bbCustomBody  = "cbodySentinel-4411x"      // RemoteStorage[].Custom.CustomBody
	bbHeaderX     = "xhdrSentinel-90q2"        // a custom header under an arbitrary key
)

func secrets() []string {
	return []string{bbPassword, bbSessionKey, bbHTTPToken, bbTargetPass, bbShortPass, bbDashPass, bbOldPass, bbStdinPass,
		bbProfileS3, bbPwHash, bbTOTPKey, bbS3Secret, bbSFTPPass, bbRWKey, bbOrgPass, bbShortResp, bbBOMResp,
		bbTinyResp, bbCleanSFTP, bbEightResp, bbNineResp, bbMaskResp, bbWebhookAuth, bbPSABasic, bbCustomBody, bbHeaderX}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "comet-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "comet-backup-cli"+exe)
	bbMCP = filepath.Join(dir, "comet-backup-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/comet-backup-cli", bbMCP: "../../cmd/comet-backup-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// echoForms is every encoding of s the server sends back whole.
func echoForms(s string) []string {
	var a85, a85Off bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	w = ascii85.NewEncoder(&a85Off)
	_, _ = w.Write([]byte("X" + s))
	_ = w.Close()
	return []string{
		s,
		"x" + s + "x",
		"Bearer " + s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
		// Inside a longer encoded stream, off the group boundaries.
		base32.StdEncoding.EncodeToString([]byte("X" + s + "Y")),
		base64.StdEncoding.EncodeToString([]byte("X" + s + "YZ")),
		a85Off.String(),
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

// sentSecrets is every known secret the request carried (its headers, a
// decoded Basic credential, the URL or the body): a real API can echo only
// what it was sent.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	seen := fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
	if u, p, ok := r.BasicAuth(); ok {
		seen += "\n" + u + "\n" + p
	}
	var out []string
	for _, s := range secrets() {
		if strings.Contains(seen, s) {
			out = append(out, s)
		}
	}
	return out
}

type server struct {
	extra   string // repeated in the default answer
	refuse  string // when set, the default answer is a refusal repeating it
	url     string
	ca      string // the PEM of the TLS test server's certificate
	clean   bool   // repeat nothing the request sent (records carry only their credential fields)
	sinkURL string // the operator's own feedback endpoint and webhook
	mu      sync.Mutex
	offHost int
	sunk    []string       // bodies the sink received
	hits    map[string]int // requests per /api/v1/admin/ method
	// fixedNow, when set, is the server's clock (Unix seconds), so a test can
	// replay the exact records a failing run printed.
	fixedNow int64
}

func (sv *server) unix() int64 {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.fixedNow != 0 {
		return sv.fixedNow
	}
	return time.Now().Unix()
}

// profile is one tenant's UserProfileConfig (the shape list-users-full and
// get-user-profile return), carrying every credential Comet keeps in it.
// The failed item's Description repeats what the request sent, so the fleet
// commands' own rows carry the echo too.
//
// A credential of three characters is held with no length floor, so some
// decoded view of ordinary output carries it by chance (about one clock in
// 300 here) and the kit withholds the records, as its contract says. That is
// right for a value that short, and wrong for a test that the records print:
// so the clean profile, what a real Comet Server answers, carries a
// credential of ordinary length there, and the 3-character one stays in the
// echo profile, where the tests check that nothing leaks.
func profile(sent []string, pwHash string, now int64) string {
	tiny := bbTinyResp
	if len(sent) == 0 {
		tiny = bbCleanSFTP
	}
	daily := `[{"FrequencyType":8011,"SecondsPast":3600}]`
	return fmt.Sprintf(`{"Username":"acme","AccountName":"Acme Corp","PasswordHash":%q,"TOTPKey":%q,"MaximumDevices":4,
"Devices":{"dev-a":{"FriendlyName":"ACME-DC01","ClientVersion":"26.8.7","RegistrationTime":%d},"dev-b":{"FriendlyName":"ACME-LAPTOP","ClientVersion":"26.5.0"}},
"Destinations":{"vault-1":{"Description":"Acme S3","DestinationType":1000,"S3SecretKey":%q,"SFTP":{"SFTPUsername":"acme","SFTPPassword":%q},"StorageLimitEnabled":true,"StorageLimitBytes":2000},
"vault-2":{"Description":"Acme FTP","DestinationType":1003,"S3SecretKey":%q,"FTP":{"FTPUsername":"acme","FTPPassword":%q},"SFTP":{"SFTPUsername":"acme","SFTPPassword":%q},"LocalcopyWinSMBPassword":%q}},
"Sources":{"item-ok":{"Engine":"engine1/file","Description":"Files","OwnerDevice":"dev-a","Statistics":{"LastBackupJob":{"GUID":"j1","Status":5000,"StartTime":%d,"EndTime":%d},"LastSuccessfulBackupJob":{"GUID":"j1","Status":5000,"StartTime":%d,"EndTime":%d}}},
"item-failed":{"Engine":"engine1/mssql","Description":%q,"OwnerDevice":"dev-a","Statistics":{"LastBackupJob":{"GUID":"j2","Status":7002,"StartTime":%d,"EndTime":%d},"LastSuccessfulBackupJob":{"GUID":"j0","Status":5000,"EndTime":%d}}}},
"BackupRules":{"r1":{"Source":"item-ok","Destination":"vault-1","Schedules":%s},"r2":{"Source":"item-failed","Destination":"vault-1","Schedules":%s}},
"echo":%s}`,
		pwHash, bbTOTPKey, now-400*86400, bbS3Secret, bbSFTPPass,
		bbMaskResp, bbEightResp, tiny, bbNineResp,
		now-3*3600, now-2*3600, now-3*3600, now-2*3600,
		"SQL "+strings.Join(sent, " "), now-5*3600, now-4*3600, now-40*3600,
		daily, daily, profileEcho(sent, pwHash))
}

// profileEcho repeats the profile's own credentials in sibling fields, unless
// the request sent nothing to repeat (cleanServer: the credentials stay under
// their own names).
func profileEcho(sent []string, pwHash string) string {
	if len(sent) == 0 {
		return `{"note":"none"}`
	}
	return echoObject(append(sent, pwHash, bbS3Secret, bbSFTPPass, bbMaskResp, bbEightResp, bbTinyResp, bbNineResp))
}

// tlsCAFile writes srv's certificate as a PEM trust bundle: the kit refuses to
// send a credential over plain http, so every Comet stub here serves https.
func tlsCAFile(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// cleanServer answers what a real Comet Server answers: credentials under their
// own field names and nothing the request sent repeated. The records print.
func cleanServer(t *testing.T) *server { return startServer(t, true) }

// echoServer repeats every credential the request sent in every encoding.
func echoServer(t *testing.T) *server { return startServer(t, false) }

func startServer(t *testing.T, clean bool) *server {
	t.Helper()
	sv := &server{hits: map[string]int{}, clean: clean}
	// A second host the server redirects to: a credential must never reach it.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(other.Close)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sv.mu.Lock()
		sv.sunk = append(sv.sunk, string(b))
		sv.mu.Unlock()
	}))
	t.Cleanup(sink.Close)
	sv.sinkURL = sink.URL
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		form, _ := url.ParseQuery(string(raw))
		p := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/")
		sv.mu.Lock()
		sv.hits[p]++
		sv.mu.Unlock()
		sent := sentSecrets(r)
		if sv.clean {
			sent = nil
		}
		echo := echoObject(sent)
		joined := strings.Join(sent, " ")
		now := sv.unix()
		switch p {
		case "list-users-full":
			fmt.Fprintf(w, `{"acme":%s}`, profile(sent, bbPwHash, now))
		case "get-user-profile":
			if form.Get("TargetUser") == "shorty" {
				fmt.Fprint(w, profile(sent, bbShortResp, now))
				return
			}
			fmt.Fprint(w, profile(sent, bbPwHash, now))
		case "list-users":
			fmt.Fprint(w, `["acme"]`)
		case "storage/list-buckets":
			// Deflate-encoded, as a server behind a compressing proxy answers.
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			fmt.Fprintf(zw, `{"acme-bucket":{"OrganizationID":"","ReadWriteKey":%q,"CreateTime":%d,"echo":%s}}`, bbRWKey, now, repeatUnlessClean(sv, sent, bbRWKey))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(z.Bytes())
		case "storage/bucket-properties":
			fmt.Fprintf(w, "\xef\xbb\xbf"+`{"OrganizationID":"","ReadWriteKey":%q,"CreateTime":%d}`, bbBOMResp, now)
		case "organization/list":
			fmt.Fprintf(w, `{"org-1":{"Name":"Acme MSP","RemoteStorage":[{"Type":"comet","Description":"Upstream","RemoteAddress":"https://upstream.example.com/","Username":"acme","Password":%q,"Custom":{"URL":"https://bucket.example.com","CustomBody":"token=%s","CustomHeaders":{"X-Vendor-Auth":%q}}}],"WebhookOptions":{"hook-1":{"URL":"https://hooks.example.com/x","CustomHeaders":{"Authorization":"Bearer %s"}}},"PSAConfigs":[{"URL":"https://psa.example.com","CustomHeaders":{"Authorization":"Basic %s"}}],"echo":%s}}`, bbOrgPass, bbCustomBody, bbHeaderX, bbWebhookAuth, bbPSABasic, repeatUnlessClean(sv, sent, bbOrgPass))
		case "dispatcher/list-active":
			fmt.Fprint(w, `{"conn-1":{"Username":"acme","DeviceID":"dev-a","ReportedVersion":"26.8.7"}}`)
		case "meta/version":
			fmt.Fprint(w, `{"Version":"26.8.7"}`)
		case "get-jobs-for-date-range", "get-jobs-recent":
			fmt.Fprintf(w, `[{"GUID":"j2","Username":"acme","DeviceID":"dev-a","SourceGUID":"item-failed","DestinationGUID":"vault-1","Status":7002,"StartTime":%d,"EndTime":%d,"echo":%s}]`, now-5*3600, now-4*3600, echo)
		case "get-job-log-entries":
			fmt.Fprintf(w, `[{"Time":%d,"Severity":"E","Message":"login refused for %s"}]`, now-4*3600, joined)
		case "policies/list":
			// A failure envelope on HTTP 200 whose message repeats what it was sent.
			fmt.Fprintf(w, `{"Status":403,"Message":"Invalid credentials %s"}`, joined)
		case "squota/list-all":
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		case "user-groups/list-full":
			fmt.Fprintf(w, `{"g1":{"Name":"Servers","split":%s}}`, splitObject(sent))
		default:
			sv.mu.Lock()
			extra, refuse := sv.extra, sv.refuse
			sv.mu.Unlock()
			if refuse != "" {
				fmt.Fprintf(w, `{"Status":400,"Message":%q}`, refuse)
				return
			}
			fmt.Fprintf(w, `{"Status":200,"Message":"OK %s%s","echo":%s}`, joined, extra, echo)
		}
	}))
	t.Cleanup(srv.Close)
	sv.url = srv.URL
	sv.ca = tlsCAFile(t, srv)
	return sv
}

func repeatUnlessClean(sv *server, sent []string, own string) string {
	if sv.clean {
		return `{"note":"none"}`
	}
	return echoObject(append(sent, own))
}

type bbEnv struct {
	home string
	env  []string
	sink string
}

func newEnv(t *testing.T, sv *server, withSessionKey bool) *bbEnv {
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
		"COMET_BACKUP_BASE_URL=" + sv.url, "COMET_BACKUP_USERNAME=admin", "COMET_BACKUP_PASSWORD=" + bbPassword,
		"COMET_BACKUP_CLI_PATH=" + bbCLI, "COMET_BACKUP_MCP_HTTP_TOKEN=" + bbHTTPToken,
		"COMET_BACKUP_FEEDBACK_ENDPOINT=" + sv.sinkURL,
		"COMET_BACKUP_CA_CERT=" + sv.ca, "SSL_CERT_FILE=" + sv.ca,
	}
	if withSessionKey {
		env = append(env, "COMET_BACKUP_SESSION_KEY="+bbSessionKey)
	}
	return &bbEnv{home: home, env: env, sink: sv.sinkURL}
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
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	for i, c := range calls {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": c})
	}
	cmd := exec.Command(bbMCP)
	cmd.Env = e.env
	cmd.Dir = e.home
	stdin, _ := cmd.StdinPipe()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(stdin, &in)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(out.String(), `"id":`) >= len(calls)+1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
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

// assertNoEcho fails when text holds a secret in any form the server sent,
// or both halves of one (a split the reader could rejoin).
func assertNoEcho(t *testing.T, where, text string) {
	t.Helper()
	for _, s := range secrets() {
		for _, f := range echoForms(s) {
			if strings.Contains(text, f) {
				t.Errorf("%s holds a credential as %q", where, f)
			}
		}
		// Split by whitespace: the text with whitespace removed.
		if strings.Contains(strings.Join(strings.Fields(text), ""), s) {
			t.Errorf("%s holds %q split by whitespace", where, s)
		}
		// Halves under three characters match by chance.
		if a, b := halves(s); len(s) >= 6 && strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
	}
}

func battery(e *bbEnv, deliverFile string) [][]string {
	profileData := fmt.Sprintf(`{"Username":"acme","Destinations":{"vault-1":{"S3SecretKey":%q}}}`, bbProfileS3)
	return [][]string{
		{"users", "list-full", "--json"},
		{"users", "list-full"},
		{"users", "list-full", "--csv"},
		{"users", "list-full", "--json", "--receipt"},
		{"users", "list-full", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"users", "list-full", "--json", "--deliver", "file:" + deliverFile},
		{"users", "list-full", "--json", "--deliver", "webhook:" + e.sink},
		{"users", "get", "--target-user", "acme", "--json"},
		{"users", "get", "--target-user", "acme"},
		{"users", "get", "--target-user", "shorty", "--json"},
		{"users", "list", "--json"},
		{"users", "add", "--target-user", "bob", "--target-password", bbTargetPass, "--json"},
		{"users", "add", "--target-user", "bob", "--target-password", bbTargetPass, "--yes", "--json"},
		{"users", "add", "--target-user", "bob", "--target-password=" + bbTargetPass, "--bogus-flag"},
		{"users", "add", "--target-user", "bob", "--target-password", bbDashPass, "--dry-run", "--json"},
		{"users", "add", "--target-user", "bob", "--target-password", bbShortPass, "--require-password-change", "1", "--yes", "--json"},
		{"users", "reset-password", "--target-user", "acme", "--new-password", bbShortPass, "--old-password", bbOldPass, "--dry-run", "--json"},
		{"users", "reset-password", "--target-user", "acme", "--new-password", bbShortPass, "--old-password", bbOldPass, "--yes", "--json"},
		{"users", "set-profile", "--target-user", "acme", "--profile-data", profileData, "--dry-run", "--json"},
		{"users", "set-profile", "--target-user", "acme", "--profile-data", profileData, "--yes", "--json"},
		{"storage", "buckets", "--json"},
		{"storage", "buckets"},
		{"storage", "bucket", "--bucket-id", "acme-bucket", "--json"},
		{"organizations", "--json"},
		{"organizations"},
		{"constellation", "last-report", "--json"},
		{"server", "stats", "--json"},
		{"policies", "list", "--json"},
		{"quotas", "list", "--json"},
		{"user-groups", "list-full", "--json"},
		{"user-groups", "list-full"},
		{"health", "--json"},
		{"health"},
		{"triage", "--json"},
		{"triage", "--group-by", "error"},
		{"usage", "--json"},
		{"snapshot", "--json"},
		{"snapshot", "--json"},
		{"changes", "--json"},
		{"devices", "stale", "--json"},
		{"devices", "outdated", "--json"},
		{"rerun-failed", "--json"},
		{"rerun-failed", "--yes", "--json"},
		{"feedback", "noted the password " + bbPassword + " in a ticket", "--send"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find tenants " + bbPassword, "--resource", "acme", "--resource-type", "users",
			"--notes", "rotated " + bbPassword, "--playbook-notes", "use " + bbPassword, "--no-validate"},
		{"users", "list", "--" + bbPassword},
		{"doctor"},
		{"auth", "status"},
	}
}

func mcpBattery() []map[string]any {
	profileData := fmt.Sprintf(`{"Username":"acme","Destinations":{"vault-1":{"S3SecretKey":%q}}}`, bbProfileS3)
	return []map[string]any{
		{"name": "comet-backup_read", "arguments": map[string]any{"endpoint_id": "users.list-full"}},
		{"name": "comet-backup_read", "arguments": map[string]any{"endpoint_id": "users.list-full", "confirm": true}},
		{"name": "comet-backup_read", "arguments": map[string]any{"endpoint_id": "users.get", "params": map[string]any{"TargetUser": "acme"}, "confirm": true}},
		{"name": "comet-backup_read", "arguments": map[string]any{"endpoint_id": "storage.buckets", "confirm": true}},
		{"name": "comet-backup_read", "arguments": map[string]any{"endpoint_id": "organizations.list", "confirm": true}},
		{"name": "comet-backup_read", "arguments": map[string]any{"endpoint_id": "policies.list"}},
		{"name": "comet-backup_read", "arguments": map[string]any{"endpoint_id": "user_groups.list-full"}},
		{"name": "comet-backup_execute", "arguments": map[string]any{"endpoint_id": "users.add", "params": map[string]any{"TargetUser": "bob", "TargetPassword": bbTargetPass}}},
		{"name": "comet-backup_execute", "arguments": map[string]any{"endpoint_id": "users.add", "params": map[string]any{"TargetUser": "bob", "TargetPassword": bbTargetPass}, "confirm": true}},
		{"name": "comet-backup_execute", "arguments": map[string]any{"endpoint_id": "users.set-profile", "params": map[string]any{"TargetUser": "acme", "ProfileData": profileData}, "confirm": true}},
		{"name": "health", "arguments": map[string]any{}},
		{"name": "triage", "arguments": map[string]any{}},
		{"name": "usage", "arguments": map[string]any{}},
		{"name": "devices_stale", "arguments": map[string]any{}},
		{"name": "rerun_failed", "arguments": map[string]any{}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// stdinRuns feed a request body on stdin (a parameterless call's --stdin):
// previewed and sent. Only the request hold knows the sent value.
func stdinRuns(e *bbEnv, t *testing.T) string {
	body := fmt.Sprintf(`{"Note":"stdin","TargetPassword":%q}`, bbStdinPass)
	return e.runIn(t, body, "constellation", "last-report", "--stdin", "--dry-run", "--json") +
		e.runIn(t, body, "constellation", "last-report", "--stdin", "--json")
}

// assertRan fails when a battery command never reached its handler (an
// unknown command or flag the CLI does not have), so no run is vacuous; the
// two runs that test a flag error on purpose are exempt.
func assertRan(t *testing.T, args []string, out string) {
	t.Helper()
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--bogus-flag") || strings.Contains(joined, "--"+bbPassword) {
		return
	}
	for _, bad := range []string{"unknown flag", "unknown command", "unknown shorthand", "required flag"} {
		if strings.Contains(out, bad) {
			t.Errorf("%v did not run: %.300s", args, out)
		}
	}
}

// assertNoFileEcho scans every file under home for every form of every secret.
func assertNoFileEcho(t *testing.T, home string) {
	t.Helper()
	var scanned int
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		scanned++
		rel, _ := filepath.Rel(home, path)
		assertNoEcho(t, "file "+rel, string(data))
		return nil
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
}

// Class credential-persisted-in-files: against a server that repeats every
// credential, and against one that only returns credentials under their own
// names, no file holds one. The clean run proves the writes really happen.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	for _, clean := range []bool{false, true} {
		t.Run(fmt.Sprintf("clean=%v", clean), func(t *testing.T) { t.Parallel(); noCredentialInWrittenFiles(t, clean) })
	}
}

func noCredentialInWrittenFiles(t *testing.T, clean bool) {
	{
		sv := startServer(t, clean)
		e := newEnv(t, sv, false)
		deliverFile := filepath.Join(e.home, "deliver.json")
		for _, args := range battery(e, deliverFile) {
			e.run(t, args...)
		}
		stdinRuns(e, t)
		e.mcpCalls(t, mcpBattery())
		assertNoFileEcho(t, e.home)
		if !clean {
			return
		}
		if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("Acme Corp")) {
			t.Errorf("deliver.json: want the delivered record (err %v)", err)
		}
		var sawStore, sawLearning bool
		_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".db") {
				if b, rerr := os.ReadFile(path); rerr == nil {
					sawStore = sawStore || bytes.Contains(b, []byte("ACME-DC01"))
				}
			}
			return nil
		})
		if !sawStore {
			t.Errorf("no local store holds the snapshot's device")
		}
		// A taught note without a credential lands; one with a credential is
		// withheld at the SQL driver (the battery's own teach run, above).
		e.run(t, "teach", "--query", "find tenants", "--resource", "acme", "--resource-type", "users", "--notes", "rotated-the-keys", "--no-validate")
		_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if b, rerr := os.ReadFile(path); rerr == nil {
					sawLearning = sawLearning || bytes.Contains(b, []byte("rotated-the-keys"))
				}
			}
			return nil
		})
		if !sawLearning {
			t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
		}
	}
}

// Class secret-in-output.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	for _, withSessionKey := range []bool{false, true} {
		t.Run(fmt.Sprintf("sessionKey=%v", withSessionKey), func(t *testing.T) {
			t.Parallel()
			noCredentialInOutput(t, sv, withSessionKey)
		})
	}
}

func noCredentialInOutput(t *testing.T, sv *server, withSessionKey bool) {
	{
		e := newEnv(t, sv, withSessionKey)
		deliverFile := filepath.Join(e.home, "deliver.json")
		for _, args := range battery(e, deliverFile) {
			out := e.run(t, args...)
			assertRan(t, args, out)
			assertNoEcho(t, fmt.Sprintf("output of %v (session key %v)", args, withSessionKey), out)
		}
		stdinOut := stdinRuns(e, t)
		assertRan(t, []string{"constellation", "last-report", "--stdin"}, stdinOut)
		assertNoEcho(t, "output of constellation last-report --stdin", stdinOut)
		out := e.mcpCalls(t, mcpBattery())
		if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
			t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
		}
		assertNoEcho(t, fmt.Sprintf("MCP session (session key %v)", withSessionKey), out)
	}
}

// The other direction: an ordinary answer is not withheld. Against a server that
// returns credentials only under their own names, every output path prints the
// records with those fields replaced by [redacted], and JSON stays JSON.
func TestRecordsStillPrintWithCredentialFieldsRedacted(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	// The clock of a CI run (Windows, PR #579) whose records were withheld
	// whole: a 3-character credential in the clean profile matched by chance
	// in a decoded view of its timestamp lines. Pinned so the records are
	// exactly those, every run.
	sv.mu.Lock()
	sv.fixedNow = 1794293770
	sv.mu.Unlock()
	e := newEnv(t, sv, false)
	cmd := exec.Command(bbCLI, "users", "list-full", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdoutBuf, &stderrBuf
	err := cmd.Run()
	stdout := stdoutBuf.Bytes()
	// stderr is the kit's reason when it withholds; it is checked for an echo
	// before it is printed, so a failure message never carries a credential.
	assertNoEcho(t, "users list-full --json stderr", stderrBuf.String())
	if err != nil {
		t.Fatalf("users list-full --json: %v\nstderr:\n%.1500s", err, stderrBuf.String())
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("Acme Corp")) || !bytes.Contains(stdout, []byte("[redacted]")) {
		t.Fatalf("want valid JSON with the record and [redacted], got:\n%.1500s\nstderr:\n%.1500s", stdout, stderrBuf.String())
	}
	assertNoEcho(t, "users list-full --json", string(stdout))
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"users", "list-full"}, "Acme Corp"},
		{[]string{"users", "get", "--target-user", "acme", "--json"}, "[redacted]"},
		{[]string{"organizations", "--json"}, "Acme MSP"},
		{[]string{"storage", "buckets", "--json"}, "[redacted]"},
		{[]string{"health", "--json"}, "ACME-DC01"},
		{[]string{"triage", "--json"}, "item-failed"},
	} {
		out := e.run(t, c.args...)
		assertRan(t, c.args, out)
		assertNoEcho(t, fmt.Sprintf("output of %v", c.args), out)
		if !strings.Contains(out, c.want) || strings.Contains(out, "credkit: withheld") {
			t.Errorf("%v: want %q printed and nothing withheld, got:\n%.1500s", c.args, c.want, out)
		}
	}
	out := e.mcpCalls(t, []map[string]any{
		{"name": "comet-backup_read", "arguments": map[string]any{"endpoint_id": "users.list-full", "confirm": true}},
		{"name": "health", "arguments": map[string]any{}},
	})
	assertNoEcho(t, "MCP session", out)
	if !strings.Contains(out, "Acme Corp") || !strings.Contains(out, "ACME-DC01") || strings.Contains(out, "credkit: withheld") {
		t.Fatalf("MCP results lost the records themselves:\n%.2000s", out)
	}
}

// Class credential-sent-to-another-host: a redirect off the configured Comet
// Server is refused before anything is sent there.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, false)
	out := e.run(t, "quotas", "list", "--json")
	e.mcpCalls(t, []map[string]any{{"name": "comet-backup_read", "arguments": map[string]any{"endpoint_id": "quotas.list"}}})
	sv.mu.Lock()
	n := sv.offHost
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the redirect target was contacted %d time(s)", n)
	}
	if !strings.Contains(out, "leave the configured Comet Server") && !strings.Contains(out, "credkit") {
		t.Fatalf("want the refusal, got:\n%.1500s", out)
	}
}

// Class credential-sent-to-another-host: the operator's own sinks on another
// host (feedback --send, --deliver webhook:) receive the records and none of
// the credentials; a send that carries one, in its body, URL or a header, is
// refused before it leaves.
func TestSinksOnAnotherHostGetNoCredential(t *testing.T) {
	t.Parallel()
	sv := cleanServer(t)
	e := newEnv(t, sv, false)
	for _, args := range [][]string{
		{"feedback", "the health view reads well", "--send"},
		{"users", "list-full", "--json", "--deliver", "webhook:" + e.sink},
	} {
		assertRan(t, args, e.run(t, args...))
	}
	count := func() int { sv.mu.Lock(); defer sv.mu.Unlock(); return len(sv.sunk) }
	sv.mu.Lock()
	sunk := append([]string(nil), sv.sunk...)
	sv.mu.Unlock()
	if len(sunk) != 2 || !strings.Contains(sunk[0], `"cli":"comet-backup-cli"`) || !strings.Contains(sunk[1], "Acme Corp") {
		t.Fatalf("the sinks did not receive the feedback and the records: %.1500q", sunk)
	}
	for i, b := range sunk {
		assertNoEcho(t, fmt.Sprintf("sink body %d", i), b)
	}
	e.run(t, "feedback", "noted the password "+bbPassword+" in a ticket", "--send")
	e.run(t, "users", "list-full", "--json", "--deliver", "webhook:"+e.sink+"/hook?token="+bbPassword)
	cmd := exec.Command(bbCLI, "users", "list-full", "--json", "--deliver", "webhook:"+e.sink)
	cmd.Env, cmd.Dir = append(append([]string{}, e.env...), "COMET_BACKUP_USER_AGENT="+bbPassword), e.home
	_, _ = cmd.CombinedOutput()
	if n := count(); n != 2 {
		t.Fatalf("a send carrying the credential reached the sink (%d bodies)", n)
	}
}

// Class write-safety: a write run with --dry-run (even with --yes) sends
// nothing; the same command without --dry-run reaches the server, so the
// clean dry run is not just an argument failure.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, false)
	profileData := `{"Username":"acme"}`
	for _, c := range []struct {
		method string
		args   []string
	}{
		{"add-user", []string{"users", "add", "--target-user", "bob", "--target-password", bbTargetPass}},
		{"reset-user-password", []string{"users", "reset-password", "--target-user", "acme", "--new-password", bbTargetPass}},
		{"set-user-profile", []string{"users", "set-profile", "--target-user", "acme", "--profile-data", profileData}},
		{"delete-user", []string{"users", "delete", "--target-user", "acme"}},
		{"job/cancel", []string{"backup-jobs", "cancel", "--target-user", "acme", "--job-id", "j2"}},
	} {
		count := func() int { sv.mu.Lock(); defer sv.mu.Unlock(); return sv.hits[c.method] }
		before := count()
		out := e.run(t, append(append([]string{}, c.args...), "--dry-run", "--yes", "--json")...)
		assertRan(t, c.args, out)
		if count() != before {
			t.Errorf("%v --dry-run sent %d request(s)", c.args, count()-before)
		}
		e.run(t, append(append([]string{}, c.args...), "--yes", "--json")...)
		if count() != before+1 {
			t.Errorf("%v without --dry-run did not reach the server (control)", c.args)
		}
	}
}
