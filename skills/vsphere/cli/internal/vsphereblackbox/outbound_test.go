// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

// Codex review r2 (class outbound-sink-bypasses-chokepoint, repeated): the URL of
// a non-API request (query, path, userinfo) can carry a held value too. The
// credential kit's guard judges the whole request, so the sink must see no
// request at all for those three sends, and a clean one must still arrive.

import (
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
		seen.WriteString(r.RequestURI + "\n" + r.Header.Get("Authorization") + "\n")
		if u, p, ok := r.BasicAuth(); ok {
			seen.WriteString(u + ":" + p + "\n")
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	base := e.env
	for _, ep := range []string{sink.URL + "/collect?token=" + bbPassword, sink.URL + "/hooks/" + bbPassword} {
		e.env = append(append([]string{}, base...), "VSPHERE_FEEDBACK_ENDPOINT="+ep)
		e.run(t, "feedback", "ok", "--send")
	}
	e.env = base
	e.run(t, "vm", "list", "--json", "--deliver", "webhook:"+sink.URL+"/deliver?key="+bbPassword)
	mu.Lock()
	got := seen.String()
	mu.Unlock()
	assertNoEcho(t, "requests to non-API hosts", got)
	// A clean endpoint still receives the (withheld) feedback, so the refusal
	// above is the URL check, not a broken sender.
	e.env = append(append([]string{}, base...), "VSPHERE_FEEDBACK_ENDPOINT="+sink.URL+"/collect")
	e.run(t, "feedback", "clean", "--send")
	mu.Lock()
	clean := seen.String()
	mu.Unlock()
	if !strings.Contains(clean, "/collect\n") {
		t.Fatalf("a clean feedback endpoint received nothing:\n%s", clean)
	}
}
