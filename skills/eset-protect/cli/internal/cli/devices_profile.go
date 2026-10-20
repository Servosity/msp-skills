// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// Hand-authored. `devices profile`: everything about one device in one call.
// The device record is required; detections, vulnerabilities, pending patches
// and recent vulnerability scans are fetched in parallel, and any of them that
// fails is reported in fetch_failures (its section is null, never zero-filled).
// A returned row that cannot be decoded is counted in its section's
// rejected_rows (and the top-level total), never dropped silently.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if devices, _, err := root.Find([]string{"devices"}); err == nil && devices != root {
			addNovelCommandIfAbsent(devices, newNovelDevicesProfileCmd(flags))
		}
	})
}

const (
	devicesProfileListCap      = 10
	devicesProfileDetectPages  = 2
	devicesProfileVulnPages    = 3
	devicesProfilePatchPages   = 3
	devicesProfileScanPages    = 5
	devicesProfileSyncResource = "devices,device-groups"
)

type devicesProfileComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type devicesProfileDevice struct {
	UUID                string                    `json:"uuid"`
	Name                string                    `json:"name"`
	Hostname            string                    `json:"hostname"`
	OS                  string                    `json:"os"`
	FunctionalityStatus string                    `json:"functionality_status"`
	ProblemCount        int64                     `json:"problem_count"`
	Muted               bool                      `json:"muted"`
	LastSeen            string                    `json:"last_seen"`
	IP                  string                    `json:"ip"`
	Components          []devicesProfileComponent `json:"components"`
	Customer            string                    `json:"customer"`
	// CustomerUUID is null for a bucket (handfixes.json:
	// customer-uuid-beside-customer-name).
	CustomerUUID *string `json:"customer_uuid"`
	GroupPath    string  `json:"group_path"`
}

type devicesProfileDetection struct {
	UUID       string   `json:"uuid"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Severity   string   `json:"severity"`
	Object     string   `json:"object"`
	SHA1       string   `json:"sha1"`
	OccurredAt string   `json:"occurred_at"`
	Responses  []string `json:"responses"`
}

type devicesProfileDetections struct {
	Since        string                    `json:"since"`
	Count        int                       `json:"count"`
	BySeverity   map[string]int            `json:"by_severity"`
	Latest       []devicesProfileDetection `json:"latest"`
	Truncated    bool                      `json:"truncated"`
	RejectedRows int                       `json:"rejected_rows"`
}

type devicesProfileVuln struct {
	CVE            string `json:"cve"`
	RiskScore      int64  `json:"risk_score"`
	Severity       string `json:"severity"`
	Scope          string `json:"scope"`
	Target         string `json:"target"`
	PatchAvailable bool   `json:"patch_available"`
}

type devicesProfileVulns struct {
	Count        int                  `json:"count"`
	MaxRiskScore int64                `json:"max_risk_score"`
	Patchable    int                  `json:"patchable"`
	Top          []devicesProfileVuln `json:"top"`
	Truncated    bool                 `json:"truncated"`
	RejectedRows int                  `json:"rejected_rows"`
}

type devicesProfilePatch struct {
	UUID            string   `json:"uuid"`
	Kind            string   `json:"kind"`
	Name            string   `json:"name"`
	Vendor          string   `json:"vendor"`
	Version         string   `json:"version"`
	CatalogID       int64    `json:"catalog_id"`
	Category        string   `json:"category"`
	Severity        string   `json:"severity"`
	RestartRequired string   `json:"restart_required"` // YES, NO or UNSPECIFIED (hand-fix profile-restart-keeps-unknown)
	FixedCVEs       []string `json:"fixed_cves"`
}

type devicesProfilePatches struct {
	Count         int                   `json:"count"`
	SecurityCount int                   `json:"security_count"`
	Items         []devicesProfilePatch `json:"items"`
	Truncated     bool                  `json:"truncated"`
	RejectedRows  int                   `json:"rejected_rows"`
}

type devicesProfileScan struct {
	StartTime          string `json:"start_time"`
	EndTime            string `json:"end_time"`
	EndedSuccessfully  bool   `json:"ended_successfully"`
	Trigger            string `json:"trigger"`
	VulnerabilityCount int64  `json:"vulnerability_count"`
}

// devicesProfileScans summarizes the recent scans. ESET does not order the
// list, so when the page cap is hit (truncated) the newest or a successful
// scan may sit on an unread page: latest and last_successful are then null
// and note says why, rather than presenting a possibly wrong scan.
type devicesProfileScans struct {
	Count          int                 `json:"count"`
	Latest         *devicesProfileScan `json:"latest"`
	LastSuccessful *devicesProfileScan `json:"last_successful"`
	Truncated      bool                `json:"truncated"`
	Note           string              `json:"note,omitempty"`
	RejectedRows   int                 `json:"rejected_rows"`
}

type devicesProfileFailure struct {
	Part  string `json:"part"`
	Error string `json:"error"`
}

type devicesProfileView struct {
	Device          devicesProfileDevice      `json:"device"`
	Detections      *devicesProfileDetections `json:"detections"`
	Vulnerabilities *devicesProfileVulns      `json:"vulnerabilities"`
	Patches         *devicesProfilePatches    `json:"patches"`
	Scans           *devicesProfileScans      `json:"scans"`
	FetchFailures   []devicesProfileFailure   `json:"fetch_failures"`
	RejectedRows    int                       `json:"rejected_rows"` // sum over the sections
	// SyncedHint is set when the mirror that supplies customer, group_path
	// and name resolution is missing or has no completed sync.
	SyncedHint string `json:"synced_hint,omitempty"`
}

// devicesProfileParts holds the raw result of every parallel fetch.
type devicesProfileParts struct {
	device     json.RawMessage
	deviceErr  error
	detections esetPageResult
	detErr     error
	vulns      esetPageResult
	vulnErr    error
	patches    esetPageResult
	patchErr   error
	scans      esetPageResult
	scanErr    error
}

// devicesProfileResolve turns the <device> argument into a UUID. With a
// mirror it accepts a UUID or an exact display name / hostname; a UUID that
// is not mirrored yet is still used as-is. Without a mirror only a UUID works.
func devicesProfileResolve(q string, devices []esetDevice, haveMirror bool) (string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", fmt.Errorf("device name or UUID is empty")
	}
	if haveMirror {
		d, err := resolveEsetDevice(devices, q)
		if err == nil {
			return d.UUID, nil
		}
		if reportVulnsUUIDRE.MatchString(q) {
			return q, nil
		}
		return "", err
	}
	if reportVulnsUUIDRE.MatchString(q) {
		return q, nil
	}
	return "", fmt.Errorf("no local mirror to resolve device name %q: pass the device UUID, or run 'eset-protect-cli sync --resources %s' first", q, devicesProfileSyncResource)
}

// devicesProfileGather runs the five fetches in parallel.
func devicesProfileGather(ctx context.Context, c *client.Client, deviceUUID string, since time.Time, dogfood bool) devicesProfileParts {
	pages := func(n int) int {
		if dogfood {
			return 1
		}
		return n
	}
	var p devicesProfileParts
	var wg sync.WaitGroup
	run := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	byDevice := map[string]string{"deviceUuid": deviceUUID}
	run(func() {
		p.device, p.deviceErr = c.GetNoCache(ctx, replacePathParam(esetHost("device-management", "/v1/devices/{deviceUuid}"), "deviceUuid", deviceUUID), nil)
	})
	run(func() {
		params := map[string]string{"deviceUuid": deviceUUID}
		if !since.IsZero() {
			params["startTime"] = since.UTC().Format(time.RFC3339)
		}
		p.detections, p.detErr = esetFetchAll(ctx, c, esetHost("incident-management", "/v1/detections"), params, "detections", pages(devicesProfileDetectPages))
	})
	run(func() {
		p.vulns, p.vulnErr = esetFetchAll(ctx, c, esetHost("vulnerability-management", "/v1/device-vulnerabilities"), byDevice, "vulnerabilities", pages(devicesProfileVulnPages))
	})
	run(func() {
		p.patches, p.patchErr = esetFetchAll(ctx, c, esetHost("patch-management", "/v1/device-patches"), byDevice, "devices", pages(devicesProfilePatchPages))
	})
	run(func() {
		p.scans, p.scanErr = esetFetchAll(ctx, c, esetHost("vulnerability-management", "/v1/scans/recent"), byDevice, "scanDetails", pages(devicesProfileScanPages))
	})
	wg.Wait()
	return p
}

// devicesProfileParseDevice reads GET /v1/devices/{uuid} ({"device": {...}}).
func devicesProfileParseDevice(raw json.RawMessage) (esetDevice, error) {
	var env struct {
		Device json.RawMessage `json:"device"`
	}
	body := raw
	if err := json.Unmarshal(raw, &env); err == nil && len(env.Device) > 0 {
		body = env.Device
	}
	if !esetIsObject(body) {
		return esetDevice{}, errors.New("decoding device: not a JSON object")
	}
	var d esetDevice
	if err := json.Unmarshal(body, &d); err != nil {
		return esetDevice{}, fmt.Errorf("decoding device: %w", err)
	}
	if d.UUID == "" {
		return esetDevice{}, fmt.Errorf("device response carried no uuid")
	}
	return d, nil
}

func devicesProfileDeviceView(d esetDevice, groups []esetGroup) devicesProfileDevice {
	v := devicesProfileDevice{
		UUID:                d.UUID,
		Name:                d.name(),
		Hostname:            d.Hostname,
		OS:                  d.OperatingSystem.DisplayName,
		FunctionalityStatus: shortEnum(d.FunctionalityStatus, "DEVICE_FUNCTIONALITY_STATUS_"),
		ProblemCount:        int64(d.ProblemCount),
		Muted:               d.IsMuted,
		LastSeen:            d.LastSyncTime,
		IP:                  d.PrimaryIP,
		Components:          make([]devicesProfileComponent, 0, len(d.Components)),
	}
	for _, comp := range d.Components {
		name := comp.DisplayName
		if name == "" {
			name = comp.Name
		}
		v.Components = append(v.Components, devicesProfileComponent{Name: name, Version: comp.Version.String()})
	}
	if len(groups) > 0 {
		// A group the synced tree lacks is owner-unknown ("(group not in
		// synced tree)"), never "(unassigned)", unless it is the hidden scope
		// root of a scoped API user (handfixes.json:
		// customer-views-count-unattributed-rows,
		// ownership-needs-complete-ancestry).
		tree := newEsetTree(groups)
		owner, _ := tree.ownerOf(d.ParentGroupUUID)
		v.Customer = owner.Name
		v.CustomerUUID = esetCustomerUUID(owner)
		v.GroupPath = tree.path(d.ParentGroupUUID)
	}
	return v
}

// devicesProfileTime parses an RFC3339 timestamp; zero when absent or malformed.
func devicesProfileTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func devicesProfileDetectionsSummary(items []json.RawMessage, truncated bool, since time.Time, rej *esetRejects) *devicesProfileDetections {
	if rej == nil {
		rej = &esetRejects{}
	}
	start := rej.N
	out := &devicesProfileDetections{
		Count:      0,
		BySeverity: map[string]int{},
		Latest:     make([]devicesProfileDetection, 0),
		Truncated:  truncated,
	}
	if !since.IsZero() {
		out.Since = since.UTC().Format(time.RFC3339)
	}
	all := make([]devicesProfileDetection, 0, len(items))
	for i, raw := range items {
		var d struct {
			UUID           string `json:"uuid"`
			DisplayName    string `json:"displayName"`
			TypeName       string `json:"typeName"`
			ObjectName     string `json:"objectName"`
			ObjectHashSha1 string `json:"objectHashSha1"`
			OccurTime      string `json:"occurTime"`
			SeverityLevel  string `json:"severityLevel"`
			Responses      []struct {
				DisplayName string `json:"displayName"`
			} `json:"responses"`
		}
		if !esetIsObject(raw) {
			rej.add(fmt.Sprintf("detection row %d", i), errors.New("not a JSON object"))
			continue
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			rej.add(fmt.Sprintf("detection row %d", i), err)
			continue
		}
		sev := reportVulnsSeverity(d.SeverityLevel)
		out.Count++
		out.BySeverity[sev]++
		det := devicesProfileDetection{
			UUID: d.UUID, Name: d.DisplayName, Type: d.TypeName, Severity: sev,
			Object: d.ObjectName, SHA1: d.ObjectHashSha1, OccurredAt: d.OccurTime,
			Responses: make([]string, 0, len(d.Responses)),
		}
		for _, r := range d.Responses {
			if r.DisplayName != "" {
				det.Responses = append(det.Responses, r.DisplayName)
			}
		}
		all = append(all, det)
	}
	sort.SliceStable(all, func(i, j int) bool {
		ti, tj := devicesProfileTime(all[i].OccurredAt), devicesProfileTime(all[j].OccurredAt)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return all[i].UUID < all[j].UUID
	})
	if len(all) > devicesProfileListCap {
		all = all[:devicesProfileListCap]
	}
	out.Latest = all
	out.RejectedRows = rej.N - start
	return out
}

// devicesProfileVulnsSummary ranks the device's vulnerabilities by risk score.
// osName labels OS vulnerabilities, which carry only an OS family id.
func devicesProfileVulnsSummary(items []json.RawMessage, truncated bool, osName string, rej *esetRejects) *devicesProfileVulns {
	if rej == nil {
		rej = &esetRejects{}
	}
	start := rej.N
	out := &devicesProfileVulns{Top: make([]devicesProfileVuln, 0), Truncated: truncated}
	all := make([]devicesProfileVuln, 0, len(items))
	for i, raw := range items {
		r, err := reportVulnsParse(raw)
		if err != nil {
			rej.add(fmt.Sprintf("vulnerability row %d", i), err)
			continue
		}
		out.Count++
		if r.RiskScore > out.MaxRiskScore {
			out.MaxRiskScore = r.RiskScore
		}
		if r.PatchAvailable {
			out.Patchable++
		}
		target := r.Target
		if r.Scope == "os" {
			switch {
			case osName != "":
				target = osName
			case r.OSFamilyID != 0:
				target = fmt.Sprintf("OS family %d", r.OSFamilyID)
			default:
				target = "operating system"
			}
		}
		all = append(all, devicesProfileVuln{
			CVE: r.CVE, RiskScore: r.RiskScore, Severity: r.Severity, Scope: r.Scope,
			Target: target, PatchAvailable: r.PatchAvailable,
		})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].RiskScore != all[j].RiskScore {
			return all[i].RiskScore > all[j].RiskScore
		}
		if ri, rj := reportVulnsSeverityRank(all[i].Severity), reportVulnsSeverityRank(all[j].Severity); ri != rj {
			return ri > rj
		}
		return all[i].CVE < all[j].CVE
	})
	if len(all) > devicesProfileListCap {
		all = all[:devicesProfileListCap]
	}
	out.Top = all
	out.RejectedRows = rej.N - start
	return out
}

type devicesProfilePatchCore struct {
	CatalogID            flexInt  `json:"catalogId"`
	Category             string   `json:"category"`
	DeveloperDisplayName string   `json:"developerDisplayName"`
	FixedCves            []string `json:"fixedCves"`
	RestartRequired      string   `json:"restartRequired"`
	ResultVersionName    string   `json:"resultVersionName"`
	Severity             string   `json:"severity"`
}

func devicesProfilePatchesSummary(items []json.RawMessage, truncated bool, rej *esetRejects) *devicesProfilePatches {
	if rej == nil {
		rej = &esetRejects{}
	}
	start := rej.N
	out := &devicesProfilePatches{Items: make([]devicesProfilePatch, 0), Truncated: truncated}
	all := make([]devicesProfilePatch, 0, len(items))
	for i, raw := range items {
		var w struct {
			UUID        string                   `json:"uuid"`
			Application *devicesProfilePatchCore `json:"applicationPatch"`
			OS          *struct {
				devicesProfilePatchCore
				OSFamilyID flexInt `json:"osFamilyId"`
			} `json:"osPatch"`
			Package *struct {
				devicesProfilePatchCore
				Package struct {
					DisplayName string `json:"displayName"`
					Name        string `json:"name"`
				} `json:"package"`
			} `json:"packagePatch"`
		}
		if !esetIsObject(raw) {
			rej.add(fmt.Sprintf("patch row %d", i), errors.New("not a JSON object"))
			continue
		}
		if err := json.Unmarshal(raw, &w); err != nil {
			rej.add(fmt.Sprintf("patch row %d", i), err)
			continue
		}
		p := devicesProfilePatch{UUID: w.UUID, Kind: "unknown"}
		var core *devicesProfilePatchCore
		switch {
		case w.Application != nil:
			p.Kind = "application"
			core = w.Application
			p.Name = core.DeveloperDisplayName
		case w.OS != nil:
			p.Kind = "os"
			core = &w.OS.devicesProfilePatchCore
			p.Name = "operating system update"
		case w.Package != nil:
			p.Kind = "package"
			core = &w.Package.devicesProfilePatchCore
			p.Name = w.Package.Package.DisplayName
			if p.Name == "" {
				p.Name = w.Package.Package.Name
			}
		}
		p.FixedCVEs = make([]string, 0)
		p.Severity = "UNSPECIFIED"
		p.Category = "UNSPECIFIED"
		p.RestartRequired = "UNSPECIFIED"
		if core != nil {
			p.Vendor = core.DeveloperDisplayName
			p.Version = core.ResultVersionName
			p.CatalogID = int64(core.CatalogID)
			if c := shortEnum(core.Category, "PATCH_CATEGORY_"); c != "" {
				p.Category = c
			}
			p.Severity = reportVulnsSeverity(core.Severity)
			// Hand-fix profile-restart-keeps-unknown: ESET's UNSPECIFIED (or an
			// omitted value) means unknown, not "no restart"; keep the enum,
			// as patches backlog does.
			if r := shortEnum(core.RestartRequired, "RESTART_REQUIREMENT_"); r != "" {
				p.RestartRequired = r
			}
			if core.FixedCves != nil {
				p.FixedCVEs = core.FixedCves
			}
		}
		out.Count++
		if p.Category == "SECURITY_UPDATE" {
			out.SecurityCount++
		}
		all = append(all, p)
	}
	sort.SliceStable(all, func(i, j int) bool {
		si, sj := all[i].Category == "SECURITY_UPDATE", all[j].Category == "SECURITY_UPDATE"
		if si != sj {
			return si
		}
		if ri, rj := reportVulnsSeverityRank(all[i].Severity), reportVulnsSeverityRank(all[j].Severity); ri != rj {
			return ri > rj
		}
		if len(all[i].FixedCVEs) != len(all[j].FixedCVEs) {
			return len(all[i].FixedCVEs) > len(all[j].FixedCVEs)
		}
		return all[i].Name < all[j].Name
	})
	if len(all) > devicesProfileListCap {
		all = all[:devicesProfileListCap]
	}
	out.Items = all
	out.RejectedRows = rej.N - start
	return out
}

// devicesProfileScansSummary picks the newest and the newest successful scan
// among the scans read. pages is how many pages were read; when truncated, the
// two picks are withheld (null) because an unread page may hold a newer or a
// successful scan, and note explains it.
func devicesProfileScansSummary(items []json.RawMessage, truncated bool, pages int, rej *esetRejects) *devicesProfileScans {
	if rej == nil {
		rej = &esetRejects{}
	}
	start := rej.N
	out := &devicesProfileScans{Truncated: truncated}
	all := make([]devicesProfileScan, 0, len(items))
	for i, raw := range items {
		var s struct {
			StartTime          string  `json:"startTime"`
			EndTime            string  `json:"endTime"`
			EndedSuccessfully  bool    `json:"endedSuccessfully"`
			ScanTrigger        string  `json:"scanTrigger"`
			VulnerabilityCount flexInt `json:"vulnerabilityCount"`
		}
		if !esetIsObject(raw) {
			rej.add(fmt.Sprintf("scan row %d", i), errors.New("not a JSON object"))
			continue
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			rej.add(fmt.Sprintf("scan row %d", i), err)
			continue
		}
		all = append(all, devicesProfileScan{
			StartTime: s.StartTime, EndTime: s.EndTime, EndedSuccessfully: s.EndedSuccessfully,
			Trigger: shortEnum(s.ScanTrigger, "SCAN_TRIGGER_"), VulnerabilityCount: int64(s.VulnerabilityCount),
		})
	}
	out.RejectedRows = rej.N - start
	out.Count = len(all)
	if truncated {
		out.Note = fmt.Sprintf("only the first %d page(s) of scans were read and ESET does not order them, so latest and last_successful are unknown (null); %d scan(s) read", pages, len(all))
		return out
	}
	at := func(s devicesProfileScan) time.Time {
		if t := devicesProfileTime(s.EndTime); !t.IsZero() {
			return t
		}
		return devicesProfileTime(s.StartTime)
	}
	sort.SliceStable(all, func(i, j int) bool { return at(all[i]).After(at(all[j])) })
	if len(all) > 0 {
		latest := all[0]
		out.Latest = &latest
	}
	for _, s := range all {
		if s.EndedSuccessfully {
			ok := s
			out.LastSuccessful = &ok
			break
		}
	}
	return out
}

// devicesProfileBuild shapes the gathered parts. It fails only when the
// required device record is missing or unreadable. Undecodable rows in any
// section are counted in rej and in rejected_rows.
func devicesProfileBuild(p devicesProfileParts, groups []esetGroup, since time.Time, rej *esetRejects) (devicesProfileView, error) {
	if rej == nil {
		rej = &esetRejects{}
	}
	start := rej.N
	view := devicesProfileView{FetchFailures: make([]devicesProfileFailure, 0)}
	if p.deviceErr != nil {
		return view, p.deviceErr
	}
	d, err := devicesProfileParseDevice(p.device)
	if err != nil {
		return view, err
	}
	view.Device = devicesProfileDeviceView(d, groups)
	fail := func(part string, err error) {
		view.FetchFailures = append(view.FetchFailures, devicesProfileFailure{Part: part, Error: err.Error()})
	}
	if p.detErr != nil {
		fail("detections", p.detErr)
	} else {
		view.Detections = devicesProfileDetectionsSummary(p.detections.Items, p.detections.Truncated, since, rej)
	}
	if p.vulnErr != nil {
		fail("vulnerabilities", p.vulnErr)
	} else {
		view.Vulnerabilities = devicesProfileVulnsSummary(p.vulns.Items, p.vulns.Truncated, d.OperatingSystem.DisplayName, rej)
	}
	if p.patchErr != nil {
		fail("patches", p.patchErr)
	} else {
		view.Patches = devicesProfilePatchesSummary(p.patches.Items, p.patches.Truncated, rej)
	}
	if p.scanErr != nil {
		fail("scans", p.scanErr)
	} else {
		view.Scans = devicesProfileScansSummary(p.scans.Items, p.scans.Truncated, p.scans.Pages, rej)
	}
	view.RejectedRows = rej.N - start
	return view, nil
}

func newNovelDevicesProfileCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath string
	cmd := &cobra.Command{
		Use:   "profile <device>",
		Short: "One device in one call: health, recent detections, CVEs, pending patches and vulnerability scans",
		Long: "Use this command for everything about one device in one call: health, recent detections, CVEs, pending patches and scans. " +
			"Do NOT use it for a whole customer; use 'report customer' instead.\n\n" +
			"<device> is a device UUID, or an exact display name or hostname resolved through the synced\n" +
			"device mirror. The device record is required; detections (in the --since window), device\n" +
			"vulnerabilities, pending patches and recent vulnerability scans are fetched live in parallel.\n" +
			"A part that fails is listed in fetch_failures and its section is null; it is never shown as zero.\n" +
			"Each section's truncated flag marks a page cap; a truncated scans section leaves latest and\n" +
			"last_successful null (ESET does not order scans) and says so in note. A returned row that cannot\n" +
			"be decoded is counted in that section's rejected_rows and the top-level rejected_rows.\n" +
			"customer and group_path come from the synced device-group tree and are empty without one.",
		Example: strings.Trim(`
  eset-protect-cli devices profile FRONTDESK-01 --agent
  eset-protect-cli devices profile 3f2b8c1e-6d4a-4b7f-9e21-0a5c7d9e1f42 --since 7d --json
  eset-protect-cli devices profile frontdesk-01.acmedental.local --select device,vulnerabilities.top`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "device=example-value",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "devices profile")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass exactly one device: a UUID, or an exact display name or hostname"))
			}
			start, err := esetSince(since)
			if err != nil {
				return err
			}

			devices := make([]esetDevice, 0)
			groups := make([]esetGroup, 0)
			db, err := openEsetStore(cmd, dbPath, "devices", "device-groups")
			if err != nil {
				return err
			}
			mirrorHint := esetMirrorHint(db, "devices", "device-groups")
			if db != nil {
				esetSyncHints(cmd, db, flags.maxAge, "devices", "device-groups")
				devices, err = loadEsetDevices(db)
				if err == nil {
					groups, err = loadEsetGroups(db)
				}
				_ = db.Close()
				if err != nil {
					return fmt.Errorf("reading the local mirror: %w", err)
				}
			}
			deviceUUID, err := devicesProfileResolve(args[0], devices, db != nil)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			parts := devicesProfileGather(ctx, c, deviceUUID, start, cliutil.IsDogfoodEnv())
			var rej esetRejects
			view, err := devicesProfileBuild(parts, groups, start, &rej)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view.SyncedHint = mirrorHint
			rej.warn(cmd)
			if n := len(view.FetchFailures); n > 0 {
				names := make([]string, 0, n)
				for _, f := range view.FetchFailures {
					names = append(names, f.Part)
				}
				esetFprintf(cmd.ErrOrStderr(), "warning: %d of 4 profile parts failed (%s); see fetch_failures\n", n, strings.Join(names, ", "))
			}

			w := cmd.OutOrStdout()
			if !wantsHumanTable(w, flags) {
				return printJSONFiltered(w, view, flags)
			}
			return devicesProfileHuman(w, view, since)
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "Detection window, e.g. 24h, 7d, 30d")
	cmd.Flags().StringVar(&dbPath, "db", "", esetDBFlagUsage)
	return cmd
}

// devicesProfilePartial renders a section's caveats: the page cap and any
// undecodable rows.
func devicesProfilePartial(truncated bool, rejected int) string {
	parts := make([]string, 0, 2)
	if truncated {
		parts = append(parts, "partial: stopped at the page cap")
	}
	if rejected > 0 {
		parts = append(parts, fmt.Sprintf("%d row(s) could not be decoded", rejected))
	}
	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, "; ") + "]"
}

func devicesProfileHuman(w io.Writer, v devicesProfileView, since string) error {
	d := v.Device
	esetFprintf(w, "%s (%s)\n", d.Name, d.UUID)
	if d.Customer != "" || d.GroupPath != "" {
		esetFprintf(w, "  customer: %s   group: %s\n", d.Customer, d.GroupPath)
	}
	esetFprintf(w, "  status: %s, %d problem(s), muted=%t, last seen %s\n", d.FunctionalityStatus, d.ProblemCount, d.Muted, d.LastSeen)
	esetFprintf(w, "  os: %s   hostname: %s   ip: %s\n", d.OS, d.Hostname, d.IP)
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	if v.Detections != nil {
		esetFprintf(w, "\nDetections (last %s): %d%s\n", since, v.Detections.Count, devicesProfilePartial(v.Detections.Truncated, v.Detections.RejectedRows))
		if len(v.Detections.Latest) > 0 {
			fmt.Fprintln(tw, "  OCCURRED\tSEVERITY\tNAME\tOBJECT")
			for _, x := range v.Detections.Latest {
				esetFprintf(tw, "  %s\t%s\t%s\t%s\n", x.OccurredAt, x.Severity, x.Name, x.Object)
			}
			_ = tw.Flush()
		}
	}
	if v.Vulnerabilities != nil {
		esetFprintf(w, "\nVulnerabilities: %d (max risk %d, %d patchable)%s\n", v.Vulnerabilities.Count, v.Vulnerabilities.MaxRiskScore, v.Vulnerabilities.Patchable,
			devicesProfilePartial(v.Vulnerabilities.Truncated, v.Vulnerabilities.RejectedRows))
		if len(v.Vulnerabilities.Top) > 0 {
			fmt.Fprintln(tw, "  CVE\tRISK\tSEVERITY\tTARGET\tPATCH")
			for _, x := range v.Vulnerabilities.Top {
				esetFprintf(tw, "  %s\t%d\t%s\t%s\t%t\n", x.CVE, x.RiskScore, x.Severity, x.Target, x.PatchAvailable)
			}
			_ = tw.Flush()
		}
	}
	if v.Patches != nil {
		esetFprintf(w, "\nPending patches: %d (%d security)%s\n", v.Patches.Count, v.Patches.SecurityCount, devicesProfilePartial(v.Patches.Truncated, v.Patches.RejectedRows))
		if len(v.Patches.Items) > 0 {
			fmt.Fprintln(tw, "  KIND\tNAME\tVERSION\tCATEGORY\tCVES")
			for _, x := range v.Patches.Items {
				esetFprintf(tw, "  %s\t%s\t%s\t%s\t%d\n", x.Kind, x.Name, x.Version, x.Category, len(x.FixedCVEs))
			}
			_ = tw.Flush()
		}
	}
	if v.Scans != nil {
		partial := devicesProfilePartial(v.Scans.Truncated, v.Scans.RejectedRows)
		switch {
		case v.Scans.Truncated:
			esetFprintf(w, "\nVulnerability scans: %d read%s; latest and last successful unknown (%s)\n", v.Scans.Count, partial, v.Scans.Note)
		case v.Scans.Latest == nil:
			esetFprintf(w, "\nVulnerability scans: none recorded%s\n", partial)
		default:
			l := v.Scans.Latest
			esetFprintf(w, "\nVulnerability scans: %d recent; latest ended %s (success=%t, %d vulnerabilities)%s\n", v.Scans.Count, l.EndTime, l.EndedSuccessfully, l.VulnerabilityCount, partial)
		}
	}
	for _, f := range v.FetchFailures {
		esetFprintf(w, "\n%s: not available (%s)\n", f.Part, f.Error)
	}
	if v.SyncedHint != "" {
		esetFprintf(w, "\nsynced data: %s\n", v.SyncedHint)
	}
	return nil
}
