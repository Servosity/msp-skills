// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keeper-pp-cli/internal/cliutil/testenv"
)

const recordSentinel = "VAULTSENTINEL-9f3aZ7"

type hostileAnswer struct {
	status int
	body   string
}

// hostileServer answers one Commander command or one GET path with a hostile body and
// everything else like an honest Service Mode.
func hostileServer(t *testing.T, command, path string, a hostileAnswer) {
	t.Helper()
	honest := (&fakeServiceMode{twoFA: true}).handler(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var rq struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(body, &rq)
		if (command != "" && strings.HasPrefix(rq.Command, command)) || (path != "" && r.URL.Path == path) {
			w.WriteHeader(a.status)
			_, _ = w.Write([]byte(a.body))
			return
		}
		switch {
		case r.Method != http.MethodGet:
			honest(w, r)
		case r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("KEEPER_BASE_URL", srv.URL)
	t.Setenv("KEEPER_API_KEY", "Zq7mW2xR9vT4bN6kL1pS8cH3")
}

// noSentinel fails when the record content reached the output or any file under home.
func noSentinel(t *testing.T, home, what, out string, err error) {
	t.Helper()
	if strings.Contains(out, recordSentinel) {
		t.Errorf("%s printed record content:\n%s", what, out)
	}
	if err != nil && strings.Contains(err.Error(), recordSentinel) {
		t.Errorf("%s error carries record content: %v", what, err)
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && bytes.Contains(b, []byte(recordSentinel)) {
			t.Errorf("%s wrote record content to %s", what, p)
		}
		return nil
	})
}

// Failing input (adversarial review): a Commander error or message member carrying
// record content skipped the record check, which ran only on data rows, and reached
// stderr, posture stdout, the store and MCP results.
func TestErrorAndMessageMembersCarryNoRecordContent(t *testing.T) {
	s := recordSentinel
	for i, a := range []hostileAnswer{
		{200, fmt.Sprintf(`{"status":"error","error":{"title":"Server","notes":%q,"login_url":"https://x"}}`, s)},
		{200, fmt.Sprintf(`{"status":"error","error":{"detail":%q}}`, s)},
		{200, fmt.Sprintf(`{"status":"success","message":{"fields":[{"type":"note","value":[%q]}]}}`, s)},
		{500, fmt.Sprintf(`{"status":"error","error":"otpauth://totp/x?secret=%s"}`, s)},
		{500, fmt.Sprintf(`{"status":"error","error":"-----BEGIN RSA PRIVATE KEY-----\n%s"}`, s)},
		{500, fmt.Sprintf(`[{"notes":%q}]`, s)},
		{500, fmt.Sprintf(`otpauth://totp/x?secret=%s`, s)},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			home := testenv.Isolate(t)
			hostileServer(t, "security-audit-report", "", a)
			for _, args := range [][]string{
				{"security-audit", "report", "--mc", "4421", "--json"},
				{"posture", "report", "--mc", "4421", "--json"},
			} {
				out, err := runKeeperCLI(t, args...)
				noSentinel(t, home, strings.Join(args[:2], " "), out, err)
			}
		})
	}
}

// Failing input (adversarial review): health and queue printed the Service Mode body
// with no record check.
func TestServiceModeReadsCarryNoRecordContent(t *testing.T) {
	s := recordSentinel
	for _, tool := range []struct{ cmd, path string }{{"health", "/health"}, {"queue", "/api/v2/queue/status"}} {
		for i, a := range []hostileAnswer{
			{200, fmt.Sprintf(`{"status":"ok","notes":%q}`, s)},
			{200, fmt.Sprintf(`{"status":"ok","nested":{"fields":[{"type":"password","value":[%q]}]}}`, s)},
			{200, fmt.Sprintf(`{"status":"ok","x":"otpauth://totp/x?secret=%s"}`, s)},
			{500, fmt.Sprintf(`{"notes":%q}`, s)},
			{500, fmt.Sprintf(`otpauth://totp/x?secret=%s`, s)},
		} {
			t.Run(fmt.Sprintf("%s/%d", tool.cmd, i), func(t *testing.T) {
				home := testenv.Isolate(t)
				hostileServer(t, "", tool.path, a)
				out, err := runKeeperCLI(t, tool.cmd, "--json")
				noSentinel(t, home, tool.cmd, out, err)
				if err == nil {
					t.Errorf("%s exited clean on a record-shaped answer:\n%s", tool.cmd, out)
				}
			})
		}
	}
	// The honest answers still print.
	for _, tool := range []struct{ cmd, path, body, want string }{
		{"health", "/health", `{"status":"ok"}`, `"ok"`},
		{"queue", "/api/v2/queue/status", `{"status":"success","queue_size":2,"active_requests":1,"completed_requests":9,"currently_processing":null,"worker_running":true}`, `"queue_size"`},
	} {
		testenv.Isolate(t)
		hostileServer(t, "", tool.path, hostileAnswer{200, tool.body})
		out, err := runKeeperCLI(t, tool.cmd, "--json")
		if err != nil || !strings.Contains(out, tool.want) {
			t.Errorf("%s on an honest answer: %v\n%s", tool.cmd, err, out)
		}
	}
}

// Failing input (adversarial review): a Service Mode in legacy mode answers 404 at
// /api/v2/queue/status and serves /api/v1, and doctor told the MSP to change a correct
// base URL. Commander's routes.py registers /health in both modes.
func TestDoctorOnALegacyModeHost(t *testing.T) {
	notFound := hostileAnswer{404, `{"message":"The requested URL was not found on the server."}`}
	testenv.Isolate(t)
	hostileServer(t, "", "/api/v2/queue/status", notFound)
	out, err := runKeeperCLI(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, "is not a Keeper Service Mode API") || !strings.Contains(out, "in legacy mode") {
		t.Errorf("doctor on a legacy-mode host:\n%s", out)
	}

	// A host with no /health either is no Service Mode.
	testenv.Isolate(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	t.Setenv("KEEPER_BASE_URL", srv.URL)
	t.Setenv("KEEPER_API_KEY", "Zq7mW2xR9vT4bN6kL1pS8cH3")
	out, err = runKeeperCLI(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "is not a Keeper Service Mode API") {
		t.Errorf("doctor on a wrong host:\n%s", out)
	}
}
