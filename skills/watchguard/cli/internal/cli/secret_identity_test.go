// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"watchguard-pp-cli/internal/store"
)

func allStoreResourceTypes() []string {
	seen := map[string]bool{}
	var out []string
	add := func(r string) {
		if r != "" && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, r := range knownSyncResourceNames() {
		add(r)
	}
	for _, d := range dependentResourceDefs() {
		add(d.Name)
	}
	add("activations")
	return out
}

func decodeRow(t *testing.T, raw string) map[string]any {
	t.Helper()
	obj, err := store.DecodeJSONObject(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

// hand-fix (handfixes.json secret-never-row-identity): no secret-named field
// may be, or contribute to, a row identity on any id-resolution path. Every
// spec-shaped list item with a secret field (no invented "id") is resolved as
// every store resource type: the id never holds the secret and does not move
// when only the secret changes.
func TestNoSecretFieldEverKeysARow(t *testing.T) {
	rows := map[string]string{}
	for path, raw := range specShapedSecretRows {
		rows[path] = raw
	}
	// Hand-shaped edges the spec allows: a line item with no lineItemId, and a
	// batch with no batchId whose only key-like field is the nested secret.
	rows["line item, key only"] = `{"activationKey":"SENTINEL-activationKey","status":"Activated"}`
	rows["batch, no batchId"] = `{"activationStatus":"Complete","lineItemCount":1,"firstLineItem":{"activationKey":"SENTINEL-activationKey"}}`
	rows["secret only"] = `{"activationKey":"SENTINEL-activationKey","psk":"SENTINEL-psk","pfx":"SENTINEL-pfx","password":"SENTINEL-password"}`
	checked := 0
	for name, raw := range rows {
		other := strings.ReplaceAll(raw, "SENTINEL-", "DIFFERENT-")
		for _, rt := range allStoreResourceTypes() {
			for label, resolve := range map[string]func(string, map[string]any) string{
				"ExtractResourceID": store.ExtractResourceID,
				"ResolveStorageID":  store.ResolveStorageID,
			} {
				a, b := resolve(rt, decodeRow(t, raw)), resolve(rt, decodeRow(t, other))
				checked++
				if strings.Contains(a, "SENTINEL") || strings.Contains(b, "DIFFERENT") {
					t.Errorf("%s(%s) keyed %s on a secret: %q", label, rt, name, a)
				} else if a != b {
					t.Errorf("%s(%s) for %s changed with only the secret: %q vs %q", label, rt, name, a, b)
				}
			}
		}
	}
	if checked < 500 {
		t.Fatalf("only %d resolutions checked", checked)
	}
}

// Activation rows key on the vendor's own ids: a recent-activations batch on
// batchId, a batch-status line item on lineItemId.
func TestActivationRowsKeyOnVendorIDs(t *testing.T) {
	for raw, want := range map[string]string{
		specShapedSecretRows["/rest/platform/activation/v1/recentactivations"]:                 `v-batchId`,
		specShapedSecretRows["/rest/platform/activation/v1/activationbatchstatuses/{batchId}"]: `00000000-0000-0000-0000-000000000000`,
	} {
		if got := store.ResolveStorageID("activations", decodeRow(t, raw)); got != want {
			t.Errorf("activations row keyed %q, want %q (%s)", got, want, raw)
		}
	}
}

type wgActivationsGetter struct{}

func (wgActivationsGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"results":[` +
		`{"activationStatus":"Complete","batchId":"b-1","lineItemCount":1,"firstLineItem":{"lineItemId":"li-1","activationKey":"SENTINEL-KEY-1"}},` +
		`{"activationStatus":"Processing","lineItemCount":2,"firstLineItem":{"activationKey":"SENTINEL-KEY-2"}}` +
		`],"pagination":{"totalItems":2}}`), nil
}
func (wgActivationsGetter) RateLimit() float64 { return 0 }

// A full activations sync over spec-shaped (id-less) rows succeeds - a batch
// with no batchId is stored under a fingerprint of its non-secret fields
// rather than failing the run - and no activation key reaches the database
// (resources.id, the typed table or FTS).
func TestActivationsSyncStoresNoKey(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var events bytes.Buffer
	res := syncResource(context.Background(), wgActivationsGetter{}, db, "activations", "", true, 0, false, true, nil, &events)
	if res.Err != nil || res.IntegrityFailure || res.Count != 2 {
		t.Fatalf("activations sync: count=%d err=%v integrity=%v events=%s", res.Count, res.Err, res.IntegrityFailure, events.String())
	}
	if row, _ := db.Get("activations", "b-1"); row == nil {
		t.Fatal("batch b-1 not stored under its batchId")
	}
	line := `{"lineItemId":"li-9","activationKey":"SENTINEL-KEY-3","status":"Activated"}`
	keyOnly := `{"activationKey":"SENTINEL-KEY-4","status":"Activated"}`
	if _, _, err := db.UpsertBatch("activations", []json.RawMessage{json.RawMessage(line), json.RawMessage(keyOnly)}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertActivations(json.RawMessage(keyOnly)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	for _, f := range []string{path, path + "-wal"} {
		if b, err := os.ReadFile(f); err == nil && bytes.Contains(b, []byte("SENTINEL-KEY")) {
			t.Fatalf("an activation key reached %s", filepath.Base(f))
		}
	}
}
