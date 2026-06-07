// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence command — hand-built over the local insights store.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
)

// pp:data-source local
func newNovelQbrCmd(flags *rootFlags) *cobra.Command {
	var since string
	var format string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "qbr",
		Short: "Assemble a client quarterly review in one command",
		Long: strings.TrimSpace(`
Assemble the full client quarterly review from the local store: account risk and
its movement, the phish-prone trend, training completion, repeat/untrained clicker
counts, and the top-risk humans. Composes several cross-entity, cross-time local
computations no single KnowBe4 endpoint emits.

Use --format md for a paste-ready markdown report, or the default JSON for agents.
Run 'knowbe4-cli sync' first.`),
		Example:     "  knowbe4-cli qbr --since 90d --format md",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "json", "md", "markdown":
			default:
				return fmt.Errorf("--format must be json or md; got %q", format)
			}
			db, st, closeFn, err := openInsightsDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			insightsSyncHints(cmd, st, "", flags)
			rep, err := insights.QBR(cmd.Context(), db, since)
			if err != nil {
				return err
			}
			if f := strings.ToLower(strings.TrimSpace(format)); f == "md" || f == "markdown" {
				fmt.Fprint(cmd.OutOrStdout(), renderQBRMarkdown(rep))
				return nil
			}
			return flags.printJSON(cmd, rep)
		},
	}
	cmd.Flags().StringVar(&since, "since", "90d", "Reporting window (e.g. 90d, 6mo, 1y)")
	cmd.Flags().StringVar(&format, "format", "json", "Output format: json or md")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}

func renderQBRMarkdown(rep insights.QBRReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Security Awareness QBR\n\n")
	fmt.Fprintf(&b, "_Generated %s · window %s_\n\n", rep.GeneratedAt, rep.Window)
	fmt.Fprintf(&b, "## Risk posture\n\n")
	fmt.Fprintf(&b, "- Account risk score: **%.1f**", rep.AccountRiskScore)
	if rep.AccountRiskDelta != nil {
		dir := "improved"
		if *rep.AccountRiskDelta > 0 {
			dir = "worsened"
		}
		fmt.Fprintf(&b, " (%s %+.1f over the window)", dir, *rep.AccountRiskDelta)
	}
	fmt.Fprintf(&b, "\n")
	t := rep.PhishProneTrend
	if len(t.Points) > 0 {
		fmt.Fprintf(&b, "- Phish-prone trend: **%.1f%% → %.1f%%** (%+.1f) across %d tests\n", t.FirstPct, t.LastPct, t.DeltaPct, len(t.Points))
	} else {
		fmt.Fprintf(&b, "- Phish-prone trend: no tests in window\n")
	}
	fmt.Fprintf(&b, "- Training completion: **%.1f%%**\n", rep.TrainingCompletion)
	fmt.Fprintf(&b, "- Repeat clickers: **%d** · Untrained clickers: **%d**\n\n", rep.RepeatClickerCount, rep.UntrainedClickers)

	fmt.Fprintf(&b, "## Top-risk users\n\n")
	if len(rep.TopRiskUsers) == 0 {
		fmt.Fprintf(&b, "_No users synced._\n")
		return b.String()
	}
	fmt.Fprintf(&b, "| User | Risk | Phish-prone | Clicked | Reported | Open trainings |\n")
	fmt.Fprintf(&b, "|------|-----:|------------:|--------:|---------:|---------------:|\n")
	for _, u := range rep.TopRiskUsers {
		name := u.Name
		if name == "" {
			name = u.Email
		}
		fmt.Fprintf(&b, "| %s | %.1f | %.1f%% | %d | %d | %d |\n", name, u.CurrentRisk, u.PhishPronePct, u.ClickedPSTs, u.ReportedPSTs, u.OpenTrainings)
	}
	return b.String()
}
