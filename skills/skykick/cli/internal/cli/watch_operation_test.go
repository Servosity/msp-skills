// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNovelWatchOperationWiring(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newNovelWatchOperationCmd(flags)
	assertFlags(t, cmd, "timeout", "interval")
	if cmd.Annotations["pp:happy-args"] == "" {
		t.Errorf("watch-operation needs pp:happy-args fixture for verify")
	}
	out := runNovelDry(t, cmd, []string{"1a2b3c4d-0000-0000-0000-000000000000"})
	assertWould(t, out)
}

func TestNovelWatchOperationRequiresID(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelWatchOperationCmd(flags)
	_ = cmd.Flags().Set("timeout", "5") // NFlag>0 so the help-only branch doesn't swallow it
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "operationId") {
		t.Fatalf("missing operationId must be a usage error, got: %v", err)
	}
}

func TestNovelWatchOperationBareInvocationShowsHelp(t *testing.T) {
	flags := &rootFlags{}
	cmd := newNovelWatchOperationCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("bare invocation should show help, got error: %v", err)
	}
	if !strings.Contains(out.String(), "watch-operation") {
		t.Errorf("help output expected, got: %q", out.String())
	}
}
