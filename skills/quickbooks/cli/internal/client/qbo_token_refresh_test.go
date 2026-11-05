// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package client

import (
	"fmt"
	"sync"
	"testing"

	"quickbooks-pp-cli/internal/config"
)

// A refresh writes the token fields under lockTokenState; every reader takes
// the same lock. Run with -race: an unlocked read is a data race here.
func TestTokenReadersTakeTheTokenLock(t *testing.T) {
	cfg := &config.Config{AccessToken: "zz-start"}
	c := &Client{Config: cfg}
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			c.lockTokenState()
			cfg.AccessToken = fmt.Sprintf("zz-token-%d", i)
			c.unlockTokenState()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = c.likelyCredentialed(nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = c.cachedAuthHeader()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = c.maskCredentialText("zz text")
		}
	}()
	wg.Wait()
}
