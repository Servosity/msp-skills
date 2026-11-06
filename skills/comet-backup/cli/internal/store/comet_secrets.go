// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// comet-backup handfix (store-never-keeps-secrets): every store write passes
// mergeIncomingResourceData (sync batches, typed upserts, the write-through
// cache and the keep-richer merge), which returns withoutStoredSecrets of the
// record it is about to write. Values under credential field names
// (cliutil.SecretFieldName, the rule output redaction uses: a PasswordHash or
// TOTPKey, destination S3/Azure/B2/SFTP keys and passwords, a bucket
// ReadWriteKey, session keys, tokens) are replaced by "[redacted]" at any
// depth; a record that cannot be decoded is stored as {}. A held credential in
// any other place is the credential kit's: every statement passes its SQL
// driver (internal/credsql).

import (
	"bytes"
	"encoding/json"

	"comet-backup-pp-cli/internal/cliutil"
)

func withoutStoredSecrets(data json.RawMessage) json.RawMessage {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return json.RawMessage("{}")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(cliutil.RedactSecretFields(v)); err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}
