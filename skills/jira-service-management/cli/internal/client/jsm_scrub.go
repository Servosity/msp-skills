// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"bytes"
	"encoding/json"
)

// Assets object responses carry avatar.mediaClientConfig.mediaJwtToken, a
// bearer JWT for Atlassian's media API that has nothing to do with the
// question asked. Every successful response passes through here before the
// response cache, the local store, write-through or stdout sees it, so the
// token is never persisted or echoed into an agent transcript.
// ponytail: exact-key scrub on the literal key; a vendor that \u-escapes key
// names would slip past the pre-check (Atlassian does not).
const jsmIncidentalSecretKey = "mediaJwtToken"

func jsmScrubIncidentalSecrets(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"`+jsmIncidentalSecretKey+`"`)) {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return body
	}
	jsmDropKey(v)
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if enc.Encode(v) != nil {
		return body
	}
	return bytes.TrimRight(out.Bytes(), "\n")
}

func jsmDropKey(v any) {
	switch t := v.(type) {
	case map[string]any:
		delete(t, jsmIncidentalSecretKey)
		for _, child := range t {
			jsmDropKey(child)
		}
	case []any:
		for _, child := range t {
			jsmDropKey(child)
		}
	}
}
