// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// keeper handfix (store-never-keeps-secrets): every store write passes
// mergeIncomingResourceData, which returns withoutStoredSecrets of the record it
// is about to write. Values under credential field names (cliutil.SecretFieldName)
// are replaced by "[redacted]" at any depth; a record that cannot be decoded is
// stored as {}. A held value anywhere else in a statement is refused by the
// credential kit's SQLite driver (internal/credsql).

import (
	"bytes"
	"encoding/json"

	"keeper-pp-cli/internal/cliutil"
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
