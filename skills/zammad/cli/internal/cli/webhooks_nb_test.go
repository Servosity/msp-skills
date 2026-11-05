// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): next-batch Codex round 1 on the zammad
// 4.34.0 publish (2026-10-27). Pinned by this connector's handfixes.json
// under the "webhooks-listener" entry (class "webhooks").

package cli

import (
	"net/http"
	"strings"
	"testing"

	"zammad-pp-cli/internal/credkit"
)

// TestNBWebhookRowIsCheckedWhole: the delivery id came from a request
// header and was stored as sent, so a delivery id that repeated the signing
// secret put it in vendor_events (Codex nb-r1 [P1]). The class: every row,
// from a delivery or the catch-up, passes the held check whole before it is
// stored (webhookListener.record is the one insert).
func TestNBWebhookRowIsCheckedWhole(t *testing.T) {
	if !kitInChild(t) {
		return
	}
	f := newWebhookFixture(t)
	secret := "nbw-" + strings.Repeat("Wb6r", 12)
	credkit.Hold("minted:webhook-secret", secret) // LoadWebhookSecret holds it in the real command
	f.l.secret = []byte(secret)
	body := []byte(`{"event":"zammad.ticket.updated","ticket_id":"42","article_id":""}`)
	if w := f.post(http.MethodPost, body, sign(secret, body), secret); w.Code == http.StatusOK {
		t.Errorf("a delivery id repeating the secret was answered %d %s", w.Code, w.Body)
	}
	for _, e := range f.rows(t) {
		if strings.Contains(e.DeliveryID+e.EventID+e.ResourceID+string(e.Data), secret) {
			t.Fatalf("vendor_events holds the secret: %+v", e)
		}
	}
	if w := f.post(http.MethodPost, body, sign(secret, body), "nb-d1"); w.Code != http.StatusOK {
		t.Errorf("a clean delivery after the refused one: %d %s", w.Code, w.Body)
	}
}
