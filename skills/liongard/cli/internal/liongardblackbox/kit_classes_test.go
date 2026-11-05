// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package liongardblackbox

// The open classes of the factory's Codex round 2 (.review/factory/codex-r2.md),
// each driven on the built binaries with the recorded failing input, and the
// startup-ordering class for a config file that does not parse. Each closes
// through the shared credential kit (handfixes.json credkit-wiring): the kit
// holds every Set-Cookie value and every declared response field and MCP
// argument at any JSON type, matches a held value after every decoding the
// contract names at any depth, judges each unit alone for any 8-character
// piece of a held value (so two halves in two records, two runs or two MCP
// frames are each withheld), and reads every credential file token by token
// before the first byte of output.

import (
	"bytes"
	"encoding/pem"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// classServer is a TLS fake whose answers h decides, with the trust file the
// CLI reads through LIONGARD_CA_CERT.
func classServer(t *testing.T, h http.HandlerFunc) *server {
	t.Helper()
	srv := tlsServer(t, h)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return &server{url: srv.URL + "/api/v1", caFile: ca, headers: map[string]int{}}
}

// withEnv replaces (or adds) each NAME=value in kvs.
func withEnv(env []string, kvs ...string) []string {
	for _, kv := range kvs {
		name, _, _ := strings.Cut(kv, "=")
		env = append(withoutEnv(env, name), kv)
	}
	return env
}

// Class holder-incomplete, Set-Cookie: a session cookie under a name that
// does not look like a credential (sid), with a value of six characters, is
// echoed in the error body.
func TestKitClassSetCookieEchoedInErrorBody(t *testing.T) {
	sv := classServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "sid=K7q9vT; HttpOnly")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"K7q9vT"}`))
	})
	e := newEnv(t, sv, credEnvKey)
	out := e.run(t, "environments", "get", "--json", "--no-learn")
	if !strings.Contains(out, "400") && !strings.Contains(out, "withheld") {
		t.Fatalf("the error case did not run:\n%s", out)
	}
	if strings.Contains(out, "K7q9vT") {
		t.Errorf("the error output repeats the Set-Cookie value:\n%s", out)
	}
}

// Class holder-incomplete, numbers: a number in an array under a credential
// name (tokens), echoed under Name, must not reach the local store.
func TestKitClassNumericCredentialInArrayNeverStored(t *testing.T) {
	sv := classServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/environments") {
			_, _ = w.Write([]byte(`[{"ID":42,"tokens":[58293417],"Name":"58293417"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})
	e := newEnv(t, sv, credEnvKey)
	out := e.run(t, "sync", "--resources", "environments", "--no-learn")
	var dbs int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".db") || strings.Contains(d.Name(), ".db-") {
			dbs++
		}
		if bytes.Contains(data, []byte("58293417")) {
			rel, _ := filepath.Rel(e.home, path)
			t.Errorf("%s holds the numeric credential", rel)
		}
		return nil
	})
	if dbs == 0 {
		t.Fatalf("sync wrote no local store; the case did not run:\n%s", out)
	}
	if strings.Contains(out, "58293417") {
		t.Errorf("sync output repeats the numeric credential:\n%s", out)
	}
}

// Class holder-incomplete, MCP: a numeric value under a credential argument
// name (NumericPassword) in a liongard_execute preview.
func TestKitClassNumericMCPArgumentNeverPreviewed(t *testing.T) {
	sv := classServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	e := newEnv(t, sv, credEnvKey)
	out := e.mcpCalls(t, []map[string]any{{"name": "liongard_execute", "arguments": map[string]any{
		"endpoint_id": "launchpoints.launchpoint", "params": map[string]any{"NumericPassword": 58293418}}}})
	if !strings.Contains(out, `"id":1`) {
		t.Fatalf("the MCP call was not answered:\n%s", out)
	}
	if strings.Contains(out, "58293418") {
		t.Errorf("the MCP preview repeats the numeric credential argument:\n%s", out)
	}
}

// Class secret-in-encoded-output: a held value behind three levels of
// quoted-printable escapes (=3D3D3D is "=" encoded three times).
func TestKitClassDeepQuotedPrintableWithheld(t *testing.T) {
	sv := classServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	e := newEnv(t, sv, credPair)
	e.env = withEnv(e.env, "LIONGARD_ACCESS_KEY_SECRET=QpBoundarySecret=ValueNoDigits")
	out := e.run(t, "QpBoundarySecret=3D3D3DValueNoDigits", "--no-learn")
	if strings.Contains(out, "3D3D3DValueNoDigits") || strings.Contains(out, "QpBoundarySecret") {
		t.Errorf("stderr repeats the encoded secret:\n%s", out)
	}
}

// Class credential-split-across-output-records, stdout: tail prints the two
// halves of a held secret in two polls, with tail's own lines between them.
func TestKitClassSplitAcrossTailPolls(t *testing.T) {
	var mu sync.Mutex
	polls := 0
	sv := classServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		polls++
		n := polls
		mu.Unlock()
		if n%2 == 1 {
			_, _ = w.Write([]byte(`[{"ID":1,"Name":"StreamSplit"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"ID":1,"Name":"SecretABCDEFGH"}]`))
	})
	e := newEnv(t, sv, credPair)
	e.env = withEnv(e.env, "LIONGARD_ACCESS_KEY_SECRET=StreamSplitSecretABCDEFGH")
	cmd := exec.Command(bbCLI, "tail", "environments", "--interval", "1s", "--no-learn")
	cmd.Env, cmd.Dir = e.env, e.home
	var out lockedBuffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := polls
		mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	if runtime.GOOS == "windows" {
		_ = cmd.Process.Kill()
	} else {
		_ = cmd.Process.Signal(os.Interrupt)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	mu.Lock()
	n := polls
	mu.Unlock()
	if n < 2 {
		t.Fatalf("tail polled %d time(s); the case did not run:\n%s", n, out.String())
	}
	got := out.String()
	if strings.Contains(got, "StreamSplit") || strings.Contains(got, "SecretABCDEFGH") {
		t.Errorf("tail printed a half of the held secret:\n%s", got)
	}
}

// Class credential-split-across-output-records, files: two feedback runs
// append the two halves of a held secret to one feedback.jsonl.
func TestKitClassSplitAcrossFeedbackRuns(t *testing.T) {
	sv := classServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	e := newEnv(t, sv, credPair)
	e.env = withEnv(e.env, "LIONGARD_ACCESS_KEY_SECRET=RecordSplitSecretABCDEFGH")
	out := e.run(t, "feedback", "control note one", "--no-learn")
	out += e.run(t, "feedback", "RecordSplit", "--no-learn")
	out += e.run(t, "feedback", "SecretABCDEFGH", "--no-learn")
	var sawControl bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(e.home, path)
		sawControl = sawControl || bytes.Contains(data, []byte("control note one"))
		for _, half := range []string{"RecordSplit", "SecretABCDEFGH"} {
			if bytes.Contains(data, []byte(half)) {
				t.Errorf("%s holds the half %q of the held secret", rel, half)
			}
		}
		return nil
	})
	if !sawControl {
		t.Fatalf("the control feedback was not written; the case did not run:\n%s", out)
	}
	for _, half := range []string{"RecordSplit", "SecretABCDEFGH"} {
		if strings.Contains(out, half) {
			t.Errorf("feedback output repeats the half %q:\n%s", half, out)
		}
	}
}

// Class credential-split-across-output-records, MCP: two protocol errors for
// unknown tool names carry the two halves of a held secret.
func TestKitClassSplitAcrossMCPProtocolErrors(t *testing.T) {
	sv := classServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	e := newEnv(t, sv, credPair)
	e.env = withEnv(e.env, "LIONGARD_ACCESS_KEY_SECRET=ProtocolSplitSecretMNO")
	out := e.mcpPhases(t,
		[]map[string]any{{"name": "ProtocolSplit", "arguments": map[string]any{}}},
		[]map[string]any{{"name": "SecretMNO", "arguments": map[string]any{}}})
	if !strings.Contains(out, `"id":2`) {
		t.Fatalf("the MCP calls were not answered:\n%s", out)
	}
	for _, half := range []string{"ProtocolSplit", "SecretMNO"} {
		if strings.Contains(out, half) {
			t.Errorf("an MCP protocol error repeats the half %q:\n%s", half, out)
		}
	}
}

// Startup ordering: a credential in a config file that does not parse is held
// before the first byte of output, CLI and MCP, so the unknown-command error
// and the unknown-tool error that repeat it are withheld.
func TestKitStartupMalformedConfigHeldFirst(t *testing.T) {
	const key = "MalformedCfgKey-7f3a91"
	sv := classServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	e := newEnv(t, sv, credStored)
	cfg := filepath.Join(e.home, "cfg", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(`api_key = "`+key+`" junk`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = withEnv(e.env, "LIONGARD_CONFIG="+cfg)
	cli := e.run(t, key, "--no-learn")
	mcp := e.mcpCalls(t, []map[string]any{{"name": key, "arguments": map[string]any{}}})
	if !strings.Contains(cli, "unknown command") && !strings.Contains(cli, "withheld") {
		t.Fatalf("the CLI case did not run:\n%s", cli)
	}
	if !strings.Contains(mcp, `"id":1`) {
		t.Fatalf("the MCP case did not run:\n%s", mcp)
	}
	for where, out := range map[string]string{"CLI": cli, "MCP": mcp} {
		if strings.Contains(out, key) {
			t.Errorf("%s startup output repeats the credential from the malformed config:\n%s", where, out)
		}
	}
}
