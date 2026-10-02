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

func TestRequestContractDELETEKeepsBodyAndQueryIndependent(t *testing.T) {
	received := make(chan struct{}, 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		if r.Method != http.MethodDelete || r.URL.Path != "/ExecApiClient" {
			t.Errorf("DELETE route: %s %s", r.Method, r.URL)
		}
		if want := map[string][]string{"Action": {"query-action"}}; !reflect.DeepEqual(map[string][]string(r.URL.Query()), want) {
			t.Errorf("DELETE query: got %v want %v", r.URL.Query(), want)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("DELETE needs its declared JSON body: %v", err)
		}
		if want := map[string]any{"Action": "body-action", "ClientId": "fixture-client"}; !reflect.DeepEqual(body, want) {
			t.Errorf("DELETE body: got %#v want %#v", body, want)
		}
		if r.Header.Get("Authorization") != "Bearer contract-test-token" {
			t.Error("DELETE lost authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fixture":true}`))
	}))
	defer fixture.Close()
	requestContractConfig(t, fixture.URL)
	params := map[string]any{"action": "query-action", "Action": "body-action", "ClientId": "fixture-client"}
	result, err := handleCodeOrchExecute(context.Background(), requestContractCall(map[string]any{"endpoint_id": "exec-api-client.delete", "params": params}))
	if err != nil || result == nil || result.IsError {
		t.Fatalf("DELETE execute: %#v %v", result, err)
	}
	select {
	case <-received:
	default:
		t.Fatal("DELETE fixture was not called")
	}
}

func TestRequestContractIdenticalQueryAndBodyNames(t *testing.T) {
	for _, withQuery := range []bool{true, false} {
		name := "body only"
		if withQuery {
			name = "independent query and body"
		}
		t.Run(name, func(t *testing.T) {
			received := make(chan struct{}, 1)
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- struct{}{}
				if r.Method != http.MethodPost || r.URL.Path != "/EditSpamFilter" {
					t.Errorf("route: %s %s", r.Method, r.URL)
				}
				wantQuery := map[string][]string{}
				if withQuery {
					wantQuery["name"] = []string{"query-name"}
				}
				if !reflect.DeepEqual(map[string][]string(r.URL.Query()), wantQuery) {
					t.Errorf("same-name query: got %v want %v", r.URL.Query(), wantQuery)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if want := map[string]any{"name": "body-name"}; !reflect.DeepEqual(body, want) {
					t.Errorf("body name stolen by query: got %#v want %#v", body, want)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"fixture":true}`))
			}))
			defer fixture.Close()
			requestContractConfig(t, fixture.URL)
			params := map[string]any{"name": "body-name"}
			if withQuery {
				params["query_name"] = "query-name"
			}
			result, err := handleCodeOrchExecute(context.Background(), requestContractCall(map[string]any{"endpoint_id": "edit-spam-filter.create", "params": params}))
			if err != nil || result == nil || result.IsError {
				t.Fatalf("execute: %#v %v", result, err)
			}
			select {
			case <-received:
			default:
				t.Fatal("same-name fixture was not called")
			}
		})
	}
}
