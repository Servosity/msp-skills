// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: tenant-rows-never-persisted).

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quotewerks-pp-cli/internal/config"
)

// TestResponsesNeverCached fails if a GET of a secret-bearing tenant row
// writes anything to the on-disk response cache.
func TestResponsesNeverCached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"type":"SecurityAccounts","id":"1","attributes":{"Password":"SENTINEL-SECRET-7731"}}}`))
	}))
	t.Cleanup(server.Close)

	c := New(&config.Config{BaseURL: server.URL, AuthHeaderVal: "qww_test_key"}, time.Second, 0)
	c.HTTPClient = server.Client()
	c.cacheDir = t.TempDir()
	for _, get := range []func() error{
		func() error {
			_, err := c.Get(context.Background(), "/api/v1/qw/tables/SecurityAccounts/1", nil)
			return err
		},
		func() error {
			_, err := c.GetNoCache(context.Background(), "/api/v1/qw/cloudsystemsettings", nil)
			return err
		},
	} {
		if err := get(); err != nil {
			t.Fatal(err)
		}
	}
	_ = filepath.WalkDir(c.cacheDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			t.Errorf("response cached on disk: %s", path)
		}
		return nil
	})
}
