// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// Hand-authored. `triage`: one ranked work queue from the local mirror,
// merging v1 endpoint detections in a window with open incidents. v1
// detections carry no resolved state, so every detection in the window is
// listed. Absorbs the ESET-MCP incident_triage prompt. The severity helpers
// here are shared with `detections latest`.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNovelTriageCmd(flags))
	})
}

// triageSeverityRanks orders the short severity names. "unknown" (an
// unspecified or unrecognized level) sorts between low and info.
var triageSeverityRanks = map[string]int{
	"high":       5,
	"medium":     4,
	"low":        3,
	"unknown":    2,
	"info":       1,
	"diagnostic": 0,
}

// triageSeverityShort maps SEVERITY_LEVEL_* (detections) and
// INCIDENT_SEVERITY_LEVEL_* (incidents) onto one short scale.
func triageSeverityShort(level string) string {
	s := strings.ToUpper(strings.TrimSpace(level))
	s = strings.TrimPrefix(s, "INCIDENT_SEVERITY_LEVEL_")
	s = strings.TrimPrefix(s, "SEVERITY_LEVEL_")
	switch s {
	case "HIGH":
		return "high"
	case "MEDIUM":
		return "medium"
	case "LOW":
		return "low"
	case "INFORMATIONAL", "INFO":
		return "info"
	case "DIAGNOSTIC":
		return "diagnostic"
	}
	return "unknown"
}

func triageSeverityRank(short string) int {
	if r, ok := triageSeverityRanks[short]; ok {
		return r
	}
	return triageSeverityRanks["unknown"]
}

// triageParseMinSeverity normalizes a --min-severity value. Empty means no
// floor. Accepts the short names plus "informational".
func triageParseMinSeverity(s string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "":
		return "", nil
	case "informational":
		return "info", nil
	case "high", "medium", "low", "info", "diagnostic":
		return v, nil
	}
	return "", fmt.Errorf("invalid --min-severity %q: use low, medium or high (or info, diagnostic)", s)
}

// triageSeverityPasses reports whether a short severity clears the floor.
// An unspecified severity is kept at a floor of low or below, so an
// unlabelled incident never silently drops out of the default queue.
func triageSeverityPasses(short, min string) bool {
	if min == "" {
		return true
	}
	if short == "unknown" {
		return triageSeverityRank("low") >= triageSeverityRank(min)
	}
	return triageSeverityRank(short) >= triageSeverityRank(min)
}

// triageParseTime parses an RFC3339 timestamp; ok is false when absent or malformed.
func triageParseTime(s string) (time.Time, bool) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// triageDetection is the subset of a v1 (and v2) detection the queue and
// `detections latest` read. v1 carries the device in context.deviceUuid;
// v2 (Cloud Office / Inspect) carries it in device.
type triageDetection struct {
	UUID           string `json:"uuid"`
	DisplayName    string `json:"displayName"`
	Category       string `json:"category"`
	TypeName       string `json:"typeName"`
	ObjectName     string `json:"objectName"`
	ObjectHashSha1 string `json:"objectHashSha1"`
	OccurTime      string `json:"occurTime"`
	SeverityLevel  string `json:"severityLevel"`
	Context        struct {
		DeviceUUID string `json:"deviceUuid"`
	} `json:"context"`
	Device struct {
		UUID        string `json:"uuid"`
		DisplayName string `json:"displayName"`
	} `json:"device"`
}

func (d triageDetection) deviceUUID() string {
	if d.Context.DeviceUUID != "" {
		return d.Context.DeviceUUID
	}
	return d.Device.UUID
}

// title prefers displayName, then "typeName: objectName".
func (d triageDetection) title() string {
	if d.DisplayName != "" {
		return d.DisplayName
	}
	switch {
	case d.TypeName != "" && d.ObjectName != "":
		return d.TypeName + ": " + d.ObjectName
	case d.TypeName != "":
		return d.TypeName
	case d.ObjectName != "":
		return d.ObjectName
	}
	return d.UUID
}

type triageIncident struct {
	UUID        string   `json:"uuid"`
	DisplayName string   `json:"displayName"`
	Severity    string   `json:"severity"`
	Status      string   `json:"status"`
	CreateTime  string   `json:"createTime"`
	UpdateTime  string   `json:"updateTime"`
	DeviceUUIDs []string `json:"deviceUuids"`
}

const triageIncidentClosed = "INCIDENT_STATUS_CLOSED"

type triageItem struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Time     string `json:"time"`
	Title    string `json:"title"`
	Device   string `json:"device"`
	Customer string `json:"customer"`
	Status   string `json:"status,omitempty"`
	UUID     string `json:"uuid"`
	Next     string `json:"next"`

	at time.Time
}

type triageCounts struct {
	Detections int `json:"detections"`
	Incidents  int `json:"incidents"`
}

type triageView struct {
	Items        []triageItem `json:"items"`
	Total        int          `json:"total"`
	Since        string       `json:"since"`
	Counts       triageCounts `json:"counts"`
	RejectedRows int          `json:"rejected_rows"`
	// UnattributedIncidents counts open incidents in the window that no
	// device ties to a customer (the list row carries no deviceUuids, or
	// devices the mirror does not know or cannot walk to a customer). With
	// --customer they are left out of items (they might be this customer's,
	// so a queue with a non-zero count is never reported as clear); without
	// it they are in items with their customer shown as "(owner unknown)",
	// "(unknown device)" or "(group not in synced tree)" and counted here too.
	// UnattributedDetections is the same for detections. (Round 3: both used
	// to be always 0 without --customer.)
	UnattributedIncidents      int      `json:"unattributed_incidents"`
	UnattributedIncidentUUIDs  []string `json:"unattributed_incident_uuids,omitempty"`
	UnattributedDetections     int      `json:"unattributed_detections"`
	UnattributedDetectionUUIDs []string `json:"unattributed_detection_uuids,omitempty"`
	// SyncedHint is set when the mirror is missing or a needed resource has no
	// completed sync, so an empty queue is not read as "nothing to triage".
	SyncedHint string `json:"synced_hint,omitempty"`

	scoped bool // --customer was given (human wording only)
	// parentNotVisible names the synced groups whose parent this API user
	// cannot see (esetTree.parentNotVisible), for the unattributed line.
	parentNotVisible []string
	// ParentNotVisible is the same set for agents: each synced group (uuid
	// and name) whose parent this API user cannot see. Rows under them stay
	// unattributed after any sync (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	ParentNotVisible []esetGroupRef `json:"parent_not_visible,omitempty"`
}

// triageOwnerUnknown labels a queue row no device ties to a customer.
const triageOwnerUnknown = "(owner unknown)"

type triageOptions struct {
	Start                 time.Time
	Customer              *esetGroup // nil: every customer
	MinSeverity           string
	Limit                 int
	IncludeStaleIncidents bool
}

// triageDeviceCustomer returns the device name and customer for a device
// UUID. Unknown devices keep the UUID as the name and an empty customer. A
// synced device whose group the tree lacks is owner-unknown too (customer
// "(group not in synced tree)", known false), so --customer counts it as
// unattributed instead of filing it under another customer
// (customer-views-count-unattributed-rows).
func triageDeviceCustomer(uuid string, byUUID map[string]esetDevice, tree *esetTree) (name string, customer esetGroup, known bool) {
	// A tree with no COMPANY group is single-tenant: every row is the tenant's.
	single := tree != nil && !tree.hasCompanies
	if uuid == "" {
		if single {
			return "", esetGroup{Name: esetCustomerTenant}, true
		}
		return "", esetGroup{Name: triageOwnerUnknown}, false
	}
	d, ok := byUUID[uuid]
	if !ok {
		if single {
			return uuid, esetGroup{Name: esetCustomerTenant}, true
		}
		return uuid, esetGroup{Name: detectionsSpreadUnknownDevice}, false
	}
	owner, known := tree.ownerOf(d.ParentGroupUUID)
	return d.name(), owner, known
}

// triageBuild ranks detections and incidents into one queue. A row that does
// not decode, has no uuid, or (for a detection) has no parseable occurTime
// cannot be placed in the window; it is counted in rej, never silently
// dropped. An open incident with neither a parseable createTime nor
// updateTime is counted the same way unless stale incidents are kept anyway.
func triageBuild(detections, incidents []json.RawMessage, devices []esetDevice, tree *esetTree, opt triageOptions, rej *esetRejects) triageView {
	if rej == nil {
		rej = &esetRejects{}
	}
	start := rej.N
	byUUID := make(map[string]esetDevice, len(devices))
	for _, d := range devices {
		byUUID[d.UUID] = d
	}
	label := detectionsSpreadCustomerLabels(tree)
	view := triageView{Items: make([]triageItem, 0), scoped: opt.Customer != nil}
	if !opt.Start.IsZero() {
		view.Since = opt.Start.UTC().Format(time.RFC3339)
	}
	keepCustomer := func(c esetGroup, known bool) bool {
		if opt.Customer == nil {
			return true
		}
		return known && c.UUID == opt.Customer.UUID
	}
	all := make([]triageItem, 0)

	for i, raw := range detections {
		if !esetIsObject(raw) {
			rej.add(fmt.Sprintf("detection row %d", i), errors.New("not a JSON object"))
			continue
		}
		var d triageDetection
		if err := json.Unmarshal(raw, &d); err != nil {
			rej.add(fmt.Sprintf("detection row %d", i), err)
			continue
		}
		if d.UUID == "" {
			rej.add(fmt.Sprintf("detection row %d", i), errors.New("no uuid"))
			continue
		}
		at, ok := triageParseTime(d.OccurTime)
		if !ok {
			rej.add("detection row "+d.UUID, fmt.Errorf("occurTime %q is not an RFC 3339 time", d.OccurTime))
			continue
		}
		if at.Before(opt.Start) {
			continue
		}
		sev := triageSeverityShort(d.SeverityLevel)
		if !triageSeverityPasses(sev, opt.MinSeverity) {
			continue
		}
		name, cust, known := triageDeviceCustomer(d.deviceUUID(), byUUID, tree)
		if !known {
			// No device the mirror can place: counted whether or not
			// --customer is set, before the customer filter.
			view.UnattributedDetections++
			view.UnattributedDetectionUUIDs = append(view.UnattributedDetectionUUIDs, d.UUID)
		}
		if !keepCustomer(cust, known) {
			continue
		}
		custName := ""
		if cust.Name != "" {
			custName = label(cust)
		}
		next := esetCLIName + " detections get " + d.UUID
		if sha := strings.ToLower(strings.TrimSpace(d.ObjectHashSha1)); sha != "" {
			next = esetCLIName + " detections spread --sha1 " + sha
		}
		all = append(all, triageItem{
			Kind: "detection", Severity: sev, Time: at.UTC().Format(time.RFC3339),
			Title: d.title(), Device: name, Customer: custName, UUID: d.UUID, Next: next, at: at,
		})
	}

	for i, raw := range incidents {
		if !esetIsObject(raw) {
			rej.add(fmt.Sprintf("incident row %d", i), errors.New("not a JSON object"))
			continue
		}
		var in triageIncident
		if err := json.Unmarshal(raw, &in); err != nil {
			rej.add(fmt.Sprintf("incident row %d", i), err)
			continue
		}
		if in.UUID == "" {
			rej.add(fmt.Sprintf("incident row %d", i), errors.New("no uuid"))
			continue
		}
		if strings.EqualFold(in.Status, triageIncidentClosed) {
			continue
		}
		created, cok := triageParseTime(in.CreateTime)
		updated, uok := triageParseTime(in.UpdateTime)
		at := created
		if uok && (!cok || updated.After(created)) {
			at = updated
		}
		inWindow := (cok && !created.Before(opt.Start)) || (uok && !updated.Before(opt.Start))
		if !inWindow && !opt.IncludeStaleIncidents {
			if !cok && !uok {
				rej.add("incident row "+in.UUID, errors.New("neither createTime nor updateTime is an RFC 3339 time"))
			}
			continue
		}
		sev := triageSeverityShort(in.Severity)
		if !triageSeverityPasses(sev, opt.MinSeverity) {
			continue
		}
		// An incident can span customers: resolve every device, list each
		// customer once (by COMPANY UUID, so two customers sharing a name
		// both show), and match --customer against any of them.
		var name string
		var custNames []string
		keep := opt.Customer == nil
		seenCust := map[string]bool{}
		// unresolved: a device the mirror cannot place (or no device at all),
		// so --customer cannot rule the incident out.
		unresolved := len(in.DeviceUUIDs) == 0
		anyKnown := false
		for j, du := range in.DeviceUUIDs {
			dn, c, known := triageDeviceCustomer(du, byUUID, tree)
			if j == 0 {
				name = dn
			}
			if !known {
				unresolved = true
			} else {
				anyKnown = true
			}
			id := c.UUID
			if id == "" {
				id = "bucket:" + c.Name
			}
			if c.Name != "" && !seenCust[id] {
				seenCust[id] = true
				custNames = append(custNames, label(c))
			}
			if !keep && keepCustomer(c, known) {
				keep = true
			}
		}
		if extra := len(in.DeviceUUIDs) - 1; extra > 0 {
			name = fmt.Sprintf("%s (+%d more)", name, extra)
		}
		if len(in.DeviceUUIDs) == 0 && tree != nil && !tree.hasCompanies {
			anyKnown, unresolved = true, false // single-tenant: the tenant's
		}
		// Handfix triage-customer-counts-unattributed-incidents: with
		// --customer an incident it cannot rule out is counted; without it
		// every incident no device ties to a customer is counted.
		if (!keep && unresolved) || (opt.Customer == nil && !anyKnown) {
			view.UnattributedIncidents++
			view.UnattributedIncidentUUIDs = append(view.UnattributedIncidentUUIDs, in.UUID)
		}
		if !keep {
			continue
		}
		if len(custNames) == 0 {
			custNames = append(custNames, triageOwnerUnknown)
		}
		ts := ""
		if !at.IsZero() {
			ts = at.UTC().Format(time.RFC3339)
		}
		title := in.DisplayName
		if title == "" {
			title = in.UUID
		}
		all = append(all, triageItem{
			Kind: "incident", Severity: sev, Time: ts, Title: title, Device: name, Customer: strings.Join(custNames, ", "),
			Status: strings.ToLower(shortEnum(in.Status, "INCIDENT_STATUS_")), UUID: in.UUID,
			Next: esetCLIName + " incidents context " + in.UUID, at: at,
		})
	}

	sort.SliceStable(all, func(i, j int) bool {
		ri, rj := triageSeverityRank(all[i].Severity), triageSeverityRank(all[j].Severity)
		if ri != rj {
			return ri > rj
		}
		if !all[i].at.Equal(all[j].at) {
			return all[i].at.After(all[j].at)
		}
		return all[i].UUID < all[j].UUID
	})
	for _, it := range all {
		if it.Kind == "detection" {
			view.Counts.Detections++
		} else {
			view.Counts.Incidents++
		}
	}
	view.Total = len(all)
	view.RejectedRows = rej.N - start
	sort.Strings(view.UnattributedIncidentUUIDs)
	sort.Strings(view.UnattributedDetectionUUIDs)
	if opt.Limit > 0 && len(all) > opt.Limit {
		all = all[:opt.Limit]
	}
	view.Items = append(view.Items, all...)
	return view
}

// triageHiddenRemedy is " " + esetHiddenParentRemedy, or "".
func triageHiddenRemedy(v triageView) string {
	if r := esetHiddenParentRemedy(v.parentNotVisible); r != "" {
		return " " + r
	}
	return ""
}

// triagePrintUnattributed names the open incidents --customer could not rule
// out, so the human queue never reads as clear while one may be this
// customer's.
func triagePrintUnattributed(w io.Writer, v triageView) {
	list := func(ids []string) string {
		if len(ids) > 5 {
			return strings.Join(ids[:5], ", ") + fmt.Sprintf(" (+%d more)", len(ids)-5)
		}
		return strings.Join(ids, ", ")
	}
	if !v.scoped {
		if v.UnattributedIncidents > 0 {
			esetFprintf(w, "%d open incident(s) in the queue name no device the mirror can tie to a customer (customer %s): %s. Check each with '%s incidents context <uuid>'.\n",
				v.UnattributedIncidents, triageOwnerUnknown, list(v.UnattributedIncidentUUIDs), esetCLIName)
		}
		if v.UnattributedDetections > 0 {
			esetFprintf(w, "%d detection(s) in the queue are on a device the mirror cannot tie to a customer (not synced, or its device group is not%s): %s. Run '%s sync --resources devices,device-groups' to attribute them.%s\n",
				v.UnattributedDetections, esetHiddenParentCause(v.parentNotVisible), list(v.UnattributedDetectionUUIDs), esetCLIName, triageHiddenRemedy(v))
		}
		return
	}
	if v.UnattributedIncidents > 0 {
		esetFprintf(w, "%d open incident(s) name no device the mirror can tie to a customer, so --customer could not rule them out: %s. Check each with '%s incidents context <uuid>' or rerun without --customer.\n",
			v.UnattributedIncidents, list(v.UnattributedIncidentUUIDs), esetCLIName)
	}
	if v.UnattributedDetections > 0 {
		esetFprintf(w, "%d detection(s) are on a device the mirror cannot tie to a customer (not synced, or its device group is not%s), so --customer could not rule them out: %s. Check each with '%s detections get <uuid>', sync devices and device-groups, or rerun without --customer.%s\n",
			v.UnattributedDetections, esetHiddenParentCause(v.parentNotVisible), list(v.UnattributedDetectionUUIDs), esetCLIName, triageHiddenRemedy(v))
	}
}

func triageClip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var since, customer, minSeverity, dbPath string
	var limit int
	var includeStale bool
	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Ranked morning work queue: endpoint detections in the window plus open incidents, worst first",
		Long: "Use this command for this morning's work queue: endpoint detections in the window (v1 has no\n" +
			"resolved state) and open incidents, worst first, with device and customer. Do NOT use it to\n" +
			"see how far one file hash spread; use 'detections spread' instead. Do NOT use it for a\n" +
			"quarterly customer summary; use 'report customer' instead.\n\n" +
			"Reads the local mirror (sync detections, incidents, devices and device-groups first).\n" +
			"Detections are every v1 endpoint detection whose occurTime falls inside --since; v1 has no\n" +
			"resolved state, so a detection already handled still lists until it ages out. Incidents are\n" +
			"every incident not CLOSED that was created or updated inside --since; add\n" +
			"--include-stale-incidents to keep older open incidents too. Rows rank by severity\n" +
			"(high, medium, low, info, diagnostic; incident severities share the scale), then newest\n" +
			"first. A row with no severity is kept at the default --min-severity low. Incident list\n" +
			"rows often carry no device, so their device is blank and their customer \"(owner unknown)\".\n" +
			"Such an incident (or one naming only devices the mirror does not know or cannot walk to a\n" +
			"customer) is counted in unattributed_incidents, with its uuid in unattributed_incident_uuids;\n" +
			"a detection on such a device is counted the same way in unattributed_detections. Without\n" +
			"--customer those rows stay in items; --customer cannot rule them out, so it leaves them out of\n" +
			"items and still counts them. A non-zero count is never reported as an empty queue. Each\n" +
			"row's 'next' names the follow-up command. A stored row that cannot be\n" +
			"decoded, or carries no uuid or no parseable time, is left out and counted in rejected_rows.",
		Example: strings.Trim(`
  eset-protect-cli triage
  eset-protect-cli triage --since 72h --min-severity medium --agent
  eset-protect-cli triage --customer "Acme Dental" --include-stale-incidents --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "local",
			"pp:typed-exit-codes": "0,2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "triage")
			}
			start, err := esetSince(since)
			if err != nil {
				return err
			}
			if start.IsZero() || start.After(time.Now()) {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--since must be a positive duration such as 24h, 3d or 1w"))
			}
			minSev, err := triageParseMinSeverity(minSeverity)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			if minSev == "" {
				minSev = "low"
			}
			if limit < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be at least 1"))
			}
			resources := []string{"detections", "incidents", "devices", "device-groups"}
			db, err := openEsetStore(cmd, dbPath, resources...)
			if err != nil {
				return err
			}
			opt := triageOptions{Start: start, MinSeverity: minSev, Limit: limit, IncludeStaleIncidents: includeStale}
			view := triageView{Items: make([]triageItem, 0), Since: start.UTC().Format(time.RFC3339)}
			if db != nil {
				defer db.Close()
				esetSyncHints(cmd, db, flags.maxAge, resources...)
				groups, err := loadEsetGroups(db)
				if err != nil {
					return fmt.Errorf("reading device groups: %w", err)
				}
				devices, err := loadEsetDevices(db)
				if err != nil {
					return fmt.Errorf("reading devices: %w", err)
				}
				dets, err := loadEsetRaw(db, "detections")
				if err != nil {
					return fmt.Errorf("reading detections: %w", err)
				}
				incs, err := loadEsetRaw(db, "incidents")
				if err != nil {
					return fmt.Errorf("reading incidents: %w", err)
				}
				tree := newEsetTree(groups)
				if strings.TrimSpace(customer) != "" {
					g, err := tree.resolveCustomer(customer)
					if err != nil {
						return usageErr(err)
					}
					opt.Customer = &g
				}
				var rej esetRejects
				view = triageBuild(dets, incs, devices, tree, opt, &rej)
				view.parentNotVisible = tree.parentNotVisible()
				view.ParentNotVisible = tree.parentNotVisibleGroups()
				rej.warn(cmd)
			}
			view.SyncedHint = esetMirrorHint(db, resources...)
			w := cmd.OutOrStdout()
			if !wantsHumanTable(w, flags) {
				return printJSONFiltered(w, view, flags)
			}
			if len(view.Items) == 0 {
				if view.UnattributedIncidents+view.UnattributedDetections > 0 {
					esetFprintf(w, "No detections or open incidents attributed to %s at %s or above since %s.\n", strings.TrimSpace(customer), minSev, view.Since)
				} else {
					esetFprintf(w, "Nothing to triage: no detections or open incidents at %s or above since %s.\n", minSev, view.Since)
				}
				triagePrintUnattributed(w, view)
				if view.SyncedHint != "" {
					esetFprintln(w, view.SyncedHint)
				}
				if view.RejectedRows > 0 {
					esetFprintf(w, "%d stored row(s) could not be read and were left out.\n", view.RejectedRows)
				}
				return nil
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "SEVERITY\tKIND\tTIME\tTITLE\tDEVICE\tCUSTOMER\tNEXT")
			for _, it := range view.Items {
				esetFprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", it.Severity, it.Kind, it.Time, triageClip(it.Title, 48), triageClip(it.Device, 28), triageClip(it.Customer, 28), it.Next)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			esetFprintf(w, "\n%d of %d items (%d detections, %d incidents) since %s\n", len(view.Items), view.Total, view.Counts.Detections, view.Counts.Incidents, view.Since)
			triagePrintUnattributed(w, view)
			if view.RejectedRows > 0 {
				esetFprintf(w, "%d stored row(s) could not be read and were left out.\n", view.RejectedRows)
			}
			if view.SyncedHint != "" {
				esetFprintln(w, view.SyncedHint)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Window to look back over (e.g. 24h, 3d, 1w)")
	cmd.Flags().StringVar(&customer, "customer", "", "Only this customer (COMPANY group name, unique name fragment or UUID)")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "low", "Lowest severity to keep: low, medium or high (also info, diagnostic)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to return")
	cmd.Flags().BoolVar(&includeStale, "include-stale-incidents", false, "Also keep open incidents last touched before --since")
	cmd.Flags().StringVar(&dbPath, "db", "", esetDBFlagUsage)
	return cmd
}
