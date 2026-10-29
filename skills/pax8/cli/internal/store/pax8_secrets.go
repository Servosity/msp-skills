// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

// pax8 handfix (store-never-keeps-secrets): every store write passes
// mergeIncomingResourceData (sync batches, typed upserts, the write-through
// cache and the keep-richer merge), which returns withoutStoredSecrets of the
// record it is about to write. Values under credential field names
// (cliutil.SecretFieldName, the rule output redaction uses: API key secrets,
// the S3 secretAccessKey, tunnel PSKs, passwords, tokens) are replaced by
// "[redacted]" at any depth, and any key or string that repeats a credential
// this process holds, in any form cliutil.HeldEchoes finds, is withheld. A
// record that cannot be decoded, or that still repeats a held value, is
// stored as {}. Row identifiers pass storedID (resourceStorageID and
// upsertGenericResourceTx), so an id that repeats a held value is never
// written to an id column or the FTS index.

import (
	"bytes"
	"encoding/json"

	"pax8-pp-cli/internal/cliutil"
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
	if err := enc.Encode(withoutHeldValues(cliutil.RedactSecretFields(v))); err != nil {
		return json.RawMessage("{}")
	}
	out := bytes.TrimRight(buf.Bytes(), "\n")
	if cliutil.HeldEchoes(string(out)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(out)
}

func withoutHeldValues(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if cliutil.HeldEchoes(k) {
				delete(t, k)
				continue
			}
			t[k] = withoutHeldValues(child)
		}
	case []any:
		for i, child := range t {
			t[i] = withoutHeldValues(child)
		}
	case string:
		if cliutil.HeldEchoes(t) {
			return cliutil.FunnelWithheldText
		}
	}
	return v
}

// withheldStorageID is the one row id every record whose own id repeats a
// held credential is stored under.
const withheldStorageID = "[withheld]"

func storedID(id string) string {
	if cliutil.HeldEchoes(id) {
		return withheldStorageID
	}
	return id
}
