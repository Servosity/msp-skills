// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): Zammad's webhook signature check, alone in
// its file so the base-owned security policy can bind these reviewed bytes
// (gosec G505 flags the crypto/sha1 import). Pinned by this connector's
// handfixes.json under the "webhooks-listener" entry.

package cli

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

// validWebhookSignature checks Zammad's X-Hub-Signature: "sha1=" and the
// hex HMAC-SHA1 of the raw body under the shared secret
// (lib/user_agent.rb). SHA-1 is Zammad's fixed choice, not ours; as an
// HMAC it is not open to the collision attacks that retired SHA-1 hashes.
func validWebhookSignature(secret, body []byte, header string) bool {
	sig, ok := strings.CutPrefix(strings.TrimSpace(header), "sha1=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(sig)
	if err != nil || len(got) != sha1.Size {
		return false
	}
	mac := hmac.New(sha1.New, secret)
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
