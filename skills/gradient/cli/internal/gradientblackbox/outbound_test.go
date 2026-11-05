// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package gradientblackbox

// Codex review r2 (class outbound-sink-bypasses-chokepoint, repeated): the
// round-1 fix withheld the request BODY of every non-API sender, but the URL
// (query, path, userinfo) went out as configured. Every non-API request now
// goes through the shared credential kit's transport (credkit.Transport,
// credkit-wiring), which refuses a URL, header or body that carries a held
// value to an origin it is not declared for. The sink must see no request at
// all for those three sends, and a clean one must still arrive.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNonAPISendersNeverCarryAHeldValue(t *testing.T) {
	var mu sync.Mutex
	var seen strings.Builder
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		// Every header, GRADIENT-TOKEN and Authorization included.
		seen.WriteString(r.RequestURI + "\n" + fmt.Sprint(r.Header) + "\n")
		if u, p, ok := r.BasicAuth(); ok {
			seen.WriteString(u + ":" + p + "\n")
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvToken)
	base := e.env
	for _, ep := range []string{sink.URL + "/collect?token=" + bbToken, sink.URL + "/hooks/" + bbToken} {
		e.env = append(append([]string{}, base...), "GRADIENT_FEEDBACK_ENDPOINT="+ep)
		e.run(t, "feedback", "ok", "--send")
	}
	e.env = base
	e.run(t, "accounts", "list", "--json", "--deliver", "webhook:"+sink.URL+"/deliver?key="+bbToken)
	mu.Lock()
	got := seen.String()
	mu.Unlock()
	assertNoEcho(t, "requests to non-API hosts", got)
	// A clean endpoint still receives the (withheld) feedback, so the refusal
	// above is the URL check, not a broken sender.
	e.env = append(append([]string{}, base...), "GRADIENT_FEEDBACK_ENDPOINT="+sink.URL+"/collect")
	e.run(t, "feedback", "clean", "--send")
	mu.Lock()
	clean := seen.String()
	mu.Unlock()
	if !strings.Contains(clean, "/collect\n") {
		t.Fatalf("a clean feedback endpoint received nothing:\n%s", clean)
	}
}
