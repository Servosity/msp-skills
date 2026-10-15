package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fortinet-pp-cli/internal/client"
	"fortinet-pp-cli/internal/cliutil/testenv"
	"fortinet-pp-cli/internal/config"
)

// fakeFortiGate serves canned FortiOS REST responses for one device with a
// deliberately bad posture: SSL-VPN open on WAN, super_admin without 2FA or
// trusted hosts, firmware behind, a licence expiring in 10 days.
func fakeFortiGate(t *testing.T, token string, backup *string) *httptest.Server {
	t.Helper()
	exp := time.Now().Add(10 * 24 * time.Hour).Unix()
	certExp := time.Now().Add(5 * 24 * time.Hour).Unix()
	routes := map[string]string{
		"/api/v2/monitor/system/status":                 `{"results":{"model_name":"FortiGate","model":"FGT60F","hostname":"hq"},"serial":"FGT60F0000000001","version":"v7.4.3","build":2573,"status":"success"}`,
		"/api/v2/monitor/system/firmware":               `{"results":{"current":{"version":"v7.4.3","major":7,"minor":4,"patch":3},"available":[{"version":"v7.4.7","major":7,"minor":4,"patch":7}]},"status":"success"}`,
		"/api/v2/cmdb/system/interface":                 `{"results":[{"name":"wan1","role":"wan","allowaccess":"ping https"},{"name":"lan","role":"lan","allowaccess":"ping https ssh"}],"status":"success"}`,
		"/api/v2/cmdb/system/vdom":                      `{"results":[{"name":"root"}],"status":"success"}`,
		"/api/v2/cmdb/vpn.ssl/settings":                 `{"results":{"status":"enable","port":10443,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"all"}]},"status":"success"}`,
		"/api/v2/monitor/vpn/ssl":                       `{"results":[{"index":0,"user_name":"bob","remote_host":"198.51.100.7"}],"status":"success"}`,
		"/api/v2/cmdb/system/admin":                     `{"results":[{"name":"admin","accprofile":"super_admin","two-factor":"disable","trusthost1":"0.0.0.0 0.0.0.0"}],"status":"success"}`,
		"/api/v2/cmdb/system/accprofile":                `{"results":[],"status":"success"}`,
		"/api/v2/cmdb/system/api-user":                  `{"results":[],"status":"success"}`,
		"/api/v2/monitor/license/status":                fmt.Sprintf(`{"results":{"antivirus":{"type":"downloaded_fds_object","status":"licensed","expires":%d}},"status":"success"}`, exp),
		"/api/v2/monitor/system/ha-peer":                `{"results":[{"hostname":"hq","serial_no":"FGT60F0000000001","primary":true}],"status":"success"}`,
		"/api/v2/monitor/system/ha-checksums":           `{"results":[{"serial_no":"a","checksum":{"all":"1"}},{"serial_no":"b","checksum":{"all":"2"}}],"status":"success"}`,
		"/api/v2/monitor/vpn/ipsec":                     `{"results":[{"name":"branch","proxyid":[{"p2name":"branch","status":"down"}]}],"status":"success"}`,
		"/api/v2/monitor/virtual-wan/health-check":      `{"results":{"sla":{"wan1":{"status":"up"}}},"status":"success"}`,
		"/api/v2/monitor/system/resource/usage":         `{"results":{"cpu":[{"current":12}],"mem":[{"current":40}]},"status":"success"}`,
		"/api/v2/monitor/system/available-certificates": fmt.Sprintf(`{"results":[{"name":"vpn-cert","valid_to":%d}],"status":"success"}`, certExp),
		"/api/v2/cmdb/firewall/policy":                  `{"results":[{"policyid":1,"name":"any","action":"accept","status":"enable","srcaddr":[{"name":"all"}],"dstaddr":[{"name":"all"}],"service":[{"name":"ALL"}],"logtraffic":"disable"},{"policyid":2,"name":"web","action":"accept","status":"enable","srcaddr":[{"name":"lan"}],"dstaddr":[{"name":"all"}],"service":[{"name":"HTTPS"}],"logtraffic":"all"}],"status":"success"}`,
		"/api/v2/monitor/firewall/policy":               `{"results":[{"policyid":1,"hit_count":9},{"policyid":2,"hit_count":0}],"status":"success"}`,
	}
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"http_status":401,"status":"error"}`))
			return
		}
		if r.URL.Path == "/api/v2/monitor/system/config/backup" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(*backup))
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"http_status":404,"status":"error"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

type fleetFixture struct {
	devices string
	backup  string
}

func newFleetFixture(t *testing.T) *fleetFixture {
	t.Helper()
	testenv.Isolate(t)
	f := &fleetFixture{backup: "config system global\n    set hostname \"hq\"\nend\n"}
	good := fakeFortiGate(t, "tok-a", &f.backup)
	bad := fakeFortiGate(t, "never-matches", &f.backup)
	t.Cleanup(good.Close)
	t.Cleanup(bad.Close)
	dir := t.TempDir()
	for name, tok := range map[string]string{"a.tok": "tok-a", "b.tok": "wrong"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(tok), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.devices = filepath.Join(dir, "devices.toml")
	toml := fmt.Sprintf(`[[device]]
name = "a"
url = %q
token_file = %q
insecure = true

[[device]]
name = "b"
url = %q
token_file = %q
insecure = true
`, good.URL, filepath.Join(dir, "a.tok"), bad.URL, filepath.Join(dir, "b.tok"))
	if err := os.WriteFile(f.devices, []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestFleetAuditGradesAndTracksNewFindings(t *testing.T) {
	f := newFleetFixture(t)
	out, err := runCLI(t, "fleet", "audit", "--devices", f.devices, "--json")
	if err != nil {
		t.Fatalf("fleet audit: %v\n%s", err, out)
	}
	var view fleetAuditView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	rows := map[string]auditRow{}
	for _, r := range view.Matrix {
		rows[r.Device] = r
	}
	a, b := rows["a"], rows["b"]
	if a.Firmware != "WARN" || a.SSLVPN != "FAIL" || a.Admins != "FAIL" || a.Licenses != "WARN" || a.Status != "FAIL" {
		t.Errorf("device a row = %+v", a)
	}
	if b.Status != "UNKNOWN" || b.SSLVPN != "UNKNOWN" {
		t.Errorf("unreadable device b must be UNKNOWN, got %+v", b)
	}
	if len(view.Findings) == 0 {
		t.Fatal("no findings reported")
	}

	// Second run with --new: nothing changed, so no new findings.
	out, err = runCLI(t, "fleet", "audit", "--devices", f.devices, "--json", "--new")
	if err != nil {
		t.Fatalf("fleet audit --new: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Findings) != 0 || view.PreviousRun == "" {
		t.Errorf("--new on an unchanged fleet = %+v", view)
	}

	// --agent compacts output but must keep the VDOM of SSL-VPN findings.
	out, err = runCLI(t, "fleet", "audit", "--devices", f.devices, "--agent", "--check", "sslvpn")
	if err != nil {
		t.Fatalf("fleet audit --agent: %v", err)
	}
	if !strings.Contains(out, `"vdom":"root"`) && !strings.Contains(out, `"vdom": "root"`) {
		t.Errorf("--agent dropped vdom from findings: %s", out)
	}

	// An unreachable device (UNKNOWN) trips --fail-on too: "not checked" is not "fine".
	if _, err := runCLI(t, "fleet", "audit", "--devices", f.devices, "--device", "b", "--json", "--fail-on", "fail"); ExitCode(err) != 6 {
		t.Errorf("unreachable-only fleet with --fail-on fail exit = %d (%v)", ExitCode(err), err)
	}

	// --fail-on fail exits 6.
	_, err = runCLI(t, "fleet", "audit", "--devices", f.devices, "--json", "--fail-on", "fail", "--check", "sslvpn")
	if ExitCode(err) != 6 {
		t.Errorf("--fail-on fail exit = %d (%v)", ExitCode(err), err)
	}
}

func TestAuditLicensesQueue(t *testing.T) {
	f := newFleetFixture(t)
	out, err := runCLI(t, "audit", "licenses", "--devices", f.devices, "--device", "a", "--within", "30d", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var v licenseView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Queue) != 1 || v.Queue[0].Service != "antivirus" || v.Queue[0].Grade != "WARN" || v.Queue[0].Serial != "FGT60F0000000001" {
		t.Errorf("queue = %+v", v.Queue)
	}
	// Outside the window nothing is flagged.
	out, err = runCLI(t, "audit", "licenses", "--devices", f.devices, "--device", "a", "--within", "1d", "--json")
	var narrow licenseView
	if err != nil || json.Unmarshal([]byte(out), &narrow) != nil || len(narrow.Findings) != 0 {
		t.Errorf("1d window: err=%v out=%s", err, out)
	}
}

func TestPoliciesHygieneAndFleetHealth(t *testing.T) {
	f := newFleetFixture(t)
	out, err := runCLI(t, "policies", "hygiene", "--devices", f.devices, "--device", "a", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var h hygieneView
	if err := json.Unmarshal([]byte(out), &h); err != nil {
		t.Fatal(err)
	}
	if h.Summary["any-any-accept"] != 1 || h.Summary["logging-disabled"] != 1 || h.Summary["unused"] != 1 {
		t.Errorf("summary = %v", h.Summary)
	}

	out, err = runCLI(t, "fleet", "health", "--devices", f.devices, "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var fh fleetHealthView
	if err := json.Unmarshal([]byte(out), &fh); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range fh.Devices {
		got[d.Device] = d.Status
		if d.Device == "a" && (d.HASync != "out-of-sync" || d.TunnelsDown != 1 || len(d.CertsExpiring) != 1) {
			t.Errorf("device a health = %+v", d)
		}
	}
	if got["a"] != "FAIL" || got["b"] != "UNKNOWN" {
		t.Errorf("health statuses = %v", got)
	}
}

func TestFleetBackupThenChanges(t *testing.T) {
	f := newFleetFixture(t)
	dir := t.TempDir()
	out, err := runCLI(t, "fleet", "backup", "--devices", f.devices, "--device", "a", "--out-dir", dir, "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "a@*", "*.conf"))
	if len(files) != 1 {
		t.Fatalf("backup files = %v", files)
	}
	if info, _ := os.Stat(files[0]); info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v", info.Mode().Perm())
	}
	// A second, changed backup: rename the first so both timestamps differ.
	old := filepath.Join(filepath.Dir(files[0]), "a-20260101T000000Z.conf")
	if err := os.Rename(files[0], old); err != nil {
		t.Fatal(err)
	}
	f.backup = "config system global\n    set hostname \"hq-renamed\"\nend\nconfig system admin\n    edit \"contractor\"\n        set accprofile \"prof_admin\"\n    next\nend\n"
	if _, err := runCLI(t, "fleet", "backup", "--devices", f.devices, "--device", "a", "--out-dir", dir, "--json"); err != nil {
		t.Fatal(err)
	}
	out, err = runCLI(t, "fleet", "changes", "--dir", dir, "--since", "1d", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var res []deviceChanges
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || len(res[0].Changes) != 2 || res[0].Changes[0].Table != "system admin" || res[0].Changes[0].Kind != "added" {
		t.Errorf("changes = %+v", res)
	}

	// No backups at all: empty JSON list, exit 0.
	out, err = runCLI(t, "fleet", "changes", "--dir", t.TempDir(), "--json")
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("empty backups = %q, %v", out, err)
	}
}

// token_env is rejected, never read: an env var named at run time cannot be
// declared in the MCP manifest (check_env_schema), so it would silently fail there.
func TestFleetTokenEnvRejected(t *testing.T) {
	t.Setenv("FGT_X", "tok-x")
	if _, err := resolveToken(fleetDevice{Name: "x", TokenEnv: "FGT_X"}); err == nil || !strings.Contains(err.Error(), "token_file") {
		t.Errorf("token_env = %v, want a token_file error", err)
	}
}

func TestFleetDevicesNeverPrintsTokens(t *testing.T) {
	f := newFleetFixture(t)
	out, err := runCLI(t, "fleet", "devices", "--devices", f.devices, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "tok-a") || !strings.Contains(out, "a.tok") {
		t.Errorf("devices output leaked or lost the token source: %s", out)
	}
	if _, err := runCLI(t, "fleet", "audit", "--devices", f.devices, "--device", "nope", "--json"); ExitCode(err) != 2 {
		t.Errorf("unknown --device exit = %d", ExitCode(err))
	}
}

func TestNormalizeAPIURL(t *testing.T) {
	got, err := normalizeAPIURL("https://203.0.113.10:8443/")
	if err != nil || got != "https://203.0.113.10:8443/api/v2" {
		t.Fatalf("normalize = %q, %v", got, err)
	}
	if got, _ := normalizeAPIURL("https://fgt.example.com/api/v2"); got != "https://fgt.example.com/api/v2" {
		t.Fatalf("already-normalized = %q", got)
	}
	if _, err := normalizeAPIURL("http://fgt.example.com"); err == nil {
		t.Fatal("plain http accepted")
	}
}

// A devices file the operator named must exist and list devices: falling back
// to the single FORTINET_BASE_URL device would audit one box while the
// operator believes the whole fleet passed.
func TestFleetNamedDevicesFileMustExistAndListDevices(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORTINET_BASE_URL", "https://203.0.113.10/api/v2")
	t.Setenv("FORTINET_API_TOKEN", "tok")
	t.Setenv("FORTINET_DEVICES", filepath.Join(dir, "missing.toml"))
	if _, err := runCLI(t, "fleet", "devices", "--json"); err == nil {
		t.Error("missing FORTINET_DEVICES file fell back to the single device")
	}
	typo := filepath.Join(dir, "typo.toml")
	_ = os.WriteFile(typo, []byte("[[devices]]\nname = \"a\"\n"), 0o600)
	t.Setenv("FORTINET_DEVICES", typo)
	if _, err := runCLI(t, "fleet", "audit", "--fail-on", "fail", "--json"); err == nil {
		t.Error("devices file with no [[device]] entries passed")
	}
	t.Setenv("FORTINET_DEVICES", "${user_config.fortinet_devices}")
	if out, err := runCLI(t, "fleet", "devices", "--json"); err != nil || !strings.Contains(out, "203.0.113.10") {
		t.Errorf("unset .mcpb placeholder should read as unset: %q, %v", out, err)
	}
	t.Setenv("FORTINET_DEVICES", "<your-fortinet_devices>")
	if out, err := runCLI(t, "fleet", "devices", "--json"); err != nil || !strings.Contains(out, "203.0.113.10") {
		t.Errorf("unedited mcp-install.md placeholder should read as unset: %q, %v", out, err)
	}
}

// Zones are per-VDOM: the same zone name can hold a LAN port in root and a
// WAN port in another VDOM, so each VDOM's SSL-VPN must be graded with its own.
func TestSSLVPNZonesResolvedPerVDOM(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/cmdb/system/interface":
			_, _ = w.Write([]byte(`{"results":[{"name":"wan1","role":"wan"},{"name":"lan","role":"lan"}],"status":"success"}`))
		case "/api/v2/cmdb/system/zone":
			member := "lan"
			if r.URL.Query().Get("vdom") == "guest" {
				member = "wan1"
			}
			_, _ = fmt.Fprintf(w, `{"results":[{"name":"Z","interface":[{"interface-name":%q}]}],"status":"success"}`, member)
		case "/api/v2/cmdb/vpn.ssl/settings":
			_, _ = w.Write([]byte(`{"results":{"status":"enable","source-interface":[{"name":"Z"}],"source-address":[{"name":"all"}]},"status":"success"}`))
		case "/api/v2/monitor/vpn/ssl":
			_, _ = w.Write([]byte(`{"results":[],"status":"success"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "a.tok")
	if err := os.WriteFile(tok, []byte("t"), 0o600); err != nil {
		t.Fatal(err)
	}
	devices := filepath.Join(dir, "devices.toml")
	toml := fmt.Sprintf("[[device]]\nname = \"a\"\nurl = %q\ntoken_file = %q\ninsecure = true\nvdoms = [\"root\", \"guest\"]\n", srv.URL, tok)
	if err := os.WriteFile(devices, []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runCLI(t, "audit", "sslvpn", "--devices", devices, "--json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		var wrapped map[string]json.RawMessage
		if json.Unmarshal([]byte(out), &wrapped) != nil {
			t.Fatalf("not JSON: %v\n%s", err, out)
		}
		for _, v := range wrapped {
			if json.Unmarshal(v, &rows) == nil && len(rows) > 0 {
				break
			}
		}
	}
	got := map[string]string{}
	for _, r := range rows {
		vdom, _ := r["vdom"].(string)
		status, _ := r["status"].(string)
		if got[vdom] == "" || status == "FAIL" {
			got[vdom] = status
		}
	}
	if got["guest"] != "FAIL" || got["root"] == "FAIL" {
		t.Errorf("per-VDOM grades = %v, want guest FAIL (zone holds wan1) and root not FAIL\n%s", got, out)
	}
}

// --timeout is per request. A fleet that takes longer in total than --timeout
// must still be read in full, and the audit history must still be saved.
func TestFleetTimeoutIsPerRequestNotPerFleet(t *testing.T) {
	testenv.Isolate(t)
	backup := "config system global\nend\n"
	inner := fakeFortiGate(t, "tok", &backup)
	defer inner.Close()
	slow := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(40 * time.Millisecond)
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer slow.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "a.tok")
	if err := os.WriteFile(tok, []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}
	var toml strings.Builder
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&toml, "[[device]]\nname = \"d%d\"\nurl = %q\ntoken_file = %q\ninsecure = true\n\n", i, slow.URL, tok)
	}
	devices := filepath.Join(dir, "devices.toml")
	if err := os.WriteFile(devices, []byte(toml.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"fleet", "audit", "--devices", devices, "--json", "--concurrency", "1", "--timeout", "1s", "--rate-limit", "0"}
	start := time.Now()
	out, err := runCLI(t, args...)
	if err != nil {
		t.Fatalf("fleet audit: %v\n%s", err, out)
	}
	if time.Since(start) < time.Second {
		t.Skip("fleet finished inside one --timeout; the test cannot tell per-request from per-fleet")
	}
	var view fleetAuditView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	for _, r := range view.Matrix {
		if r.Status == "UNKNOWN" {
			t.Errorf("device %s UNKNOWN: the fleet-wide deadline cut it off", r.Device)
		}
	}
	out, _ = runCLI(t, append(args, "--new")...)
	if err := json.Unmarshal([]byte(out), &view); err != nil || view.PreviousRun == "" {
		t.Errorf("audit history not saved after a run longer than --timeout: %v %q", err, view.PreviousRun)
	}
}

// The documented --select view must keep the devices it could not read.
func TestAuditLicensesSelectKeepsFailures(t *testing.T) {
	f := newFleetFixture(t)
	out, _ := runCLI(t, "audit", "licenses", "--devices", f.devices, "--within", "30d", "--select", "queue,findings", "--agent")
	if !strings.Contains(out, `"findings"`) || !strings.Contains(out, `"b"`) {
		t.Errorf("--select queue,findings dropped the unreadable device b: %s", out)
	}
}

// A 404 for vpn.ssl/settings on a VDOM that does not answer is UNKNOWN, not
// "no SSL-VPN on this build" (PASS): a mistyped --vdoms must not grade green.
func TestSSLVPN404OnUnknownVDOMIsUnknown(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/cmdb/system/interface" {
			_, _ = w.Write([]byte(`{"results":[{"name":"wan1","role":"wan"}],"status":"success"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"http_status":404,"status":"error"}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "a.tok")
	_ = os.WriteFile(tok, []byte("t"), 0o600)
	devices := filepath.Join(dir, "devices.toml")
	_ = os.WriteFile(devices, []byte(fmt.Sprintf("[[device]]\nname = \"a\"\nurl = %q\ntoken_file = %q\ninsecure = true\n", srv.URL, tok)), 0o600)
	out, _ := runCLI(t, "audit", "sslvpn", "--devices", devices, "--vdoms", "nosuchvdom", "--json")
	if !strings.Contains(out, `"UNKNOWN"`) || strings.Contains(out, `"PASS"`) {
		t.Errorf("404 on an unconfirmed VDOM graded %s", out)
	}
}

// A device that sends the backup's headers and then stalls must not hang
// fleet backup: the streaming download is bounded at five request timeouts.
func TestBackupStalledBodyTimesOut(t *testing.T) {
	testenv.Isolate(t)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "100000")
		_, _ = w.Write([]byte("config system global\n"))
		w.(http.Flusher).Flush()
		<-release
	}))
	defer srv.Close()
	defer close(release) // runs first: frees the stalled handler so Close can return
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.BaseURL = srv.URL + "/api/v2"
	cfg.FortinetApiToken = "t"
	c := client.New(cfg, 200*time.Millisecond, 0)
	c.NoCache = true
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := backupConfig(context.Background(), c); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stalled backup returned no error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fleet backup hung on a body that never finishes")
	}
}

// When the address book cannot be read, the UNKNOWN finding says why.
func TestSSLVPNSourceUnresolvedNamesTheReadError(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/cmdb/system/interface":
			_, _ = w.Write([]byte(`{"results":[{"name":"wan1","role":"wan"}],"status":"success"}`))
		case "/api/v2/cmdb/system/zone", "/api/v2/monitor/vpn/ssl":
			_, _ = w.Write([]byte(`{"results":[],"status":"success"}`))
		case "/api/v2/cmdb/vpn.ssl/settings":
			_, _ = w.Write([]byte(`{"results":{"status":"enable","source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}]},"status":"success"}`))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"http_status":403,"status":"error"}`))
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "a.tok")
	_ = os.WriteFile(tok, []byte("t"), 0o600)
	devices := filepath.Join(dir, "devices.toml")
	_ = os.WriteFile(devices, []byte(fmt.Sprintf("[[device]]\nname = \"a\"\nurl = %q\ntoken_file = %q\ninsecure = true\n", srv.URL, tok)), 0o600)
	out, _ := runCLI(t, "audit", "sslvpn", "--devices", devices, "--json")
	if !strings.Contains(out, "source-unresolved") || !strings.Contains(out, "firewall/address") || !strings.Contains(out, "Firewall read access") {
		t.Errorf("unresolved source finding does not name the failed read: %s", out)
	}
}

// A 403 on the doctor probe is a warning, so doctor --fail-on warn trips on it.
func TestDoctorProbe403IsWarn(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"http_status":403,"status":"error"}`))
	}))
	defer srv.Close()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.BaseURL = srv.URL + "/api/v2"
	cfg.FortinetApiToken = "t"
	c := client.New(cfg, 2*time.Second, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	report := map[string]any{}
	doctorProbeCredentials(context.Background(), c, "fortinet-cli", report)
	if v, _ := report["credentials"].(string); !strings.HasPrefix(v, "WARN scope-limited") {
		t.Errorf("403 credentials verdict = %q, want a WARN", v)
	}
}

// With no VDOMs configured every VDOM the device lists is checked, so SSL-VPN
// open in a non-root VDOM is a FAIL; an unreadable VDOM list is never PASS.
func TestSSLVPNChecksEveryListedVDOM(t *testing.T) {
	testenv.Isolate(t)
	vdomList := `{"results":[{"name":"root"},{"name":"cust1"}],"status":"success"}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/cmdb/system/vdom":
			if vdomList == "" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"http_status":403,"status":"error"}`))
				return
			}
			_, _ = w.Write([]byte(vdomList))
		case "/api/v2/cmdb/system/interface":
			_, _ = w.Write([]byte(`{"results":[{"name":"wan1","role":"wan"}],"status":"success"}`))
		case "/api/v2/cmdb/system/zone", "/api/v2/monitor/vpn/ssl", "/api/v2/cmdb/firewall/policy", "/api/v2/monitor/firewall/policy":
			_, _ = w.Write([]byte(`{"results":[],"status":"success"}`))
		case "/api/v2/cmdb/vpn.ssl/settings":
			status := "disable"
			if r.URL.Query().Get("vdom") == "cust1" {
				status = "enable"
			}
			_, _ = fmt.Fprintf(w, `{"results":{"status":%q,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"all"}]},"status":"success"}`, status)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "a.tok")
	_ = os.WriteFile(tok, []byte("t"), 0o600)
	devices := filepath.Join(dir, "devices.toml")
	_ = os.WriteFile(devices, []byte(fmt.Sprintf("[[device]]\nname = \"a\"\nurl = %q\ntoken_file = %q\ninsecure = true\n", srv.URL, tok)), 0o600)
	out, err := runCLI(t, "audit", "sslvpn", "--devices", devices, "--json", "--fail-on", "fail")
	if err == nil || !strings.Contains(out, `"vdom": "cust1"`) || !strings.Contains(out, "FAIL") {
		t.Errorf("SSL-VPN open in VDOM cust1 not graded FAIL: %v %s", err, out)
	}
	vdomList = ""
	out, _ = runCLI(t, "audit", "sslvpn", "--devices", devices, "--json")
	if !strings.Contains(out, "vdoms-unlisted") || !strings.Contains(out, "UNKNOWN") {
		t.Errorf("unreadable VDOM list did not grade UNKNOWN: %s", out)
	}
	out, _ = runCLI(t, "policies", "hygiene", "--devices", devices, "--human-friendly")
	if strings.Contains(out, "No risky or unused policies found.\n") || !strings.Contains(out, "UNKNOWN a (could not list VDOMs") {
		t.Errorf("hygiene human output read clean with the VDOM list unreadable: %q", out)
	}
}

// Keyless live-state monitor tables fail the store integrity check, so the
// default sync must not include them (it exited 1 on every real FortiGate).
func TestDefaultSyncSkipsKeylessMonitorTables(t *testing.T) {
	for _, r := range defaultSyncResources() {
		switch r {
		case "monitor", "monitor-router-ipv4", "monitor-system-firmware-upgrade-paths", "monitor-system-ha-checksums",
			"monitor-user-firewall", "monitor-user-fortitoken", "monitor-wifi-managed-ap":
			t.Errorf("default sync includes keyless table %s", r)
		}
	}
}

// Unreadable hit counters mean "unused" was never evaluated, so the human
// output must not print the clean line.
func TestHygieneHumanOutputNamesUnreadableCounters(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/cmdb/system/vdom":
			_, _ = w.Write([]byte(`{"results":[{"name":"root"}],"status":"success"}`))
		case "/api/v2/cmdb/firewall/policy":
			_, _ = w.Write([]byte(`{"results":[{"policyid":1,"action":"accept","status":"enable","logtraffic":"all","srcaddr":[{"name":"lan"}],"dstaddr":[{"name":"all"}],"service":[{"name":"HTTPS"}]}],"status":"success"}`))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"http_status":403,"status":"error"}`))
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "a.tok")
	_ = os.WriteFile(tok, []byte("t"), 0o600)
	devices := filepath.Join(dir, "devices.toml")
	_ = os.WriteFile(devices, []byte(fmt.Sprintf("[[device]]\nname = \"a\"\nurl = %q\ntoken_file = %q\ninsecure = true\n", srv.URL, tok)), 0o600)
	out, _ := runCLI(t, "policies", "hygiene", "--devices", devices, "--human-friendly")
	if strings.Contains(out, "No risky or unused policies found.") || !strings.Contains(out, "unused policies were not evaluated there") {
		t.Errorf("hygiene human output read clean with hit counters unreadable: %q", out)
	}
}

// An authentication rule whose source-address names an object covering every
// address opens the listener even though the global list is "all"-free: the
// collector must read the address book for rule-level names, and the grade
// must be FAIL (Fortinet tip 96303: the rule's source-address takes precedence).
func TestSSLVPNAuthenticationRuleSourceResolved(t *testing.T) {
	testenv.Isolate(t)
	var addrReads int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/cmdb/system/interface":
			_, _ = w.Write([]byte(`{"results":[{"name":"wan1","role":"wan"}],"status":"success"}`))
		case "/api/v2/cmdb/system/zone":
			_, _ = w.Write([]byte(`{"results":[],"status":"success"}`))
		case "/api/v2/cmdb/vpn.ssl/settings":
			_, _ = w.Write([]byte(`{"results":{"status":"enable","source-interface":[{"name":"wan1"}],"source-address":[{"name":"all"}],"authentication-rule":[{"id":3,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"Internet-v4"}],"groups":[{"name":"vpn"}]}]},"status":"success"}`))
		case "/api/v2/cmdb/firewall/address":
			addrReads++
			_, _ = w.Write([]byte(`{"results":[{"name":"Internet-v4","type":"ipmask","subnet":"0.0.0.0 0.0.0.0"}],"status":"success"}`))
		case "/api/v2/cmdb/firewall/addrgrp":
			_, _ = w.Write([]byte(`{"results":[],"status":"success"}`))
		case "/api/v2/monitor/vpn/ssl":
			_, _ = w.Write([]byte(`{"results":[],"status":"success"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "a.tok")
	if err := os.WriteFile(tok, []byte("t"), 0o600); err != nil {
		t.Fatal(err)
	}
	devices := filepath.Join(dir, "devices.toml")
	toml := fmt.Sprintf("[[device]]\nname = \"a\"\nurl = %q\ntoken_file = %q\ninsecure = true\nvdoms = [\"root\"]\n", srv.URL, tok)
	if err := os.WriteFile(devices, []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runCLI(t, "audit", "sslvpn", "--devices", devices, "--json")
	if addrReads == 0 {
		t.Fatalf("firewall/address never read for a rule-level source name\n%s", out)
	}
	if !strings.Contains(out, `"open-to-internet"`) || !strings.Contains(out, "authentication rule 3") {
		t.Fatalf("rule naming a 0.0.0.0/0 object not graded FAIL open-to-internet:\n%s", out)
	}
}

// Two firewalls sharing an inventory name (an "hq" in two clients' devices
// files) must not share --new history, while one device whose credential
// fails to resolve must keep the scope it has when it succeeds.
func TestAuditScopeSeparatesDevicesByURL(t *testing.T) {
	checks := map[string]bool{"sslvpn": true}
	hq := func(url string) []fleetTarget {
		return []fleetTarget{{fleetDevice: fleetDevice{Name: "hq"}, apiURL: url}}
	}
	if auditScope(hq("https://192.0.2.1/api/v2"), nil, checks) == auditScope(hq("https://198.51.100.1/api/v2"), nil, checks) {
		t.Fatal("two different firewalls named hq share an audit history scope")
	}
	failed := []fetchFailure{{ID: "hq", url: "https://192.0.2.1/api/v2"}}
	if auditScope(hq("https://192.0.2.1/api/v2"), nil, checks) != auditScope(nil, failed, checks) {
		t.Fatal("a credential failure changed the device's audit history scope")
	}
}

// Two firewalls sharing an inventory name in different devices files (an "hq"
// per client) must get separate backup directories, and fleet changes must
// never diff one client's configuration against the other's.
func TestFleetBackupSeparatesFirewallsSharingAName(t *testing.T) {
	testenv.Isolate(t)
	backupA := "config system global\n    set hostname \"client-a\"\nend\n"
	backupB := "config system global\n    set hostname \"client-b\"\nend\nconfig system admin\n    edit \"other-client-admin\"\n        set accprofile \"super_admin\"\n    next\nend\n"
	srvA := fakeFortiGate(t, "tok", &backupA)
	srvB := fakeFortiGate(t, "tok", &backupB)
	t.Cleanup(srvA.Close)
	t.Cleanup(srvB.Close)
	cfg := t.TempDir()
	tok := filepath.Join(cfg, "hq.tok")
	if err := os.WriteFile(tok, []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}
	devicesFor := func(file, url string) string {
		p := filepath.Join(cfg, file)
		body := fmt.Sprintf("[[device]]\nname = \"hq\"\nurl = %q\ntoken_file = %q\ninsecure = true\n", url, tok)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	dir := t.TempDir()
	for _, devices := range []string{devicesFor("client-a.toml", srvA.URL), devicesFor("client-b.toml", srvB.URL)} {
		if out, err := runCLI(t, "fleet", "backup", "--devices", devices, "--out-dir", dir, "--json"); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	dirs, _ := filepath.Glob(filepath.Join(dir, "hq@*"))
	if len(dirs) != 2 {
		t.Fatalf("two firewalls named hq share a backup directory: %v", dirs)
	}
	out, err := runCLI(t, "fleet", "changes", "--dir", dir, "--device", "hq", "--since", "0s", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var res []deviceChanges
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].DeviceID == "" || res[0].DeviceID == res[1].DeviceID {
		t.Fatalf("want one entry per firewall with distinct device_id: %+v", res)
	}
	for _, r := range res {
		if r.Device != "hq" || len(r.Changes) != 0 {
			t.Errorf("client A's backup was diffed against client B's: %+v", r)
		}
	}
}

// The human fleet health line must name the checks it skipped: a PASS covers
// only the evaluated signals, so a bare "ok" would read as full coverage.
func TestFleetHealthHumanOutputNamesSkippedChecks(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/monitor/system/status":
			_, _ = w.Write([]byte(`{"results":{"model":"FGT60F"},"serial":"FGT60F0000000001","version":"v7.4.3","status":"success"}`))
		case "/api/v2/monitor/system/resource/usage":
			_, _ = w.Write([]byte(`{"results":{"unexpected":"shape"},"status":"success"}`))
		case "/api/v2/monitor/system/available-certificates":
			_, _ = w.Write([]byte(`{"results":[{"name":"vpn-cert","valid_to":"soon"}],"status":"success"}`))
		case "/api/v2/cmdb/system/vdom":
			_, _ = w.Write([]byte(`{"results":[{"name":"root"}],"status":"success"}`))
		default:
			_, _ = w.Write([]byte(`{"results":[],"status":"success"}`))
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "a.tok")
	if err := os.WriteFile(tok, []byte("t"), 0o600); err != nil {
		t.Fatal(err)
	}
	devices := filepath.Join(dir, "devices.toml")
	if err := os.WriteFile(devices, []byte(fmt.Sprintf("[[device]]\nname = \"a\"\nurl = %q\ntoken_file = %q\ninsecure = true\n", srv.URL, tok)), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "fleet", "health", "--devices", devices, "--human-friendly")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "not evaluated: cpu/memory") || !strings.Contains(out, "certificates vpn-cert (no recognizable valid_to)") {
		t.Fatalf("human health output hides a skipped check (cpu/memory, undated certificate):\n%s", out)
	}
}
