// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the stop-report inputs of the
// press 4.34.0 reprint review, driven through the commands an operator runs.
// Hand-fix code: cove-short-credential-funnel.

package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func coveAnswering(t *testing.T, status int, headers map[string]string, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	fleetTestEnv(t, srv)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

// Round 2 adversarial P1, through the raw endpoint command and through call.
func TestCoveCommandsWithholdAVisaAfterAnotherName(t *testing.T) {
	const fresh = "FRESHVISAq8W3e5R7t9Y1u2I4K"
	for _, body := range []string{"error: visa=" + fresh, "upstream: visa = " + fresh + " (rotated)"} {
		for _, args := range [][]string{
			{"server", "--visa", "sent-visa-0123456789", "--json"},
			{"call", "GetServerInfo"},
		} {
			coveAnswering(t, 400, map[string]string{"Content-Type": "text/plain"}, body)
			shown, err := coveRunCaptured(t, args...)
			if err != nil {
				shown += "\n" + err.Error()
			}
			if coveCLILeaks(shown, fresh) {
				t.Fatalf("%v with %q shows the visa:\n%s", args, body, shown)
			}
		}
	}
}

// Codex round 2 finding 1, through the raw endpoint command.
func TestCoveServerCommandNeverQuotesTheContentType(t *testing.T) {
	const sent = "sent-visa-ct-0123456789"
	coveAnswering(t, 200, map[string]string{"Content-Type": "application/octet-stream; name=" + sent}, "ok")
	shown, err := coveRunCaptured(t, "server", "--visa", sent, "--json")
	if err != nil {
		shown += "\n" + err.Error()
	}
	if coveCLILeaks(shown, sent) {
		t.Fatalf("the sent visa reached the output:\n%s", shown)
	}
}

// Codex round 2 finding 2, through the raw endpoint command.
func TestCoveServerCommandContentEncodingShowsNoVisa(t *testing.T) {
	const fresh = "fresh-visa-ce-0123456789"
	coveAnswering(t, 200, map[string]string{"Content-Encoding": fresh, "Content-Type": "application/json"}, `{"visa":"`+fresh+`","result":{"result":{}}}`)
	shown, err := coveRunCaptured(t, "server", "--visa", "sent-visa-0123456789", "--json")
	if err != nil {
		shown += "\n" + err.Error()
	}
	if coveCLILeaks(shown, fresh) {
		t.Fatalf("the visa reached the output:\n%s", shown)
	}
}
