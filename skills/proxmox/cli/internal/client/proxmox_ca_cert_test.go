// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by handfixes.json "unpinned-service-keeps-press-tls-trust".

package client

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"proxmox-pp-cli/internal/config"
)

// A service with no fingerprint pin keeps the press's own TLS trust
// (--ca-cert / PROXMOX_CA_CERT, --tls-server-name): SetServicePins must not
// replace it with Go's bare defaults, for PVE or for PBS.
func TestUnpinnedServiceKeepsCACert(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	pve, pbs := httptest.NewTLSServer(ok), httptest.NewTLSServer(ok)
	defer pve.Close()
	defer pbs.Close()
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pve.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	build := func(caCert string) *Client {
		cfg := &config.Config{
			BaseURL:            pve.URL + "/api2/json",
			TemplateVars:       map[string]string{"pbs_host": strings.TrimPrefix(pbs.URL, "https://")},
			ProxmoxApiToken:    "root@pam!cli=s3cret",
			ProxmoxPbsApiToken: "root@pam!cli:s3cret",
			CACert:             caCert,
		}
		c := New(cfg, 5*time.Second, 0)
		c.NoCache = true
		if err := c.TLSConfigError(); err != nil {
			t.Fatal(err)
		}
		if err := c.SetServicePins(nil, nil); err != nil {
			t.Fatal(err)
		}
		return c
	}
	// httptest signs every server with one certificate, so it vouches for both.
	c := build(ca)
	if _, err := c.Get(context.Background(), "/nodes", nil); err != nil {
		t.Fatalf("PVE with --ca-cert and no pin failed: %v", err)
	}
	if _, err := c.Get(context.Background(), pbsSpecOrigin+"/api2/json/admin/datastore", nil); err != nil {
		t.Fatalf("PBS with --ca-cert and no pin failed: %v", err)
	}
	if _, err := build("").Get(context.Background(), "/nodes", nil); err == nil {
		t.Fatal("an unknown CA was trusted without --ca-cert")
	}
}
