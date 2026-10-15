// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// fleetGap is one security or commercial gap for a tenant.
type fleetGap struct {
	Kind         string `json:"kind"` // security | commercial
	Type         string `json:"type"`
	Detail       string `json:"detail"`
	Count        int    `json:"count,omitempty"`
	Score        *int   `json:"score,omitempty"`
	Snoozed      bool   `json:"snoozed,omitempty"`
	SnoozeExpiry string `json:"snooze_expiry,omitempty"`
}

type tenantGaps struct {
	TenantID   string     `json:"tenant_id"`
	TenantName string     `json:"tenant_name"`
	GapCount   int        `json:"gap_count"`
	Gaps       []fleetGap `json:"gaps"`
}

type gapsView struct {
	Items        []tenantGaps   `json:"items"`
	Total        int            `json:"total"`
	ByType       map[string]int `json:"by_type"`
	BlindTenants []blindTenant  `json:"blind_tenants"`
	SyncedAt     *time.Time     `json:"synced_at"`
	Note         string         `json:"note,omitempty"`
}

type gapOptions struct {
	threshold      int
	kind           string
	includeServers bool
	now            time.Time
}

func newNovelFleetGapsCmd(flags *rootFlags) *cobra.Command {
	var flagThreshold, flagLimit int
	var flagKind, dbPath string
	var flagIncludeServerGap bool

	cmd := &cobra.Command{
		Use:   "gaps",
		Short: "Per-customer security and commercial gaps: low protection, partial tamper protection, risky exclusions",
		Long: `Use this command for a per-tenant list of security and commercial gaps (low protection, partial tamper protection, risky exclusions, unencrypted or unlicensed devices, licence over-usage or expiry). Do NOT use this command to rank tenants by overall health score; use 'fleet health' instead. Do NOT use it to enable tamper protection; use 'fleet tamper' instead.

Security gaps: Account Health Check scores below --threshold, tamper protection off globally, risky scanning exclusions, devices with encryption assigned but not encrypted. Commercial gaps: unlicensed devices, assigned products not installed, licences used beyond their quantity, licences ending within 30 days, and (with --include-server-gap) tenants with no protected servers. Snoozed checks stay listed with their snooze expiry. Reads the store filled by 'fleet refresh'.`,
		Example: strings.Trim(`
  sophos-central-cli fleet gaps --threshold 80 --kind security
  sophos-central-cli fleet gaps --kind commercial --include-server-gap --limit 20 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet gaps")
			}
			switch flagKind {
			case "", "all", "security", "commercial":
			default:
				return usageErr(fmt.Errorf("--kind %q: want security, commercial or all", flagKind))
			}
			if flagThreshold < 0 || flagThreshold > 100 {
				return usageErr(fmt.Errorf("--threshold must be 0-100"))
			}
			if flagLimit <= 0 {
				return usageErr(fmt.Errorf("--limit must be positive"))
			}
			path := fleetDBPath(dbPath)
			db, exists, err := openFleetStore(cmd.Context(), path)
			if err != nil {
				return err
			}
			if !exists {
				fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: sophos-central-cli fleet refresh --db %s\n", path, path)
				if wantsHumanTable(cmd.OutOrStdout(), flags) {
					fmt.Fprintln(cmd.OutOrStdout(), "No fleet refresh has run yet. Run: sophos-central-cli fleet refresh")
					return nil
				}
				return printJSONFiltered(cmd.OutOrStdout(), gapsView{Items: make([]tenantGaps, 0), ByType: map[string]int{}, BlindTenants: make([]blindTenant, 0), Note: fleetNeverRefreshedNote}, flags)
			}
			defer db.Close()
			kind := flagKind
			if kind == "all" {
				kind = ""
			}
			if kind == "commercial" && cmd.Flags().Changed("threshold") {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: --threshold applies to Account Health Check scores, which are security gaps; it has no effect with --kind commercial")
			}
			filter, err := fleetTenantFilter()
			if err != nil {
				return err
			}
			view, err := buildGaps(db, filter, gapOptions{threshold: flagThreshold, kind: kind, includeServers: flagIncludeServerGap, now: time.Now().UTC()})
			if err != nil {
				return err
			}
			view.SyncedAt, view.Note = fleetFreshness(cmd.ErrOrStderr(), loadFleetMeta(db), loadFleetLedgerQuiet(db), filter, flags.maxAge, gapsNeeded(kind)...)
			if len(view.Items) > flagLimit {
				view.Items = view.Items[:flagLimit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			defer warnBlind(cmd.ErrOrStderr(), len(view.BlindTenants))
			if len(view.Items) == 0 {
				if view.SyncedAt == nil {
					fmt.Fprintln(cmd.OutOrStdout(), "No fleet refresh has run yet. Run: sophos-central-cli fleet refresh")
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "No gaps found (fleet data from %s).\n", view.SyncedAt.Format(time.RFC3339))
				}
				return nil
			}
			rows := make([]map[string]any, 0)
			for _, t := range view.Items {
				for _, g := range t.Gaps {
					rows = append(rows, map[string]any{"tenant": t.TenantName, "kind": g.Kind, "type": g.Type, "detail": g.Detail})
				}
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().IntVar(&flagThreshold, "threshold", 80, "Account Health Check score below which a check is a gap (0-100)")
	cmd.Flags().StringVar(&flagKind, "kind", "all", "Gap kind: security, commercial or all")
	cmd.Flags().BoolVar(&flagIncludeServerGap, "include-server-gap", false, "Also flag tenants with no protected servers (upsell signal)")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum tenants to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// endpointTally counts the per-tenant endpoint conditions gaps care about.
type endpointTally struct {
	unencrypted, unlicensed, notInstalled, servers int
}

func buildGaps(db fleetReader, filter map[string]bool, opt gapOptions) (gapsView, error) {
	ledger, err := loadFleetLedger(db)
	if err != nil {
		return gapsView{}, fmt.Errorf("reading fleet ledger: %w", err)
	}
	names := tenantNames(db, ledger)
	unreadable := unreadableRows{}
	gaps := map[string][]fleetGap{}
	add := func(tenantID string, g fleetGap) {
		if tenantID == "" || (filter != nil && !filter[tenantID]) {
			return
		}
		if opt.kind != "" && g.Kind != opt.kind {
			return
		}
		gaps[tenantID] = append(gaps[tenantID], g)
	}

	// Account Health Check: low scores, global tamper off, risky exclusions.
	err = db.ListScan(fleetHealthType, func(_ string, raw json.RawMessage) bool {
		doc, checks, data, derr := tenantHealthChecks(raw)
		if derr != nil || doc.TenantID == "" {
			unreadable.add(raw, "health")
			return true
		}
		if names[doc.TenantID] == "" {
			names[doc.TenantID] = doc.TenantName
		}
		for _, c := range checks {
			if c.Score >= opt.threshold {
				continue
			}
			score := c.Score
			g := fleetGap{Kind: "security", Type: "low_health_score", Detail: fmt.Sprintf("%s scored %d (threshold %d)", c.Check, c.Score, opt.threshold), Score: &score, Snoozed: c.Snoozed, SnoozeExpiry: c.SnoozeExpiry}
			if n := c.Detail["numberOfSecurityRisks"]; n > 0 && strings.Contains(c.Check, "exclusions") {
				g.Type, g.Count = "risky_exclusions", n
				g.Detail = fmt.Sprintf("%d risky scanning exclusion(s) (%s scored %d)", n, c.Check, c.Score)
			}
			add(doc.TenantID, g)
		}
		if enabled, ok := jsonPath(data, "endpoint", "tamperProtection", "globalDetail", "enabled").(bool); ok && !enabled {
			add(doc.TenantID, fleetGap{Kind: "security", Type: "tamper_global_off", Detail: "tamper protection is turned off for the whole tenant"})
		} else if global, ok := jsonPath(data, "endpoint", "tamperProtection", "global").(bool); ok && !global {
			add(doc.TenantID, fleetGap{Kind: "security", Type: "tamper_global_off", Detail: "tamper protection is turned off for the whole tenant"})
		}
		return true
	})
	if err != nil {
		return gapsView{}, fmt.Errorf("reading health checks: %w", err)
	}

	// Endpoints: encryption, licensing, product install state, servers.
	tally := map[string]*endpointTally{}
	err = db.ListScan("endpoints", func(_ string, raw json.RawMessage) bool {
		var e map[string]any
		if json.Unmarshal(raw, &e) != nil {
			unreadable.add(raw, "endpoints")
			return true
		}
		tid := jsonString(e, "tenant", "id")
		t := tally[tid]
		if t == nil {
			t = &endpointTally{}
			tally[tid] = t
		}
		if jsonString(e, "type") == "server" {
			t.servers++
		}
		encryptionAssigned := false
		products, _ := jsonPath(e, "assignedProducts").([]any)
		for _, p := range products {
			pm, _ := p.(map[string]any)
			if strings.Contains(strings.ToLower(jsonString(pm, "code")), "encryption") {
				encryptionAssigned = true
			}
			if jsonString(pm, "status") == "notInstalled" {
				t.notInstalled++
			}
		}
		if encryptionAssigned && jsonString(e, "encryption", "overallStatus") == "notEncrypted" {
			t.unencrypted++
		}
		if jsonString(e, "packages", "protection", "status") == "unlicensed" {
			t.unlicensed++
		}
		return true
	})
	if err != nil {
		return gapsView{}, fmt.Errorf("reading endpoints: %w", err)
	}
	// A tenant whose endpoints were fetched but which has none still gets
	// its server check (no endpoint rows means no tally entry otherwise).
	for tid, row := range ledger {
		if o, ok := row.Resources["endpoints"]; ok && o.Status == "ok" && tally[tid] == nil {
			tally[tid] = &endpointTally{}
		}
	}
	for tid, t := range tally {
		if t.unencrypted > 0 {
			add(tid, fleetGap{Kind: "security", Type: "unencrypted_devices", Count: t.unencrypted, Detail: fmt.Sprintf("%d device(s) have encryption assigned but are not encrypted", t.unencrypted)})
		}
		if t.unlicensed > 0 {
			add(tid, fleetGap{Kind: "commercial", Type: "unlicensed_devices", Count: t.unlicensed, Detail: fmt.Sprintf("%d device(s) report protection as unlicensed", t.unlicensed)})
		}
		if t.notInstalled > 0 {
			add(tid, fleetGap{Kind: "commercial", Type: "products_not_installed", Count: t.notInstalled, Detail: fmt.Sprintf("%d assigned product install(s) not completed", t.notInstalled)})
		}
		if opt.includeServers && t.servers == 0 {
			add(tid, fleetGap{Kind: "commercial", Type: "no_server_protection", Detail: "no protected servers in this tenant"})
		}
	}

	// Licences: over-usage and expiry.
	err = db.ListScan(fleetLicensesType, func(_ string, raw json.RawMessage) bool {
		var doc tenantHealthDoc // same wrapper shape: tenant_id, tenant_name, data
		if json.Unmarshal(raw, &doc) != nil || doc.TenantID == "" {
			unreadable.add(raw, "licences")
			return true
		}
		var lic struct {
			Licenses []*struct {
				Product struct {
					Name string `json:"name"`
					Code string `json:"code"`
				} `json:"product"`
				EndDate   string `json:"endDate"`
				Perpetual bool   `json:"perpetual"`
				Unlimited bool   `json:"unlimited"`
				Quantity  int    `json:"quantity"`
				Usage     struct {
					Current struct {
						Count int `json:"count"`
					} `json:"current"`
				} `json:"usage"`
			} `json:"licenses"`
		}
		// null data or a null licenses list decodes cleanly; [] is a real empty list.
		if json.Unmarshal(doc.Data, &lic) != nil || lic.Licenses == nil || slices.Contains(lic.Licenses, nil) {
			unreadable.add(raw, "licences")
			return true
		}
		for _, l := range lic.Licenses {
			name := l.Product.Name
			if name == "" {
				name = l.Product.Code
			}
			if !l.Unlimited && l.Quantity > 0 && l.Usage.Current.Count > l.Quantity {
				add(doc.TenantID, fleetGap{Kind: "commercial", Type: "licence_over_usage", Count: l.Usage.Current.Count - l.Quantity,
					Detail: fmt.Sprintf("%s: %d in use, %d licensed", name, l.Usage.Current.Count, l.Quantity)})
			}
			if !l.Perpetual && l.EndDate != "" {
				if end, perr := time.Parse("2006-01-02", l.EndDate[:min(len(l.EndDate), 10)]); perr == nil && end.After(opt.now) && end.Sub(opt.now) <= 30*24*time.Hour {
					add(doc.TenantID, fleetGap{Kind: "commercial", Type: "licence_expiring", Detail: fmt.Sprintf("%s ends %s", name, end.Format("2006-01-02"))})
				}
			}
		}
		return true
	})
	if err != nil {
		return gapsView{}, fmt.Errorf("reading licences: %w", err)
	}

	view := gapsView{Items: make([]tenantGaps, 0, len(gaps)), ByType: map[string]int{}, BlindTenants: append(blindTenantsFrom(ledger, filter, gapsNeeded(opt.kind)...), unreadable.blind(names, filter)...)}
	for tid, list := range gaps {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Type < list[j].Type })
		view.Items = append(view.Items, tenantGaps{TenantID: tid, TenantName: names[tid], GapCount: len(list), Gaps: list})
		for _, g := range list {
			view.ByType[g.Type]++
		}
	}
	sort.SliceStable(view.Items, func(i, j int) bool {
		if view.Items[i].GapCount != view.Items[j].GapCount {
			return view.Items[i].GapCount > view.Items[j].GapCount
		}
		return view.Items[i].TenantName < view.Items[j].TenantName
	})
	view.Total = len(view.Items)
	return view, nil
}

// gapsNeeded names the refreshed resources a gaps view reads: security gaps
// come from health checks and endpoints, commercial ones from endpoints and
// licences.
func gapsNeeded(kind string) []string {
	switch kind {
	case "security":
		return []string{"health", "endpoints"}
	case "commercial":
		return []string{"endpoints", "licenses"}
	}
	return []string{"health", "endpoints", "licenses"}
}
