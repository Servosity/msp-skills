// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 1 (press 4.34.0 reprint), class "a vendor write without
// the operator's confirmation": the generated client sent any JSON-RPC method
// a raw endpoint command, a --stdin body or an MCP tool handed it. The guard
// is the one point every generated request body passes. Hand-fix code:
// cove-rpc-read-method-guard.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
	"cove-pp-cli/internal/config"
)

func rpcGuardClient(t *testing.T) (*Client, *atomic.Int32) {
	t.Helper()
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"jsonrpc":"2.0","result":{"result":{"Id":42}}}`))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	return c, &hits
}

// jsonEscape is the six-character JSON escape for the code unit hex, built at
// run time so no tool or editor decodes it on the way into this file.
func jsonEscape(hex string) string { return string(rune(92)) + "u" + hex }

func TestGeneratedClientRefusesEveryNonReadRPCBody(t *testing.T) {
	for name, body := range map[string]any{
		"write method":           map[string]any{"jsonrpc": "2.0", "id": 1, "method": "RemoveAccount", "params": map[string]any{"accountId": 42}},
		"batch read then write":  []any{map[string]any{"jsonrpc": "2.0", "id": 1, "method": "GetServerInfo"}, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "RemoveAccount"}},
		"batch write then read":  []any{map[string]any{"jsonrpc": "2.0", "id": 1, "method": "RemoveAccount"}, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "GetServerInfo"}},
		"empty batch":            []any{},
		"batch of a non-object":  []any{"GetServerInfo"},
		"escaped write value":    json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"` + jsonEscape("0052") + `emoveAccount"}`),
		"escaped method key":     json.RawMessage(`{"jsonrpc":"2.0","id":1,"` + jsonEscape("006d") + `ethod":"RemoveAccount"}`),
		"duplicate, write last":  json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"GetServerInfo","method":"RemoveAccount"}`),
		"duplicate, write first": json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"RemoveAccount","method":"GetServerInfo"}`),
		"duplicate reads":        json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"GetServerInfo","method":"GetServerInfo"}`),
		"case-folded second key": json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"GetServerInfo","METHOD":"RemoveAccount"}`),
		"only a case-folded key": json.RawMessage(`{"jsonrpc":"2.0","id":1,"Method":"GetServerInfo"}`),
		"no method":              map[string]any{"jsonrpc": "2.0", "id": 1},
		"empty object":           map[string]any{},
		"empty method":           map[string]any{"method": ""},
		"non-string method":      map[string]any{"method": 7},
		"null method":            map[string]any{"method": nil},
		"scalar body":            "GetServerInfo",
		"null body":              json.RawMessage(`null`),
		"read prefix, no name":   map[string]any{"method": "Get"},
		"read prefix, lower":     map[string]any{"method": "Getaway"},
		"upper-case prefix":      map[string]any{"method": "GETRemoveAccount"},
		"lower-case prefix":      map[string]any{"method": "getAccountInfoById"},
		"leading space":          map[string]any{"method": " GetServerInfo"},
		"trailing space":         map[string]any{"method": "GetServerInfo "},
		"embedded NUL":           map[string]any{"method": "GetServerInfo\x00RemoveAccount"},
		"non-ASCII letter":       map[string]any{"method": "GetÄccount"},
	} {
		t.Run(name, func(t *testing.T) {
			c, hits := rpcGuardClient(t)
			_, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, body)
			if err == nil {
				t.Fatalf("body %v was sent", body)
			}
			if !errors.Is(err, ErrRPCRequestRefused) {
				t.Fatalf("body %v: error %v is not ErrRPCRequestRefused", body, err)
			}
			if n := hits.Load(); n != 0 {
				t.Fatalf("body %v reached the server %d time(s)", body, n)
			}
		})
	}
}

func TestGeneratedClientRefusesAWriteOnEveryVerb(t *testing.T) {
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "RemoveAccount"}
	c, hits := rpcGuardClient(t)
	ctx := context.Background()
	if _, _, err := c.Post(ctx, "/jsonapi", body); !errors.Is(err, ErrRPCRequestRefused) {
		t.Fatalf("Post: %v", err)
	}
	if _, _, err := c.PostWithParams(ctx, "/somewhere-else", nil, body); !errors.Is(err, ErrRPCRequestRefused) {
		t.Fatalf("PostWithParams to another path: %v", err)
	}
	if _, _, err := c.Put(ctx, "/jsonapi", body); !errors.Is(err, ErrRPCRequestRefused) {
		t.Fatalf("Put: %v", err)
	}
	if _, _, err := c.DeleteWithBody(ctx, "/jsonapi", body); !errors.Is(err, ErrRPCRequestRefused) {
		t.Fatalf("DeleteWithBody: %v", err)
	}
	if _, err := c.Get(ctx, "/jsonapi", map[string]string{"method": "RemoveAccount"}); !errors.Is(err, ErrRPCRequestRefused) {
		t.Fatalf("Get with a method query parameter: %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("a write reached the server %d time(s)", n)
	}
}

func TestGeneratedClientRefusesAWriteUnderDryRun(t *testing.T) {
	c, hits := rpcGuardClient(t)
	c.DryRun = true
	if _, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, map[string]any{"method": "RemoveAccount"}); !errors.Is(err, ErrRPCRequestRefused) {
		t.Fatalf("dry-run preview of a write was not refused: %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("dry-run reached the server %d time(s)", n)
	}
}

func TestGeneratedClientStillSendsReads(t *testing.T) {
	for name, body := range map[string]any{
		"get":            map[string]any{"jsonrpc": "2.0", "id": 1, "method": "GetAccountInfoById", "params": map[string]any{"accountId": 42}},
		"enumerate":      map[string]any{"jsonrpc": "2.0", "id": 1, "method": "EnumerateAccountStatistics"},
		"escaped read":   json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"` + jsonEscape("0047") + `etServerInfo"}`),
		"batch of reads": []any{map[string]any{"method": "GetServerInfo"}, map[string]any{"method": "EnumeratePartners"}},
		"nested method":  map[string]any{"method": "GetServerInfo", "params": map[string]any{"method": "RemoveAccount"}},
	} {
		t.Run(name, func(t *testing.T) {
			c, hits := rpcGuardClient(t)
			if _, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, body); err != nil {
				t.Fatalf("read %v refused: %v", body, err)
			}
			if n := hits.Load(); n != 1 {
				t.Fatalf("read %v reached the server %d time(s), want 1", body, n)
			}
		})
	}
	// A body-less request (the doctor reachability probe) carries no method.
	c, hits := rpcGuardClient(t)
	if _, err := c.Get(context.Background(), "/jsonapi", nil); err != nil {
		t.Fatalf("body-less GET refused: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("body-less GET was not sent")
	}
}

// Wire bytes the generated client never marshals itself but the guard must
// still judge (a json.RawMessage body is re-compacted, not re-encoded).
func TestRPCGuardJudgesRawWireBytes(t *testing.T) {
	refused := map[string]string{
		"invalid JSON":         `{"method":"GetServerInfo"`,
		"trailing request":     `{"method":"GetServerInfo"}{"method":"RemoveAccount"}`,
		"trailing garbage":     `{"method":"GetServerInfo"} x`,
		"invalid UTF-8":        "{\"method\":\"GetServerInfo\",\"params\":\"\xff\"}",
		"lone surrogate key":   `{"` + jsonEscape("d800") + `method":"RemoveAccount","method":"GetServerInfo"}`,
		"NUL-suffixed key":     `{"method` + jsonEscape("0000") + `":"RemoveAccount","method":"GetServerInfo"}`,
		"unknown member":       `{"method":"GetServerInfo","action":"RemoveAccount"}`,
		"duplicate visa":       `{"method":"GetServerInfo","visa":"a","visa":"b"}`,
		"nested batch":         `[[{"method":"GetServerInfo"}]]`,
		"escaped NUL in value": `{"method":"GetServerInfo` + jsonEscape("0000") + `RemoveAccount"}`,
	}
	for name, body := range refused {
		if err := coveRefuseNonReadRPC("/jsonapi", nil, []byte(body)); !errors.Is(err, ErrRPCRequestRefused) {
			t.Errorf("%s: %q was not refused (%v)", name, body, err)
		}
	}
	for name, body := range map[string]string{
		"read":            `{"jsonrpc":"2.0","id":1,"method":"GetServerInfo","params":{},"visa":"v"}`,
		"spaced read":     " \n{ \"method\" : \"EnumerateAccounts\" }\n",
		"batch of reads":  `[{"method":"GetServerInfo"},{"method":"EnumeratePartners"}]`,
		"no body":         "",
		"params anything": `{"method":"GetServerInfo","params":{"method":"RemoveAccount","Method":["x"]}}`,
	} {
		raw := []byte(body)
		if body == "" {
			raw = nil
		}
		if err := coveRefuseNonReadRPC("/jsonapi", nil, raw); err != nil {
			t.Errorf("%s: %q refused: %v", name, body, err)
		}
	}
	if err := coveRefuseNonReadRPC("/jsonapi", map[string]string{" Method ": "RemoveAccount"}, nil); !errors.Is(err, ErrRPCRequestRefused) {
		t.Errorf("a method-like query parameter naming a write was not refused: %v", err)
	}
	if err := coveRefuseNonReadRPC("/jsonapi", map[string]string{"limit": "RemoveAccount"}, nil); err != nil {
		t.Errorf("an unrelated query parameter was refused: %v", err)
	}
	err := coveRefuseNonReadRPC("/jsonapi", nil, []byte(`{"method":"RemoveAccount"}`))
	if err == nil || !strings.Contains(err.Error(), "cove-cli call <Method> --yes") || !strings.Contains(err.Error(), `"RemoveAccount"`) {
		t.Errorf("write refusal does not name the method and `cove-cli call <Method> --yes`: %v", err)
	}
}

// The strict check covers the JSON-RPC endpoint in every spelling a server
// might route there, and a method-bearing body on any path; a body sent to
// another path that names no method calls no method and is left alone (the
// press's generic transport tests post such bodies).
func TestRPCGuardScopesByPathAndMethodMembers(t *testing.T) {
	for _, path := range []string{"/jsonapi", "/jsonapi/", "/JSONAPI", "jsonapi", "/jsonapi?x=1", "/jsonapi#f"} {
		if err := coveRefuseNonReadRPC(path, nil, []byte(`{"synthetic":true}`)); !errors.Is(err, ErrRPCRequestRefused) {
			t.Errorf("method-less body to %q was not refused: %v", path, err)
		}
	}
	for _, body := range []string{
		`{"synthetic":true,"method":"RemoveAccount"}`,
		`{"synthetic":true," METHOD":"GetServerInfo"}`,
		`[{"a":1},{"method":"RemoveAccount"}]`,
		`[[{"method":"RemoveAccount"}]]`,
		"{\"method\":\"GetServerInfo\",\"x\":\"\xff\"}",
	} {
		if err := coveRefuseNonReadRPC("/mutation", nil, []byte(body)); !errors.Is(err, ErrRPCRequestRefused) {
			t.Errorf("method-bearing body %q to another path was not refused: %v", body, err)
		}
	}
	for _, body := range []string{`{"synthetic":true}`, `[{"a":1}]`, `"text"`, `{"params":{"method":"RemoveAccount"}}`} {
		if err := coveRefuseNonReadRPC("/mutation", nil, []byte(body)); err != nil {
			t.Errorf("method-less body %q to another path refused: %v", body, err)
		}
	}
	if err := coveRefuseNonReadRPC("/mutation", nil, []byte(`{"method":"GetServerInfo"}`)); err != nil {
		t.Errorf("a read to another path refused: %v", err)
	}
}
