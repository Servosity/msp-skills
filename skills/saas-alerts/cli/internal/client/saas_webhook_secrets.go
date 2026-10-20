// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "webhook-subscription-token-withheld" entry.

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// WebhookTokenWithheld stands in for a webhook subscription's token.
const WebhookTokenWithheld = "[withheld]"

// withholdWebhookTokens replaces every `token` value in a Webhooks API
// (/outgoingWebhookApi/) response with WebhookTokenWithheld. SaaS Alerts
// attaches that token to each delivery so the MSP's receiver can tell a real
// delivery from a forged one, which makes it a credential: it must not reach
// command output, the response cache, the local store or an MCP client. The
// body is returned unchanged when the path is not a Webhooks API path or no
// value carries a token. Every JSON value in the body is walked (an answer
// may hold several, one per line); text that does not parse as JSON but holds
// a "token" key is withheld whole, since its value cannot be found reliably.
// webhookTokenStrict finds a JSON-shaped "token" key with a non-empty string
// value; it is the backstop for answers that parsed as JSON, where free text
// such as `missing token:` or `Refresh token: 'revoked'` must not match.
var webhookTokenStrict = regexp.MustCompile(`(?i)"token"[\s\p{Z}]*:[\s\p{Z}]*"([^"\n]+)"`)

// webhookTokenLoose also accepts single-quoted or unquoted keys and values (a
// JavaScript inspect print); it is used only on text that is not JSON. The
// value must close and hold no JSON punctuation, so it cannot run across a
// string boundary.
var webhookTokenLoose = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])["']?token["']?[\s\p{Z}]*:[\s\p{Z}]*["']([^"'\n,{}\[\]]+)["']`)

// webhookTokenWord is a cheap prefilter: a view without "token" cannot match.
var webhookTokenWord = regexp.MustCompile(`(?i)token`)

func withholdWebhookTokens(path string, body json.RawMessage) json.RawMessage {
	if !strings.Contains(path, "/outgoingWebhookApi/") {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	changed := false
	for {
		var v any
		err := dec.Decode(&v)
		if err == io.EOF {
			break
		}
		if err != nil {
			if webhookTokenValueLeft(body, webhookTokenLoose) {
				whole, _ := json.Marshal(WebhookTokenWithheld)
				return whole
			}
			return body
		}
		if withholdTokenKeys(v) {
			changed = true
		}
		if err := enc.Encode(v); err != nil {
			return body
		}
	}
	out := json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
	if !changed {
		out = body
	}
	// Backstop on what would leave: a value the walk could not see (a
	// duplicate key whose last copy is null, an escaped key name) still
	// withholds the whole answer.
	if webhookTokenValueLeft(out, webhookTokenStrict) {
		whole, _ := json.Marshal(WebhookTokenWithheld)
		return whole
	}
	return out
}

// webhookTokenValueLeft reports whether text, in any view the secret funnel's
// decoders produce (escapes, percent, HTML entities, unprintable characters,
// so the two cannot drift apart), still holds a token key with a value other
// than the placeholder.
func webhookTokenValueLeft(text []byte, pattern *regexp.Regexp) bool {
	return anyDecodedView(string(text), func(view string) bool {
		if !webhookTokenWord.MatchString(view) {
			return false
		}
		for _, m := range pattern.FindAllStringSubmatch(view, -1) {
			if v := strings.TrimSpace(m[1]); v != "" && v != WebhookTokenWithheld {
				return true
			}
		}
		return false
	})
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
