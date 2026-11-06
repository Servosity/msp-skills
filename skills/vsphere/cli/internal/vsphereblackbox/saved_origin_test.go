// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

import (
	"bytes"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Saved credentials for the saved-credential-origin class. None shares an
// 8-character piece with another or with ordinary output.
const (
	soUser    = "ro@acme.local"
	soPass    = "AcmePass-1111-aaaa-2222"
	soHdrPass = "SavedPass-1111-bbbb-3333"
	soHdrX    = "SavedHdrVal-4444-cccc-5555"
	soCookie  = "SavedCookie-6666-dddd-7777"
)

// recorder is a fake vCenter that records everything it was sent, so a test
// can count the requests that carried a credential.
type recorder struct {
	url, ca string
	mu      sync.Mutex
	seen    []string
}

func newRecorder(t *testing.T) *recorder {
	t.Helper()
	rc := &recorder{}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen := r.Method + " " + r.URL.String() + "\n" + fmt.Sprint(r.Header) + "\n" + string(body)
		if u, p, ok := r.BasicAuth(); ok {
			seen += "\n" + u + ":" + p
		}
		rc.mu.Lock()
		rc.seen = append(rc.seen, seen)
		rc.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/session":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`"origin-test-session-5e1c"`))
		case r.URL.Path == "/api/vcenter/vm":
			_, _ = w.Write([]byte(`[{"vm":"vm-1021","name":"HQ-Record","power_state":"POWERED_ON"}]`))
		case strings.HasPrefix(r.URL.Path, "/api/"):
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	rc.url = srv.URL
	rc.ca = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(rc.ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return rc
}

// saw is the number of requests that carried secret.
func (rc *recorder) saw(secret string) int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	n := 0
	for _, s := range rc.seen {
		if strings.Contains(s, secret) {
			n++
		}
	}
	return n
}

// soRun runs the CLI, or the MCP server with one vm_list call when args is
// nil, and returns what it printed and its exit code.
func soRun(t *testing.T, home string, env []string, args ...string) (string, int) {
	t.Helper()
	if args == nil {
		return (&bbEnv{home: home, env: env}).mcpCalls(t, []map[string]any{{"name": "vm_list", "arguments": map[string]any{}}}), 0
	}
	cmd := exec.Command(bbCLI, args...)
	cmd.Env, cmd.Dir = env, home
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.String(), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String(), 0
}

// Class saved-credential-origin: a credential saved on disk goes only to the
// vCenter it was saved for, from every source it can be saved in (the config
// file's own fields and [headers], an agentcookie-managed config, the
// credentials file beside a config, the global one), whatever names the target.
// A credential in the environment goes where the environment says.
func TestSavedCredentialsGoOnlyToTheirOrigin(t *testing.T) {
	t.Parallel()
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("ro@a.local:"+soHdrPass))
	vmList := []string{"vm", "list", "--json"}

	// refused asserts B saw none of secrets and the command did not succeed.
	refused := func(t *testing.T, b *recorder, out string, code int, mcp bool, secrets ...string) {
		t.Helper()
		for _, s := range secrets {
			if n := b.saw(s); n != 0 {
				t.Errorf("B received a credential saved for A %d times:\n%.1500s", n, out)
			}
		}
		if !mcp && code == 0 {
			t.Errorf("the command succeeded at B with a credential saved for A:\n%.1500s", out)
		}
		if mcp && strings.Contains(out, "HQ-Record") {
			t.Errorf("vm_list answered from B with a credential saved for A:\n%.1500s", out)
		}
	}

	t.Run("global store under --config in VSPHERE_HOME naming B", func(t *testing.T) {
		t.Parallel()
		a, b := newRecorder(t), newRecorder(t)
		p := t.TempDir()
		env := cleanEnv(p, "VSPHERE_HOME="+p, "VSPHERE_CA_CERT="+a.ca)
		if out, code := soRun(t, p, append(env, "VSPHERE_BASE_URL="+a.url), "auth", "set-credentials", soUser, soPass); code != 0 {
			t.Fatalf("auth set-credentials: rc=%d\n%s", code, out)
		}
		globex := filepath.Join(p, "globex.toml")
		writeFile(t, globex, "base_url = \""+b.url+"\"\n")
		out, code := soRun(t, p, env, "--config", globex, "vm", "list", "--json")
		refused(t, b, out, code, false, soPass)
	})
	t.Run("global store under --config in VSPHERE_HOME, environment names B", func(t *testing.T) {
		t.Parallel()
		a, b := newRecorder(t), newRecorder(t)
		p := t.TempDir()
		env := cleanEnv(p, "VSPHERE_HOME="+p, "VSPHERE_CA_CERT="+a.ca)
		if out, code := soRun(t, p, append(env, "VSPHERE_BASE_URL="+a.url), "auth", "set-credentials", soUser, soPass); code != 0 {
			t.Fatalf("auth set-credentials: rc=%d\n%s", code, out)
		}
		for name, body := range map[string]string{"profile.toml": "", "named-a.toml": "base_url = \"" + a.url + "\"\n"} {
			cfg := filepath.Join(p, name)
			writeFile(t, cfg, body)
			out, code := soRun(t, p, append(env, "VSPHERE_BASE_URL="+b.url), "--config", cfg, "vm", "list", "--json")
			refused(t, b, out, code, false, soPass)
		}
	})
	for name, headers := range map[string]string{
		"Authorization": "Authorization = \"" + basic + "\"\n",
		"X-Api-Key":     "X-Api-Key = \"" + soHdrX + "\"\n",
		"Cookie":        "Cookie = \"vmware-api-session-id=" + soCookie + "\"\n",
	} {
		t.Run("config.toml [headers] "+name+" for A, environment names B", func(t *testing.T) {
			t.Parallel()
			a, b := newRecorder(t), newRecorder(t)
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+a.url+"\"\n\n[headers]\n"+headers)
			env := cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_CA_CERT="+a.ca, "VSPHERE_BASE_URL="+b.url)
			out, code := soRun(t, home, env, vmList...)
			refused(t, b, out, code, false, soHdrPass, soHdrX, soCookie)
			out, _ = soRun(t, home, env)
			refused(t, b, out, 0, true, soHdrPass, soHdrX, soCookie)
			// With a login of its own in the environment, B gets that login only.
			withLogin := append(append([]string(nil), env...), "VSPHERE_USERNAME="+bbUser, "VSPHERE_PASSWORD="+bbPassword)
			soRun(t, home, withLogin, vmList...)
			soRun(t, home, withLogin)
			if b.saw(soHdrPass)+b.saw(soHdrX)+b.saw(soCookie) != 0 {
				t.Errorf("B received A's saved header alongside an environment login")
			}
			if b.saw(bbPassword) == 0 {
				t.Errorf("the environment login never reached B; the control did not run")
			}
		})
	}
	t.Run("agentcookie-managed config for A, environment names B", func(t *testing.T) {
		t.Parallel()
		a, b := newRecorder(t), newRecorder(t)
		home := t.TempDir()
		cfg := filepath.Join(home, "cfg", "c.toml")
		writeFile(t, cfg, "base_url = \""+a.url+"\"\nusername = \"ro@a.local\"\npassword = \""+soHdrPass+"\"\n")
		writeFile(t, filepath.Join(home, "cfg", ".agentcookie-managed"), "")
		env := cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_CA_CERT="+a.ca, "VSPHERE_BASE_URL="+b.url)
		out, code := soRun(t, home, env, "--config", cfg, "vm", "list", "--json")
		refused(t, b, out, code, false, soHdrPass)
	})

	// A base_url written out is the origin even when it spells the built-in
	// default (Codex more-pass round 1).
	t.Run("config naming the built-in default base_url, environment names B", func(t *testing.T) {
		t.Parallel()
		a, b := newRecorder(t), newRecorder(t)
		home := t.TempDir()
		cfg := filepath.Join(home, "cfg", "c.toml")
		writeFile(t, cfg, "base_url = \"https://vcenter.example.com\"\nusername = \"ro@a.local\"\npassword = \""+soHdrPass+"\"\n")
		env := cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_CA_CERT="+a.ca, "VSPHERE_BASE_URL="+b.url)
		out, code := soRun(t, home, env, "--config", cfg, "vm", "list", "--json")
		refused(t, b, out, code, false, soHdrPass)
		out, _ = soRun(t, home, append(env, "VSPHERE_CONFIG="+cfg))
		refused(t, b, out, 0, true, soHdrPass)
	})

	// Controls: each source still works at the vCenter it was saved for.
	t.Run("controls", func(t *testing.T) {
		t.Parallel()
		a := newRecorder(t)
		p := t.TempDir()
		env := cleanEnv(p, "VSPHERE_HOME="+p, "VSPHERE_CA_CERT="+a.ca)
		soRun(t, p, append(env, "VSPHERE_BASE_URL="+a.url), "auth", "set-credentials", soUser, soPass)
		if out, code := soRun(t, p, append(env, "VSPHERE_BASE_URL="+a.url), vmList...); code != 0 || a.saw(soPass) == 0 {
			t.Errorf("credentials saved for A were not used at A: rc=%d\n%.1500s", code, out)
		}
		own := filepath.Join(t.TempDir(), "own.toml")
		writeFile(t, own, "base_url = \""+a.url+"\"\nusername = \"ro@a.local\"\npassword = \""+soHdrPass+"\"\n")
		if out, code := soRun(t, p, env, "--config", own, "vm", "list", "--json"); code != 0 || a.saw(soHdrPass) == 0 {
			t.Errorf("a config's own password was not used at its own base_url: rc=%d\n%.1500s", code, out)
		}
		home := t.TempDir()
		writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+a.url+"/\"\n\n[headers]\nX-Api-Key = \""+soHdrX+"\"\n")
		if out, code := soRun(t, home, cleanEnv(home, "VSPHERE_HOME="+home, "VSPHERE_CA_CERT="+a.ca, "VSPHERE_USERNAME="+bbUser, "VSPHERE_PASSWORD="+bbPassword, "VSPHERE_BASE_URL="+strings.Replace(a.url, "https://", "HTTPS://", 1)), vmList...); code != 0 || a.saw(soHdrX) == 0 {
			t.Errorf("a saved header was not sent to its own vCenter spelled another way: rc=%d\n%.1500s", code, out)
		}
	})
}
