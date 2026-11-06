// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"nerdio-pp-cli/internal/cliutil"
)

type nerdioRoundTripFunc func(*http.Request) (*http.Response, error)

func (f nerdioRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// nerdio handfix tenant-secrets-shown-never-stored, the write side: a secure
// variable create or update carries the customer secret as "value". Before
// this it was neither held nor shown-only, so an NMM error or response that
// echoed it reached stderr and every file sink as plain text. The guard
// registers it as a tenant secret before the request is sent, and the server
// still receives the whole body.
func TestSecureVariableWriteValueIsATenantSecret(t *testing.T) {
	const value = "SvWriteValue-4c81e2Z"
	var got []byte
	tr := tenantSecretTripper{next: nerdioRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		got, _ = io.ReadAll(r.Body)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"name":"sv-backup"}`)), Request: r}, nil
	})}
	for _, p := range []string{"/rest-api/v1/accounts/101/secure-variables", "/rest-api/v1/secure-variables"} {
		body := `{"name":"sv-backup","value":"` + value + `"}`
		req, _ := http.NewRequest(http.MethodPost, "https://nmm.contoso.com"+p, bytes.NewReader([]byte(body)))
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("POST %s: %v", p, err)
		}
		_ = resp.Body.Close()
		if string(got) != body {
			t.Fatalf("POST %s: the server received %q, want the whole body", p, got)
		}
	}
	if !cliutil.TenantSecretEchoes(`{"echo":"` + value + `"}`) {
		t.Fatal("the secure variable value a write sent is not a tenant secret: an echo of it would reach stderr and every file")
	}
	if cliutil.TenantSecretEchoes(`{"name":"sv-backup"}`) {
		t.Fatal("the variable's name was registered as a secret")
	}
}

// tenant-secrets-shown-never-stored: secure-variables is no sync resource at
// all. The spec's syncable: false only took it out of the default set, so
// `sync --resources secure-variables` still fetched the values for the store.
func TestSecureVariablesIsNoSyncResource(t *testing.T) {
	for _, n := range knownSyncResourceNames() {
		if n == "secure-variables" {
			t.Fatal("sync accepts --resources secure-variables")
		}
	}
	if _, err := syncResourcePath("secure-variables"); err == nil {
		t.Fatal("sync has a path for secure-variables")
	}
	if _, ok := syncOwnedPartitions["secure-variables"]; ok {
		t.Fatal("secure-variables is a store partition")
	}
}

// nerdio handfix guard-refusal-is-final: a request the host guard refuses
// (here: the placeholder base URL) is reported at once. Before this the retry
// loop backed it off three times (1 s + 2 s + 4 s, "network error ...,
// retrying") before failing, on every command run without NERDIO_BASE_URL.
func TestGuardRefusalIsNotRetried(t *testing.T) {
	fence := fenceNetwork(t)
	m := newFakeInstance(t, "finalClientId-3f8a", "finalSecret-7b2e", "finalMinted-1c9d", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the fake NMM instance was reached: %s %s", r.Method, r.URL.Path)
	})
	credRulesHome(t)
	m.useAs(t)
	t.Setenv("NERDIO_BASE_URL", "")
	t.Setenv("NERDIO_NO_CONFIG_WRITE", "1")
	var out bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"accounts", "--json"})
	start := time.Now()
	_ = cmd.Execute()
	if took := time.Since(start); took > 2500*time.Millisecond || strings.Contains(out.String(), "retrying") {
		t.Errorf("the guard's refusal was retried (%s):\n%s", took.Round(time.Millisecond), out.String())
	}
	if !strings.Contains(out.String(), "NERDIO_BASE_URL") {
		t.Errorf("the refusal does not name NERDIO_BASE_URL:\n%s", out.String())
	}
	if _, escaped := fence.result(); len(escaped) != 0 {
		t.Errorf("non-loopback dials escaped: %v", escaped)
	}
}
