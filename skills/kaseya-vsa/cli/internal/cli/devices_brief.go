// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"kaseya-vsa-pp-cli/internal/client"
)

type briefSectionFailure struct {
	Section string `json:"section"`
	Error   string `json:"error"`
}

type deviceBrief struct {
	Device              map[string]any   `json:"device"`
	NotificationCounts  map[string]any   `json:"notification_counts"`
	LastSeenOnline      any              `json:"last_seen_online"`
	Security            json.RawMessage  `json:"security"`
	Disks               json.RawMessage  `json:"disks"`
	PendingUpdates      json.RawMessage  `json:"pending_updates"`
	Antivirus           json.RawMessage  `json:"antivirus"`
	AppliedPolicies     []map[string]any `json:"applied_policies"`
	RecentNotifications json.RawMessage  `json:"recent_notifications"`
	RecentAudit         []auditDigestRow `json:"recent_audit"`
	Partial             bool             `json:"partial"`
	sectionErrs         []error
	FetchFailures       []briefSectionFailure `json:"fetch_failures"`
}

// deviceBriefFields are the device record fields the digest keeps. Custom
// fields are deliberately excluded: MSPs keep secrets in them.
var deviceBriefFields = []string{"Identifier", "Name", "OrganizationName", "SiteName", "GroupName", "Description",
	"IsOnline", "Uptime", "ComputerType", "InMaintenance", "ClientVersion", "ExternalIpAddress", "IsAgentInstalled"}

func newNovelDevicesBriefCmd(flags *rootFlags) *cobra.Command {
	var notifications, auditEntries int

	cmd := &cobra.Command{
		Use:   "brief [device]",
		Short: "Get one device's online state, notifications, security posture, disks, pending updates, antivirus, policies",
		Long: `Builds one compact digest of a single device from six live calls: the device record, its asset
security, disk and update-count sections, antivirus status, applied policies, its newest
notifications, and its newest audit-log entries. The device can be its name or its Identifier.

The digest never requests the asset AssetInfo section (it carries the Windows product key) or the
device's custom fields (MSPs keep secrets there). A section that fails is reported in
fetch_failures, partial is set, and the command exits non-zero.

Use this command for the current state and recent activity of ONE device. Do NOT use this command
for fleet-wide exception lists; use 'health' instead.`,
		Example: strings.Trim(`
  kaseya-vsa-cli devices brief FRONTDESK-01 --agent
  kaseya-vsa-cli devices brief 11111111-2222-3333-4444-555555555555 --notifications 5 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "device=example-value"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "devices brief")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("device name or Identifier is required"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if err := requireTenantBaseURL(c); err != nil {
				return err
			}
			c.NoCache = true // a live digest must not be served from the response cache
			id, _, err := resolveDeviceID(ctx, flags, c, args[0])
			if err != nil {
				return err
			}
			brief, err := buildDeviceBrief(ctx, c, id, notifications, auditEntries)
			if err != nil {
				return err
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if perr := printJSONFiltered(cmd.OutOrStdout(), brief, flags); perr != nil {
					return perr
				}
			} else {
				printBriefHuman(cmd, brief)
			}
			if brief.Partial {
				// Auth (4) outranks rate limit (7), which outranks a generic API failure (5), across all sections.
				msg := fmt.Errorf("%d of 6 sections failed; see fetch_failures", len(brief.FetchFailures))
				code := 5
				for _, e := range brief.sectionErrs {
					var typed *cliError
					if errors.As(classifyAPIErrorOnly(e), &typed) {
						if typed.code == 4 || (typed.code == 7 && code != 4) {
							code = typed.code
						}
					}
				}
				return &cliError{code: code, err: msg}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&notifications, "notifications", 10, "Number of newest device notifications to include")
	cmd.Flags().IntVar(&auditEntries, "audit", 10, "Number of newest audit-log entries for the device to include")
	return cmd
}

// buildDeviceBrief fetches the six sections concurrently. A 404 on the device
// record itself is returned as not-found; any other section failure is kept
// in fetch_failures so the caller still sees everything that did load.
func buildDeviceBrief(ctx context.Context, c *client.Client, id string, nNotifications, nAudit int) (*deviceBrief, error) {
	type section struct {
		name   string
		path   string
		params map[string]string
	}
	sections := []section{
		{"device", "/devices/" + id, nil},
		{"assets", "/assets/" + id, map[string]string{"include": "Security,Disks,Updates"}},
		{"antivirus", "/devices/" + id + "/antivirus", nil},
		{"applied_policies", "/devices/" + id + "/appliedpolicies", nil},
		{"notifications", "/devices/" + id + "/notifications", map[string]string{"$top": fmt.Sprint(nNotifications), "$orderby": "DateTime desc"}},
		{"audit", "/auditlogs", map[string]string{"$filter": "DeviceIdentifier eq " + odataQuote(id), "$top": fmt.Sprint(nAudit), "$orderby": "AuditLogRecordId desc"}},
	}
	results := make([]json.RawMessage, len(sections))
	errs := make([]error, len(sections))
	var wg sync.WaitGroup
	for i, s := range sections {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = vsaGet(ctx, c, s.path, s.params)
		}()
	}
	wg.Wait()
	if errs[0] != nil {
		// Device not found stays not-found (3); otherwise auth (4) outranks rate limit (7)
		// across every section, so a device 500 cannot hide an antivirus 403.
		first := classifyAPIErrorOnly(errs[0])
		var typed *cliError
		if errors.As(first, &typed) && typed.code == 3 {
			return nil, first
		}
		code := 5
		for _, e := range errs {
			if e != nil && errors.As(classifyAPIErrorOnly(e), &typed) && (typed.code == 4 || (typed.code == 7 && code != 4)) {
				code = typed.code
			}
		}
		return nil, &cliError{code: code, err: errs[0]}
	}
	b := &deviceBrief{Device: map[string]any{}, NotificationCounts: map[string]any{},
		AppliedPolicies: make([]map[string]any, 0), RecentAudit: make([]auditDigestRow, 0),
		FetchFailures: make([]briefSectionFailure, 0), RecentNotifications: json.RawMessage("[]")}
	for i, s := range sections {
		if errs[i] != nil {
			b.sectionErrs = append(b.sectionErrs, errs[i])
			b.FetchFailures = append(b.FetchFailures, briefSectionFailure{Section: s.name, Error: errs[i].Error()})
			continue
		}
		if err := applyBriefSection(b, s.name, results[i]); err != nil {
			b.FetchFailures = append(b.FetchFailures, briefSectionFailure{Section: s.name, Error: failureText(c, err)})
		}
	}
	sort.Slice(b.FetchFailures, func(i, j int) bool { return b.FetchFailures[i].Section < b.FetchFailures[j].Section })
	b.Partial = len(b.FetchFailures) > 0
	return b, nil
}

func applyBriefSection(b *deviceBrief, name string, data json.RawMessage) error {
	switch name {
	case "device":
		var d map[string]any
		if err := json.Unmarshal(data, &d); err != nil {
			return fmt.Errorf("decode device: %w", err)
		}
		for _, k := range deviceBriefFields {
			if v, ok := d[k]; ok {
				b.Device[k] = v
			}
		}
		for _, k := range []string{"Critical", "Elevated", "Normal", "Low"} {
			if v, ok := d[k+"Notifications"]; ok {
				b.NotificationCounts[strings.ToLower(k)] = v
			}
		}
	case "assets":
		var a struct {
			LastSeenOnline any             `json:"LastSeenOnline"`
			Security       json.RawMessage `json:"Security"`
			Disks          json.RawMessage `json:"Disks"`
			Updates        json.RawMessage `json:"Updates"`
		}
		if err := json.Unmarshal(data, &a); err != nil {
			return fmt.Errorf("decode assets: %w", err)
		}
		b.LastSeenOnline, b.Security, b.Disks, b.PendingUpdates = a.LastSeenOnline, a.Security, a.Disks, a.Updates
	case "antivirus":
		b.Antivirus = data
	case "applied_policies":
		var ps []map[string]any
		if err := json.Unmarshal(data, &ps); err != nil {
			return fmt.Errorf("decode applied policies: %w", err)
		}
		for _, p := range ps {
			b.AppliedPolicies = append(b.AppliedPolicies, map[string]any{"type": p["PolicyType"], "id": p["PolicyId"], "name": p["PolicyName"]})
		}
	case "notifications":
		if len(data) > 0 && string(data) != "null" {
			b.RecentNotifications = data
		}
	case "audit":
		var es []auditEntry
		if err := json.Unmarshal(data, &es); err != nil {
			return fmt.Errorf("decode audit: %w", err)
		}
		for _, e := range es {
			b.RecentAudit = append(b.RecentAudit, auditDigestRow{RecordID: e.RecordID, Time: e.DateTime, User: e.User,
				Category: auditCategoryName(e.CategoryID), EventType: auditEventTypeName(e.EventTypeID), Event: e.Description})
		}
	}
	return nil
}

func printBriefHuman(cmd *cobra.Command, b *deviceBrief) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%v (%v) org=%v site=%v online=%v uptime=%v\n", b.Device["Name"], b.Device["Identifier"],
		b.Device["OrganizationName"], b.Device["SiteName"], b.Device["IsOnline"], b.Device["Uptime"])
	fmt.Fprintf(w, "last seen: %v  notifications: %v\n", b.LastSeenOnline, b.NotificationCounts)
	fmt.Fprintf(w, "pending updates: %s\nantivirus: %s\n", string(b.PendingUpdates), string(b.Antivirus))
	fmt.Fprintf(w, "security: %s\ndisks: %s\n", string(b.Security), string(b.Disks))
	fmt.Fprintf(w, "recent notifications: %s\n", string(b.RecentNotifications))
	for _, p := range b.AppliedPolicies {
		fmt.Fprintf(w, "policy: %v = %v\n", p["type"], p["name"])
	}
	for _, a := range b.RecentAudit {
		fmt.Fprintf(w, "audit: %s %s %s %s\n", a.Time, a.User, a.Category, a.Event)
	}
	for _, f := range b.FetchFailures {
		fmt.Fprintf(w, "FAILED %s: %s\n", f.Section, f.Error)
	}
}
