// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kaseyablackbox

import (
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// testCAFiles maps a test server's host:port to a PEM file holding its
// certificate. Tests trust the server through KASEYA_VSA_CA_CERT; the binary
// has no environment switch that turns TLS verification off.
var testCAFiles sync.Map

func trustTestServer(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	host := strings.TrimPrefix(srv.URL, "https://")
	path := filepath.Join(t.TempDir(), "ca.pem")
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(path, block, 0o600); err != nil {
		t.Fatalf("write test CA: %v", err)
	}
	testCAFiles.Store(host, path)
	return host
}

func testCAFile(t *testing.T, host string) string {
	t.Helper()
	// A host no test server answers (a look-alike the pin must refuse
	// before any lookup) has no CA: leave KASEYA_VSA_CA_CERT empty.
	if v, ok := testCAFiles.Load(host); ok {
		return v.(string)
	}
	return ""
}
