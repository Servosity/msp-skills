// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newNovelClientCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "client <organization-name-or-id>",
		Short: "One client organization's open tickets by status and type, oldest open, and how many are breaching, from synced data.",
		Long: strings.Trim(`
Resolves a client organization by name (case-insensitive) or id in the local
store, joins its members to the reporters of synced requests, and summarizes the
open ones. Run 'sync --resources organization,request,requesttype' first.

Use this command for one client organization's open-ticket and SLA picture.
Do NOT use this command for customer or organization membership hygiene; use 'roster' instead.
Do NOT use this command for live cross-queue SLA risk; use 'breaches' instead.`, "\n"),
		Example:     `  jira-service-management-cli client "Acme Corp" --agent`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "organization-name-or-id=Acme Corp"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "client")
			}
			want := strings.TrimSpace(strings.Join(args, " "))
			if want == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an organization name or id is required"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, hint, err := openMirror(ctx, dbPath, "organization,request,requesttype")
			if err != nil {
				return err
			}
			if db == nil {
				fmt.Fprintln(cmd.ErrOrStderr(), hint)
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					// Not a zeroed picture: an unsynced store says nothing about the client.
					return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"org": want, "found": false, "known_organizations": []string{}, "note": hint}, flags)
				}
				return nil
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "organization") {
				hintIfStale(cmd, db, "request", flags.maxAge)
			}
			orgRaws, err := resourceData(ctx, db, "organization")
			if err != nil {
				return err
			}
			orgID, orgName := "", ""
			candidates := make([]string, 0)
			for _, raw := range orgRaws {
				var o jsmOrg
				if json.Unmarshal(raw, &o) != nil {
					continue
				}
				candidates = append(candidates, o.Name)
				if string(o.ID) == want || strings.EqualFold(o.Name, want) {
					orgID, orgName = string(o.ID), o.Name
					break
				}
			}
			if orgID == "" {
				// A local miss usually means "not synced yet", not a bad
				// request, so report it as an empty picture with the
				// names the mirror does know.
				sort.Strings(candidates)
				if len(candidates) > 20 {
					candidates = candidates[:20]
				}
				miss := map[string]any{"org": want, "found": false, "known_organizations": candidates,
					"note": fmt.Sprintf("no synced organization named %q; run 'sync --resources organization' if it is new", want)}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), miss, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), termSafe(fmt.Sprint(miss["note"])))
				if len(candidates) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "known: %s\n", termSafe(strings.Join(candidates, ", ")))
				}
				return nil
			}
			members, err := orgMembers(ctx, db, orgID)
			if err != nil {
				return err
			}
			reqs, err := localRequests(ctx, db)
			if err != nil {
				return err
			}
			_, types := nameIndex(ctx, db)
			p := clientRollup(orgID, orgName, members, reqs, types, time.Now().UnixMilli())
			if p.SLAIncomplete > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %d of this client's synced requests carry only part of their SLA metrics, so the breach count may undercount; re-run sync --resources request\n", p.SLAIncomplete)
			}
			if len(members) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: no synced members for %s; run sync --resources organization to fetch organization members\n", termSafe(orgName))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), p, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s: %s, %s, %d open, %d breaching\n", termSafe(p.Org), plural(p.Members, "member"), plural(p.Requests, "request"), p.Open, p.OngoingBreached)
			if p.OldestOpen != "" {
				age := "<1h"
				if p.OldestOpenAgeH > 0 {
					age = humanDuration(time.Duration(p.OldestOpenAgeH) * time.Hour)
				}
				fmt.Fprintf(w, "oldest open: %s (%s)\n", termSafe(p.OldestOpen), age)
			}
			for _, s := range sortedKeys(p.ByStatus) {
				fmt.Fprintf(w, "  status %-28s %d\n", termSafe(s), p.ByStatus[s])
			}
			for _, t := range sortedKeys(p.ByRequestType) {
				fmt.Fprintf(w, "  type   %-28s %d\n", termSafe(t), p.ByRequestType[t])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// sortedKeys orders counts by descending value, then name, for stable output.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
