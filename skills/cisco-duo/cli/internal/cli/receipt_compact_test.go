// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// agentFlags is what --agent expands to: JSON plus --compact.
func agentFlags() *rootFlags { return &rootFlags{agent: true, asJSON: true, compact: true} }

// partialFixSteps is a lockout fix whose bypass code was issued before the
// activation step failed: only one of three steps carries a result, so the
// compact frequency rule (keep a key present in 80% of rows) drops it.
func partialFixSteps() []fixStep {
	return []fixStep{
		{Step: "unlock", Status: "skipped", Detail: "status is active; nothing to unlock"},
		{Step: "bypass-code", Status: "done", Result: map[string]any{"codes": []string{"123456789"}, "valid_secs": 3600, "uses": 1}},
		{Step: "reactivate-phone", Status: "failed", Detail: "HTTP 500"},
	}
}

// Codex merge review (P2) twin: the receipt of a partially applied write must
// reach an --agent caller. Under --compact the bypass code (fixStep.Result)
// was stripped from the receipt exactly when a later step failed, so the code
// was issued and nobody received it.
func TestAgentReceiptKeepsStepResults(t *testing.T) {
	t.Run("lockout fix", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)
		view := fixView{UserID: "DUAAAAAAAAAAAAAAAAAA", Username: "jdoe", Applied: true, Steps: partialFixSteps()}
		if err := writeFixView(cmd, agentFlags(), view); err != nil {
			t.Fatalf("writeFixView: %v", err)
		}
		if !strings.Contains(buf.String(), "123456789") {
			t.Fatalf("--agent lockout fix receipt dropped the issued bypass code:\n%s", buf.String())
		}
	})

	t.Run("phone swap", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)
		steps := []fixStep{
			{Step: "create-phone", Status: "done", Result: map[string]string{"phone_id": "DPNEWNEWNEWNEWNEWNEW"}},
			{Step: "attach-phone", Status: "failed", Detail: "HTTP 500"},
			{Step: "send-activation", Status: "planned"},
		}
		view := swapView{UserID: "DUAAAAAAAAAAAAAAAAAA", Username: "jdoe", Number: "+15555550100", Applied: true, Steps: steps}
		if err := writeSwapView(cmd, agentFlags(), view); err != nil {
			t.Fatalf("writeSwapView: %v", err)
		}
		if !strings.Contains(buf.String(), "DPNEWNEWNEWNEWNEWNEW") {
			t.Fatalf("--agent phone swap receipt dropped the created phone_id:\n%s", buf.String())
		}
	})

	t.Run("enrollment chase", func(t *testing.T) {
		var buf bytes.Buffer
		targets := []chaseTarget{
			{UserID: "DU1", Username: "a", Email: "a@example.com", Status: "sent"},
			{UserID: "DU2", Username: "b", Email: "b@example.com", Status: "sent"},
			{UserID: "DU3", Username: "c", Email: "c@example.com", Status: "sent"},
			{UserID: "DU4", Username: "d", Email: "d@example.com", Status: "sent"},
			{UserID: "DU5", Username: "e", Email: "e@example.com", Status: "failed", Error: "duo error 40002: invalid email"},
		}
		if err := printChaseJSON(&buf, map[string]any{"applied": true, "targets": targets}, agentFlags()); err != nil {
			t.Fatalf("printChaseJSON: %v", err)
		}
		if !strings.Contains(buf.String(), "invalid email") {
			t.Fatalf("--agent enrollment chase receipt dropped the failed target's error:\n%s", buf.String())
		}
	})
}
