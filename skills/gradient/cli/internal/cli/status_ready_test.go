// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"testing"
)

func TestNovelStatusReadyRejectsLocalDataSource(t *testing.T) {
	flags := &rootFlags{asJSON: true, dataSource: "local"}
	cmd := newNovelStatusReadyCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("status ready must reject --data-source local (readiness is live state)")
	}
}

func TestNovelStatusReadyDryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true, dataSource: "auto"}
	cmd := newNovelStatusReadyCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("dry-run must exit clean: %v", err)
	}
	if out.Len() == 0 {
		t.Error("dry-run should print its plan")
	}
}

func TestNovelStatusReadyRejectsPositionals(t *testing.T) {
	flags := &rootFlags{dataSource: "auto"}
	cmd := newNovelStatusReadyCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.RunE(cmd, []string{"bogus"}); err == nil {
		t.Fatal("status ready must reject positional arguments with a usage error")
	}
}
