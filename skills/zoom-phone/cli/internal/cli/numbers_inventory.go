// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: phone number inventory cross-tab.
// pp:data-source local
// Joins the local numbers rows (GET /phone/numbers) to phone users. No API
// calls.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var localInventorySyncResources = []string{"numbers", "users"}

type localInventoryCell struct {
	Site         string `json:"site"`
	Source       string `json:"source"`
	AssigneeType string `json:"assignee_type"`
	Count        int    `json:"count"`
}

type localInventoryNumber struct {
	ID               string `json:"id"`
	Number           string `json:"number"`
	Site             string `json:"site"`
	Source           string `json:"source"`
	NumberType       string `json:"number_type,omitempty"`
	Status           string `json:"status,omitempty"`
	Location         string `json:"location,omitempty"`
	AssigneeType     string `json:"assignee_type,omitempty"`
	AssigneeID       string `json:"assignee_id,omitempty"`
	AssigneeName     string `json:"assignee_name,omitempty"`
	UserStatus       string `json:"user_status,omitempty"`
	ActivationStatus string `json:"activation_status,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type localInventoryCounts struct {
	Total            int            `json:"total"`
	Assigned         int            `json:"assigned"`
	Unassigned       int            `json:"unassigned"`
	Pending          int            `json:"pending"`
	InactiveAssignee int            `json:"inactive_assignee"`
	BySource         map[string]int `json:"by_source"`
	ByAssigneeType   map[string]int `json:"by_assignee_type"`
}

type localInventoryResult struct {
	Assessed         bool                   `json:"assessed"`
	SiteFilter       string                 `json:"site_filter,omitempty"`
	Counts           localInventoryCounts   `json:"counts"`
	Breakdown        []localInventoryCell   `json:"breakdown"`
	Unassigned       []localInventoryNumber `json:"unassigned"`
	Pending          []localInventoryNumber `json:"pending"`
	InactiveAssignee []localInventoryNumber `json:"inactive_assignee"`
	MissingLocal     []string               `json:"missing_local"`
	MissingReasons   map[string]string      `json:"missing_reasons"`
	Note             string                 `json:"note,omitempty"`
}

func localNewInventoryResult(site string) *localInventoryResult {
	return &localInventoryResult{
		SiteFilter:       site,
		Counts:           localInventoryCounts{BySource: map[string]int{}, ByAssigneeType: map[string]int{}},
		Breakdown:        make([]localInventoryCell, 0),
		Unassigned:       make([]localInventoryNumber, 0),
		Pending:          make([]localInventoryNumber, 0),
		InactiveAssignee: make([]localInventoryNumber, 0),
		MissingLocal:     make([]string, 0),
		MissingReasons:   map[string]string{},
	}
}

// localNumbersInventory builds the cross-tab. "unassigned" excludes pending
// numbers (a pending number is not free to assign); a number assigned to a
// user that is deactivated, "not ready", or absent from the synced phone
// users list is reported under inactive_assignee.
//
// usersSynced is whether the users collection finished syncing: only then can
// an assignee missing from it be called orphaned (a completed but empty users
// sync makes every user assignee an orphan).
func localNumbersInventory(numbers []localNumber, users []localUser, usersSynced bool, site string) *localInventoryResult {
	res := localNewInventoryResult(site)
	usersByID := map[string]localUser{}
	for _, u := range users {
		usersByID[u.ID.String()] = u
	}
	cells := map[[3]string]int{}
	for _, n := range numbers {
		if !localSiteMatches(site, n.Site) {
			continue
		}
		siteLabel := localSiteLabel(n.Site)
		if siteLabel == "" {
			siteLabel = "(no site)"
		}
		source := localNumberSourceClass(n)
		assigneeType := "unassigned"
		if n.Assignee != nil && (n.Assignee.ID.String() != "" || n.Assignee.Type.String() != "") {
			assigneeType = n.Assignee.Type.String()
			if assigneeType == "" {
				assigneeType = "unknown"
			}
		}
		row := localInventoryNumber{
			ID:         n.ID.String(),
			Number:     n.Number.String(),
			Site:       siteLabel,
			Source:     source,
			NumberType: n.NumberType.String(),
			Status:     n.Status.String(),
			Location:   n.Location.String(),
		}
		if assigneeType != "unassigned" {
			row.AssigneeType = assigneeType
			row.AssigneeID = n.Assignee.ID.String()
			row.AssigneeName = n.Assignee.Name.String()
		}

		res.Counts.Total++
		res.Counts.BySource[source]++
		res.Counts.ByAssigneeType[assigneeType]++
		cells[[3]string{siteLabel, source, assigneeType}]++

		pending := strings.EqualFold(n.Status.String(), "pending")
		if pending {
			res.Counts.Pending++
			res.Pending = append(res.Pending, row)
		}
		if assigneeType == "unassigned" {
			if !pending {
				res.Counts.Unassigned++
				res.Unassigned = append(res.Unassigned, row)
			}
			continue
		}
		res.Counts.Assigned++
		if !strings.EqualFold(assigneeType, "user") {
			continue
		}
		u, ok := usersByID[row.AssigneeID]
		reason := ""
		switch {
		case ok:
			reason = localUserInactiveReason(u)
			row.UserStatus = u.Status.String()
			row.ActivationStatus = u.ActivationStatus.String()
		case usersSynced && row.AssigneeID != "":
			reason = "not_in_phone_users"
		}
		if reason != "" {
			row.Reason = reason
			res.Counts.InactiveAssignee++
			res.InactiveAssignee = append(res.InactiveAssignee, row)
		}
	}
	for k, v := range cells {
		res.Breakdown = append(res.Breakdown, localInventoryCell{Site: k[0], Source: k[1], AssigneeType: k[2], Count: v})
	}
	sort.Slice(res.Breakdown, func(i, j int) bool {
		a, b := res.Breakdown[i], res.Breakdown[j]
		if a.Site != b.Site {
			return a.Site < b.Site
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.AssigneeType < b.AssigneeType
	})
	for _, list := range [][]localInventoryNumber{res.Unassigned, res.Pending, res.InactiveAssignee} {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Number < list[j].Number })
	}
	return res
}

func newNovelNumbersInventoryCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var site string

	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Count synced phone numbers by site, source and assignee type, and list unassigned, pending and inactive-assignee numbers (local store; --site filters).",
		Long: strings.TrimSpace(`
Counts every synced phone number per site x source x assignee type, then
lists unassigned numbers (excluding pending ones), pending numbers, and
numbers assigned to phone users who are deactivated, not ready to activate,
or no longer in the phone users list.

Source is internal or external as returned by GET /phone/numbers; numbers
carrying BYOC carrier or SIP group details (synced with type=byoc) count as
byoc. Reads the local store only; --site filters by site name or id.
"assessed" is true only when numbers and users both have a completed, full,
unfiltered sync (zoom-phone-cli sync --full --resources numbers,users; a
plain sync does not delete removed numbers), i.e. missing_local is empty;
missing_reasons says why. The additive BYOC walk for e911
(sync --resources numbers --resource-param numbers:type=byoc, no --full)
leaves that coverage intact; the same walk run with --full deletes every
non-BYOC number and so un-assesses the inventory. A --db path that does not
exist is a usage error (exit 2).`),
		Example: strings.Trim(`
  zoom-phone-cli numbers inventory --agent
  zoom-phone-cli numbers inventory --site "San Jose HQ" --json --select unassigned`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "local",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "numbers inventory")
			}
			res := localNewInventoryResult(site)
			db, err := localOpenStore(cmd, dbPath, localInventorySyncResources)
			if err != nil {
				return err
			}
			if db != nil {
				// Rows and coverage come from one read snapshot. numbers needs an
				// unfiltered --full sync; an additive byoc walk keeps that record
				// in force (see localRecordSyncScope).
				var numbers []localNumber
				var users []localUser
				var cov localCoverage
				err := localReadSnapshot(cmd.Context(), db, func(q localQuerier) error {
					var err error
					if numbers, err = localLoad[localNumber](q, "numbers"); err != nil {
						return err
					}
					if users, err = localLoadUsers(q); err != nil {
						return err
					}
					cov = localCoverageFor(q, nil, localInventorySyncResources...)
					return nil
				})
				synced := cov.synced
				_ = db.Close()
				if err != nil {
					return err
				}
				res = localNumbersInventory(numbers, users, synced["users"], site)
				res.MissingLocal = localMissing(synced, localInventorySyncResources...)
				res.MissingReasons = localMissingReasons(cov, res.MissingLocal)
				// assessed: every resource the inventory reads finished syncing.
				res.Assessed = len(res.MissingLocal) == 0
				localHintMissing(cmd, res.MissingLocal, res.MissingReasons)
				if res.Counts.Total == 0 && site != "" && len(numbers) > 0 {
					res.Note = fmt.Sprintf("no numbers matched --site %q (%d numbers synced)", site, len(numbers))
				}
				if !res.Assessed {
					res.Note = localNotAssessedNote(res.MissingLocal, res.Note)
				}
			} else {
				res.MissingLocal = append(res.MissingLocal, localInventorySyncResources...)
				sort.Strings(res.MissingLocal)
				for _, m := range res.MissingLocal {
					res.MissingReasons[m] = "no local store"
				}
				res.Note = "no local store; nothing was assessed; run: " + localSyncCommand(localInventorySyncResources)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), res, flags)
			}
			localWriteNotAssessed(cmd.OutOrStdout(), res.Assessed, res.Note)
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "SITE\tSOURCE\tASSIGNEE TYPE\tCOUNT")
			for _, c := range res.Breakdown {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", c.Site, c.Source, c.AssigneeType, c.Count)
			}
			fmt.Fprintf(w, "\n%d numbers: %d assigned, %d unassigned, %d pending, %d assigned to inactive users\n",
				res.Counts.Total, res.Counts.Assigned, res.Counts.Unassigned, res.Counts.Pending, res.Counts.InactiveAssignee)
			for _, n := range res.InactiveAssignee {
				fmt.Fprintf(w, "inactive assignee\t%s\t%s\t%s\n", n.Number, n.AssigneeName, n.Reason)
			}
			for _, n := range res.Unassigned {
				fmt.Fprintf(w, "unassigned\t%s\t%s\t%s\n", n.Number, n.Site, n.Source)
			}
			if res.Note != "" && res.Assessed {
				fmt.Fprintf(w, "note: %s\n", res.Note)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: the synced store for the active account)")
	cmd.Flags().StringVar(&site, "site", "", "Only count numbers in this site (name or id, case-insensitive)")
	return cmd
}
