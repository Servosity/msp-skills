// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestStreamingDownloadHeaderStallTimesOut (handfixes.json:
// streaming-download-header-timeout): a binary download runs on
// client.StreamingHTTPClient, which drops the whole-call timeout and re-arms
// ResponseHeaderTimeout only on a bare *http.Transport. Behind EsetTransport
// a quarantine download whose server never sends headers hung forever; the
// command's --timeout must still bound the header wait.
func TestStreamingDownloadHeaderStallTimesOut(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { // never answer: no status line, no headers
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	t.Setenv("ESET_PROTECT_BASE_URL", srv.URL)
	t.Setenv("ESET_PROTECT_TOKEN", "test-token")
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_USERNAME", "")
	t.Setenv("ESET_PROTECT_PASSWORD", "")

	flags := &rootFlags{timeout: 300 * time.Millisecond}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.HTTPClient.Transport.(*client.EsetTransport); !ok {
		t.Fatalf("the ESET transport is not installed (got %T), so this test proves nothing", c.HTTPClient.Transport)
	}
	c.NoCache = true
	done := make(chan error, 1)
	go func() {
		_, _, err := c.PostWithParamsAndHeaders(context.Background(), "https://{region}.quarantine-management.eset.systems/v1/quarantined-objects:download", nil, map[string]any{}, map[string]string{client.BinaryResponseHeader: "true"})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a download whose server never sent headers returned no error")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("a quarantine download whose server never sends headers hung past 20s; --timeout 300ms must bound the header wait")
	}
}
