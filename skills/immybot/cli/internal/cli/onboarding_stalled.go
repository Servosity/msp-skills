// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: stalled onboarding queue.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type stalledOnboard struct {
	ComputerID   string  `json:"computer_id"`
	ComputerName string  `json:"computer_name"`
	Tenant       string  `json:"tenant"`
	Serial       string  `json:"serial,omitempty"`
	Status       string  `json:"onboarding_status,omitempty"`
	Online       bool    `json:"online"`
	Attempted    bool    `json:"onboarding_attempted"`
	Failed       bool    `json:"onboarding_failed"`
	Outcome      string  `json:"outcome"`
	AgeDays      float64 `json:"age_days"`
	AgeBucket    string  `json:"age_bucket"`
	UpdatedDate  string  `json:"updated_date,omitempty"`
	SessionID    string  `json:"onboarding_session_id,omitempty"`
}

type onboardingStalledView struct {
	OlderThan      string           `json:"older_than,omitempty"`
	ScannedQueued  int              `json:"scanned_queued"`
	StalledCount   int              `json:"stalled_count"`
	NeverAttempted int              `json:"never_attempted"`
	FailedAttempts int              `json:"failed_attempts"`
	BucketCounts   map[string]int   `json:"bucket_counts"`
	Computers      []stalledOnboard `json:"computers"`
	Note           string           `json:"note,omitempty"`
}

func ageBucket(days float64) string {
	switch {
	case days < 1:
		return "<1d"
	case days < 3:
		return "1-3d"
	case days < 7:
		return "3-7d"
	case days < 30:
		return "7-30d"
	default:
		return "30d+"
	}
}

func newNovelOnboardingStalledCmd(flags *rootFlags) *cobra.Command {
	var (
		flagOlderThan string
		flagTenant    string
		flagLimit     int
		dbPath        string
	)

	cmd := &cobra.Command{
		Use:   "onboarding-stalled",
		Short: "Computers stuck waiting to onboard, bucketed by age",
		Long: "Computers stuck waiting to onboard, bucketed by age and annotated with whether " +
			"onboarding was ever attempted and how it ended.",
		Example: strings.Trim(`
  immybot-cli onboarding-stalled --older-than 3d
  immybot-cli onboarding-stalled --older-than 1w --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--older-than=3d",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "onboarding-stalled")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			var minAge time.Duration
			if s := strings.TrimSpace(flagOlderThan); s != "" {
				d, err := cliutil.ParseDurationLoose(s)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--older-than %q: %w", s, err))
				}
				minAge = d
			}

			dbPath = immyMirrorPath(dbPath)
			view := onboardingStalledView{
				OlderThan:    strings.TrimSpace(flagOlderThan),
				BucketCounts: map[string]int{},
				Computers:    make([]stalledOnboard, 0),
			}
			if immyMirrorMissing(cmd, dbPath, "computers-onboarding") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}

			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "computers-onboarding") {
				hintIfStale(cmd, db, "computers-onboarding", flags.maxAge)
			}

			rows, err := db.DB().QueryContext(ctx, `
				SELECT
					id,
					json_extract(data,'$.computerName'),
					json_extract(data,'$.tenantName'),
					json_extract(data,'$.serial'),
					json_extract(data,'$.onboardingStatus'),
					json_extract(data,'$.isOnline'),
					json_extract(data,'$.onboardingFailed'),
					json_extract(data,'$.onboardingSessionId'),
					json_extract(data,'$.updatedDate')
				FROM resources
				WHERE resource_type = 'computers-onboarding'`)
			if err != nil {
				return fmt.Errorf("querying onboarding queue: %w", err)
			}

			now := time.Now().UTC()
			scanned := make([]stalledOnboard, 0)
			for rows.Next() {
				var id, name, tenant, serial, status, online, failed, session, updated sql.NullString
				if err := rows.Scan(&id, &name, &tenant, &serial, &status, &online, &failed, &session, &updated); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning onboarding row: %w", err)
				}
				rec := stalledOnboard{
					ComputerID:   nullStr(id),
					ComputerName: nullStr(name),
					Tenant:       nullStr(tenant),
					Serial:       nullStr(serial),
					Status:       nullStr(status),
					Online:       truthy(nullStr(online)),
					Failed:       truthy(nullStr(failed)),
					SessionID:    nullStr(session),
					UpdatedDate:  nullStr(updated),
				}
				rec.Attempted = strings.TrimSpace(rec.SessionID) != "" && rec.SessionID != "0"
				switch {
				case rec.Failed:
					rec.Outcome = "attempted-failed"
				case rec.Attempted:
					rec.Outcome = "attempted-incomplete"
				default:
					rec.Outcome = "never-attempted"
				}
				if rec.UpdatedDate != "" {
					if t, err := time.Parse(time.RFC3339, rec.UpdatedDate); err == nil {
						rec.AgeDays = now.Sub(t).Hours() / 24
					}
				}
				rec.AgeBucket = ageBucket(rec.AgeDays)
				scanned = append(scanned, rec)
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("iterating onboarding queue: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("closing onboarding queue: %w", err)
			}

			tenantFilter := strings.ToLower(strings.TrimSpace(flagTenant))
			for _, rec := range scanned {
				view.ScannedQueued++
				if tenantFilter != "" && !strings.Contains(strings.ToLower(rec.Tenant), tenantFilter) {
					continue
				}
				if minAge > 0 && rec.UpdatedDate != "" && time.Duration(rec.AgeDays*24)*time.Hour < minAge {
					continue
				}
				view.StalledCount++
				view.BucketCounts[rec.AgeBucket]++
				switch rec.Outcome {
				case "never-attempted":
					view.NeverAttempted++
				case "attempted-failed":
					view.FailedAttempts++
				}
				view.Computers = append(view.Computers, rec)
			}
			sort.Slice(view.Computers, func(i, j int) bool {
				if view.Computers[i].AgeDays != view.Computers[j].AgeDays {
					return view.Computers[i].AgeDays > view.Computers[j].AgeDays
				}
				return view.Computers[i].ComputerName < view.Computers[j].ComputerName
			})
			if flagLimit > 0 && len(view.Computers) > flagLimit {
				view.Computers = view.Computers[:flagLimit]
			}
			if view.ScannedQueued == 0 {
				view.Note = "no onboarding queue rows in the local mirror; run 'immybot-cli sync --resources computers-onboarding'"
			} else if view.StalledCount == 0 {
				view.Note = fmt.Sprintf("scanned %d queued computer(s); none matched the age and tenant filters", view.ScannedQueued)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Computers) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No stalled onboards found (scanned %d queued computers).\n", view.ScannedQueued)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-28s %-26s %-20s %s\n", "AGE", "COMPUTER", "TENANT", "OUTCOME", "STATUS")
			for _, c := range view.Computers {
				fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-28s %-26s %-20s %s\n",
					c.AgeBucket, immyTruncate(c.ComputerName, 28), immyTruncate(c.Tenant, 26), c.Outcome, immyTruncate(c.Status, 24))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d stalled; %d never attempted, %d failed attempts.\n",
				view.StalledCount, view.NeverAttempted, view.FailedAttempts)
			return nil
		},
	}
	cmd.Flags().StringVar(&flagOlderThan, "older-than", "", "Only show computers queued longer than this (e.g. 3d, 1w)")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only show computers whose tenant name contains this substring")
	cmd.Flags().IntVar(&flagLimit, "limit", 100, "Maximum computers to return (0 for all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

// truthy interprets the JSON boolean shapes SQLite's json_extract can return
// (1/0, true/false) plus the string forms.
func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes":
		return true
	}
	return false
}
