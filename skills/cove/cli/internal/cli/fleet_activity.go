// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: console activity digest over the audit trail.
// pp:data-source live
package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cove-pp-cli/internal/coverpc"

	"github.com/spf13/cobra"
)

// activityUnknown stands in for an audit field the server left empty, so a row
// is always counted and never silently dropped.
const activityUnknown = "(unknown)"

// Cove keeps the User Actions log for one year.
const activityMaxWindow = 366 * 24 * time.Hour

type activityUser struct {
	User       string         `json:"user"`
	Partner    string         `json:"partner"`
	Actions    int            `json:"actions"`
	Operations map[string]int `json:"operations"`
	Results    map[string]int `json:"results"`
}

type activityOperation struct {
	Entity    string         `json:"entity"`
	Operation string         `json:"operation"`
	Actions   int            `json:"actions"`
	Users     int            `json:"users"`
	Results   map[string]int `json:"results"`
}

type activityView struct {
	From         string              `json:"from"`
	To           string              `json:"to"`
	TotalActions int                 `json:"total_actions"`
	Limit        int                 `json:"limit"`
	Truncated    bool                `json:"truncated"`
	Results      map[string]int      `json:"results"`
	Users        []activityUser      `json:"users"`
	Operations   []activityOperation `json:"operations"`
	Note         string              `json:"note,omitempty"`
}

// auditText reads one audit field as text through the shared column parser
// (a numeric or list value is rendered, not dropped); absent or blank becomes
// activityUnknown.
func auditText(row map[string]any, key string) string {
	if v, ok := coverpc.RowText(row, key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return activityUnknown
}

// digestAuditActions rolls audit rows up by user and by entity+operation.
// Every row is counted exactly once in each rollup.
func digestAuditActions(rows []map[string]any) (users []activityUser, ops []activityOperation, results map[string]int) {
	results = map[string]int{}
	byUser := map[string]*activityUser{}
	byOp := map[string]*activityOperation{}
	opUsers := map[string]map[string]bool{}
	for _, row := range rows {
		user, partner := auditText(row, "UserName"), auditText(row, "PartnerName")
		entity, operation := auditText(row, "EntityName"), auditText(row, "OperationName")
		result := auditText(row, "ResultName")
		results[result]++

		uk := partner + "\x00" + user
		u := byUser[uk]
		if u == nil {
			u = &activityUser{User: user, Partner: partner, Operations: map[string]int{}, Results: map[string]int{}}
			byUser[uk] = u
		}
		u.Actions++
		u.Operations[entity+" "+operation]++
		u.Results[result]++

		ok := entity + "\x00" + operation
		o := byOp[ok]
		if o == nil {
			o = &activityOperation{Entity: entity, Operation: operation, Results: map[string]int{}}
			byOp[ok] = o
			opUsers[ok] = map[string]bool{}
		}
		o.Actions++
		o.Results[result]++
		opUsers[ok][uk] = true
	}
	users = make([]activityUser, 0, len(byUser))
	for _, u := range byUser {
		users = append(users, *u)
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].Actions != users[j].Actions {
			return users[i].Actions > users[j].Actions
		}
		if users[i].User != users[j].User {
			return users[i].User < users[j].User
		}
		return users[i].Partner < users[j].Partner
	})
	ops = make([]activityOperation, 0, len(byOp))
	for k, o := range byOp {
		o.Users = len(opUsers[k])
		ops = append(ops, *o)
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Actions != ops[j].Actions {
			return ops[i].Actions > ops[j].Actions
		}
		if ops[i].Entity != ops[j].Entity {
			return ops[i].Entity < ops[j].Entity
		}
		return ops[i].Operation < ops[j].Operation
	})
	return users, ops, results
}

func newNovelFleetActivityCmd(flags *rootFlags) *cobra.Command {
	var since string
	var limit int
	cmd := &cobra.Command{
		Use:   "activity",
		Short: "Who did what in the Cove console: audit actions grouped by user and operation",
		Long: strings.Trim(`
Reads the management console audit trail (JSON-RPC EnumerateAuditActions,
whole sub-partner tree) for a window and rolls it up two ways: per user (how
many actions, which operations, which results) and per entity+operation (how
many actions, how many distinct users). Result names are shown exactly as Cove
reports them, never reinterpreted.

When the server returns --limit rows the window was cut short: truncated is
true and the digest covers only the newest rows. Raise --limit or narrow
--since.

Use this command for a review of console activity. Do NOT use it for backup
session results; use 'devices failures' or 'fleet health' instead.
`, "\n"),
		Example: strings.Trim(`
  cove-cli fleet activity --since 7d --agent
  cove-cli fleet activity --since 24h --json --select users.user,users.actions
  cove-cli fleet activity --since 30d --limit 5000 --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:data-source":         "live",
			"pp:no-error-path-probe": "true",
			"pp:requires-tier":       "credentials",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			window, err := parseCoveSince(since)
			if err != nil {
				return usageErr(err)
			}
			if window <= 0 {
				return usageErr(fmt.Errorf("--since must be a positive window such as 24h or 7d"))
			}
			if window > activityMaxWindow {
				return usageErr(fmt.Errorf("--since %s is longer than the one year Cove keeps", since))
			}
			if limit < 1 || limit > 100000 {
				return usageErr(fmt.Errorf("--limit must be between 1 and 100000, not %d", limit))
			}
			if dryRunOK(flags) {
				return coveDryRun(cmd, flags, "would read the console audit trail and group it by user and operation")
			}
			c, err := newCoveRPC(flags)
			if err != nil {
				return err
			}
			// Sign in first, like the partner lookup the other fleet commands
			// make: a credential failure exits 4, not as an API error.
			if _, err := c.Visa(cmd.Context()); err != nil {
				return authErr(err)
			}
			to := time.Now().UTC()
			from := to.Add(-window)
			result, err := c.Call(cmd.Context(), "EnumerateAuditActions", map[string]any{
				"actionInfo":            map[string]any{},
				"from":                  from.Unix(),
				"to":                    to.Unix(),
				"countLimit":            limit,
				"includeAllSubPartners": true,
				"reverseOrder":          true,
			})
			if err != nil {
				return apiErr(err)
			}
			rows, err := coverpc.Rows(result)
			if err != nil {
				return apiErr(err)
			}
			users, ops, results := digestAuditActions(rows)
			view := activityView{
				From:         from.Format(time.RFC3339),
				To:           to.Format(time.RFC3339),
				TotalActions: len(rows),
				Limit:        limit,
				Truncated:    len(rows) >= limit,
				Results:      results,
				Users:        users,
				Operations:   ops,
			}
			switch {
			case view.Truncated:
				view.Note = fmt.Sprintf("the server returned the %d-row limit, so this digest covers only the newest actions in the window; raise --limit or narrow --since", limit)
			case len(rows) == 0:
				view.Note = "no console actions recorded in the window"
			}
			return emitCoveJSON(cmd, flags, view, view.Users)
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Window to read, ending now (e.g. 24h, 7d, 30d; at most one year)")
	cmd.Flags().IntVar(&limit, "limit", 1000, "Maximum audit rows to request (countLimit)")
	return cmd
}
