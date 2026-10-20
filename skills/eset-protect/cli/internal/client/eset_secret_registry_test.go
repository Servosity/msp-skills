// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/credvariants"
	"eset-protect-pp-cli/internal/config"
	"eset-protect-pp-cli/internal/secrets"
)

// echoServer answers every request with the status, Content-Type and body
// the test last set.
type echoServer struct {
	mu     sync.Mutex
	status int
	ctype  string
	body   string
}

func (e *echoServer) set(status int, ctype, body string) {
	e.mu.Lock()
	e.status, e.ctype, e.body = status, ctype, body
	e.mu.Unlock()
}

func (e *echoServer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	e.mu.Lock()
	status, ctype, body := e.status, e.ctype, e.body
	e.mu.Unlock()
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// TestRegisteredSecretsNeverLeaveTheAPIClient is the API half of the
// boundary property test (handfixes.json: secret-registry-masks-every-boundary).
// A random secret, registered either as a field of the client's config (the
// request credential, the password) or only in the process registry (a bearer
// another client minted, the 401 re-mint), is echoed by the API in every
// spelling: in JSON, plain-text and HTML error bodies on a JSON endpoint and
// on a declared binary download, in a body long enough that the secret sits
// across the error-text cut, and in a successful JSON body and HTML page.
// Neither the error text nor the returned data may let the oracle recover the
// secret or a ten-byte run of it.
func TestRegisteredSecretsNeverLeaveTheAPIClient(t *testing.T) {
	t.Setenv("ESET_PROTECT_NO_LEARN", "1")
	echo := &echoServer{status: http.StatusOK}
	srv := httptest.NewServer(echo)
	t.Cleanup(srv.Close)
	r := credvariants.NewRand(17)
	statuses := []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity}
	targets := []struct {
		method, path string
		header       map[string]string
	}{
		{http.MethodGet, "/v1/devices", nil},
		{http.MethodPost, "/v1/quarantined-objects:download", map[string]string{BinaryResponseHeader: "true"}},
	}
	calls := 0
	for trial := 0; trial < 16; trial++ {
		secret := credvariants.RandomSecret(r)
		cfg := &config.Config{BaseURL: srv.URL}
		if trial%2 == 0 {
			cfg.EsetProtectPassword = secret
		} else {
			secrets.Register(secret)
		}
		c := New(cfg, 5*time.Second, 0)
		c.HTTPClient = srv.Client()
		c.NoCache = true
		for fi, f := range credvariants.SecretForms(secret) {
			pad := strings.Repeat("x", maxErrorBodyBytes-len("Rejected credential ")-len(f.Text)/2)
			bodies := []struct{ ctype, body string }{
				{"application/json", `{"code":16,"message":"Rejected credential ` + f.Text + `","details":[]}`},
				{"text/plain", "Rejected credential " + f.Text},
				{"text/html", "<html><head><title>Rejected " + f.Text + "</title></head><body>no</body></html>"},
				{"text/plain", "Rejected credential " + pad + f.Text + " tail"},
			}
			for bi, b := range bodies {
				for ti, tg := range targets {
					status := statuses[(fi+bi+ti)%len(statuses)]
					echo.set(status, b.ctype, b.body)
					calls++
					data, _, err := c.do(context.Background(), tg.method, tg.path, map[string]string{"n": strconv.Itoa(calls)}, nil, tg.header)
					if err == nil {
						t.Fatalf("%s: HTTP %d returned no error", tg.path, status)
					}
					if credvariants.SecretRecoverable([]byte(err.Error()), secret) || credvariants.SecretRecoverable(data, secret) {
						t.Fatalf("trial %d, form %s, %s %s HTTP %d (%s): the secret reached the error text\nsecret %q\n  body %q\n   err %v",
							trial, f.Name, tg.method, tg.path, status, b.ctype, secret, b.body, err)
					}
				}
			}
			for _, ok := range []struct{ ctype, body string }{
				{"application/json", `{"devices":[{"displayName":"` + f.Text + `","uuid":"00000000-0000-0000-0000-000000000001"}]}`},
				{"text/html", "<html><head><title>Sign in " + f.Text + "</title></head></html>"},
			} {
				echo.set(http.StatusOK, ok.ctype, ok.body)
				calls++
				data, _, err := c.do(context.Background(), http.MethodGet, "/v1/devices", map[string]string{"n": strconv.Itoa(calls)}, nil, nil)
				text := string(data)
				if err != nil {
					text += err.Error()
				}
				if credvariants.SecretRecoverable([]byte(text), secret) {
					t.Fatalf("trial %d, form %s: a successful %s body carried the secret out\nsecret %q\n  body %q\n   out %q",
						trial, f.Name, ok.ctype, secret, ok.body, text)
				}
			}
		}
	}
	if calls < 2000 {
		t.Fatalf("too few calls: %d", calls)
	}
}

// TestSetAuthRegistersTheRefreshedBearer (Codex round 6, P1): a bearer the
// 401 re-mint publishes on the transport is a secret from that moment, so an
// error body echoing it is masked although no config holds it.
func TestSetAuthRegistersTheRefreshedBearer(t *testing.T) {
	const t2 = "T2-refreshed-bearer-7Q4M-not-in-any-config"
	tr := &EsetTransport{}
	tr.SetAuth("Bearer " + t2)
	if got := secrets.Mask(`{"message":"Rejected credential ` + t2 + `"}`); strings.Contains(got, "T2-refreshed") {
		t.Fatalf("the re-minted bearer was not registered: %q", got)
	}
}
