// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"

	"zoom-phone-pp-cli/internal/cliutil"
)

// zoomSyncReplaceDepth > 0 makes writes replace the stored row instead of the
// keep-richer merge. Every syncResource call (the sync command, the pre-read
// auto-refresh and the channel workflow) holds it for its duration: a list
// sync is the authoritative current state, so a field that went absent, null
// or empty (an unassigned number's assignee, an emptied phone_numbers array)
// must not survive. Write-through from detail reads keeps merging, so a detail
// response does not erase list-only fields. A counter, not a flag, so
// concurrent sync workers do not switch it off for each other.
var zoomSyncReplaceDepth atomic.Int32

// BeginSyncReplace switches replace-on-write on until the returned func runs.
func BeginSyncReplace() func() {
	zoomSyncReplaceDepth.Add(1)
	return func() { zoomSyncReplaceDepth.Add(-1) }
}

// zoomSecretKeys are response fields that carry credentials (device SIP
// passwords, desk-phone and port-order PINs, smartphone activation codes,
// Team Chat channel tokens). They are never written to the local store, so the
// read-only `sql` / `search` tools cannot return them.
var zoomSecretKeys = cliutil.SecretFieldKeys

func zoomRedactSecrets(data json.RawMessage) json.RawMessage {
	if !bytes.Contains(data, []byte(`"`)) {
		return data
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return data
	}
	if !zoomRedactValue(v) {
		return data
	}
	out, err := json.Marshal(v)
	if err != nil {
		return data
	}
	return out
}

func zoomRedactValue(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if zoomSecretKeys[k] && val != nil {
				if s, ok := val.(string); !ok || s != "" {
					t[k] = "<redacted>"
					changed = true
					continue
				}
			}
			if zoomRedactValue(val) {
				changed = true
			}
		}
	case []any:
		for _, item := range t {
			if zoomRedactValue(item) {
				changed = true
			}
		}
	}
	return changed
}

// RedactSecrets returns data with the credential fields in zoomSecretKeys
// replaced, for live output paths that must match what the store keeps.
func RedactSecrets(data json.RawMessage) json.RawMessage { return zoomRedactSecrets(data) }

var zoomUTF8BOM = []byte{0xEF, 0xBB, 0xBF}

// RedactSecretsStrict decodes data as exactly one JSON value (after a UTF-8
// BOM, as the client accepts), redacts credential fields by their decoded key
// names (so \u-escaped keys are caught), and reports whether anything changed.
// It returns an error when the value cannot be decoded or has trailing data, so
// callers can fail closed instead of passing an unredacted body through.
func RedactSecretsStrict(data []byte) ([]byte, bool, error) {
	body := bytes.TrimPrefix(data, zoomUTF8BOM)
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return data, false, err
	}
	if _, err := dec.Token(); err == nil {
		return data, false, errors.New("trailing data after the JSON value")
	} else if !errors.Is(err, io.EOF) {
		return data, false, err
	}
	if !zoomRedactValue(v) {
		return data, false, nil
	}
	out, err := json.Marshal(v)
	if err != nil {
		return data, false, err
	}
	return out, true, nil
}
