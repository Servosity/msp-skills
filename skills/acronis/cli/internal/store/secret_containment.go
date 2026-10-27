// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/acronis/handfixes.json under
// the "credential-fields-never-persisted" entry. See docs/reprint-survival.md.
// Ported from skills/servosity/cli/internal/store/secret_containment.go; a
// store written by an earlier release is not purged (accepted risk).

package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"acronis-pp-cli/internal/cliutil"
)

// Secret fields are recognised by cliutil.IsSecretField on the normalized key
// (lower case, separators removed), so client_secret, clientSecret,
// access_token, refresh_token, id_token and a nested password match alike.
// They are dropped from every row before it is written (the resources table,
// each typed table's data and columns, resources_fts) and are never used as a
// row's identity, so neither MCP sql/search nor any local reader sees them.

// neverPersistedResource reports whether a resource type's whole payload is a
// credential: the identity provider's answers (idp request-token returns an
// access, refresh and id token). The local store never keeps them, whatever
// fields the answer carries. The first word of the type decides: idp,
// idp-token and IDP_Tokens are refused; id-policies is not.
func neverPersistedResource(resourceType string) bool {
	words := strings.FieldsFunc(strings.ToLower(resourceType), func(r rune) bool {
		switch r {
		case '_', '-', '.', ' ', '/', ':':
			return true
		}
		return false
	})
	return len(words) > 0 && words[0] == "idp"
}

// errNotOneJSONDocument refuses a row that may carry a credential but cannot
// be parsed, so its fields cannot be judged.
var errNotOneJSONDocument = errors.New("refusing to persist a body that is not one JSON document and may carry a credential")

// persistableRow is the write choke point every resources, resources_fts and
// typed-table insert passes through. It refuses a never-persisted resource
// type, drops every secret field from obj in place (the typed columns read
// obj), and returns data without them.
func persistableRow(resourceType string, obj map[string]any, data json.RawMessage) (json.RawMessage, error) {
	if neverPersistedResource(resourceType) {
		return nil, fmt.Errorf("refusing to persist credential resource type %q: identity-provider answers carry tokens", resourceType)
	}
	if obj != nil {
		dropSecretFields(obj)
	}
	return scrubPersistedSecrets(data)
}

// cleanIncomingRow is the entry of every store write (Upsert, UpsertBatch,
// UpsertBatchDetailed and each typed Upsert*): it refuses a never-persisted
// resource type and returns the whole row without any secret field at any
// depth, before identity resolution, envelope unwrapping, merge or projection
// can read it. A credential-named subtree is gone before anything can pick it
// as the row or its id.
func cleanIncomingRow(resourceType string, data json.RawMessage) (json.RawMessage, error) {
	return persistableRow(resourceType, nil, data)
}

// CleanIncomingResponse is cleanIncomingRow for a whole API answer on its way
// to a store write outside sync (write-through and mutation answers): every
// secret field at any depth is gone before an envelope is unwrapped, so a
// collection under a credential-named key (tokens, secrets, credentials) can
// never become rows.
func CleanIncomingResponse(resourceType string, data json.RawMessage) (json.RawMessage, error) {
	return cleanIncomingRow(resourceType, data)
}

// scrubPersistedSecrets returns data without any secret field at any depth.
// Data that carries no such key is returned unchanged; data that may carry one
// but is not one JSON document is refused.
func scrubPersistedSecrets(data json.RawMessage) (json.RawMessage, error) {
	if !cliutil.MayCarrySecretField(data) {
		return data, nil
	}
	trimmed := bytes.TrimSpace(data)
	if !json.Valid(trimmed) {
		return nil, errNotOneJSONDocument
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, errNotOneJSONDocument
	}
	if !dropSecretFields(v) {
		return data, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("re-encoding a row without its credentials: %w", err)
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// scrubMergedSecrets is scrubPersistedSecrets for a step that cannot return an
// error: a row it cannot judge is passed on unchanged, and persistableRow
// refuses it at the write.
func scrubMergedSecrets(data json.RawMessage) json.RawMessage {
	if out, err := scrubPersistedSecrets(data); err == nil {
		return out
	}
	return data
}

// dropSecretFields removes every secret field from v at any depth and reports
// whether it removed anything.
func dropSecretFields(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if cliutil.IsSecretField(k, child) {
				delete(t, k)
				changed = true
				continue
			}
			if dropSecretFields(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range t {
			if dropSecretFields(child) {
				changed = true
			}
		}
	}
	return changed
}

// identityView returns obj as identity resolution may read it: without any
// secret field, so a credential never becomes resources.id, a typed id or an
// FTS id. obj itself is not modified; it is copied only when it holds one.
func identityView(obj map[string]any) map[string]any {
	if obj == nil || !holdsSecretField(obj) {
		return obj
	}
	view, _ := copyJSONValue(obj).(map[string]any)
	dropSecretFields(view)
	return view
}

func holdsSecretField(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if cliutil.IsSecretField(k, child) || holdsSecretField(child) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if holdsSecretField(child) {
				return true
			}
		}
	}
	return false
}

func copyJSONValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[k] = copyJSONValue(child)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = copyJSONValue(child)
		}
		return out
	}
	return v
}

// indexableField reports whether a field's value may enter resources_fts: a
// secret field (cliutil.IsSecretField) never does.
func indexableField(key string, value any) bool {
	return !cliutil.IsSecretField(key, value)
}
