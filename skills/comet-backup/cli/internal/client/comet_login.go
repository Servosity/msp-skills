// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (not generated). comet-backup handfix
// (doctor-verdict-tracks-reality): doctor asks the client, not its own copy of
// the config, whether a Comet API call will carry an admin login, so it never
// reports a login it did not send as valid.

package client

import (
	"strings"

	"comet-backup-pp-cli/internal/config"
)

// CometLoginConfigured reports whether cometFormAuth adds an admin login to
// Comet API calls made with cfg (a username plus a password or session key),
// and the AuthType it sends (Password or SessionKey). It never returns the
// secret itself.
func CometLoginConfigured(cfg *config.Config) (authType string, ok bool) {
	if cfg == nil || strings.TrimSpace(cfg.CometBackupUsername) == "" {
		return "", false
	}
	field, secret := cometSecret(cfg)
	if secret == "" {
		return "", false
	}
	return field, true
}
