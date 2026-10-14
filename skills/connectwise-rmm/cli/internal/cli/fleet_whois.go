// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
// Reads the synced fleet list locally; availability, patches and sessions come from the live API.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"connectwise-rmm-pp-cli/internal/fleet"
)

type whoisView struct {
	Query            string           `json:"query"`
	ScannedEndpoints int              `json:"scanned_endpoints"`
	CapHit           bool             `json:"scan_cap_hit,omitempty"`
	Matches          []fleet.WhoisRow `json:"matches"`
	FetchFailures    []fetchFailure   `json:"fetch_failures,omitempty"`
	Note             string           `json:"note,omitempty"`
}

func newNovelFleetWhoisCmd(flags *rootFlags) *cobra.Command {
	var company, cacheTTL, dbPath string
	var maxEndpoints int
	var refresh bool
	cmd := &cobra.Command{
		Use:   "whois [username]",
		Short: "Find the endpoints a user (username substring) is logged on to now or was last logged on to",
		Long: strings.Trim(`
Which endpoints a user is logged on to now (current) or was the last user of
(last), with company and site names. Matching is a case-insensitive substring,
so "jsmith" finds "ACME\jsmith".

Reads each in-scope endpoint's system state once and caches it (--cache-ttl),
so later lookups and 'fleet brief' name resolution are instant. Scope with
--company on large fleets.

Use this command when you know the person. Do NOT use this command when you
know the machine; use 'fleet brief' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-rmm-cli fleet whois jsmith --company "Acme Corp" --agent
  connectwise-rmm-cli fleet whois "ACME\mlee" --refresh`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "username=jsmith"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet whois")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a username is required, e.g. fleet whois jsmith"))
			}
			ttl, err := parseDurationFlag("cache-ttl", cacheTTL)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view := whoisView{Query: args[0], Matches: make([]fleet.WhoisRow, 0)}
			db, missing, err := openFleetStore(ctx, cmd, dbPath)
			if err != nil {
				return err
			}
			if missing {
				view.Note = noMirrorNote
				return emptyResult(cmd, flags, view)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "endpoints") {
				hintIfStale(cmd, db, "endpoints", flags.maxAge)
			}
			eps, err := loadFleet(db)
			if err != nil {
				return err
			}
			eps, view.CapHit = capEndpoints(filterFleet(eps, company, ""), maxEndpoints)
			view.ScannedEndpoints = len(eps)
			if len(eps) > 0 {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				raw, failures := cachedEndpointData(ctx, cmd, db, c, cacheSystemState, endpointIDs(eps), ttl, refresh, systemStatePath)
				states, bad := systemStates(raw)
				failures = append(failures, bad...)
				if len(states) == 0 && len(failures) > 0 {
					return classifyAPIErrorOnly(fmt.Errorf("system state API: %s", failures[0].Error))
				}
				warnFailures(cmd, "system state", failures, len(eps))
				view.FetchFailures = failures
				view.Matches = fleet.Whois(eps, states, args[0])
			}
			if note := unsyncedNote(view.ScannedEndpoints); note != "" {
				view.Note = note
			} else if len(view.Matches) == 0 && view.CapHit {
				view.Note = fmt.Sprintf("no match in the first %d endpoints; scope with --company or raise --max-endpoints", view.ScannedEndpoints)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Matches) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No endpoint has %q logged on (checked %d).\n", args[0], view.ScannedEndpoints)
				return nil
			}
			rows := make([]map[string]any, 0, len(view.Matches))
			for _, r := range view.Matches {
				rows = append(rows, map[string]any{"match": r.Match, "user": r.Username, "endpoint": firstNonEmpty(r.Name, r.ID), "company": r.CompanyName, "site": r.SiteName, "logon": r.LogonTime})
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&company, "company", "", "Limit to one company (ID or name)")
	cmd.Flags().StringVar(&cacheTTL, "cache-ttl", "1h", "Reuse cached system state younger than this")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Ignore cached system state and re-read every endpoint")
	cmd.Flags().IntVar(&maxEndpoints, "max-endpoints", 500, "Maximum endpoints to read system state for (0 = no cap)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
