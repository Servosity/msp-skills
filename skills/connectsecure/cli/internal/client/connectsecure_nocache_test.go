// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/json"
	"testing"
	"time"

	"connectsecure-pp-cli/internal/config"
)

func TestSecretPathsBypassResponseCache(t *testing.T) {
	c := New(&config.Config{BaseURL: "https://pod1.myconnectsecure.com"}, time.Second, 0)
	c.cacheDir = t.TempDir()
	body := json.RawMessage(`{"data":[{"params":{"password":"SENTINEL"}}]}`)
	for _, p := range []string{"/r/company/credentials", "/r/company/credentials/7", "/r/integration/integration_credentials", "/r/company/get_uninstall_secret"} {
		c.writeCacheWithHeaders(p, nil, nil, body)
		if _, ok := c.readCacheWithHeaders(p, nil, nil); ok {
			t.Errorf("%s: secret body served from the response cache", p)
		}
	}
	c.writeCacheWithHeaders("/r/company/agent_credentials_mapping", nil, nil, body)
	if _, ok := c.readCacheWithHeaders("/r/company/agent_credentials_mapping", nil, nil); !ok {
		t.Error("non-secret paths must still cache")
	}
}
