// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cipp/handfixes.json under the
// "secret-echo-decoded-funnel" entry (class "secret-echo").

package cli

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestAuthLogin_AADErrorBranchWithholdsEncodedSecret is the Codex round-2
// input: a token endpoint whose Azure AD JSON error echoes the client secret
// base64-encoded in error_description. The complete raw body is funnelled
// before either error branch formats a field out of it.
func TestAuthLogin_AADErrorBranchWithholdsEncodedSecret(t *testing.T) {
	echo := base64.StdEncoding.EncodeToString([]byte(loginTestSecret))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"invalid_client","error_description":"%s"}`, echo)
	}))
	defer srv.Close()
	_, err := runAuthLogin(t, srv.URL, filepath.Join(t.TempDir(), "config.toml"))
	if err == nil {
		t.Fatal("a 400 from the token endpoint must surface as an error")
	}
	msg := err.Error()
	if strings.Contains(msg, echo) || strings.Contains(msg, loginTestSecret) {
		t.Fatalf("the Azure AD error branch printed the client secret:\n%s", msg)
	}
	if !strings.Contains(msg, "HTTP 400") {
		t.Fatalf("the status line was lost:\n%s", msg)
	}
}
