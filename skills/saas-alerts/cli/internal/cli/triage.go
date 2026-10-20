// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source local
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type saasTriageGroup struct {
	CustomerID   string `json:"customer_id"`
	CustomerName string `json:"customer_name"`
	// User is a display label (an email when the events carry one, else the
	// user name). AccountID is the grouping identity: the stable account id
	// (user id, UserId, or lowercased email), empty for the explicit
	// "unknown account" bucket of events that name no stable account.
	User           string `json:"user"`
	AccountID      string `json:"account_id"`
	UnknownAccount bool   `json:"unknown_account"`
	// Product is the product name (else id). A raw account id is only unique
	// per product, so the product is part of the group identity.
	Product           string   `json:"product"`
	Severity          string   `json:"severity"`
	JointType         string   `json:"joint_type"`
	JointDesc         string   `json:"joint_desc"`
	Count             int      `json:"count"`
	FirstSeen         string   `json:"first_seen"`
	LastSeen          string   `json:"last_seen"`
	DistinctIPs       []string `json:"distinct_ips"`
	Countries         []string `json:"countries"`
	Tickets           []string `json:"tickets"`
	NoTicket          bool     `json:"no_ticket"`
	MaxPullLagSeconds *int64   `json:"max_pull_lag_seconds"`

	rank     int
	first    time.Time
	last     time.Time
	ips      map[string]bool
	country  map[string]bool
	maxLag   *float64
	tickets  map[string]bool
	anyTkt   bool
	userSort string
}

type saasTriageTotals struct {
	Critical int `json:"critical"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	// Other counts events whose alertStatus is not low|medium|critical.
	// Omitted when zero so the documented shape stays exact.
	Other int `json:"other,omitempty"`
}

type saasTriageResult struct {
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	MinSeverity string `json:"min_severity"`
	// Synced is false when sync_state has never recorded events; Note then
	// says how to sync so an empty digest is not read as all-clear.
	Synced        bool              `json:"synced"`
	Note          string            `json:"note,omitempty"`
	ScannedEvents int               `json:"scanned_events"`
	TotalGroups   int               `json:"total_groups"`
	Groups        []saasTriageGroup `json:"groups"`
	// WindowTotalsBySeverity counts every deduped event in the window (after
	// --customer) before --min-severity and --limit are applied.
	WindowTotalsBySeverity saasTriageTotals `json:"window_totals_by_severity"`
}

const saasTriageSyncResources = "customers,events"

// saasTriageUnknownAccount labels the per-customer, per-event-type bucket for
// events with no stable account id.
const saasTriageUnknownAccount = "unknown account"

var saasTriageKeep = []string{"customer_id", "customer_name", "user", "account_id", "unknown_account", "product", "severity", "joint_type", "joint_desc", "count", "first_seen", "last_seen", "distinct_ips", "countries", "tickets", "no_ticket", "max_pull_lag_seconds"}

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagMinSeverity string
	var flagCustomer string
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "See what fired across every customer in a window, critical first, grouped by customer and user",
		Long: "Use this command for a severity-ordered digest of what fired across all customers in a window. Do NOT use this command for raw filtered event rows; use 'events list' instead. Do NOT use it for one account's history; use 'investigate' instead.\n\n" +
			"Reads the local store only: run 'saas-alerts-cli sync --resources customers,events' first. " +
			"Groups are per customer, product, account and event type. The account is the stable account id (user id, else UserId, else the lowercased userEmail or email, else an email-shaped user name); user is only a display label, and events that name no stable account are grouped as 'unknown account'. " +
			"window_totals_by_severity counts every event in the whole window before --min-severity and --limit are applied; groups honor both. " +
			"synced is false (with a note) when events were never synced, so an empty digest is not an all-clear.",
		Example: strings.Trim(`
  saas-alerts-cli triage --since 24h --min-severity medium --agent
  saas-alerts-cli triage --since 7d --customer 6FxMluNmiTljhF4x3tZO --limit 20
  saas-alerts-cli triage --since 24h --select groups.customer_name,groups.user,groups.count --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--since=24h"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "triage")
			}
			since, err := saasParseWindow("since", flagSince)
			if err != nil {
				return err
			}
			minSev := saasNormalizeSeverity(flagMinSeverity)
			minRank := saasSeverityRank(minSev)
			if minRank == 0 {
				return usageErr(fmt.Errorf("invalid --min-severity %q: use low, medium or critical", flagMinSeverity))
			}
			if flagLimit < 0 {
				return usageErr(fmt.Errorf("--limit must be 0 (no limit) or greater, got %d", flagLimit))
			}
			now := saasLocalNow().UTC()
			start := now.Add(-since)
			result := saasTriageResult{
				WindowStart: saasFormatTime(start),
				WindowEnd:   saasFormatTime(now),
				MinSeverity: minSev,
				Groups:      make([]saasTriageGroup, 0),
				Note:        saasUnsyncedNote([]string{"events"}, saasTriageSyncResources),
			}

			dbPath = saasResolveDBPath(dbPath)
			if handled, err := saasLocalMirrorMissing(cmd, flags, dbPath, saasTriageSyncResources, result, saasTriageKeep); handled {
				return err
			}
			db, err := saasOpenLocalStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			events, err := saasLoadLocalEvents(cmd.Context(), db, saasEventQuery{Start: start, End: now})
			if err != nil {
				return err
			}
			saasLocalSyncHints(cmd, db, flags, "events")
			unsynced := saasUnsyncedResources(db, "events")
			result.Synced = len(unsynced) == 0
			result.Note = saasUnsyncedNote(unsynced, saasTriageSyncResources)
			saasBuildTriage(&result, events, minRank, flagCustomer, flagLimit)

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				return saasRenderTriageHuman(cmd, result)
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), result, flags, saasTriageKeep...)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Look-back window (24h, 7d, 1w)")
	cmd.Flags().StringVar(&flagMinSeverity, "min-severity", "low", "Lowest severity to list: low, medium or critical (window_totals_by_severity still counts every severity)")
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Only this customer (customer id, or exact customer name, case-insensitive)")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum groups to return (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// saasBuildTriage groups windowed events by customer -> user -> jointDesc.
// scanned_events counts every deduped event in the window;
// window_totals_by_severity counts events after --customer but before
// --min-severity and --limit.
func saasBuildTriage(result *saasTriageResult, events []saasLocalEvent, minRank int, customerFilter string, limit int) {
	result.ScannedEvents = len(events)
	customerFilter = strings.TrimSpace(customerFilter)
	groups := map[string]*saasTriageGroup{}
	for _, ev := range events {
		if customerFilter != "" && ev.CustomerID != customerFilter && !strings.EqualFold(strings.TrimSpace(ev.CustomerName), customerFilter) {
			continue
		}
		rank := saasSeverityRank(ev.Severity)
		switch ev.Severity {
		case "critical":
			result.WindowTotalsBySeverity.Critical++
		case "medium":
			result.WindowTotalsBySeverity.Medium++
		case "low":
			result.WindowTotalsBySeverity.Low++
		default:
			result.WindowTotalsBySeverity.Other++
		}
		// Unrecognized severities rank 0 and are listed only at --min-severity low.
		if rank < minRank && !(rank == 0 && minRank == 1) {
			continue
		}
		desc := ev.JointDesc
		if desc == "" {
			desc = ev.JointType
		}
		custKey := ev.CustomerID
		if custKey == "" {
			custKey = "name:" + ev.CustomerName
		}
		// Group by the stable account id inside the customer; a display name
		// never merges two accounts. No stable id: the unknown-account bucket.
		accountKey, label := "a:"+ev.AccountID, ev.AccountLabel
		if ev.AccountID == "" {
			accountKey, label = "unknown", saasTriageUnknownAccount
		}
		// Product identity (id, else name) is part of the account: Microsoft
		// and Google accounts can share a raw id inside one customer.
		productKey := ev.ProductID
		if productKey == "" {
			productKey = "name:" + strings.ToLower(ev.ProductName)
		}
		productLabel := ev.ProductName
		if productLabel == "" {
			productLabel = ev.ProductID
		}
		key := custKey + "\x00" + productKey + "\x00" + accountKey + "\x00" + desc
		g, ok := groups[key]
		if !ok {
			g = &saasTriageGroup{
				CustomerID:     ev.CustomerID,
				CustomerName:   ev.CustomerName,
				User:           label,
				AccountID:      ev.AccountID,
				UnknownAccount: ev.AccountID == "",
				Product:        productLabel,
				JointType:      ev.JointType,
				JointDesc:      desc,
				first:          ev.Time,
				last:           ev.Time,
				rank:           -1,
				ips:            map[string]bool{},
				country:        map[string]bool{},
				tickets:        map[string]bool{},
				userSort:       strings.ToLower(label),
			}
			groups[key] = g
		}
		// Prefer an email label once any event of this account carries one.
		if !g.UnknownAccount && !saasEmailShaped(g.User) && len(ev.AccountEmails) > 0 {
			g.User = ev.AccountEmails[0]
			g.userSort = strings.ToLower(g.User)
		}
		g.Count++
		if rank > g.rank {
			g.rank = rank
			g.Severity = ev.Severity
		}
		if ev.Time.Before(g.first) {
			g.first = ev.Time
		}
		if ev.Time.After(g.last) {
			g.last = ev.Time
		}
		if g.CustomerName == "" {
			g.CustomerName = ev.CustomerName
		}
		if ev.IP != "" {
			g.ips[ev.IP] = true
		}
		if ev.Country != "" {
			g.country[ev.Country] = true
		}
		for _, tk := range ev.Tickets {
			g.tickets[tk] = true
		}
		g.anyTkt = g.anyTkt || ev.HasTicket
		if ev.PullLag != nil && (g.maxLag == nil || *ev.PullLag > *g.maxLag) {
			lag := *ev.PullLag
			g.maxLag = &lag
		}
	}
	list := make([]saasTriageGroup, 0, len(groups))
	for _, g := range groups {
		g.FirstSeen = saasFormatTime(g.first)
		g.LastSeen = saasFormatTime(g.last)
		g.DistinctIPs = saasSortedKeys(g.ips)
		g.Countries = saasSortedKeys(g.country)
		g.Tickets = saasSortedKeys(g.tickets)
		g.NoTicket = !g.anyTkt
		g.MaxPullLagSeconds = saasLagSeconds(g.maxLag)
		if g.Severity == "" {
			g.Severity = "unknown"
		}
		list = append(list, *g)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.rank != b.rank {
			return a.rank > b.rank
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if !a.last.Equal(b.last) {
			return a.last.After(b.last)
		}
		if a.CustomerName != b.CustomerName {
			return a.CustomerName < b.CustomerName
		}
		if a.userSort != b.userSort {
			return a.userSort < b.userSort
		}
		if a.JointDesc != b.JointDesc {
			return a.JointDesc < b.JointDesc
		}
		if a.Product != b.Product {
			return a.Product < b.Product
		}
		return a.AccountID < b.AccountID
	})
	result.TotalGroups = len(list)
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	result.Groups = list
}

func saasRenderTriageHuman(cmd *cobra.Command, r saasTriageResult) error {
	out := cmd.OutOrStdout()
	if r.Note != "" {
		fmt.Fprintf(out, "note: %s\n", r.Note)
	}
	fmt.Fprintf(out, "Window %s .. %s: %d events (critical %d, medium %d, low %d); listing %s and above\n",
		r.WindowStart, r.WindowEnd, r.ScannedEvents, r.WindowTotalsBySeverity.Critical, r.WindowTotalsBySeverity.Medium, r.WindowTotalsBySeverity.Low, r.MinSeverity)
	if len(r.Groups) == 0 {
		fmt.Fprintln(out, "No matching alerts in this window.")
		return nil
	}
	tw := newTabWriter(out)
	fmt.Fprintln(tw, "SEVERITY\tCUSTOMER\tUSER\tPRODUCT\tEVENT\tCOUNT\tLAST SEEN\tIPS\tTICKETS")
	for _, g := range r.Groups {
		tickets := strings.Join(g.Tickets, ",")
		if g.NoTicket {
			tickets = "none"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\t%s\n",
			g.Severity, truncate(g.CustomerName, 28), truncate(g.User, 32), truncate(g.Product, 20), truncate(g.JointDesc, 44), g.Count, g.LastSeen, len(g.DistinctIPs), tickets)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if r.TotalGroups > len(r.Groups) {
		fmt.Fprintf(out, "showing %d of %d groups (raise --limit for more)\n", len(r.Groups), r.TotalGroups)
	}
	return nil
}
