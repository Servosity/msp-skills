// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json license-keys-fingerprinted (request side).

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

// allocations deallocate sends the key as a query parameter: it is withheld
// from an error echo, a transport error and the --dry-run preview; a Firebox
// serial in the same position stays readable.
func TestLicenseKeyInQueryNeverEchoedOrPreviewed(t *testing.T) {
	q := func(product, key string) map[string]string {
		return map[string]string{"productName": product, "allocationType": "linked_to_license", "serialOrLicense": key}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"echo ` + r.URL.RawQuery + `"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "tok-0001"}, 5*time.Second, 0)
	c.NoCache = true
	if _, _, err := c.DeleteWithParams(context.Background(), "/rest/platform/allocation/v2/ACC-1/assets", q("WES_EDR", "LICKEYSENTINEL0002")); err == nil || strings.Contains(err.Error(), "LICKEYSENTINEL") {
		t.Fatalf("error text leaks the license key: %v", err)
	}
	// The MCP server builds declared query params into the path itself.
	if _, _, err := c.DeleteWithParams(context.Background(), "/rest/platform/allocation/v2/ACC-1/assets?productName=WES_EDR&serialOrLicense=LICKEYSENTINEL0005", nil); err == nil || strings.Contains(err.Error(), "LICKEYSENTINEL") {
		t.Fatalf("error text leaks a license key carried in the path: %v", err)
	}
	if _, _, err := c.DeleteWithParams(context.Background(), "/rest/platform/allocation/v2/ACC-1/assets?productName=firebox&serialOrLicense=SERIAL0005", nil); err == nil || !strings.Contains(err.Error(), "SERIAL0005") {
		t.Fatalf("a Firebox serial in the path must stay readable: %v", err)
	}
	// The transport error (and its retry notice on stderr) names the CLI's
	// own URL, query included.
	r, w, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	orig := os.Stderr
	os.Stderr = w
	dead := New(&config.Config{BaseURL: "http://127.0.0.1:1", AccessToken: "tok-0001"}, 30*time.Second, 0)
	dead.NoCache = true
	_, _, err := dead.DeleteWithParams(context.Background(), "/rest/platform/allocation/v2/ACC-1/assets", q("WES_EDR", "LICKEYSENTINEL0003"))
	os.Stderr = orig
	_ = w.Close()
	notices, _ := io.ReadAll(r)
	if err == nil || strings.Contains(err.Error()+string(notices), "LICKEYSENTINEL") {
		t.Fatalf("transport error leaks the license key: %v\n%s", err, notices)
	}
	preview := func(params map[string]string) string {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		orig := os.Stderr
		os.Stderr = w
		d := New(&config.Config{BaseURL: "http://127.0.0.1:1"}, 5*time.Second, 0)
		d.DryRun = true
		_, _, _ = d.DeleteWithParams(context.Background(), "/rest/platform/allocation/v2/ACC-1/assets", params)
		os.Stderr = orig
		_ = w.Close()
		out, _ := io.ReadAll(r)
		return string(out)
	}
	if out := preview(q("WES_EDR", "LICKEYSENTINEL0004")); strings.Contains(out, "LICKEYSENTINEL") || !strings.Contains(out, "serialOrLicense=[redacted]") {
		t.Fatalf("dry-run preview shows the license key:\n%s", out)
	}
	if out := preview(q("firebox", "SERIAL0002")); !strings.Contains(out, "SERIAL0002") {
		t.Fatalf("dry-run preview masked a Firebox serial:\n%s", out)
	}
}
