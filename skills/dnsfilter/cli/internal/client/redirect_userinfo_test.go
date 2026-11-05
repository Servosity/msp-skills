// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written for the reprint review (handfixes.json: redirect-never-carries-userinfo).

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/config"
)

// A same-origin redirect whose Location carries userinfo is refused before Go
// turns that userinfo into a Basic Authorization header. Codex review round 1:
// `Location: https://u:<key>@...` handed the key on, base64-encoded, after
// every header strip had run. A cross-origin hop with userinfo is refused as
// cross-origin (TestRedirectRefusesEveryCrossOriginHop).
func TestRedirectNeverCarriesUserinfo(t *testing.T) {
	testenv.Isolate(t, cliutil.CacheDir)
	const key = "test-key-000000000000"
	var landed atomic.Int32
	var target string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/same-origin" {
			w.Header().Set("Location", target)
			w.WriteHeader(http.StatusFound)
			return
		}
		landed.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(origin.Close)
	target = strings.Replace(origin.URL, "http://", "http://u:"+key+"@", 1) + "/v1/landed"
	c := New(&config.Config{BaseURL: origin.URL, DnsfilterApiKey: key}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), "/v1/same-origin", nil); !errors.Is(err, ErrRedirectUserinfo) {
		t.Fatalf("a same-origin redirect carrying userinfo was followed: %v", err)
	}
	if n := landed.Load(); n != 0 {
		t.Fatalf("the userinfo target received %d request(s)", n)
	}
}
