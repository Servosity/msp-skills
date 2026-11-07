// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package msgraphblackbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	bbOwnTenant = "11111111-2222-3333-4444-555555555555"
	// bbTranscriptLine is meeting content: it may reach stdout, an MCP result
	// and the --out file the operator named, and no other file.
	bbTranscriptLine = "TranscriptContentMarker-3c9d"
)

func bbJWT(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	return "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(b) + ".c2ln"
}

// transcriptsServer is a Graph fake for the transcript paths only.
func transcriptsServer(t *testing.T) *server {
	t.Helper()
	sv := &server{}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		sv.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/content") {
			w.Header().Set("Content-Type", "text/vtt")
			_, _ = w.Write([]byte("WEBVTT\n\n00:00:00.000 --> 00:00:02.000\n<v Ada>" + bbTranscriptLine + "</v>\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"t1","meetingId":"m1","createdDateTime":"2026-10-20T10:00:00Z","note":"` + bbTranscriptLine + `"}]}`))
	}))
	sv.url = srv.URL + "/v1.0"
	sv.caFile = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(sv.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return sv
}

// transcripts-own-tenant: nothing is kept. Every transcripts path runs, through
// the CLI and through MCP, with journaling on; afterwards no file the program
// wrote holds meeting content or a trace of the transcripts run, except the
// --out file the operator named on the command line.
func TestTranscriptsKeepNothingOnDisk(t *testing.T) {
	sv := transcriptsServer(t)
	e := newEnv(t, sv, false)
	iss := "https://sts.windows.net/" + bbOwnTenant + "/"
	delegated := bbJWT(map[string]any{"tid": bbOwnTenant, "iss": iss, "scp": "OnlineMeetingTranscript.Read.All"})
	app := bbJWT(map[string]any{"tid": bbOwnTenant, "iss": iss, "roles": []string{"OnlineMeetingTranscript.Read.All"}})
	base := append(e.env, "MICROSOFT_GRAPH_HOME_TENANT="+bbOwnTenant)
	outFile := filepath.Join(e.home, "named-by-operator.vtt")

	e.env = append(append([]string(nil), base...), "MICROSOFT_GRAPH_TOKEN="+delegated)
	var shown strings.Builder
	for _, args := range [][]string{
		{"transcripts", "list", "--meeting", "m1", "--json"},
		{"transcripts", "list", "--meeting", "m1"},
		{"transcripts", "download", "--meeting", "m1", "--transcript", "t1"},
		{"transcripts", "download", "--meeting", "m1", "--transcript", "t1", "--agent"},
		{"transcripts", "download", "--meeting", "m1", "--transcript", "t1", "--no-speakers"},
		{"transcripts", "download", "--meeting", "m1", "--transcript", "t1", "--out", outFile},
		// Every other sink is refused (Codex r1 class transcript-persisted-outside-out);
		// the scan below finds any copy they would have kept.
		{"transcripts", "download", "--meeting", "m1", "--transcript", "t1", "--deliver", "file:" + filepath.Join(e.home, "delivered.vtt")},
		{"transcripts", "list", "--meeting", "m1", "--receipt-file", filepath.Join(e.home, "receipt.json")},
		{"transcripts", "list", "--meeting", "m1", "--audit-dir", filepath.Join(e.home, "audit")},
	} {
		shown.WriteString(e.run(t, args...))
	}
	shownCLI := shown.String()
	mcp := e.mcpCalls(t, []map[string]any{
		{"name": "transcripts_list", "arguments": map[string]any{"meeting": "m1"}},
		{"name": "transcripts_download", "arguments": map[string]any{"meeting": "m1", "transcript": "t1"}},
		{"name": "transcripts_download", "arguments": map[string]any{"meeting": "m1", "transcript": "t1", "out": filepath.Join(e.home, "mcp-chosen.vtt")}},
	})
	e.env = append(append([]string(nil), base...), "MICROSOFT_GRAPH_TOKEN="+app, "MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1")
	shownApp := e.run(t, "transcripts", "all", "--organizer", "u1", "--since", "7d", "--json")
	shownApp += e.run(t, "transcripts", "list", "--meeting", "m1", "--user", "u1")

	// The runs really fetched transcripts, so the scan below is not vacuous.
	if strings.Count(shownCLI, bbTranscriptLine) < 5 || !strings.Contains(shownApp, bbTranscriptLine) {
		t.Fatalf("the CLI runs did not show the transcript:\n%s\n%s", shownCLI, shownApp)
	}
	if strings.Count(mcp, bbTranscriptLine) < 2 {
		t.Fatalf("MCP did not return the transcript:\n%s", mcp)
	}
	if b, err := os.ReadFile(outFile); err != nil || !bytes.Contains(b, []byte(bbTranscriptLine)) {
		t.Fatalf("--out file: %q (%v)", b, err)
	}
	var scanned int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == outFile || path == sv.caFile {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		scanned++
		rel, _ := filepath.Rel(e.home, path)
		for _, kept := range []string{bbTranscriptLine, "transcripts", "onlineMeetings", "m1"} {
			if bytes.Contains(data, []byte(kept)) {
				t.Errorf("file %s keeps %q from a transcripts run", rel, kept)
			}
		}
		return nil
	})
	t.Logf("scanned %d other written file(s)", scanned)
	if _, err := os.Stat(filepath.Join(e.home, "mcp-chosen.vtt")); err == nil {
		t.Error("an MCP argument chose a file and it was written")
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	for _, m := range sv.methods {
		if !strings.HasPrefix(m, "GET ") {
			t.Errorf("a transcripts run sent %s", m)
		}
	}
}

// The MCP off switch removes every transcripts tool; nothing reaches Graph.
func TestMCPTranscriptsOffSwitch(t *testing.T) {
	sv := transcriptsServer(t)
	e := newEnv(t, sv, false)
	e.env = append(e.env, "MICROSOFT_GRAPH_HOME_TENANT="+bbOwnTenant, "MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1",
		"MICROSOFT_GRAPH_TOKEN="+bbJWT(map[string]any{"tid": bbOwnTenant, "iss": "x", "scp": "x"}))
	out := e.mcpCalls(t, []map[string]any{
		{"name": "transcripts_list", "arguments": map[string]any{"meeting": "m1"}},
		{"name": "transcripts_download", "arguments": map[string]any{"meeting": "m1", "transcript": "t1"}},
	})
	if strings.Contains(out, bbTranscriptLine) {
		t.Fatalf("the off switch still returned a transcript:\n%s", out)
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if len(sv.methods) != 0 {
		t.Fatalf("the off switch still reached Graph: %v", sv.methods)
	}
}
