package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fortinet-pp-cli/internal/cliutil/testenv"
)

func TestCheckAPIURLRefusesCredentialLeaks(t *testing.T) {
	for _, bad := range []string{"https://admin:pw@fgt.example.net", "http://fgt.example.net", "ftp://fgt.example.net"} {
		if _, err := checkAPIURL(bad); err == nil {
			t.Errorf("%s accepted", bad)
		} else if strings.Contains(err.Error(), "pw") {
			t.Errorf("error leaks the password: %v", err)
		}
	}
	for _, ok := range []string{"https://fgt.example.net:8443", "http://127.0.0.1:9000", "http://localhost:9000"} {
		if _, err := checkAPIURL(ok); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
}

func TestFallbackRefusesPlainHTTPAndDeviceFilter(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("FORTINET_BASE_URL", "http://fgt.example.net/api/v2")
	if _, err := runCLI(t, "fleet", "audit", "--json"); ExitCode(err) != 10 {
		t.Errorf("plain-http FORTINET_BASE_URL exit = %d (%v)", ExitCode(err), err)
	}
	t.Setenv("FORTINET_BASE_URL", "https://fgt.example.net/api/v2")
	if _, err := runCLI(t, "fleet", "audit", "--json", "--device", "acme"); ExitCode(err) != 2 {
		t.Errorf("--device without a devices file exit = %d (%v)", ExitCode(err), err)
	}
}

func TestDeviceClientDropsConfiguredCredentialHeaders(t *testing.T) {
	testenv.Isolate(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[headers]\nAuthorization = \"Bearer configured\"\nX-Trace = \"keep\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := &rootFlags{configPath: cfgPath}
	c, err := fleetTarget{fleetDevice: fleetDevice{Name: "a"}, token: "device-token", apiURL: "https://fgt.example.net/api/v2"}.client(flags)
	if err != nil {
		t.Fatal(err)
	}
	for k := range c.Config.Headers {
		if strings.EqualFold(k, "authorization") {
			t.Fatalf("configured Authorization header survived: %v", c.Config.Headers)
		}
	}
	if c.Config.Headers["X-Trace"] != "keep" {
		t.Errorf("non-credential header dropped: %v", c.Config.Headers)
	}
	if got := c.Config.AuthHeader(); got != "Bearer device-token" {
		t.Errorf("auth header = %q", got)
	}
}

func TestBackupFilesNeverOverwriteOrCollide(t *testing.T) {
	if deviceDirName("a/b") == deviceDirName("a_b") || deviceDirName("a/b") == deviceDirName(deviceDirName("a/b")) {
		t.Fatal("sanitized names collide")
	}
	if deviceDirName("acme-hq") != "acme-hq" {
		t.Fatal("clean names should be kept as-is")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "x.conf")
	if err := writeNewPrivateFile(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := writeNewPrivateFile(path, []byte("two")); err == nil {
		t.Fatal("existing backup overwritten")
	}
	link := filepath.Join(dir, "link.conf")
	if err := os.Symlink(filepath.Join(dir, "elsewhere"), link); err != nil {
		t.Fatal(err)
	}
	if err := writeNewPrivateFile(link, []byte("secret")); err == nil {
		t.Fatal("followed a planted symlink")
	}
	if _, err := os.Stat(filepath.Join(dir, "elsewhere")); err == nil {
		t.Fatal("symlink target was created")
	}
}

func TestFleetBackupFailureExits5(t *testing.T) {
	f := newFleetFixture(t)
	// Device b rejects the token, so its backup fails.
	_, err := runCLI(t, "fleet", "backup", "--devices", f.devices, "--out-dir", t.TempDir(), "--json")
	if ExitCode(err) != 5 {
		t.Errorf("failed backup exit = %d (%v)", ExitCode(err), err)
	}
}

func TestMissingResultsIsUnknownNotPass(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/cmdb/system/interface":
			_, _ = w.Write([]byte(`{"results":[{"name":"wan1","role":"wan"}],"status":"success"}`))
		default: // settings with no results, as a broken proxy or odd build might return
			_, _ = w.Write([]byte(`{"status":"success"}`))
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "t.tok")
	_ = os.WriteFile(tok, []byte("tok"), 0o600)
	devices := filepath.Join(dir, "devices.toml")
	_ = os.WriteFile(devices, []byte(fmt.Sprintf("[[device]]\nname = \"t\"\nurl = %q\ntoken_file = %q\ninsecure = true\n", srv.URL, tok)), 0o600)
	out, err := runCLI(t, "audit", "sslvpn", "--devices", devices, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"UNKNOWN"`) || strings.Contains(out, `"PASS"`) {
		t.Errorf("missing results graded as %s", out)
	}
}

// A FortiGate (or anything answering on its HTTPS address, e.g. behind
// --insecure) that redirects to plain http must never receive the bearer
// token over that http hop, on the generated client's normal or streaming
// (backup download) path, or the fallback POST form.
func TestRedirectNeverSendsTokenOverPlainHTTP(t *testing.T) {
	testenv.Isolate(t)
	var leaked []string
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a := r.Header.Get("Authorization"); a != "" {
			leaked = append(leaked, r.URL.Path+" "+a)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"status":"success"}`))
	}))
	defer plain.Close()
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	defer tlsSrv.Close()
	t.Setenv("FORTINET_BASE_URL", tlsSrv.URL+"/api/v2")
	t.Setenv("FORTINET_API_TOKEN", "secret-token")
	if out, err := runCLI(t, "addresses", "list", "--insecure", "--json", "--data-source", "live"); err == nil {
		t.Errorf("an https-to-http redirect was followed:\n%s", out)
	}
	dir := t.TempDir()
	tok := filepath.Join(dir, "a.tok")
	if err := os.WriteFile(tok, []byte("secret-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	devices := filepath.Join(dir, "devices.toml")
	if err := os.WriteFile(devices, []byte(fmt.Sprintf("[[device]]\nname = \"a\"\nurl = %q\ntoken_file = %q\ninsecure = true\n", tlsSrv.URL, tok)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = runCLI(t, "fleet", "backup", "--devices", devices, "--out-dir", t.TempDir(), "--json")
	_, _ = runCLI(t, "audit", "sslvpn", "--devices", devices, "--json")
	if len(leaked) > 0 {
		t.Fatalf("bearer token sent over plain http after a redirect: %v", leaked)
	}
}

// A redirect off the FortiGate's origin (another public host, even over
// https) is refused: Go strips Authorization on a cross-host hop but copies
// every other header, so a configured credential header (a gateway's
// X-API-Key) would otherwise reach the other host. httptest servers are
// loopback, which the generated client already refuses as a private
// destination, so this drives the client's real CheckRedirect with public
// names, for the single-device client and a fleet device client.
func TestRedirectOffOriginIsRefused(t *testing.T) {
	testenv.Isolate(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[headers]\nX-API-Key = \"gateway-secret\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORTINET_BASE_URL", "https://fgt.example.net/api/v2")
	t.Setenv("FORTINET_API_TOKEN", "secret-token")
	flags := &rootFlags{configPath: cfgPath}
	single, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	device, err := fleetTarget{fleetDevice: fleetDevice{Name: "a"}, token: "device-token", apiURL: "https://fgt.example.net/api/v2"}.client(flags)
	if err != nil {
		t.Fatal(err)
	}
	hop := func(raw string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for name, c := range map[string]*http.Client{"single": single.HTTPClient, "fleet": device.HTTPClient} {
		if c.CheckRedirect == nil {
			t.Fatalf("%s client has no redirect policy", name)
		}
		via := []*http.Request{hop("https://fgt.example.net/api/v2/cmdb/firewall/address")}
		for _, target := range []string{"https://attacker.example.com/api/v2/cmdb/firewall/address", "https://fgt.example.net:8443/api/v2/cmdb/firewall/address"} {
			if err := c.CheckRedirect(hop(target), via); err == nil {
				t.Errorf("%s client follows a redirect off origin to %s", name, target)
			}
		}
		if err := c.CheckRedirect(hop("https://fgt.example.net/api/v2/cmdb/firewall/address?vdom=root"), via); err != nil {
			t.Errorf("%s client refuses a same-origin redirect: %v", name, err)
		}
	}
}
