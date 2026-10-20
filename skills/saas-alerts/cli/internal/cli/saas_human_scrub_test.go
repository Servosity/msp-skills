// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "human-renderers-scrub-terminal" entry.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSaasHumanRenderersScrubTerminal: a tenant-controlled name carrying an
// OSC or CSI sequence reaches the human renderer as plain text.
func TestSaasHumanRenderersScrubTerminal(t *testing.T) {
	evil := "evil\x1b]52;c;cHduZWQ=\x07\x1b[2Jname"
	var b bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&b)
	r := saasInvestigateResult{Email: evil, WindowStart: "a", WindowEnd: "b", Note: evil}
	if err := saasRenderInvestigateHuman(cmd, r); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(b.String(), "\x1b\x07") || !strings.Contains(b.String(), "evil]52;c;cHduZWQ=[2Jname") {
		t.Fatalf("human output carries terminal controls: %q", b.String())
	}
}
