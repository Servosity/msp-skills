// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"exchange-online-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type queueItem struct {
	Tenant       string   `json:"tenant"`
	Identity     string   `json:"identity"`
	Received     string   `json:"received"`
	AgeHours     int      `json:"age_hours"`
	DaysToExpiry *int     `json:"days_to_expiry,omitempty"`
	Sender       string   `json:"sender"`
	Recipients   []string `json:"recipients"`
	Subject      string   `json:"subject"`
	Type         string   `json:"type"`
	Policy       string   `json:"policy,omitempty"`
	AdminOnly    bool     `json:"admin_only"`
	receivedAt   time.Time
}

type senderGroup struct {
	Sender   string   `json:"sender"`
	Messages int      `json:"messages"`
	Tenants  []string `json:"tenants"`
}

type queueSnapshot struct {
	Tenant    string `json:"tenant"`
	FetchedAt string `json:"fetched_at"`
	Requests  int    `json:"requests"`
	Shown     int    `json:"shown"`
}

type quarantineQueueView struct {
	Queue     []queueItem     `json:"queue"`
	Senders   []senderGroup   `json:"repeat_senders"`
	Snapshots []queueSnapshot `json:"snapshots"`
	OlderThan string          `json:"older_than,omitempty"`
	Hidden    int             `json:"hidden_by_older_than"`
	Note      string          `json:"note,omitempty"`
}

func newNovelQuarantineQueueCmd(flags *rootFlags) *cobra.Command {
	var tenant, olderThan string
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "One oldest-first queue of quarantine release requests across every tenant you have snapshotted.",
		Long: `Reads the release requests saved by 'quarantine snapshot' for every tenant (or --tenant) from the
local store and lists them oldest first with age, days until the message expires, and whether only
an admin can release it under the default policy (high confidence phishing, malware). Senders
that appear in several requests or tenants are grouped under repeat_senders to expose one campaign.
Local only: refresh each tenant with 'quarantine snapshot' using that tenant's token.

Use this command for the cross-tenant list of quarantined messages users asked to release, oldest
first, from synced snapshots. Do NOT use this command to find out why one message never arrived;
use 'where-is' instead. To act on one item, use 'quarantine release'.`,
		Example:     "  exchange-online-cli quarantine queue --older-than 24h --agent\n  exchange-online-cli quarantine queue --tenant 0550b473-9fd2-4dbb-a058-a1640b4bf458 --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "quarantine queue")
			}
			var minAge time.Duration
			if olderThan != "" {
				d, err := cliutil.ParseDurationLoose(olderThan)
				if err != nil || d < 0 {
					return usageErr(fmt.Errorf("--older-than must be a duration like 24h or 3d, got %q", olderThan))
				}
				minAge = d
			}
			tenant = strings.ToLower(strings.TrimSpace(tenant))
			if tenant != "" && !exoTenantRe.MatchString(tenant) {
				return usageErr(fmt.Errorf("--tenant must be a tenant ID (GUID) or domain"))
			}
			got, err := readSnapshots(cmd.Context(), tenant, "quarantine_meta", "quarantine_request")
			if err != nil {
				return err
			}
			metas, rows := got["quarantine_meta"], got["quarantine_request"]
			now := time.Now().UTC()
			v := quarantineQueueView{Queue: []queueItem{}, Senders: []senderGroup{}, Snapshots: []queueSnapshot{}, OlderThan: olderThan}
			snaps := map[string]*queueSnapshot{}
			for _, m := range metas {
				n, _ := m.Data["requests"].(float64)
				snaps[m.Tenant] = &queueSnapshot{Tenant: m.Tenant, FetchedAt: m.FetchedAt.Format(time.RFC3339), Requests: int(n)}
				if flags.maxAge > 0 && now.Sub(m.FetchedAt) > flags.maxAge {
					fmt.Fprintf(cmd.ErrOrStderr(), "hint: quarantine snapshot for %s is %s old; run 'exchange-online-cli quarantine snapshot %s'\n",
						m.Tenant, now.Sub(m.FetchedAt).Round(time.Minute), m.Tenant)
				}
			}
			senders := map[string]*senderGroup{}
			for _, s := range rows {
				sn := snaps[s.Tenant]
				if sn == nil { // rows without a meta row: count them anyway
					sn = &queueSnapshot{Tenant: s.Tenant, FetchedAt: s.FetchedAt.Format(time.RFC3339)}
					snaps[s.Tenant] = sn
				}
				r := s.Data
				recv, ok := parseExoTime(rs(r, "ReceivedTime"))
				it := queueItem{Tenant: s.Tenant, Identity: rs(r, "Identity"), Received: rs(r, "ReceivedTime"), Sender: smtpOf(rs(r, "SenderAddress")),
					Recipients: rl(r, "RecipientAddress"), Subject: rs(r, "Subject"), Type: rs(r, "Type"), Policy: rs(r, "PolicyName")}
				it.AdminOnly = adminOnlyQuarantine(it.Type)
				if ok {
					it.receivedAt = recv
					age := now.Sub(recv)
					if age < minAge {
						v.Hidden++
						continue
					}
					it.AgeHours = int(age.Hours())
				} else if minAge > 0 {
					v.Hidden++
					continue
				}
				sn.Shown++
				if exp, ok := parseExoTime(rs(r, "Expires")); ok {
					d := int(exp.Sub(now).Hours() / 24)
					it.DaysToExpiry = &d
				}
				v.Queue = append(v.Queue, it)
				g := senders[it.Sender]
				if g == nil {
					g = &senderGroup{Sender: it.Sender}
					senders[it.Sender] = g
				}
				g.Messages++
				if !containsStr(g.Tenants, it.Tenant) {
					g.Tenants = append(g.Tenants, it.Tenant)
				}
			}
			// Oldest first by the real timestamp; unparseable times sort last.
			sort.SliceStable(v.Queue, func(i, j int) bool {
				a, b := v.Queue[i].receivedAt, v.Queue[j].receivedAt
				if a.IsZero() != b.IsZero() {
					return b.IsZero()
				}
				return a.Before(b)
			})
			for _, g := range senders {
				if g.Sender != "" && (g.Messages >= 3 || len(g.Tenants) >= 2) {
					sort.Strings(g.Tenants)
					v.Senders = append(v.Senders, *g)
				}
			}
			sort.Slice(v.Senders, func(i, j int) bool { return v.Senders[i].Messages > v.Senders[j].Messages })
			for _, sn := range snaps {
				v.Snapshots = append(v.Snapshots, *sn)
			}
			sort.Slice(v.Snapshots, func(i, j int) bool { return v.Snapshots[i].Tenant < v.Snapshots[j].Tenant })
			switch {
			case len(snaps) == 0 && tenant != "":
				v.Note = "No snapshot stored for " + tenant + ". Run 'exchange-online-cli quarantine snapshot " + tenant + "' first."
			case len(snaps) == 0:
				v.Note = "No snapshots stored. Run 'exchange-online-cli quarantine snapshot' with each tenant ID first."
			case v.Hidden > 0:
				v.Note = fmt.Sprintf("%d request(s) hidden by --older-than %s.", v.Hidden, olderThan)
			}
			if len(snaps) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), v.Note)
			}
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Only this tenant's requests")
	cmd.Flags().StringVar(&olderThan, "older-than", "", "Only requests older than this, e.g. 24h or 3d")
	return cmd
}
