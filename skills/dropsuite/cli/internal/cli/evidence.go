// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: one organization's restore-readiness evidence.
// pp:data-source live

package cli

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/cliutil"
)

type evidenceKind struct {
	Kind         string `json:"kind"`
	Objects      int    `json:"objects"`
	Graded       int    `json:"graded"`
	OK           int    `json:"ok"`
	Warn         int    `json:"warn"`
	Crit         int    `json:"crit"`
	Deactivated  int    `json:"deactivated"`
	OldestBackup string `json:"oldest_last_backup"`
	NewestBackup string `json:"newest_last_backup"`
	TotalItems   int64  `json:"total_items"`
	TotalStorage int64  `json:"total_storage"`
	// UnknownCounts counts graded objects whose item or storage count the
	// API did not return; the totals leave them out.
	UnknownCounts int    `json:"objects_with_unknown_counts"`
	ReadError     string `json:"read_error,omitempty"`
}

type evidenceRetention struct {
	Name   string `json:"name"`
	Type   string `json:"retention_type"`
	Period string `json:"period"`
}

type evidenceFreshness struct {
	Day     string `json:"day"`
	RunID   string `json:"run_id"`
	Objects int    `json:"objects"`
	Unknown int    `json:"unreadable_workloads"`
	// NotChecked lists workloads that snapshot neither read nor failed to
	// read (health ran with --kinds).
	NotChecked []string `json:"workloads_not_checked"`
	Complete   bool     `json:"complete"`
	OKPct      *float64 `json:"ok_pct"`
}

type evidencePack struct {
	GeneratedAt   string              `json:"generated_at"`
	Window        string              `json:"window"`
	Disclaimer    string              `json:"disclaimer"`
	Organization  map[string]any      `json:"organization"`
	Coverage      *coverageOrg        `json:"coverage"`
	Workloads     []evidenceKind      `json:"workloads"`
	Retention     []evidenceRetention `json:"retention_policies"`
	Freshness     []evidenceFreshness `json:"freshness_history"`
	RestoreTests  []restoreTest       `json:"restore_tests"`
	SSOLink       string              `json:"sso_link,omitempty"`
	FetchFailures []fetchFailure      `json:"fetch_failures"`
}

const evidenceDisclaimer = "This pack reports what the Dropsuite API and this CLI's local records show. It does not perform or verify a restore; restore tests listed were recorded by a technician with 'restore-test record'."

// summarizeKinds folds graded objects into per-workload evidence rows.
func summarizeKinds(objs []objectRec, fails []kindFailure, kinds []string, now time.Time, staleAfter, criticalAfter time.Duration) []evidenceKind {
	byKind := map[string]*evidenceKind{}
	for _, k := range kinds {
		byKind[k] = &evidenceKind{Kind: k}
	}
	for _, f := range fails {
		if ek, ok := byKind[f.Kind]; ok {
			ek.ReadError = errText(f.Err)
		}
	}
	oldest := map[string]time.Time{}
	newest := map[string]time.Time{}
	for _, o := range objs {
		ek, ok := byKind[o.Kind]
		if !ok {
			continue
		}
		ek.Objects++
		grade, _ := gradeObject(o, now, staleAfter, criticalAfter)
		switch grade {
		case "skip":
			ek.Deactivated++
			continue
		case "ok":
			ek.OK++
		case "warn":
			ek.Warn++
		case "crit":
			ek.Crit++
		}
		ek.Graded++
		if o.Items == nil || o.Storage == nil {
			ek.UnknownCounts++
		}
		if o.Items != nil {
			ek.TotalItems += *o.Items
		}
		if o.Storage != nil {
			ek.TotalStorage += *o.Storage
		}
		if !o.LastBackup.IsZero() {
			if oldest[o.Kind].IsZero() || o.LastBackup.Before(oldest[o.Kind]) {
				oldest[o.Kind] = o.LastBackup
			}
			if o.LastBackup.After(newest[o.Kind]) {
				newest[o.Kind] = o.LastBackup
			}
		}
	}
	out := make([]evidenceKind, 0, len(kinds))
	for _, k := range kinds {
		ek := byKind[k]
		ek.OldestBackup = formatTime(oldest[k])
		ek.NewestBackup = formatTime(newest[k])
		out = append(out, *ek)
	}
	return out
}

// freshnessByDay keeps the last snapshot of each day and its ok share.
func freshnessByDay(runs []snapshotRun, rowsByRun map[string][]snapshotRow) []evidenceFreshness {
	lastOfDay := map[string]snapshotRun{}
	for _, r := range runs {
		day := r.TakenAt.UTC().Format("2006-01-02")
		if cur, ok := lastOfDay[day]; !ok || r.TakenAt.After(cur.TakenAt) {
			lastOfDay[day] = r
		}
	}
	days := make([]string, 0, len(lastOfDay))
	for d := range lastOfDay {
		days = append(days, d)
	}
	sort.Strings(days)
	out := make([]evidenceFreshness, 0, len(days))
	for _, d := range days {
		run := lastOfDay[d]
		objects, ok, unknown := 0, 0, 0
		covered := map[string]bool{}
		for _, row := range rowsByRun[run.RunID] {
			if row.Object == scopeMarker {
				covered[row.Kind] = true
				continue
			}
			if row.Object == "*" {
				covered[row.Kind] = true
				unknown++
				continue
			}
			objects++
			if row.Grade == "ok" {
				ok++
			}
		}
		// A day with an unreadable workload gets no success percentage: the
		// share of ok objects says nothing about what could not be read.
		notChecked := make([]string, 0)
		for _, k := range allHealthKinds {
			if !covered[k] {
				notChecked = append(notChecked, k)
			}
		}
		f := evidenceFreshness{Day: d, RunID: run.RunID, Objects: objects, Unknown: unknown, NotChecked: notChecked,
			Complete: unknown == 0 && len(notChecked) == 0}
		if objects > 0 && f.Complete {
			v := float64(int64(float64(ok)/float64(objects)*1000+0.5)) / 10
			f.OKPct = &v
		}
		out = append(out, f)
	}
	return out
}

func newNovelEvidenceCmd(flags *rootFlags) *cobra.Command {
	var windowRaw, staleAfterRaw, criticalAfterRaw, dbPath string
	var markdown, withSSOLink bool
	var maxPages int
	var sweepTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "evidence",
		Short: "Produce one client's dated proof of protection: coverage, last successful backups, item counts, retention policies",
		Long: `Use this command to produce one org's proof-of-protection pack (coverage, last successful backups, item counts, retention policies, recorded restore tests, freshness history). Do NOT use this command to record a restore test you just performed; use 'restore-test record' instead.

Requires --org. It reads the organization's subscription detail, retention
policies, tenant coverage and every protected workload live, then adds the
daily freshness history from local health snapshots and the restore tests
recorded in the local ledger within --window. The pack never claims a restore
was performed by this CLI. --with-sso-link adds a one-time 30-minute portal
sign-in link for performing a restore test (it invalidates the organization's
previous link; treat it as a secret).`,
		Example: strings.Trim(`
  dropsuite-cli evidence --org 26561-14 --window 90d --markdown
  dropsuite-cli evidence --org owner@example.com --json --select coverage,workloads`, "\n"),
		Annotations: map[string]string{"mcp:read-only-unless-flags": "with-sso-link", "pp:data-source": "live", "pp:happy-args": "--org=example-value"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 && orgSelector == "" {
				return cmd.Help()
			}
			if err := rejectArgs(args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "evidence")
			}
			if markdown && (flags.asJSON || flags.agent || flags.csv) {
				return usageErr(fmt.Errorf("--markdown cannot be combined with --json, --agent or --csv; pick one output format"))
			}
			if strings.TrimSpace(orgSelector) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--org is required (organization id or owner email)"))
			}
			window, err := parseDurationFlag("window", windowRaw, "90d")
			if err != nil {
				return err
			}
			staleAfter, err := parseDurationFlag("stale-after", staleAfterRaw, "48h")
			if err != nil {
				return err
			}
			criticalAfter, err := parseDurationFlag("critical-after", criticalAfterRaw, "7d")
			if err != nil {
				return err
			}
			if criticalAfter < staleAfter {
				return usageErr(fmt.Errorf("--critical-after (%s) must not be shorter than --stale-after (%s)", criticalAfter, staleAfter))
			}
			ctx, cancel := sweepCtx(cmd, sweepTimeout)
			defer cancel()
			admin, err := adminClient(flags)
			if err != nil {
				return err
			}
			orgs, err := fetchOrgs(ctx, admin)
			if err != nil {
				return err
			}
			org, err := matchOrg(orgs, orgSelector)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			pack := evidencePack{
				GeneratedAt:   now.Format(time.RFC3339),
				Window:        fmtDuration(window),
				Disclaimer:    evidenceDisclaimer,
				Organization:  map[string]any{"id": org.ID, "email": org.Email, "external_id": org.ExternalID, "seats_used": org.SeatsUsed, "seats_available": org.SeatsAvailable},
				Workloads:     make([]evidenceKind, 0),
				Retention:     make([]evidenceRetention, 0),
				Freshness:     make([]evidenceFreshness, 0),
				RestoreTests:  make([]restoreTest, 0),
				FetchFailures: make([]fetchFailure, 0),
			}
			fail := func(scope string, err error) {
				pack.FetchFailures = append(pack.FetchFailures, fetchFailure{OrgID: org.ID, OrgEmail: org.Email, Scope: scope, Error: errText(err)})
			}
			if data, err := admin.Get(ctx, "/users/"+url.PathEscape(org.ID), nil); err != nil {
				fail("organization detail", err)
			} else if detail, err := singleObject(data); err != nil {
				fail("organization detail", err)
			} else {
				items := []map[string]any{detail}
				for _, k := range []string{"organization_name", "plan_id", "auto_license", "active_seats", "required_seats", "deactivated_seats", "paid_shared_mailboxes", "free_shared_mailboxes", "archive", "customer_deactivated", "flg_suspended"} {
					if v, ok := items[0][k]; ok {
						pack.Organization[k] = v
					}
				}
			}
			oc, ocErr := orgClient(flags, org)
			if ocErr != nil {
				fail("organization token", ocErr)
				oc = nil
			}
			if oc != nil {
				items, err := fetchList(ctx, oc, "/users/"+url.PathEscape(org.ID)+"/retention_policies", nil, pagedEnvelope, maxPages)
				if err != nil {
					fail("retention policies", err)
				}
				for _, m := range items {
					pack.Retention = append(pack.Retention, evidenceRetention{
						Name: str(m, "name"), Type: str(m, "retention_type"),
						Period: strings.TrimSpace(str(m, "period_number") + " " + str(m, "period_unit")),
					})
				}
			}
			cov := orgCoverage(ctx, admin, oc, org, false, false, maxPages)
			pack.Coverage = &cov.org
			pack.FetchFailures = append(pack.FetchFailures, cov.fails...)
			if oc != nil {
				objs, fails := collectOrgObjects(ctx, oc, allHealthKinds, maxPages)
				pack.Workloads = summarizeKinds(objs, fails, allHealthKinds, now, staleAfter, criticalAfter)
				for _, f := range fails {
					fail(f.Kind, f.Err)
				}
				if withSSOLink {
					if cliutil.IsAnyHarness() {
						fail("sso link", fmt.Errorf("skipped under a Printing Press harness"))
					} else if data, err := oc.GetMutating(ctx, "/users/generate_redirect_link", nil); err != nil {
						fail("sso link", err)
					} else if link, err := ssoLinkFrom(data); err != nil {
						fail("sso link", err)
					} else {
						pack.SSOLink = link
					}
				}
			}
			if db, err := openFleetDBReadOnly(ctx, dbPath); err != nil {
				fail("local store", err)
			} else if db != nil {
				defer db.Close()
				since := now.Add(-window)
				if runs, err := listSnapshotRuns(ctx, db.DB()); err != nil {
					fail("local health snapshots", err)
				} else {
					inWindow := make([]snapshotRun, 0)
					rowsByRun := map[string][]snapshotRow{}
					for _, r := range runs {
						if r.TakenAt.Before(since) {
							continue
						}
						rows, err := loadSnapshotRows(ctx, db.DB(), r.RunID, org.ID)
						if err != nil {
							fail("local health snapshot "+r.RunID, err)
							continue
						}
						if len(rows) == 0 {
							continue
						}
						inWindow = append(inWindow, r)
						rowsByRun[r.RunID] = rows
					}
					pack.Freshness = freshnessByDay(inWindow, rowsByRun)
				}
				if tests, err := listRestoreTests(ctx, db.DB(), since); err != nil {
					fail("local restore-test ledger", err)
				} else {
					all := knownFromOrgs(orgs)
					if hist, err := knownOrgs(ctx, db.DB()); err != nil {
						fail("local organization history", err)
					} else {
						all = mergeKnown(all, hist)
					}
					target := knownFromOrgs([]dsOrg{org})[0]
					for _, o := range all {
						if o.ID == org.ID {
							target = o
						}
					}
					pack.RestoreTests = testsFor(all, target, tests)
				}
			}
			if len(pack.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d read(s) failed; the pack marks those sections incomplete\n", len(pack.FetchFailures))
			}
			if markdown {
				return renderEvidenceMarkdown(cmd.OutOrStdout(), pack)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), pack, flags)
			}
			return renderEvidenceMarkdown(cmd.OutOrStdout(), pack)
		},
	}
	cmd.Flags().StringVar(&windowRaw, "window", "90d", "History window for freshness and restore tests (e.g. 30d, 90d, 365d)")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "Render the pack as Markdown for an audit or insurance file")
	cmd.Flags().BoolVar(&withSSOLink, "with-sso-link", false, "Add a one-time 30-minute portal sign-in link (invalidates the organization's previous link)")
	cmd.Flags().StringVar(&staleAfterRaw, "stale-after", "48h", "Grade objects warn when the last backup is older than this")
	cmd.Flags().StringVar(&criticalAfterRaw, "critical-after", "7d", "Grade objects crit when the last backup is older than this")
	cmd.Flags().IntVar(&maxPages, "max-pages", 40, "Maximum pages read per list")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's data.db)")
	cmd.Flags().DurationVar(&sweepTimeout, "sweep-timeout", defaultSweepTimeout, "Budget for the whole run across all organizations (each request still uses --timeout)")
	return cmd
}

func renderEvidenceMarkdown(w io.Writer, p evidencePack) error {
	var b strings.Builder
	name := fmt.Sprint(p.Organization["organization_name"])
	if name == "" || name == "<nil>" {
		name = fmt.Sprint(p.Organization["email"])
	}
	fmt.Fprintf(&b, "# Backup evidence: %s\n\n", name)
	fmt.Fprintf(&b, "Generated %s. Window %s.\n\n", p.GeneratedAt, p.Window)
	fmt.Fprintf(&b, "> %s\n\n", p.Disclaimer)
	fmt.Fprintf(&b, "## Organization\n\n")
	keys := make([]string, 0, len(p.Organization))
	for k := range p.Organization {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "- %s: %v\n", k, p.Organization[k])
	}
	fmt.Fprintf(&b, "\n## Coverage\n\n")
	if c := p.Coverage; c != nil {
		pct := "n/a"
		if c.CoveragePct != nil {
			pct = fmt.Sprintf("%.1f%%", *c.CoveragePct)
		}
		fmt.Fprintf(&b, "Status %s. %d tenant(s); %d mailboxes protected, %d available but not protected, %d excluded. Coverage %s.\n", c.Status, c.Tenants, c.Active, c.Available, c.Excluded, pct)
		if c.Note != "" {
			fmt.Fprintf(&b, "\nNote: %s\n", c.Note)
		}
	}
	fmt.Fprintf(&b, "\n## Protected workloads\n\n")
	fmt.Fprintf(&b, "| Workload | Objects | OK | Warn | Crit | Deactivated | Oldest last backup | Newest last backup | Items | Storage |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|\n")
	for _, k := range p.Workloads {
		label := k.Kind
		if k.ReadError != "" {
			label += " (read failed)"
		}
		items, storage := strconv.FormatInt(k.TotalItems, 10), strconv.FormatInt(k.TotalStorage, 10)
		if k.UnknownCounts > 0 {
			// A partial sum is labelled, never shown as the total.
			suffix := fmt.Sprintf(" + %d unknown", k.UnknownCounts)
			items, storage = "at least "+items+suffix, "at least "+storage+suffix
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %s | %s | %s | %s |\n", label, k.Objects, k.OK, k.Warn, k.Crit, k.Deactivated, k.OldestBackup, k.NewestBackup, items, storage)
	}
	fmt.Fprintf(&b, "\n## Retention policies\n\n")
	if len(p.Retention) == 0 {
		fmt.Fprintf(&b, "None returned.\n")
	}
	for _, r := range p.Retention {
		fmt.Fprintf(&b, "- %s (%s): %s\n", r.Name, r.Type, r.Period)
	}
	fmt.Fprintf(&b, "\n## Freshness history (local health snapshots)\n\n")
	if len(p.Freshness) == 0 {
		fmt.Fprintf(&b, "None recorded in the window. Run 'health' daily to build this history.\n")
	}
	for _, f := range p.Freshness {
		if !f.Complete {
			notChecked := "none"
			if len(f.NotChecked) > 0 {
				notChecked = strings.Join(f.NotChecked, ", ")
			}
			fmt.Fprintf(&b, "- %s: incomplete (%d workload read(s) failed; not checked: %s); %d objects read\n",
				f.Day, f.Unknown, notChecked, f.Objects)
			continue
		}
		pct := "n/a"
		if f.OKPct != nil {
			pct = fmt.Sprintf("%.1f%%", *f.OKPct)
		}
		fmt.Fprintf(&b, "- %s: %s of %d objects graded ok\n", f.Day, pct, f.Objects)
	}
	fmt.Fprintf(&b, "\n## Restore tests (recorded by technicians)\n\n")
	if len(p.RestoreTests) == 0 {
		fmt.Fprintf(&b, "None recorded in the window.\n")
	}
	for _, t := range p.RestoreTests {
		fmt.Fprintf(&b, "- %s: %s %s, result %s", t.TestedAt, t.Kind, t.Object, t.Result)
		if t.Technician != "" {
			fmt.Fprintf(&b, ", by %s", t.Technician)
		}
		if t.Note != "" {
			fmt.Fprintf(&b, ". %s", t.Note)
		}
		fmt.Fprintf(&b, "\n")
	}
	if p.SSOLink != "" {
		fmt.Fprintf(&b, "\n## Portal sign-in for a restore test\n\nOne-time link, valid 30 minutes: %s\n", p.SSOLink)
	}
	if len(p.FetchFailures) > 0 {
		fmt.Fprintf(&b, "\n## Incomplete sections\n\n")
		for _, f := range p.FetchFailures {
			fmt.Fprintf(&b, "- %s: %s\n", f.Scope, f.Error)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
