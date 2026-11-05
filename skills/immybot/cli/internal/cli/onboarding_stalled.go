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

	"github.com/spf13/cobra"
)

type stalledOnboard struct {
	ComputerID   string `json:"computer_id"`
	ComputerName string `json:"computer_name"`
	Tenant       string `json:"tenant"`
	Serial       string `json:"serial,omitempty"`
	Status       string `json:"onboarding_status,omitempty"`
	Online       bool   `json:"online"`
	Attempted    bool   `json:"onboarding_attempted"`
	Failed       bool   `json:"onboarding_failed"`
	Outcome      string `json:"outcome"`
	// AgeDays is null and AgeBucket "unknown" when updatedDate is absent or
	// does not parse: such a row has no age.
	AgeDays     *float64 `json:"age_days"`
	AgeBucket   string   `json:"age_bucket"`
	UpdatedDate string   `json:"updated_date,omitempty"`
	SessionID   string   `json:"onboarding_session_id,omitempty"`
	// at is the row's time (immyOnboardingTimeFields). The --older-than
	// filter compares the exact age as a duration: AgeDays is for display.
	at immyRowTime
}

type onboardingStalledView struct {
	OlderThan      string `json:"older_than,omitempty"`
	ScannedQueued  int    `json:"scanned_queued"`
	StalledCount   int    `json:"stalled_count"`
	NeverAttempted int    `json:"never_attempted"`
	FailedAttempts int    `json:"failed_attempts"`
	// Undated counts the computers --older-than left out because their
	// updatedDate is absent or does not parse, so their age is unknown.
	Undated      int              `json:"undated"`
	BucketCounts map[string]int   `json:"bucket_counts"`
	Computers    []stalledOnboard `json:"computers"`
	Note         string           `json:"note,omitempty"`
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
			"onboarding was ever attempted and how it ended. Age is measured from updatedDate; a " +
			"computer without a readable one has no age: it is bucketed unknown, never passes " +
			"--older-than, and is counted in undated when --older-than leaves it out.",
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

			db, err := immyOpenMirror(ctx, dbPath)
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
					`+immyTimeColumns(immyOnboardingTimeFields)+`
				FROM resources
				WHERE resource_type = 'computers-onboarding'`)
			if err != nil {
				return fmt.Errorf("querying onboarding queue: %w", err)
			}

			now := time.Now().UTC()
			scanned := make([]stalledOnboard, 0)
			for rows.Next() {
				var id, name, tenant, serial, status, online, failed, session sql.NullString
				ts := newImmyTimeScan(immyOnboardingTimeFields)
				dest := append([]any{&id, &name, &tenant, &serial, &status, &online, &failed, &session}, ts.dest()...)
				if err := rows.Scan(dest...); err != nil {
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
					at:           ts.time(),
				}
				// Shown as mirrored, even when it does not parse.
				rec.UpdatedDate = ts.raw("updatedDate")
				rec.Attempted = strings.TrimSpace(rec.SessionID) != "" && rec.SessionID != "0"
				switch {
				case rec.Failed:
					rec.Outcome = "attempted-failed"
				case rec.Attempted:
					rec.Outcome = "attempted-incomplete"
				default:
					rec.Outcome = "never-attempted"
				}
				rec.AgeBucket = immyUndatedBucket
				if rec.at.Known {
					days := now.Sub(rec.at.At).Hours() / 24
					rec.AgeDays, rec.AgeBucket = &days, ageBucket(days)
				}
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
				if minAge > 0 && !rec.at.AgeAtLeast(now, minAge) {
					if !rec.at.Known {
						view.Undated++
					}
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
			// Oldest first; a computer with no age sorts after every dated one.
			sort.Slice(view.Computers, func(i, j int) bool {
				a, b := view.Computers[i], view.Computers[j]
				if (a.AgeDays == nil) != (b.AgeDays == nil) {
					return b.AgeDays == nil
				}
				if a.AgeDays != nil && *a.AgeDays != *b.AgeDays {
					return *a.AgeDays > *b.AgeDays
				}
				return a.ComputerName < b.ComputerName
			})
			if flagLimit > 0 && len(view.Computers) > flagLimit {
				view.Computers = view.Computers[:flagLimit]
			}
			if view.ScannedQueued == 0 {
				view.Note = "no onboarding queue rows in the local mirror; run 'immybot-cli sync --resources computers-onboarding'"
			} else if view.StalledCount == 0 {
				view.Note = fmt.Sprintf("scanned %d queued computer(s); none matched the age and tenant filters", view.ScannedQueued)
			}
			if view.Undated > 0 {
				msg := fmt.Sprintf("%d queued computer(s) have no updatedDate that parses, so their age is unknown "+
					"and --older-than left them out; run without --older-than to list them", view.Undated)
				view.Note = strings.TrimPrefix(view.Note+"; "+msg, "; ")
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Computers) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No stalled onboards found (scanned %d queued computers).\n", view.ScannedQueued)
				if view.Undated > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				}
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-28s %-26s %-20s %s\n", "AGE", "COMPUTER", "TENANT", "OUTCOME", "STATUS")
			for _, c := range view.Computers {
				fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-28s %-26s %-20s %s\n",
					c.AgeBucket, immyTruncate(c.ComputerName, 28), immyTruncate(c.Tenant, 26), c.Outcome, immyTruncate(c.Status, 24))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d stalled; %d never attempted, %d failed attempts.\n",
				view.StalledCount, view.NeverAttempted, view.FailedAttempts)
			if view.Undated > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%d left out: no updatedDate that parses, so their age is unknown.\n", view.Undated)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagOlderThan, "older-than", "", "Only show computers queued longer than this (e.g. 3d, 1w); a computer with no readable updatedDate never matches and is counted in undated")
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
