// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// Hand-written (NOT generated; handfixes.json store-never-keeps-secrets): every
// store write passes mergeIncomingResourceData (sync batches, typed upserts,
// the write-through cache and the keep-richer merge), which returns the record
// with every value under a credential field name (cliutil.SecretFieldName: a
// device's SNMP communities, its SNMP v3 keys, and its credentials object)
// replaced by "[redacted]". The credential kit's SQL driver
// (internal/credsql) withholds a statement carrying a held credential; masking
// by name first means such a record is stored, and a sync of it completes,
// without the credential. A record that does not decode is stored as written,
// and the kit judges it.

import (
	"bytes"
	"encoding/json"

	"domotz-pp-cli/internal/cliutil"
)

func withoutStoredSecrets(data json.RawMessage) json.RawMessage {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return data
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(cliutil.RedactSecretFields(v)); err != nil {
		return data
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}
