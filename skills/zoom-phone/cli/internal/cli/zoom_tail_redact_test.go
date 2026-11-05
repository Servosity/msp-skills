// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestZoomRedactingLineWriter(t *testing.T) {
	var out bytes.Buffer
	w := &zoomRedactingLineWriter{w: &out}
	// Split a secret-bearing NDJSON line across writes, then a plain line, then an unterminated one.
	chunks := []string{`{"id":"a1","chat_channels":[{"token":"CHAT-`, "SECRET\"}]}\n", "not json\n", `{"pin":"1234"}`}
	for _, c := range chunks {
		if _, err := w.Write([]byte(c)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "CHAT-SECRET") || strings.Contains(got, "1234") {
		t.Fatalf("secret printed: %q", got)
	}
	if !strings.Contains(got, "not json\n") || strings.Count(got, "\n") != 2 {
		t.Fatalf("non-JSON lines or line breaks altered: %q", got)
	}
}

func TestZoomTailIsWrapped(t *testing.T) {
	if zoomFindExact(RootCmd(), "tail") == nil {
		t.Fatal("tail command not found; update zoom_tail_redact.go")
	}
}

func TestZoomTailRedactsLiveSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alert_settings":[{"alert_setting_id":"a1","chat_channels":[{"name":"ops","token":"CHAT-CHANNEL-TOKEN-LEAK"}]}],"next_page_token":""}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		t.Setenv(k, dir)
	}
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"tail", "alert_settings", "--follow=false", "--no-cache"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tail: %v", err)
	}
	if strings.Contains(out.String(), "CHAT-CHANNEL-TOKEN-LEAK") || !strings.Contains(out.String(), "a1") {
		t.Fatalf("tail output not redacted or empty: %q", out.String())
	}
}
