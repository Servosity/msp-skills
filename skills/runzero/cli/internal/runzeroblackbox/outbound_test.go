// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

// Codex review r2 (class outbound-sink-bypasses-chokepoint, repeated): the
// round-1 fix withheld the request BODY of every non-API sender, but the URL
// (query, path, userinfo) went out as configured. Fixed after the stop rule
// (no further Codex round): every non-API request goes through
// cliutil.SendOutbound, which refuses a URL or header that repeats a held
// value. The sink must see no request at all for those sends, and a clean
// one must still arrive.

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestNonAPISendersNeverCarryAHeldValue(t *testing.T) {
	t.Parallel() // its own fake runZero and home
	var mu sync.Mutex
	var seen strings.Builder
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		// Every header, Authorization included.
		seen.WriteString(r.RequestURI + "\n" + fmt.Sprint(r.Header) + "\n")
		if u, p, ok := r.BasicAuth(); ok {
			seen.WriteString(u + ":" + p + "\n")
		}
		if u := r.URL.User; u != nil {
			seen.WriteString(u.String() + "\n")
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvOverStored)
	base := e.env
	for _, ep := range []string{
		sink.URL + "/collect?token=" + bbKey,
		sink.URL + "/hooks/" + bbKey,
		sink.URL + "/hooks/" + bbHTTPToken,
		sink.URL + "/hooks/x?k=" + bbStored,
		sink.URL + "/hooks/Bearer%20" + bbKey,
		sink.URL + "/hooks/" + url.PathEscape(base64.StdEncoding.EncodeToString([]byte(bbKey))),
		sink.URL + "/collect?event=" + url.QueryEscape(bearer(bbKey)),
		sink.URL + "/collect?stored=" + url.QueryEscape(bearer(bbStored)),
		"http://" + url.UserPassword("rz", bbKey).String() + "@" + strings.TrimPrefix(sink.URL, "http://") + "/collect",
	} {
		e.env = append(append([]string{}, base...), "RUNZERO_FEEDBACK_ENDPOINT="+ep)
		e.run(t, "feedback", "ok", "--send")
	}
	e.env = base
	e.run(t, "org", "get-assets", "--json", "--deliver", "webhook:"+sink.URL+"/deliver?key="+bbKey)
	e.run(t, "org", "get-assets", "--json", "--deliver", "webhook:"+sink.URL+"/deliver/"+url.PathEscape(bearer(bbKey)))
	e.run(t, "account", "get-users", "--json", "--deliver", "webhook:"+sink.URL+"/deliver/"+bbStored)
	e.run(t, "org", "get-asset", bbAssetID, "--json", "--deliver", "webhook:"+sink.URL+"/deliver/"+bbHTTPToken)
	e.run(t, "org", "get-asset", bbAssetID, "--json", "--deliver", "webhook:http://"+url.UserPassword("rz", bbKey).String()+"@"+strings.TrimPrefix(sink.URL, "http://")+"/deliver")
	// A response credential the process now holds (an API key's token) in a
	// later delivery URL.
	e.run(t, "account", "get-keys", "--json", "--deliver", "webhook:"+sink.URL+"/deliver/"+bbKeyToken)
	mu.Lock()
	got := seen.String()
	mu.Unlock()
	assertNoEcho(t, "requests to non-API hosts", got)
	// A clean endpoint still receives the (withheld) feedback, so the refusal
	// above is the URL check, not a broken sender.
	e.env = append(append([]string{}, base...), "RUNZERO_FEEDBACK_ENDPOINT="+sink.URL+"/collect")
	e.run(t, "feedback", "clean", "--send")
	mu.Lock()
	clean := seen.String()
	mu.Unlock()
	if !strings.Contains(clean, "/collect\n") {
		t.Fatalf("a clean feedback endpoint received nothing:\n%s", clean)
	}
}
