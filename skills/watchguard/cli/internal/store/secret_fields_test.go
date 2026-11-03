// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// hand-fix guard (handfixes.json secret-fields-never-persisted): a sentinel
// credential in a synced Firebox or activation row must not land anywhere in
// the SQLite file - not the generic blob, the typed columns or the FTS index.
func TestSecretFieldsNeverPersisted(t *testing.T) {
	const sentinel = "SENTINEL-WG-SECRET-7d1"
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	tunnel := json.RawMessage(`{"object":"t1","name":"branch-vpn","psk":"` + sentinel + `","psk_encrypted":"` + sentinel + `","big":12345678901234567890}`)
	if err := s.UpsertFireboxBovpnTunnels(tunnel); err != nil {
		t.Fatalf("UpsertFireboxBovpnTunnels: %v", err)
	}
	cert := json.RawMessage(`{"object":"c1","name":"web","password":"` + sentinel + `","pvt_key":"` + sentinel + `"}`)
	if _, _, err := s.UpsertBatch("firebox_certificates", []json.RawMessage{cert}); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}
	network := json.RawMessage(`{"object":"n1","name":"wan","pppoe_client":{"password":"` + sentinel + `"},"interfaces":[{"interface":{"wireless_settings":{"Passphrase":"` + sentinel + `"}}}]}`)
	if err := s.Upsert("firebox_networks", "n1", network); err != nil {
		t.Fatalf("Upsert networks: %v", err)
	}
	act := json.RawMessage(`{"id":"a1","activationKey":"` + sentinel + `","firstLineItem":{"activationKey":"` + sentinel + `"}}`)
	if _, _, err := s.UpsertBatch("activations", []json.RawMessage{act}); err != nil {
		t.Fatalf("UpsertBatch activations: %v", err)
	}
	if err := s.Upsert("endpoint_installers", "i1", json.RawMessage(`{"url":"https://dl.test/`+sentinel+`/setup.msi"}`)); err != nil {
		t.Fatalf("Upsert installers: %v", err)
	}

	var got string
	if err := s.DB().QueryRow(`SELECT data FROM resources WHERE resource_type = 'firebox_bovpn_tunnels'`).Scan(&got); err != nil {
		t.Fatalf("read tunnel row: %v", err)
	}
	if !strings.Contains(got, `"name":"branch-vpn"`) || !strings.Contains(got, "12345678901234567890") {
		t.Fatalf("non-secret fields lost: %s", got)
	}

	rows, err := s.DB().Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		tables = append(tables, n)
	}
	rows.Close()
	for _, tbl := range tables {
		r, err := s.DB().Query(`SELECT * FROM "` + tbl + `"`)
		if err != nil {
			continue
		}
		cols, _ := r.Columns()
		for r.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			_ = r.Scan(ptrs...)
			for i, v := range vals {
				var str string
				switch tv := v.(type) {
				case string:
					str = tv
				case []byte:
					str = string(tv)
				}
				if strings.Contains(str, sentinel) {
					t.Errorf("secret persisted in %s.%s", tbl, cols[i])
				}
			}
		}
		r.Close()
	}
}
