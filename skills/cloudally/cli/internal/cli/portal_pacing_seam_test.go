// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"

	"cloudally-pp-cli/internal/portal"
)

// The portal session paces itself to four calls a second, which the portal
// package's own tests prove (throttle_test.go). In this package's in-process
// tests (and the helper processes they start, which run this init too) it only
// added about a second to every download run, and with the run counts these
// tests need, it pushed the package past the 600-second go test budget. A test
// that sets its own rate keeps it.
func init() {
	paced := openPortal
	openPortal = func(ctx context.Context, cfg portal.Options, token string) (*portal.Session, error) {
		if cfg.RatePerSec == 0 {
			cfg.RatePerSec = -1
		}
		return paced(ctx, cfg, token)
	}
}
