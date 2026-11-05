// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// maxio handfix legacy-basic-env (hand file; a reprint keeps it): the published
// connector (0.1.x, press 4.24) told operators to set MAXIO_USERNAME (the API
// key) and MAXIO_PASSWORD (the literal x). Press 4.34.2 renders the documented
// single-secret form, MAXIO_API_KEY with the constant password x
// (spec.json x-auth-env-vars), so the pair keeps working here:
//   - MAXIO_USERNAME is the API key when MAXIO_API_KEY is unset (MAXIO_API_KEY wins).
//   - MAXIO_PASSWORD may only be x when the pair is what authenticates: Maxio
//     API keys always use it. Any other value is refused with a clear error
//     rather than silently sent or dropped, and the error never repeats it.
//     With MAXIO_API_KEY set the pair is not used, so the password is unused.

import (
	"errors"
	"strings"

	"maxio-pp-cli/internal/cliutil"
)

// ErrLegacyPassword is returned by Load when MAXIO_PASSWORD is set to anything
// but the constant x.
var ErrLegacyPassword = errors.New("MAXIO_PASSWORD must be the literal x (Maxio API keys always use the password x); unset MAXIO_PASSWORD or set MAXIO_API_KEY to your API key")

func legacyBasicUsername() string {
	return strings.TrimSpace(cliutil.EnvOverride("MAXIO_USERNAME"))
}

// applyLegacyBasicEnv runs after the MAXIO_API_KEY override in Load.
func applyLegacyBasicEnv(cfg *Config) error {
	if strings.TrimSpace(cliutil.EnvOverride("MAXIO_API_KEY")) != "" {
		return nil
	}
	if p := cliutil.EnvOverride("MAXIO_PASSWORD"); p != "" && strings.TrimSpace(p) != "x" {
		return ErrLegacyPassword
	}
	if v := legacyBasicUsername(); v != "" {
		cfg.MaxioApiKey = v
		cfg.markEnvOverride("MaxioApiKey")
		cfg.AuthSource = "env:MAXIO_USERNAME"
		cfg.CredentialSource = "env:MAXIO_USERNAME"
	}
	return nil
}
