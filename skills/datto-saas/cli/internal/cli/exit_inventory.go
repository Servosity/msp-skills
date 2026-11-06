// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"datto-saas-pp-cli/internal/client"
)

type invDomain struct {
	Domain           string   `json:"domain"`
	ProductType      string   `json:"product_type"`
	SubscriptionID   string   `json:"external_subscription_id"`
	RetentionType    string   `json:"retention_type"`
	SeatsUsed        *float64 `json:"seats_used"`
	ActiveServices   *float64 `json:"active_services"`
	RecentBackup     *float64 `json:"recent_backup_services"`
	BackupPercentage *float64 `json:"backup_percentage"`
}

type invSeat struct {
	Name      string `json:"name"`
	MainID    string `json:"main_id"`
	RemoteID  string `json:"remote_id"`
	SeatType  string `json:"seat_type"`
	SeatState string `json:"seat_state"`
	Billable  *bool  `json:"billable"`
	DateAdded string `json:"date_added"`
}

type invBlocker struct {
	Kind   string   `json:"kind"`
	Count  int      `json:"count"`
	Detail string   `json:"detail"`
	Items  []string `json:"items,omitempty"`
}

type exitInventory struct {
	Source      string `json:"source"`
	GeneratedAt string `json:"generated_at"`
	Customer    struct {
		SaasCustomerID   string `json:"saas_customer_id"`
		Name             string `json:"name"`
		OrganizationName string `json:"organization_name,omitempty"`
	} `json:"customer"`
	Domains        []invDomain `json:"domains"`
	Subscriptions  []string    `json:"external_subscription_ids"`
	RetentionTypes []string    `json:"retention_types"`
	SeatTotals     struct {
		Total           int            `json:"total"`
		ByState         map[string]int `json:"by_state"`
		ByType          map[string]int `json:"by_type"`
		Billable        int            `json:"billable"`
		BillableUnknown int            `json:"billable_unknown"`
		WithoutRemoteID int            `json:"without_remote_id"`
	} `json:"seat_totals"`
	Seats   []invSeat `json:"seats"`
	Storage struct {
		UsedBytes                 *float64        `json:"used_bytes"`
		UsedBytesStatus           string          `json:"used_bytes_status"`
		UsedBytesPartial          *float64        `json:"used_bytes_partial,omitempty"`
		DetailedBackupStatsStatus string          `json:"detailed_backup_stats_status"`
		DetailedBackupStats       json.RawMessage `json:"detailed_backup_stats,omitempty"`
	} `json:"storage"`
	Applications  []appHealthRow `json:"applications"`
	Blockers      []invBlocker   `json:"blockers"`
	ReadyToCancel bool           `json:"ready_to_cancel"`
}

// capItems keeps blocker item lists readable; the full list is in seats.
func capItems(items []string) []string {
	if len(items) > 25 {
		return append(items[:25:25], fmt.Sprintf("... and %d more", len(items)-25))
	}
	return items
}

func buildExitInventory(c saasCustomer, seats []saasSeat, apps []saasAppReport, stats json.RawMessage, statsStatus, source string) exitInventory {
	inv := exitInventory{Source: source, GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Domains: make([]invDomain, 0), Seats: make([]invSeat, 0), Applications: make([]appHealthRow, 0), Blockers: make([]invBlocker, 0)}
	inv.Customer.SaasCustomerID, inv.Customer.Name, inv.Customer.OrganizationName = c.ID, c.Name, c.OrgName
	inv.Subscriptions = c.subscriptions()
	inv.RetentionTypes = c.unique(func(d saasDomain) string { return d.RetentionType })
	inv.SeatTotals.ByState, inv.SeatTotals.ByType = map[string]int{}, map[string]int{}

	var missingSub, noRecent, noStats int
	for _, d := range c.Domains {
		inv.Domains = append(inv.Domains, invDomain{Domain: d.Domain, ProductType: d.ProductType, SubscriptionID: d.SubscriptionID,
			RetentionType: d.RetentionType, SeatsUsed: d.SeatsUsed.ptr(), ActiveServices: d.BackupStats.Active.ptr(),
			RecentBackup: d.BackupStats.Recent.ptr(), BackupPercentage: d.BackupStats.Percent.ptr()})
		if strings.TrimSpace(d.SubscriptionID) == "" {
			missingSub++
		}
		a, r := d.BackupStats.Active, d.BackupStats.Recent
		switch {
		case !validPair(a, r):
			noStats++
		case a.v > r.v:
			noRecent += int(a.v - r.v)
		}
	}

	var unprotected, paused, noRemote []string
	for _, s := range seats {
		b := s.billable()
		inv.Seats = append(inv.Seats, invSeat{Name: s.Name, MainID: s.MainID, RemoteID: s.RemoteID, SeatType: s.SeatType,
			SeatState: s.SeatState, Billable: b, DateAdded: string(s.DateAdded)})
		inv.SeatTotals.Total++
		inv.SeatTotals.ByState[s.SeatState]++
		inv.SeatTotals.ByType[s.SeatType]++
		switch {
		case b == nil:
			inv.SeatTotals.BillableUnknown++
		case *b:
			inv.SeatTotals.Billable++
		}
		label := s.MainID
		if label == "" {
			label = s.Name
		}
		switch s.SeatState {
		case "Unprotected":
			unprotected = append(unprotected, label)
		case "Paused":
			paused = append(paused, label)
		}
		if strings.TrimSpace(s.RemoteID) == "" {
			noRemote = append(noRemote, label)
			inv.SeatTotals.WithoutRemoteID++
		}
	}
	sort.SliceStable(inv.Seats, func(a, b int) bool { return inv.Seats[a].MainID < inv.Seats[b].MainID })

	var used flexNum
	var incomplete, unusable []string
	rows, missingUsed, emptyReports := 0, 0, 0
	for _, a := range apps {
		if len(appHealth(a)) == 0 {
			emptyReports++
		}
		if a.UsedBytes.ok && a.UsedBytes.v >= 0 {
			used = flexNum{v: used.v + a.UsedBytes.v, ok: true}
		} else {
			missingUsed++
		}
		for _, row := range appHealth(a) {
			rows++
			inv.Applications = append(inv.Applications, row)
			switch {
			case !row.CountsComplete:
				unusable = append(unusable, row.AppType)
			case *row.WithBackup < *row.ActiveServices:
				incomplete = append(incomplete, row.AppType)
			}
		}
	}
	// A partial sum is never presented as the total.
	switch {
	case len(apps) == 0:
		inv.Storage.UsedBytesStatus = "no_application_report"
	case missingUsed == 0:
		inv.Storage.UsedBytes, inv.Storage.UsedBytesStatus = used.ptr(), "complete"
	default:
		inv.Storage.UsedBytesPartial = used.ptr()
		inv.Storage.UsedBytesStatus = fmt.Sprintf("incomplete: %d of %d backup reports lack usedBytes", missingUsed, len(apps))
	}
	inv.Storage.DetailedBackupStatsStatus = statsStatus
	inv.Storage.DetailedBackupStats = stats

	add := func(kind, detail string, items []string, count int) {
		if count > 0 {
			inv.Blockers = append(inv.Blockers, invBlocker{Kind: kind, Count: count, Detail: detail, Items: capItems(items)})
		}
	}
	add("unprotected_seats", "seats with no backups; they hold nothing to export", unprotected, len(unprotected))
	add("paused_seats", "backups stopped; data after the pause is not protected", paused, len(paused))
	add("services_without_recent_backup", "active services without a recent backup, from domain backupStats", nil, noRecent)
	add("apps_latest_window_incomplete", "apps whose latest backup window did not cover every active service", incomplete, len(incomplete))
	add("apps_without_usable_history", "apps with no backup history or with missing or inconsistent latest-window counts", unusable, len(unusable))
	add("seats_without_remote_id", "seats that cannot be targeted by bulkSeatChange", noRemote, len(noRemote))
	add("missing_subscription_id", "domains without an externalSubscriptionId", nil, missingSub)
	add("backup_stats_missing", "domains without usable backupStats counts, so coverage cannot be confirmed", nil, noStats)
	if len(seats) == 0 {
		add("no_seats", "no seats found for this client; sync or check the client id", nil, 1)
	}
	if len(apps) == 0 {
		add("no_application_report", "no backup report found for this client", nil, 1)
	} else if rows == 0 || emptyReports > 0 {
		add("no_backup_history", "a backup report lists no applications, so recent backups cannot be confirmed", nil, max(emptyReports, 1))
	}
	inv.ReadyToCancel = len(inv.Blockers) == 0
	return inv
}

// decodeAppReports accepts the {pagination, items} envelope (S4) or a bare
// array. morePages is true when the envelope says pages beyond the first
// exist: no public source names this endpoint's paging parameters, so the
// caller reports the inventory as incomplete instead of guessing them.
func decodeAppReports(raw json.RawMessage, customerID string) (list []saasAppReport, morePages bool, err error) {
	var env struct {
		Pagination struct {
			Page       flexNum `json:"page"`
			TotalPages flexNum `json:"totalPages"`
		} `json:"pagination"`
		Items []saasAppReport `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, false, fmt.Errorf("decode applications: %w", err)
		}
		list = env.Items
		page := 1.0
		if validCount(env.Pagination.Page) && env.Pagination.Page.v > 0 {
			page = env.Pagination.Page.v
		}
		morePages = validCount(env.Pagination.TotalPages) && env.Pagination.TotalPages.v > page
	}
	for i := range list {
		list[i].parentID = customerID
	}
	return list, morePages, nil
}

// isTransportFailure reports a request that got no HTTP response at all: a
// *url.Error from http.Client.Do that wraps a net.Error (dial, DNS,
// connection refused or reset, header timeout). A *url.Error that wraps a
// redirect-policy error, a bare net.Error or deadline from reading an error
// response's body (a stalled 401), HTTP errors (APIError), CLI errors and
// decode errors never qualify, whatever their text.
func isTransportFailure(err error) bool {
	var apiErr *client.APIError
	var ce *cliError
	if errors.As(err, &apiErr) || errors.As(err, &ce) {
		return false
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return false
	}
	var netErr net.Error
	return errors.As(urlErr.Err, &netErr)
}

func liveExitInventory(cmd *cobra.Command, flags *rootFlags, arg string) (exitInventory, error) {
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()
	c, err := flags.newClient()
	if err != nil {
		return exitInventory{}, err
	}
	// Uncached reads: a manifest marked live must reflect the API right now.
	raw, err := c.GetNoCache(ctx, "/saas/domains", nil)
	if err != nil {
		return exitInventory{}, err
	}
	var domains []saasDomain
	if err := json.Unmarshal(raw, &domains); err != nil {
		return exitInventory{}, fmt.Errorf("decode domains: %w", err)
	}
	cust, err := resolveCustomer(groupCustomers(domains), arg)
	if err != nil {
		return exitInventory{}, err
	}
	raw, err = c.GetNoCache(ctx, "/saas/"+cust.ID+"/seats", nil)
	if err != nil {
		return exitInventory{}, err
	}
	var seats []saasSeat
	if err := json.Unmarshal(raw, &seats); err != nil {
		return exitInventory{}, fmt.Errorf("decode seats: %w", err)
	}
	for i := range seats {
		seats[i].CustomerID = cust.ID
	}
	raw, err = c.GetNoCache(ctx, "/saas/"+cust.ID+"/applications", map[string]string{"daysUntil": "30"})
	if err != nil {
		return exitInventory{}, err
	}
	apps, morePages, err := decodeAppReports(raw, cust.ID)
	if err != nil {
		return exitInventory{}, err
	}
	// detailedBackupStats is optional context: a non-auth API error on it is
	// reported in the manifest; 401/403 still fail the command.
	status := "included"
	stats, err := c.GetNoCache(ctx, "/saas/"+cust.ID+"/detailedBackupStats", nil)
	var apiErr *client.APIError
	switch {
	case err == nil && !json.Valid(stats):
		stats, status = nil, "unreadable"
	case errors.As(err, &apiErr) && apiErr.StatusCode != 401 && apiErr.StatusCode != 403:
		stats, status = nil, fmt.Sprintf("error: HTTP %d", apiErr.StatusCode)
	case err != nil:
		return exitInventory{}, err
	}
	inv := buildExitInventory(cust, seats, apps, stats, status, "live")
	if morePages {
		markInventoryIncomplete(&inv, "application_report_paginated",
			"the backup report has more pages than were read; applications and storage are incomplete", nil)
	}
	return inv, nil
}

// markInventoryIncomplete records evidence the manifest is known to be
// missing: it adds a blocker, clears ready_to_cancel, and demotes a storage
// total to a partial sum.
func markInventoryIncomplete(inv *exitInventory, kind, detail string, items []string) {
	inv.Blockers = append(inv.Blockers, invBlocker{Kind: kind, Count: max(len(items), 1), Detail: detail, Items: items})
	inv.ReadyToCancel = false
	if inv.Storage.UsedBytes != nil {
		inv.Storage.UsedBytesPartial, inv.Storage.UsedBytes = inv.Storage.UsedBytes, nil
	}
	inv.Storage.UsedBytesStatus = "incomplete: " + detail
}

func localExitInventory(cmd *cobra.Command, flags *rootFlags, dbPath, arg string) (exitInventory, bool, error) {
	db, missing, err := openSaasStore(cmd, dbPath)
	if err != nil || missing {
		return exitInventory{}, missing, err
	}
	defer db.Close()
	rd, err := beginSaasRead(cmd, db, flags, "domains", "seats", "applications")
	if err != nil {
		return exitInventory{}, false, err
	}
	defer rd.Close()
	domains, err := loadDomains(cmd.Context(), rd.tx)
	if err != nil {
		return exitInventory{}, false, err
	}
	if len(domains) == 0 {
		return exitInventory{}, true, nil
	}
	cust, err := resolveCustomer(groupCustomers(domains), arg)
	if err != nil {
		return exitInventory{}, false, err
	}
	seats, err := loadSeats(cmd.Context(), rd.tx, cust.ID)
	if err != nil {
		return exitInventory{}, false, err
	}
	apps, err := loadApplications(cmd.Context(), rd.tx, cust.ID)
	if err != nil {
		return exitInventory{}, false, err
	}
	inv := buildExitInventory(cust, seats, apps, nil, "not_synced: run with --data-source live to include it", "local")
	// Rows left behind by an interrupted or failed sync look healthy; only
	// sync_state knows the mirror is incomplete.
	if partial := rd.partial; len(partial) > 0 {
		markInventoryIncomplete(&inv, "local_mirror_incomplete",
			"the last sync of these resources did not finish (or never ran); run datto-saas-cli sync", partial)
	}
	return inv, false, nil
}

// saasCredentialState reports whether a usable key pair is configured. Only a
// genuinely absent pair returns (false, nil); configuration errors, refused
// credential files and half-configured pairs are errors, never a silent local
// fallback.
func saasCredentialState(flags *rootFlags) (bool, error) {
	c, err := flags.newClient()
	if err != nil {
		return false, err
	}
	if c == nil || c.Config == nil {
		return false, nil
	}
	cfg := c.Config
	if cfg.AuthHeader() != "" {
		return true, nil
	}
	if len(cfg.CredentialRefusals) > 0 {
		return false, configErr(fmt.Errorf("stored Datto SaaS credentials were refused (check the credentials file permissions; run 'datto-saas-cli doctor')"))
	}
	if cfg.DattoSaasPublicKey != "" || cfg.DattoSaasSecretKey != "" {
		return false, configErr(fmt.Errorf("incomplete Datto SaaS key pair: set both DATTO_SAAS_PUBLIC_KEY and DATTO_SAAS_SECRET_KEY"))
	}
	return false, nil
}

func newNovelExitInventoryCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "exit-inventory <customer>",
		Short: "Build one manifest for a client leaving SaaS Protection: domains, subscription ids, retention, every seat with remoteId",
		Long: strings.Trim(`
Builds one manifest for a client leaving SaaS Protection, from a
saasCustomerId, customer name, or domain: every domain with product,
externalSubscriptionId, retention type, seatsUsed and backup stats; every
seat with name, mainId, remoteId, type, state, billable flag and date added;
seat totals; used bytes from the backup report; per-application latest
backup windows; and a blockers checklist (unprotected or paused seats,
services without a recent backup, incomplete latest windows, seats without a
remoteId). ready_to_cancel is true only when the checklist is empty.

With --data-source auto (the default) the client's data is fetched live and
includes detailedBackupStats; if the API is unreachable it falls back to the
synced store, which does not hold detailedBackupStats. Authentication errors
are never masked by the fallback. --csv prints the seat rows only.

Use this command to build the complete exit manifest for one client leaving SaaS Protection: domains, subscription ids, retention, every seat with remoteId, storage, last backups, and a blockers checklist.
Do NOT use this command for a fleet-wide seat export; use 'seats roster' instead.
Do NOT use it to pause or unlicense that client's seats; use 'seats bulk-plan' instead.`, "\n"),
		Example: strings.Trim(`
  datto-saas-cli exit-inventory 123456 --agent
  datto-saas-cli exit-inventory contoso.com --data-source local --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:happy-args": "customer=123456"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "exit-inventory")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one <customer> is required: a saasCustomerId, customer name, or domain"))
			}
			arg := args[0]
			var inv exitInventory
			var err error
			useLocal := flags.dataSource == "local"
			configured, credErr := saasCredentialState(flags)
			if flags.dataSource != "local" && credErr != nil {
				return credErr
			}
			if flags.dataSource == "auto" && !configured {
				// auto means live when possible; with no key pair configured the
				// only possible answer is the synced store.
				fmt.Fprintln(cmd.ErrOrStderr(), "note: no DATTO_SAAS_PUBLIC_KEY/DATTO_SAAS_SECRET_KEY configured; using the local store")
				useLocal = true
			}
			if !useLocal {
				inv, err = liveExitInventory(cmd, flags, arg)
				// Fall back only on a typed transport failure (no HTTP response).
				if err != nil && (flags.dataSource == "live" || !isTransportFailure(err)) {
					return classifyAPIErrorOnly(err)
				}
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: API unreachable (%v); using the local store\n", err)
				}
			}
			if useLocal || err != nil {
				var empty bool
				inv, empty, err = localExitInventory(cmd, flags, dbPath, arg)
				if err != nil {
					return err
				}
				if empty {
					flags.agentSource = "local"
					if flags.csv {
						return printSaasRows(cmd, flags, make([]invSeat, 0), "No local data. Run: datto-saas-cli sync")
					}
					note := "no local data; run datto-saas-cli sync or use --data-source live"
					if !configured {
						// Without a key pair both sync and live mode get HTTP 401.
						note = "no local data; set DATTO_SAAS_PUBLIC_KEY and DATTO_SAAS_SECRET_KEY, then run datto-saas-cli sync"
					}
					return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"source": "local", "note": note}, flags)
				}
			}
			// The --agent envelope must name where the data actually came from.
			flags.agentSource = inv.Source
			if flags.csv {
				return printSaasRows(cmd, flags, inv.Seats, "No seats.")
			}
			return printJSONFiltered(cmd.OutOrStdout(), inv, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path for the local fallback (default: the CLI's synced store)")
	return cmd
}
