// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"gradient-pp-cli/internal/ledger"
)

func TestNovelUsageDriftReadsLedger(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRADIENT_LEDGER_DIR", dir)
	if err := ledger.AppendPushes(dir, []ledger.PushRecord{
		{RunID: "r1", ServiceID: "s1", AccountID: "a1", UnitCount: 10, Status: "sent"},
		{RunID: "r2", ServiceID: "s1", AccountID: "a1", UnitCount: 12, Status: "sent"},
	}); err != nil {
		t.Fatal(err)
	}
	flags := &rootFlags{asJSON: true, dataSource: "auto"}
	cmd := newNovelUsageDriftCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("drift: %v (output: %s)", err, out.String())
	}
	var rep ledger.DriftReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if len(rep.Changes) != 1 || rep.Changes[0].Delta != 2 {
		t.Errorf("expected one +2 change, got %+v", rep.Changes)
	}
}

func TestNovelUsageDriftRejectsLiveDataSource(t *testing.T) {
	t.Setenv("GRADIENT_LEDGER_DIR", t.TempDir())
	flags := &rootFlags{asJSON: true, dataSource: "live"}
	cmd := newNovelUsageDriftCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("drift must reject --data-source live (no live equivalent)")
	}
}
