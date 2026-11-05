// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is one a review named. Set SYNOLOGY_LEAK_BIN_DIR to
// run them against binaries built elsewhere: how the build without the kit was
// seen failing (.review/classes-fail-first.txt).

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/credkit"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/dsm/dsmtest"

	"synology-pp-cli/internal/cli"
	"synology-pp-cli/internal/credwire"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

// Fixtures, built in two parts so a secret scanner does not read them as real.
const (
	acmePw    = "AcmePw-" + "4mN8vB2cDe"
	bravoPw   = "BravoPw-" + "9tR5wL1eFg"
	otherPw   = "OtherPw-" + "3eR4tY5uI6"
	stdinPw   = "StdinPw-" + "5tG6yH7uJ8"
	mcpBearer = "mcp-bearer-4344-" + "0123456789abcdef-long"
)

func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	if runtime.GOOS == "windows" {
		t.Skip("drives Unix config-file modes; the Windows build is vetted and compiled by the gates")
	}
	binOnce.Do(func() {
		if d := os.Getenv("SYNOLOGY_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "synology-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"synology-cli", "../../cmd/synology-cli"}, {"synology-mcp", "../../cmd/synology-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, "synology-cli"), filepath.Join(binDir, "synology-mcp")
}

// nas is a TLS stub of DSM: the kit never sends a credential over plain http,
// loopback included. passwords is every passwd a login carried.
type nas struct {
	url, fingerprint string
	mu               sync.Mutex
	passwords        []string
	requests         []string
	hits             atomic.Int64
}

func (n *nas) sent() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return strings.Join(n.requests, "\n")
}

func (n *nas) logins() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.passwords)
}

// startNAS serves the login and API discovery; answer gets every other call.
func startNAS(t *testing.T, sid string, answer func(w http.ResponseWriter, api, method string)) *nas {
	t.Helper()
	n := &nas{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		_ = r.ParseForm()
		n.mu.Lock()
		n.requests = append(n.requests, r.URL.RawQuery+" "+r.Form.Encode()+" "+r.Header.Get("Cookie"))
		n.mu.Unlock()
		api, method := r.Form.Get("api"), r.Form.Get("method")
		switch {
		case api == "SYNO.API.Auth" && method == "login":
			n.mu.Lock()
			n.passwords = append(n.passwords, r.Form.Get("passwd"))
			n.mu.Unlock()
			fmt.Fprintf(w, `{"success":true,"data":{"sid":%q,"synotoken":"Tok-%s"}}`, sid, sid)
		case api == "SYNO.API.Auth":
			io.WriteString(w, `{"success":true,"data":{}}`)
		default:
			answer(w, api, method)
		}
	}))
	t.Cleanup(srv.Close)
	n.url = srv.URL
	sum := sha256.Sum256(srv.Certificate().Raw)
	n.fingerprint = hex.EncodeToString(sum[:])
	return n
}

func emptyAnswer(w http.ResponseWriter, _, _ string) {
	io.WriteString(w, `{"success":true,"data":{"tasks":[],"total":0,"items":[]}}`)
}

// closedEnv is a closed environment under one sandbox home.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "SYNOLOGY_HOME=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func (n *nas) env(password string) []string {
	return []string{"SYNOLOGY_BASE_URL=" + n.url, "SYNOLOGY_TLS_FINGERPRINT=" + n.fingerprint, "SYNOLOGY_ACCOUNT=admin", "SYNOLOGY_PASSWORD=" + password}
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
	return out.String()
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertAbsent(t *testing.T, label, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("%s shows %q:\n%s", label, s, out)
		}
	}
}

// mcpCall runs one MCP stdio session of tools/call params and returns stdout
// plus stderr, with every call answered.
func mcpCall(t *testing.T, bin string, env []string, args []string, calls ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for i, c := range calls {
		fmt.Fprintf(stdin, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":%s}`+"\n", i+2, c)
	}
	var out strings.Builder
	answered := 0
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for answered < len(calls) && sc.Scan() {
		out.WriteString(sc.Text() + "\n")
		var msg struct {
			ID *int `json:"id"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) == nil && msg.ID != nil && *msg.ID >= 2 {
			answered++
		}
	}
	_ = stdin.Close()
	_, _ = io.Copy(&out, stdout)
	_ = cmd.Wait()
	if answered != len(calls) {
		t.Errorf("MCP answered %d of %d calls:\n%s\n%s", answered, len(calls), out.String(), stderr.String())
	}
	return out.String() + stderr.String()
}

// TestRoundTwoCharacterCodesAreWithheld is the input the review's round 2 named:
// a password as character codes in integral-decimal spelling, in an answer to
// abb_settings, through both the MCP server and the CLI.
func TestRoundTwoCharacterCodesAreWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const pw = "p@ssw0rd"
	for _, note := range []string{
		`[112.0,64.0,115.0,115.0,119.0,48.0,114.0,100.0]`,
		`[112,64,115,115,119,48,114,100]`,
	} {
		n := startNAS(t, "SidR2-8Hn3Kd7Lp2Qx", func(w http.ResponseWriter, api, _ string) {
			fmt.Fprintf(w, `{"success":true,"data":{"note":%s}}`, note)
		})
		home := t.TempDir()
		env := closedEnv(home, append(n.env(pw), "SYNOLOGY_NO_LEARN=1")...)
		mcpOut := mcpCall(t, mcpBin, append(env, "SYNOLOGY_CLI_PATH="+cliBin), nil, `{"name":"abb_settings","arguments":{}}`)
		cliOut := run(t, cliBin, env, "abb", "settings", "--json")
		// The answer really was fetched, or this proves nothing.
		if !slices.Contains(n.logins(), pw) {
			t.Fatalf("the NAS saw logins %q, so no answer came back:\nMCP: %s\nCLI: %s", n.logins(), mcpOut, cliOut)
		}
		// One code from inside the list, in any JSON layout or escaping level.
		for label, out := range map[string]string{"MCP abb_settings": mcpOut, "CLI abb settings": cliOut} {
			flat := strings.NewReplacer(" ", "", "\n", "", "\\n", "", "\t", "", `\`, "").Replace(out)
			if strings.Contains(flat, strings.ReplaceAll(note[:len(note)/2], " ", "")) {
				t.Errorf("%s shows the password as character codes:\n%s", label, out)
			}
			if !strings.Contains(out, "withheld") {
				t.Errorf("%s: want the answer withheld, naming why:\n%s", label, out)
			}
		}
	}
}

// plantedForms are the encodings the fake NAS echoes a value in.
func plantedForms(s string) map[string]string {
	var pct, ent strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&pct, "%%%02X", s[i])
		fmt.Fprintf(&ent, "&#%d;", s[i])
	}
	nested := s
	for i := 0; i < 5; i++ {
		nested = base64.StdEncoding.EncodeToString([]byte(nested))
	}
	rot := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case r >= 'A' && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
	return map[string]string{
		"percent":  pct.String(),
		"entities": ent.String(),
		"base64":   base64.StdEncoding.EncodeToString([]byte(s)),
		"base64x5": nested,
		"hex":      "id-" + hex.EncodeToString([]byte(s)),
		"rot13":    "x " + rot + " y",
	}
}

// TestNoHeldCredentialReachesAnyOutput runs the real CLI and MCP binaries against
// a NAS that echoes every credential the process holds, from every source, in
// many encodings, in string values, field names and error pages. Then it scans
// everything the runs produced: stdout and stderr of every command, every MCP
// response, and every file written except the credential stores themselves.
func TestNoHeldCredentialReachesAnyOutput(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	secrets := map[string]string{
		"env-password":    "abcdefgh",
		"config-password": "CfgPw-4mN8vB2c",  // gitleaks:allow synthetic test fixture
		"creds-password":  "CredPw-9tR5wL1e", // gitleaks:allow synthetic test fixture
		"env-otp":         "482913",
		"env-device":      "EnvDevTok-3hJ8sK2mQ",
		"mcp-token":       "McpTok-Zq8Lm2Nx5Vb7Kc9Ds1Fg3Hj6Wt4Yr0Pe",
		"fleet-password":  "FleetPw-2Wq7Ys4u", // gitleaks:allow synthetic test fixture
		"cached-sid":      "SidCached-1Qa2Ws3Ed",
		"cached-device":   "DidCached-7Uj8Ik9Ol",
		"minted-sid":      "SidMint-8Hn3Kd7Lp2Qx",
		"minted-device":   "DidMint-6Gt4Fr8Ew",
	}
	planted := map[string]any{}
	for name, s := range secrets {
		for form, v := range plantedForms(s) {
			planted[name+"_"+form] = v
		}
	}
	plantedJSON, _ := json.Marshal(planted)
	var srv *nas
	srv = startNAS(t, secrets["minted-sid"], func(w http.ResponseWriter, api, _ string) {
		switch api {
		case "SYNO.Core.SyslogClient.Log":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, "<html><body>upstream error %s</body></html>", plantedJSON)
		case "SYNO.Storage.CGI.Storage":
			data := map[string]any{"volumes": []any{}}
			for name, s := range secrets {
				data["key_"+name] = map[string]any{s: true, base64.StdEncoding.EncodeToString([]byte(s)): 1}
			}
			b, _ := json.Marshal(map[string]any{"success": true, "data": data})
			_, _ = w.Write(b)
		default:
			row := map[string]any{"id": 1, "task_id": 7, "name": "share-" + secrets["env-password"], "task_name": "nightly " + secrets["env-password"]}
			for k, v := range planted {
				row[k] = v
			}
			b, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{
				"tasks": []any{row}, "items": []any{row}, "shares": []any{row}, "groups": []any{row}, "users": []any{row},
				"packages": []any{row}, "result": []any{row}, "list": []any{row}, "logs": []any{row}, "total": 1,
			}})
			_, _ = w.Write(b)
		}
	})
	root := t.TempDir()
	home := filepath.Join(root, "home")
	stores := []string{
		filepath.Join(home, "config", "config.toml"),
		filepath.Join(home, "data", "credentials.toml"),
		filepath.Join(home, "config", "fleet.json"),
	}
	other := "https://10.9.9.9:5001"
	writeFile(t, stores[0], fmt.Sprintf("password = %q\n", secrets["config-password"]))
	writeFile(t, stores[1], fmt.Sprintf("password = %q\n", secrets["creds-password"]))
	writeFile(t, stores[2], fmt.Sprintf(`{"nas":[{"name":"acme","url":%q,"client":"Acme","env_prefix":"SYNOLOGY_ACME","tls_fingerprint":%q}]}`, srv.url, srv.fingerprint))
	writeFile(t, filepath.Join(home, "data", dsm.SessionFileName(other)), fmt.Sprintf(`{"token":%q,"account":"admin","origin":%q,"synotoken":"CachedTok-4Rf5Tg6Yh","device_ids":{"admin":%q},"cookies":[],"saved":%q}`,
		secrets["cached-sid"], dsm.SessionKey(other), secrets["cached-device"], time.Now().UTC().Format(time.RFC3339Nano)))
	env := closedEnv(home, append(srv.env(secrets["env-password"]),
		"SYNOLOGY_ACME_ACCOUNT=admin", "SYNOLOGY_CLI_PATH="+cliBin,
		"SYNOLOGY_OTP_CODE="+secrets["env-otp"], "SYNOLOGY_DEVICE_ID="+secrets["env-device"],
		"SYNOLOGY_MCP_HTTP_TOKEN="+secrets["mcp-token"], "SYNOLOGY_ACME_PASSWORD="+secrets["fleet-password"],
		"SYNOLOGY_CONFIG_DIR="+filepath.Join(home, "config"), "SYNOLOGY_DATA_DIR="+filepath.Join(home, "data"),
		"SYNOLOGY_STATE_DIR="+filepath.Join(home, "state"), "SYNOLOGY_CACHE_DIR="+filepath.Join(home, "cache"))...)
	outputs := map[string]string{}
	for i, args := range [][]string{
		{"auth", "login"},
		{"auth", "status", "--json"},
		{"doctor", "--json"},
		{"abb", "tasks", "--json"},
		{"abb", "settings", "--json"},
		{"storage", "volumes", "--json"},
		{"abb", "results", "--json", "--receipt"},
		{"abb", "tasks", "--json", "--deliver", "file:" + filepath.Join(root, "out", "deliver.json")},
		{"system", "get", "--json"},
		{"log", "--json"},
		{"sync", "--json"},
		{"export", "folder", "--format", "json", "-o", filepath.Join(root, "out", "export.json")},
		{"search", secrets["env-password"], "--json"},
		{"folder", "--json", "--human-friendly"},
		{"fleet", "sync", "--json"},
		{"fleet", "status", "--json"},
		{"abb", "failures", "--json"},
		{"abb", "triage", "7", "--nas", "acme", "--json"},
		{"feedback", "the password is " + secrets["env-password"]},
		{"teach", "--query", "find " + secrets["env-password"], "--resource-type", "folder", "--resource", "1"},
	} {
		outputs[fmt.Sprintf("cli[%d] %s", i, strings.Join(args[:min(2, len(args))], " "))] = run(t, cliBin, env, args...)
	}
	var calls []string
	for _, c := range []string{"abb_settings", "storage_volumes", "abb_tasks", "system_get", "log_list", "folder_list", "user_list", "sync", "fleet_sync", "fleet_status", "fleet_list", "abb_failures", "context", "session_apis"} {
		calls = append(calls, fmt.Sprintf(`{"name":%q,"arguments":{}}`, c))
	}
	calls = append(calls, `{"name":"sql","arguments":{"query":"SELECT data FROM resources"}}`, `{"name":"search","arguments":{"query":"share"}}`, `{"name":"abb_triage","arguments":{"task":"7","nas":"acme"}}`)
	outputs["mcp"] = mcpCall(t, mcpBin, env, nil, calls...)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || slices.Contains(stores, p) {
			return nil
		}
		base := strings.ToLower(d.Name())
		if strings.Contains(base, "session") { // the session cache is a credential store
			return nil
		}
		if data, rerr := os.ReadFile(p); rerr == nil {
			outputs["file "+p] = string(data)
		}
		return nil
	})
	if !slices.Contains(srv.logins(), secrets["env-password"]) && !slices.Contains(srv.logins(), secrets["fleet-password"]) {
		t.Fatalf("no login reached the NAS, so nothing was echoed and this proves nothing: %v", outputs)
	}
	// The oracle is independent of the code under test: every planted form,
	// byte for byte, and the value itself in any letter case.
	var leaks []string
	for name, s := range secrets {
		for out, data := range outputs {
			if strings.Contains(strings.ToLower(data), strings.ToLower(s)) {
				leaks = append(leaks, fmt.Sprintf("%s: %s literal", out, name))
			}
			for form, v := range plantedForms(s) {
				if strings.Contains(data, v) {
					leaks = append(leaks, fmt.Sprintf("%s: %s %s", out, name, form))
				}
			}
		}
	}
	sort.Strings(leaks)
	if len(leaks) > 0 {
		t.Errorf("held credentials reached output (%d):\n%s", len(leaks), strings.Join(leaks, "\n"))
	}
}

// TestMalformedConfigHeldBeforeAnythingPrints: a config that does not parse still
// has its password held, before the parse error can quote it.
func TestMalformedConfigHeldBeforeAnythingPrints(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	home := t.TempDir()
	cfg := filepath.Join(home, "bad.toml")
	const pw = "MalformedPw-6Yh3Kd9Lq"
	writeFile(t, cfg, "base_url = \"https://127.0.0.1:1\"\npassword = \""+pw+"\npassword = \""+pw+"\" = broken\n")
	env := closedEnv(home, "SYNOLOGY_CONFIG="+cfg, "SYNOLOGY_NO_LEARN=1", "SYNOLOGY_CLI_PATH="+cliBin)
	for _, args := range [][]string{{"doctor", "--json"}, {"abb", "tasks", "--json"}, {"auth", "status"}, {"--config", cfg, "system", "dsm"}} {
		assertAbsent(t, strings.Join(args, " "), run(t, cliBin, env, args...), pw)
	}
	assertAbsent(t, "MCP", mcpCall(t, mcpBin, env, nil, `{"name":"abb_tasks","arguments":{}}`), pw)

	// The input that tells: a registered NAS needs nothing from the broken file, so
	// the run goes on, and that NAS echoes the password the file carries.
	n := startNAS(t, "SidBad-1Qa2Ws3Ed4Rf", func(w http.ResponseWriter, _, _ string) {
		fmt.Fprintf(w, `{"success":true,"data":{"tasks":[{"task_id":7,"task_name":"nightly %s","total":1}],"total":1}}`, pw)
	})
	writeFile(t, filepath.Join(home, "config", "fleet.json"), fmt.Sprintf(`{"nas":[{"name":"acme","url":%q,"env_prefix":"SYNOLOGY_ACME","tls_fingerprint":%q}]}`, n.url, n.fingerprint))
	nasEnv := append(slices.Clone(env), "SYNOLOGY_CONFIG_DIR="+filepath.Join(home, "config"), "SYNOLOGY_ACME_ACCOUNT=admin", "SYNOLOGY_ACME_PASSWORD="+acmePw)
	out := run(t, cliBin, nasEnv, "abb", "triage", "7", "--nas", "acme", "--json")
	if !slices.Contains(n.logins(), acmePw) {
		t.Fatalf("a broken config kept a registered NAS from its login, so nothing was echoed:\n%s", out)
	}
	assertAbsent(t, "abb triage", out, pw)
}

// twoNAS registers two NAS units, each pinned and with its own password.
func twoNAS(t *testing.T, answer func(w http.ResponseWriter, api, method string)) (a, b *nas, home string, env []string) {
	t.Helper()
	a = startNAS(t, "SidA-1Qa2Ws3Ed4Rf", answer)
	b = startNAS(t, "SidB-5Tg6Yh7Uj8Ik", answer)
	home = t.TempDir()
	writeFile(t, filepath.Join(home, "config", "fleet.json"), fmt.Sprintf(
		`{"nas":[{"name":"acme","url":%q,"client":"Acme","env_prefix":"SYNOLOGY_ACME","tls_fingerprint":%q},{"name":"bravo","url":%q,"client":"Bravo","env_prefix":"SYNOLOGY_BRAVO","tls_fingerprint":%q}]}`,
		a.url, a.fingerprint, b.url, b.fingerprint))
	env = closedEnv(home, "SYNOLOGY_NO_LEARN=1", "SYNOLOGY_CONFIG_DIR="+filepath.Join(home, "config"),
		"SYNOLOGY_ACME_ACCOUNT=admin", "SYNOLOGY_ACME_PASSWORD="+acmePw, "SYNOLOGY_BRAVO_ACCOUNT=admin", "SYNOLOGY_BRAVO_PASSWORD="+bravoPw)
	return a, b, home, env
}

// TestFleetSyncRunsOneProcessPerNAS: the kit lets a credential go to one origin
// per process, so a fleet sync runs each NAS in its own, and each NAS gets its
// own password and no other's.
func TestFleetSyncRunsOneProcessPerNAS(t *testing.T) {
	cliBin, _ := binaries(t)
	a, b, _, env := twoNAS(t, emptyAnswer)
	out := run(t, cliBin, env, "fleet", "sync", "--json")
	if got := a.logins(); !slices.Equal(got, []string{acmePw}) {
		t.Errorf("acme saw logins %q:\n%s", got, out)
	}
	if got := b.logins(); !slices.Equal(got, []string{bravoPw}) {
		t.Errorf("bravo saw logins %q:\n%s", got, out)
	}
	for _, want := range []string{`"nas": "acme"`, `"nas": "bravo"`} {
		if !strings.Contains(out, want) {
			t.Errorf("fleet sync has no row %s:\n%s", want, out)
		}
	}
	assertAbsent(t, "fleet sync", out, acmePw, bravoPw, "credkit:")
	status := run(t, cliBin, env, "fleet", "status", "--json")
	for _, want := range []string{"acme", "bravo"} {
		if !strings.Contains(status, want) {
			t.Errorf("fleet status lost %s, which a NAS process committed:\n%s", want, status)
		}
	}
}

// TestSoleNASCommandReachesItsNAS: a command that needs no --nas when one NAS is
// registered reruns for that NAS, since it is not the NAS this process started for.
func TestSoleNASCommandReachesItsNAS(t *testing.T) {
	cliBin, _ := binaries(t)
	n := startNAS(t, "SidSole-1Qa2Ws3Ed4Rf", emptyAnswer)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "fleet.json"), fmt.Sprintf(`{"nas":[{"name":"acme","url":%q,"env_prefix":"SYNOLOGY_ACME","tls_fingerprint":%q}]}`, n.url, n.fingerprint))
	env := closedEnv(home, "SYNOLOGY_NO_LEARN=1", "SYNOLOGY_CONFIG_DIR="+filepath.Join(home, "config"), "SYNOLOGY_ACME_ACCOUNT=admin", "SYNOLOGY_ACME_PASSWORD="+acmePw)
	out := run(t, cliBin, env, "abb", "triage", "7", "--json")
	if got := n.logins(); !slices.Equal(got, []string{acmePw}) {
		t.Errorf("the NAS saw logins %q:\n%s", got, out)
	}
	assertAbsent(t, "abb triage", out, acmePw, "credkit:")
}

// TestOneNASProcessSendsNoCredentialToAnother: a process started for bravo holds
// acme's password and has nowhere to send it; its only origin is bravo.
func TestOneNASProcessSendsNoCredentialToAnother(t *testing.T) {
	cliBin, _ := binaries(t)
	a, b, _, env := twoNAS(t, emptyAnswer)
	out := run(t, cliBin, env, "fleet", "sync", "--nas", "bravo", "--json")
	if a.hits.Load() != 0 {
		t.Errorf("a process for bravo reached acme %d times", a.hits.Load())
	}
	if got := b.logins(); !slices.Equal(got, []string{bravoPw}) {
		t.Errorf("bravo saw logins %q:\n%s", got, out)
	}
}

// TestOneTimeCodeEqualToAResultID: a held one-time code that equals a number in an
// answer withholds that answer (documented: "then unset the code"), never parses
// it short or prints it, and the same call works once the code is unset.
func TestOneTimeCodeEqualToAResultID(t *testing.T) {
	cliBin, _ := binaries(t)
	const otp = "482913"
	n := startNAS(t, "SidOtp-1Qa2Ws3Ed4Rf", func(w http.ResponseWriter, _, _ string) {
		fmt.Fprintf(w, `{"success":true,"data":{"result_id":%s,"tasks":[],"total":0}}`, otp)
	})
	home := t.TempDir()
	env := closedEnv(home, append(n.env("OtpPw-11111111Aa"), "SYNOLOGY_NO_LEARN=1")...)
	out := run(t, cliBin, append(env, "SYNOLOGY_OTP_CODE="+otp), "abb", "settings", "--json")
	assertAbsent(t, "with the code set", out, otp)
	if !strings.Contains(out, "withheld") {
		t.Errorf("with the code set: want the answer withheld, naming why:\n%s", out)
	}
	if out := run(t, cliBin, env, "abb", "settings", "--json"); !strings.Contains(out, otp) {
		t.Errorf("after the code is unset the answer should print:\n%s", out)
	}
}

// TestChildGetsItsCredentials: the MCP server's CLI child gets no credential in its
// argv or environment (the kit hands it over on its own pipe), and still logs in.
func TestChildGetsItsCredentials(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const pw = "ChildPw-4142aBcD"
	n := startNAS(t, "SidChild-1Qa2Ws3Ed4Rf", emptyAnswer)
	home := t.TempDir()
	env := closedEnv(home, append(n.env(pw), "SYNOLOGY_NO_LEARN=1", "SYNOLOGY_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, nil, `{"name":"fleet_sync","arguments":{}}`)
	assertAbsent(t, "fleet_sync", out, pw, "not started", "withheld from child")
	if got := n.logins(); !slices.Contains(got, pw) {
		t.Errorf("the child sent logins %q, want the handed password:\n%s", got, out)
	}
}

// TestPlainHTTPSendsNoCredential: any http, loopback included, gets no credential.
func TestPlainHTTPSendsNoCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const pw = "PlainPw-4142aBcD"
	var sawPassword atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("passwd") != "" {
			sawPassword.Store(true)
		}
		if !dsmtest.ServeAPIInfo(w, r) {
			io.WriteString(w, `{"success":true,"data":{"sid":"s","synotoken":"t"}}`)
		}
	}))
	defer srv.Close()
	env := closedEnv(t.TempDir(), "SYNOLOGY_BASE_URL="+srv.URL, "SYNOLOGY_ACCOUNT=admin", "SYNOLOGY_PASSWORD="+pw, "SYNOLOGY_NO_LEARN=1")
	out := run(t, cliBin, env, "abb", "tasks", "--json")
	if sawPassword.Load() {
		t.Errorf("a plain http address received the password:\n%s", out)
	}
	assertAbsent(t, "abb tasks", out, pw)
	if !strings.Contains(out, "https") {
		t.Errorf("the refusal should say a credential needs https:\n%s", out)
	}
}

// TestExplicitConfigDecidesTheOrigin: a discovered default config names one NAS and
// --config (or SYNOLOGY_CONFIG) another. The credential reaches only the one the
// program uses; the default's address is no allowed origin, and its values are held.
func TestExplicitConfigDecidesTheOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const pw, defaultPw = "ExplicitPw-7hJ3kL9mNp", "DefaultPw-2sD6fG8hJk"
	for _, how := range []string{"--config", "SYNOLOGY_CONFIG"} {
		t.Run(how, func(t *testing.T) {
			defaultNAS := startNAS(t, "SidDefault-1Qa2Ws3Ed4Rf", emptyAnswer)
			chosen := startNAS(t, "SidChosen-5Tg6Yh7Uj8Ik", emptyAnswer)
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "config.toml"),
				fmt.Sprintf("base_url = %q\naccount = \"admin\"\npassword = %q\n", defaultNAS.url, defaultPw))
			cfg := filepath.Join(t.TempDir(), "other.toml")
			writeFile(t, cfg, fmt.Sprintf("base_url = %q\naccount = \"admin\"\n", chosen.url))
			env := closedEnv(home, "SYNOLOGY_TLS_FINGERPRINT="+chosen.fingerprint, "SYNOLOGY_PASSWORD="+pw, "SYNOLOGY_NO_LEARN=1")
			args := []string{"abb", "tasks", "--json"}
			if how == "--config" {
				args = append(args, "--config", cfg)
			} else {
				env = append(env, "SYNOLOGY_CONFIG="+cfg)
			}
			out := run(t, cliBin, env, args...)
			if got := chosen.logins(); !slices.Equal(got, []string{pw}) {
				t.Errorf("the NAS the config names saw logins %q:\n%s", got, out)
			}
			if defaultNAS.hits.Load() != 0 {
				t.Errorf("the discovered default's NAS was reached %d times", defaultNAS.hits.Load())
			}
			assertAbsent(t, how, out, pw, defaultPw)
		})
	}
}

// TestDiscoveredDefaultValuesAreHeld: a config this run does not read still has its
// values held, so a NAS answer that echoes one is withheld.
func TestDiscoveredDefaultValuesAreHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const defaultPw = "DefaultHeld-8uI7yT6rEw"
	n := startNAS(t, "SidHeld-1Qa2Ws3Ed4Rf", func(w http.ResponseWriter, _, _ string) {
		fmt.Fprintf(w, `{"success":true,"data":{"tasks":[{"task_name":%q}],"total":1}}`, defaultPw)
	})
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), fmt.Sprintf("password = %q\n", defaultPw))
	cfg := filepath.Join(t.TempDir(), "other.toml")
	writeFile(t, cfg, fmt.Sprintf("base_url = %q\naccount = \"admin\"\n", n.url))
	env := closedEnv(home, "SYNOLOGY_TLS_FINGERPRINT="+n.fingerprint, "SYNOLOGY_PASSWORD="+otherPw, "SYNOLOGY_NO_LEARN=1")
	out := run(t, cliBin, env, "abb", "tasks", "--json", "--config", cfg)
	assertAbsent(t, "a value from a config this run does not read", out, defaultPw)
}

// TestRunProfileStore: a profile can carry a URL, a credential-named value or a key
// no command declares. A URL's password and credential query values, and a
// credential-named value, are held from startup; a key that is not a real flag never
// reaches a run; and an ordinary profile withholds nothing.
func TestRunProfileStore(t *testing.T) {
	cliBin, _ := binaries(t)
	const urlPw, queryTok, namedTok = "ProfUrlPw-4aS5dF6gH7", "ProfQueryTok-8jK9lZ0xC1", "ProfNamedTok-2vB3nM4qW5"
	n := startNAS(t, "SidProf-1Qa2Ws3Ed4Rf", func(w http.ResponseWriter, _, _ string) {
		fmt.Fprintf(w, `{"success":true,"data":{"tasks":[{"task_name":"held %s","note":"%s %s"}],"total":1}}`, urlPw, queryTok, namedTok)
	})
	home := t.TempDir()
	store := fmt.Sprintf(`{"profiles":{"acme":{"name":"acme","values":{"json":"true","limit":"50",
		"base-url":"https://admin:%s@nas.example.net:5001/?sid=%s","api-token":%q,"nosuchflag":"NotAFlagValue-9"}}}}`, urlPw, queryTok, namedTok)
	writeFile(t, filepath.Join(home, "config", "profiles.json"), store)
	env := closedEnv(home, append(n.env("ProfPw-6eR7tY8uI9"), "SYNOLOGY_NO_LEARN=1")...)
	out := run(t, cliBin, env, "abb", "tasks", "--json")
	assertAbsent(t, "a profile's credentials", out, urlPw, queryTok, namedTok)
	if !strings.Contains(out, "withheld") {
		t.Errorf("want the answer withheld, naming why:\n%s", out)
	}
	shown := run(t, cliBin, env, "profile", "show", "acme", "--json")
	assertAbsent(t, "profile show", shown, urlPw, queryTok, namedTok, "NotAFlagValue-9", "nosuchflag")
	if got := n.logins(); len(got) == 0 {
		t.Errorf("the profile store refused the run:\n%s", out)
	}

	plain := startNAS(t, "SidPlainProf-1Qa2Ws3Ed4", emptyAnswer)
	home2 := t.TempDir()
	writeFile(t, filepath.Join(home2, "config", "profiles.json"), `{"profiles":{"acme":{"name":"acme","values":{"json":"true","limit":"50","nosuchflag":"NotAFlagValue-9"}}}}`)
	env2 := closedEnv(home2, append(plain.env("ProfPw-6eR7tY8uI9"), "SYNOLOGY_NO_LEARN=1")...)
	out = run(t, cliBin, env2, "abb", "tasks", "--json")
	if strings.Contains(out, "credkit:") {
		t.Errorf("an ordinary profile store must withhold nothing:\n%s", out)
	}
	shown = run(t, cliBin, env2, "profile", "show", "acme", "--json")
	assertAbsent(t, "a key no command declares", shown, "NotAFlagValue-9", "nosuchflag")
	if !strings.Contains(shown, "limit") {
		t.Errorf("an allowed value should survive the filter:\n%s", shown)
	}
}

// TestAnotherNASCacheIsSentNowhere: the data directory is shared, so a process bound
// to one NAS can read another NAS's session cache and any config it does not use. It
// holds them and sends them nowhere, even as a value the operator's filter names.
func TestAnotherNASCacheIsSentNowhere(t *testing.T) {
	cliBin, _ := binaries(t)
	const bravoSID, unusedPw = "BravoSid-" + "Ab12Cd34Ef56", "UnusedCfg-" + "9kL8jH7gF6"
	home := t.TempDir()
	acme := startNAS(t, "SidAcme-1Qa2Ws3Ed4Rf", emptyAnswer)
	writeFile(t, filepath.Join(home, "data", "session-https-bravo.example.net-5001-0a1b2c3d.json"),
		fmt.Sprintf(`{"token":%q,"account":"admin","origin":"https://bravo.example.net:5001","cookies":[]}`, bravoSID))
	writeFile(t, filepath.Join(home, "config", "config.toml"), fmt.Sprintf("password = %q\n", unusedPw))
	used := filepath.Join(home, "used.toml")
	writeFile(t, used, fmt.Sprintf("base_url = %q\naccount = \"admin\"\n", acme.url))
	env := closedEnv(home, append(acme.env(acmePw), "SYNOLOGY_NO_LEARN=1", "SYNOLOGY_CONFIG="+used)...)
	for _, v := range []string{bravoSID, unusedPw} {
		run(t, cliBin, env, "abb", "tasks", "--data-source", "live", "--no-cache", "--filter", fmt.Sprintf(`{"task_name":%q}`, v))
		assertAbsent(t, "what acme received", acme.sent(), v)
	}
}

// TestOwnSessionCacheStillReachesItsNAS is the other direction: a cache recorded for
// this NAS is bound to it, so the next run reuses the session and logs in no more.
func TestOwnSessionCacheStillReachesItsNAS(t *testing.T) {
	cliBin, _ := binaries(t)
	const sid = "SidOwn-" + "7Yh6Tg5Rf4Ed"
	n := startNAS(t, sid, emptyAnswer)
	home := t.TempDir()
	env := closedEnv(home, append(n.env(acmePw), "SYNOLOGY_NO_LEARN=1")...)
	run(t, cliBin, env, "auth", "login")
	if got := n.logins(); !slices.Equal(got, []string{acmePw}) {
		t.Fatalf("logins after auth login: %q", got)
	}
	out := run(t, cliBin, env, "abb", "tasks", "--data-source", "live", "--no-cache", "--json")
	if !strings.Contains(n.sent(), sid) {
		t.Errorf("the second run did not send this NAS its cached session:\n%s\nsent:\n%s", out, n.sent())
	}
	if strings.Contains(out, "credkit: withheld") {
		t.Errorf("the NAS's own cached session was withheld:\n%s", out)
	}
}

// TestSessionStoreHoldsEveryCredential: --help names "synology-cli", so a session
// store that holds that value withholds help. Each store carries it in a form a
// reader of the file could miss.
func TestSessionStoreHoldsEveryCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const v = "synology-cli"
	for name, body := range map[string]string{
		"first of a duplicate key":    `{"token":"` + v + `","token":"OtherValue-Ab12Cd34"}`,
		"last of a duplicate key":     `{"token":"OtherValue-Ab12Cd34","token":"` + v + `"}`,
		"any cookie's value":          `{"token":"x-Ab12Cd34","cookies":[{"url":"https://nas:5001/","name":"nas_pref","value":"` + v + `","path":"/"}]}`,
		"a duplicate cookie value":    `{"cookies":[{"name":"nas_pref","value":"` + v + `","value":"OtherValue-Ab12Cd34"}]}`,
		"a torn file, a short value":  `{"token":"abb","cookies":[`,
		"a torn file, an open string": `{"token":"abb`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "data", "session.json"), body)
			out := run(t, cliBin, closedEnv(home, "SYNOLOGY_NO_LEARN=1"), "abb", "--help")
			if strings.Contains(out, "abb") && !strings.Contains(out, "credkit: withheld") && !strings.Contains(out, "credkit: refused") {
				t.Errorf("help was printed with the store's value unheld:\n%s", out)
			}
		})
	}
	// Numbers count: a stored number a NAS answer repeats is withheld.
	for name, body := range map[string]string{
		"a numeric credential":  `{"token":20261103}`,
		"a torn file, a number": `{"token":20261103,"cookies":[`,
	} {
		t.Run(name, func(t *testing.T) {
			n := startNAS(t, "SidNum-1Qa2Ws3Ed4Rf", func(w http.ResponseWriter, _, _ string) {
				io.WriteString(w, `{"success":true,"data":{"tasks":[{"task_id":20261103}],"total":1}}`)
			})
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "data", "session.json"), body)
			out := run(t, cliBin, closedEnv(home, append(n.env(acmePw), "SYNOLOGY_NO_LEARN=1")...), "abb", "tasks", "--data-source", "live", "--no-cache", "--json")
			assertAbsent(t, name, out, "20261103")
		})
	}
	t.Run("a remember-me cookie refuses nothing", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, "data", "session.json"), `{"cookies":[{"url":"https://nas:5001/","name":"stay_login","value":"1","path":"/"}]}`)
		if out := run(t, cliBin, closedEnv(home, "SYNOLOGY_NO_LEARN=1"), "abb", "--help"); !strings.Contains(out, "abb") || strings.Contains(out, "credkit:") {
			t.Errorf("a normal session cache kept help from printing:\n%s", out)
		}
	})
}

// TestActiveRegistryPrefixIsHeld: two registries name one NAS with different
// prefixes. The one the run resolves to decides, and both are held.
func TestActiveRegistryPrefixIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	old, active := filepath.Join(home, "old"), filepath.Join(home, "active")
	writeFile(t, filepath.Join(old, "synology-cli", "fleet.json"), `{"nas":[{"name":"acme","url":"https://stale.example.net:5001","env_prefix":"OLD_ACME"}]}`)
	writeFile(t, filepath.Join(active, "fleet.json"), `{"nas":[{"name":"acme","url":"https://active.example.net:5001","env_prefix":"ACTIVE_ACME"}]}`)
	for _, name := range []string{"ACTIVE_ACME_PASSWORD", "OLD_ACME_PASSWORD"} {
		env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "XDG_CONFIG_HOME=" + old, "SYNOLOGY_CONFIG_DIR=" + active, "TMPDIR=" + os.TempDir(), "NO_COLOR=1", "SYNOLOGY_NO_LEARN=1", name + "=synology-cli"}
		assertAbsent(t, "help with "+name+" set", run(t, cliBin, env, "--help"), "synology-cli")
	}
}

// TestStoredLoginBelongsToTheNASItsFileNames: SYNOLOGY_BASE_URL replaces the address,
// but a file's stored password stays with the NAS that file names.
func TestStoredLoginBelongsToTheNASItsFileNames(t *testing.T) {
	cliBin, _ := binaries(t)
	const stored = "StoredPw-" + "6yH7uJ8iK9"
	for _, how := range []string{"--config", "default config"} {
		t.Run(how, func(t *testing.T) {
			a := startNAS(t, "SidA-1Qa2Ws3Ed4Rf", emptyAnswer)
			b := startNAS(t, "SidB-5Tg6Yh7Uj8Ik", emptyAnswer)
			home := t.TempDir()
			body := fmt.Sprintf("base_url = %q\naccount = \"admin\"\npassword = %q\n", b.url, stored)
			args := []string{"abb", "tasks", "--json"}
			if how == "--config" {
				cfg := filepath.Join(home, "b.toml")
				writeFile(t, cfg, body)
				args = append(args, "--config", cfg)
			} else {
				writeFile(t, filepath.Join(home, "config", "config.toml"), body)
			}
			out := run(t, cliBin, closedEnv(home, "SYNOLOGY_BASE_URL="+a.url, "SYNOLOGY_TLS_FINGERPRINT="+a.fingerprint, "SYNOLOGY_NO_LEARN=1"), args...)
			assertAbsent(t, "what the other NAS received", a.sent(), stored)
			assertAbsent(t, "output", out, stored)
			// The other direction: the address the file names gets its password.
			home2 := t.TempDir()
			writeFile(t, filepath.Join(home2, "config", "config.toml"), body)
			out = run(t, cliBin, closedEnv(home2, "SYNOLOGY_BASE_URL="+b.url, "SYNOLOGY_TLS_FINGERPRINT="+b.fingerprint, "SYNOLOGY_NO_LEARN=1"), "abb", "tasks", "--json")
			if got := b.logins(); !slices.Equal(got, []string{stored}) {
				t.Errorf("the NAS the file names saw logins %q:\n%s", got, out)
			}
		})
	}
}

// TestLogoutOfAnotherNASKeepsTheStoredLogin: --nas names one NAS, so the config's
// login, which belongs to another, stays. Logging out of its own NAS clears it.
func TestLogoutOfAnotherNASKeepsTheStoredLogin(t *testing.T) {
	cliBin, _ := binaries(t)
	const stored = "StoredPw-" + "2wS3eD4rF5"
	a, _, home, env := twoNAS(t, emptyAnswer)
	cfg := filepath.Join(home, "config", "config.toml")
	writeFile(t, cfg, fmt.Sprintf("base_url = %q\naccount = \"admin\"\npassword = %q\n", a.url, stored))
	out := run(t, cliBin, env, "auth", "logout", "--nas", "bravo")
	if data, _ := os.ReadFile(cfg); !strings.Contains(string(data), stored) {
		t.Errorf("logging out of bravo cleared acme's stored login:\n%s", out)
	}
	out = run(t, cliBin, env, "auth", "logout", "--nas", "acme")
	if data, _ := os.ReadFile(cfg); strings.Contains(string(data), stored) {
		t.Errorf("logging out of acme kept its stored login:\n%s", out)
	}
}

// TestNASProcessNeverStartsAnother: a process started for one NAS that finds it is not
// bound stops. It never starts a further process, whatever made the binding disagree.
func TestNASProcessNeverStartsAnother(t *testing.T) {
	cliBin, _ := binaries(t)
	n := startNAS(t, "SidOne-1Qa2Ws3Ed4Rf", emptyAnswer)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "fleet.json"), fmt.Sprintf(`{"nas":[{"name":"acme","url":%q,"env_prefix":"SYNOLOGY_ACME","tls_fingerprint":%q}]}`, n.url, n.fingerprint))
	env := closedEnv(home, "SYNOLOGY_NO_LEARN=1", "SYNOLOGY_CONFIG_DIR="+filepath.Join(home, "config"), "SYNOLOGY_ACME_ACCOUNT=admin", "SYNOLOGY_ACME_PASSWORD="+acmePw)
	run(t, cliBin, env, "abb", "triage", "8", "--json")
	if n.hits.Load() == 0 {
		t.Fatalf("the first process did not start one for its NAS")
	}
	first := n.hits.Load()
	out := run(t, cliBin, append(env, "PP_NAS_PROCESS=1"), "abb", "triage", "8", "--json")
	if got := n.hits.Load(); got != first {
		t.Errorf("a NAS process started another (%d more requests):\n%s", got-first, out)
	}
}

// TestRepeatedNASFlagRunsOnce: the MCP server turns a JSON array into repeated --nas.
func TestRepeatedNASFlagRunsOnce(t *testing.T) {
	cliBin, _ := binaries(t)
	a, b, _, env := twoNAS(t, emptyAnswer)
	out := run(t, cliBin, env, "abb", "triage", "8", "--nas", "acme", "--nas", "bravo", "--json")
	if a.hits.Load() != 0 {
		t.Errorf("acme was reached %d times:\n%s", a.hits.Load(), out)
	}
	if got := b.logins(); len(got) > 1 {
		t.Errorf("bravo saw %d logins, so more than one process ran:\n%s", len(got), out)
	}
}

// TestFleetAddCheckLogsInToTheNASItNames: the documented way to check a registered NAS.
func TestFleetAddCheckLogsInToTheNASItNames(t *testing.T) {
	cliBin, _ := binaries(t)
	_, b, _, env := twoNAS(t, emptyAnswer)
	out := run(t, cliBin, env, "fleet", "add", "bravo", "--check")
	if got := b.logins(); !slices.Equal(got, []string{bravoPw}) {
		t.Errorf("bravo saw logins %q:\n%s", got, out)
	}
}

// TestProfileBaseURLIsASource: every query and fragment value of a profile's base-url is
// held, however its name is spelled or whatever it is called, as the kit holds a base
// URL's, and it cannot print through the profile's own description.
func TestProfileBaseURLIsASource(t *testing.T) {
	cliBin, _ := binaries(t)
	for name, rawURL := range map[string]string{
		"encoded name":         "https://nas/?%70assword=",
		"plain name":           "https://nas/?password=",
		"any name":             "https://nas/?x=",
		"fragment":             "https://nas/#token=",
		"encoded value":        "https://nas/?x=%56",
		"literal ? in a value": "https://nas/?%70assword=FirstHalf42?",
	} {
		t.Run(name, func(t *testing.T) {
			const secret = "VelvetQ7m9z"
			value := secret
			if strings.HasSuffix(rawURL, "%56") { // %56 is V, the secret's first letter
				value = secret[1:]
			}
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "profiles.json"), fmt.Sprintf(
				`{"profiles":{"x":{"name":"x","description":%q,"values":{"base-url":%q}}}}`, secret, rawURL+value))
			env := closedEnv(home, "SYNOLOGY_NO_LEARN=1")
			out := run(t, cliBin, env, "profile", "show", "x", "--json")
			assertAbsent(t, "profile show", out, secret)
		})
	}
}

// TestURLPasswordsAreHeld: a proxy variable Go accepts with no scheme, and a hand-edited
// registry address, both carry a password a NAS answer could repeat.
func TestURLPasswordsAreHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const proxyPw, regPw = "ProxyBare-" + "5tG6yH7uJ8", "RegUrlPw-" + "5tG6yH7uJ9"
	n := startNAS(t, "SidEcho-1Qa2Ws3Ed4Rf", func(w http.ResponseWriter, _, _ string) {
		fmt.Fprintf(w, `{"success":true,"data":{"tasks":[{"task_name":"%s %s"}],"total":1}}`, proxyPw, regPw)
	})
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "fleet.json"), `{"nas":[{"name":"zz","url":"https://u:`+regPw+`@127.0.0.1:1","env_prefix":"SYNOLOGY_ZZ"}]}`)
	env := closedEnv(home, append(n.env(acmePw), "SYNOLOGY_NO_LEARN=1", "SYNOLOGY_CONFIG_DIR="+filepath.Join(home, "config"), "HTTPS_PROXY=u:"+proxyPw+"@127.0.0.1:9", "NO_PROXY=127.0.0.1")...)
	out := run(t, cliBin, env, "abb", "tasks", "--json", "--no-cache")
	assertAbsent(t, "output", out, proxyPw, regPw)
}

// connectStub is an HTTP CONNECT proxy to target; auths gets each Proxy-Authorization.
func connectStub(target string, auths *[]string, mu *sync.Mutex) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*auths = append(*auths, r.Header.Get("Proxy-Authorization"))
		mu.Unlock()
		up, err := net.Dial("tcp", target)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer up.Close()
		w.WriteHeader(http.StatusOK)
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = rw.Flush()
		// Hijack flushes the 200 first, so the server's background read can already hold the
		// client's first byte. It is in rw, and reading conn alone would lose it.
		go func() { _, _ = io.Copy(up, rw) }()
		_, _ = io.Copy(conn, up)
	})
}

// TestProxyPassword: a proxy's own password is the proxy's. Over TLS it reaches that proxy
// and the login goes through. The kit sends nothing held over plain http, so a plain http
// proxy with a password fails closed, and one without a password is an ordinary hop.
func TestProxyPassword(t *testing.T) {
	cliBin, _ := binaries(t)
	const pw, proxyPw = "ProxiedPw-" + "4142aBcD", "ProxyPw-" + "5tG6yH7uJ8"
	n := startNAS(t, "SidProxy-1Qa2Ws3Ed4Rf", emptyAnswer)
	target := strings.TrimPrefix(n.url, "https://")
	_, port, _ := net.SplitHostPort(target)
	var mu sync.Mutex
	var auths []string
	plain := httptest.NewServer(connectStub(target, &auths, &mu))
	defer plain.Close()
	secure := httptest.NewTLSServer(connectStub(target, &auths, &mu))
	defer secure.Close()
	home := t.TempDir()
	ca := filepath.Join(home, "ca.pem")
	writeFile(t, ca, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: secure.Certificate().Raw})))
	// A loopback address would skip the proxy. example.com is the name in httptest's certificate.
	base := []string{"SYNOLOGY_BASE_URL=https://nas.client.test:" + port, "SYNOLOGY_ACCOUNT=admin", "SYNOLOGY_PASSWORD=" + pw, "SYNOLOGY_NO_LEARN=1"}
	for _, c := range []struct {
		name, proxy string
		trust       []string
		loggedIn    bool
		auth        bool
	}{
		{"tls proxy with a password", "https://puser:" + proxyPw + "@" + strings.TrimPrefix(secure.URL, "https://"), []string{"SYNOLOGY_CA_CERT=" + ca, "SSL_CERT_FILE=" + ca, "SYNOLOGY_TLS_SERVER_NAME=example.com"}, true, true},
		{"plain proxy without a password", plain.URL, []string{"SYNOLOGY_TLS_FINGERPRINT=" + n.fingerprint}, true, false},
		{"plain proxy with a password", "http://puser:" + proxyPw + "@" + strings.TrimPrefix(plain.URL, "http://"), []string{"SYNOLOGY_TLS_FINGERPRINT=" + n.fingerprint}, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			before := len(n.logins())
			mu.Lock()
			auths = nil
			mu.Unlock()
			env := closedEnv(t.TempDir(), append(append(slices.Clone(base), c.trust...), "HTTPS_PROXY="+c.proxy)...)
			out := run(t, cliBin, env, "auth", "login")
			assertAbsent(t, "auth login", out, pw, proxyPw)
			if got := len(n.logins()) > before; got != c.loggedIn {
				t.Errorf("login reached the NAS: %v, want %v:\n%s", got, c.loggedIn, out)
			}
			if !c.loggedIn && !strings.Contains(out, "refused before sending") {
				t.Errorf("the refusal should say so:\n%s", out)
			}
			mu.Lock()
			defer mu.Unlock()
			if c.auth && (len(auths) == 0 || !strings.HasPrefix(auths[0], "Basic ")) {
				t.Errorf("the proxy got no credentials (%q):\n%s", auths, out)
			}
			if !c.auth && slices.ContainsFunc(auths, func(a string) bool { return a != "" }) {
				t.Errorf("the plain proxy got credentials (%q):\n%s", auths, out)
			}
		})
	}
}

// TestUnfilledMCPBPlaceholderIsNotHeld: an MCPB host leaves ${user_config.x} in a variable
// the user left empty. It is no credential, so it withholds nothing and sends nothing.
func TestUnfilledMCPBPlaceholderIsNotHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	home := t.TempDir()
	env := closedEnv(home,
		"SYNOLOGY_PASSWORD=${user_config.synology_password}",
		"SYNOLOGY_OTP_CODE=${user_config.synology_otp_code}",
		"SYNOLOGY_DEVICE_ID=${user_config.synology_device_id}",
		"SYNOLOGY_MCP_HTTP_TOKEN=${user_config.synology_mcp_http_token}")
	out := run(t, cliBin, env, "version")
	if strings.Contains(out, "credkit") || !strings.Contains(out, "synology") {
		t.Errorf("version under an unfilled placeholder printed:\n%s", out)
	}
	if out := run(t, mcpBin, env, "--help"); strings.Contains(out, "credkit") || !strings.Contains(out, "transport") {
		t.Errorf("synology-mcp --help under an unfilled placeholder printed:\n%s", out)
	}
}

// TestFeedbackEndpointPasswordIsHeld: a URL in SYNOLOGY_FEEDBACK_ENDPOINT can carry a
// password; it is a credential from startup and the collector never gets it by accident.
func TestFeedbackEndpointPasswordIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const endPw = "EndpointPw-" + "5tG6yH7uJ8"
	n := startNAS(t, "SidEnd-1Qa2Ws3Ed4Rf", func(w http.ResponseWriter, _, _ string) {
		fmt.Fprintf(w, `{"success":true,"data":{"tasks":[{"task_name":"%s"}],"total":1}}`, endPw)
	})
	env := closedEnv(t.TempDir(), append(n.env(acmePw), "SYNOLOGY_NO_LEARN=1",
		"SYNOLOGY_FEEDBACK_ENDPOINT=https://collector:"+endPw+"@collector.example.net/in")...)
	out := run(t, cliBin, env, "abb", "tasks", "--json", "--no-cache")
	assertAbsent(t, "abb tasks", out, endPw)
	if !strings.Contains(out, "withheld") {
		t.Errorf("the answer carrying the endpoint's password should be withheld:\n%s", out)
	}
}

// TestMCPHelpPrints: the standard flag package's exit would lose the kit's buffered text.
func TestMCPHelpPrints(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	if out := run(t, mcpBin, closedEnv(home), "--help"); !strings.Contains(out, "transport") {
		t.Errorf("synology-mcp --help printed %q", out)
	}
}

// TestUnrelatedFilesStopNothing: a --config can sit in any project folder, and a run
// must not stop over a file that only has "session" in its name, or over a torn cache.
func TestUnrelatedFilesStopNothing(t *testing.T) {
	cliBin, _ := binaries(t)
	for name, body := range map[string]string{
		"session-notes.json": `{"env":[{"name":"DEBUG","value":"true"}]}`,
		"session-a.json":     `{"id":"data","msg":"hello",}`,
		"session-b.json":     `{"note":"a","count":7,}`,
		"session-c.json":     `started: 12:30 ok "error" here`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, "clients")
			writeFile(t, filepath.Join(dir, "acme.toml"), "base_url = \"https://acme.example.net:5001\"\naccount = \"admin\"\n")
			writeFile(t, filepath.Join(dir, name), body)
			if out := run(t, cliBin, closedEnv(home, "SYNOLOGY_NO_LEARN=1"), "--config", filepath.Join(dir, "acme.toml"), "version"); !strings.Contains(out, "synology") || strings.Contains(out, "credkit:") {
				t.Errorf("version: %q", out)
			}
		})
	}
	for name, body := range map[string]string{
		"an unfinished address":  `{"token":"x-Ab12Cd34","origin":"https://other.nas:5001`,
		"an unfinished key":      `{"server":"https://10.0.0.5:5001","tok`,
		"an unfinished date":     `{"token":"x-Ab12Cd34","saved":"2026-11-03T`,
		"an unfinished remember": `{"cookies":[{"url":"https://nas:5001/","name":"stay_login","value":"1`,
		"a very deep file":       strings.Repeat("[", 200000),
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "data", ".session-x-0a1b2c3d.json.123.tmp"), body)
			if out := run(t, cliBin, closedEnv(home, "SYNOLOGY_NO_LEARN=1"), "abb", "--help"); !strings.Contains(out, "abb") || strings.Contains(out, "credkit:") {
				t.Errorf("help: %.300q", out)
			}
		})
	}
	// The other direction: what the torn file does name stays held, whole.
	t.Run("a credential shaped like an address", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, "data", "session.json"), `{"token":"https://synology-cli"`)
		assertAbsent(t, "help", run(t, cliBin, closedEnv(home, "SYNOLOGY_NO_LEARN=1"), "abb", "--help"), "synology-cli")
	})
}

// TestProfileNamedLikeACredential: a profile's name is not a credential's name.
func TestProfileNamedLikeACredential(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	env := closedEnv(home, "SYNOLOGY_NO_LEARN=1")
	run(t, cliBin, env, "profile", "save", "token-check", "--json")
	if out := run(t, cliBin, env, "version"); strings.Contains(out, "credkit:") {
		t.Errorf("version: %q", out)
	}
}

// TestCredentialsFileBelongsToItsNAS: credentials.toml names the server it was saved for.
func TestCredentialsFileBelongsToItsNAS(t *testing.T) {
	cliBin, _ := binaries(t)
	const stored = "CredsPw-" + "6yH7uJ8iK9"
	a := startNAS(t, "SidA-1Qa2Ws3Ed4Rf", emptyAnswer)
	b := startNAS(t, "SidB-5Tg6Yh7Uj8Ik", emptyAnswer)
	for _, tc := range []struct {
		target *nas
		want   []string
	}{{b, nil}, {a, []string{stored}}} {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, "data", "credentials.toml"), fmt.Sprintf("base_url = %q\naccount = \"admin\"\npassword = %q\n", a.url, stored))
		before := len(tc.target.logins())
		out := run(t, cliBin, closedEnv(home, "SYNOLOGY_BASE_URL="+tc.target.url, "SYNOLOGY_TLS_FINGERPRINT="+tc.target.fingerprint, "SYNOLOGY_NO_LEARN=1"), "auth", "login")
		if got := tc.target.logins()[before:]; !slices.Equal(got, tc.want) {
			t.Errorf("%s saw logins %q, want %q:\n%s", tc.target.url, got, tc.want, out)
		}
	}
}

// TestCredentialsFileWithAConfigBelongsToItsNAS: an operator repoints config.toml at
// another NAS. The login in credentials.toml still belongs to the NAS it names, in
// either direction, on the config's address or the environment's.
func TestCredentialsFileWithAConfigBelongsToItsNAS(t *testing.T) {
	cliBin, _ := binaries(t)
	const storedPw = "CredsFilePw-" + "6yH7uJ8iK9"
	a := startNAS(t, "SidCfgA-1Qa2Ws3Ed4Rf", emptyAnswer)
	b := startNAS(t, "SidCfgB-5Tg6Yh7Uj8Ik", emptyAnswer)
	setup := func(t *testing.T, configURL, credsURL string) string {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+configURL+"\"\n")
		writeFile(t, filepath.Join(home, "data", "credentials.toml"), "base_url = \""+credsURL+"\"\naccount = \"badmin\"\npassword = \""+storedPw+"\"\n")
		return home
	}
	t.Run("config names another NAS", func(t *testing.T) {
		home := setup(t, a.url, b.url)
		out := run(t, cliBin, closedEnv(home, "SYNOLOGY_TLS_FINGERPRINT="+a.fingerprint), "auth", "login")
		if got := a.logins(); len(got) != 0 {
			t.Errorf("NAS A received logins %q, the password of NAS B:\n%s", got, out)
		}
		assertAbsent(t, "auth login", out, storedPw)
	})
	t.Run("environment names another NAS", func(t *testing.T) {
		home := setup(t, b.url, b.url)
		out := run(t, cliBin, closedEnv(home, "SYNOLOGY_BASE_URL="+a.url, "SYNOLOGY_TLS_FINGERPRINT="+a.fingerprint), "auth", "login")
		if got := a.logins(); len(got) != 0 {
			t.Errorf("NAS A received logins %q, the password of NAS B:\n%s", got, out)
		}
	})
	t.Run("the NAS it names still gets it", func(t *testing.T) {
		home := setup(t, b.url, b.url)
		out := run(t, cliBin, closedEnv(home, "SYNOLOGY_TLS_FINGERPRINT="+b.fingerprint), "auth", "login")
		if got := b.logins(); !slices.Equal(got, []string{storedPw}) {
			t.Errorf("NAS B received logins %q:\n%s", got, out)
		}
	})
}

// TestLogoutKeepsACredentialsFileOfAnotherNAS: logging out of one NAS leaves the login
// credentials.toml holds for another, and clears it for the NAS it names.
func TestLogoutKeepsACredentialsFileOfAnotherNAS(t *testing.T) {
	cliBin, _ := binaries(t)
	a, b, home, env := twoNAS(t, emptyAnswer)
	stored := filepath.Join(home, "data", "credentials.toml")
	writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+a.url+"\"\n")
	writeFile(t, stored, "base_url = \""+b.url+"\"\naccount = \"badmin\"\npassword = \"CredsFilePw-6yH7uJ8iK9\"\n")
	out := run(t, cliBin, env, "auth", "logout", "--nas", "acme")
	if _, err := os.Stat(stored); err != nil {
		t.Errorf("logging out of acme removed bravo's login: %v\n%s", err, out)
	}
	out = run(t, cliBin, env, "auth", "logout", "--nas", "bravo")
	if _, err := os.Stat(stored); err == nil {
		t.Errorf("logging out of bravo kept its login:\n%s", out)
	}
}

// TestMCPServerTakesNoPositionalArguments: binding reads a fleet add line before any parser,
// and the MCP server has no such command, so a launch line carrying one must not start
// a server that sends the environment's password to that address.
func TestMCPServerTakesNoPositionalArguments(t *testing.T) {
	_, mcpBin := binaries(t)
	a := startNAS(t, "SidMcpA-1Qa2Ws3Ed4Rf", emptyAnswer)
	c := startNAS(t, "SidMcpC-5Tg6Yh7Uj8Ik", emptyAnswer)
	home := t.TempDir()
	env := closedEnv(home, "SYNOLOGY_BASE_URL="+a.url, "SYNOLOGY_ACCOUNT=eadmin", "SYNOLOGY_PASSWORD="+acmePw, "SYNOLOGY_SKIP_TLS_VERIFY=1", "SYNOLOGY_NO_LEARN=1")
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"abb_tasks","arguments":{}}}` + "\n"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "fleet", "add", "x", "--url", c.url)
	cmd.Env, cmd.Stdin = env, strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if got := c.logins(); len(got) != 0 {
		t.Errorf("the address on the launch line received logins %q:\n%s", got, out)
	}
	if err == nil || !strings.Contains(string(out), "takes no positional arguments") {
		t.Errorf("the server should refuse to start (err=%v):\n%s", err, out)
	}
}

// TestArgvNeverBindsAProcessTheParserDoesNot: a scan of argv can read --nas or --url where
// the flag parser reads something else. The environment's password stays with its NAS.
func TestArgvNeverBindsAProcessTheParserDoesNot(t *testing.T) {
	cliBin, _ := binaries(t)
	a, b, _, _ := twoNAS(t, emptyAnswer)
	for name, args := range map[string][]string{
		"flag value":            {"auth", "login", "--select", "--nas", "bravo"},
		"after dashes":          {"auth", "login", "--json", "--", "--nas", "bravo"},
		"words in a positional": {"auth", "login", "--json", "--", "fleet", "add", "--url", b.url},
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "fleet.json"), `{"nas":[{"name":"bravo","url":"`+b.url+`","env_prefix":"BRAVO","tls_fingerprint":"`+b.fingerprint+`"}]}`)
			env := closedEnv(home, append(a.env(acmePw), "SYNOLOGY_SKIP_TLS_VERIFY=1")...)
			before := len(a.logins())
			out := run(t, cliBin, env, args...)
			if got := b.logins(); slices.Contains(got, acmePw) {
				t.Errorf("bravo received A's password (logins %q):\n%s", got, out)
			}
			if got := a.logins(); len(got) != before+1 {
				t.Errorf("A received logins %q, want one more than %d:\n%s", got, before, out)
			}
		})
	}
}

// TestCachedCookieOfAnotherNASIsSentNowhere: a record's origin says nothing of each
// cookie's own address. A cookie for this NAS still reaches it.
func TestCachedCookieOfAnotherNASIsSentNowhere(t *testing.T) {
	cliBin, _ := binaries(t)
	const bravoCookie, ownCookie = "BravoCookie-" + "Ab12Cd34", "OwnCookie-" + "Ef56Gh78"
	n := startNAS(t, "SidOwn-7Yh6Tg5Rf4Ed", emptyAnswer)
	home := t.TempDir()
	env := closedEnv(home, append(n.env(acmePw), "SYNOLOGY_NO_LEARN=1")...)
	run(t, cliBin, env, "auth", "login")
	files, _ := filepath.Glob(filepath.Join(home, "data", "session-*.json"))
	if len(files) != 1 {
		t.Fatalf("session files: %q", files)
	}
	data, _ := os.ReadFile(files[0])
	var rec map[string]any
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	rec["cookies"] = []map[string]any{
		{"url": "https://bravo.internal:5001/", "name": "id", "value": bravoCookie, "path": "/"},
		{"url": n.url + "/", "name": "id", "value": ownCookie, "path": "/"},
	}
	data, _ = json.Marshal(rec)
	writeFile(t, files[0], string(data))
	run(t, cliBin, env, "abb", "tasks", "--data-source", "live", "--no-cache", "--filter", fmt.Sprintf(`{"task_name":%q}`, bravoCookie))
	assertAbsent(t, "what this NAS received", n.sent(), bravoCookie)
	out := run(t, cliBin, env, "abb", "tasks", "--data-source", "live", "--no-cache", "--filter", fmt.Sprintf(`{"task_name":%q}`, ownCookie))
	if !strings.Contains(n.sent(), ownCookie) {
		t.Errorf("this NAS's own cookie is bound to it, so naming it reaches it:\n%s\n%s", out, n.sent())
	}
}

// TestReissuedSessionIsSent: what a login returns came from the NAS logged in to, even
// when it equals a value a cache already holds.
func TestReissuedSessionIsSent(t *testing.T) {
	cliBin, _ := binaries(t)
	a, _, _, env := twoNAS(t, emptyAnswer)
	run(t, cliBin, append(slices.Clone(env), "SYNOLOGY_BASE_URL="+a.url, "SYNOLOGY_TLS_FINGERPRINT="+a.fingerprint, "SYNOLOGY_ACCOUNT=admin", "SYNOLOGY_PASSWORD="+acmePw), "auth", "login")
	if out := run(t, cliBin, env, "fleet", "sync", "--nas", "acme", "--json"); strings.Contains(out, "refused before sending") {
		t.Errorf("a session the NAS issued again was refused:\n%s", out)
	}
}

// TestFeedbackStdinBody: a --stdin body is held whole and line by line before anything
// can print (Declarations.StdinFlags), so feedback --stdin could only store a withheld
// notice. It refuses instead, stores nothing, and shows no part of the body.
func TestFeedbackStdinBody(t *testing.T) {
	cliBin, _ := binaries(t)
	feedback := func(t *testing.T, body string, args ...string) (string, string, error) {
		t.Helper()
		home := t.TempDir()
		cmd := exec.Command(cliBin, args...)
		cmd.Env = closedEnv(home, "SYNOLOGY_NO_LEARN=1")
		cmd.Stdin = strings.NewReader(body)
		out, err := cmd.CombinedOutput()
		var stored strings.Builder
		_ = filepath.WalkDir(home, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if data, rerr := os.ReadFile(p); rerr == nil {
					stored.Write(data)
				}
			}
			return nil
		})
		return string(out), stored.String(), err
	}
	const note = "the sync command is slow on a large NAS"
	for _, body := range []string{
		note + "\nsecond line of notes\n",
		"it failed with password: " + stdinPw,
		`{"password":"` + stdinPw + `"}`,
		"{\n\"password\":\n\"" + stdinPw + "\"\n}\n",
		"see https://admin:" + stdinPw + "@nas.example.net/",
	} {
		out, stored, err := feedback(t, body, "feedback", "--stdin", "--json")
		if err == nil {
			t.Errorf("feedback --stdin reported success on %q:\n%s", body, out)
		}
		for _, part := range []string{stdinPw, note} {
			assertAbsent(t, "feedback output for "+body, out, part)
			assertAbsent(t, "stored feedback for "+body, stored, part)
		}
	}
	// The hold itself: the body's value withheld where a later error would print it.
	out, _, _ := feedback(t, stdinPw+"\n", "feedback", "list", "--stdin", "--limit", stdinPw)
	assertAbsent(t, "an error quoting a held stdin line", out, stdinPw)
	// A note as an argument still records.
	out, stored, err := feedback(t, "", "feedback", note, "--json")
	if err != nil || !strings.Contains(stored, note) {
		t.Errorf("feedback as an argument should record: err=%v stored=%q out=%q", err, stored, out)
	}
}

// TestTLSKeyFileIsHeld: the key --tls-key names is held from startup, line by line,
// and its armor is not: a PEM certificate's armor stays printable.
func TestTLSKeyFileIsHeld(t *testing.T) {
	var held []string
	old := cliutil.Hold
	cliutil.Hold = func(_ credkit.Source, values ...string) { held = append(held, values...) }
	defer func() { cliutil.Hold = old }()
	path := filepath.Join(t.TempDir(), "key.pem")
	writeFile(t, path, "-----BEGIN "+"PRIVATE KEY-----\nMIIEvKeyBodyLineOne\nKeyBodyLineTwo\n-----END "+"PRIVATE KEY-----\n")
	cliutil.HoldTLSKeyFile(path)
	if !slices.Equal(held, []string{"MIIEvKeyBodyLineOne", "KeyBodyLineTwo"}) {
		t.Errorf("held %q", held)
	}
}

// TestMCPHTTPRefusalsPassTheKit: the HTTP server's refusals and results leave
// through credkit.MCPHTTP, so a refused request echoes nothing held.
func TestMCPHTTPRefusalsPassTheKit(t *testing.T) {
	_, mcpBin := binaries(t)
	const token = mcpBearer
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", addr)
	cmd.Env = closedEnv(t.TempDir(), "SYNOLOGY_MCP_HTTP_TOKEN="+token)
	var serverOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &serverOut, &serverOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	post := func(auth, origin string) (int, string) {
		var last error
		for i := 0; i < 50; i++ {
			req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if auth != "" {
				req.Header.Set("Authorization", "Bearer "+auth)
			}
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				last = err
				time.Sleep(100 * time.Millisecond)
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return resp.StatusCode, string(b)
		}
		t.Fatalf("the server never answered: %v\n%s", last, serverOut.String())
		return 0, ""
	}
	if code, body := post("", ""); code != http.StatusUnauthorized || strings.Contains(body, token) {
		t.Errorf("no token: %d %s", code, body)
	}
	if code, body := post("wrong-"+token, ""); code != http.StatusUnauthorized || strings.Contains(body, token) {
		t.Errorf("wrong token echoed: %d %s", code, body)
	}
	if code, body := post(token, "https://evil.example"); code != http.StatusForbidden || strings.Contains(body, token) {
		t.Errorf("foreign origin: %d %s", code, body)
	}
	if code, body := post(token, ""); code != http.StatusOK || !strings.Contains(body, "serverInfo") || strings.Contains(body, token) {
		t.Errorf("good token: %d %s", code, body)
	}
}

// TestDeclaredCredentialFlagsMatchTheCommands keeps CredentialFlags complete: a
// string flag named like a credential is declared, and a declared flag exists.
func TestDeclaredCredentialFlagsMatchTheCommands(t *testing.T) {
	declared := credwire.Declarations.CredentialFlags
	looks := func(name string) bool {
		for _, w := range []string{"password", "passwd", "secret", "otp"} {
			if strings.Contains(name, w) {
				return true
			}
		}
		return strings.HasSuffix(name, "-token") || strings.HasSuffix(name, "-key")
	}
	seen := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) {
			seen["--"+f.Name] = true
			if f.Value.Type() == "string" && looks(f.Name) && !slices.Contains(declared, "--"+f.Name) {
				t.Errorf("%s --%s looks like a credential and is not in credwire.Declarations.CredentialFlags", c.CommandPath(), f.Name)
			}
		}
		c.Flags().VisitAll(visit)
		c.PersistentFlags().VisitAll(visit)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())
	for _, f := range declared {
		if !seen[f] {
			t.Errorf("credwire declares %s, which no command has", f)
		}
	}
}

// TestEveryEnvVarIsClassified fails on a SYNOLOGY_ variable the code names that
// is neither declared a secret nor given a reason: a new credential variable must
// be declared before it can be printed.
func TestEveryEnvVarIsClassified(t *testing.T) {
	re := regexp.MustCompile(`"(SYNOLOGY_[A-Z0-9_]+)"`)
	known := map[string]bool{}
	for _, name := range credwire.Declarations.SecretEnv {
		known[name] = true
	}
	for _, id := range credwire.Declarations.Identifiers {
		known[strings.TrimPrefix(id.Source, "env:")] = true
	}
	var unclassified []string
	root := filepath.Join("..", "..")
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || strings.Contains(filepath.ToSlash(p), "/credkit/") {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			if !known[m[1]] {
				unclassified = append(unclassified, m[1]+" ("+p+")")
			}
		}
		return nil
	})
	sort.Strings(unclassified)
	if len(unclassified) > 0 {
		t.Fatalf("unclassified SYNOLOGY_ variables (declare them in SecretEnv or Identifiers):\n%s", strings.Join(unclassified, "\n"))
	}
}

// writeExempt are the files that write past the kit, each a credential store or a
// file with no new content (publish brief, credential stores and config rewrites).
var writeExempt = map[string]string{
	"internal/cliutil/paths.go":         "the credential store's own atomic writer; the private writer goes through the kit",
	"internal/cliutil/filelock.go":      "lock files: no content",
	"internal/config/config.go":         "the config file, a declared credential source (Declarations.Files)",
	"internal/cli/auth_logout_dsm.go":   "removes credentials from the session and config files, both credential sources",
	"internal/dsm/devicecache.go":       "session file with the trusted-device token, a credential source",
	"internal/cli/store_adopt_linux.go": "changes an existing database file's ownership: no new content",
	"internal/platform/migration.go":    "copies an existing tenant database: no new content",
	"internal/store/store.go":           "creates the SQLite journal empty, so its mode is private; rows go through internal/credsql",
}

// TestEveryFileWriteGoesThroughTheKit fails on a file that writes to disk or opens
// SQLite past the kit and is not a listed exemption. A file may open or create a
// file empty and write it with the kit, so opening one only counts where nothing
// in that source file uses the kit.
func TestEveryFileWriteGoesThroughTheKit(t *testing.T) {
	direct := regexp.MustCompile(`\bos\.WriteFile\(|sql\.Open\("sqlite"`)
	opens := regexp.MustCompile(`\bos\.Create\(|\bos\.CreateTemp\(|\bos\.OpenFile\([^)]*(O_WRONLY|O_RDWR|O_APPEND|O_CREATE)`)
	root := filepath.Join("..", "..")
	var unchecked []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "/credkit/") || strings.Contains(rel, "/testenv/") || strings.Contains(rel, "/dsmtest/") {
			return nil
		}
		b, _ := os.ReadFile(p)
		if !direct.Match(b) && (!opens.Match(b) || bytes.Contains(b, []byte("credkit."))) {
			return nil
		}
		if _, ok := writeExempt[rel]; !ok {
			unchecked = append(unchecked, rel)
		}
		return nil
	})
	sort.Strings(unchecked)
	if len(unchecked) > 0 {
		t.Fatalf("files that write past the kit (route them through it, or list the reason in writeExempt):\n%s", strings.Join(unchecked, "\n"))
	}
}
