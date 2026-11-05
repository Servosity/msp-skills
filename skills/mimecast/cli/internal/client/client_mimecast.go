// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: Mimecast API 2.0 envelope semantics the generic client cannot
// infer from the spec. Wired into client.go by two one-line calls (see
// handfixes.json): account-code injection before marshal, and function-level
// failure detection on 2xx responses.

package client

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
)

// mimecastInjectAccountCode adds meta.accountCode (Mimecast "Delegate Access")
// to envelope-shaped bodies ({"data":[...]}) when MIMECAST_ACCOUNT_CODE is set
// and the caller did not already supply one.
func mimecastInjectAccountCode(body any) any {
	code := MimecastAccountCode()
	if code == "" {
		return body
	}
	m, ok := body.(map[string]any)
	if !ok {
		return body
	}
	if _, isEnvelope := m["data"]; !isEnvelope {
		return body
	}
	meta, _ := m["meta"].(map[string]any)
	if _, set := meta["accountCode"]; set {
		return body
	}
	merged := map[string]any{"accountCode": code}
	maps.Copy(merged, meta)
	out := maps.Clone(m)
	out["meta"] = merged
	return out
}

// mimecastFunctionFailure detects Mimecast's "HTTP 200, function failed"
// shape. For reads any fail[] entry is a failure (a data wrapper such as
// [{"heldEmails":[]}] next to fail[] is not an answer). For writes only a
// fail[] with no data is: partial successes are left to the command layer
// so the successful items still print. It returns the joined "code: message".
func mimecastFunctionFailure(body []byte, read bool) (string, bool) {
	var env struct {
		Data json.RawMessage `json:"data"`
		Fail []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Errors  []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"fail"`
	}
	if err := json.Unmarshal(body, &env); err != nil || len(env.Fail) == 0 {
		return "", false
	}
	if d := strings.TrimSpace(string(env.Data)); !read && d != "" && d != "null" && d != "[]" {
		return "", false
	}
	var msgs []string
	for _, f := range env.Fail {
		if f.Code != "" || f.Message != "" {
			msgs = append(msgs, strings.TrimPrefix(f.Code+": "+f.Message, ": "))
		}
		for _, e := range f.Errors {
			msgs = append(msgs, strings.TrimPrefix(e.Code+": "+e.Message, ": "))
		}
	}
	if len(msgs) == 0 {
		msgs = append(msgs, "request failed (empty fail entry)")
	}
	return fmt.Sprintf("mimecast function failure: %s", strings.Join(msgs, "; ")), true
}

// accountCodeSource supplies the delegated-access account code. The cli
// package installs it at init (it reads MIMECAST_ACCOUNT_CODE there), so the
// optional MSP setting is not mistaken for a required install credential by
// the manifest's client/config env scan.
var accountCodeSource = func() string { return "" }

// SetAccountCodeSource installs the delegated-access account code reader.
func SetAccountCodeSource(f func() string) { accountCodeSource = f }

// MimecastAccountCode is the delegated-access account code, or "" for the
// API application's own account.
func MimecastAccountCode() string { return strings.TrimSpace(accountCodeSource()) }
