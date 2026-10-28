// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: overview.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// overviewReport is one tenant's current state. Every section's sync status is
// in Sections ("ok", "not synced", "failed: ..."); a section that is not "ok"
// leaves its totals null rather than reporting a misleading zero.
type overviewReport struct {
	Tenant                   *fleetTenant      `json:"tenant"`
	Sections                 map[string]string `json:"sections"`
	OpenDetections           []triageRow       `json:"open_detections"`
	OpenDetectionsTotal      *int              `json:"open_detections_total"`
	StaleDevices             []staleAgentRow   `json:"stale_devices"`
	StaleDevicesTotal        *int              `json:"stale_devices_total"`
	ExploitedVulnerabilities []map[string]any  `json:"exploited_vulnerabilities"`
	ExploitedTotal           *int              `json:"exploited_vulnerabilities_total"`
	LastScans                map[string]string `json:"last_completed_scan_by_type"`
	Posture                  map[string]any    `json:"posture"`
	DarkwebExposures         *int              `json:"darkweb_exposures"`
	DarkwebPasswordsExposed  *int              `json:"darkweb_passwords_exposed"`
	Cloud                    []map[string]any  `json:"cloud"`
	Contacts                 []map[string]any  `json:"contacts"`
	ContactsTotal            *int              `json:"contacts_total"`
	Sync                     []fleetSyncState  `json:"sync"`
}

func newNovelOverviewCmd(flags *rootFlags) *cobra.Command {
	var staleDays, top int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "overview [tenant]",
		Short: "One offline current-state view of a tenant: detections, stale devices, CVEs, scans, posture, exposures, contacts",
		Long: `Use this command for a current-state, offline look at one tenant.
Do NOT use this command for windowed client-reporting numbers; use 'client-report' instead.
Do NOT use it for the all-tenant detection queue; use 'triage' instead.

Resolves the tenant by id or name from the local store and reads every family
fleet-sync stored for it: oldest open detections, devices whose agent is silent
for --stale-days, exploited or prioritized vulnerabilities, last completed scan
per type, posture, dark-web exposure counts, Cloud Response connection state and
escalation contacts. The sections map gives each section's sync status; a
section that failed or was never synced shows null totals, not zero. An
unknown tenant, or a store with no fleet data, exits 3 (not found).`,
		Example: strings.Trim(`
  blackpoint-cli overview "Acme Dental" --agent
  blackpoint-cli overview 3f2a9c1e-7b4d-4e8a-9f10-2c6d8b5e4a71 --stale-days 7`, "\n"),
		// Exit 3 (not found) is a declared outcome: without fleet data no tenant resolves.
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "tenant=Acme Dental", "pp:typed-exit-codes": "0,3"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "overview: read one tenant's state from the local fleet store")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("tenant (name or id) is required"))
			}
			if top < 1 || staleDays < 0 {
				return usageErr(fmt.Errorf("--top must be at least 1 and --stale-days zero or more"))
			}
			query := strings.Join(args, " ")
			noData := notFoundErr(fmt.Errorf("no synced tenant matches %q: the local store has no fleet data yet; run 'blackpoint-cli fleet-sync' first", query))
			db, ok, err := openFleetForRead(cmd, flags, &dbPath)
			if err != nil {
				return err
			}
			if !ok {
				return noData
			}
			defer db.Close()
			_, tenants, err := fleetTenantIndex(db.DB())
			if err != nil {
				return err
			}
			if len(tenants) == 0 {
				return noData
			}
			t, err := resolveFleetTenant(tenants, query)
			if err != nil {
				return notFoundErr(err)
			}
			rep, err := buildOverview(db.DB(), t, time.Now().UTC(), staleDays, top)
			if err != nil {
				return err
			}
			return printOverview(cmd, flags, rep)
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 14, "Devices silent at least this many days count as stale")
	cmd.Flags().IntVar(&top, "top", 10, "Rows per section")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's local data.db)")
	return cmd
}

func normalizeOverview(r *overviewReport) *overviewReport {
	if r.Sections == nil {
		r.Sections = map[string]string{}
	}
	if r.OpenDetections == nil {
		r.OpenDetections = []triageRow{}
	}
	if r.StaleDevices == nil {
		r.StaleDevices = []staleAgentRow{}
	}
	for _, p := range []*[]map[string]any{&r.ExploitedVulnerabilities, &r.Cloud, &r.Contacts} {
		if *p == nil {
			*p = []map[string]any{}
		}
	}
	if r.LastScans == nil {
		r.LastScans = map[string]string{}
	}
	if r.Sync == nil {
		r.Sync = []fleetSyncState{}
	}
	return r
}

func intPtr(n int) *int { return &n }

func buildOverview(db *sql.DB, t fleetTenant, now time.Time, staleDays, top int) (*overviewReport, error) {
	idx := map[string]fleetTenant{t.ID: t}
	load := func(kind string) ([]fleetRecord, error) { return loadFleetRows(db, kind, t.ID) }
	rep := &overviewReport{Tenant: &t, Sections: map[string]string{}}
	all, err := loadFleetSyncState(db)
	if err != nil {
		return nil, err
	}
	status := func(section, kind string) bool {
		st := fleetSectionStatus(all, kind, t.ID)
		rep.Sections[section] = st
		return st == "ok"
	}

	if status("detections", fleetKindDetection) {
		dets, err := load(fleetKindDetection)
		if err != nil {
			return nil, err
		}
		rep.OpenDetections = buildTriageRows(dets, idx, now, 0, "", "")
		rep.OpenDetectionsTotal = intPtr(len(rep.OpenDetections))
		if len(rep.OpenDetections) > top {
			rep.OpenDetections = rep.OpenDetections[:top]
		}
	}

	if status("devices", fleetKindDevice) {
		devs, err := load(fleetKindDevice)
		if err != nil {
			return nil, err
		}
		rep.StaleDevices = buildStaleAgentRows(devs, idx, now, staleDays, false)
		rep.StaleDevicesTotal = intPtr(len(rep.StaleDevices))
		if len(rep.StaleDevices) > top {
			rep.StaleDevices = rep.StaleDevices[:top]
		}
	}

	if status("vulnerabilities", fleetKindVuln) {
		vulns, err := load(fleetKindVuln)
		if err != nil {
			return nil, err
		}
		for _, v := range vulns {
			exploited := strings.EqualFold(fieldStr(v.Obj, "exploitability"), "Attacked")
			prioritized, _ := fieldBool(v.Obj, "prioritized")
			if !exploited && !prioritized {
				continue
			}
			score, _ := fieldNum(v.Obj, "baseScore")
			assets, _ := fieldNum(v.Obj, "assetsAmount")
			rep.ExploitedVulnerabilities = append(rep.ExploitedVulnerabilities, map[string]any{
				"cve": fieldStr(v.Obj, "cveId", "cve", "name", "id"), "severity": fieldStr(v.Obj, "severity"),
				"base_score": score, "assets": assets, "exploited": exploited, "prioritized": prioritized,
			})
		}
		rep.ExploitedTotal = intPtr(len(rep.ExploitedVulnerabilities))
		sort.SliceStable(rep.ExploitedVulnerabilities, func(i, j int) bool {
			return rep.ExploitedVulnerabilities[i]["base_score"].(float64) > rep.ExploitedVulnerabilities[j]["base_score"].(float64)
		})
		if len(rep.ExploitedVulnerabilities) > top {
			rep.ExploitedVulnerabilities = rep.ExploitedVulnerabilities[:top]
		}
	}

	if status("scans", fleetKindScan) {
		gaps, err := buildGapsRows(db, t.ID, now, 30, true)
		if err != nil {
			return nil, err
		}
		if len(gaps) == 1 {
			rep.LastScans = gaps[0].LastScanByType
		}
	}

	if status("posture", fleetKindPosture) {
		ps, err := load(fleetKindPosture)
		if err != nil {
			return nil, err
		}
		if len(ps) > 0 {
			score, hasScore := fieldNum(ps[0].Obj, "score")
			maxScore, hasMax := fieldNum(ps[0].Obj, "maximumScore")
			if hasScore && hasMax {
				fn, d := weakestNISTFunction(ps[0].Obj)
				rep.Posture = map[string]any{"score": score, "maximum_score": maxScore, "maturity_level": fieldStr(ps[0].Obj, "maturityLevel"),
					"weakest_nist_function": fn, "weakest_deduction": d}
			}
		}
	}

	if status("darkweb", fleetKindDarkweb) {
		dw, err := load(fleetKindDarkweb)
		if err != nil {
			return nil, err
		}
		pw := 0
		for _, e := range dw {
			if b, _ := fieldBool(e.Obj, "passwordExposed"); b {
				pw++
			}
		}
		rep.DarkwebExposures, rep.DarkwebPasswordsExposed = intPtr(len(dw)), intPtr(pw)
	}

	if status("cloud", fleetKindCloudStatus) {
		conns, err := load(fleetKindM365Conn)
		if err != nil {
			return nil, err
		}
		for _, c := range conns {
			rep.Cloud = append(rep.Cloud, map[string]any{"provider": "m365", "connection": firstNonEmpty(fieldStr(c.Obj, "primaryDomain"), c.ID),
				"stage": fieldStr(c.Obj, "onboardingStage"), "onboarded": c.Obj["onboarded"]})
		}
		onbs, err := load(fleetKindOnboarding)
		if err != nil {
			return nil, err
		}
		for _, o := range onbs {
			rep.Cloud = append(rep.Cloud, map[string]any{"provider": fieldStr(o.Obj, "_provider"), "connection": firstNonEmpty(nestedStr(o.Obj, "config", "domain"), o.ID),
				"stage": fieldStr(o.Obj, "state"), "error": fieldStr(o.Obj, "error")})
		}
	}

	rep.Sections["contacts"] = "no contact group assigned"
	if t.ContactGroupID != "" {
		contacts, err := loadFleetContacts(db, all)
		if err != nil {
			return nil, err
		}
		g, found, healthy := contacts.lookup(t)
		switch {
		case !healthy:
			rep.Sections["contacts"] = "not synced or last contacts fetch failed"
		case !found:
			rep.Sections["contacts"] = "contact group not found in synced accounts"
		default:
			rep.Sections["contacts"] = "ok"
			for _, m := range fieldList(g, "members") {
				prio, _ := fieldNum(m, "priority")
				rep.Contacts = append(rep.Contacts, map[string]any{"group": fieldStr(g, "name"), "name": fieldStr(m, "name"),
					"email": fieldStr(m, "email"), "phone": fieldStr(m, "phoneNumber"), "priority": prio, "availability": fieldStr(m, "availability")})
			}
			// The fetch embeds up to 50 members; contacts_total is the group's real size.
			if n, known := contactMembers(g); known {
				rep.ContactsTotal = intPtr(n)
			}
		}
	}

	if rep.Sync, err = loadFleetSyncState(db, t.ID); err != nil {
		return nil, err
	}
	return normalizeOverview(rep), nil
}

func printOverview(cmd *cobra.Command, flags *rootFlags, rep *overviewReport) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
	}
	w := cmd.OutOrStdout()
	count := func(n *int) string {
		if n == nil {
			return "?"
		}
		return fmt.Sprint(*n)
	}
	fmt.Fprintf(w, "%s (%s) type=%s\n", rep.Tenant.Name, rep.Tenant.ID, rep.Tenant.Type)
	fmt.Fprintf(w, "open detections: %s   stale devices: %s   exploited/prioritized CVEs: %s   dark-web exposures: %s (%s with passwords)\n",
		count(rep.OpenDetectionsTotal), count(rep.StaleDevicesTotal), count(rep.ExploitedTotal), count(rep.DarkwebExposures), count(rep.DarkwebPasswordsExposed))
	sections := make([]string, 0, len(rep.Sections))
	for s := range rep.Sections {
		sections = append(sections, s)
	}
	sort.Strings(sections)
	for _, s := range sections {
		if st := rep.Sections[s]; st != "ok" {
			fmt.Fprintf(w, "section %s: %s\n", s, st)
		}
	}
	if rep.Posture != nil {
		fmt.Fprintf(w, "posture: %v / %v (%v), weakest: %v\n", rep.Posture["score"], rep.Posture["maximum_score"], rep.Posture["maturity_level"], rep.Posture["weakest_nist_function"])
	}
	for typ, at := range rep.LastScans {
		fmt.Fprintf(w, "last completed %s scan: %s\n", typ, at)
	}
	for _, d := range rep.OpenDetections {
		fmt.Fprintf(w, "  detection %s age=%s alerts=%v host=%s user=%s ticket=%s\n", d.DetectionID, d.Age, d.AlertCount, d.Hostname, d.Username, d.TicketStatus)
	}
	for _, c := range rep.Contacts {
		fmt.Fprintf(w, "  contact %v <%v> %v priority=%v %v\n", c["name"], c["email"], c["phone"], c["priority"], c["availability"])
	}
	return nil
}
