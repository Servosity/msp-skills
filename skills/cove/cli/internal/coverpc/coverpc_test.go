// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package coverpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(srv.URL)
	c.Creds = Credentials{Partner: "acme", Username: "api@acme.test", Password: "secret"}
	c.SessionPath = filepath.Join(t.TempDir(), "session.json")
	return c
}

func TestLoginCachesVisaAndCallInjectsIt(t *testing.T) {
	var sawVisa string
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		switch req["method"] {
		case "Login":
			params := req["params"].(map[string]any)
			if params["partner"] != "acme" || params["username"] != "api@acme.test" {
				t.Fatalf("unexpected login params: %v", params)
			}
			w.Write([]byte(`{"id":"jsonrpc","jsonrpc":"2.0","visa":"visa-1","result":{"result":{"Id":7,"PartnerId":1234,"EmailAddress":"api@acme.test"}}}`))
		case "GetServerInfo":
			sawVisa, _ = req["visa"].(string)
			w.Write([]byte(`{"id":"jsonrpc","jsonrpc":"2.0","visa":"visa-2","result":{"result":{"Version":"25.1"}}}`))
		default:
			t.Fatalf("unexpected method %v", req["method"])
		}
	})

	result, err := c.Call(context.Background(), "GetServerInfo", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if sawVisa != "visa-1" {
		t.Fatalf("expected injected visa-1, got %q", sawVisa)
	}
	inner, err := InnerResult(result)
	if err != nil {
		t.Fatalf("InnerResult: %v", err)
	}
	var info struct{ Version string }
	if err := json.Unmarshal(inner, &info); err != nil || info.Version != "25.1" {
		t.Fatalf("unexpected inner result: %s err=%v", inner, err)
	}
	// Visa rotation: the response's visa-2 must replace visa-1.
	if c.visa != "visa-2" {
		t.Fatalf("expected rotated visa-2, got %q", c.visa)
	}
	if calls != 2 {
		t.Fatalf("expected 2 HTTP calls (Login + GetServerInfo), got %d", calls)
	}
}

func TestCallRetriesOnceOnVisaError(t *testing.T) {
	step := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		step++
		switch step {
		case 1: // first Login (no cached session)
			w.Write([]byte(`{"id":"jsonrpc","jsonrpc":"2.0","visa":"stale","result":{"result":{}}}`))
		case 2: // EnumeratePartners with stale visa → 1701
			w.Write([]byte(`{"id":"jsonrpc","jsonrpc":"2.0","error":{"code":-32603,"data":1701,"message":"Visa is inconsistent/corrupted."}}`))
		case 3: // re-login
			if req["method"] != "Login" {
				t.Fatalf("step 3 expected re-Login, got %v", req["method"])
			}
			w.Write([]byte(`{"id":"jsonrpc","jsonrpc":"2.0","visa":"fresh","result":{"result":{}}}`))
		case 4: // retried call succeeds
			if v, _ := req["visa"].(string); v != "fresh" {
				t.Fatalf("retry should carry fresh visa, got %q", v)
			}
			w.Write([]byte(`{"id":"jsonrpc","jsonrpc":"2.0","visa":"fresh","result":{"result":[{"Id":1}]}}`))
		default:
			t.Fatalf("unexpected extra call %d", step)
		}
	})

	result, err := c.Call(context.Background(), "EnumeratePartners", map[string]any{"parentPartnerId": 1})
	if err != nil {
		t.Fatalf("Call after visa heal: %v", err)
	}
	rows, err := Rows(result)
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected 1 row, got %v err=%v", rows, err)
	}
}

func TestRPCErrorMapping(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["method"] == "Login" {
			w.Write([]byte(`{"id":"jsonrpc","jsonrpc":"2.0","error":{"code":-32603,"data":2100,"message":"Unknown partner/username or bad password"}}`))
			return
		}
		t.Fatalf("unexpected method %v", req["method"])
	})
	_, err := c.Login(context.Background())
	if err == nil {
		t.Fatal("expected login error")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T: %v", err, err)
	}
	if rpcErr.Data != ErrDataBadCredentials {
		t.Fatalf("expected data 2100, got %d", rpcErr.Data)
	}
}

func TestSessionPersistsAndClears(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"jsonrpc","jsonrpc":"2.0","visa":"persisted","result":{"result":{}}}`))
	})
	if _, err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	age, ok := c.SessionAge()
	if !ok || age > time.Minute {
		t.Fatalf("expected fresh persisted session, ok=%v age=%v", ok, age)
	}
	// A fresh client against the same path should reuse the cached visa.
	c2 := New(c.BaseURL)
	c2.Creds = c.Creds
	c2.SessionPath = c.SessionPath
	visa, err := c2.Visa(context.Background())
	if err != nil || visa != "persisted" {
		t.Fatalf("expected cached visa reuse, got %q err=%v", visa, err)
	}
	if err := c.ClearSession(); err != nil {
		t.Fatalf("ClearSession: %v", err)
	}
	if _, ok := c.SessionAge(); ok {
		t.Fatal("expected no session after clear")
	}
}

func TestFlattenSettings(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want map[string]string
	}{
		{
			name: "list of single-key objects",
			in:   []any{map[string]any{"I1": "host-1"}, map[string]any{"I14": "1048576"}},
			want: map[string]string{"I1": "host-1", "I14": "1048576"},
		},
		{
			name: "array values keep first element",
			in:   []any{map[string]any{"I78": []any{"D1,D2"}}},
			want: map[string]string{"I78": "D1,D2"},
		},
		{
			name: "non-list input yields empty map",
			in:   map[string]any{"I1": "x"},
			want: map[string]string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FlattenSettings(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("key %s: got %q want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestSettingHelpersAndStatusNames(t *testing.T) {
	settings := map[string]string{"D9F00": "2", "D9F09": "1700000000", "I14": "notanumber"}
	if n, ok := SettingInt(settings, "D9F00"); !ok || n != 2 {
		t.Fatalf("SettingInt D9F00: got %d ok=%v", n, ok)
	}
	if _, ok := SettingInt(settings, "I14"); ok {
		t.Fatal("SettingInt should fail on non-numeric")
	}
	if _, ok := SettingInt(settings, "MISSING"); ok {
		t.Fatal("SettingInt should fail on missing key")
	}
	ts, ok := SettingTime(settings, "D9F09")
	if !ok || ts.Year() < 2023 {
		t.Fatalf("SettingTime: got %v ok=%v", ts, ok)
	}
	if StatusName(2) != "Failed" || StatusName(99) != "Status99" {
		t.Fatalf("StatusName mapping broken: %s / %s", StatusName(2), StatusName(99))
	}
	if !BadSessionStatuses[2] || BadSessionStatuses[5] {
		t.Fatal("BadSessionStatuses misclassifies")
	}
}
