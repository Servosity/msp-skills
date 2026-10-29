// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import "jamf-pro-pp-cli/internal/cliutil"

// No test here runs credkit.Start and the kit's held set has no reset, so a
// fixture held by one test would refuse every later statement. The built
// binaries prove the hold (internal/jamfblackbox).
func init() { cliutil.Hold = func(string, ...string) {} }
