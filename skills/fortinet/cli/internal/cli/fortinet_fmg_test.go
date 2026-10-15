package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fortinet-pp-cli/internal/cliutil/testenv"
)

func TestFMGDevicesAcrossADOMs(t *testing.T) {
	testenv.Isolate(t)
	var methods []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Method string           `json:"method"`
			Params []map[string]any `json:"params"`
		}
		_ = json.Unmarshal(b, &req)
		methods = append(methods, req.Method)
		url, _ := req.Params[0]["url"].(string)
		switch {
		case url == "/dvmdb/adom":
			_, _ = w.Write([]byte(`{"id":1,"result":[{"status":{"code":0,"message":"OK"},"data":[{"name":"acme"},{"name":"globex"}]}]}`))
		case url == "/dvmdb/adom/acme/device":
			_, _ = w.Write([]byte(`{"id":2,"result":[{"status":{"code":0,"message":"OK"},"data":[{"name":"acme-hq","sn":"FGT60F0000000001","platform_str":"FortiGate-60F","os_ver":"7.0","mr":4,"patch":3,"build":2573,"conn_status":"up","conf_status":"insync"}]}]}`))
		case strings.HasPrefix(url, "/dvmdb/adom/globex/"):
			_, _ = w.Write([]byte(`{"id":3,"result":[{"status":{"code":-11,"message":"No permission for the resource"}}]}`))
		default:
			t.Errorf("unexpected url %q", url)
		}
	}))
	defer srv.Close()
	t.Setenv("FORTINET_FMG_URL", srv.URL)
	t.Setenv("FORTINET_FMG_API_KEY", "k")

	out, err := runCLI(t, "fmg", "devices", "--insecure", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var view struct {
		Devices       []fmgDeviceRow `json:"devices"`
		FetchFailures []fetchFailure `json:"fetch_failures"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(view.Devices) != 1 || view.Devices[0].Firmware != "7.4.3 build2573" || view.Devices[0].ADOM != "acme" {
		t.Errorf("devices = %+v", view.Devices)
	}
	if len(view.FetchFailures) != 1 || view.FetchFailures[0].ID != "globex" {
		t.Errorf("a permission error must surface as a failure, got %+v", view.FetchFailures)
	}
	for _, m := range methods {
		if m != "get" {
			t.Errorf("non-get JSON-RPC method sent: %q", m)
		}
	}
	if _, err := runCLI(t, "fmg", "devices", "--insecure", "--json", "--adom", "../sys"); err != nil {
		t.Errorf("invalid ADOM should be reported per ADOM, not fail the command: %v", err)
	}
}

func TestFMGRejectedKeyExitsAuth(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("FORTINET_FMG_URL", srv.URL)
	t.Setenv("FORTINET_FMG_API_KEY", "bad")
	if _, err := runCLI(t, "fmg", "status", "--insecure", "--json"); ExitCode(err) != 4 {
		t.Errorf("rejected key exit = %d (%v)", ExitCode(err), err)
	}
}

func TestFMGRequiresConfig(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("FORTINET_FMG_URL", "")
	if _, err := runCLI(t, "fmg", "adoms", "--json"); ExitCode(err) != 10 {
		t.Errorf("missing FMG config exit = %d (%v)", ExitCode(err), err)
	}
}
