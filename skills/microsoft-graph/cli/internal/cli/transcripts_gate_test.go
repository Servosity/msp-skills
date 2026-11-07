// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"microsoft-graph-pp-cli/internal/client"
	"microsoft-graph-pp-cli/internal/cliutil"
	"microsoft-graph-pp-cli/internal/cliutil/testenv"
	"microsoft-graph-pp-cli/internal/config"
)

const (
	ownTenant    = "11111111-2222-3333-4444-555555555555"
	clientTenant = "99999999-8888-7777-6666-555555555555"
)

func fakeJWT(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	return "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(b) + ".c2ln"
}

func delegatedToken(tid string) string {
	return fakeJWT(map[string]any{"tid": tid, "iss": "https://sts.windows.net/" + tid + "/", "scp": "OnlineMeetingTranscript.Read.All"})
}

func appToken(tid string) string {
	return fakeJWT(map[string]any{"tid": tid, "iss": "https://sts.windows.net/" + tid + "/", "roles": []string{"OnlineMeetingTranscript.Read.All"}})
}

func gateWith(tok string) error {
	_, err := transcriptsGate(&client.Client{Config: &config.Config{MicrosoftGraphToken: tok}, HTTPClient: &http.Client{}})
	return err
}

func exitCodeOf(err error) int {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	return -1
}

// transcripts-own-tenant: only a member's (or an opted-in app's) token for the
// declared own tenant passes; every other credential is refused before any request.
func TestTranscriptsGateOwnTenantOnly(t *testing.T) {
	t.Setenv(homeTenantEnv, "")
	if err := gateWith(delegatedToken(ownTenant)); exitCodeOf(err) != 10 || !strings.Contains(err.Error(), homeTenantEnv) {
		t.Fatalf("no own tenant declared: want config error naming %s, got %v", homeTenantEnv, err)
	}
	t.Setenv(homeTenantEnv, strings.ToUpper(ownTenant))
	t.Setenv(transcriptsAppOnlyEnv, "")
	gdap := fakeJWT(map[string]any{"tid": ownTenant, "iss": "https://sts.windows.net/" + ownTenant + "/", "idp": "https://sts.windows.net/" + clientTenant + "/", "scp": "x"})
	for name, tc := range map[string]struct {
		tok  string
		want string
	}{
		"client tenant":       {delegatedToken(clientTenant), "is for tenant " + clientTenant},
		"client tenant app":   {appToken(clientTenant), "is for tenant " + clientTenant},
		"GDAP or guest user":  {gdap, "signed in from another tenant"},
		"app-only, no opt-in": {appToken(ownTenant), "Grant-CsApplicationAccessPolicy"},
		"opaque":              {"opaqueToken-1234567890", "cannot be confirmed"},
		"no token":            {"", "cannot be confirmed"},
		"no tid":              {fakeJWT(map[string]any{"scp": "x"}), "cannot be confirmed"},
	} {
		err := gateWith(tc.tok)
		if exitCodeOf(err) != 4 || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want auth refusal containing %q, got %v", name, tc.want, err)
		}
	}
	if err := gateWith(delegatedToken(ownTenant)); err != nil {
		t.Errorf("own-tenant delegated token refused: %v", err)
	}
	t.Setenv(transcriptsAppOnlyEnv, "1")
	if err := gateWith(appToken(ownTenant)); err != nil {
		t.Errorf("opted-in own-tenant app token refused: %v", err)
	}
	if err := gateWith(appToken(clientTenant)); exitCodeOf(err) != 4 {
		t.Errorf("the app-only opt-in let a client tenant through: %v", err)
	}
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	t.Setenv(mcpNoTranscriptsEnv, "1")
	if err := gateWith(delegatedToken(ownTenant)); exitCodeOf(err) != 2 || !strings.Contains(err.Error(), "switched off") {
		t.Errorf("MCP off switch: want refusal, got %v", err)
	}
}

func TestMeetingTranscriptsPath(t *testing.T) {
	del, app := tokenClaims{Scp: "x"}, tokenClaims{Roles: []string{"r"}}
	if p, err := meetingTranscriptsPath(del, "", "m/1"); err != nil || p != "/me/onlineMeetings/m%2F1/transcripts" {
		t.Errorf("delegated: %q %v", p, err)
	}
	if p, err := meetingTranscriptsPath(app, "u1", "m1"); err != nil || p != "/users/u1/onlineMeetings/m1/transcripts" {
		t.Errorf("app-only: %q %v", p, err)
	}
	for name, err := range map[string]error{
		"no meeting":        func() error { _, e := meetingTranscriptsPath(del, "", ""); return e }(),
		"app-only, no user": func() error { _, e := meetingTranscriptsPath(app, "", "m1"); return e }(),
		"delegated, --user": func() error { _, e := meetingTranscriptsPath(del, "u1", "m1"); return e }(),
	} {
		if exitCodeOf(err) != 2 {
			t.Errorf("%s: want usage error, got %v", name, err)
		}
	}
}

const vttBody = "WEBVTT\n\n00:00:00.000 --> 00:00:02.000\n<v Ada>TranscriptLineMarker-6b2f</v>\n"

// transcriptsFake is a Graph fake that records each request it got.
func transcriptsFake(t *testing.T) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL.Path+"?"+r.URL.RawQuery+" accept="+r.Header.Get("Accept"))
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/content") {
			w.Header().Set("Content-Type", "text/vtt")
			_, _ = w.Write([]byte(vttBody))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"t1","meetingId":"m1","createdDateTime":"2026-10-20T10:00:00Z"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), got...) }
}

func runTranscripts(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func TestTranscriptsCommandsAgainstGraph(t *testing.T) {
	testenv.Isolate(t)
	srv, requests := transcriptsFake(t)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv(homeTenantEnv, ownTenant)
	t.Setenv("MICROSOFT_GRAPH_TOKEN", delegatedToken(ownTenant))

	out, err := runTranscripts(t, "transcripts", "list", "--meeting", "m1", "--json")
	if err != nil || !strings.Contains(out, `"t1"`) {
		t.Fatalf("list: %v\n%s", err, out)
	}
	file := filepath.Join(t.TempDir(), "meeting.vtt")
	if _, err := runTranscripts(t, "transcripts", "download", "--meeting", "m1", "--transcript", "t1", "--out", file); err != nil {
		t.Fatalf("download --out: %v", err)
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != vttBody {
		t.Fatalf("download --out wrote %q (%v)", b, err)
	}
	if info, _ := os.Stat(file); runtime.GOOS != "windows" && info != nil && info.Mode().Perm() != 0o600 {
		t.Errorf("--out mode %v, want 0600", info.Mode().Perm())
	}
	out, err = runTranscripts(t, "transcripts", "download", "--meeting", "m1", "--transcript", "t1", "--no-speakers")
	if err != nil || !strings.Contains(out, "TranscriptLineMarker-6b2f") {
		t.Fatalf("download to stdout: %v\n%s", err, out)
	}
	if _, err := runTranscripts(t, "transcripts", "all", "--organizer", "u1"); exitCodeOf(err) != 4 {
		t.Errorf("all with a delegated token: want auth refusal, got %v", err)
	}

	t.Setenv("MICROSOFT_GRAPH_TOKEN", appToken(ownTenant))
	t.Setenv(transcriptsAppOnlyEnv, "1")
	if _, err := runTranscripts(t, "transcripts", "list", "--meeting", "m1", "--user", "u1"); err != nil {
		t.Fatalf("app-only list: %v", err)
	}
	if _, err := runTranscripts(t, "transcripts", "all", "--organizer", "u1", "--since", "7d"); err != nil {
		t.Fatalf("all: %v", err)
	}
	if _, err := runTranscripts(t, "transcripts", "all", "--organizer", "u1')/x"); exitCodeOf(err) != 2 {
		t.Errorf("an organizer that breaks out of the OData parameter: want usage error, got %v", err)
	}

	got := strings.Join(requests(), "\n")
	for _, want := range []string{
		"/v1.0/me/onlineMeetings/m1/transcripts?",
		"/v1.0/me/onlineMeetings/m1/transcripts/t1/content?%24format=text%2Fvtt",
		"/v1.0/me/onlineMeetings/m1/transcripts/t1/content? accept=application/vnd.microsoft.graph.transcript+text",
		"/v1.0/users/u1/onlineMeetings/m1/transcripts?",
		"/v1.0/users/u1/onlineMeetings/getAllTranscripts(meetingOrganizerUserId='u1',startDateTime=",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no request %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/x") {
		t.Errorf("the refused organizer reached Graph:\n%s", got)
	}

	// A client tenant's token sends nothing at all.
	before := len(requests())
	t.Setenv("MICROSOFT_GRAPH_TOKEN", appToken(clientTenant))
	if _, err := runTranscripts(t, "transcripts", "list", "--meeting", "m1", "--user", "u1"); exitCodeOf(err) != 4 {
		t.Errorf("client tenant: want auth refusal, got %v", err)
	}
	if n := len(requests()); n != before {
		t.Errorf("a refused client-tenant token still sent %d request(s)", n-before)
	}
}

// Through MCP a child never writes a file, and the off switch hides the subtree.
func TestTranscriptsMCPNeverWritesAFile(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	file := filepath.Join(t.TempDir(), "x.vtt")
	_, err := runTranscripts(t, "transcripts", "download", "--meeting", "m1", "--transcript", "t1", "--out", file)
	if exitCodeOf(err) != 2 || !strings.Contains(err.Error(), "not available through MCP") {
		t.Fatalf("MCP --out: want usage refusal, got %v", err)
	}
	if _, statErr := os.Stat(file); statErr == nil {
		t.Fatal("an MCP child wrote the --out file")
	}
	t.Setenv(mcpNoTranscriptsEnv, "1")
	for _, c := range RootCmd().Commands() {
		if c.Name() == "transcripts" && c.Annotations["mcp:hidden"] != "true" {
			t.Fatal("MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1 left the transcripts tools registered")
		}
	}
}

// Codex r1 class cross-tenant transcript read: the gate judges the credential
// that is sent, not the one the configuration resolves first. A config file's
// [headers] Authorization replaces the token on the wire, so it is the one
// checked, and every request is checked again as it leaves.
func TestTranscriptsGateJudgesTheSentCredential(t *testing.T) {
	testenv.Isolate(t)
	srv, requests := transcriptsFake(t)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv(homeTenantEnv, ownTenant)
	t.Setenv("MICROSOFT_GRAPH_TOKEN", delegatedToken(ownTenant))
	cfg := filepath.Join(t.TempDir(), "cross.toml")
	if err := os.WriteFile(cfg, []byte("[headers]\nAuthorization = \"Bearer "+delegatedToken(clientTenant)+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runTranscripts(t, "--config", cfg, "transcripts", "list", "--meeting", "m1"); exitCodeOf(err) != 4 {
		t.Errorf("a client tenant's token in [headers]: want auth refusal, got %v", err)
	}
	if got := requests(); len(got) != 0 {
		t.Errorf("the client tenant's token reached Graph: %v", got)
	}

	// On the wire: a header the gate never saw (a per-request override) is refused.
	c := &client.Client{Config: &config.Config{MicrosoftGraphToken: delegatedToken(ownTenant)}, HTTPClient: &http.Client{}}
	if _, err := transcriptsGate(c); err != nil {
		t.Fatal(err)
	}
	for _, auth := range []string{"Bearer " + delegatedToken(clientTenant), "Bearer " + appToken(ownTenant), ""} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1.0/me/onlineMeetings/m1/transcripts", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		if resp, err := c.HTTPClient.Do(req); err == nil {
			resp.Body.Close()
			t.Errorf("a request with Authorization %.40q was sent", auth)
		}
	}
	if got := requests(); len(got) != 0 {
		t.Errorf("a refused credential reached Graph: %v", got)
	}
}

// Codex r1 class transcript persisted outside --out: every other output sink is
// refused before any request (--deliver file or webhook, run receipts, audit
// directories), so the transcript reaches stdout, an MCP result or the --out
// file the operator named, and nothing else.
func TestTranscriptsRefuseEveryOtherSink(t *testing.T) {
	testenv.Isolate(t)
	srv, requests := transcriptsFake(t)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv(homeTenantEnv, ownTenant)
	t.Setenv("MICROSOFT_GRAPH_TOKEN", delegatedToken(ownTenant))
	dir := t.TempDir()
	for name, sink := range map[string][]string{
		"deliver file":    {"--deliver", "file:" + filepath.Join(dir, "kept.vtt")},
		"deliver webhook": {"--deliver", "webhook:" + srv.URL + "/hook"},
		"receipt":         {"--receipt"},
		"receipt file":    {"--receipt-file", filepath.Join(dir, "receipt.json")},
		"audit dir":       {"--audit-dir", filepath.Join(dir, "audit")},
	} {
		for _, leaf := range [][]string{
			{"transcripts", "download", "--meeting", "m1", "--transcript", "t1"},
			{"transcripts", "list", "--meeting", "m1"},
			{"transcripts", "all", "--organizer", "u1"},
		} {
			if _, err := runTranscripts(t, append(append([]string(nil), leaf...), sink...)...); exitCodeOf(err) != 2 {
				t.Errorf("%s %s: want usage refusal, got %v", name, leaf[1], err)
			}
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a refused sink still wrote %d file(s) in %s", len(entries), dir)
	}
	if got := requests(); len(got) != 0 {
		t.Errorf("a refused sink still fetched: %v", got)
	}
}
