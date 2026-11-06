// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kbmsblackbox

// Codex review r2 (class outbound-sink-bypasses-chokepoint, repeated): the
// round-1 fix withheld the request BODY of every non-API sender, but the URL
// (query, path, userinfo) went out as configured. Fixed after the stop rule
// (no further Codex round): every non-API request goes through
// cliutil.SendOutbound, which refuses a URL or header that repeats a held
// value. The sink must see no request at all for those three sends, and a
// clean one must still arrive.

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNonAPISendersNeverCarryAHeldValue(t *testing.T) {
	t.Parallel() // its own server, home and env: independent of every other test
	var mu sync.Mutex
	var seen strings.Builder
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		// Every header, Authorization included, and the body.
		b, _ := io.ReadAll(r.Body)
		seen.WriteString(r.RequestURI + "\n" + fmt.Sprint(r.Header) + "\n" + string(b) + "\n")
		if u, p, ok := r.BasicAuth(); ok {
			seen.WriteString(u + ":" + p + "\n")
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	// Each credential mode: the feedback URL repeats a credential the process
	// holds from its start (the stored token, the login password, the cached
	// minted token, the password under an environment token); the webhook URL
	// repeats the token this run sends (with the login, the minted one, held
	// once the mint answers).
	for _, mode := range []credMode{credStored, credToken, credLogin, credLoginCached} {
		mu.Lock()
		seen.Reset()
		mu.Unlock()
		sv := echoServer(t)
		e := newEnv(t, sv, mode)
		sent := strings.TrimPrefix(mode.header(), "Bearer ")
		base := e.env
		for _, ep := range []string{sink.URL + "/collect?token=" + mode.held(), sink.URL + "/hooks/" + mode.held()} {
			e.env = append(append([]string{}, base...), "KASEYA_BMS_FEEDBACK_ENDPOINT="+ep)
			e.run(t, "feedback", "ok", "--send")
		}
		e.env = base
		e.run(t, "servicedesk", "get-tickets", "--json", "--deliver", "webhook:"+sink.URL+"/deliver?key="+sent)
		e.run(t, "servicedesk", "get-tickets", "--json", "--deliver", "webhook:"+sink.URL+"/deliver/"+sent)
		mu.Lock()
		got := seen.String()
		mu.Unlock()
		if strings.Contains(got, "/deliver") || strings.Contains(got, "/hooks") || strings.Contains(got, "/collect") {
			t.Errorf("%s: a non-API URL that repeats a held value was sent to:\n%s", mode, got)
		}
		assertNoEcho(t, mode.String()+": requests to non-API hosts", got)
		// A clean endpoint still receives the (withheld) feedback and the
		// delivered records, so the refusals above are the URL check, not a
		// broken sender.
		e.env = append(append([]string{}, base...), "KASEYA_BMS_FEEDBACK_ENDPOINT="+sink.URL+"/collect")
		e.run(t, "feedback", "clean", "--send")
		e.env = base
		sv.setEcho(echoNone) // records that echo nothing (the kit refuses a delivery that echoes the token)
		e.run(t, "servicedesk", "get-tickets", "--json", "--deliver", "webhook:"+sink.URL+"/deliver-clean")
		sv.setEcho(nil)
		mu.Lock()
		clean := seen.String()
		mu.Unlock()
		if !strings.Contains(clean, "/collect\n") || !strings.Contains(clean, "/deliver-clean\n") {
			t.Fatalf("%s: a clean feedback or webhook endpoint received nothing:\n%s", mode, clean)
		}
		assertNoEcho(t, mode.String()+": requests to clean non-API hosts", clean)
	}
}
