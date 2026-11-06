// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring).

package fgtcloud

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fortinet-pp-cli/internal/credkit"
)

// The token cache is a credential source (handfixes.json credkit-wiring): its
// write goes outside the kit, which would withhold a held token from any file
// by contract and lose the refresh token the grant just returned.
func TestTokenCacheIsWrittenOutsideTheKit(t *testing.T) {
	const access, refresh = "fcacc-" + "Wq35NvX8kP2h", "fcref-" + "Gd71HsQ4mZ9c"
	credkit.Hold("minted:fgtcloud-access", access)
	credkit.Hold("minted:fgtcloud-refresh", refresh)
	path := filepath.Join(t.TempDir(), "fortigate-cloud-token.json")
	if err := writeCache(path, &cachedToken{User: "u", AccessToken: access, RefreshToken: refresh, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), access) || !strings.Contains(string(b), refresh) {
		t.Fatalf("the cache lost its tokens: %s", b)
	}
}
