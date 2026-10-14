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

// hand-fix (handfixes.json tail-redacts-secrets): tail is a read-only MCP
// tool, so policy passwords and subscription signing keys never reach it.
func TestTailNeverStreamsSecretFields(t *testing.T) {
	body := `{"policies":[{"id":"p1","name":"Default","as_uninstall_password":"SENTINEL-PW"}],` +
		`"subscriptions":[{"id":"n1","secret_token":"SENTINEL-TOKEN","custom_headers":[{"key":"authorization","value":"SENTINEL-HDR"}]}]}`
	var out bytes.Buffer
	if err := fetchAndEmit(context.Background(), tailSecretGetter{body}, "/v1/policies", resourceReadConfig{}, json.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "SENTINEL") {
		t.Fatalf("tail streamed a secret field: %s", out.String())
	}
	if !strings.Contains(out.String(), `"Default"`) {
		t.Fatalf("tail dropped ordinary fields: %s", out.String())
	}
}
