// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "kaseya-vsa-pp-cli/internal/cliutil"

// holdConfigCredentials registers every credential the configuration carries
// (environment or credential store) with the one holder the output
// chokepoint reads (kaseya-vsa handfix: output-chokepoint).
func holdConfigCredentials(c *Config) {
	cliutil.FunnelHold(c.KaseyaVsaTokenSecret, c.AuthHeaderVal, c.AccessToken, c.RefreshToken, c.ClientSecret, c.AuthHeader())
}
