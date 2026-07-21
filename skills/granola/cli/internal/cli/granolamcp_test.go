// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// parseMCPResult is the security-relevant validation path: non-2xx must be
// rejected before the body is trusted, an unparseable 2xx must fail, and a
// JSON-RPC error must surface by numeric code only (never the raw body).
func TestParseMCPResult(t *testing.T) {
	const ct = "application/json"
	cases := []struct {
		name    string
		st      int
		ctype   string
		body    string
		wantErr bool
		wantKey string // a top-level key expected on success
	}{
		{"ok json result", 200, ct, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`, false, "result"},
		{"sse result", 200, "text/event-stream", "event: message\ndata: {\"result\":{\"ok\":true}}\n\n", false, "result"},
		{"non-2xx rejected before parse", 302, ct, `{"result":{"looks":"fine"}}`, true, ""},
		{"500 error", 500, ct, `{"error":{"code":-32000}}`, true, ""},
		{"unparseable 2xx", 200, ct, `not json`, true, ""},
		{"empty 2xx", 200, ct, ``, true, ""},
		{"jsonrpc error numeric code", 200, ct, `{"error":{"code":-32602,"message":"secret leak here"}}`, true, ""},
		{"jsonrpc error non-numeric code", 200, ct, `{"error":{"code":"weird","message":"x"}}`, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			obj, err := parseMCPResult("test", c.st, c.ctype, []byte(c.body))
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got obj=%v", obj)
				}
				// The raw body/message must never appear in the error text.
				for _, marker := range []string{"secret leak here", "looks", "fine", "weird"} {
					if strings.Contains(c.body, marker) && strings.Contains(err.Error(), marker) {
						t.Fatalf("error leaked response body (%q): %q", marker, err.Error())
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, ok := obj[c.wantKey]; !ok {
				t.Fatalf("missing key %q in %v", c.wantKey, obj)
			}
		})
	}
}

func TestFirstGranolaUUID(t *testing.T) {
	got := firstGranolaUUID("noise 0b9a63e7-468f-4950-a9f6-dabea60de3e7 more")
	if got != "0b9a63e7-468f-4950-a9f6-dabea60de3e7" {
		t.Fatalf("got %q", got)
	}
	if firstGranolaUUID("no uuid not_Q8jUBbI7HtFPx6 here") != "" {
		t.Fatal("REST not_* id must not match the uuid regex")
	}
}

func TestSanitizeCode(t *testing.T) {
	if got := sanitizeCode("access_denied"); got != "access_denied" {
		t.Fatalf("got %q", got)
	}
	// The security property: no control bytes or spaces survive (a hostile
	// redirect can't inject terminal escapes into our output).
	out := sanitizeCode("evil\x1b[2J\x07 desc")
	for _, r := range out {
		if r < 0x20 || r == ' ' {
			t.Fatalf("control/space char survived: %q", out)
		}
	}
	if got := sanitizeCode("!!!"); got != "unknown" {
		t.Fatalf("empty-after-strip should be 'unknown', got %q", got)
	}
}
