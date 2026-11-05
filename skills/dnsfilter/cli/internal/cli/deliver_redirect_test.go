// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "redirect-never-replays-a-body" entry.

package cli

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A webhook that redirects (307 keeps the body) never gets the output
// replayed to the host it names; delivery fails instead.
func TestDeliverWebhookNeverFollowsARedirect(t *testing.T) {
	var reached atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
	}))
	t.Cleanup(other.Close)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/collect", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(hook.Close)
	if err := deliverWebhook(hook.URL, []byte(`{"secret_key":"R18HOOK"}`), false); err == nil {
		t.Fatal("a redirected webhook delivery reported success")
	}
	if n := reached.Load(); n != 0 {
		t.Fatalf("the redirect target received %d request(s)", n)
	}
}
