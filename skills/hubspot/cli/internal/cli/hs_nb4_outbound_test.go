// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Next-batch round 2 (handfixes.json credkit-wiring): the
// webhook and the feedback endpoint are other hosts, so a request to either
// that carries a held value is refused before it is sent.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hubspot-pp-cli/internal/credkit"
)

func TestNB4WebhookAndFeedbackNeverCarryAHeldValue(t *testing.T) {
	if runInChild(t) {
		return
	}
	const v = "nb4-webhook-secret-0001"
	credkit.Hold("config:test", v)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	if err := deliverWebhook(srv.URL+"/hook?t="+v, []byte(`{"ok":true}`), false); err == nil {
		t.Errorf("a webhook URL carrying a held value was posted to")
	}
	if err := deliverWebhook(srv.URL+"/hook", []byte(`{"note":"`+v+`"}`), false); err == nil {
		t.Errorf("a webhook body carrying a held value was posted")
	}
	if err := postFeedback(srv.URL, FeedbackEntry{Text: "broke on " + v}); err == nil {
		t.Errorf("feedback carrying a held value was posted")
	}
	if hits != 0 {
		t.Errorf("%d request(s) carrying a held value reached another host", hits)
	}
	if err := deliverWebhook(srv.URL+"/hook", []byte(`{"ok":true}`), false); err != nil || hits != 1 {
		t.Errorf("a clean webhook was not posted: %v", err)
	}
}
