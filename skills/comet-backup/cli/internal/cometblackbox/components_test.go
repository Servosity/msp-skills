// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cometblackbox

// The Codex review inputs, end to end through the built binaries (comet-backup
// handfix output-chokepoint, classes held-span and credential-components):
//
//   - a 9,001-character admin password, and a plaintext job log that carries
//     its ASCII85 form with one '&' written as a JSON escape, so the form
//     spans both halves of a search window (backup-jobs log --json);
//   - an organization whose Authorization header holds a seven-character
//     token after the "Token" scheme word, and whose custom body holds a form
//     value, each repeated in a sibling Name field (organizations --json).
//
// Each is checked on the CLI's output, an MCP tool result and the
// --deliver file: sink.

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	bbTokenComp = "Qz7!kV9"          // after "Token" in CustomHeaders.Authorization
	bbBodyComp  = "BodySecret-4411x" // the token= value of RemoteStorage[].Custom.CustomBody
)

func reviewA85JobLog() string {
	var b strings.Builder
	for i := 1; i <= 2250; i++ {
		b.WriteString("5s[e")
		if i == 1125 {
			b.WriteString(string(rune(92)) + "u0026") // the JSON escape of '&', built at run time
		} else {
			b.WriteString("&")
		}
	}
	b.WriteString("5l!")
	return b.String()
}

func reviewServer(t *testing.T) *server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		switch strings.TrimPrefix(r.URL.Path, "/api/v1/admin/") {
		case "get-job-log":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, reviewA85JobLog())
		case "organization/list":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"org-1":{"Name":%q,"CustomHeaders":{"Authorization":"Token %s"},"Description":"Acme MSP"},`+
				`"org-2":{"Name":%q,"RemoteStorage":[{"Type":"custom","Custom":{"URL":"https://bucket.example.com","CustomBody":"token=%s"}}],"Description":"Acme Two"}}`,
				bbTokenComp, bbTokenComp, bbBodyComp, bbBodyComp)
		default:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"Status":200,"Message":"OK"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return &server{url: srv.URL, ca: tlsCAFile(t, srv), sinkURL: srv.URL, hits: map[string]int{}}
}

// withPassword replaces the environment's admin password.
func (e *bbEnv) withPassword(pw string) *bbEnv {
	env := make([]string, 0, len(e.env))
	for _, kv := range e.env {
		if !strings.HasPrefix(kv, "COMET_BACKUP_PASSWORD=") {
			env = append(env, kv)
		}
	}
	return &bbEnv{home: e.home, env: append(env, "COMET_BACKUP_PASSWORD="+pw), sink: e.sink}
}

func TestReviewLongCredentialJobLogWithheld(t *testing.T) {
	t.Parallel()
	e := newEnv(t, reviewServer(t), false).withPassword(strings.Repeat("A", 9001))
	deliver := filepath.Join(e.home, "joblog.json")
	outputs := map[string]string{
		"cli --json": e.run(t, "backup-jobs", "log", "--job-id", "j", "--json"),
		"cli text":   e.run(t, "backup-jobs", "log", "--job-id", "j"),
		"cli deliver": e.run(t, "backup-jobs", "log", "--job-id", "j", "--json", "--deliver", "file:"+deliver) +
			readFile(deliver),
		"mcp": e.mcpCalls(t, []map[string]any{{"name": "comet-backup_read", "arguments": map[string]any{
			"endpoint_id": "backup_jobs.log", "params": map[string]any{"JobID": "j"}}}}),
	}
	for where, out := range outputs {
		assertRan(t, []string{where}, out)
		if strings.Contains(out, "5s[e") {
			t.Errorf("%s printed the ASCII85 form of the 9,001-character password", where)
		}
		if strings.Contains(out, strings.Repeat("A", 64)) {
			t.Errorf("%s printed the password", where)
		}
	}
	if !strings.Contains(outputs["mcp"], `"id":1`) {
		t.Fatalf("the MCP call was not answered:\n%.1500s", outputs["mcp"])
	}
}

func TestReviewComponentsInSiblingFieldsWithheld(t *testing.T) {
	t.Parallel()
	e := newEnv(t, reviewServer(t), false)
	deliver := filepath.Join(e.home, "orgs.json")
	outputs := map[string]string{
		"cli --json": e.run(t, "organizations", "--json"),
		"cli table":  e.run(t, "organizations"),
		"cli deliver": e.run(t, "organizations", "--json", "--deliver", "file:"+deliver) +
			readFile(deliver),
		"mcp": e.mcpCalls(t, []map[string]any{{"name": "comet-backup_read", "arguments": map[string]any{
			"endpoint_id": "organizations.list", "confirm": true}}}),
	}
	for where, out := range outputs {
		assertRan(t, []string{where}, out)
		for _, comp := range []string{bbTokenComp, bbBodyComp} {
			if strings.Contains(out, comp) {
				t.Errorf("%s printed the credential component %q", where, comp)
			}
		}
	}
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "\n[no file: " + err.Error() + "]"
	}
	return "\n" + string(b)
}
