// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// whereCollections is every collection the lists where join reads: the
// policies' lists, sites, roaming clients and scheduled policies (impact
// counts) and org names.
var whereCollections = []string{"organizations", "networks", "policies", "scheduled-policies", "user-agents"}

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
	// ScheduledSites and ScheduledRoamingClients count the sites and roaming
	// clients above that reach the policy through a slot of their scheduled
	// policy (their own or inherited from the site): it governs them only in
	// those 15-minute slots.
	ScheduledSites          int `json:"scheduled_sites"`
	ScheduledRoamingClients int `json:"scheduled_roaming_clients"`
}

func newNovelListsWhereCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "where [domain]",
		Short: "Find every client policy whose allow or block list contains a domain or a parent domain",
		Long: `Use this command for fleet-wide questions about where a domain sits on allow or block lists across every client org, parent-domain entries included.
Do NOT use this command for the effective verdict for one client; use 'why-blocked' instead.
Do NOT use this command for a security review of all allow lists; use 'policy-audit' instead.

Reads the local mirror: run 'dnsfilter-cli sync --full --resources organizations,networks,policies,scheduled-policies,user-agents' first. An entry matches the domain itself or any parent of it (an entry for example.com matches cdn.example.com).
sites and roaming_clients count every site and installed roaming client the policy governs: its own assignment, a roaming client inheriting its site's policy, or any slot of a scheduled policy (the client's own or inherited from its site). scheduled_sites and scheduled_roaming_clients say how many of those reach the policy through a schedule slot, so it governs them only part of the week.`,
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
			db, ok, err := openLocalMirror(cmd, flags, dbPath, "organizations,networks,policies,scheduled-policies,user-agents", incompleteMirrorJSON(make([]listsWhereRow, 0), whereCollections))
			if !ok {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "policies") {
				hintIfStale(cmd, db, "policies", flags.maxAge)
			}
			ctx := cmd.Context()
			loaded := map[string][]jaRes{}
			for _, rt := range []string{"policies", "organizations", "networks", "user-agents", "scheduled-policies"} {
				if loaded[rt], err = loadLocal(ctx, db, rt); err != nil {
					return err
				}
			}
			incomplete, err := incompleteCollections(ctx, db, whereCollections...)
			if err != nil {
				return err
			}
			rows, unresolved := computeListsWhere(normalizeDomain(args[0]), loaded["policies"], loaded["organizations"], loaded["networks"], loaded["user-agents"], loaded["scheduled-policies"])
			// A schedule the mirror lacks (or with no slot policy) leaves those
			// deployments' policies unknown; re-syncing a complete collection
			// cannot fix it, so it is reported apart from incomplete collections.
			withheld := len(incomplete) > 0 || len(unresolved) > 0
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if withheld {
					res := incompleteMirrorJSON(rows, incomplete)
					res.Meta.Note, res.Meta.UnresolvedSchedules = whereWithheldNote(incomplete, unresolved), sortedKeys(unresolved)
					fmt.Fprintln(cmd.ErrOrStderr(), "hint: "+res.Meta.Note)
					return printJSONFiltered(cmd.OutOrStdout(), res, flags)
				}
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			w := cmd.OutOrStdout()
			if len(rows) == 0 && !withheld {
				fmt.Fprintf(w, "No synced policy lists %s or a parent domain.\n", humanText(normalizeDomain(args[0])))
				return nil
			}
			for _, r := range rows {
				fmt.Fprintf(w, "%-5s %-28s %-24s policy %s %q sites=%d roaming=%d global=%v", humanText(r.List), humanText(r.Entry), humanText(r.OrgName), humanText(r.PolicyID), r.PolicyName, r.Sites, r.RoamingClients, r.IsGlobal)
				if r.ScheduledSites > 0 || r.ScheduledRoamingClients > 0 {
					fmt.Fprintf(w, " (via a schedule slot: sites=%d roaming=%d)", r.ScheduledSites, r.ScheduledRoamingClients)
				}
				fmt.Fprintln(w)
			}
			if withheld {
				// Unwalked sites or clients read as sites=0 roaming=0: no impact.
				fmt.Fprintln(w, humanText(whereWithheldNote(incomplete, unresolved)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI's synced database)")
	return cmd
}

// whereWithheldNote is what lists where prints instead of an all-clear: the
// schedules it could not resolve, then any incomplete collections (that note
// ends in the sync command).
func whereWithheldNote(incomplete []string, unresolved map[string]int) string {
	notes := make([]string, 0, 2)
	if len(unresolved) > 0 {
		n := 0
		for _, c := range unresolved {
			n += c
		}
		notes = append(notes, fmt.Sprintf("%d site or roaming-client assignment(s) use a scheduled policy that is not in the local mirror or names no policy in any slot (scheduled policy ids %s); their impact counts are unknown, so this is not an all-clear.", n, strings.Join(sortedKeys(unresolved), ", ")))
	}
	if len(incomplete) > 0 {
		notes = append(notes, incompleteSyncNote(incomplete))
	}
	return strings.Join(notes, " ")
}

// computeListsWhere is the pure join behind `lists where`. unresolved maps
// each scheduled policy id that deployments use but schedules lacks (or that
// lists no slot policy) to how many deployments use it; their policies, and
// so the counts, are unknown.
func computeListsWhere(domain string, policies, orgs, networks, agents, schedules []jaRes) (_ []listsWhereRow, unresolved map[string]int) {
	unresolved = map[string]int{}
	sites, roaming := map[string]int{}, map[string]int{}
	schedSites, schedRoaming := map[string]int{}, map[string]int{}
	// slotPolicies returns every policy any of a schedule's 672 slots uses.
	schedByID := byID(schedules)
	slotPolicies := func(schedID string) map[string]bool {
		ids := map[string]bool{}
		if schedID == "" {
			return ids
		}
		if sp, ok := schedByID[schedID]; ok {
			for _, id := range sp.strs("policy_ids") {
				if id != "0" {
					ids[id] = true
				}
			}
		}
		if len(ids) == 0 {
			unresolved[schedID]++
		}
		return ids
	}
	for _, n := range networks {
		ids := map[string]bool{n.relID("policy"): true}
		for _, id := range n.relIDs("policies") {
			ids[id] = true
		}
		for id := range slotPolicies(n.relID("scheduled_policy")) {
			ids[id] = true
			schedSites[id]++
		}
		for id := range ids {
			if id != "" {
				sites[id]++
			}
		}
	}
	sitePolicy, siteSched := map[string]string{}, map[string]string{}
	for _, n := range networks {
		if n.relID("scheduled_policy") == "" {
			sitePolicy[n.ID] = n.relID("policy")
		} else {
			siteSched[n.ID] = n.relID("scheduled_policy")
		}
	}
	for _, a := range agents {
		if isUninstalled(a) {
			continue
		}
		id, sched := a.relID("policy"), a.relID("scheduled_policy")
		// A client without its own assignment inherits its site's policy or
		// its site's schedule.
		if id == "" && sched == "" {
			id, sched = sitePolicy[a.relID("network")], siteSched[a.relID("network")]
		}
		ids := map[string]bool{id: true}
		for sid := range slotPolicies(sched) {
			ids[sid] = true
			schedRoaming[sid]++
		}
		for id := range ids {
			if id != "" {
				roaming[id]++
			}
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
				rows = append(rows, listsWhereRow{List: list.name, Entry: e, Match: match, PolicyID: p.ID, PolicyName: p.str("name"), OrgID: orgID, OrgName: orgByID[orgID].str("name"), IsGlobal: p.boolean("is_global_policy"), Sites: sites[p.ID], RoamingClients: roaming[p.ID], ScheduledSites: schedSites[p.ID], ScheduledRoamingClients: schedRoaming[p.ID]})
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
	return rows, unresolved
}
