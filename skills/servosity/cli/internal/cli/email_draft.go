// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"servosity-msp-pp-cli/internal/store"
)

// emailDraft is one ready-to-paste follow-up email for a single client.
type emailDraft struct {
	CompanyID   int    `json:"company_id"`
	CompanyName string `json:"company_name"`
	Subject     string `json:"subject"`
	Body        string `json:"body"`
	StaleCount  int    `json:"stale_count"`
}

type emailDraftView struct {
	Drafts   []emailDraft `json:"drafts"`
	Count    int          `json:"count"`
	DaysOver int          `json:"days_over"`
	Note     string       `json:"note,omitempty"`
}

// pp:data-source local
func newNovelEmailDraftCmd(flags *rootFlags) *cobra.Command {
	var stale bool
	var days int
	var engine string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "email-draft",
		Short: "Draft follow-up emails for every client with a stale backup",
		Long: `Turn the stale-backup list into ready-to-paste follow-up email bodies, one
per client, filled from the local store: client name, affected hosts, engine,
days stale, and last successful backup.

The fill is mechanical (no AI involved) — pipe the output into your own
rephrasing step if you want a different tone.

Reads the same pp_last_success freshness data as 'stale-backups'. Run
'servosity-cli stale-backups --refresh' first to hydrate it.`,
		Example: `  # Friday sweep: drafts for everything 7+ days stale
  servosity-cli email-draft --stale --days 7

  # Restic engine only, as JSON for scripting
  servosity-cli email-draft --stale --engine restic --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would draft follow-up emails from the local stale-backup slice")
				return nil
			}

			if !stale {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--stale=false has no other mode in v1; only stale-backup drafts are supported"))
			}
			switch engine {
			case "all", "classic", "restic", "dr":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --engine %q: must be one of classic, restic, dr, all", engine))
			}

			if dbPath == "" {
				dbPath = defaultDBPath("servosity-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "companies") {
				hintIfStale(cmd, db, "companies", flags.maxAge)
			}

			if err := ensureLastSuccessTable(cmd.Context(), db); err != nil {
				return err
			}
			now := time.Now()
			entries, err := deriveStaleEntries(cmd.Context(), db, now)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: freshness not hydrated yet — run 'servosity-cli stale-backups --refresh' first")
				view := emailDraftView{Drafts: []emailDraft{}, Count: 0, DaysOver: days, Note: "no per-backup freshness cached; run 'servosity-cli stale-backups --refresh' to hydrate"}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}

			names := loadCompanyNames(db)

			// Group qualifying stale entries per company.
			type group struct {
				name  string
				hosts []staleBackupEntry
			}
			byCompany := map[int]*group{}
			for _, e := range entries {
				if e.DaysStale < days {
					continue
				}
				if !engineMatches(e.Engine, engine) {
					continue
				}
				g := byCompany[e.CompanyID]
				if g == nil {
					g = &group{name: coalesceCompany(names[e.CompanyID])}
					byCompany[e.CompanyID] = g
				}
				g.hosts = append(g.hosts, e)
			}

			drafts := make([]emailDraft, 0, len(byCompany))
			for id, g := range byCompany {
				sort.Slice(g.hosts, func(i, j int) bool { return g.hosts[i].DaysStale > g.hosts[j].DaysStale })
				var b strings.Builder
				fmt.Fprintf(&b, "Hi %s team,\n\n", g.name)
				if len(g.hosts) == 1 {
					b.WriteString("Our monitoring shows one of your protected systems has not completed a successful backup recently:\n\n")
				} else {
					fmt.Fprintf(&b, "Our monitoring shows %d of your protected systems have not completed a successful backup recently:\n\n", len(g.hosts))
				}
				for _, h := range g.hosts {
					host := h.Hostname
					if host == "" {
						host = fmt.Sprintf("backup %s", h.BackupID)
					}
					last := h.LastBackupAt
					if last == "" {
						last = "no successful backup on record"
					}
					fmt.Fprintf(&b, "  - %s (%s engine): %d days since last success (%s)\n", host, h.Engine, h.DaysStale, last)
				}
				b.WriteString("\nWe're investigating from our side; if any of these machines were retired, renamed, or are expected to be offline, let us know so we can update the backup plan.\n\nThanks,\n")
				drafts = append(drafts, emailDraft{
					CompanyID:   id,
					CompanyName: g.name,
					Subject:     fmt.Sprintf("Backup follow-up: %d system(s) need attention", len(g.hosts)),
					Body:        b.String(),
					StaleCount:  len(g.hosts),
				})
			}
			sort.Slice(drafts, func(i, j int) bool { return drafts[i].StaleCount > drafts[j].StaleCount })

			view := emailDraftView{Drafts: drafts, Count: len(drafts), DaysOver: days}
			if len(drafts) == 0 {
				view.Note = fmt.Sprintf("no clients with backups %d+ days stale (engine=%s); lower --days to widen", days, engine)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(drafts) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			for i, d := range drafts {
				if i > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "\n"+strings.Repeat("-", 72)+"\n")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "To:      %s\nSubject: %s\n\n%s", d.CompanyName, d.Subject, d.Body)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&stale, "stale", true, "Draft for stale-backup clients (the only mode in v1)")
	cmd.Flags().IntVar(&days, "days", 7, "Minimum days stale to include a system")
	cmd.Flags().StringVar(&engine, "engine", "all", "Filter by engine: classic, restic, dr, all")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/servosity-cli/data.db)")
	return cmd
}
