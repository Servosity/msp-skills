// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/config"
)

// The token mint passes the client id beside the secret; the id is an
// identifier and is not held, the secret is.
func TestCallerCredentialsDropTheIdentifiersOnly(t *testing.T) {
	got := callerCredentials([]string{"app-id", "s3cret", " ", "tenant-id"}, []string{"app-id", "tenant-id"})
	if strings.Join(got, ",") != "s3cret" {
		t.Fatalf("callerCredentials = %q, want only the secret", got)
	}
}

// tierStub answers the credential-tier read with a secret and redirects every
// other GET onto it, counting how often the tier path is reached.
func tierStub(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	const tier = "/api/v1/auth/get-azure-tenant-auth-details/11111111-1111-4111-8111-111111111111"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tier {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"customAppRegSecret":"new-customer-secret"}`))
			return
		}
		http.Redirect(w, r, tier, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// A credential-tier read is reachable only inside a run of its own command
// (round-1 next-batch review): a redirect from another read, or any other
// command (export, a recipe), is refused before the secret is fetched.
func TestTheCredentialTierAnswersOnlyItsOwnCommand(t *testing.T) {
	srv, hits := tierStub(t)
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "Bearer synthetic-tier-run"}, 5*time.Second, 0)
	c.NoCache = true
	const tier = "/api/v1/auth/get-azure-tenant-auth-details/11111111-1111-4111-8111-111111111111"
	if _, err := c.Get(context.Background(), "/api/v1/computers", nil); err == nil || hits.Load() != 0 {
		t.Fatalf("a redirect from a plain read reached the credential tier (err %v, %d hits)", err, hits.Load())
	}
	if _, err := c.Get(context.Background(), tier, nil); err == nil || hits.Load() != 0 {
		t.Fatalf("a read outside the tier command reached the credential tier (err %v, %d hits)", err, hits.Load())
	}
	if _, err := c.Get(cliutil.WithCredentialTierRun(context.Background()), tier, nil); err != nil || hits.Load() != 1 {
		t.Fatalf("the tier command's own run was refused (err %v, %d hits)", err, hits.Load())
	}
}

// A GET declared state-changing (cliutil.MutatingGETPaths) is sent every
// time: a cached answer would report a cancellation that never ran (round-1
// next-batch review). A plain read is still answered from the cache.
func TestAMutatingGETIsNeverAnsweredFromTheCache(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"ok"`))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "Bearer synthetic-mutating-get"}, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	for _, tc := range []struct {
		path string
		want int32
	}{
		{"/api/v1/maintenance-sessions/cancel-for-schedule/4421", 2},
		{"/api/v1/schedules/running-schedule-ids", 1},
	} {
		before := hits.Load()
		for i := 0; i < 2; i++ {
			if _, err := c.Get(context.Background(), tc.path, nil); err != nil {
				t.Fatalf("%s: %v", tc.path, err)
			}
		}
		if got := hits.Load() - before; got != tc.want {
			t.Errorf("%s reached the vendor %d times in two calls, want %d", tc.path, got, tc.want)
		}
	}
}
