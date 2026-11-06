// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package abnormalblackbox

// Codex review r2 (class outbound-sink-bypasses-chokepoint): a non-API
// sender's URL (query, path, userinfo) must not carry a held value either.
// Every non-API client sends through credkit.Transport, which checks the URL,
// the headers and the body. The sink must see no request at all for those
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
	e := newEnv(t, sv, credEnv)
	base := e.env
	for _, ep := range []string{
		sink.URL + "/collect?token=" + bbToken,
		sink.URL + "/hooks/" + bbToken,
		sink.URL + "/hooks/" + bbHTTPToken,
	} {
		e.env = append(append([]string{}, base...), "ABNORMAL_FEEDBACK_ENDPOINT="+ep)
		e.run(t, "feedback", "ok", "--send")
	}
	e.env = base
	e.run(t, "threats", "retrieve", "--json", "--deliver", "webhook:"+sink.URL+"/deliver?key="+bbToken)
	mu.Lock()
	got := seen.String()
	mu.Unlock()
	assertNoEcho(t, "requests to non-API hosts", got)
	// A clean endpoint still receives the (withheld) feedback, so the refusal
	// above is the URL check, not a broken sender.
	e.env = append(append([]string{}, base...), "ABNORMAL_FEEDBACK_ENDPOINT="+sink.URL+"/collect")
	e.run(t, "feedback", "clean", "--send")
	mu.Lock()
	clean := seen.String()
	mu.Unlock()
	if !strings.Contains(clean, "/collect\n") {
		t.Fatalf("a clean feedback endpoint received nothing:\n%s", clean)
	}
}
