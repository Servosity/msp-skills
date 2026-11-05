// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeperblackbox

// Class outbound-sink-bypasses-chokepoint, through the built binaries: a held key in
// the URL or a header of a request to another host (the feedback endpoint, a --deliver
// webhook) is refused before anything is sent, neither sender follows a redirect, and
// a credential the endpoint variable itself carries is held. A clean send still
// arrives, so a refusal is not a broken sender.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestOutboundSendersRefuseAHeldKeyAndFollowNoRedirect(t *testing.T) {
	t.Parallel()
	var offHost atomic.Int32
	off := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { offHost.Add(1) }))
	t.Cleanup(off.Close)
	var mu sync.Mutex
	var reached []string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reached = append(reached, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, off.URL+"/taken", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	take := func() []string {
		mu.Lock()
		defer mu.Unlock()
		got := reached
		reached = nil
		return got
	}
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, true)
	feedback := func(endpoint string, more ...string) string {
		t.Helper()
		return e.with(append([]string{"KEEPER_FEEDBACK_ENDPOINT=" + endpoint}, more...)...).run(t, "feedback", "a note", "--send")
	}
	deliver := func(url string, more ...string) string {
		t.Helper()
		return e.with(more...).run(t, "companies", "list", "--json", "--deliver", "webhook:"+url)
	}
	for _, url := range []string{
		sink.URL + "/collect?token=" + bbToken,
		sink.URL + "/hooks/" + bbToken,
		strings.Replace(sink.URL, "http://", "http://u:"+bbToken+"@", 1) + "/collect",
	} {
		noPiece(t, "feedback output", feedback(url), bbToken)
		noPiece(t, "deliver output", deliver(url), bbToken)
	}
	// A header: the operator's User-Agent is what the deliver sender sends.
	noPiece(t, "deliver output", deliver(sink.URL+"/header", "KEEPER_USER_AGENT="+bbToken), bbToken)
	if got := take(); len(got) != 0 {
		t.Errorf("a request carrying a held key reached the sink: %q", got)
	}
	feedback(sink.URL + "/moved")
	deliver(sink.URL + "/moved")
	if got := take(); len(got) != 2 {
		t.Errorf("want both senders to reach the redirecting sink once, got %q", got)
	}
	if offHost.Load() != 0 {
		t.Errorf("a sender followed a redirect off the host it was given (%d)", offHost.Load())
	}
	feedback(sink.URL + "/collect")
	deliver(sink.URL + "/deliver")
	if got := strings.Join(take(), " "); got != "/collect /deliver" {
		t.Errorf("a clean send did not arrive: %q", got)
	}
	// A credential only the endpoint variable carries: a refused connection names the URL.
	closed := httptest.NewServer(http.NotFoundHandler())
	dead := closed.URL
	closed.Close()
	const urlOnly = "Xw4Tz8Lq2Vn6Rk9Pm3Hs"
	noPiece(t, "a failed feedback send", feedback(dead+"/collect?token="+urlOnly), urlOnly)
}
