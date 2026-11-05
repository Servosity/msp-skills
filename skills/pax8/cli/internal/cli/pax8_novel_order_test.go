// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"encoding/json"

	"pax8-pp-cli/internal/store"
)

// A record stored under two names is read once: the row written last. The
// resources table stamps updated_at in whole seconds, so a tie has to be broken
// by something that advances on every write.

const (
	usageY = "00000000-0000-4000-8000-0000000000a1"
	usageZ = "00000000-0000-4000-8000-0000000000a2"
)

func usageRow(id, charges string) string {
	return `{"id":"` + id + `","companyId":"` + cid1 + `","productId":"` + pid1 + `","currentCharges":` + charges + `}`
}

func setUpdatedAt(t *testing.T, db *store.Store, resourceType, id, at string) {
	t.Helper()
	res, err := db.DB().Exec(`UPDATE resources SET updated_at = ? WHERE resource_type = ? AND id LIKE ?`, at, resourceType, "%"+id)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("pinned %d rows of %s/%s, want 1", n, resourceType, id)
	}
}

// Codex round 1: a live read stores Y under usage-summaries with 10, then a sync
// updates its older row to 90 inside the same second. The older row keeps its
// smaller rowid, so the stale 10 won and overage missed the 1.8x overage.
func TestSameSecondUsageSummaryWrittenLastWins(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions_usage_summaries", "subscriptions", sidA, usageRow(usageY, "50"), usageRow(usageZ, "10"))
	if err := db.UpsertUsageSummaries(json.RawMessage(usageRow(usageY, "10"))); err != nil {
		t.Fatal(err)
	}
	seedAsSync(t, db, "subscriptions_usage_summaries", "subscriptions", sidA, usageRow(usageY, "90"))

	// The same second, whichever one the clock really gave them.
	const second = "2026-11-05T06:00:00Z"
	setUpdatedAt(t, db, "subscriptions_usage_summaries", usageY, second)
	setUpdatedAt(t, db, "usage-summaries", usageY, second)

	var flagged []overageRow
	runNovelJSON(t, path, &flagged, "overage")
	if len(flagged) != 1 || flagged[0].UsageSummaryID != usageY || flagged[0].CurrentCharges != 90 || flagged[0].OverageFactor != 1.8 {
		t.Fatalf("overage = %+v, want Y at 90 flagged 1.8x", flagged)
	}
}

// The other direction of the same tie: the live read is the newest write.
func TestSameSecondLiveReadWrittenLastWins(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions_usage_summaries", "subscriptions", sidA, usageRow(usageY, "10"), usageRow(usageZ, "10"))
	if err := db.UpsertUsageSummaries(json.RawMessage(usageRow(usageY, "50"))); err != nil {
		t.Fatal(err)
	}
	seedAsSync(t, db, "subscriptions_usage_summaries", "subscriptions", sidA, usageRow(usageY, "10"))
	if err := db.UpsertUsageSummaries(json.RawMessage(usageRow(usageY, "90"))); err != nil {
		t.Fatal(err)
	}
	const second = "2026-11-05T06:00:00Z"
	setUpdatedAt(t, db, "subscriptions_usage_summaries", usageY, second)
	setUpdatedAt(t, db, "usage-summaries", usageY, second)

	var flagged []overageRow
	runNovelJSON(t, path, &flagged, "overage")
	if len(flagged) != 1 || flagged[0].CurrentCharges != 90 {
		t.Fatalf("overage = %+v, want the 90 the live read wrote last", flagged)
	}
}

// A store written by an earlier release: its rows' rowids do not follow their
// writes, so their updated_at still decides when the seconds differ.
func TestUsageSummaryRowsFromAnEarlierStoreStillReadByTime(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions_usage_summaries", "subscriptions", sidA, usageRow(usageY, "90"), usageRow(usageZ, "10"))
	if err := db.UpsertUsageSummaries(json.RawMessage(usageRow(usageY, "10"))); err != nil {
		t.Fatal(err)
	}
	// The sync row has the smaller rowid and the later stamp, which is what an
	// update in place left behind before.
	setUpdatedAt(t, db, "subscriptions_usage_summaries", usageY, "2026-11-05T06:00:05Z")
	setUpdatedAt(t, db, "usage-summaries", usageY, "2026-11-05T06:00:01Z")

	var flagged []overageRow
	runNovelJSON(t, path, &flagged, "overage")
	if len(flagged) != 1 || flagged[0].CurrentCharges != 90 {
		t.Fatalf("overage = %+v, want the row stamped later", flagged)
	}
}
