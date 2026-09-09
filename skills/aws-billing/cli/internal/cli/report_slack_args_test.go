// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"reflect"
	"strings"
	"testing"
)

// TestSlackMessageArgsJoinValues guards the hand-fix slack-delegation-argv-joined:
// the Slack channel comes from an MCP-settable flag, so a value beginning with
// "--" must stay contained inside its own --channel= element.
func TestSlackMessageArgsJoinValues(t *testing.T) {
	got := slackMessageArgs("--token=x", "hello --deliver=y")
	want := []string{"messages", "post_message", "--channel=--token=x", "--text=hello --deliver=y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("slackMessageArgs = %#v, want %#v", got, want)
	}
	for _, tok := range got[2:] {
		if !strings.Contains(tok, "=") {
			t.Fatalf("value emitted as its own argv element: %q", tok)
		}
	}
}
