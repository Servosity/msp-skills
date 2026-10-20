// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "webhook-subscription-token-withheld" entry.

package client

import (
	"bytes"
	"encoding/json"
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
