// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

// Codex review r2 (class outbound-sink-bypasses-chokepoint, repeated): the
// round-1 fix withheld the request BODY of every non-API sender, but the URL
// (query, path, userinfo) went out as configured. Every non-API request goes
// through credkit.Transport, which refuses a request whose URL, headers or
// body carry a held value. The sink must see no request at all for those
// sends, and a clean one must still arrive.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNonAPISendersNeverCarryAHeldValue(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var seen strings.Builder
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		// Every header, Authorization included.
		seen.WriteString(r.RequestURI + "\n" + fmt.Sprint(r.Header) + "\n")
		if u, p, ok := r.BasicAuth(); ok {
			seen.WriteString(u + ":" + p + "\n")
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvZoneUser)
	base := e.env
	for _, ep := range []string{
		sink.URL + "/collect?token=" + bbCode,
		sink.URL + "/hooks/" + bbCode,
		sink.URL + "/hooks/" + bbHTTPToken,
		sink.URL + "/hooks/x?k=" + bbSecret,
	} {
		e.env = append(append([]string{}, base...), "AUTOTASK_FEEDBACK_ENDPOINT="+ep)
		e.run(t, "feedback", "ok", "--send")
	}
	e.env = base
	q := e.api("tickets", "query", "--filter", allFilter, "--json", "--deliver")
	e.run(t, append(q, "webhook:"+sink.URL+"/deliver?key="+bbCode)...)
	e.run(t, append(q, "webhook:"+sink.URL+"/deliver/"+bbSecret)...)
	mu.Lock()
	got := seen.String()
	mu.Unlock()
	assertNoEcho(t, "requests to non-API hosts", got)
	// A clean endpoint still receives the (withheld) feedback, so the refusal
	// above is the URL check, not a broken sender.
	e.env = append(append([]string{}, base...), "AUTOTASK_FEEDBACK_ENDPOINT="+sink.URL+"/collect")
	e.run(t, "feedback", "clean", "--send")
	mu.Lock()
	clean := seen.String()
	mu.Unlock()
	if !strings.Contains(clean, "/collect\n") {
		t.Fatalf("a clean feedback endpoint received nothing:\n%s", clean)
	}
}
