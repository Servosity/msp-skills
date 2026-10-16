// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json license-keys-fingerprinted (request side).

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"watchguard-pp-cli/internal/config"
)

// A software license key sent by allocations allocate never comes back in an
// error echo or the --dry-run preview; a Firebox serial stays readable.
func TestLicenseKeyNeverEchoedOrPreviewed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid allocation","echo":` + strconv.Quote(string(body)) + `}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "tok-0001"}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/rest/platform/allocation/v1/assets", nil,
		map[string]any{"productName": "WES_EPDR", "serialOrLicense": "LICKEYSENTINEL0001"}, nil)
	if err == nil || strings.Contains(err.Error(), "LICKEYSENTINEL") {
		t.Fatalf("error text leaks the license key: %v", err)
	}
	for _, body := range []string{`{"serialOrLicense":"LICKEYSENTINEL0001"}`, `{"productName":"WES_EPDR","serialOrLicense":"LICKEYSENTINEL0001"}`} {
		if got := string(redactBody([]byte(body))); strings.Contains(got, "LICKEYSENTINEL") {
			t.Errorf("dry-run preview shows the license key: %s", got)
		}
	}
	if got := string(redactBody([]byte(`{"productName":"firebox","serialOrLicense":"SERIAL0001"}`))); !strings.Contains(got, "SERIAL0001") {
		t.Errorf("dry-run preview masked a Firebox serial: %s", got)
	}
}
