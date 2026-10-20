// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/config"
)

type countingRT struct{ n int32 }

func (r *countingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&r.n, 1)
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}, Request: req}, nil
}

// TestCredentialHostRefusalIsNotRetried (handfixes.json:
// credential-host-pinning): the pin's refusal is final, so the client must
// fail on the first attempt instead of retrying it three times with backoff
// as if the network had blipped.
func TestCredentialHostRefusalIsNotRetried(t *testing.T) {
	t.Setenv(cliutil.VerifyEnvVar, "")
	t.Setenv(cliutil.DogfoodEnvVar, "")
	cfg := &config.Config{BaseURL: "https://{region}.device-management.eset.systems", AuthHeaderVal: "Bearer test-token", TemplateVars: map[string]string{"region": "eu"}}
	c := New(cfg, 30*time.Second, 0)
	c.NoCache = true
	rec := &countingRT{}
	var refusals int32
	c.HTTPClient.Transport = &EsetTransport{Base: rec, AllowHost: func(string) bool {
		atomic.AddInt32(&refusals, 1)
		return false
	}}

	start := time.Now()
	_, err := c.Get(context.Background(), "https://attacker.example/v1/devices", nil)
	elapsed := time.Since(start)

	var refused *CredentialHostRefusedError
	if err == nil || !errors.As(err, &refused) || !strings.Contains(err.Error(), `refusing to send ESET credentials to "attacker.example"`) {
		t.Fatalf("want the credential host refusal, got %v", err)
	}
	if n := atomic.LoadInt32(&refusals); n != 1 {
		t.Fatalf("the refused request was attempted %d times, want 1 (a pin refusal is not a network error)", n)
	}
	if elapsed > 900*time.Millisecond {
		t.Fatalf("the refusal took %s: it was retried with backoff", elapsed)
	}
	if n := atomic.LoadInt32(&rec.n); n != 0 {
		t.Fatalf("a refused request reached the network %d times", n)
	}
}

func TestIsNonRetryableTransportError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{&CredentialHostRefusedError{Host: "attacker.example"}, true},
		{&ErrPendingTimeout{ResponseID: "r", Waited: time.Minute}, true},
		{errors.New("connection reset by peer"), false},
		{nil, false},
	} {
		if got := IsNonRetryableTransportError(tc.err); got != tc.want {
			t.Errorf("IsNonRetryableTransportError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
