// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package prtgblackbox

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Class credential-to-other-host (prtg handfix: secondary-sinks-withheld):
// a request to any host other than the pinned PRTG core (feedback --send,
// --deliver webhook) never carries a held credential in its URL, headers or
// body. The receiving server records every byte it was sent; the oracle is
// that record.
type sinkLog struct {
	sync.Mutex
	got []string
}

func (l *sinkLog) all() string {
	l.Lock()
	defer l.Unlock()
	return strings.Join(l.got, "\n")
}

func sinkServer(t *testing.T) (string, *sinkLog) {
	t.Helper()
	log := &sinkLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var hdr strings.Builder
		_ = r.Header.Write(&hdr)
		log.Lock()
		log.got = append(log.got, r.Method+" "+r.URL.String()+"\n"+hdr.String()+"\n"+string(body))
		log.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, log
}

func TestNoCredentialReachesAnotherHost(t *testing.T) {
	sink, log := sinkServer(t)
	host := honestServer(t)
	e := newEnv(t, host, true)
	e.env = append(e.env, "PRTG_FEEDBACK_ENDPOINT="+sink+"/feedback")
	// With nothing to withhold, every other-host send gets there.
	e.run(t, "feedback", "a plain note", "--send")
	e.run(t, "sensors", "list-experimental", "--json", "--deliver", "webhook:"+sink+"/hook")
	if got := log.all(); !strings.Contains(got, "/feedback") || !strings.Contains(got, "/hook") || !strings.Contains(got, "Ping-1") {
		t.Fatalf("the sinks were not reached, so nothing was tested:\n%.2000s", got)
	}
	setHonest(host, false)
	e.env = append(e.env, "PRTG_USER_AGENT=agent "+bbKey)
	e.run(t, "feedback", "Authentication failed for "+bbKey, "--send")
	e.run(t, "feedback", "--send", "the key", bbKey[:len(bbKey)/2], bbKey[len(bbKey)/2:])
	e.runIn(t, "pasted "+bbKey+"\n", "feedback", "--stdin", "--send")
	e.run(t, "sensors", "list-experimental", "--json", "--deliver", "webhook:"+sink+"/hook")
	e.run(t, "system", "license-info", "--json", "--deliver", "webhook:"+sink+"/hook")
	// A URL that itself repeats a credential is refused outright: there is
	// no way to send it without the credential.
	before := len(log.got)
	out := e.run(t, "sensors", "list-experimental", "--json", "--deliver", "webhook:"+sink+"/hook?k="+bbKey)
	assertNoEcho(t, "output of a refused webhook", out)
	feedEnv := append([]string(nil), e.env...)
	for i, kv := range feedEnv {
		if strings.HasPrefix(kv, "PRTG_FEEDBACK_ENDPOINT=") {
			feedEnv[i] = "PRTG_FEEDBACK_ENDPOINT=" + sink + "/feedback/" + bbKey
		}
	}
	saved := e.env
	e.env = feedEnv
	assertNoEcho(t, "output of a refused feedback POST", e.run(t, "feedback", "a note", "--send"))
	e.env = saved
	log.Lock()
	after := len(log.got)
	log.Unlock()
	if after != before {
		t.Errorf("a request whose URL repeats a credential was sent (%d new requests)", after-before)
	}
	assertNoEcho(t, "what another host received", log.all())
	if strings.Contains(log.all(), bbLicense) || strings.Contains(log.all(), bbTenantPass) {
		t.Error("another host received a credential the core returned")
	}
}
