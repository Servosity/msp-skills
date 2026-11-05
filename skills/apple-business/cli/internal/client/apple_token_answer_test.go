// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Hand-written (apple-business): both token paths (the client_credentials mint and
// the refresh) run where the token-state lock is held, and a refused answer must
// return with its reason instead of deadlocking. The kit holds a credential in that
// answer and withholds it at the sink; the blackbox test proves that on the binary.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"apple-business-pp-cli/internal/config"
)

func TestRefusedTokenAnswersReturnTheirReason(t *testing.T) {
	const echoed = "NewToken-Rq83Vm2Lx7Pd4"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_client","access_token":"`+echoed+`"}`)
	}))
	defer srv.Close()
	cfg := &config.Config{
		BaseURL: srv.URL + "/v1", TokenURL: srv.URL + "/auth/oauth2/token",
		ClientID: "BUSINESSAPI.tokenanswer", ClientSecret: appleTestKeyPEM(t), AppleBusinessKeyId: "kid-1",
		RefreshToken: "Refresh-Ab12Cd34Ef56Gh",
	}
	c := New(cfg, 10*time.Second, 0)
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"mint", func() error { return c.mintClientCredentials(context.Background(), cfg.ClientID, cfg.ClientSecret) }},
		{"refresh", func() error { return c.refreshAccessToken(context.Background()) }},
	} {
		done := make(chan error, 1)
		go func() {
			// As authHeader and the 401 re-mint call it: under the token-state lock.
			c.lockTokenState()
			defer c.unlockTokenState()
			done <- tc.run()
		}()
		var err error
		select {
		case err = <-done:
		case <-time.After(15 * time.Second):
			t.Fatalf("%s: a refused token request deadlocked on the token-state lock", tc.name)
		}
		if err == nil {
			t.Fatalf("%s: a 400 token answer must fail", tc.name)
		}
		if !strings.Contains(err.Error(), "invalid_client") {
			t.Errorf("%s: the error lost the answer's reason: %v", tc.name, err)
		}
	}
}
