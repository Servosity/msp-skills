// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// seatMetric numbers are nil (JSON null, empty CSV cell) when unknown.
type seatMetric struct {
	Count  *float64 `json:"count"`
	Limit  *float64 `json:"limit"` // 0 = no limit configured (status "unlimited")
	Pct    *float64 `json:"pct"`   // count/limit*100, one decimal; nil when unlimited or unknown
	Status string   `json:"status"`
	Source string   `json:"source"` // billing | local | unavailable
}

type seatRow struct {
	Domain               string                `json:"domain"`
	Reseller             string                `json:"reseller,omitempty"`
	Description          string                `json:"description,omitempty"`
	Status               string                `json:"status"`
	Metrics              map[string]seatMetric `json:"metrics"`
	DevicesRegistered    *float64              `json:"devices_registered"`      // nil (null) when billing lacks the field
	DevicesTotal         *float64              `json:"devices_total"`           // nil (null) when billing lacks the field
	PhoneNumbers         *float64              `json:"phone_numbers"`           // nil (null) when billing lacks the field
	TollFree             *float64              `json:"toll_free_numbers"`       // nil (null) when billing lacks the field
	OffnetCallsLastMonth *float64              `json:"offnet_calls_last_month"` // nil (null) when billing lacks the field
	SMSSentLastMonth     *float64              `json:"sms_sent_last_month"`     // nil (null) when billing lacks the field
	SMSReceivedLastMonth *float64              `json:"sms_received_last_month"` // nil (null) when billing lacks the field
	HasBilling           *bool                 `json:"has_billing"`             // nil when billing did not complete a sync
}

var seatStatusRank = map[string]int{"over": 3, "near": 2, "ok": 1, "unlimited": 0, "unknown": -1}

// seatMetricFor compares a count against its limit. A missing or 0 limit means
// NetSapiens enforces none, which is reported as "unlimited", never as over.
func seatMetricFor(count, limit, near float64, source string) seatMetric {
	m := seatMetric{Count: &count, Limit: &limit, Source: source, Status: "unlimited"}
	if limit > 0 {
		pct := math.Round(count/limit*1000) / 10 // one decimal
		m.Pct = &pct
		switch {
		case count > limit:
			m.Status = "over"
		case count >= near*limit:
			m.Status = "near"
		default:
			m.Status = "ok"
		}
	}
	return m
}

// seatCoverage says which local tables completed a sync; an uncovered table
// cannot stand in for a missing billing count (it would read as zero).
type seatCoverage struct{ billing, users, callqueues, autoattendants bool }

func buildSeatRows(domains, billing, users, queues, attendants []fleetRow, near float64, cov seatCoverage) []seatRow {
	billingKnown := cov.billing
	billByDomain := map[string]fleetRow{}
	for _, b := range billing {
		billByDomain[b.domainOf()] = b
	}
	localCount := func(rows []fleetRow) map[string]float64 {
		m := map[string]float64{}
		for _, r := range rows {
			m[r.domainOf()]++
		}
		return m
	}
	nUsers, nQueues, nAAs := localCount(users), localCount(queues), localCount(attendants)

	out := make([]seatRow, 0, len(domains))
	for _, d := range domains {
		name := d.fstr("domain")
		if name == "" {
			continue
		}
		lk := name
		b, hasBill := billByDomain[lk]
		if b == nil {
			b = fleetRow{}
		}
		// count: billing (v46 kebab, then legacy snake) else the locally synced rows.
		metric := func(countKeys []string, local map[string]float64, covered bool, limitKeys []string) seatMetric {
			var count *float64
			source := "unavailable"
			if v, ok := b.fnum(countKeys...); ok {
				count, source = &v, "billing"
			} else if covered {
				v := local[lk]
				count, source = &v, "local"
			}
			// billing's limit wins, including an explicit 0 (unlimited); the domain
			// record is only a fallback when billing omits the field
			limit := b.fnumPtr(limitKeys...)
			if limit == nil {
				limit = d.fnumPtr(limitKeys...)
			}
			if count == nil || limit == nil { // unknown count or no limit field anywhere
				return seatMetric{Count: count, Limit: limit, Source: source, Status: "unknown"}
			}
			return seatMetricFor(*count, *limit, near, source)
		}
		row := seatRow{
			Domain:      name,
			Reseller:    d.fstr("reseller", "territory"),
			Description: d.fstr("description"),
			HasBilling:  optBool(billingKnown, hasBill),
			Metrics: map[string]seatMetric{
				"users":           metric([]string{"count-users-total", "sub_count"}, nUsers, cov.users, []string{"limits-max-users", "limits-max-users-toal", "max_user"}),
				"call_queues":     metric([]string{"count-call-queues"}, nQueues, cov.callqueues, []string{"limits-max-call-queues", "max_call_queue"}),
				"auto_attendants": metric([]string{"count-auto-attendants"}, nAAs, cov.autoattendants, []string{"limits-max-auto-attendants", "max_aa"}),
				"conferences":     metric([]string{"count-conferences"}, nil, false, []string{"limits-max-conferences", "max_conference"}), // no local conference table
			},
		}
		row.DevicesRegistered = b.fnumPtr("count-devices-registered")
		row.DevicesTotal = b.fnumPtr("count-devices-total")
		row.PhoneNumbers = b.fnumPtr("count-phone-numbers")
		row.TollFree = b.fnumPtr("count-phone-numbers-toll-free")
		row.OffnetCallsLastMonth = b.fnumPtr("active-calls-offnet-last-month")
		row.SMSSentLastMonth = b.fnumPtr("sms-sent-last-month")
		row.SMSReceivedLastMonth = b.fnumPtr("sms-received-last-month")
		row.Status = "unknown" // the worst known metric wins below
		for _, m := range row.Metrics {
			if seatStatusRank[m.Status] > seatStatusRank[row.Status] {
				row.Status = m.Status
			}
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := seatStatusRank[out[i].Status], seatStatusRank[out[j].Status]; a != b {
			return a > b
		}
		if a, b := pctOf(out[i].Metrics["users"]), pctOf(out[j].Metrics["users"]); a != b {
			return a > b
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}

// seatReport is the machine result: one row per domain plus whether the
// domain list and billing summaries behind them were fully synced.
type seatReport struct {
	auditCoverage
	Domains []seatRow `json:"domains"`
}

func newNovelFleetSeatsCmd(flags *rootFlags) *cobra.Command {
	var dbPath, domain string
	var near float64
	var flagged bool
	cmd := &cobra.Command{
		Use:   "seats",
		Short: "Per-client users, queues, attendants and conferences against limits, flagging over and near-limit domains",
		Long: `Use this command for current per-client counts against limits and billable usage (who is over or near a limit, true-up CSV).
Do NOT use this command to find what was added or removed since a previous check; use 'fleet changes' instead.

Counts come from each domain's synced billing summary, falling back to locally synced users,
call queues and auto attendants. A missing or zero limit means none is configured ("unlimited").`,
		Example: strings.Trim(`
  netsapiens-cli fleet seats --agent
  netsapiens-cli fleet seats --flagged --near 0.8 --select domains.domain,domains.status
  netsapiens-cli fleet seats --csv --select domain,status,users_count,users_limit`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet seats")
			}
			if near <= 0 || near > 1 {
				return usageErr(fmt.Errorf("--near must be a fraction between 0 and 1, got %v", near))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openFleetStore(ctx, cmd, flags, dbPath, seatReport{auditCoverage: noMirrorCoverage(), Domains: []seatRow{}})
			if db == nil || err != nil {
				return err
			}
			defer db.Close()
			hintCoverage(cmd, db, rtDomains, rtBilling)
			cov := fleetCov{}
			load := func(rt string) []fleetRow {
				if err != nil {
					return nil
				}
				var rows []fleetRow
				rows, err = loadCovered(ctx, db, cov, rt)
				return filterDomain(rows, domain)
			}
			domains, billing, users, queues, aas := load(rtDomains), load(rtBilling), load(rtUsers), load(rtCallqueues), load(rtAutoattendants)
			if err != nil {
				return err
			}
			rows := buildSeatRows(domains, billing, users, queues, aas, near, seatCoverage{billing: cov[rtBilling], users: cov[rtUsers], callqueues: cov[rtCallqueues], autoattendants: cov[rtAutoattendants]})
			if flagged {
				kept := rows[:0]
				for _, r := range rows {
					if r.Status == "over" || r.Status == "near" {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			ac := fleetAuditCoverage(cmd, flags, db, cov, []string{rtDomains}, []string{rtBilling})
			if flags.csv { // one flat row per domain so the true-up CSV has a column per count and limit
				return printFleetAudit(cmd, flags, ac, nil, flatSeatRows(rows))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printFleetAudit(cmd, flags, ac, seatReport{auditCoverage: ac, Domains: rows}, rows)
			}
			printIncomplete(cmd, ac)
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No domains in the local mirror match. Run: "+fleetSyncHint)
				return nil
			}
			table := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				u := r.Metrics["users"]
				table = append(table, map[string]any{
					"domain": r.Domain, "status": r.Status,
					"users":       numText(u.Count) + "/" + limitText(u.Limit),
					"queues":      numText(r.Metrics["call_queues"].Count) + "/" + limitText(r.Metrics["call_queues"].Limit),
					"devices_reg": numText(r.DevicesRegistered) + "/" + numText(r.DevicesTotal),
					"dids":        numText(r.PhoneNumbers),
				})
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI data directory)")
	cmd.Flags().StringVar(&domain, "domain", "", "Only this client domain (for example acme.example)")
	cmd.Flags().Float64Var(&near, "near", 0.9, "Fraction of a limit that counts as near-limit")
	cmd.Flags().BoolVar(&flagged, "flagged", false, "Only domains that are over or near a limit")
	return cmd
}

func limitText(l *float64) string {
	switch {
	case l == nil:
		return "?"
	case *l <= 0:
		return "-"
	}
	return fmt.Sprintf("%.0f", *l)
}

func pctOf(m seatMetric) float64 {
	if m.Pct == nil {
		return -1
	}
	return *m.Pct
}

// flatSeatRows spreads each metric into <metric>_count/_limit/_status columns.
func flatSeatRows(rows []seatRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		m := map[string]any{
			"domain": r.Domain, "reseller": r.Reseller, "status": r.Status,
			"devices_registered": r.DevicesRegistered, "devices_total": r.DevicesTotal,
			"phone_numbers": r.PhoneNumbers, "toll_free_numbers": r.TollFree,
			"offnet_calls_last_month": r.OffnetCallsLastMonth,
			"sms_sent_last_month":     r.SMSSentLastMonth, "sms_received_last_month": r.SMSReceivedLastMonth,
		}
		for name, mt := range r.Metrics {
			m[name+"_count"], m[name+"_limit"], m[name+"_status"] = mt.Count, mt.Limit, mt.Status // nil -> empty cell
		}
		out = append(out, m)
	}
	return out
}

func numText(v *float64) string {
	if v == nil {
		return "?"
	}
	return fmt.Sprintf("%.0f", *v)
}
