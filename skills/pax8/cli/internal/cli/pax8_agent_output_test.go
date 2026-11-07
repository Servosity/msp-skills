// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// --agent and --compact keep every field a novel command's rows define, so a
// counter that marks a figure as partial survives when only some rows carry it
// (pax8 handfix: novel-money-shapes; review round 2).
func TestNovelAgentOutputKeepsPartialMarkers(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		subscriptionRow(moneyID(1), "Annual", 1, 120, 100, "USD"),
		subscriptionRow(moneyID(2), "Annual", 1, 80, nil, "USD"),
		subscriptionRow(moneyID(3), "2-Year", 1, 240, 200, "USD"),
		subscriptionRow(moneyID(4), "3-Year", 1, 360, 300, "USD"))
	seedAsSync(t, db, "invoices", "", "",
		invoiceRow(11, "acme", "Paid", 100, "USD"),
		invoiceRow(12, "beta", "Paid", 50, "USD"),
		invoiceRow(13, "gamma", "Paid", 70, "USD"),
		invoiceRow(14, "delta", "Paid", 20, "USD"),
		invoiceRow(15, "delta", "Void", 900, "USD"))
	for _, mode := range [][]string{{"--agent"}, {"--json", "--compact"}} {
		out := runNovelText(t, path, append([]string{"mrr"}, mode...)...)
		if !strings.Contains(out, `"subscriptionsWithoutCost": 1`) {
			t.Errorf("mrr %s hides that the Annual margin is partial:\n%s", mode, out)
		}
		out = runNovelText(t, path, append([]string{"spend"}, mode...)...)
		if !strings.Contains(out, `"notOwedInvoices": 1`) {
			t.Errorf("spend %s hides the invoice it left out:\n%s", mode, out)
		}
	}
}
