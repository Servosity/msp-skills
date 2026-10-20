// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"saas-alerts-pp-cli/internal/cliutil"
	"saas-alerts-pp-cli/internal/store"
)

// rqQueueEnvelope is the 'rule-triggers queue' result.
type rqQueueEnvelope struct {
	Source         string `json:"source"`
	FallbackReason string `json:"fallback_reason"`
	Fetched        int    `json:"fetched"`
	Pages          int    `json:"pages"`
	Truncated      bool   `json:"truncated"`
	TruncatedWhy   string `json:"truncated_reason"`
	Now            string `json:"now"`
	ExpiringWithin int64  `json:"expiring_within_seconds,omitempty"`
	ExpiredCount   int    `json:"expired_count"`
	// Undecoded counts rows rejected for a missing id or a missing/unknown
	// status; any makes the result incomplete and the exit non-zero.
	Undecoded int `json:"undecoded"`
	// Synced is set for local results: false when there is no local mirror
	// or it has never been synced for rule-triggers, so an empty list is not
	// mistaken for an empty queue. Note says why.
	Synced *bool         `json:"synced,omitempty"`
	Note   string        `json:"note,omitempty"`
	Items  []rqQueueItem `json:"items"`
}

func newNovelRuleTriggersQueueCmd(flags *rootFlags) *cobra.Command {
	var flagStatuses string
	var flagExpiringWithin string
	var flagOrg string
	var flagRule string
	var flagMaxPages int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "queue",
		Short: "List pending and failed Respond triggers by how soon they expire or auto-ignore",
		Long:  "Use this command to see and prioritize Respond triggers waiting on a decision (read-only). Do NOT use this command to approve, reject or ignore; use 'rule-triggers decide' instead.",
		Example: strings.Trim(`
  saas-alerts-cli rule-triggers queue --expiring-within 2h --agent
  saas-alerts-cli rule-triggers queue --statuses PENDING --rule RULE_ID_12345 --json
  saas-alerts-cli rule-triggers queue --data-source local --org ORGANIZATION_ID_12345`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
			"pp:happy-args":  "--expiring-within=2h",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "rule-triggers queue")
			}
			statuses, err := rqParseStatuses(flagStatuses, "statuses")
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			var within time.Duration
			if strings.TrimSpace(flagExpiringWithin) != "" {
				within, err = cliutil.ParseDurationLoose(flagExpiringWithin)
				if err != nil || within <= 0 {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--expiring-within must be a positive duration like 30m, 2h or 1d (got %q)", flagExpiringWithin))
				}
			}
			if flagMaxPages < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--max-pages must be at least 1"))
			}
			filter := rqFilter{Statuses: statuses, OrgIDs: rqSplitCSV(flagOrg), RuleIDs: rqSplitCSV(flagRule)}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			now := rqNow()
			env := rqQueueEnvelope{Now: rqFormatTime(now), Items: make([]rqQueueItem, 0)}
			if within > 0 {
				env.ExpiringWithin = int64(within / time.Second)
			}
			var triggers []rqTrigger
			useLocal := flags.dataSource == "local"
			if !useLocal {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				switch {
				case c.Config == nil || !c.Config.CredentialConfigured():
					if flags.dataSource == "live" {
						return authErr(fmt.Errorf("no API key configured: set SAAS_ALERTS_API_KEY (the CLI exchanges it for a Respond access token)"))
					}
					useLocal = true
					env.FallbackReason = "no API key configured for the live Respond API"
				default:
					res, err := rqFetchLive(ctx, c, filter, flagMaxPages)
					if err != nil {
						if flags.dataSource == "auto" && rqLiveUnavailable(err) {
							useLocal = true
							env.FallbackReason = "live Respond API unavailable: " + err.Error()
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s; showing the local mirror\n", env.FallbackReason)
						} else {
							return classifyAPIError(cmd.OutOrStdout(), err, flags)
						}
					} else {
						env.Source = "live"
						env.Pages = res.Pages
						env.Truncated = res.Truncated
						env.TruncatedWhy = res.TruncatedReason
						triggers = res.Triggers
						if res.Truncated {
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: queue is incomplete after %d page(s): %s\n", res.Pages, res.TruncatedReason)
						}
						env.Undecoded = res.Undecoded
						if res.Undecoded > 0 {
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: rejected %d trigger row(s) without an id or a RuleTrigger status; the queue is incomplete\n", res.Undecoded)
						}
					}
				}
			}
			if useLocal {
				env.Source = "local"
				flags.agentSource = "local"
				dbPath := flagDB
				if dbPath == "" {
					dbPath = defaultDBPath("saas-alerts-cli")
				}
				if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
					// One "hint:" line: the MCP shell-out keeps only stderr lines
					// that start with hint: or warning:.
					fmt.Fprintf(cmd.ErrOrStderr(), "hint: no local mirror at %s; run: saas-alerts-cli sync --resources rule-triggers --db %s\n", dbPath, dbPath)
					notSynced := false
					env.Synced = &notSynced
					env.Note = "no local mirror at " + dbPath + "; this empty result is not an empty queue: run saas-alerts-cli sync --resources rule-triggers --db " + dbPath
					if wantsHumanTable(cmd.OutOrStdout(), flags) {
						return nil
					}
					return printJSONFiltered(cmd.OutOrStdout(), env, flags)
				}
				// The local read gets its own context: a live attempt that timed
				// out has already spent the bounded one.
				lctx := cmd.Context()
				db, err := store.OpenReadOnlyContext(lctx, dbPath)
				if err != nil {
					return fmt.Errorf("opening local store %s: %w", dbPath, err)
				}
				defer db.Close()
				loaded, bad, err := rqLoadLocal(lctx, db)
				if err != nil {
					return fmt.Errorf("reading synced rule-triggers: %w", err)
				}
				env.Undecoded = bad
				if bad > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: rejected %d synced trigger row(s) without an id or a RuleTrigger status; the queue is incomplete\n", bad)
				}
				synced := !hintIfUnsynced(cmd, db, "rule-triggers")
				if synced {
					hintIfStale(cmd, db, "rule-triggers", flags.maxAge)
				} else {
					env.Note = "the local store has never been synced for rule-triggers; run saas-alerts-cli sync --resources rule-triggers"
				}
				env.Synced = &synced
				triggers = loaded
			}

			// Live results were filtered server-side; filtering again keeps the
			// answer exact if the API ignores a parameter, and is the only filter
			// on the local path.
			triggers = filter.apply(triggers)
			env.Fetched = len(triggers)
			items := rqAnnotate(triggers, now)
			rqSortQueue(items)
			for _, it := range items {
				if it.Expired {
					env.ExpiredCount++
				}
				if within > 0 {
					if it.TimeLeft == nil || it.Expired || time.Duration(*it.TimeLeft)*time.Second > within {
						continue
					}
				}
				env.Items = append(env.Items, it)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), env, flags); err != nil {
					return err
				}
			} else if err := rqPrintQueueHuman(cmd, env); err != nil {
				return err
			}
			// Rejected rows may be triggers that belong in this queue, so the
			// result is incomplete: exit non-zero rather than look complete.
			if env.Undecoded > 0 {
				return apiErr(fmt.Errorf("%d rule trigger row(s) were rejected (no id or no RuleTrigger status); the queue is incomplete", env.Undecoded))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagStatuses, "statuses", "PENDING,REMEDIATION_FAILED", "Comma-separated trigger statuses to include")
	cmd.Flags().StringVar(&flagExpiringWithin, "expiring-within", "", "Only show triggers whose deadline falls within this window from now (such as 30m, 2h or 1d); expired triggers are excluded but still counted in expired_count")
	cmd.Flags().StringVar(&flagOrg, "org", "", "Comma-separated Respond organization IDs to include")
	cmd.Flags().StringVar(&flagRule, "rule", "", "Comma-separated Respond rule IDs to include")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", rqDefaultMaxPages, "Maximum pages of live rule triggers to fetch (100 per page)")
	cmd.Flags().StringVar(&flagDB, "db", "", "Local store path for the offline fallback (default: the CLI's local store)")
	return cmd
}

func rqPrintQueueHuman(cmd *cobra.Command, env rqQueueEnvelope) error {
	w := saasHumanWriter(cmd.OutOrStdout()) // saas-alerts hand-fix: tenant strings never drive the terminal
	fmt.Fprintf(w, "source: %s, %d trigger(s) fetched, %d expired", env.Source, env.Fetched, env.ExpiredCount)
	if env.FallbackReason != "" {
		fmt.Fprintf(w, " (%s)", env.FallbackReason)
	}
	fmt.Fprintln(w)
	if len(env.Items) == 0 {
		fmt.Fprintln(w, "No triggers waiting on a decision.")
		return nil
	}
	tw := newTabWriter(w)
	fmt.Fprintln(tw, "TIME LEFT\tSTATUS\tSEVERITY\tRULE\tORGANIZATION\tACCOUNTS\tWOULD FIRE\tTRIGGER")
	for _, it := range env.Items {
		left := rqHumanLeft(it.TimeLeft)
		if it.Superseded {
			left += " (superseded)"
		}
		if it.RemindersSent != nil {
			left += fmt.Sprintf(" (%d/%d reminders)", *it.RemindersSent, rqFailedReminderLimit)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			left, it.Status, it.Severity, truncate(it.RuleName, 32), truncate(it.OrganizationName, 24),
			truncate(strings.Join(it.Accounts, ","), 32), strings.Join(it.WouldFire, ", "), it.TriggerID)
	}
	return tw.Flush()
}
