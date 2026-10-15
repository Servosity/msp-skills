// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: secret-bearing ConnectSecure reads bypass the on-disk response cache.

package client

import "connectsecure-pp-cli/internal/cliutil"

// connectsecureNoCachePath gates the recorded readCacheWithHeaders /
// writeCacheWithHeaders guards; the list lives in cliutil.
func connectsecureNoCachePath(path string) bool {
	return cliutil.ConnectsecureSecretPath(path)
}
