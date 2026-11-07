// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring). The run profile
// stores are a credential source (brief item 8(c)): a saved value under a
// credential flag's name or under base-url is held inside credkit.Start's window,
// from Files, so it is a startup credential like any other: one that collides
// with public text refuses startup instead of printing.

package credwire

import (
	"os"
	"path/filepath"
	"strings"

	"immybot-pp-cli/internal/cliutil"
)

// profileCredentialMarkers are the name parts that make a profile key a credential.
// internal/cli's TestProfileCredentialNamesCoverTheCLIs keeps them a superset of
// the CLI's own credentialLikeRunProfileFlag.
var profileCredentialMarkers = []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key", "client-secret"}

// ProfileCredentialName reports a profile key whose value is a credential.
func ProfileCredentialName(name string) bool {
	if CredentialFlag(name) {
		return true
	}
	name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
	for _, m := range profileCredentialMarkers {
		if strings.Contains(name, m) {
			return true
		}
	}
	return false
}

// ProfileStorePaths is every profile store a run could read: under the default
// home, under each --home, and the legacy store.
func ProfileStorePaths(homes []string) []string {
	var out []string
	for _, home := range append([]string{""}, homes...) {
		underHome(home, func() {
			if dir, err := cliutil.ConfigDir(); err == nil {
				out = append(out, filepath.Join(dir, "profiles.json"))
			}
		})
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".immybot-cli", "profiles.json"))
	}
	return out
}

// ReadProfileStores calls hold with the bytes of each regular file among paths.
func ReadProfileStores(paths []string, hold func([]byte)) {
	for _, p := range paths {
		if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
			continue // a pipe here would block every run
		}
		if data, err := os.ReadFile(filepath.Clean(p)); err == nil {
			hold(data)
		}
	}
}

// HoldProfileCredentials holds each saved value under a credential name, every
// duplicate included, and the password and query values of a base-url value.
func HoldProfileCredentials(data []byte) {
	cliutil.JSONLeaves(data, func(keys []string, value string) {
		if len(keys) < 4 || value == "" || !strings.EqualFold(keys[0], "profiles") || !strings.EqualFold(keys[2], "values") {
			return
		}
		switch {
		case strings.EqualFold(keys[3], "base-url"):
			cliutil.HoldURLCredentials(value)
		case ProfileCredentialName(keys[3]):
			cliutil.HoldValues("minted:profile", value)
		}
	})
}
