// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"slices"
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
sites and roaming_clients count every site and installed roaming client the policy governs: its own assignment, a roaming client inheriting its site's policy, or any slot of a scheduled policy (the client's own or inherited from its site). scheduled_sites and scheduled_roaming_clients say how many of those reach the policy through a schedule slot, so it governs them only part of the week.
Assignments are resolved as why-blocked resolves them: a scheduled policy replaces a retained direct policy, and a site's to-many policies relationship is not an assignment and is not counted. User, collection and LAN-subnet assignments are not exposed by the policy API and are not counted.
Unknown is never an all-clear: a policy whose allow or block list is absent, null or unreadable in the mirror is named in meta.unreadable_policies, a scheduled policy whose 672 slots are not all usable policy ids in meta.unresolved_schedules, roaming clients inheriting from a site the mirror lacks in meta.unplaced_roaming_clients, policies a site, roaming client or schedule slot uses that the mirror lacks in meta.missing_policies, and sites or roaming clients whose policy, scheduled-policy or site link cannot be read in meta.unreadable_assignments.`,
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
			// Rows and their completeness come from one snapshot.
			snap, err := mirrorSnapshot(ctx, db)
			if err != nil {
				return err
			}
			defer snap.Rollback()
			loaded := map[string][]jaRes{}
			for _, rt := range []string{"policies", "organizations", "networks", "user-agents", "scheduled-policies"} {
				if loaded[rt], err = loadLocal(ctx, snap, rt); err != nil {
					return err
				}
			}
			afterMirrorRowsRead()
			incomplete, err := incompleteCollections(ctx, snap, whereCollections...)
			if err != nil {
				return err
			}
			rows, unresolved := computeListsWhere(normalizeDomain(args[0]), loaded["policies"], loaded["organizations"], loaded["networks"], loaded["user-agents"], loaded["scheduled-policies"])
			// A schedule the mirror lacks (or with no slot policy) leaves those
			// deployments' policies unknown; re-syncing a complete collection
			// cannot fix it, so it is reported apart from incomplete collections.
			withheld := len(incomplete) > 0 || len(unresolved) > 0
			// HAND-FIX(lists-where-unreadable-policy-lists): a complete policies
			// sync does not make each policy's lists readable; one whose list is
			// absent or null cannot be ruled out, and neither can the impact of
			// a client inheriting from a site the mirror lacks.
			unreadable := unreadablePolicyLists(loaded["policies"])
			unplaced := unplacedRoamingClients(loaded["user-agents"], loaded["networks"])
			withheld = withheld || len(unreadable) > 0 || unplaced > 0
			// HAND-FIX(lists-where-missing-referenced-policies): a policy a
			// site, roaming client or schedule slot uses but the policies
			// table lacks cannot be ruled out, and neither can a site or
			// client whose assignment link is unreadable.
			missing := missingReferencedPolicies(loaded["policies"], loaded["networks"], loaded["user-agents"], loaded["scheduled-policies"])
			unreadableLinks := unreadableAssignments(loaded["networks"], loaded["user-agents"])
			withheld = withheld || len(missing) > 0 || unreadableLinks > 0
			extraNote := joinNotes(whereUnreadableNote(unreadable, unplaced), whereMissingNote(missing, unreadableLinks))
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if withheld {
					res := incompleteMirrorJSON(rows, incomplete)
					res.Meta.Note, res.Meta.UnresolvedSchedules = whereWithheldNote(incomplete, unresolved), sortedKeys(unresolved)
					res.Meta.Note, res.Meta.UnreadablePolicies, res.Meta.UnplacedRoamingClients = joinNotes(extraNote, res.Meta.Note), unreadable, unplaced
					res.Meta.MissingPolicies, res.Meta.UnreadableAssignments = missing, unreadableLinks
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
				fmt.Fprintln(w, humanText(joinNotes(extraNote, whereWithheldNote(incomplete, unresolved))))
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

// unreadablePolicyLists returns, sorted, the ids of policies whose
// whitelist_domains or blacklist_domains is absent, null, not a list, or holds
// an unreadable entry: lists where cannot rule the domain out of them.
func unreadablePolicyLists(policies []jaRes) []string {
	ids := map[string]bool{}
	for _, p := range policies {
		for _, attr := range []string{"whitelist_domains", "blacklist_domains"} {
			if _, ok := p.strsKnown(attr); !ok {
				ids[p.ID] = true
			}
		}
	}
	return sortedKeys(ids)
}

// unplacedRoamingClients counts installed roaming clients with no assignment
// of their own whose site is not in the mirror: their inherited policy, and
// so every impact count, is unknown.
func unplacedRoamingClients(agents, networks []jaRes) int {
	sites := byID(networks)
	n := 0
	for _, a := range agents {
		if isUninstalled(a) || a.relID("policy") != "" || a.relID("scheduled_policy") != "" {
			continue
		}
		if _, ok := sites[a.relID("network")]; !ok {
			n++
		}
	}
	return n
}

// missingReferencedPolicies returns, sorted, the ids of policies a site, an
// installed roaming client or a usable scheduled-policy slot puts in effect
// that the mirror's policies table does not hold: lists where cannot rule the
// domain out of them.
func missingReferencedPolicies(policies, networks, agents, schedules []jaRes) []string {
	have := byID(policies)
	schedByID := byID(schedules)
	missing := map[string]bool{}
	use := func(policyID, schedID string) {
		ids := []string{policyID}
		if schedID != "" {
			// A schedule replaces a retained direct policy; an unresolved
			// schedule is reported apart (meta.unresolved_schedules).
			ids = nil
			if slots, valid := scheduleSlotIDs(schedByID[schedID]); valid {
				ids = slots
			}
		}
		for _, id := range ids {
			if _, ok := have[id]; !ok && id != "" && id != "0" {
				missing[id] = true
			}
		}
	}
	for _, n := range networks {
		use(n.relID("policy"), n.relID("scheduled_policy"))
	}
	for _, a := range agents {
		if !isUninstalled(a) {
			use(a.relID("policy"), a.relID("scheduled_policy")) // an inheriting client uses its site's, checked above
		}
	}
	return sortedKeys(missing)
}

// unreadableAssignments counts sites and installed roaming clients whose
// policy or scheduled-policy relationship (or, for a client, site link) is
// present but unreadable: relID reads it as "no assignment", so the counts
// would fall back to a direct or inherited policy.
func unreadableAssignments(networks, agents []jaRes) int {
	readable := func(r jaRes, rels ...string) bool {
		for _, rel := range rels {
			if _, ok := r.relRef(rel); !ok {
				return false
			}
		}
		return true
	}
	n := 0
	for _, s := range networks {
		if !readable(s, "policy", "scheduled_policy") {
			n++
		}
	}
	for _, a := range agents {
		if !isUninstalled(a) && !readable(a, "policy", "scheduled_policy", "network") {
			n++
		}
	}
	return n
}

// whereMissingNote describes missing referenced policies and unreadable assignment links ("" when neither).
func whereMissingNote(missing []string, unreadableLinks int) string {
	notes := make([]string, 0, 2)
	if len(missing) > 0 {
		notes = append(notes, fmt.Sprintf("%d policies in use by a site, roaming client or schedule slot are not in the local mirror (policy ids %s), so the domain cannot be ruled out of them; this is not an all-clear.", len(missing), strings.Join(missing, ", ")))
	}
	if unreadableLinks > 0 {
		notes = append(notes, fmt.Sprintf("%d sites or roaming clients carry a policy, scheduled-policy or site link that cannot be read, so their policy is unknown; this is not an all-clear.", unreadableLinks))
	}
	return joinNotes(notes...)
}

// whereUnreadableNote describes unreadable policy lists and unplaced clients ("" when neither).
func whereUnreadableNote(unreadable []string, unplaced int) string {
	notes := make([]string, 0, 2)
	if len(unreadable) > 0 {
		notes = append(notes, fmt.Sprintf("%d policies have an allow or block list that is absent, null or unreadable in the local mirror (policy ids %s), so the domain cannot be ruled out of them; this is not an all-clear.", len(unreadable), strings.Join(unreadable, ", ")))
	}
	if unplaced > 0 {
		notes = append(notes, fmt.Sprintf("%d roaming clients inherit their policy from a site that is not in the local mirror; their policies, and the roaming-client counts, are unknown.", unplaced))
	}
	return joinNotes(notes...)
}

// computeListsWhere is the pure join behind `lists where`. unresolved maps
// each scheduled policy id that deployments use but schedules lacks (or whose
// raw slot list is not 672 usable policy ids) to how many deployments use it;
// their policies, and so the counts, are unknown.
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
		// HAND-FIX(lists-where-schedule-every-slot-resolved): one usable id
		// among 671 nulls resolved the schedule. Every slot must name a policy
		// (scheduleSlotIDs, and no "0"), or the slot policies are unknown.
		if sp, ok := schedByID[schedID]; ok {
			if slots, valid := scheduleSlotIDs(sp); valid && !slices.Contains(slots, "0") {
				for _, id := range sp.strs("policy_ids") {
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
		// A scheduled assignment replaces the direct one (why-blocked precedence).
		// The to-many policies relationship is not an assignment; why-blocked
		// ignores it, so it is not counted.
		ids := map[string]bool{}
		if n.relID("scheduled_policy") == "" {
			ids[n.relID("policy")] = true
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
		// A schedule replaces a retained direct policy (why-blocked precedence).
		if sched != "" {
			id = ""
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
		orgID := p.idAttr("organization_id")
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
