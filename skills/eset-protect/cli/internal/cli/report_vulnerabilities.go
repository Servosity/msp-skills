// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// Hand-authored. `report vulnerabilities`: a per-customer rollup of the live
// device-vulnerability list. ESET Connect cannot store device vulnerabilities
// (the rows carry no identifier), so the rows are always fetched live; the
// synced device-group tree only supplies customer names. The row parser here
// (reportVulnsParse) is also used by `devices profile`.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"eset-protect-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if report, _, err := root.Find([]string{"report"}); err == nil && report != root {
			addNovelCommandIfAbsent(report, newNovelReportVulnerabilitiesCmd(flags))
		}
	})
}

// reportVulnsUUIDRE recognises an RFC-4122-shaped identifier. ESET calls its
// identifiers opaque, so this is only used to decide whether a value with no
// local mirror to resolve it can be sent to the API as-is.
var reportVulnsUUIDRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// reportVulnsCore holds the fields shared by application, OS and package
// vulnerabilities (v1ApplicationVulnerability / v1OsVulnerability /
// v1PackageVulnerability).
type reportVulnsCore struct {
	CVENumber       string  `json:"cveNumber"`
	RiskScore       flexInt `json:"riskScore"`
	Severity        string  `json:"severity"`
	PatchAvailable  bool    `json:"patchAvailable"`
	FirstDetectTime string  `json:"firstDetectTime"`
	LastDetectTime  string  `json:"lastDetectTime"`
}

// reportVulnsWire is one v1DeviceVulnerability row: exactly one of the three
// variant objects is set.
type reportVulnsWire struct {
	DeviceUUID          string   `json:"deviceUuid"`
	DeviceGroupUUID     string   `json:"deviceGroupUuid"`
	AvailablePatchUUIDs []string `json:"availablePatchUuids"`
	Application         *struct {
		reportVulnsCore
		Application struct {
			DisplayName string      `json:"displayName"`
			Version     esetVersion `json:"version"`
		} `json:"application"`
	} `json:"applicationVulnerability"`
	OS *struct {
		reportVulnsCore
		OSFamilyID flexInt `json:"osFamilyId"`
	} `json:"osVulnerability"`
	Package *struct {
		reportVulnsCore
		Package struct {
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
			VersionName string `json:"versionName"`
		} `json:"package"`
	} `json:"packageVulnerability"`
}

// reportVulnsRow is a normalised device vulnerability.
type reportVulnsRow struct {
	DeviceUUID      string
	DeviceGroupUUID string
	Scope           string // application | os | package
	CVE             string
	RiskScore       int64
	Severity        string // HIGH, MEDIUM, LOW, INFORMATIONAL, DIAGNOSTIC, UNSPECIFIED (or an unknown value as sent)
	PatchAvailable  bool
	Target          string // application or package name and version; empty for OS rows
	OSFamilyID      int64
	FirstDetectTime string
	LastDetectTime  string
}

// reportVulnsSeverity shortens SEVERITY_LEVEL_HIGH to HIGH; empty becomes UNSPECIFIED.
func reportVulnsSeverity(v string) string {
	s := shortEnum(strings.ToUpper(strings.TrimSpace(v)), "SEVERITY_LEVEL_")
	if s == "" {
		return "UNSPECIFIED"
	}
	return s
}

// reportVulnsSeverityRank orders short severity names; unknown values rank 0.
func reportVulnsSeverityRank(s string) int {
	switch s {
	case "HIGH":
		return 5
	case "MEDIUM":
		return 4
	case "LOW":
		return 3
	case "INFORMATIONAL":
		return 2
	case "DIAGNOSTIC":
		return 1
	}
	return 0
}

// reportVulnsParse normalises one device-vulnerability row. It fails when the
// row is not a JSON object (null decodes "successfully" into an empty row), a
// field has the wrong type (riskScore must be an integer), or the row does not
// carry exactly one of applicationVulnerability / osVulnerability /
// packageVulnerability, so the caller counts the row as rejected instead of
// reading it as a risk-0 vulnerability. A row whose availablePatchUuids is
// non-empty counts as patchable even if patchAvailable was omitted.
func reportVulnsParse(raw json.RawMessage) (reportVulnsRow, error) {
	if !esetIsObject(raw) {
		return reportVulnsRow{}, errors.New("not a JSON object")
	}
	var w reportVulnsWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return reportVulnsRow{}, err
	}
	if err := exposureVulnVariantErr(w.Application != nil, w.OS != nil, w.Package != nil); err != nil {
		return reportVulnsRow{}, err
	}
	row := reportVulnsRow{
		DeviceUUID:      w.DeviceUUID,
		DeviceGroupUUID: w.DeviceGroupUUID,
	}
	var core *reportVulnsCore
	switch {
	case w.Application != nil:
		row.Scope = "application"
		core = &w.Application.reportVulnsCore
		row.Target = strings.TrimSpace(w.Application.Application.DisplayName + " " + w.Application.Application.Version.String())
	case w.OS != nil:
		row.Scope = "os"
		core = &w.OS.reportVulnsCore
		row.OSFamilyID = int64(w.OS.OSFamilyID)
	case w.Package != nil:
		row.Scope = "package"
		core = &w.Package.reportVulnsCore
		name := w.Package.Package.DisplayName
		if name == "" {
			name = w.Package.Package.Name
		}
		row.Target = strings.TrimSpace(name + " " + w.Package.Package.VersionName)
	}
	row.CVE = strings.TrimSpace(core.CVENumber)
	row.RiskScore = int64(core.RiskScore)
	row.Severity = reportVulnsSeverity(core.Severity)
	row.PatchAvailable = core.PatchAvailable
	row.FirstDetectTime = core.FirstDetectTime
	row.LastDetectTime = core.LastDetectTime
	if len(w.AvailablePatchUUIDs) > 0 {
		row.PatchAvailable = true
	}
	return row, nil
}

type reportVulnsCVE struct {
	CVE             string `json:"cve"`
	AffectedDevices int    `json:"affected_devices"`
	MaxRiskScore    int64  `json:"max_risk_score"`
	Severity        string `json:"severity"`
	PatchAvailable  bool   `json:"patch_available"`
}

type reportVulnsCustomer struct {
	Customer          string           `json:"customer"`
	CustomerUUID      string           `json:"customer_uuid"`
	VulnerableDevices int              `json:"vulnerable_devices"`
	Vulnerabilities   int              `json:"vulnerabilities"`
	BySeverity        map[string]int   `json:"by_severity"`
	MaxRiskScore      int64            `json:"max_risk_score"`
	PatchAvailable    int              `json:"patch_available"`
	TopCVEs           []reportVulnsCVE `json:"top_cves"`
}

type reportVulnsView struct {
	Customers              []reportVulnsCustomer `json:"customers"`
	ScannedVulnerabilities int                   `json:"scanned_vulnerabilities"`
	Pages                  int                   `json:"pages"`
	Truncated              bool                  `json:"truncated"`
	RejectedRows           int                   `json:"rejected_rows"`
	OtherCustomerRows      int                   `json:"other_customer_rows"`
	// UnattributedRows counts rows whose device group is not in the synced
	// tree, so their customer is unknown: under --customer they are left out
	// (they may be the customer's), without it they are the
	// "(group not in synced tree)" row (handfixes.json:
	// customer-views-count-unattributed-rows).
	UnattributedRows        int      `json:"unattributed_rows"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
	Note                    string   `json:"note,omitempty"`
	// SyncedHint is set when the device-group mirror that attributes rows to
	// customers is missing or has no completed sync.
	SyncedHint string `json:"synced_hint,omitempty"`
	// parentNotVisible names the synced groups whose parent this API user
	// cannot see (esetTree.parentNotVisible), for the unattributed line.
	parentNotVisible []string
	// ParentNotVisible is the same set for agents: each synced group (uuid
	// and name) whose parent this API user cannot see. Rows under them stay
	// unattributed after any sync (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	ParentNotVisible []esetGroupRef `json:"parent_not_visible,omitempty"`
}

// reportVulnsAttribute returns the customer a row belongs to and whether the
// report keeps it. Every row is attributed to the nearest COMPANY of its own
// deviceGroupUuid. With --customer (target non-nil), ESET's deviceGroupUuid
// filter is recursive, so the answer also holds rows of any COMPANY nested
// under the target; those are not the target's and are left out (keep
// false). A row whose group the synced tree does not know (created after the
// last sync) lands in the "(group not in synced tree)" bucket: kept without
// --customer, left out under it, because recursive scoping cannot prove that
// the target is its nearest COMPANY. With no synced tree at all nothing can
// be attributed, so a --customer run (scoped by group UUID) keeps every row
// under the target, as exposure does with its unsynced bucket.
func reportVulnsAttribute(tree *esetTree, target *esetGroup, groupUUID string) (esetGroup, bool) {
	// One ancestry walk (esetTree.resolve): a missing ancestor or a cycle is
	// owner-unknown exactly like a group the tree lacks.
	owner, known := tree.ownerOf(groupUUID)
	if target == nil {
		return owner, true
	}
	if len(tree.groups) == 0 {
		return *target, true
	}
	if !known {
		return owner, false
	}
	return owner, owner.UUID != "" && owner.UUID == target.UUID
}

// reportVulnsRollup groups rows by the customer owning each row's device
// group. Customers are ordered worst first: highest max risk score, then most
// vulnerable devices. Rows without a CVE id count toward the totals but are
// left out of top_cves (they would otherwise merge unrelated findings).
func reportVulnsRollup(rows []reportVulnsRow, customerOf func(groupUUID string) esetGroup, top int) []reportVulnsCustomer {
	type cveAcc struct {
		devices  map[string]bool
		maxRisk  int64
		severity string
		patch    bool
	}
	type custAcc struct {
		out     reportVulnsCustomer
		devices map[string]bool
		cves    map[string]*cveAcc
	}
	accs := map[string]*custAcc{}
	order := make([]string, 0)
	for _, r := range rows {
		g := customerOf(r.DeviceGroupUUID)
		key := g.UUID
		if key == "" {
			key = "bucket:" + g.Name
		}
		a, ok := accs[key]
		if !ok {
			a = &custAcc{
				out:     reportVulnsCustomer{Customer: g.Name, CustomerUUID: g.UUID, BySeverity: map[string]int{}, TopCVEs: make([]reportVulnsCVE, 0)},
				devices: map[string]bool{},
				cves:    map[string]*cveAcc{},
			}
			accs[key] = a
			order = append(order, key)
		}
		a.out.Vulnerabilities++
		if r.DeviceUUID != "" {
			a.devices[r.DeviceUUID] = true
		}
		sev := r.Severity
		if sev == "" {
			sev = "UNSPECIFIED"
		}
		a.out.BySeverity[sev]++
		if r.RiskScore > a.out.MaxRiskScore {
			a.out.MaxRiskScore = r.RiskScore
		}
		if r.PatchAvailable {
			a.out.PatchAvailable++
		}
		if r.CVE == "" {
			continue
		}
		c, ok := a.cves[r.CVE]
		if !ok {
			c = &cveAcc{devices: map[string]bool{}, severity: sev}
			a.cves[r.CVE] = c
		}
		if r.DeviceUUID != "" {
			c.devices[r.DeviceUUID] = true
		}
		if r.RiskScore > c.maxRisk {
			c.maxRisk = r.RiskScore
		}
		if reportVulnsSeverityRank(sev) > reportVulnsSeverityRank(c.severity) {
			c.severity = sev
		}
		if r.PatchAvailable {
			c.patch = true
		}
	}
	out := make([]reportVulnsCustomer, 0, len(order))
	for _, key := range order {
		a := accs[key]
		a.out.VulnerableDevices = len(a.devices)
		for cve, c := range a.cves {
			a.out.TopCVEs = append(a.out.TopCVEs, reportVulnsCVE{
				CVE: cve, AffectedDevices: len(c.devices), MaxRiskScore: c.maxRisk,
				Severity: c.severity, PatchAvailable: c.patch,
			})
		}
		sort.Slice(a.out.TopCVEs, func(i, j int) bool {
			x, y := a.out.TopCVEs[i], a.out.TopCVEs[j]
			if x.MaxRiskScore != y.MaxRiskScore {
				return x.MaxRiskScore > y.MaxRiskScore
			}
			if x.AffectedDevices != y.AffectedDevices {
				return x.AffectedDevices > y.AffectedDevices
			}
			if rx, ry := reportVulnsSeverityRank(x.Severity), reportVulnsSeverityRank(y.Severity); rx != ry {
				return rx > ry
			}
			return x.CVE < y.CVE
		})
		if top >= 0 && len(a.out.TopCVEs) > top {
			a.out.TopCVEs = a.out.TopCVEs[:top]
		}
		out = append(out, a.out)
	}
	sort.SliceStable(out, func(i, j int) bool {
		x, y := out[i], out[j]
		if x.MaxRiskScore != y.MaxRiskScore {
			return x.MaxRiskScore > y.MaxRiskScore
		}
		if x.VulnerableDevices != y.VulnerableDevices {
			return x.VulnerableDevices > y.VulnerableDevices
		}
		if x.Vulnerabilities != y.Vulnerabilities {
			return x.Vulnerabilities > y.Vulnerabilities
		}
		return strings.ToLower(x.Customer) < strings.ToLower(y.Customer)
	})
	return out
}

// reportVulnsBuildView parses a live page walk and rolls it up. target is the
// --customer group (nil for every customer). A row that does not decode is
// counted in rej; a row that belongs to another customer under --customer is
// counted in other_customer_rows. Neither enters any customer's totals.
func reportVulnsBuildView(res esetPageResult, tree *esetTree, target *esetGroup, top, maxPages int, notes []string, rej *esetRejects) reportVulnsView {
	if rej == nil {
		rej = &esetRejects{}
	}
	start := rej.N
	view := reportVulnsView{
		ScannedVulnerabilities: len(res.Items),
		Pages:                  res.Pages,
		Truncated:              res.Truncated,
	}
	rows := make([]reportVulnsRow, 0, len(res.Items))
	var un esetUnattributed
	for i, raw := range res.Items {
		r, err := reportVulnsParse(raw)
		if err != nil {
			rej.add(fmt.Sprintf("vulnerability row %d", i), err)
			continue
		}
		owner, keep := reportVulnsAttribute(tree, target, r.DeviceGroupUUID)
		unknown := owner.UUID == "" && esetOwnerUnknownBucket(owner.Name)
		if unknown {
			un.add(r.DeviceUUID)
		}
		if !keep {
			if !unknown {
				view.OtherCustomerRows++
			}
			continue
		}
		rows = append(rows, r)
	}
	view.UnattributedRows, view.UnattributedDeviceUUIDs = un.N, un.UUIDs()
	view.parentNotVisible = tree.parentNotVisible()
	view.ParentNotVisible = tree.parentNotVisibleGroups()
	view.RejectedRows = rej.N - start
	customerOf := func(groupUUID string) esetGroup {
		c, _ := reportVulnsAttribute(tree, target, groupUUID)
		return c
	}
	view.Customers = reportVulnsRollup(rows, customerOf, top)
	if view.OtherCustomerRows > 0 && target != nil {
		notes = append(notes, fmt.Sprintf("%d row(s) belong to other customers whose groups sit under %s (ESET's group filter includes nested groups) and were left out", view.OtherCustomerRows, target.Name))
	}
	scope := ""
	if target != nil {
		scope = target.Name
	}
	if line := esetUnattributedLine(view.UnattributedRows, "vulnerability row(s)", view.UnattributedDeviceUUIDs, scope, "Run '"+esetCLIName+" sync --resources device-groups' to attribute them.", view.parentNotVisible); line != "" {
		notes = append(notes, line)
	}
	if view.RejectedRows > 0 {
		notes = append(notes, fmt.Sprintf("%d row(s) could not be decoded and were left out of the totals", view.RejectedRows))
	}
	if res.Truncated {
		notes = append(notes, fmt.Sprintf("stopped after %d page(s) (--max-scan-pages %d) with more rows left; counts are partial, raise --max-scan-pages for the full picture", res.Pages, maxPages))
	}
	view.Note = strings.Join(notes, "; ")
	return view
}

func newNovelReportVulnerabilitiesCmd(flags *rootFlags) *cobra.Command {
	var customer, dbPath string
	var maxPages, top int
	cmd := &cobra.Command{
		Use:   "vulnerabilities",
		Short: "Per-customer vulnerability rollup: vulnerable devices, top CVEs by risk, what is patchable",
		Long: "Use this command for a vulnerability rollup per customer: how many devices, which CVEs, what is patchable. " +
			"Do NOT use it to look up one CVE; use 'exposure' instead. Do NOT use it to plan patch deployment; use 'patches backlog' instead.\n\n" +
			"Device vulnerabilities are always read live from ESET Connect (they cannot be synced). Each row is\n" +
			"mapped to its customer through the synced device-group tree (the nearest COMPANY ancestor); without\n" +
			"a synced tree every row rolls into one (tenant) row. --customer limits the live query to that\n" +
			"customer's group, which ESET applies to every nested group; rows whose nearest COMPANY is\n" +
			"another customer nested under it are left out and counted in other_customer_rows. Rows\n" +
			"from device groups not yet in the synced tree (created after the last sync) have no known\n" +
			"customer: they are reported as \"(group not in synced tree)\" (or \"(parent group not\n" +
			"visible)\" when the group is synced but a parent is not visible to the API user; see 'fleet\n" +
			"health'), excluded from a --customer scope (they may be that customer's), and counted in\n" +
			"unattributed_rows with their device uuids in unattributed_device_uuids; re-sync\n" +
			"device-groups to attribute them, except rows under a parent group the API user cannot see,\n" +
			"which no sync attributes. Rows that cannot be\n" +
			"decoded (including null rows and rows without exactly one vulnerability variant) are\n" +
			"left out and counted in rejected_rows.\n\n" +
			"Per customer: vulnerable_devices (distinct devices), vulnerabilities (rows), by_severity,\n" +
			"max_risk_score (0-100), patch_available (rows with a patch) and top_cves ranked by risk score,\n" +
			"then by affected devices. Customers are listed worst first.",
		Example: strings.Trim(`
  eset-protect-cli report vulnerabilities --agent
  eset-protect-cli report vulnerabilities --customer 'Acme Dental' --top 5 --json
  eset-protect-cli report vulnerabilities --max-scan-pages 50 --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "report vulnerabilities")
			}
			if len(args) > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("report vulnerabilities takes no arguments; use --customer to pick one customer"))
			}
			if top < 1 {
				return usageErr(fmt.Errorf("--top must be at least 1"))
			}
			if maxPages < 1 {
				return usageErr(fmt.Errorf("--max-scan-pages must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}

			groups := make([]esetGroup, 0)
			db, err := openEsetStore(cmd, dbPath, "device-groups")
			if err != nil {
				return err
			}
			mirrorHint := esetMirrorHint(db, "device-groups")
			if db != nil {
				esetSyncHints(cmd, db, flags.maxAge, "device-groups")
				groups, err = loadEsetGroups(db)
				_ = db.Close()
				if err != nil {
					return fmt.Errorf("reading synced device groups: %w", err)
				}
			}
			tree := newEsetTree(groups)
			var target *esetGroup
			notes := make([]string, 0)
			params := map[string]string{}
			if strings.TrimSpace(customer) != "" {
				var g esetGroup
				switch {
				case len(groups) > 0:
					g, err = tree.resolveCustomer(customer)
					if err != nil {
						_ = cmd.Usage()
						return usageErr(err)
					}
				case reportVulnsUUIDRE.MatchString(strings.TrimSpace(customer)):
					id := strings.TrimSpace(customer)
					g = esetGroup{UUID: id, Name: id}
					notes = append(notes, "no synced device-group tree, so the customer is shown by its group UUID and every row ESET returned for that group and the groups under it (which can include nested customers) is counted; run 'eset-protect-cli sync --resources device-groups' for names and per-customer attribution")
				default:
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("cannot resolve customer %q without a synced device-group tree: run 'eset-protect-cli sync --resources device-groups' or pass the customer's group UUID", customer))
				}
				params["deviceGroupUuid"] = g.UUID
				target = &g
			} else if len(groups) == 0 {
				notes = append(notes, "no synced device-group tree, so every row rolls into one (tenant) row; run 'eset-protect-cli sync --resources device-groups' for per-customer rows")
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			res, err := esetFetchAll(ctx, c, esetHost("vulnerability-management", "/v1/device-vulnerabilities"), params, "vulnerabilities", maxPages)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			var rej esetRejects
			view := reportVulnsBuildView(res, tree, target, top, maxPages, notes, &rej)
			view.SyncedHint = mirrorHint
			rej.warn(cmd)

			w := cmd.OutOrStdout()
			if !wantsHumanTable(w, flags) {
				return printJSONFiltered(w, view, flags)
			}
			if len(view.Customers) == 0 {
				if view.UnattributedRows+view.OtherCustomerRows > 0 {
					// Rows were read but none could be credited: never an all-clear.
					esetFprintf(w, "No vulnerability row could be attributed to %s (%d page(s) read, %d row(s) left out).\n", target.Name, view.Pages, view.UnattributedRows+view.OtherCustomerRows)
				} else {
					esetFprintf(w, "ESET reported no device vulnerabilities (%d page(s) read).\n", view.Pages)
				}
				if view.Note != "" {
					esetFprintf(w, "note: %s\n", view.Note)
				}
				if view.SyncedHint != "" {
					esetFprintf(w, "synced data: %s\n", view.SyncedHint)
				}
				return nil
			}
			tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "CUSTOMER\tDEVICES\tVULNS\tMAX RISK\tPATCHABLE\tWORST CVE")
			for _, cu := range view.Customers {
				worst := "-"
				if len(cu.TopCVEs) > 0 {
					t := cu.TopCVEs[0]
					worst = fmt.Sprintf("%s (risk %d, %d device(s))", t.CVE, t.MaxRiskScore, t.AffectedDevices)
				}
				esetFprintf(tw, "%s\t%d\t%d\t%d\t%d\t%s\n", cu.Customer, cu.VulnerableDevices, cu.Vulnerabilities, cu.MaxRiskScore, cu.PatchAvailable, worst)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			esetFprintf(w, "\n%d vulnerability row(s) across %d page(s). Use --json for every customer's top CVEs.\n", view.ScannedVulnerabilities, view.Pages)
			if view.Note != "" {
				esetFprintf(w, "note: %s\n", view.Note)
			}
			if view.SyncedHint != "" {
				esetFprintf(w, "synced data: %s\n", view.SyncedHint)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Limit to one customer: COMPANY group name or UUID (resolved through the synced device-group tree)")
	cmd.Flags().IntVar(&maxPages, "max-scan-pages", 20, "Maximum pages of 1000 vulnerability rows to read (1 under dogfood)")
	cmd.Flags().IntVar(&top, "top", 10, "Top CVEs to list per customer")
	cmd.Flags().StringVar(&dbPath, "db", "", esetDBFlagUsage)
	return cmd
}
