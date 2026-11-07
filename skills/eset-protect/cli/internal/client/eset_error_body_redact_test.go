// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/credvariants"
	"eset-protect-pp-cli/internal/config"
)

const errorBodySentinel = "ERRBODYSECRET-4K9W"

// TestErrorBodiesAreRedactedOnEveryEndpoint (Codex round 5, P1): with
// binaryResponse true (both quarantine downloads), client.do skipped the
// deviceToken redactor for every HTTP error body, so a 400 JSON body naming
// a device token reached CLI error output and the MCP execute result. Every
// error body is text that is shown, so it goes through the same
// redact-or-withhold funnel on every endpoint, declared binary or not. For
// every corpus variant of deviceToken (valid, malformed, truncated, non-JSON),
// under every Content-Type a server or proxy might use, on the declared
// binary download and a plain JSON endpoint, at several error statuses: no
// error text holds a recoverable token.
func TestErrorBodiesAreRedactedOnEveryEndpoint(t *testing.T) {
	corpus := credvariants.Corpus([]string{"deviceToken"}, errorBodySentinel, RedactedValue, 53, false)
	statuses := []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		i, _ := strconv.Atoi(q.Get("i"))
		ci, _ := strconv.Atoi(q.Get("ct"))
		si, _ := strconv.Atoi(q.Get("st"))
		if ct := binaryDeclaredContentTypes[ci]; ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(statuses[si])
		_, _ = w.Write([]byte(corpus[i]))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.HTTPClient = srv.Client()

	targets := []struct {
		path   string
		header map[string]string
	}{
		{"/v1/quarantined-objects:download", map[string]string{BinaryResponseHeader: "true"}},
		{"/v1/quarantined-objects:batchDownload", map[string]string{BinaryResponseHeader: "true"}},
		{"/v1/devices", nil},
	}
	// Each sampled variant once, rotating Content-Type, target and status so
	// every target meets every Content-Type and every status.
	calls := 0
	for i := 0; i < len(corpus); i += 4 {
		n := i / 4
		ci := n % len(binaryDeclaredContentTypes)
		tg := targets[(n/len(binaryDeclaredContentTypes))%len(targets)]
		si := (n / (len(binaryDeclaredContentTypes) * len(targets))) % len(statuses)
		params := map[string]string{"i": strconv.Itoa(i), "ct": strconv.Itoa(ci), "st": strconv.Itoa(si)}
		calls++
		data, _, err := c.do(context.Background(), http.MethodPost, tg.path, params, map[string]any{}, tg.header)
		if err == nil {
			t.Fatalf("%s: an HTTP %d answer returned no error", tg.path, statuses[si])
		}
		if binaryRecoverable(data, errorBodySentinel) {
			t.Fatalf("%s returned the device token as data: %s", tg.path, data)
		}
		if credvariants.Recoverable([]byte(err.Error()), errorBodySentinel) {
			t.Fatalf("%s (declared binary %v, Content-Type %q, HTTP %d): the error text carries the device token:\n in %q\nerr %v",
				tg.path, tg.header != nil, binaryDeclaredContentTypes[ci], statuses[si], corpus[i], err)
		}
	}
	if calls < 2000 {
		t.Fatalf("too few calls: %d", calls)
	}
}

// TestEscapeBombBodiesAreWithheld (Codex round 5, P2): a body whose escapes
// unwind two bytes per decoding pass is withheld whole, on the success path
// and the error path, inside the request deadline, instead of being decoded
// to a fixed point.
func TestEscapeBombBodiesAreWithheld(t *testing.T) {
	bomb := "%" + strings.Repeat("25", 500000)
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(bomb))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.HTTPClient = srv.Client()
	for _, st := range []int{http.StatusOK, http.StatusBadRequest} {
		status = st
		done := make(chan error, 1)
		go func() {
			_, _, err := c.do(context.Background(), http.MethodGet, "/v1/devices", nil, nil, nil)
			done <- err
		}()
		select {
		case err := <-done:
			// Adversarial round 6: the bomb names no credential, so the
			// message says why it was withheld instead of blaming a device
			// token (handfixes.json: withheld-body-names-its-cause).
			if err == nil || !strings.Contains(err.Error(), UndecodableBodyWithheld) || strings.Contains(err.Error(), "deviceToken") {
				t.Fatalf("HTTP %d escape bomb: want the body withheld as undecodable, got %v", st, truncateForTest(err))
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("HTTP %d escape bomb: no answer in 5s, escape decoding is unbounded", st)
		}
	}
}

func truncateForTest(err error) string {
	if err == nil {
		return "<nil>"
	}
	s := err.Error()
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// TestWithheldBodyMessageNamesItsCause: a body that names deviceToken is
// withheld as a credential body; one that names nothing but is still
// unwinding at the pass budget is withheld as undecodable.
func TestWithheldBodyMessageNamesItsCause(t *testing.T) {
	bomb := "%" + strings.Repeat("25", 40)
	if got := WithheldBodyMessage([]byte(bomb)); got != UndecodableBodyWithheld {
		t.Fatalf("escape bomb: %q", got)
	}
	if got := string(RedactCredentialFieldsOrWithhold([]byte(bomb))); got != UndecodableBodyWithheld {
		t.Fatalf("escape bomb error body: %q", got)
	}
	named := `device%54oken=` + errorBodySentinel
	if got := WithheldBodyMessage([]byte(named)); got != CredentialBodyWithheld {
		t.Fatalf("a body naming deviceToken: %q", got)
	}
	nested := named
	for i := 0; i < 20; i++ {
		nested = strings.ReplaceAll(nested, "%", "%25")
	}
	if got := WithheldBodyMessage([]byte(nested)); got != UndecodableBodyWithheld {
		t.Fatalf("a body naming deviceToken only past the budget is undecodable: %q", got)
	}
}
