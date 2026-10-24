// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 1 (press 4.34.0 reprint), class "a vendor write without
// the operator's confirmation": the raw generated endpoint commands sent
// whatever JSON-RPC method --method or a --stdin body named, so
// `devices get --method RemoveAccount` removed a device without --yes. Each
// test below failed on the code Codex reviewed. Hand-fix code:
// cove-rpc-read-method-guard.
package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"cove-pp-cli/internal/coverpc"
)

// rawRPCServer records every request the generated client sends.
type rawRPCServer struct {
	srv    *httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
}

func (s *rawRPCServer) requests() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.bodies...)
}

func newRawRPCServer(t *testing.T) *rawRPCServer {
	t.Helper()
	s := &rawRPCServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if body == nil {
			body = map[string]any{"_raw": string(raw), "_verb": r.Method}
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"jsonrpc":"2.0","result":{"result":{"Id":42}}}`)
	}))
	t.Cleanup(s.srv.Close)
	fleetTestEnv(t, s.srv)
	return s
}

// jsonEscape is the six-character JSON escape for the code unit hex, built at
// run time so no tool or editor decodes it on the way into this file.
func jsonEscape(hex string) string { return string(rune(92)) + "u" + hex }

// withStdin feeds body to the command's os.Stdin for the rest of the test.
func withStdin(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = old
		_ = f.Close()
	})
}

func TestEndpointCommandRefusesAWriteMethod(t *testing.T) {
	s := newRawRPCServer(t)
	out, err := runCoveCmd(t, "devices", "get", "--method", "RemoveAccount", "--params-account-id", "42", "--visa", "visa-0123456789", "--agent")
	if err == nil || ExitCode(err) == 0 {
		t.Fatalf("devices get --method RemoveAccount succeeded (exit %d):\n%s", ExitCode(err), out)
	}
	if n := len(s.requests()); n != 0 {
		t.Fatalf("devices get --method RemoveAccount sent %d request(s): %v", n, s.requests())
	}
	if !strings.Contains(err.Error(), "cove-cli call <Method>") {
		t.Fatalf("refusal does not name `cove-cli call <Method>`: %v", err)
	}
}

// The endpoint's spec method is pinned: another method, even a read, is not
// what `devices get` documents, so it is refused before anything is sent.
func TestEndpointCommandPinsItsSpecMethod(t *testing.T) {
	s := newRawRPCServer(t)
	if out, err := runCoveCmd(t, "devices", "get", "--method", "GetPartnerInfoById", "--params-account-id", "42", "--visa", "visa-0123456789", "--agent"); err == nil {
		t.Fatalf("devices get --method GetPartnerInfoById was not refused:\n%s", out)
	}
	if out, err := runCoveCmd(t, "devices", "get", "--method", "", "--params-account-id", "42", "--visa", "visa-0123456789", "--agent"); err == nil {
		t.Fatalf("devices get --method '' was not refused:\n%s", out)
	}
	if n := len(s.requests()); n != 0 {
		t.Fatalf("a pinned endpoint sent %d request(s) for another method: %v", n, s.requests())
	}
	if out, err := runCoveCmd(t, "devices", "get", "--method", "GetAccountInfoById", "--params-account-id", "42", "--visa", "visa-0123456789", "--json"); err != nil {
		t.Fatalf("devices get --method <its own spec method> refused: %v\n%s", err, out)
	}
}

func TestEndpointStdinBodyRefusesAWriteMethod(t *testing.T) {
	for name, body := range map[string]string{
		"plain":                  `{"jsonrpc":"2.0","id":1,"method":"RemoveAccount","visa":"visa-0123456789","params":{"accountId":42}}`,
		"escaped method value":   `{"jsonrpc":"2.0","id":1,"method":"` + jsonEscape("0052") + `emoveAccount","visa":"visa-0123456789","params":{"accountId":42}}`,
		"escaped method key":     `{"jsonrpc":"2.0","id":1,"` + jsonEscape("006d") + `ethod":"RemoveAccount","visa":"visa-0123456789","params":{"accountId":42}}`,
		"second method by case":  `{"jsonrpc":"2.0","id":1,"method":"GetAccountInfoById","Method":"RemoveAccount","visa":"visa-0123456789"}`,
		"second method by space": `{"jsonrpc":"2.0","id":1,"method":"GetAccountInfoById"," method ":"RemoveAccount","visa":"visa-0123456789"}`,
		"no method":              `{"jsonrpc":"2.0","id":1,"visa":"visa-0123456789","params":{"accountId":42}}`,
		"non-string method":      `{"jsonrpc":"2.0","id":1,"method":["RemoveAccount"],"visa":"visa-0123456789"}`,
		"lower-case write":       `{"jsonrpc":"2.0","id":1,"method":"removeAccount","visa":"visa-0123456789"}`,
		"padded read prefix":     `{"jsonrpc":"2.0","id":1,"method":"GetX RemoveAccount","visa":"visa-0123456789"}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := newRawRPCServer(t)
			withStdin(t, body)
			out, err := runCoveCmd(t, "devices", "get", "--stdin", "--agent")
			if err == nil || ExitCode(err) == 0 {
				t.Fatalf("stdin body was sent (exit %d):\n%s", ExitCode(err), out)
			}
			if n := len(s.requests()); n != 0 {
				t.Fatalf("stdin body sent %d request(s): %v", n, s.requests())
			}
		})
	}
}

func TestEndpointStdinBodyStillSendsARead(t *testing.T) {
	s := newRawRPCServer(t)
	withStdin(t, `{"jsonrpc":"2.0","id":1,"method":"GetAccountInfoById","visa":"visa-0123456789","params":{"accountId":42}}`)
	if out, err := runCoveCmd(t, "devices", "get", "--stdin", "--json"); err != nil {
		t.Fatalf("stdin read refused: %v\n%s", err, out)
	}
	reqs := s.requests()
	if len(reqs) != 1 || reqs[0]["method"] != "GetAccountInfoById" {
		t.Fatalf("stdin read: requests = %v", reqs)
	}
}

func TestEndpointCommandDefaultReadStillWorks(t *testing.T) {
	s := newRawRPCServer(t)
	out, err := runCoveCmd(t, "devices", "get", "--params-account-id", "42", "--visa", "visa-0123456789", "--json")
	if err != nil {
		t.Fatalf("devices get (default method) failed: %v\n%s", err, out)
	}
	reqs := s.requests()
	if len(reqs) != 1 {
		t.Fatalf("devices get sent %d requests, want 1: %v", len(reqs), reqs)
	}
	got := reqs[0]
	if got["method"] != "GetAccountInfoById" || got["jsonrpc"] != "2.0" || got["id"] != float64(1) {
		t.Fatalf("devices get envelope = %v", got)
	}
}

// Every raw generated endpoint command sends exactly one request, and its
// method is a non-empty read: the one its spec names (coverpc.EndpointMethod)
// and its own help text names. On the press 4.34.0 reprint four of them
// (audit, devices stats, storage nodes, storage stats) were printed with an
// empty --method default and sent no method at all.
func TestEveryEndpointCommandSendsItsSpecReadMethod(t *testing.T) {
	root := newRootCmd(&rootFlags{})
	seen := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		id := c.Annotations["pp:endpoint"]
		if id == "" {
			return
		}
		seen[id] = true
		want, ok := coverpc.EndpointMethod(id)
		if !ok {
			t.Errorf("%s (%s) has no spec method in coverpc.EndpointMethod", c.CommandPath(), id)
			return
		}
		if !coverpc.IsReadMethod(want) {
			t.Errorf("%s: spec method %q is not a read", id, want)
		}
		if !strings.Contains(c.Short, "(JSON-RPC "+want) {
			t.Errorf("%s: help %q does not name %s", c.CommandPath(), c.Short, want)
		}
		args := strings.Fields(strings.TrimPrefix(c.CommandPath(), root.Name()+" "))
		for _, a := range strings.Fields(c.Annotations["pp:happy-args"]) {
			value := "9000"
			if strings.Contains(a, "-ids=") {
				value = "[9000]"
			}
			args = append(args, strings.ReplaceAll(a, "example-value", value))
		}
		args = append(args, "--visa", "visa-0123456789", "--json")
		t.Run(id, func(t *testing.T) {
			s := newRawRPCServer(t)
			if out, err := runCoveCmd(t, args...); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}
			reqs := s.requests()
			if len(reqs) != 1 {
				t.Fatalf("%v sent %d requests, want 1: %v", args, len(reqs), reqs)
			}
			got, _ := reqs[0]["method"].(string)
			if got == "" || !coverpc.IsReadMethod(got) || got != want {
				t.Fatalf("%v sent method %q, want the read %s (body %v)", args, got, want, reqs[0])
			}
			if reqs[0]["jsonrpc"] != "2.0" || reqs[0]["id"] != float64(1) {
				t.Fatalf("%v envelope = %v", args, reqs[0])
			}
		})
	}
	walk(root)
	if len(seen) < 16 {
		t.Fatalf("walked %d raw endpoint commands, want at least 16: %v", len(seen), seen)
	}
	for _, id := range coverpc.EndpointIDs() {
		if !seen[id] {
			t.Errorf("coverpc.EndpointMethod names %s but no command carries it", id)
		}
	}
}

// `call` is unchanged: a read is sent, a write needs --yes and then goes out
// through the coverpc client.
func TestCallStillReadsAndConfirmsWrites(t *testing.T) {
	m := newRPCMock(t, func(string, map[string]any) any { return map[string]any{"ok": true} })
	fleetTestEnv(t, m.srv)
	if out, err := runCoveCmd(t, "call", "GetServerInfo", "--agent"); err != nil || m.count("GetServerInfo") != 1 {
		t.Fatalf("call GetServerInfo: err=%v sent=%d\n%s", err, m.count("GetServerInfo"), out)
	}
	if out, err := runCoveCmd(t, "call", "RemoveAccount", "--params", `{"accountId":42}`, "--agent"); err == nil || m.count("RemoveAccount") != 0 {
		t.Fatalf("call RemoveAccount without --yes: err=%v sent=%d\n%s", err, m.count("RemoveAccount"), out)
	}
	if out, err := runCoveCmd(t, "call", "RemoveAccount", "--params", `{"accountId":42}`, "--yes", "--agent"); err != nil || m.count("RemoveAccount") != 1 {
		t.Fatalf("call RemoveAccount --yes: err=%v sent=%d\n%s", err, m.count("RemoveAccount"), out)
	}
}
