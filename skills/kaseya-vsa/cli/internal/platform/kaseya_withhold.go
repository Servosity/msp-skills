// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

import "kaseya-vsa-pp-cli/internal/cliutil"

// withholdPrivateFile passes every file writePrivateFile writes (run receipts,
// which record argv, the audit index, client profiles, the default-profile
// config and the rate limit ledger) through the output chokepoint
// (kaseya-vsa handfix: output-chokepoint, review r1 P1-C).
func withholdPrivateFile(data []byte) []byte {
	return cliutil.WithholdBytes(data)
}
