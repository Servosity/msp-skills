// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type sweptRule struct {
	Mailbox      string   `json:"mailbox"`
	Name         string   `json:"name"`
	Identity     string   `json:"identity"`
	Enabled      *bool    `json:"enabled"`
	Flags        []string `json:"flags"`
	ForwardsTo   []string `json:"forwards_to,omitempty"`
	MoveToFolder string   `json:"move_to_folder,omitempty"`
}

type sweepView struct {
	Tenant    string      `json:"tenant"`
	Types     []string    `json:"mailbox_types"`
	Mailboxes int         `json:"scanned_mailboxes"`
	Total     int         `json:"total_mailboxes"`
	Rules     int         `json:"rules"`
	Flagged   int         `json:"flagged"`
	StoredAt  string      `json:"stored_at"`
	Note      string      `json:"note,omitempty"`
	Results   []sweptRule `json:"results"`
	Errors    []sourceErr `json:"errors"`
}

func newNovelInboxRulesSweepCmd(flags *rootFlags) *cobra.Command {
	var all bool
	var concurrency, maxMailboxes int
	var types string
	cmd := &cobra.Command{
		Use:   "sweep <tenantId>",
		Short: "Pull every mailbox's inbox rules in one tenant and flag the classic attacker rules.",
		Long: `Lists the tenant's mailboxes, runs Get-InboxRule for each one (routed to that mailbox), stores
every rule in the local store for forwarding-audit, and flags BEC signatures taken from CIPP's
BecHeuristics.json: deletes messages, moves to a low-visibility folder (RSS, Archive, Junk,
Conversation History, ...), marks read and hides, sensitive keywords or names, near-empty names,
and forwarding or redirecting to an outside address (judged from the address the rule shows;
forwarding-audit also resolves contacts). Hidden rules are included. Prints flagged rules (all rules with --all).

Use this command to harvest every mailbox's inbox rules in one tenant and flag BEC signatures
(delete, mark-as-read, move to RSS Feeds or Conversation History, finance keywords, external
forward/redirect). Do NOT use this command to decide whether forwarded mail actually leaves the
tenant; use 'forwarding-audit' instead. Do NOT use this command for one mailbox's rules; use
'inbox-rules list' or, for a suspected compromise, 'bec triage' instead.`,
		Example:     "  exchange-online-cli inbox-rules sweep 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent",
		Annotations: map[string]string{"mcp:local-write": "true", "pp:live-happy-path": "true", "pp:data-source": "live", "pp:happy-args": "tenantId=0550b473-9fd2-4dbb-a058-a1640b4bf458"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "inbox-rules sweep")
			}
			if concurrency < 1 || concurrency > 16 || maxMailboxes < 1 {
				return usageErr(fmt.Errorf("--concurrency must be 1-16 and --max-mailboxes at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxMailboxes > 10 {
				maxMailboxes = 10
			}
			tenant, err := exoTenantArg(cmd, args, 0)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			v, stored, err := sweepInboxRules(cmd.Context(), c, tenant, types, concurrency, maxMailboxes)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			at, err := storeSweep(cmd.Context(), tenant, v, stored)
			if err != nil {
				return err
			}
			v.StoredAt = at.Format("2006-01-02T15:04:05Z")
			warnPartial(cmd, v.Errors, v.Mailboxes, "mailbox")
			if !all {
				kept := v.Results[:0]
				for _, r := range v.Results {
					if len(r.Flags) > 0 {
						kept = append(kept, r)
					}
				}
				v.Results = kept
			}
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "List every rule, not only flagged ones")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Mailboxes queried in parallel (1-16)")
	cmd.Flags().StringVar(&types, "mailbox-types", "UserMailbox,SharedMailbox", "Comma-separated RecipientTypeDetails to sweep")
	cmd.Flags().IntVar(&maxMailboxes, "max-mailboxes", 2000, "Most mailboxes to sweep; the rest are reported in note")
	return cmd
}

// sweepInboxRules returns the sweep view plus the raw rule rows (tagged with
// _mailbox and _flags) for the snapshot table.
func sweepInboxRules(ctx context.Context, c *client.Client, tenant, types string, concurrency, maxMailboxes int) (sweepView, []exoRow, error) {
	v := sweepView{Tenant: tenant, Results: []sweptRule{}, Errors: []sourceErr{}}
	accepted, err := acceptedDomainSet(ctx, c, tenant)
	if err != nil {
		return v, nil, err
	}
	mbx, err := exoGet(ctx, c, exoBeta(tenant, "Get-Mailbox"), exoRow{"ResultSize": "Unlimited"}, "UserPrincipalName,PrimarySmtpAddress,RecipientTypeDetails")
	if err != nil {
		return v, nil, err
	}
	want := map[string]bool{}
	for _, t := range strings.Split(types, ",") {
		if t = strings.TrimSpace(t); t != "" {
			want[strings.ToLower(t)] = true
			v.Types = append(v.Types, t)
		}
	}
	var targets []string
	for _, m := range mbx {
		if len(want) > 0 && !want[strings.ToLower(rs(m, "RecipientTypeDetails"))] {
			continue
		}
		addr := rs(m, "UserPrincipalName")
		if addr == "" {
			addr = smtpOf(rs(m, "PrimarySmtpAddress"))
		}
		if addr != "" {
			targets = append(targets, addr)
		}
	}
	v.Total = len(targets)
	if len(targets) > maxMailboxes {
		targets = targets[:maxMailboxes]
		v.Note = fmt.Sprintf("swept the first %d of %d mailboxes; raise --max-mailboxes to cover the rest", maxMailboxes, v.Total)
	}
	v.Mailboxes = len(targets)

	var mu sync.Mutex
	var stored []exoRow
	jobs := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for addr := range jobs {
				rules, err := exoGet(ctx, c, exoBeta(tenant, "Get-InboxRule"), exoRow{"Mailbox": addr, "IncludeHidden": true}, "")
				mu.Lock()
				if err != nil {
					v.Errors = append(v.Errors, sourceErr{Source: "Get-InboxRule " + addr, Error: err.Error()})
				}
				for _, r := range rules {
					f := ruleFlags(r, accepted)
					r["_mailbox"] = addr
					r["_flags"] = f
					stored = append(stored, r)
					v.Results = append(v.Results, sweptRule{Mailbox: addr, Name: rs(r, "Name"), Identity: rs(r, "Identity"),
						Enabled: rb(r, "Enabled"), Flags: f, ForwardsTo: ruleTargets(r), MoveToFolder: rs(r, "MoveToFolder")})
				}
				mu.Unlock()
			}
		}()
	}
	for _, t := range targets {
		select {
		case jobs <- t:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return v, nil, err
	}
	v.Rules = len(v.Results)
	for _, r := range v.Results {
		if len(r.Flags) > 0 {
			v.Flagged++
		}
	}
	sort.Slice(v.Results, func(i, j int) bool {
		if len(v.Results[i].Flags) != len(v.Results[j].Flags) {
			return len(v.Results[i].Flags) > len(v.Results[j].Flags)
		}
		return v.Results[i].Mailbox+v.Results[i].Name < v.Results[j].Mailbox+v.Results[j].Name
	})
	return v, stored, nil
}
