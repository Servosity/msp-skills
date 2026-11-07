// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "net/http"

// MemoryOnlyJar returns a jar that serves every cookie jar already loaded from
// disk and keeps every later Set-Cookie (Riverside rotates the session cookie on
// each response) in memory only. RIVERSIDE_FM_NO_CONFIG_WRITE installs it so no
// credential this process handles reaches cookies.json. Any other jar is
// returned unchanged.
func MemoryOnlyJar(jar http.CookieJar) http.CookieJar {
	if cj, ok := jar.(*cookieJar); ok {
		return cj.inner
	}
	return jar
}
