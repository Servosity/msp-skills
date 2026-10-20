// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/config"
)

const deviceTokenSentinel = "XX-DEVICE-TOKEN-SENTINEL-XX"

func TestRedactCredentialFields(t *testing.T) {
	cases := map[string]string{
		"top level":       `{"deviceToken":"` + deviceTokenSentinel + `","uuid":"u"}`,
		"list":            `{"devices":[{"uuid":"a","deviceToken":"` + deviceTokenSentinel + `"},{"uuid":"b","deviceToken":"` + deviceTokenSentinel + `"}],"nextPageToken":""}`,
		"nested":          `{"device":{"uuid":"a","nested":{"deviceToken":"` + deviceTokenSentinel + `"}}}`,
		"escaped key":     `{"device\u0054oken":"` + deviceTokenSentinel + `"}`,
		"other case":      `{"DeviceToken":"` + deviceTokenSentinel + `"}`,
		"kelvin key":      "{\"deviceTo\u212aen\":\"" + deviceTokenSentinel + "\"}",
		"escaped value":   `{"deviceToken":"XX\"-DEVICE-TOKEN-SENTINEL-XX"}`,
		"duplicate empty": `{"deviceToken":"` + deviceTokenSentinel + `","deviceToken":""}`,
	}
	for name, in := range cases {
		out, ok := RedactCredentialFields([]byte(in))
		if !ok {
			t.Errorf("%s: a body that is one JSON value was withheld", name)
			continue
		}
		if strings.Contains(string(out), "DEVICE-TOKEN-SENTINEL") {
			t.Errorf("%s: credential survived: %s", name, out)
		}
		if !strings.Contains(string(out), RedactedValue) && name != "duplicate empty" {
			t.Errorf("%s: no redaction marker: %s", name, out)
		}
	}
	// Round 3 (Codex P1): a body that is not one JSON value and names the key
	// in any encoding is withheld whole; no pattern finds every value in it.
	for name, in := range map[string]string{
		"codex concatenation": `{}]{"deviceToken":"` + deviceTokenSentinel + `"}`,
		"trailing junk":       `{"deviceToken":"` + deviceTokenSentinel + `"}garbage`,
		"truncated":           `{"devices":[{"deviceToken" : "` + deviceTokenSentinel + `","uuid":"a"`,
		"unterminated value":  `{"devices":[{"deviceToken":"` + deviceTokenSentinel,
		"escaped key junk":    `[{"device\u0054oken":"` + deviceTokenSentinel + `"}`,
		"form encoded":        `device%54oken=` + deviceTokenSentinel,
	} {
		if out, ok := RedactCredentialFields([]byte(in)); ok {
			t.Errorf("%s: a body that cannot be redacted was returned: %s", name, out)
		}
		if got := string(RedactCredentialFieldsOrWithhold([]byte(in))); got != CredentialBodyWithheld {
			t.Errorf("%s: error text = %q, want the withheld marker", name, got)
		}
	}

	// Bodies without a credential stay byte for byte identical, and a
	// rewritten body keeps big integers, HTML characters and the other fields.
	for _, in := range []string{
		`{"devices":[{"uuid":"a"}],"nextPageToken":"abc"}`,
		`{"deviceToken":null,"n":12345678901234567890}`,
		`{"deviceToken":""}`,
		"not json at all",
		`{"devices":[{"uuid":"a"`,
		``,
	} {
		if out, ok := RedactCredentialFields([]byte(in)); !ok || string(out) != in {
			t.Errorf("unchanged body was rewritten: %q -> %q (ok %v)", in, out, ok)
		}
	}
	out, _ := RedactCredentialFields([]byte(`{"n":12345678901234567890,"name":"a<b>&c","deviceToken":"` + deviceTokenSentinel + `"}`))
	var back map[string]json.RawMessage
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("rewritten body is not JSON: %v (%s)", err, out)
	}
	if string(back["n"]) != "12345678901234567890" || string(back["name"]) != `"a<b>&c"` || string(back["deviceToken"]) != `"[redacted]"` {
		t.Fatalf("rewrite changed other fields: %s", out)
	}
}

// The redaction lives in client.do(), the one place every response body is
// read, so every generated command, sync, the mirror and MCP inherit it.
func TestClientRedactsDeviceTokenAtDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"devices":[{"uuid":"00000000-0000-0000-0000-000000000001","deviceToken":"`+deviceTokenSentinel+`"}],"nextPageToken":""}`)
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "Bearer test-token"}, 5*time.Second, 0)
	c.NoCache = true
	data, err := c.Get(context.Background(), "/v1/devices", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), deviceTokenSentinel) {
		t.Fatalf("client returned the device token: %s", data)
	}
	if !strings.Contains(string(data), `"deviceToken":"[redacted]"`) || !strings.Contains(string(data), "00000000-0000-0000-0000-000000000001") {
		t.Fatalf("device row lost or not marked redacted: %s", data)
	}
}
