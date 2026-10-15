// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type tailSecretGetter struct{ body string }

func (g tailSecretGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(g.body), nil
}

// hand-fix (handfixes.json tail-redacts-secrets, ported from threatdown):
// tail never streams a secret-named field. The activations resource returns
// product activation keys; Firebox rows carry PSKs and private keys.
func TestTailNeverStreamsSecretFields(t *testing.T) {
	body := `{"results":[{"id":"a1","serial":"FB-1","activationKey":"SENTINEL-ACT","firstLineItem":{"activationKey":"SENTINEL-ACT2"}},` +
		`{"id":"a2","serial":"FB-2","psk":"SENTINEL-PSK","pvt_key":"SENTINEL-PVT","wifi":{"passphrase":"SENTINEL-WIFI"}}]}`
	var out bytes.Buffer
	if err := fetchAndEmit(context.Background(), tailSecretGetter{body}, "/rest/platform/activation/v1/recentactivations", resourceReadConfigFor("activations"), json.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "SENTINEL") {
		t.Fatalf("tail streamed a secret field: %s", out.String())
	}
	if !strings.Contains(out.String(), `"FB-1"`) || !strings.Contains(out.String(), `"FB-2"`) {
		t.Fatalf("tail dropped ordinary fields: %s", out.String())
	}
}
