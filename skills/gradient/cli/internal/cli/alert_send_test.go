// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNovelAlertSendRequiredFlags(t *testing.T) {
	flags := &rootFlags{asJSON: true, dataSource: "auto"}
	cmd := newNovelAlertSendCmd(flags)
	if err := cmd.Flags().Set("account", "123"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("alert send without --title/--description must fail")
	}
	if !strings.Contains(err.Error(), "--title") || !strings.Contains(err.Error(), "--description") {
		t.Errorf("error should name the missing flags, got: %v", err)
	}
}

func TestNovelAlertSendCommandShape(t *testing.T) {
	cmd := newNovelAlertSendCmd(&rootFlags{})
	for _, f := range []string{"account", "title", "description", "alert-id", "priority", "status", "due", "service", "wait", "timeout-wait", "interval"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("alert send missing --%s", f)
		}
	}
	if cmd.Annotations["mcp:read-only"] != "false" {
		t.Error("alert send mutates upstream state; mcp:read-only must be false")
	}
}
