// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).

package cliutil

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"threecx-pp-cli/internal/credkit"
)

// HoldProfileStore holds each value a profiles.json saves under a credential
// flag's name or under base-url, read token by token so every member of a
// duplicate key and everything before a syntax error counts. loadProfileStore
// calls it on the bytes it just read, whichever home they came from, before
// anything can print them; startup calls it for every home it can name, for a
// response that echoes a value without the profile being read. profiles.json is
// not in Declarations.Files: the kit would hold every string in it, its own keys
// included, and withhold any output naming them.
func HoldProfileStore(data []byte) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	holdProfileValue(dec, "")
}

// holdProfileValue reads one JSON value saved under key and reports whether the
// file can still be read.
func holdProfileValue(dec *json.Decoder, key string) bool {
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	switch v := tok.(type) {
	case json.Delim:
		for dec.More() {
			name := key
			if v == '{' {
				k, err := dec.Token()
				if err != nil {
					return false
				}
				name, _ = k.(string)
			}
			if !holdProfileValue(dec, name) {
				return false
			}
		}
		_, err := dec.Token()
		return err == nil
	case string:
		if credentialProfileKey(key) {
			credkit.Hold("profile:value", v)
		}
	case json.Number:
		if credentialProfileKey(key) {
			credkit.Hold("profile:value", v.String())
		}
	}
	return true
}

// ProfileCredentialFlags is credwire.Declarations.CredentialFlags: the checker's
// rule 1 lets only credkit.Start read that variable
// (TestProfileFlagsMatchTheDeclaration keeps the two the same).
var ProfileCredentialFlags = []string{
	"--api-key-value", "--config-custom-relay-config-client-private-key", "--settings-location-sftp-private-key", "--custom-relay-config-client-private-key-contents", "--license-key", "--location-sftp-private-key-contents",
	"--management-api-key-value", "--model-api-key", "--private-key-contents", "--remote-postgre-config-client-private-key-contents",
	"--sbc-private-key-contents", "--transcribe-settings-transcribe-key-grok-value",
	"--access-password", "--access-token", "--auth-password", "--auth-password-value", "--client-secret", "--client-secret-value",
	"--description-contents-encrypt-backup-password", "--deskphone-password", "--dir-search-settings-exchange-password-value",
	"--location-ftp-password-value", "--location-ns-password-value", "--location-sftp-password-value", "--mail-password-value",
	"--password", "--pin-number", "--provision-link", "--provision-url", "--provisioning-prov-link",
	"--remote-my-sql-config-password-value", "--remote-postgre-config-password-value", "--remote-sql-server-config-password-value",
	"--secret", "--service-account-credentials-access-key", "--service-account-credentials-secret-key-value",
	"--settings-contents-encrypt-backup-password", "--settings-encrypt-backup-password-value", "--settings-location-ftp-password",
	"--settings-location-ns-password", "--settings-location-sftp-password", "--settings-mail-password-value",
	"--state-pkce-code-verifier", "--transcribe-settings-transcribe-secret-key-value", "--trunk-auth-password-value",
	"--user-access-password", "--user-auth-password", "--user-deskphone-password", "--user-vmpin", "--verifier", "--vmpin",
}

// credentialProfileKey reports a profile value's name as a credential flag's,
// spelled as a flag or with underscores, or base-url. The markers are the ones
// profile save and apply refuse (credentialLikeRunProfileFlag).
func credentialProfileKey(key string) bool {
	name := strings.ToLower(strings.ReplaceAll(strings.TrimLeft(strings.TrimSpace(key), "-"), "_", "-"))
	if name == "base-url" || slices.Contains(ProfileCredentialFlags, "--"+name) {
		return true
	}
	for _, marker := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}
