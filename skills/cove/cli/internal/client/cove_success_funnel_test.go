// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: tests for the generated endpoint
// commands' success funnel (cove_success_funnel.go). Hand-fix code:
// cove-short-credential-funnel.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
	"cove-pp-cli/internal/config"
)

func coveRawClient(t *testing.T, status int, answer string) *Client {
	t.Helper()
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	return New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
}

func coveRawBody(visa string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": 1, "method": "GetServerInfo", "visa": visa}
}

// The rotated visa every Cove answer carries is dropped from what an endpoint
// command prints, under any spelling of its member name.
func TestCoveRawEndpointDropsTheReturnedVisa(t *testing.T) {
	esc := func(s string) string { return strings.ReplaceAll(s, "~", string(rune(92))) }
	answer := esc(`{"jsonrpc":"2.0","id":1,"visa":"rotated-visa-0123456789","~u0076isa":"rotated-visa-0123456789","VISA":"rotated-visa-0123456789","result":{"result":{"Version":"5.1"}}}`)
	c := coveRawClient(t, 200, answer)
	data, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody("sent-visa-0123456789"))
	if err != nil {
		t.Fatalf("clean answer refused: %v", err)
	}
	if strings.Contains(string(data), "rotated-visa-0123456789") || !strings.Contains(string(data), `"Version":"5.1"`) {
		t.Fatalf("returned visa shown, or the result lost: %s", data)
	}
}

// A JSON-RPC error riding an HTTP 200 answer that repeats the visa the
// request sent, or a credential this process holds, is refused, not printed.
func TestCoveRawEndpointRefusesAnEchoInASuccessAnswer(t *testing.T) {
	for name, answer := range map[string]string{
		"sent visa":       `{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"data":1701,"message":"visa sent-visa-0123456789 expired"}}`,
		"held short":      `{"jsonrpc":"2.0","id":1,"result":{"result":{"Echo":"Hq2@n"}}}`,
		"nested visa":     `{"jsonrpc":"2.0","id":1,"result":{"result":{"visa":"other-visa-0123456789","Note":"other-visa-0123456789"}}}`,
		"numeric echo":    `{"jsonrpc":"2.0","id":1,"result":{"result":{"n":98765432}}}`,
		"not json at all": `visa=sent-visa-0123456789`,
	} {
		t.Run(name, func(t *testing.T) {
			cliutil.FunnelHold("Hq2@n", "98765432")
			c := coveRawClient(t, 200, answer)
			data, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody("sent-visa-0123456789"))
			if err == nil {
				t.Fatalf("an answer repeating a credential was returned: %s", data)
			}
			for _, v := range []string{"sent-visa-0123456789", "other-visa-0123456789", "Hq2@n", "98765432"} {
				if strings.Contains(err.Error(), v) {
					t.Fatalf("the refusal repeats %q: %v", v, err)
				}
			}
		})
	}
}

// A visa the request carries is held, so an HTTP error answer that echoes it
// is withheld by the backstops the error passes.
func TestCoveRawEndpointHoldsTheSentVisa(t *testing.T) {
	c := coveRawClient(t, 400, `{"detail":"visa held-visa-0123456789abc rejected"}`)
	_, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody("held-visa-0123456789abc"))
	if err == nil {
		t.Fatal("expected an HTTP error")
	}
	if shown := cliutil.FunnelHeld(err.Error()); strings.Contains(shown, "held-visa-0123456789abc") {
		t.Fatalf("the sent visa is not held: %s", shown)
	}
}

// Dropping the visa keeps every other member's text as the server wrote it.
func TestCoveRawEndpointKeepsTheAnswerText(t *testing.T) {
	got := string(coveDropVisaMembers([]byte(`{"visa":"v-0123456789","result":{"Name":"AT&T <lab>","n":1.50e2}}`)))
	if got != `{"result":{"Name":"AT&T <lab>","n":1.50e2}}` {
		t.Fatalf("answer text changed: %s", got)
	}
	if got := string(coveDropVisaMembers([]byte(`[{"visa":"v"}]`))); got != `[{"visa":"v"}]` {
		t.Fatalf("a non-object answer changed: %s", got)
	}
}
