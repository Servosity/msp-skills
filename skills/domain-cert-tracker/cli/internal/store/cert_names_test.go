// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"path/filepath"
	"testing"
)

// CertNamesFor passes the ids as one JSON parameter; more ids than the old
// 500-per-query chunk must all come back, and unknown ids must not.
func TestCertNamesForManyIDs(t *testing.T) {
	ctx := context.Background()
	st, err := OpenWithContext(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var recs []CertNames
	var ids []int64
	for i := int64(1); i <= 1200; i++ {
		recs = append(recs, CertNames{CrtID: i, Status: CertNamesOK, Names: []string{"a.test"}})
		ids = append(ids, i)
	}
	if err := st.PutCertNames(ctx, recs); err != nil {
		t.Fatal(err)
	}
	got, err := st.CertNamesFor(ctx, append(ids, 99999))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1200 || got[1200].Names[0] != "a.test" {
		t.Fatalf("got %d records, want 1200", len(got))
	}
	if empty, err := st.CertNamesFor(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("nil ids: %v %v", empty, err)
	}
}
