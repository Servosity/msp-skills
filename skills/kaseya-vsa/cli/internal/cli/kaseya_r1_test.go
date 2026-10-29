// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Review round 1 (Codex, 2026-10-27): each test reproduces a [P1] class on
// the pre-fix tree.

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"kaseya-vsa-pp-cli/internal/cliutil/testenv"
)

const (
	r1TokenID     = "id7fb3189b"
	r1TokenSecret = "R8v52W4o" + "1n7Q6u3s" // split: a synthetic fixture, not a credential for secret scanners
)

func r1Server(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return trustTestServer(t, srv)
}

func r1Env(t *testing.T, host string) string {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("KASEYA_VSA_SERVER", host)
	t.Setenv("KASEYA_VSA_BASE_URL", "")
	t.Setenv("KASEYA_VSA_CA_CERT", testCAFile(t, host))
	t.Setenv("KASEYA_VSA_TOKEN_ID", r1TokenID)
	t.Setenv("KASEYA_VSA_TOKEN_SECRET", r1TokenSecret)
	return home
}

func r1Run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err != nil {
		out.WriteString("\nerror: " + err.Error())
	}
	return out.String(), err
}

// Class: credential sent to another host. --path-context (also exposed on the
// MCP sync tool) rewrote the {server} template after the client was built, so
// the operator's Basic header went to the supplied host.
func TestR1SyncPathContextCannotMoveTheAPIHost(t *testing.T) {
	var leaked atomic.Int32
	evil := r1Server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Add(1)
		}
		_, _ = w.Write([]byte(`{"Data":[],"Meta":{"ResponseCode":200}}`))
	})
	tenant := r1Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Data":[],"Meta":{"ResponseCode":200}}`))
	})
	home := r1Env(t, tenant)
	_, _ = r1Run(t, "sync", "--resources", "assets", "--path-context", "server="+evil, "--max-pages", "1",
		"--db", filepath.Join(home, "r1.db"))
	if n := leaked.Load(); n != 0 {
		t.Fatalf("%d credentialed request(s) reached the --path-context host", n)
	}
}

// Class: a redirect is a server-chosen host. No hop may leave the origin.
func TestR1RedirectNeverLeavesTheOrigin(t *testing.T) {
	var hits atomic.Int32
	evil := r1Server(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"Data":[],"Meta":{"ResponseCode":200}}`))
	})
	tenant := r1Server(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+evil+"/api/v3/organizations", http.StatusFound)
	})
	r1Env(t, tenant)
	_, _ = r1Run(t, "organizations", "list", "--json", "--no-cache")
	if n := hits.Load(); n != 0 {
		t.Fatalf("%d request(s) followed a redirect to another origin", n)
	}
}

// Class: secret in error text. A run response without an execution ID was
// echoed raw into the error, bypassing the client's credential masking.
func TestR1RunWaitErrorsNeverEchoTheSecret(t *testing.T) {
	echo := `{"Data":{"Message":"` + r1TokenSecret + `"},"Meta":{"ResponseCode":200}}`
	host := r1Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(echo))
	})
	r1Env(t, host)
	for _, args := range [][]string{
		{"tasks", "run-wait", "1234", "--task-scope", "--agent"},
		{"scripts", "run-wait", "77", "--device", "11110000-0000-4000-8000-000000000000", "--agent"},
	} {
		out, err := r1Run(t, args...)
		if err == nil {
			t.Fatalf("%v: want an error for a response with no execution ID", args)
		}
		if strings.Contains(out, r1TokenSecret) {
			t.Fatalf("%v: token secret in output:\n%s", args, out)
		}
	}
}

// Class: secret-bearing tenant values persisted on disk. Custom-field
// GlobalValue/DefaultValue, webhook SecretKey and the asset Product Key are
// documented response fields (vendor-docs/api.vsax.net.html); neither sync,
// the write-through cache nor the response cache may keep them.
func TestR1SecretValuesNeverReachDisk(t *testing.T) {
	const (
		cfSecret  = "AdminPassword-7fb3189b"
		cfDefault = "DefaultPass-41c7e2"
		hookKey   = "a3f19c0e" + "5d7b2a8f" + "4c6e1d9b" + "0a7f3e5c" + "2d8b6a4f" + "1e9c7d5b" + "3a1f0e8d" + "6c4b2a9f" // split: synthetic fixture
		productK  = "VK7JG-NPHTM-C97JM-9MPGT-3V66T"
	)
	host := r1Server(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v3/customFields"):
			_, _ = w.Write([]byte(`{"Data":[{"Id":7,"Name":"LocalAdminPassword","Type":"Text","DefaultValue":"` + cfDefault + `","GlobalValue":"` + cfSecret + `","IsReadOnly":false}],"Meta":{"ResponseCode":200,"TotalCount":1}}`))
		case strings.HasPrefix(r.URL.Path, "/api/v3/notifications/webhooks"):
			_, _ = w.Write([]byte(`{"Data":[{"Id":101,"Name":"MyHook","Url":"https://example.com/hook","SecretKey":"` + hookKey + `"}],"Meta":{"ResponseCode":200,"TotalCount":1}}`))
		case strings.HasPrefix(r.URL.Path, "/api/v3/assets"):
			_, _ = w.Write([]byte(`{"Data":[{"Identifier":"11110000-0000-4000-8000-000000000000","Name":"PC-1","AssetInfo":[{"CategoryName":"Windows","CategoryData":{"Product Key":"` + productK + `"}}]}],"Meta":{"ResponseCode":200,"TotalCount":1}}`))
		default:
			_, _ = w.Write([]byte(`{"Data":[],"Meta":{"ResponseCode":200}}`))
		}
	})
	home := r1Env(t, host)
	for _, args := range [][]string{
		{"custom-fields", "list", "--json"},
		{"webhooks", "list", "--json"},
		{"assets", "list", "--json"},
		{"sync", "--resources", "custom_fields,webhooks,assets"},
	} {
		if out, err := r1Run(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		for _, secret := range []string{cfSecret, cfDefault, hookKey, productK} {
			if bytes.Contains(data, []byte(secret)) {
				t.Errorf("%s holds %q", path, secret)
			}
		}
		return nil
	})
}
