// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/cliutil/testenv"
	"immybot-pp-cli/internal/config"
)

// A vendor redirect never reaches a declared state-changing GET, on any client
// the program builds and whatever started the chain: what a redirect runs was
// never confirmed. A read is still followed, so this test proves something.
func TestARedirectNeverReachesAMutatingGET(t *testing.T) {
	const origin = "https://93.184.216.34"
	c := New(&config.Config{BaseURL: origin, AuthHeaderVal: "Bearer synthetic-redirect-class"}, 5*time.Second, 0)
	guarded := &http.Client{}
	guardHTTPClient(guarded, origin)
	first, _ := http.NewRequest(http.MethodGet, origin+"/api/v1/computers", nil)
	for name, check := range map[string]func(*http.Request, []*http.Request) error{"New": c.HTTPClient.CheckRedirect, "guardHTTPClient": guarded.CheckRedirect} {
		for _, path := range []string{
			"/api/v1/maintenance-sessions/cancel-for-schedule/4421",
			"/API/V1/Maintenance-Sessions/Cancel-For-Schedule/4421",
			"/api/v1/maintenance-sessions/4421/cancel/",
			"/api/v1/computers/../maintenance-sessions/cancel-for-schedule/4421",
			"/api//v1/maintenance-sessions/cancel-for-schedule/4421?x=1",
			"/plugins/api/v1/9/any/thing",
			"/auth/logout",
		} {
			hop, err := http.NewRequest(http.MethodGet, origin+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := check(hop, []*http.Request{first}); err == nil {
				t.Errorf("%s: a redirect reached %s", name, path)
			}
		}
		read, _ := http.NewRequest(http.MethodGet, origin+"/api/v1/computers/7", nil)
		if err := check(read, []*http.Request{first}); err != nil {
			t.Errorf("%s: a redirect to a plain read was refused: %v", name, err)
		}
	}
}

// An answer a prior binary cached (it may carry a credential-tier secret that
// reached it through a redirect, unheld) is never served, and does not stay on
// disk: the upgrade from the review's input reads the wire instead.
func TestACachedAnswerFromAPriorBinaryIsNeverServed(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":7}]`))
	}))
	t.Cleanup(srv.Close)
	cfg := &config.Config{BaseURL: srv.URL, AuthHeaderVal: "Bearer synthetic-cache-class"}
	const path = "/api/v1/computers"
	root, err := cliutil.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	prior := filepath.Join(root, "http", "resources", cacheResourceTag(path), New(cfg, 5*time.Second, 0).cacheKey(path, nil)+".json")
	if err := os.MkdirAll(filepath.Dir(prior), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prior, []byte(`{"customAppRegSecret":"new-customer-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := New(cfg, 5*time.Second, 0).Get(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "new-customer-secret") {
		t.Fatalf("a prior binary's cached answer was served: %s", got)
	}
	if !strings.Contains(string(got), `"id":7`) {
		t.Fatalf("the wire's answer did not come back: %s", got)
	}
	if _, err := os.Stat(prior); !os.IsNotExist(err) {
		t.Errorf("the prior binary's cached answer is still on disk: %v", err)
	}
}
