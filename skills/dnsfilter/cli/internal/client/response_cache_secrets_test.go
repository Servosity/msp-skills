// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResponseCacheNeverHoldsSecrets guards handfixes.json:
// response-cache-never-holds-secrets. A live networks or api-keys GET body
// carries secret_key / token; it must not be written to the on-disk response
// cache, while a body without secrets still is.
func TestResponseCacheNeverHoldsSecrets(t *testing.T) {
	dir := t.TempDir()
	c := &Client{BaseURL: "https://api.example.test", cacheDir: dir}

	secret := json.RawMessage(`{"data":[{"id":"7","attributes":{"name":"HQ","secret_key":"SENTINEL-CACHE-SECRET"}}]}`)
	c.writeCacheWithHeaders("/v1/networks", map[string]string{"page[size]": "100"}, nil, secret)
	token := json.RawMessage(`{"data":{"id":"1","attributes":{"name":"k","token":"SENTINEL-CACHE-TOKEN"}}}`)
	c.writeCacheWithHeaders("/v1/api_keys/1", nil, nil, token)

	plain := json.RawMessage(`{"data":[{"id":"1","attributes":{"name":"Acme"}}]}`)
	c.writeCacheWithHeaders("/v1/organizations", nil, nil, plain)

	var found, files int
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		files++
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), "SENTINEL-CACHE") {
			found++
			t.Errorf("secret reached the response cache: %s", path)
		}
		return nil
	})
	if found == 0 && files == 0 {
		t.Fatal("a body without secrets was not cached either; the guard is too broad")
	}
	if _, ok := c.readCache("/v1/organizations", nil); !ok {
		t.Fatal("plain body was not cached")
	}
}
