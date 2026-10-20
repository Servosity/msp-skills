// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// Hand-authored. `detections latest`: the newest detections straight from
// ESET Connect, no sync needed. Absorbs ESET-MCP latest_detections(hours,
// limit, severity_min). A local mirror, when present, only enriches rows with
// device names and customers.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if detections, _, err := root.Find([]string{"detections"}); err == nil && detections != root {
			addNovelCommandIfAbsent(detections, newNovelDetectionsLatestCmd(flags))
		}
	})
}

var detectionsLatestUUIDRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	detectionsLatestSourceEndpoint    = "endpoint"
	detectionsLatestSourceCloudOffice = "cloud-office"
)

type detectionsLatestRow struct {
	OccurredAt string `json:"occurred_at"`
	Severity   string `json:"severity"`
	Name       string `json:"name"`
	Object     string `json:"object"`
	SHA1       string `json:"sha1"`
	Category   string `json:"category"`
	Device     string `json:"device"`
	DeviceUUID string `json:"device_uuid"`
	Customer   string `json:"customer"`
	Source     string `json:"source"`
	UUID       string `json:"uuid"`

	at time.Time
}

type detectionsLatestView struct {
	Items        []detectionsLatestRow `json:"items"`
	TotalFetched int                   `json:"total_fetched"`
	Pages        int                   `json:"pages"`
	Truncated    bool                  `json:"truncated"`
	Since        string                `json:"since"`
	Warnings     []string              `json:"warnings"`
	RejectedRows int                   `json:"rejected_rows"`
	// UnattributedDetections counts returned rows whose customer the local
	// mirror cannot give: the device is not in it (customer "(unknown
	// device)") or its group is not in the synced tree ("(group not in
	// synced tree)"). 0 without a mirror, where customer is blank on every row
	// (handfixes.json: customer-views-count-unattributed-rows).
	UnattributedDetections     int      `json:"unattributed_detections"`
	UnattributedDetectionUUIDs []string `json:"unattributed_detection_uuids,omitempty"`
	// SyncedHint is set when a local mirror exists for device and customer
	// names but has no completed sync, so some names may be missing. A
	// live-only user with no mirror sees no hint.
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

// detectionsLatestParse turns raw v1 or v2 detections into rows. A row that
// does not decode or carries no UUID is counted in rej, not dropped silently.
func detectionsLatestParse(raw []json.RawMessage, source string, rej *esetRejects) []detectionsLatestRow {
	if rej == nil {
		rej = &esetRejects{}
	}
	rows := make([]detectionsLatestRow, 0, len(raw))
	for i, r := range raw {
		if !esetIsObject(r) {
			rej.add(fmt.Sprintf("%s detection row %d", source, i), errors.New("not a JSON object"))
			continue
		}
		var d triageDetection
		if err := json.Unmarshal(r, &d); err != nil {
			rej.add(fmt.Sprintf("%s detection row %d", source, i), err)
			continue
		}
		if d.UUID == "" {
			rej.add(fmt.Sprintf("%s detection row %d", source, i), errors.New("no uuid"))
			continue
		}
		name := d.DisplayName
		if name == "" {
			name = d.TypeName
		}
		row := detectionsLatestRow{
			Severity:   triageSeverityShort(d.SeverityLevel),
			Name:       name,
			Object:     d.ObjectName,
			SHA1:       strings.ToLower(strings.TrimSpace(d.ObjectHashSha1)),
			Category:   strings.ToLower(shortEnum(d.Category, "DETECTION_CATEGORY_")),
			DeviceUUID: d.deviceUUID(),
			Source:     source,
			UUID:       d.UUID,
		}
		row.Device = d.Device.DisplayName
		if row.Device == "" {
			row.Device = row.DeviceUUID
		}
		if at, ok := triageParseTime(d.OccurTime); ok {
			row.at = at
			row.OccurredAt = at.UTC().Format(time.RFC3339)
		} else {
			row.OccurredAt = d.OccurTime
		}
		rows = append(rows, row)
	}
	return rows
}

// detectionsLatestSelect filters by severity floor and device, sorts newest
// first (undated rows last) and keeps the first limit rows.
func detectionsLatestSelect(rows []detectionsLatestRow, minSeverity, deviceUUID string, limit int) []detectionsLatestRow {
	out := make([]detectionsLatestRow, 0, len(rows))
	for _, r := range rows {
		if !triageSeverityPasses(r.Severity, minSeverity) {
			continue
		}
		if deviceUUID != "" && !strings.EqualFold(r.DeviceUUID, deviceUUID) {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		zi, zj := out[i].at.IsZero(), out[j].at.IsZero()
		if zi != zj {
			return zj
		}
		if !out[i].at.Equal(out[j].at) {
			return out[i].at.After(out[j].at)
		}
		return out[i].UUID < out[j].UUID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// detectionsLatestEnrich joins device names and customers from the mirror.
// Devices missing from the mirror keep the name the API gave (or the UUID)
// and get the customer "(unknown device)"; a synced device whose group the
// tree lacks gets "(group not in synced tree)". Both are counted in the
// returned unattributed set. Customers sharing a name are told apart by a
// UUID prefix. Enrichment is skipped only when there is no mirror to join
// (no tree, or a mirror holding neither devices nor device groups): an
// empty device mirror beside a synced COMPANY tree (every endpoint deleted
// and resynced) still files each detection as "(unknown device)" and counts
// it (Codex round 4: it returned early, so retained detections showed blank
// customers and unattributed_detections 0).
func detectionsLatestEnrich(rows []detectionsLatestRow, devices []esetDevice, tree *esetTree) esetUnattributed {
	var un esetUnattributed
	if tree == nil || (len(devices) == 0 && len(tree.groups) == 0) {
		return un
	}
	label := detectionsSpreadCustomerLabels(tree)
	byUUID := esetDevicesByUUID(devices)
	for i := range rows {
		if d, ok := byUUID[rows[i].DeviceUUID]; ok {
			rows[i].Device = d.name()
		}
		owner, known := esetDeviceOwner(tree, byUUID, rows[i].DeviceUUID)
		rows[i].Customer = label(owner)
		if !known {
			un.add(rows[i].UUID)
		}
	}
	return un
}

// detectionsLatestCloudWarning explains a failed Cloud Office fetch.
func detectionsLatestCloudWarning(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		return fmt.Sprintf("cloud office detections unavailable (HTTP %d): the API user may lack ESET Cloud Office Security; showing endpoint detections only", apiErr.StatusCode)
	}
	return fmt.Sprintf("cloud office detections could not be fetched (%v); showing endpoint detections only", err)
}

func newNovelDetectionsLatestCmd(flags *rootFlags) *cobra.Command {
	var since, device, minSeverity, dbPath string
	var limit, maxPages int
	var includeCloud bool
	cmd := &cobra.Command{
		Use:   "latest",
		Short: "Newest detections straight from ESET, filtered by severity, with device and customer when synced",
		Long: "Use this command for the newest detections straight from ESET, no sync needed. Do NOT use it\n" +
			"for the ranked morning queue with incidents; use 'triage' instead. Do NOT use it for hash\n" +
			"spread across customers; use 'detections spread' instead.\n\n" +
			"Fetches v1 endpoint detections (ESET PROTECT and Inspect) with occurTime inside --since,\n" +
			"newest first. --device takes a UUID, or a display name or hostname resolved through the\n" +
			"local mirror. --include-cloud-office adds v2 ESET Cloud Office Security detections; if the\n" +
			"API user cannot read them the command warns and still returns the endpoint rows. When a\n" +
			"local mirror exists, device names and customers are joined from it; otherwise the device\n" +
			"column shows the device UUID and customer is blank. With a mirror, a row whose device it\n" +
			"does not hold shows the customer \"(unknown device)\", one whose device group is not in the\n" +
			"synced tree \"(group not in synced tree)\" (\"(parent group not visible)\" when the group is\n" +
			"synced but a parent is not visible to the API user; see 'fleet health'), and all are counted\n" +
			"in unattributed_detections.\n" +
			"A returned row that cannot be decoded or has no uuid is left out and counted in rejected_rows.",
		Example: strings.Trim(`
  eset-protect-cli detections latest
  eset-protect-cli detections latest --since 6h --min-severity high --agent
  eset-protect-cli detections latest --device FRONTDESK-01 --since 7d --limit 50 --include-cloud-office --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "live",
			"pp:typed-exit-codes": "0,2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "detections latest")
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
			if limit < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be at least 1"))
			}
			if maxPages < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--max-pages must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}

			// Optional enrichment: open the mirror only when it exists, so a
			// live-only user sees no sync nag.
			var devices []esetDevice
			var tree *esetTree
			mirrorHint := ""
			mirror := dbPath
			if mirror == "" {
				mirror = defaultDBPath(esetCLIName)
			}
			if _, statErr := os.Stat(mirror); statErr == nil {
				db, err := openEsetStore(cmd, mirror, "devices", "device-groups")
				if err != nil {
					esetFprintf(cmd.ErrOrStderr(), "warning: local mirror unreadable, skipping device names: %v\n", err)
				} else if db != nil {
					defer db.Close()
					mirrorHint = esetMirrorHint(db, "devices", "device-groups")
					groups, gerr := loadEsetGroups(db)
					devs, derr := loadEsetDevices(db)
					if gerr != nil || derr != nil {
						esetFprintf(cmd.ErrOrStderr(), "warning: local mirror unreadable, skipping device names: %v\n", errors.Join(gerr, derr))
					} else {
						devices, tree = devs, newEsetTree(groups)
						if len(devs) == 0 && len(groups) > 0 {
							mirrorHint = esetJoinHints(mirrorHint, "no devices in the local mirror, so every detection's device and customer are unknown; run '"+esetCLIName+" sync --resources devices'")
						}
					}
				}
			}

			deviceUUID := ""
			if q := strings.TrimSpace(device); q != "" {
				switch {
				case detectionsLatestUUIDRE.MatchString(q):
					deviceUUID = strings.ToLower(q)
				case len(devices) == 0:
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--device %q is not a UUID and there is no synced device mirror to resolve the name; pass the device UUID or run '%s sync --resources devices'", q, esetCLIName))
				default:
					d, err := resolveEsetDevice(devices, q)
					if err != nil {
						return usageErr(err)
					}
					deviceUUID = d.UUID
				}
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			startStr := start.UTC().Format(time.RFC3339)
			view := detectionsLatestView{Items: make([]detectionsLatestRow, 0), Since: startStr, Warnings: make([]string, 0), SyncedHint: mirrorHint}
			res, err := esetFetchAll(ctx, c, esetHost("incident-management", "/v1/detections"),
				map[string]string{"startTime": startStr, "deviceUuid": deviceUUID}, "detections", maxPages)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			var rej esetRejects
			rows := detectionsLatestParse(res.Items, detectionsLatestSourceEndpoint, &rej)
			view.TotalFetched, view.Pages, view.Truncated = len(res.Items), res.Pages, res.Truncated

			if includeCloud {
				cres, cerr := esetFetchAll(ctx, c, esetHost("incident-management", "/v2/detections"),
					map[string]string{"startTime": startStr}, "detections", maxPages)
				if cerr != nil {
					msg := detectionsLatestCloudWarning(cerr)
					esetFprintln(cmd.ErrOrStderr(), "warning: "+msg)
					view.Warnings = append(view.Warnings, msg)
				} else {
					rows = append(rows, detectionsLatestParse(cres.Items, detectionsLatestSourceCloudOffice, &rej)...)
					view.TotalFetched += len(cres.Items)
					view.Pages += cres.Pages
					view.Truncated = view.Truncated || cres.Truncated
				}
			}
			if view.Truncated {
				esetFprintf(cmd.ErrOrStderr(), "warning: stopped at --max-pages %d with more detections remaining; the newest rows may be missing - narrow --since or raise --max-pages\n", maxPages)
			}

			view.RejectedRows = rej.N
			rej.warn(cmd)

			selected := detectionsLatestSelect(rows, minSev, deviceUUID, limit)
			un := detectionsLatestEnrich(selected, devices, tree)
			view.UnattributedDetections, view.UnattributedDetectionUUIDs = un.N, un.UUIDs()
			view.parentNotVisible = tree.parentNotVisible()
			view.ParentNotVisible = tree.parentNotVisibleGroups()
			view.Items = append(view.Items, selected...)

			w := cmd.OutOrStdout()
			if !wantsHumanTable(w, flags) {
				return printJSONFiltered(w, view, flags)
			}
			if len(view.Items) == 0 {
				esetFprintf(w, "No detections since %s (%d fetched before filters).\n", view.Since, view.TotalFetched)
				if view.RejectedRows > 0 {
					esetFprintf(w, "%d returned row(s) could not be decoded and were left out.\n", view.RejectedRows)
				}
				if view.SyncedHint != "" {
					esetFprintf(w, "synced data: %s\n", view.SyncedHint)
				}
				return nil
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "OCCURRED\tSEVERITY\tNAME\tOBJECT\tDEVICE\tCUSTOMER\tSOURCE")
			for _, r := range view.Items {
				esetFprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.OccurredAt, r.Severity, triageClip(r.Name, 40), triageClip(r.Object, 40), triageClip(r.Device, 28), triageClip(r.Customer, 24), r.Source)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			esetFprintf(w, "\n%d shown, %d fetched over %d page(s) since %s\n", len(view.Items), view.TotalFetched, view.Pages, view.Since)
			if un := esetUnattributedLine(view.UnattributedDetections, "shown detection(s)", view.UnattributedDetectionUUIDs, "", esetUnattributedResync, view.parentNotVisible); un != "" {
				esetFprintln(w, un)
			}
			if view.RejectedRows > 0 {
				esetFprintf(w, "%d returned row(s) could not be decoded and were left out.\n", view.RejectedRows)
			}
			if view.SyncedHint != "" {
				esetFprintf(w, "synced data: %s\n", view.SyncedHint)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Window to look back over (e.g. 6h, 24h, 7d)")
	cmd.Flags().StringVar(&device, "device", "", "Only this device: UUID, or display name or hostname from the local mirror")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "", "Lowest severity to keep: diagnostic, info, low, medium or high (default: all)")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum rows to return")
	cmd.Flags().IntVar(&maxPages, "max-pages", 5, "Maximum pages of 1000 detections to fetch per source")
	cmd.Flags().BoolVar(&includeCloud, "include-cloud-office", false, "Also fetch v2 ESET Cloud Office Security detections (warns instead of failing when unavailable)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local mirror used for device names ("+esetDBDefault+")")
	return cmd
}
