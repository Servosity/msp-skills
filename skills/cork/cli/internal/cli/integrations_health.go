// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command. Preserved across `generate --force`.
// pp:data-source auto

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

type corkIntegration struct {
	UUID             string `json:"uuid"`
	DisplayName      string `json:"display_name"`
	ConnectionStatus string `json:"connection_status"`
	LastSyncedAt     string `json:"last_synced_at"`
	PartnerUUID      string `json:"partner_uuid"`
	Vendor           struct {
		Key  string `json:"key"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"vendor"`
}

type integrationHealthRow struct {
	UUID             string   `json:"uuid"`
	DisplayName      string   `json:"display_name"`
	Vendor           string   `json:"vendor"`
	ConnectionStatus string   `json:"connection_status"`
	LastSyncedAt     string   `json:"last_synced_at"`
	StaleHours       float64  `json:"stale_hours"`
	Verdict          string   `json:"verdict"`
	AffectedClients  []string `json:"affected_clients"`
}

type integrationHealthView struct {
	Items            []integrationHealthRow `json:"items"`
	IntegrationsSeen int                    `json:"integrations_seen"`
	Undecoded        int                    `json:"undecoded_records"`
	Unhealthy        int                    `json:"unhealthy"`
	SilentlyStale    int                    `json:"silently_stale"`
	StaleAfter       string                 `json:"stale_after"`
	Note             string                 `json:"note,omitempty"`
}

func newNovelIntegrationsHealthCmd(flags *rootFlags) *cobra.Command {
	var flagStaleAfter string
	var flagAll bool
	var flagMaxScanPages int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Flag connectors that are down, degraded, or reporting healthy while their last sync has gone stale",
		Long: "Flag connectors that are down, degraded, or reporting healthy while their\n" +
			"last sync has gone stale, and name the clients each one feeds.\n\n" +
			"Cork ships connection_status and last_synced_at in the same payload but never\n" +
			"compares that timestamp to now, so a connector whose feed stopped days ago can\n" +
			"keep reporting ok indefinitely. That silent-stale case is what this command\n" +
			"exists to catch, and it is reported separately from an openly failing one.\n\n" +
			"Use this command to find connectors that are down, degraded, or silently\n" +
			"stale. Do NOT use this command for a plain inventory of connected\n" +
			"integrations; use 'integrations get-connected' instead.",
		Example: "  cork-cli integrations health --stale-after 24h --agent",
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "integrations health")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			staleAfter, err := corkSince(flagStaleAfter, 24*time.Hour)
			if err != nil {
				return err
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			raw, _, capHit, err := corkFetchPages(ctx, c, "/integrations/connected", nil, flagMaxScanPages)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			// Map integration -> clients it feeds, via the tenants embedded on
			// each locally mirrored client.
			feeds := map[string][]string{}
			if db, ok, openErr := corkOpenStore(ctx, flagDB, cmd.ErrOrStderr(), cmd.OutOrStdout(), "clients"); openErr == nil && ok {
				defer db.Close()
				if clients, loadErr := corkLoadClients(ctx, db); loadErr == nil {
					for _, cl := range clients {
						for _, t := range cl.Tenants {
							// Index by both integration uuid and vendor key so
							// the lookup works whichever the connector reports.
							for _, k := range []string{t.Integration.UUID, t.Integration.Vendor.Key} {
								if k == "" {
									continue
								}
								feeds[k] = append(feeds[k], cl.Name)
							}
						}
					}
				}
			}

			now := time.Now()
			rows := make([]integrationHealthRow, 0, len(raw))
			seen := 0
			undecoded := 0
			unhealthy, silentlyStale := 0, 0
			for _, r := range raw {
				var in corkIntegration
				if json.Unmarshal(r, &in) != nil {
					undecoded++
					continue
				}
				seen++
				verdict := "ok"
				// Keep "how stale" and "is it knowable" separate. Overloading 0
				// for both "fresh" and "unparseable" makes a month-dead connector
				// report healthy; overloading a negative for "never synced"
				// makes a clock-skewed future timestamp report never-synced.
				var staleHours float64
				syncKnown := false
				neverSynced := in.LastSyncedAt == ""
				if t, ok := corkParseTime(in.LastSyncedAt); ok {
					syncKnown = true
					staleHours = float64(int(now.Sub(t).Hours()*10)) / 10
					if staleHours < 0 {
						staleHours = 0 // clock skew: treat a future stamp as fresh
					}
				}

				// Only degraded and down are genuine problems. queued, fetching
				// and deleting are ordinary lifecycle states and must not be
				// counted as unhealthy.
				var statusBad, statusTransient bool
				switch in.ConnectionStatus {
				case "degraded", "down":
					statusBad = true
				case "queued", "fetching", "deleting":
					statusTransient = true
				}
				isStale := syncKnown && staleHours > staleAfter.Hours()

				switch {
				case statusBad:
					verdict = "unhealthy:" + in.ConnectionStatus
					unhealthy++
				case neverSynced:
					verdict = "never-synced"
					silentlyStale++
				case !syncKnown:
					// A timestamp we cannot parse is not evidence of freshness.
					verdict = "unknown-last-sync"
					silentlyStale++
				case isStale:
					// The case the platform cannot show: green light, dead feed.
					verdict = "silently-stale"
					silentlyStale++
				case statusTransient:
					verdict = "transient:" + in.ConnectionStatus
				}
				if verdict == "ok" && !flagAll {
					continue
				}
				clientsFed := feeds[in.UUID]
				if len(clientsFed) == 0 {
					clientsFed = feeds[in.Vendor.Key]
				}
				// De-duplicate: a client with several tenants on one connector
				// must not be counted once per tenant.
				seenClient := map[string]struct{}{}
				uniq := make([]string, 0, len(clientsFed))
				for _, n := range clientsFed {
					if _, dup := seenClient[n]; dup {
						continue
					}
					seenClient[n] = struct{}{}
					uniq = append(uniq, n)
				}
				clientsFed = uniq
				corkSortStable(clientsFed, func(a, b string) bool { return a < b })
				rows = append(rows, integrationHealthRow{
					UUID:             in.UUID,
					DisplayName:      in.DisplayName,
					Vendor:           in.Vendor.Name,
					ConnectionStatus: in.ConnectionStatus,
					LastSyncedAt:     in.LastSyncedAt,
					StaleHours:       staleHours,
					Verdict:          verdict,
					AffectedClients:  clientsFed,
				})
			}

			// Openly broken first, then silent-stale, then longest stale.
			corkSortStable(rows, func(a, b integrationHealthRow) bool {
				aBad := a.Verdict != "ok" && a.Verdict != "silently-stale"
				bBad := b.Verdict != "ok" && b.Verdict != "silently-stale"
				if aBad != bBad {
					return aBad
				}
				return a.StaleHours > b.StaleHours
			})

			if seen == 0 && undecoded > 0 {
				return corkDecodeFailure("connected integration record(s)", undecoded)
			}

			view := integrationHealthView{
				Items:            rows,
				IntegrationsSeen: seen,
				Undecoded:        undecoded,
				Unhealthy:        unhealthy,
				SilentlyStale:    silentlyStale,
				StaleAfter:       staleAfter.String(),
			}
			// The cap warning must never be shadowed by the all-clear: a
			// truncated sweep that found nothing is not "all connectors healthy".
			switch {
			case seen == 0:
				view.Note = "no connected integrations returned"
			case len(rows) == 0:
				view.Note = fmt.Sprintf("all %d connector(s) examined report ok and synced within %s", seen, corkWindowLabel(flagStaleAfter, staleAfter))
				if capHit {
					view.Note += "; the integration page scan cap was reached, so connectors beyond it were not examined"
				}
			case capHit:
				view.Note = "integration page scan cap reached; results may be partial"
			}
			if undecoded > 0 {
				if view.Note != "" {
					view.Note += "; "
				}
				view.Note += fmt.Sprintf("%d connector record(s) could not be decoded and were not assessed", undecoded)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "INTEGRATION\tVENDOR\tSTATUS\tSTALE(h)\tVERDICT\tCLIENTS")
			for _, r := range rows {
				stale := fmt.Sprintf("%.1f", r.StaleHours)
				if r.Verdict == "never-synced" {
					stale = "never"
				} else if r.Verdict == "unknown-last-sync" {
					stale = "?"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\n",
					truncate(r.DisplayName, 28), truncate(r.Vendor, 20), r.ConnectionStatus, stale, r.Verdict, len(r.AffectedClients))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d unhealthy, %d silently stale, of %d connector(s)\n", view.Unhealthy, view.SilentlyStale, view.IntegrationsSeen)
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagStaleAfter, "stale-after", "24h", "Treat a connector as stale when its last sync is older than this (24h, 7d)")
	cmd.Flags().BoolVar(&flagAll, "all", false, "Include healthy connectors in the output")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", corkDefaultScanPages, "Maximum integration pages to read")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path used to map connectors to the clients they feed")
	return cmd
}
