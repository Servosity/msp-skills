// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestCredentialTierResponseCacheNeitherWritesServesNorKeeps: a
// credential-tier body (internal/cliutil/immybot_credential_tier.go) is never
// written to the response cache, and a copy an earlier binary left at its key
// is neither served nor kept. Otherwise the copy would be served before the
// transport refusal that guards an MCP-spawned process. The control path
// proves the seeded file sits at the real cache key.
func TestCredentialTierResponseCacheNeitherWritesServesNorKeeps(t *testing.T) {
	c := &Client{BaseURL: "https://example.immy.bot", cacheDir: t.TempDir()}
	const tier = "/api/v1/auth/get-azure-tenant-auth-details/11111111-1111-1111-1111-111111111111"
	secret := json.RawMessage(`{"customAppRegSecret":"credTierCacheSentinel"}`)

	c.writeCache(tier, nil, secret)
	_ = filepath.WalkDir(c.cacheDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if data, _ := os.ReadFile(path); bytes.Contains(data, []byte("credTierCacheSentinel")) {
				t.Errorf("credential-tier body written to the cache: %s", path)
			}
		}
		return nil
	})

	legacy := filepath.Join(c.cacheResourceDir(tier), c.cacheKeyFor(http.MethodGet, tier, nil, nil, nil)+".json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{legacy, legacy + ".platform.json"} {
		if err := os.WriteFile(f, secret, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if data, ok := c.readCache(tier, nil); ok {
		t.Errorf("an earlier binary's cached credential was served: %s", data)
	}
	for _, f := range []string{legacy, legacy + ".platform.json"} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("an earlier binary's cached credential was kept: %s (%v)", f, err)
		}
	}

	const ordinary = "/api/v1/auth/get-ip-addresses"
	c.writeCache(ordinary, nil, json.RawMessage(`[{"id":1}]`))
	if _, err := os.Stat(filepath.Join(c.cacheResourceDir(ordinary), c.cacheKeyFor(http.MethodGet, ordinary, nil, nil, nil)+".json")); err != nil {
		t.Fatalf("control: an ordinary read is not cached at the computed key: %v", err)
	}
	if _, ok := c.readCache(ordinary, nil); !ok {
		t.Fatal("control: an ordinary cached read was not served")
	}
}
