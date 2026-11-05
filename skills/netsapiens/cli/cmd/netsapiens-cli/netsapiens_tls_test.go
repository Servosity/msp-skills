// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// vendorTLS starts a fake vendor over https and returns the variables that make
// the built binary trust it: the credential kit refuses to send a credential over
// plain http, as it should.
func vendorTLS(t *testing.T, h http.Handler) (*httptest.Server, []string) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(ca, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return srv, []string{"NETSAPIENS_CA_CERT=" + ca, "SSL_CERT_FILE=" + ca}
}
