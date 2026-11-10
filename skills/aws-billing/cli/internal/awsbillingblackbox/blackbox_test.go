// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package awsbillingblackbox runs the built CLI and MCP binaries against a
// fake AWS that answers the AWS SDK by protocol, and checks the connector's
// own behavior through the shared credential kit (what a reviewer cannot read
// off the code):
//
//   - every AWS credential source the SDK reads (the environment, a profile in
//     the shared files, an assumed role, the SSO token cache) signs real
//     requests that pass the kit's outbound guard, and every sink (stdout,
//     stderr, MCP, the local store, exports, --deliver files, report HTML,
//     the learn journal) carries the data;
//   - one run per sink against answers that repeat every credential the
//     process holds, whole, encoded and split: none of it reaches the sink;
//   - host pinning (aws-endpoint-pinned): a custom endpoint, every redirect,
//     and in a production build the fake itself, are refused before anything
//     is sent;
//   - writes: report --post-slack previews unless --yes; the MCP report tool
//     previews until confirm is boolean true and never writes a file;
//     --dry-run sends nothing;
//   - the Codex review rounds' wrong-data classes (codex_r1_test.go,
//     codex_r2_test.go).
//
// The black-box build (-tags awsbilling_blackbox) dials the fake for every
// AWS host (internal/awsx/awsbilling_testendpoint.go), so the SDK, the pin
// and the kit all see the real AWS host names. Every process gets a tripwire
// proxy for anything else.
package awsbillingblackbox

import (
	"bytes"
	"crypto/sha1"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
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
	bbEnvKey     = "AKIA" + "BBENVKEY7Q2W9E4R"                   // AWS_ACCESS_KEY_ID (in every SigV4 credential scope it signs)
	bbEnvSecret  = "bbEnvSecret/7Qm2xZ9pR4tW1vB8nL3cK6"          // AWS_SECRET_ACCESS_KEY (signs, never sent)
	bbEnvSession = "bbEnvSessionToken+FwoGZXIvYXdzEJr7c9d1e5f3a" // AWS_SESSION_TOKEN (X-Amz-Security-Token)
	bbDefKey     = "AKIA" + "BBDEFAULTKEY0P5O"                   // [default] in ~/.aws/credentials, held unused
	bbDefSecret  = "bbDefaultSecret/4nB7vC1xZ8mQ2wE9"
	bbFileKey    = "AKIA" + "BBFILEKEY3T8Y1U6" // [bbfile] in ~/.aws/credentials
	bbFileSecret = "bbFileSecret+9kD3hT6yU1oP5sA2zX"
	bbSSOToken   = "bbSsoCacheAccessToken-aoaAAAAAGq7xY2kL9" // ~/.aws/sso/cache accessToken (x-amz-sso_bearer_token)
	bbGenToken   = "bbGenApiToken-5c1e9a7d3b2f8046"          // config.toml auth_header "Bearer <it>" (the generated client)
	bbHTTPToken  = "bbMcpHttpToken-0123456789abcdefABCDEF"   // AWS_BILLING_MCP_HTTP_TOKEN
	bbECSAuth    = "bbEcsAuthzQ-4f8e2c6a1d9b"                // AWS_CONTAINER_AUTHORIZATION_TOKEN
	bbSSOURL     = "https://bb-example.awsapps.com/start"
)

func secrets() []string {
	return append([]string{bbEnvKey, bbEnvSecret, bbEnvSession, bbDefKey, bbDefSecret, bbFileKey, bbFileSecret,
		bbSSOToken, bbGenToken, bbHTTPToken, bbECSAuth}, mintedSecrets()...)
}

// minted is every credential the fakes' credential services handed out
// (AssumeRole, SSO GetRoleCredentials): fresh for every call, as AWS mints a
// new session each time, so a process holds only its own.
var minted struct {
	sync.Mutex
	vals []string
}

func mintedSecrets() []string {
	minted.Lock()
	defer minted.Unlock()
	return append([]string(nil), minted.vals...)
}

// mint returns a fresh access key ID, secret key and session token of kind
// "role", "sso" or "ecs", and records them.
func mint(kind string, n int) []string {
	var trio []string
	switch kind {
	case "role":
		trio = []string{fmt.Sprintf("ASIA"+"BBROLEKEY9I4U%03d", n), fmt.Sprintf("bbRoleSecret/3aS6dF9gH2jK5lZ8%03d", n),
			fmt.Sprintf("bbRoleSessionToken+IQoJb3JpZ2luX2Vj%03d", n)}
	case "sso":
		trio = []string{fmt.Sprintf("ASIA"+"BBSSOKEY5R8E1%03d", n), fmt.Sprintf("bbSsoSecret/6tY9uI2oP5aS8dF1%03d", n),
			fmt.Sprintf("bbSsoSessionToken+ZmFrZVNzb1Nlc3Np%03d", n)}
	default:
		trio = []string{fmt.Sprintf("ASIA"+"BBECSKEY7T1Y4%03d", n), fmt.Sprintf("bbEcsSecret/1qW3eR5tY7uI9oPa%03d", n),
			fmt.Sprintf("bbEcsSessionToken+QWxsQ29udGFpbmVy%03d", n)}
	}
	minted.Lock()
	minted.vals = append(minted.vals, trio...)
	minted.Unlock()
	return trio
}

var bbCLI, bbMCP, bbProdCLI string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "aws-billing-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "aws-billing-cli"+exe)
	bbMCP = filepath.Join(dir, "aws-billing-mcp"+exe)
	// The production CLI (no test tag) sits in a directory of its own.
	bbProdCLI = filepath.Join(dir, "prod", "aws-billing-cli"+exe)
	for _, b := range []struct{ out, pkg, tags string }{
		{bbCLI, "../../cmd/aws-billing-cli", "awsbilling_blackbox"},
		{bbMCP, "../../cmd/aws-billing-mcp", "awsbilling_blackbox"},
		{bbProdCLI, "../../cmd/aws-billing-cli", ""},
	} {
		args := []string{"build", "-o", b.out}
		if b.tags != "" {
			args = append(args, "-tags", b.tags)
		}
		if out, err := exec.Command("go", append(args, b.pkg)...).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", b.pkg, err, out)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// echoForms is every encoding of s the fakes send back whole.
func echoForms(s string) []string {
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(s))
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

// assertNoEcho fails when text holds a secret in any form the fakes sent,
// both halves of one (a split the reader could rejoin), more than half of one
// of its forms in one run (a table cell cut to its column width, an encoded
// form cut beside a withheld notice), or one case-folded or
// re-separated (a normalizer's lowercased copy).
func assertNoEcho(t *testing.T, where, text string) {
	t.Helper()
	folded := foldedAlnum(text)
	for _, s := range secrets() {
		for _, f := range echoForms(s) {
			if strings.Contains(text, f) {
				t.Errorf("%s holds a credential as %q", where, f)
			}
		}
		if a, b := halves(s); strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q: %s / %s", where, s, around(text, a), around(text, b))
		}
		for _, f := range echoForms(s) {
			if run := majorityRun(text, f); run != "" && len(f) >= foldedMinAlnum {
				t.Errorf("%s holds %q, more than half of %q", where, run, f)
				break
			}
		}
		if n := foldedAlnum(s); len(n) >= foldedMinAlnum && strings.Contains(folded, n) {
			t.Errorf("%s holds %q case-folded or re-separated", where, s)
		}
	}
}

// around is where text holds part, with some context, for a failure message.
func around(text, part string) string {
	i := strings.Index(text, part)
	return fmt.Sprintf("at %d %q", i, text[max(0, i-160):min(len(text), i+len(part)+40)])
}

// majorityRun is a run of s longer than either half of it that text holds,
// or "".
func majorityRun(text, s string) string {
	n := len(s) - len(s)/2 + 1
	for i := 0; i+n <= len(s); i++ {
		if strings.Contains(text, s[i:i+n]) {
			return s[i : i+n]
		}
	}
	return ""
}

// source is where the AWS SDK gets the credentials it signs with.
type source int

const (
	srcEnv         source = iota // AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN
	srcProfileEnv                // the bbfile profile of ~/.aws/credentials, named by AWS_PROFILE
	srcProfileFlag               // the same profile, named by --profile-aws on each command
	srcRole                      // the bbrole profile: AssumeRole with bbfile's keys; the answer's credentials sign
	srcSSO                       // the bbsso profile: the cached SSO token buys role credentials
	srcContainer                 // no shared credentials: the container endpoint (AWS_CONTAINER_CREDENTIALS_FULL_URI) answers with credentials
)

func (s source) String() string {
	return [...]string{"env keys", "AWS_PROFILE", "--profile-aws", "assume-role profile", "SSO cache", "container credentials"}[s]
}

// signer is the access key ID that signs the service calls under s; "" for
// the sources whose credentials are minted per process (role, SSO).
func (s source) signer() string {
	return [...]string{bbEnvKey, bbFileKey, bbFileKey, "", "", ""}[s]
}

// principal is the source's own startup credentials: the fake also repeats
// each of them split in two (and the minted credentials of the process it
// answers, for role and SSO).
func (s source) principal() []string {
	switch s {
	case srcEnv:
		return []string{bbEnvKey, bbEnvSecret, bbEnvSession}
	case srcSSO:
		return []string{bbSSOToken}
	case srcContainer:
		return []string{bbECSAuth}
	}
	return []string{bbFileKey, bbFileSecret}
}

// typed is a credential every process holds from its startup sources under s
// (minted credentials are held only by a process that minted them): the value
// the tests type on command lines and into MCP arguments.
func (s source) typed() string {
	switch s {
	case srcEnv:
		return bbEnvSecret
	case srcSSO:
		return bbSSOToken
	case srcContainer:
		return bbECSAuth
	}
	return bbFileSecret
}

// held is what a process holds from its startup sources under s: the fakes
// repeat all of it in every answer.
func (s source) held() []string {
	// AWS_BILLING_MCP_HTTP_TOKEN is held too, but an MCP server's CLI children
	// never receive it (mcpChildEnviron), so no AWS answer could repeat it.
	h := []string{bbDefKey, bbDefSecret, bbFileKey, bbFileSecret, bbSSOToken, bbGenToken}
	switch s {
	case srcEnv:
		h = append(h, bbEnvKey, bbEnvSecret, bbEnvSession)
	case srcContainer:
		h = []string{bbSSOToken, bbGenToken, bbECSAuth} // the shared files are empty
	}
	return h
}

type bbEnv struct {
	src     source
	home    string // the isolated HOME (every XDG and temp directory is under it)
	work    string // the working directory of every run, under home
	slack   string // where the fake slack-pp-cli records each run
	env     []string
	aws     *fakeAWS
	api     *fakeAPI
	trip    *tripwire
	inputs  map[string]bool // files the test wrote (credential inputs), skipped by the file scan
	awsArgs []string        // appended to every AWS command (--profile-aws)
}

// newEnv builds a clean environment against new fakes, with every credential
// source on disk and src the one the SDK signs with.
func newEnv(t *testing.T, src source) *bbEnv {
	t.Helper()
	home := t.TempDir()
	e := &bbEnv{src: src, home: home, work: filepath.Join(home, "work"), slack: t.TempDir(), inputs: map[string]bool{},
		aws: newFakeAWS(t, src.held()), api: newFakeAPI(t, src.held()), trip: newTripwire(t)}
	e.aws.splits = src.principal()
	t.Cleanup(func() { e.trip.check(t) })
	tmp := filepath.Join(home, "tmp")
	for _, d := range []string{tmp, e.work} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fakeBin := filepath.Join(e.slack, "bin")
	e.writeSlackFake(t, fakeBin)
	e.input(t, filepath.Join(home, ".aws", "credentials"), fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = %s\n\n"+
		"[bbfile]\naws_access_key_id = %s\naws_secret_access_key = %s\n", bbDefKey, bbDefSecret, bbFileKey, bbFileSecret))
	e.input(t, filepath.Join(home, ".aws", "config"), "[default]\nregion = us-east-1\n\n[profile bbfile]\nregion = us-east-1\n\n"+
		"[profile bbrole]\nrole_arn = arn:aws:iam::111111111111:role/bb\nsource_profile = bbfile\nregion = us-east-1\n\n"+
		"[profile bbsso]\nsso_start_url = "+bbSSOURL+"\nsso_region = us-east-1\nsso_account_id = 111111111111\nsso_role_name = BBRead\nregion = us-east-1\n")
	sum := sha1.Sum([]byte(bbSSOURL))
	cache, _ := json.Marshal(map[string]string{"accessToken": bbSSOToken, "expiresAt": time.Now().Add(8 * time.Hour).UTC().Format(time.RFC3339),
		"region": "us-east-1", "startUrl": bbSSOURL})
	e.input(t, filepath.Join(home, ".aws", "sso", "cache", hex.EncodeToString(sum[:])+".json"), string(cache))
	cfg := filepath.Join(home, "cfg", "config.toml")
	e.input(t, cfg, fmt.Sprintf("auth_header = %q\n", "Bearer "+bbGenToken))
	e.env = []string{
		"PATH=" + fakeBin + string(os.PathListSeparator) + "/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"AWS_REGION=us-east-1", "AWS_EC2_METADATA_DISABLED=true",
		"AWS_BILLING_BLACKBOX_AWS_ENDPOINT=" + e.aws.url, "AWS_BILLING_BLACKBOX_AWS_CA=" + e.aws.caFile,
		"AWS_BILLING_BASE_URL=" + e.api.url, "AWS_BILLING_CA_CERT=" + e.api.caFile, "AWS_BILLING_CONFIG=" + cfg,
		"AWS_BILLING_CLI_PATH=" + bbCLI, "AWS_BILLING_MCP_HTTP_TOKEN=" + bbHTTPToken,
		"HTTPS_PROXY=" + e.trip.url, "HTTP_PROXY=" + e.trip.url, "https_proxy=" + e.trip.url, "http_proxy=" + e.trip.url,
		"BB_CHILD_MARKER=present",
	}
	switch src {
	case srcEnv:
		e.env = append(e.env, "AWS_ACCESS_KEY_ID="+bbEnvKey, "AWS_SECRET_ACCESS_KEY="+bbEnvSecret, "AWS_SESSION_TOKEN="+bbEnvSession)
	case srcProfileEnv:
		e.env = append(e.env, "AWS_PROFILE=bbfile")
	case srcProfileFlag:
		e.awsArgs = []string{"--profile-aws", "bbfile"}
	case srcRole:
		e.env = append(e.env, "AWS_PROFILE=bbrole")
	case srcSSO:
		e.env = append(e.env, "AWS_PROFILE=bbsso")
	case srcContainer:
		empty := filepath.Join(home, "empty")
		e.input(t, empty, "")
		e.env = append(e.env, "AWS_SHARED_CREDENTIALS_FILE="+empty, "AWS_CONFIG_FILE="+empty,
			"AWS_CONTAINER_CREDENTIALS_FULL_URI="+e.aws.containerURL+"/v2/credentials", "AWS_CONTAINER_AUTHORIZATION_TOKEN="+bbECSAuth)
	}
	return e
}

// input writes a file the test owns (a credential source); the file scan
// skips it.
func (e *bbEnv) input(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	e.inputs[path] = true
}

// writeSlackFake puts a slack-pp-cli on PATH that records its argv, stdin and
// environment, one set of files per run, under e.slack.
func (e *bbEnv) writeSlackFake(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return // the stand-in is a shell script; every test that runs it skips on Windows
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/bash\nd=" + shellQuote(e.slack) + "\nn=\"$$.$RANDOM\"\n" +
		"printf '%s\\n' \"$@\" > \"$d/argv.$n\"\nenv > \"$d/env.$n\"\ncat > \"$d/stdin.$n\"\necho '{\"ok\":true}'\n"
	if err := os.WriteFile(filepath.Join(dir, "slack-pp-cli"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// slackRuns is every recorded slack-pp-cli run: argv, environment and stdin
// joined, keyed by run.
func (e *bbEnv) slackRuns(t *testing.T) map[string]map[string]string {
	t.Helper()
	runs := map[string]map[string]string{}
	entries, _ := os.ReadDir(e.slack)
	for _, d := range entries {
		kind, id, ok := strings.Cut(d.Name(), ".")
		if !ok || d.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(e.slack, d.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if runs[id] == nil {
			runs[id] = map[string]string{}
		}
		runs[id][kind] = string(b)
	}
	return runs
}

// with returns a copy of e whose environment has kv added (a later
// assignment wins).
func (e *bbEnv) with(kv ...string) *bbEnv {
	c := *e
	c.env = append(append([]string(nil), e.env...), kv...)
	return &c
}

func withoutEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}

func (e *bbEnv) run(t *testing.T, args ...string) string {
	t.Helper()
	out, _ := e.runCode(t, "", bbCLI, args...)
	return out
}

func (e *bbEnv) runIn(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	out, _ := e.runCode(t, stdin, bbCLI, args...)
	return out
}

// aws runs an AWS command with the source's AWS arguments.
func (e *bbEnv) awsRun(t *testing.T, args ...string) string {
	t.Helper()
	return e.run(t, append(append([]string(nil), args...), e.awsArgs...)...)
}

// command is bin with args, set up to run in e.
func (e *bbEnv) command(bin string, args ...string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	cmd.Env, cmd.Dir = e.env, e.work
	return cmd
}

// runCode runs bin with args in e and returns stdout and stderr joined, and
// the exit code.
func (e *bbEnv) runCode(t *testing.T, stdin, bin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = e.env
	cmd.Dir = e.work
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	go func() { done <- cmd.Wait() }()
	start := time.Now()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%v: timed out", args)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Logf("slow: %v took %v", args, d.Round(time.Millisecond))
	}
	return out.String() + "\n" + errb.String(), cmd.ProcessState.ExitCode()
}

// mcpPhases runs one MCP stdio session, sending each phase of tools/call
// requests only after every earlier call was answered (mcp-go serves one
// phase's calls concurrently), and returns everything the server wrote. The
// calls are numbered from 1 in the order given.
func (e *bbEnv) mcpPhases(t *testing.T, phases ...[]map[string]any) string {
	t.Helper()
	cmd := exec.Command(bbMCP)
	cmd.Env = e.env
	cmd.Dir = e.work
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
	deadline := time.Now().Add(150 * time.Second)
	for _, calls := range phases {
		for _, c := range calls {
			id++
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": c})
		}
		for time.Now().Before(deadline) && len(mcpReplies(out.String())) < id+1 {
			time.Sleep(50 * time.Millisecond)
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

func (e *bbEnv) mcpCalls(t *testing.T, calls []map[string]any) string {
	t.Helper()
	return e.mcpPhases(t, calls)
}

// mcpRaw runs one MCP stdio session with raw JSON-RPC messages after the
// handshake and returns everything the server wrote.
func (e *bbEnv) mcpRaw(t *testing.T, msgs []map[string]any) string {
	t.Helper()
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	for _, m := range msgs {
		_ = enc.Encode(m)
	}
	cmd := exec.Command(bbMCP)
	cmd.Env, cmd.Dir = e.env, e.work
	cmd.Stdin = &in
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
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

// writtenFiles is every file under the isolated HOME (the working directory
// included) the program wrote: path relative to HOME -> content. The test's
// own credential inputs are left out.
func (e *bbEnv) writtenFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || e.inputs[path] {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(e.home, path)
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	return files
}

// assertNoCredentialInFiles scans every file the program wrote.
func (e *bbEnv) assertNoCredentialInFiles(t *testing.T) map[string]string {
	t.Helper()
	files := e.writtenFiles(t)
	for rel, data := range files {
		if !synccpuStoreFile(rel) { // synccpu: the local store is not checked
			assertNoEcho(t, "file "+rel, data)
		}
	}
	return files
}

// anyFile reports a written file whose name matches pred and whose content
// holds want.
func anyFile(files map[string]string, pred func(rel string) bool, want string) bool {
	for rel, data := range files {
		if pred(rel) && strings.Contains(data, want) {
			return true
		}
	}
	return false
}
