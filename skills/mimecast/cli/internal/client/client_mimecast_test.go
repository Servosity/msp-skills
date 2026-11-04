// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"os"
	"strings"
	"testing"
)

func TestMimecastFunctionFailure(t *testing.T) {
	cases := []struct {
		body string
		fail bool
		want string
	}{
		{`{"meta":{"status":200},"data":[],"fail":[{"errors":[{"code":"err_validation_blank","message":"id is blank","retryable":false}],"key":{"id":""}}]}`, true, "err_validation_blank: id is blank"},
		{`{"fail":[{"code":"InvalidAccessToken","message":"Access token is invalid"}]}`, true, "InvalidAccessToken: Access token is invalid"},
		{`{"data":[{"id":"a","release":true}],"fail":[{"errors":[{"code":"x","message":"y"}]}]}`, false, ""}, // partial: command layer
		{`{"data":[{"id":"a","release":true}],"fail":[]}`, false, ""},
		{`{"outboundIpAddresses":["1.2.3.4"]}`, false, ""},
	}
	for _, c := range cases {
		msg, ok := mimecastFunctionFailure([]byte(c.body), false)
		if ok != c.fail || !strings.Contains(msg, c.want) {
			t.Errorf("body %s: got (%q,%v) want fail=%v containing %q", c.body, msg, ok, c.fail, c.want)
		}
	}
}

func TestMimecastFunctionFailureReads(t *testing.T) {
	body := []byte(`{"data":[{"heldEmails":[]}],"fail":[{"errors":[{"code":"err_window","message":"bad window"}]}]}`)
	if _, ok := mimecastFunctionFailure(body, false); ok {
		t.Fatal("write: data next to fail[] is a partial result, not a failure")
	}
	if msg, ok := mimecastFunctionFailure(body, true); !ok || !strings.Contains(msg, "err_window") {
		t.Fatalf("read: any fail[] must fail, got (%q,%v)", msg, ok)
	}
}

func TestMimecastInjectAccountCode(t *testing.T) {
	SetAccountCodeSource(func() string { return os.Getenv("MIMECAST_ACCOUNT_CODE") })
	t.Setenv("MIMECAST_ACCOUNT_CODE", "")
	body := map[string]any{"data": []any{}}
	if got := mimecastInjectAccountCode(body).(map[string]any); got["meta"] != nil {
		t.Fatalf("no env: meta should stay unset, got %v", got["meta"])
	}
	t.Setenv("MIMECAST_ACCOUNT_CODE", "C123")
	got := mimecastInjectAccountCode(map[string]any{"meta": map[string]any{"pagination": map[string]any{"pageSize": 5}}, "data": []any{}}).(map[string]any)
	meta := got["meta"].(map[string]any)
	if meta["accountCode"] != "C123" || meta["pagination"] == nil {
		t.Fatalf("env set: want accountCode and kept pagination, got %v", meta)
	}
	kept := mimecastInjectAccountCode(map[string]any{"meta": map[string]any{"accountCode": "OTHER"}, "data": []any{}}).(map[string]any)
	if kept["meta"].(map[string]any)["accountCode"] != "OTHER" {
		t.Fatal("caller-supplied accountCode must win")
	}
	if rest := mimecastInjectAccountCode(map[string]any{"mode": "gateway"}).(map[string]any); rest["meta"] != nil {
		t.Fatal("non-envelope REST bodies must be untouched")
	}
}
