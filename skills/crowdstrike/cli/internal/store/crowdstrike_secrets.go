// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// crowdstrike handfix (store-never-keeps-secrets): every store write passes
// mergeIncomingResourceData (sync batches, typed upserts, the write-through
// cache and the keep-richer merge), which returns withoutStoredSecrets of the
// record it is about to write. Values under credential field names
// (cliutil.SecretFieldName, the one name rule: API key secrets, the S3
// secretAccessKey, tunnel PSKs, passwords, tokens) are replaced by
// "[redacted]" at any depth. A record that cannot be decoded is stored as {}.
// A statement that repeats a credential this process holds is the credential
// kit's: every store handle opens through internal/credsql.

import (
	"bytes"
	"encoding/json"

	"crowdstrike-pp-cli/internal/cliutil"
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
