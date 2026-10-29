// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (mimecast): the HTTP response cache is off, so no vendor
// answer is written to disk outside the local store. Where a credential may
// go is the credential kit's to decide (internal/credwire, handfixes.json
// credkit-wiring).

package cli

import "mimecast-pp-cli/internal/client"

func init() {
	registerClientHook(func(c *client.Client) error {
		c.NoCache = true
		return nil
	})
}
