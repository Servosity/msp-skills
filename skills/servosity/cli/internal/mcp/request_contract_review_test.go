// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRequestContractDELETEKeepsImageIDInBody(t *testing.T) {
	received := make(chan struct{}, 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		if r.Method != http.MethodDelete || r.URL.Path != "/companies/123/draas/images/" {
			t.Errorf("DELETE route: %s %s", r.Method, r.URL)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("DELETE body leaked into query: %s", r.URL.RawQuery)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("DELETE needs its declared JSON body: %v", err)
		}
		if want := map[string]any{"image_id": "fixture-image"}; !reflect.DeepEqual(body, want) {
			t.Errorf("DELETE body: got %#v want %#v", body, want)
		}
		if r.Header.Get("Authorization") != "Token contract-test-token" || r.Header.Get("X-Servosity-Mfa") != "fixture-mfa" {
			t.Error("DELETE lost authentication or the MFA header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixture":true}`))
	}))
	defer fixture.Close()
	requestContractConfig(t, fixture.URL)
	params := map[string]any{"id": "123", "image_id": "fixture-image", "X-Servosity-Mfa": "fixture-mfa"}
	result, err := handleCodeOrchExecute(context.Background(), requestContractCall(map[string]any{"endpoint_id": "companies.draas.companies-images-delete", "params": params}))
	if err != nil || result == nil || result.IsError {
		t.Fatalf("DELETE execute: %#v %v", result, err)
	}
	select {
	case <-received:
	default:
		t.Fatal("DELETE fixture was not called")
	}
}
