// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "webhook-subscription-token-withheld" entry.

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// WebhookTokenWithheld stands in for a webhook subscription's token.
const WebhookTokenWithheld = "[withheld]"

// withholdWebhookTokens replaces every `token` value in a Webhooks API
// (/outgoingWebhookApi/) response with WebhookTokenWithheld. SaaS Alerts
// attaches that token to each delivery so the MSP's receiver can tell a real
// delivery from a forged one, which makes it a credential: it must not reach
// command output, the response cache, the local store or an MCP client. The
// body is returned unchanged when the path is not a Webhooks API path, the
// body is not JSON, or it carries no token.
func withholdWebhookTokens(path string, body json.RawMessage) json.RawMessage {
	if !strings.Contains(path, "/outgoingWebhookApi/") {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return body
	}
	if !withholdTokenKeys(v) {
		return body
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return body
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

// withholdTokenKeys walks v and replaces the value of every object key named
// token (any case) that is not null. It reports whether it replaced anything.
func withholdTokenKeys(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if strings.EqualFold(k, "token") && val != nil {
				if s, ok := val.(string); ok && s == WebhookTokenWithheld {
					continue
				}
				t[k] = WebhookTokenWithheld
				changed = true
				continue
			}
			if withholdTokenKeys(val) {
				changed = true
			}
		}
	case []any:
		for _, item := range t {
			if withholdTokenKeys(item) {
				changed = true
			}
		}
	}
	return changed
}

// refuseWithheldWebhookToken rejects a Webhooks API write whose body carries
// the [withheld] placeholder as a token. Every read prints that placeholder in
// place of the real token, so an agent that copies a listed subscription into
// create or update would otherwise overwrite the receiver's real token with
// it. Refused before the dry-run branch too, so a preview fails exactly like
// the real send.
func refuseWithheldWebhookToken(method, path string, body []byte) error {
	if method == http.MethodGet || len(body) == 0 || !strings.Contains(path, "/outgoingWebhookApi/") {
		return nil
	}
	var v any
	if json.Unmarshal(body, &v) != nil || !containsWithheldToken(v) {
		return nil
	}
	return fmt.Errorf("refusing %s %s: token is %q, the placeholder this CLI prints instead of a real webhook token; send the real token, or leave token out to keep the current one", method, path, WebhookTokenWithheld)
}

func containsWithheldToken(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok && strings.EqualFold(k, "token") && s == WebhookTokenWithheld {
				return true
			}
			if containsWithheldToken(val) {
				return true
			}
		}
	case []any:
		for _, item := range t {
			if containsWithheldToken(item) {
				return true
			}
		}
	}
	return false
}

// requestWebhookTokens returns the token values a Webhooks API request body
// carries, so an error answer that quotes one back (in a message string or a
// non-JSON body) has it masked like the API key.
func requestWebhookTokens(path string, body []byte) []string {
	if len(body) == 0 || !strings.Contains(path, "/outgoingWebhookApi/") {
		return nil
	}
	var v any
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, val := range t {
				// A token under 8 characters is not masked: replacing every
				// "a" would garble the very error that says it is too short.
				if s, ok := val.(string); ok && strings.EqualFold(k, "token") {
					if len(strings.TrimSpace(s)) >= 8 {
						out = append(out, s)
					}
					continue
				}
				walk(val)
			}
		case []any:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(v)
	return out
}

// maskEchoedSecrets masks credentials (the partner key, Respond tokens, the
// request's webhook tokens) that an error answer quotes back. A body that is
// exactly one JSON value is decoded first, so an escaped echo (\/, \u002B,
// \") is caught in its keys and string values; the raw text is then masked as
// written, URL-encoded and path-encoded. It runs before the body is cut to its
// display length, so a secret straddling the cut cannot leave a prefix behind.
// Values under 8 characters are skipped, so a short one cannot garble the text.
func maskEchoedSecrets(body []byte, secrets []string) []byte {
	var tokens []string
	for _, s := range secrets {
		if len(s) >= 8 {
			tokens = append(tokens, s)
		}
	}
	if len(tokens) == 0 || len(body) == 0 {
		return body
	}
	sort.SliceStable(tokens, func(i, j int) bool { return len(tokens[i]) > len(tokens[j]) })
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	// Only a body that is exactly one JSON value is re-encoded, so trailing
	// content is never dropped from the error text.
	if dec.Decode(&v) == nil && len(bytes.TrimSpace(body[dec.InputOffset():])) == 0 {
		if masked, changed := maskTokenStrings(v, tokens); changed {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if enc.Encode(masked) == nil {
				body = bytes.TrimRight(buf.Bytes(), "\n")
			}
		}
	}
	text := string(body)
	for _, t := range tokens {
		text = strings.ReplaceAll(text, t, "****")
		text = strings.ReplaceAll(text, url.QueryEscape(t), "****")
		text = strings.ReplaceAll(text, url.PathEscape(t), "****")
	}
	return []byte(text)
}

func maskTokenStrings(v any, tokens []string) (any, bool) {
	switch t := v.(type) {
	case string:
		out := t
		for _, tok := range tokens {
			out = strings.ReplaceAll(out, tok, "****")
		}
		return out, out != t
	case map[string]any:
		out := make(map[string]any, len(t))
		changed := false
		for k, val := range t {
			nk, kc := maskTokenStrings(k, tokens)
			nv, vc := maskTokenStrings(val, tokens)
			out[nk.(string)] = nv
			changed = changed || kc || vc
		}
		return out, changed
	case []any:
		changed := false
		for i, val := range t {
			if nv, c := maskTokenStrings(val, tokens); c {
				t[i], changed = nv, true
			}
		}
		return t, changed
	}
	return v, false
}
