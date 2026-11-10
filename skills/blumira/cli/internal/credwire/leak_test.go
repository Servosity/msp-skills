// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class the print review left for
// the kit (.review/kit-findings.md), seen failing on the binaries built before
// the kit was wired. Set BLUMIRA_LEAK_BIN_DIR to run them against binaries
// built elsewhere.

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
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
	"sync/atomic"
	"testing"
	"time"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func cliBinary(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		if d := os.Getenv("BLUMIRA_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "blumira-leak-bin-")
		if binErr != nil {
			return
		}
		if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, exe("blumira-cli")), "../../cmd/blumira-cli").CombinedOutput(); err != nil {
			binErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("blumira-cli"))
}

// closedEnv is a closed environment under one sandbox home: no variable of the
// operator's reaches the binary but PATH and what Windows needs to start one.
func closedEnv(home string, extra ...string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1",
		"XDG_CONFIG_HOME=", "XDG_DATA_HOME=", "XDG_STATE_HOME=", "XDG_CACHE_HOME=", "BLUMIRA_HOME=" + filepath.Join(home, "blumira")}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SYSTEMROOT", "SYSTEMDRIVE", "TEMP", "TMP", "COMSPEC", "PATHEXT"} {
			env = append(env, k+"="+os.Getenv(k))
		}
		env = append(env, "APPDATA="+filepath.Join(home, "AppData", "Roaming"), "LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"))
	}
	return append(env, extra...)
}

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run() // exit codes vary by case; the output is judged
	if ctx.Err() != nil {
		t.Fatalf("blumira-cli %s: timed out", strings.Join(args, " "))
	}
	return out.String()
}

// pieces is every 8-character piece of v: the kit's own leak unit.
func pieces(v string) []string {
	var out []string
	for i := 0; i+8 <= len(v); i++ {
		out = append(out, v[i:i+8])
	}
	return out
}

func assertNoPiece(t *testing.T, where, text, secret string) {
	t.Helper()
	for _, p := range pieces(secret) {
		if strings.Contains(text, p) {
			if len(text) > 2000 {
				text = text[:2000]
			}
			t.Errorf("%s carries %q, a piece of a held credential:\n%q", where, p[:4]+"...", text)
			return
		}
	}
}

// treeText is every file under root, concatenated: the store, caches, journal
// and logs a run left behind.
func treeText(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if data, err := os.ReadFile(p); err == nil {
			b.Write(data)
		}
		return nil
	})
	return b.String()
}

// treeTextNoStore is treeText without the SQLite store files, which the kit
// no longer checks (synccpu, owner GO 2026-11-09).
func treeTextNoStore(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || synccpuStoreFile(p) {
			return nil
		}
		if data, err := os.ReadFile(p); err == nil {
			b.Write(data)
		}
		return nil
	})
	return b.String()
}

// Class (kit-findings: auth_login.go summarizeAuthError): the token endpoint's
// error text, which can repeat the posted client secret, was printed as the
// login error. Through the kit the secret is refused to any origin but
// https://auth.blumira.com, so a --auth-url elsewhere never receives it, and
// nothing the endpoint answers can print it.
func TestTokenEndpointEchoNeverPrints(t *testing.T) {
	bin := cliBinary(t)
	const fxID = "blmQ7x2Lr9TzWvHm4Np"
	const fxS = "blm4Kp8Nc3Hd6Yj1FsRw"
	var gotSecret atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if strings.Contains(string(body), fxS) {
			gotSecret.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "access_denied",
			"error_description": "Unauthorized client " + form.Get("client_id") + " with secret " + form.Get("client_secret"),
		})
	}))
	defer srv.Close()
	home := t.TempDir()
	env := closedEnv(home, "BLUMIRA_CLIENT_ID="+fxID, "BLUMIRA_CLIENT_SECRET="+fxS)
	out := run(t, bin, env, "auth", "login", "--auth-url", srv.URL+"/oauth/token", "--no-learn")
	if gotSecret.Load() {
		t.Errorf("the client secret was sent to %s, which is not the Blumira token endpoint", srv.URL)
	}
	assertNoPiece(t, "auth login output", out, fxS)
	assertNoPiece(t, "auth login output", out, fxID)
	assertNoPiece(t, "the files the run left", treeText(t, home), fxS)
}

// Class (kit-findings: blumira_pager.go): an API error's text, which can repeat
// the bearer token, was copied into walk errors, the
// (input: a sub-account device page answering
// {"status":"OK","data":[],"error":"Rejected bearer <token>"}), the
// msp_account_device_coverage.error column and the exposure rows. Through the
// kit, the SQLite driver and stdout/stderr withhold it.
func TestPagerErrorEchoNeverReachesStoreOrOutput(t *testing.T) {
	bin := cliBinary(t)
	const fxT = "blm9Rq2Wx5Lt8Mz3Kv6Pd"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := func(rows []map[string]any) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": rows, "links": map[string]any{"next": nil}, "status": "OK",
				"meta": map[string]any{"page": 1, "page_size": 200, "total_items": len(rows), "total_pages": 1},
			})
		}
		switch r.URL.Path {
		case "/msp/accounts":
			page([]map[string]any{{"account_id": "acct-1", "name": "Client One", "open_findings": 0}})
		case "/org/agents/devices":
			page([]map[string]any{})
		default:
			// The reviewed input: a 200 page whose error field repeats the
			// bearer (the client masks only non-2xx bodies).
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "OK", "data": []any{}, "error": "Rejected bearer " + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
			})
		}
	}))
	defer srv.Close()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	env := closedEnv(home, "BLUMIRA_API_TOKEN="+fxT, "BLUMIRA_BASE_URL="+srv.URL, "BLUMIRA_CA_CERT="+caFile, "SSL_CERT_FILE="+caFile)
	out := run(t, bin, env, "sync", "--resources", "msp,org-agents-devices", "--no-learn")
	out += run(t, bin, env, "exposure", "--json", "--no-learn")
	out += run(t, bin, env, "dc-roster", "--json", "--no-learn")
	assertNoPiece(t, "sync, exposure and dc-roster output", out, fxT)
	assertNoPiece(t, "the files the runs left", treeTextNoStore(t, home), fxT) // synccpu: the local store is not checked
	if !strings.Contains(treeText(t, home), "acct-1") {
		t.Fatalf("the sync stored no MSP account, so this run proves nothing:\n%s", out)
	}
}

// Class (orchestrator 2026-11-03 item 8c): a profiles.json value saved under a
// credential flag's name is a source, held at startup. An unknown command
// repeats its argument, so typing the saved value prints it unless it is held.
func TestProfileCredentialValueHeldAtStartup(t *testing.T) {
	bin := cliBinary(t)
	const fxP = "blm5Gh2Jk8LqVb7Ty3Ze"
	home := t.TempDir()
	cfgDir := filepath.Join(home, "cfg")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	doc := `{"profiles":{"p":{"name":"p","values":{"client-secret":"` + fxP + `"}}}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "profiles.json"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	env := closedEnv(home, "BLUMIRA_CONFIG_DIR="+cfgDir)
	const control = "XXXXXXXXXXXXXXXX"
	if out := run(t, bin, env, control, "--no-learn"); !strings.Contains(out, control) {
		t.Fatalf("an unknown command does not repeat its argument, so this run proves nothing:\n%s", out)
	}
	assertNoPiece(t, "unknown-command output", run(t, bin, env, fxP, "--no-learn"), fxP)
}

// feedback --stdin reads prose, which is no credential: compactStdinBody hands
// its body back after Start, so the local record keeps it (a --stdin body
// is otherwise held whole as a StdinFlags input).
func TestFeedbackStdinIsRecorded(t *testing.T) {
	bin := cliBinary(t)
	home := t.TempDir()
	env := closedEnv(home)
	cmd := exec.Command(bin, "feedback", "--stdin", "--no-learn")
	cmd.Env = env
	cmd.Stdin = strings.NewReader("the overview command reads well\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("feedback --stdin: %v\n%s", err, out)
	}
	if !strings.Contains(treeText(t, home), "the overview command reads well") {
		t.Fatalf("feedback --stdin recorded no body under %s", home)
	}
}
