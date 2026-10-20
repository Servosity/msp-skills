// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type listsWhereRow struct {
	List           string `json:"list"` // allow or block
	Entry          string `json:"entry"`
	Match          string `json:"match"` // exact, or parent (coverage not guaranteed: DNSFilter documents subdomain exceptions)
	PolicyID       string `json:"policy_id"`
	PolicyName     string `json:"policy_name"`
	OrgID          string `json:"org_id"`
	OrgName        string `json:"org_name"`
	IsGlobal       bool   `json:"is_global"`
	Sites          int    `json:"sites"`
	RoamingClients int    `json:"roaming_clients"`
}

func newNovelListsWhereCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "where [domain]",
		Short: "Find every client policy whose allow or block list contains a domain or a parent domain",
		Long: `Use this command for fleet-wide questions about where a domain sits on allow or block lists across every client org, parent-domain entries included.
Do NOT use this command for the effective verdict for one client; use 'why-blocked' instead.
Do NOT use this command for a security review of all allow lists; use 'policy-audit' instead.

Reads the local mirror: run 'dnsfilter-cli sync --resources organizations,networks,policies,user-agents' first. An entry matches the domain itself or any parent of it (an entry for example.com matches cdn.example.com).`,
		Example: strings.Trim(`
  dnsfilter-cli lists where example.com --agent
  dnsfilter-cli lists where cdn.example.com`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "domain=example.com"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "lists where")
			}
			if err := rejectExtraArgs(args, 1); err != nil {
				return err
			}
			if len(args) == 0 || normalizeDomain(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a domain argument is required, e.g. lists where example.com"))
			}
			db, ok, err := openLocalMirror(cmd, flags, dbPath, "organizations,networks,policies,user-agents", make([]listsWhereRow, 0))
			if !ok {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "policies") {
				hintIfStale(cmd, db, "policies", flags.maxAge)
			}
			ctx := cmd.Context()
			loaded := map[string][]jaRes{}
			for _, rt := range []string{"policies", "organizations", "networks", "user-agents"} {
				if loaded[rt], err = loadLocal(ctx, db, rt); err != nil {
					return err
				}
			}
			rows := computeListsWhere(normalizeDomain(args[0]), loaded["policies"], loaded["organizations"], loaded["networks"], loaded["user-agents"])
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			w := cmd.OutOrStdout()
			if len(rows) == 0 {
				fmt.Fprintf(w, "No synced policy lists %s or a parent domain.\n", normalizeDomain(args[0]))
				return nil
			}
			for _, r := range rows {
				fmt.Fprintf(w, "%-5s %-28s %-24s policy %s %q sites=%d roaming=%d global=%v\n", r.List, r.Entry, r.OrgName, r.PolicyID, r.PolicyName, r.Sites, r.RoamingClients, r.IsGlobal)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI's synced database)")
	return cmd
}

// computeListsWhere is the pure join behind `lists where`.
func computeListsWhere(domain string, policies, orgs, networks, agents []jaRes) []listsWhereRow {
	sites, roaming := map[string]int{}, map[string]int{}
	for _, n := range networks {
		ids := map[string]bool{n.relID("policy"): true}
		for _, id := range n.relIDs("policies") {
			ids[id] = true
		}
		for id := range ids {
			if id != "" {
				sites[id]++
			}
		}
	}
	sitePolicy := map[string]string{}
	for _, n := range networks {
		if n.relID("scheduled_policy") == "" {
			sitePolicy[n.ID] = n.relID("policy")
		}
	}
	for _, a := range agents {
		if isUninstalled(a) {
			continue
		}
		id := a.relID("policy")
		// A client without its own assignment inherits its site's policy
		// (sites on a schedule have no single policy and are not counted).
		if id == "" && a.relID("scheduled_policy") == "" {
			id = sitePolicy[a.relID("network")]
		}
		if id != "" {
			roaming[id]++
		}
	}
	orgByID := byID(orgs)
	rows := make([]listsWhereRow, 0)
	for _, p := range policies {
		orgID := p.str("organization_id")
		if orgID == "" {
			orgID = p.relID("organization")
		}
		for _, list := range []struct{ name, attr string }{{"allow", "whitelist_domains"}, {"block", "blacklist_domains"}} {
			for _, e := range p.strs(list.attr) {
				if !domainCovers(e, domain) {
					continue
				}
				match := "parent"
				if normalizeDomain(e) == domain {
					match = "exact"
				}
				rows = append(rows, listsWhereRow{List: list.name, Entry: e, Match: match, PolicyID: p.ID, PolicyName: p.str("name"), OrgID: orgID, OrgName: orgByID[orgID].str("name"), IsGlobal: p.boolean("is_global_policy"), Sites: sites[p.ID], RoamingClients: roaming[p.ID]})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].List != rows[j].List {
			return rows[i].List < rows[j].List
		}
		if rows[i].OrgName != rows[j].OrgName {
			return rows[i].OrgName < rows[j].OrgName
		}
		return rows[i].PolicyID < rows[j].PolicyID
	})
	return rows
}
