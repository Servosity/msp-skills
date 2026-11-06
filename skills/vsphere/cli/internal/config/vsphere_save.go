// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// vsphere handfix (config-save-no-credential-outside-store): persisted() is what
// config.toml keeps: the base URL without userinfo or credential-named query
// values, and no credential-named header. Both files are credential sources the
// kit reads at startup (Declarations.Files) and their writes are its declared
// exemption (cliutil.AtomicWriteCredentialFile).

import (
	"vsphere-pp-cli/internal/cliutil"
)

// persistableHeaders is h without every header whose name is a credential
// name (Authorization, Cookie, a session id, an API key).
func persistableHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if cliutil.CredentialName(k) {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
