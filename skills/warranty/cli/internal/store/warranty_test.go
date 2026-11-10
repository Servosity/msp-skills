// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-store).

package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"warranty-pp-cli/internal/credkit"
	"warranty-pp-cli/internal/warranty"
)

// The kit's SQL driver refuses a mirror write that carries a held vendor response value,
// and the transaction leaves nothing behind.
func TestMirrorRefusesHeldValues(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	s, err := OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data-warranty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	held := "store-held-secret-9P8O"
	credkit.Hold("response:password", held) // synccpu: the store refuses only a held vendor response value
	ctx := context.Background()
	if _, err := s.UpsertDevice(ctx, DeviceUpsert{Serial: "1ABC234", Customer: held}); !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("upsert with a held customer: err = %v", err)
	}
	rec := warranty.Record{Serial: "1ABC234", Vendor: warranty.Dell, Result: warranty.ResultOK,
		Lines: []warranty.Line{{LineID: "x", Kind: warranty.KindWarranty, Description: held, Hardware: true, Active: true}}}
	if _, err := s.ApplyLookup(ctx, LookupWrite{Record: rec, CheckedAt: "2026-10-28T00:00:00Z", AsOf: "2026-10-28"}); !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("lookup with a held line description: err = %v", err)
	}
	if _, ok, _ := s.GetDevice(ctx, "1ABC234"); ok {
		t.Fatal("a refused write left a row")
	}
}
