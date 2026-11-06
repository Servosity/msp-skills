// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package cliutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// QBOSaveCredentialsTo is SaveCredentials for a named credentials file: the
// one a --config profile reads (its own data/credentials.toml) or the global
// one, so a token refresh saves where the next Load looks. Every credential
// is held first; the file is written durably and its permissions verified.
func QBOSaveCredentialsTo(path string, creds *Credentials) error {
	if path == "" {
		return fmt.Errorf("no credentials file path")
	}
	QBOHoldTOMLCredentials(creds)
	data, err := toml.Marshal(credentialsFileFrom(creds))
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}
	if err := DurableAtomicWritePrivateFile(path, data, 0o600, 0o700); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return &CredentialsPermissionError{Path: path, Err: fmt.Errorf("resolving saved file: %w", err)}
	}
	if err := VerifyCredsPerms(real); err != nil {
		return &CredentialsPermissionError{Path: path, Err: err}
	}
	return nil
}

// QBORemoveCredentialsFile is RemoveCredentials for a named credentials file.
func QBORemoveCredentialsFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing credentials: %w", err)
	}
	return nil
}

// QBOStoredSupersedes is the supersedes_config digest of the store a load reads,
// under the same ownership and mode checks and the same order: an explicit
// config's own store, then, when that is missing or has no values, the global
// one. It reads silently: a store that is refused or corrupt supersedes nothing,
// and the load that applies credentials is the one that warns about it.
func QBOStoredSupersedes(configPath string, explicit bool) string {
	if explicit {
		path, err := CredentialsFilePathForConfig(configPath)
		if err != nil {
			return ""
		}
		data, err := readExplicitCredentialBytes(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ""
		}
		if err == nil {
			rec, values := qboDecodeStoreRecord(data)
			if values {
				return rec
			}
		}
	}
	path, err := credentialsPath()
	if err != nil {
		return ""
	}
	data, err := readGlobalCredentialBytes(path)
	if err != nil {
		return ""
	}
	rec, _ := qboDecodeStoreRecord(data)
	return rec
}

// qboDecodeStoreRecord is a store's digest and whether it has credential values.
func qboDecodeStoreRecord(data []byte) (digest string, values bool) {
	var rec Credentials
	if toml.Unmarshal(data, &rec) != nil {
		return "", false
	}
	return rec.SupersedesConfig, rec.HasValues()
}
