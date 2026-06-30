// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `report` — render a smart HTML/PDF bill breakdown and optionally post it to
// Slack (delegating to the slack-pp-cli binary). Read-only against AWS; the
// only side effect is writing a file and, with --post-slack, posting to Slack.
//
// pp:client-call
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
)

// slackChannelEnvVar names the env var that supplies the default Slack target
// when --slack-channel is omitted. Keeping the default out of source (no
// hardcoded personal DM/channel) keeps the CLI shareable: set it once to your
// own DM or channel ID and `report --post-slack` posts there by default.
const slackChannelEnvVar = "AWS_BILLING_SLACK_CHANNEL"

// resolveSlackChannel picks the Slack target: explicit flag wins, else the env
// var. Returns "" when neither is set so the caller can emit a clear error
// instead of silently posting to the wrong place.
func resolveSlackChannel(flagValue string) string {
	if strings.TrimSpace(flagValue) != "" {
		return flagValue
	}
	return strings.TrimSpace(os.Getenv(slackChannelEnvVar))
}

type reportData struct {
	Period      string
	GeneratedAt string
	Source      string
	TotalUSD    float64
	PriorUSD    float64
	DeltaPct    float64
	ByService   []costGroup
	ByAccount   []consolidatedAccount
	Waste       []awsx.InventoryResource
	WasteTotal  float64
}

func newReportCmd(flags *rootFlags) *cobra.Command {
	var period, format, outPath, dbPath, profile, region, slackChannel string
	var postSlack bool

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render an HTML/PDF bill breakdown and optionally post it to Slack",
		Long: `Build a smart, sectioned report of the AWS bill — total with month-over-month
delta, top services, per-account rollup, and a dollar-ranked waste list — as
HTML and/or PDF. With --post-slack it posts a summary plus the HTML to Slack
(delegating to the slack-pp-cli binary). The CLI writes files and posts only
when asked; it never mutates AWS.`,
		Example: `  # Write an HTML report for last month
  aws-billing-cli report --period last-month

  # HTML + PDF, posted to your Slack DM
  aws-billing-cli report --period last-month --format both --post-slack`,
		// No mcp:read-only: this command always writes a report file to disk and,
		// with --post-slack, posts to Slack — both are side effects an MCP host
		// should be able to prompt on. (Read-only against AWS, but not the FS/Slack.)
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if cliutil.IsVerifyEnv() {
				fmt.Fprintln(cmd.OutOrStdout(), "verify: would render the AWS bill report and (with --post-slack) post it")
				return nil
			}
			pr, err := resolvePeriod(period)
			if err != nil {
				return usageErr(err)
			}
			// Validate the Slack target before doing AWS work, so a missing
			// channel fails fast rather than after a slow gather.
			if postSlack && resolveSlackChannel(slackChannel) == "" {
				return usageErr(fmt.Errorf("no Slack target: pass --slack-channel <id> or set %s (e.g. export %s=<your-dm-or-channel-id>)", slackChannelEnvVar, slackChannelEnvVar))
			}
			data, err := gatherReport(cmd.Context(), flags, awsReadOptsFromFlags(flags, dbPath, profile, region), pr)
			if err != nil {
				return err
			}
			htmlDoc := renderReportHTML(data)

			stamp := pr.Start
			if outPath == "" {
				outPath = fmt.Sprintf("aws-bill-%s.html", stamp)
			}
			w := cmd.OutOrStdout()
			var htmlPath, pdfPath string

			wantHTML := format == "" || format == "html" || format == "both"
			wantPDF := format == "pdf" || format == "both"

			if wantHTML {
				if err := os.WriteFile(outPath, []byte(htmlDoc), 0o644); err != nil {
					return fmt.Errorf("writing HTML report: %w", err)
				}
				htmlPath = outPath
				fmt.Fprintf(w, "wrote %s\n", htmlPath)
			}
			if wantPDF {
				pdfPath = strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".pdf"
				if err := htmlToPDF(htmlDoc, pdfPath); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: PDF generation skipped: %v\n", err)
					pdfPath = ""
				} else {
					fmt.Fprintf(w, "wrote %s\n", pdfPath)
				}
			}

			summary := renderReportSummary(data)
			if !postSlack {
				fmt.Fprintln(w, "\n"+summary)
				fmt.Fprintln(w, "(re-run with --post-slack to post this to Slack)")
				return nil
			}

			channel := resolveSlackChannel(slackChannel)
			if channel == "" {
				return usageErr(fmt.Errorf("no Slack target: pass --slack-channel <id> or set %s (e.g. export %s=<your-dm-or-channel-id>)", slackChannelEnvVar, slackChannelEnvVar))
			}
			// Side-effect: posting to Slack. Short-circuit under verify.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintf(w, "verify: would post AWS bill report to Slack channel %s\n", channel)
				return nil
			}
			if err := postReportToSlack(cmd.Context(), channel, summary, htmlDoc, stamp); err != nil {
				return fmt.Errorf("posting to Slack: %w", err)
			}
			fmt.Fprintf(w, "posted report to Slack channel %s\n", channel)
			return nil
		},
	}
	cmd.Flags().StringVar(&period, "period", "last-month", "Period to report: this-month, last-month, last-3-months, ytd, or YYYY-MM-DD:YYYY-MM-DD")
	cmd.Flags().StringVar(&format, "format", "html", "Output format: html, pdf, both")
	cmd.Flags().StringVar(&outPath, "out", "", "Output HTML path (default: aws-bill-<start>.html)")
	cmd.Flags().BoolVar(&postSlack, "post-slack", false, "Post the report to Slack (opt-in side effect; delegates to slack-pp-cli)")
	cmd.Flags().StringVar(&slackChannel, "slack-channel", "", "Slack channel/DM ID to post to (default: $AWS_BILLING_SLACK_CHANNEL)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live call")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live call")
	return cmd
}

func gatherReport(ctx context.Context, flags *rootFlags, o awsReadOpts, pr periodRange) (reportData, error) {
	d := reportData{Period: pr.Label, GeneratedAt: time.Now().UTC().Format("2006-01-02 15:04 UTC")}
	cur, source, err := canonicalCostLines(ctx, o, pr)
	if err != nil {
		return d, err
	}
	d.Source = source
	prev := previousPeriod(pr)
	priorLines, _, _ := canonicalCostLines(ctx, o, prev)

	d.ByService, _ = groupCostLines(cur, "service")
	curByAcct, curTotal := sumByAccount(cur)
	priorByAcct, priorTotal := sumByAccount(priorLines)
	d.TotalUSD = round2f(curTotal)
	d.PriorUSD = round2f(priorTotal)
	d.DeltaPct = pctDelta(priorTotal, curTotal)

	nameByAcct := map[string]string{}
	for _, l := range cur {
		if l.AccountName != "" {
			nameByAcct[l.AccountID] = l.AccountName
		}
	}
	for a, amt := range curByAcct {
		d.ByAccount = append(d.ByAccount, consolidatedAccount{
			AccountID: a, Name: nameByAcct[a], AmountUSD: round2f(amt),
			PriorUSD: round2f(priorByAcct[a]), DeltaUSD: round2f(amt - priorByAcct[a]), DeltaPct: pctDelta(priorByAcct[a], amt),
		})
	}
	sort.Slice(d.ByAccount, func(i, j int) bool { return d.ByAccount[i].AmountUSD > d.ByAccount[j].AmountUSD })

	// Waste (best-effort; don't fail the report if inventory is denied).
	if waste, _, werr := ensureInventory(ctx, o, "", "", true); werr == nil {
		sort.Slice(waste, func(i, j int) bool { return waste[i].MonthlyWasteUSD > waste[j].MonthlyWasteUSD })
		d.Waste = waste
		for _, r := range waste {
			d.WasteTotal += r.MonthlyWasteUSD
		}
		d.WasteTotal = round2f(d.WasteTotal)
	}
	return d, nil
}

func renderReportSummary(d reportData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*AWS bill — %s*: $%.2f (%+.1f%% vs prior)\n", d.Period, d.TotalUSD, d.DeltaPct)
	if len(d.ByService) > 0 {
		b.WriteString("Top services:\n")
		for i, g := range d.ByService {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "  • %s — $%.2f (%.0f%%)\n", g.Key, g.AmountUSD, g.Pct)
		}
	}
	if d.WasteTotal > 0 {
		fmt.Fprintf(&b, "Estimated waste: ~$%.2f/mo across %d resources\n", d.WasteTotal, len(d.Waste))
	}
	return b.String()
}

func renderReportHTML(d reportData) string {
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>AWS Bill</title>`)
	b.WriteString(`<style>body{font-family:-apple-system,Segoe UI,Roboto,sans-serif;margin:2rem;color:#16191f;background:#fff}` +
		`h1{font-size:1.6rem}h2{margin-top:2rem;border-bottom:2px solid #ff9900;padding-bottom:.25rem}` +
		`table{border-collapse:collapse;width:100%;margin-top:.5rem}th,td{text-align:left;padding:.4rem .6rem;border-bottom:1px solid #eee}` +
		`td.n{text-align:right;font-variant-numeric:tabular-nums}.up{color:#d13212}.down{color:#1d8102}.muted{color:#687078;font-size:.85rem}` +
		`.total{font-weight:700}</style></head><body>`)
	fmt.Fprintf(&b, `<h1>AWS Bill — %s</h1>`, esc(d.Period))
	fmt.Fprintf(&b, `<p class="muted">Generated %s · source: %s</p>`, esc(d.GeneratedAt), esc(d.Source))
	deltaClass := "down"
	if d.DeltaPct > 0 {
		deltaClass = "up"
	}
	fmt.Fprintf(&b, `<p style="font-size:1.3rem">Total: <strong>$%.2f</strong> <span class="%s">(%+.1f%% vs prior $%.2f)</span></p>`,
		d.TotalUSD, deltaClass, d.DeltaPct, d.PriorUSD)

	if len(d.ByAccount) > 0 {
		b.WriteString(`<h2>By account</h2><table><tr><th>Account</th><th class="n">This</th><th class="n">Prior</th><th class="n">Δ%</th></tr>`)
		for _, a := range d.ByAccount {
			label := a.AccountID
			if a.Name != "" {
				label = a.Name + " (" + a.AccountID + ")"
			}
			cls := "down"
			if a.DeltaPct > 0 {
				cls = "up"
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td class="n">$%.2f</td><td class="n">$%.2f</td><td class="n %s">%+.1f%%</td></tr>`,
				esc(label), a.AmountUSD, a.PriorUSD, cls, a.DeltaPct)
		}
		b.WriteString(`</table>`)
	}

	b.WriteString(`<h2>By service</h2><table><tr><th>Service</th><th class="n">Amount</th><th class="n">%</th></tr>`)
	for _, g := range d.ByService {
		fmt.Fprintf(&b, `<tr><td>%s</td><td class="n">$%.2f</td><td class="n">%.1f%%</td></tr>`, esc(g.Key), g.AmountUSD, g.Pct)
	}
	fmt.Fprintf(&b, `<tr class="total"><td>TOTAL</td><td class="n">$%.2f</td><td class="n"></td></tr></table>`, d.TotalUSD)

	if len(d.Waste) > 0 {
		fmt.Fprintf(&b, `<h2>Waste — ~$%.2f/mo</h2><table><tr><th>Type</th><th>Resource</th><th>Region</th><th class="n">$/mo</th><th>Reason</th></tr>`, d.WasteTotal)
		for i, r := range d.Waste {
			if i >= 25 {
				break
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td class="n">$%.2f</td><td>%s</td></tr>`,
				esc(r.ResourceType), esc(r.ResourceID), esc(r.Region), r.MonthlyWasteUSD, esc(r.WasteReason))
		}
		b.WriteString(`</table>`)
	}
	b.WriteString(`<p class="muted">Generated by AWS Billing Intelligence (aws-billing-cli). Read-only; no resources were modified.</p>`)
	b.WriteString(`</body></html>`)
	return b.String()
}

// htmlToPDF renders HTML to PDF via headless Chrome. Returns an error (caller
// warns and continues) if Chrome isn't found.
func htmlToPDF(htmlDoc, pdfPath string) error {
	chrome := findChrome()
	if chrome == "" {
		return fmt.Errorf("Chrome not found (install Google Chrome for PDF output, or use --format html)")
	}
	tmp, err := os.CreateTemp("", "aws-bill-*.html")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(htmlDoc); err != nil {
		return err
	}
	tmp.Close()
	abs, _ := filepath.Abs(pdfPath)
	cmd := exec.Command(chrome, "--headless", "--disable-gpu", "--no-pdf-header-footer",
		"--print-to-pdf="+abs, "file://"+tmp.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("chrome: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func findChrome() string {
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// postReportToSlack delegates to the slack-pp-cli binary: a summary message
// plus the HTML uploaded as a file. Errors if slack-pp-cli isn't installed.
func postReportToSlack(ctx context.Context, channel, summary, htmlDoc, stamp string) error {
	slack := findSlackCLI()
	if slack == "" {
		return fmt.Errorf("slack-pp-cli not found on PATH or in ~/.local/bin (install it to post to Slack)")
	}
	// 1. Summary message.
	msg := exec.CommandContext(ctx, slack, "messages", "post_message", "--channel", channel, "--text", summary)
	if out, err := msg.CombinedOutput(); err != nil {
		return fmt.Errorf("post_message: %v: %s", err, strings.TrimSpace(string(out)))
	}
	// 2. Upload the HTML report via stdin JSON (avoids ARG_MAX on large docs).
	payload, _ := json.Marshal(map[string]string{
		"channels":        channel,
		"content":         htmlDoc,
		"filename":        "aws-bill-" + stamp + ".html",
		"title":           "AWS Bill " + stamp,
		"initial_comment": "Full HTML breakdown attached.",
	})
	up := exec.CommandContext(ctx, slack, "files", "upload", "--stdin")
	up.Stdin = bytes.NewReader(payload)
	if out, err := up.CombinedOutput(); err != nil {
		// Non-fatal: the summary message already landed.
		return fmt.Errorf("file upload (summary posted ok): %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func findSlackCLI() string {
	if p, err := exec.LookPath("slack-pp-cli"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	cand := filepath.Join(home, ".local", "bin", "slack-pp-cli")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	return ""
}
