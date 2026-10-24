// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package client

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"servosity-msp-pp-cli/internal/cliutil"
)

// secretReadPathFragments name the endpoints whose body is a credential
// whatever its shape (an INI file, an env listing): encryption keys, stored
// credentials, MFA codes and enrollment, DRaaS passwords and VPN config, agent,
// install, upload and download tokens, the Postmark token, and backup
// login/config. The same set is gated over MCP (internal/mcp/secret_reads.go).
var secretReadPathFragments = []string{
	"/api-token/", "/credentials/", "/encryption-key", "/mfa_codes/", "/mfa-backup-codes/",
	"/start-mfa/", "/extra-instance-password/", "/windows-helper-password/", "/client-vpn-config/",
	"/restic-env/", "/agent-token/", "/agent-install-token/", "/log-upload/", "/other-info/",
	"/servosity-one/download/", "/config.ini/", "/login/",
}

// responseNotCacheable reports whether a GET body must stay off disk: a body
// that is not JSON (the cache only serves JSON, and an INI or env body has no
// field names to judge), a response from a secret-read endpoint, or JSON that
// carries a non-empty secret-named field (cliutil.IsSecretFieldName).
func responseNotCacheable(path string, data json.RawMessage) bool {
	for _, frag := range secretReadPathFragments {
		if strings.Contains(path, frag) {
			return true
		}
	}
	return responseCarriesSecret(data)
}

// responseCarriesSecret reports whether a cached body must be withheld: it is
// not JSON, or it holds a non-empty secret-named field at any depth.
func responseCarriesSecret(data json.RawMessage) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}
	if !json.Valid(trimmed) {
		return true
	}
	if !cliutil.MayCarrySecretField(trimmed) {
		return false
	}
	var v any
	if json.Unmarshal(trimmed, &v) != nil {
		return true
	}
	return cliutil.SecretValuePresent(v)
}

// secretCachePurgeMarker records the one-time removal of every cached response
// older builds wrote from a secret read (encryption keys, credentials, MFA).
const secretCachePurgeMarker = ".security-purge-secret-reads-v0.6.11"

// purgeSecretResponseCache deletes, once per cache directory, each cached
// response whose body is not JSON or carries a secret-named field, and leaves
// every other entry alone.
// A file it cannot read or delete keeps the marker unwritten, so the next
// client tries again.
func purgeSecretResponseCache(cacheDir string) {
	if cacheDir == "" {
		return
	}
	marker := filepath.Join(cacheDir, secretCachePurgeMarker)
	if _, err := os.Stat(marker); err == nil {
		return
	}
	complete := true
	// Root-scoped: every read and delete stays inside the cache's resources
	// directory, whatever a path inside it points at.
	root, err := os.OpenRoot(filepath.Join(cacheDir, "resources"))
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		_ = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				complete = false
				return nil
			}
			if d.IsDir() || !strings.HasSuffix(name, ".json") {
				return nil
			}
			body, err := root.ReadFile(name)
			if err != nil {
				complete = false
				return nil
			}
			if responseCarriesSecret(body) {
				if err := root.Remove(name); err != nil && !os.IsNotExist(err) {
					complete = false
				}
			}
			return nil
		})
		_ = root.Close()
	}
	if complete {
		_ = cliutil.AtomicWritePrivateFile(marker, []byte("completed\n"), 0o600, 0o700)
	}
}
