// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"testing"
)

func TestNovelHygieneUnmappedRejectsLocalDataSource(t *testing.T) {
	flags := &rootFlags{asJSON: true, dataSource: "local"}
	cmd := newNovelHygieneUnmappedCmd(flags)
	if err := cmd.Flags().Set("limit", "5"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("hygiene unmapped must reject --data-source local (no local data source)")
	}
}

func TestNovelHygieneUnmappedDryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true, dataSource: "auto"}
	cmd := newNovelHygieneUnmappedCmd(flags)
	if err := cmd.Flags().Set("limit", "5"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("dry-run must exit clean: %v", err)
	}
	if out.Len() == 0 {
		t.Error("dry-run should print its plan")
	}
}
