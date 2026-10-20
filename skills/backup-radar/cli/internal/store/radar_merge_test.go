// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type mergedRow struct {
	DeviceID   *string `json:"device_id"`
	Status     string  `json:"status"`
	LastResult string  `json:"lastResult"`
	History    []struct {
		Date      string `json:"date"`
		SLAStatus string `json:"sla_status"`
	} `json:"history"`
}

func merged(t *testing.T, res, old, inc string) mergedRow {
	t.Helper()
	var got mergedRow
	if err := json.Unmarshal(mergeRadarSnapshot(res, json.RawMessage(old), json.RawMessage(inc)), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestMergeRadarSnapshotV3(t *testing.T) {
	old := `{"device_id":"d1","history":[{"date":"2026-10-12","sla_status":"Out Of Compliance"},{"date":"2026-01-01","sla_status":"In Compliance"}]}`
	got := merged(t, "devices", old, `{"device_id":null,"history":[{"date":"2026-10-13","sla_status":"In Compliance"},{"date":"2026-10-12","sla_status":"At Risk"}]}`)
	if got.DeviceID != nil {
		t.Errorf("a null scalar must replace (device became unmapped)")
	}
	if len(got.History) != 3 || got.History[0].Date != "2026-10-13" || got.History[1].SLAStatus != "At Risk" || got.History[2].Date != "2026-01-01" {
		t.Errorf("history must union by date, newest first, incoming winning: %+v", got.History)
	}
	if got := merged(t, "devices", old, `{"device_id":"d1","history":null}`); len(got.History) != 0 {
		t.Errorf("v3 null history must replace: %+v", got.History)
	}
}

func TestMergeRadarSnapshotV2(t *testing.T) {
	old := `{"status":"Failure","lastResult":"2026-10-13T02:00:00","history":[{"date":"2026-10-12T00:00:00"},{"date":"2026-10-13T00:00:00"}]}`
	// backups list without HistoryDays omits history: keep the stored history.
	if got := merged(t, "backups", old, `{"status":"Failure","lastResult":"2026-10-14T02:00:00"}`); len(got.History) != 2 || got.LastResult != "2026-10-14T02:00:00" {
		t.Errorf("v2 missing history must keep stored history and take new scalars: %+v", got)
	}
	// a stored row without history takes the incoming history as-is.
	got := merged(t, "backups", `{"status":"Failure","lastResult":"2026-10-13T02:00:00"}`, `{"status":"Success","lastResult":"2026-10-14T02:00:00","history":[{"date":"2026-10-14T00:00:00"}]}`)
	if got.Status != "Success" || len(got.History) != 1 {
		t.Errorf("incoming history must be kept when nothing is stored: %+v", got)
	}
	// non-object history entries do not panic
	if out := mergeRadarSnapshot("devices", json.RawMessage(`{"history":[{"date":"2026-10-12"}]}`), json.RawMessage(`{"history":[null,"x",{"date":"2026-10-13"}]}`)); !json.Valid(out) {
		t.Errorf("merge with non-object entries must still produce JSON: %s", out)
	}
}

func TestMergeKeepsLargeIDs(t *testing.T) {
	out := mergeRadarSnapshot("backups", json.RawMessage(`{"backupId":9007199254740993,"history":[]}`), json.RawMessage(`{"backupId":9007199254740993,"history":[{"date":"2026-10-13"}]}`))
	if !strings.Contains(string(out), "9007199254740993") {
		t.Errorf("large integer ids must survive the merge: %s", out)
	}
}

func TestUnionHistoryCapped(t *testing.T) {
	var a, b []any
	for d := 0; d < 400; d++ {
		e := map[string]any{"date": fmt.Sprintf("2026-%03d", d)}
		if d%2 == 0 {
			a = append(a, e)
		} else {
			b = append(b, e)
		}
	}
	got := unionHistory(a, b)
	if len(got) != maxHistoryDays || historyDate(got[0]) != "2026-399" || historyDate(got[len(got)-1]) != "2026-034" {
		t.Errorf("want the newest %d days, got %d from %s to %s", maxHistoryDays, len(got), historyDate(got[0]), historyDate(got[len(got)-1]))
	}
}
