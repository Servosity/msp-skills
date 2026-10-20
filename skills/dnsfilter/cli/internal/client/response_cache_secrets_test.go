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
// cache, while a body without secrets still is. Credential reads (api keys,
// the uninstall PIN, which is a bare JSON string, network secret keys, the
// PSA redirect link) and scalar bodies are never cached either.
func TestResponseCacheNeverHoldsSecrets(t *testing.T) {
	dir := t.TempDir()
	c := &Client{BaseURL: "https://api.example.test", cacheDir: dir}

	secret := json.RawMessage(`{"data":[{"id":"7","attributes":{"name":"HQ","secret_key":"SENTINEL-CACHE-SECRET"}}]}`)
	c.writeCacheWithHeaders("/v1/networks", map[string]string{"page[size]": "100"}, nil, secret)
	token := json.RawMessage(`{"data":{"id":"1","attributes":{"name":"k","token":"SENTINEL-CACHE-TOKEN"}}}`)
	c.writeCacheWithHeaders("/v1/api_keys/1", nil, nil, token)

	// Credential reads are refused by path and scalar bodies by shape, even
	// when no secret-named key is present for the name scrub to see.
	pin := json.RawMessage(`"SENTINEL-CACHE-PIN-4821"`)
	c.writeCacheWithHeaders("/v1/user_agents/uninstall_pin", map[string]string{"organization_id": "10"}, nil, pin)
	c.writeCacheWithHeaders("/v1/user_agents/uninstall_pin?organization_id=11", nil, nil, json.RawMessage(`{"data":"SENTINEL-CACHE-PIN-OBJ"}`))
	c.writeCacheWithHeaders("/v1/api_keys", nil, nil, json.RawMessage(`{"data":[{"id":"1","attributes":{"name":"SENTINEL-CACHE-KEYLIST"}}]}`))
	c.writeCacheWithHeaders("/v1/psa_integrations/redirect_link", nil, nil, json.RawMessage(`{"data":{"url":"https://psa.example/cb?code=SENTINEL-CACHE-LINK"}}`))
	c.writeCacheWithHeaders("/v1/networks/7/secret_key", nil, nil, json.RawMessage(`{"data":{"id":"7","attributes":{"value":"SENTINEL-CACHE-NETKEY"}}}`))
	c.writeCacheWithHeaders("/v1/organizations/count", nil, nil, json.RawMessage(`"SENTINEL-CACHE-SCALAR"`))

	// A PIN file an older build left on disk is removed on the next fetch
	// and never served in the meantime.
	leftover := filepath.Join(c.cacheResourceDir("/v1/user_agents/uninstall_pin"), c.cacheKeyFor("GET", "/v1/user_agents/uninstall_pin", map[string]string{"organization_id": "12"}, nil, nil)+".json")
	_ = os.MkdirAll(filepath.Dir(leftover), 0o700)
	if err := os.WriteFile(leftover, []byte(`"SENTINEL-CACHE-OLD-PIN"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.readCache("/v1/user_agents/uninstall_pin", map[string]string{"organization_id": "12"}); ok {
		t.Fatal("a leftover uninstall PIN was served from the response cache")
	}
	c.writeCacheWithHeaders("/v1/user_agents/uninstall_pin", map[string]string{"organization_id": "12"}, nil, json.RawMessage(`"SENTINEL-CACHE-NEW-PIN"`))

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
