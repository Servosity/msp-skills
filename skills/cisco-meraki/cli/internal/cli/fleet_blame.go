// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type blameSuspect struct {
	TS            string `json:"ts"`
	MinutesBefore int64  `json:"minutes_before"`
	AdminName     string `json:"admin_name"`
	AdminEmail    string `json:"admin_email"`
	Page          string `json:"page"`
	Label         string `json:"label"`
	SSIDName      string `json:"ssid_name,omitempty"`
	OldValue      string `json:"old_value"`
	NewValue      string `json:"new_value"`
}

type blameOutage struct {
	OrganizationID string         `json:"organization_id"`
	NetworkID      string         `json:"network_id"`
	NetworkName    string         `json:"network_name"`
	OutageAt       string         `json:"outage_at"`
	Serial         string         `json:"serial"`
	DeviceName     string         `json:"device_name"`
	Status         string         `json:"status"`
	SuspectCount   int            `json:"suspect_count"`
	Suspects       []blameSuspect `json:"suspects"`
}

// correlateOutages attaches to each transition into offline/alerting every
// change made, within window before it, on the network the device was in at
// that moment (history events carry their own network, which matters after a
// device moves), closest first. changesByNet is keyed by network ID.
func correlateOutages(orgID string, history []availabilityChange, changesByNet map[string][]rawChange, window time.Duration) []blameOutage {
	type parsed struct {
		t  time.Time
		ch rawChange
	}
	byNet := map[string][]parsed{}
	for net, changes := range changesByNet {
		for _, ch := range changes {
			if t, ok := parseMerakiTime(ch.TS); ok {
				byNet[net] = append(byNet[net], parsed{t, ch})
			}
		}
	}
	out := make([]blameOutage, 0)
	for _, h := range history {
		st := statusOf(h.Details.New)
		if st != "offline" && st != "alerting" {
			continue
		}
		at, ok := parseMerakiTime(h.TS)
		if !ok {
			continue
		}
		o := blameOutage{OrganizationID: orgID, NetworkID: h.Network.ID, NetworkName: h.Network.Name, OutageAt: at.Format(time.RFC3339),
			Serial: h.Device.Serial, DeviceName: h.Device.Name, Status: st, Suspects: make([]blameSuspect, 0)}
		for _, p := range byNet[h.Network.ID] {
			if p.t.After(at) || at.Sub(p.t) > window {
				continue
			}
			o.Suspects = append(o.Suspects, blameSuspect{TS: p.ch.TS, MinutesBefore: int64(at.Sub(p.t) / time.Minute),
				AdminName: p.ch.AdminName, AdminEmail: p.ch.AdminEmail, Page: p.ch.Page, Label: p.ch.Label,
				SSIDName: p.ch.SSIDName, OldValue: p.ch.OldValue, NewValue: p.ch.NewValue})
		}
		sort.Slice(o.Suspects, func(i, j int) bool { return o.Suspects[i].MinutesBefore < o.Suspects[j].MinutesBefore })
		o.SuspectCount = len(o.Suspects)
		out = append(out, o)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].OutageAt > out[j].OutageAt })
	return out
}

func newNovelFleetBlameCmd(flags *rootFlags) *cobra.Command {
	var since, window string
	var onlySuspects bool
	cmd := &cobra.Command{
		Use:   "blame [network-id|serial]",
		Short: "Find the config changes made just before each time one network or device went offline or started alerting (--since, --window)",
		Long: strings.TrimSpace(`
Takes a network ID (N_/L_...) or a device serial, finds every transition into
offline or alerting in the device availability history (up to 31 days back),
and lists the configuration changes made on that network within --window
before each one, closest first, with admin and old/new values. The org is
resolved from the network, so no --org is needed.

Use this command to find the configuration changes made shortly before a network or device went offline or started alerting.
Do NOT use this command for a general who-changed-what listing; use 'fleet changes' instead.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet blame L_646829496481105433 --window 2h --agent
  cisco-meraki-cli fleet blame Q2QN-9J8L-SLPD --since 7d --only-suspects --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "target=L_646829496481105433"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet blame")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("one network ID or device serial is required"))
			}
			sinceD, err := parseWindow("since", since)
			if err != nil {
				return err
			}
			windowD, err := parseWindow("window", window)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			histStart, histEnd, clamped := fixedWindow(now, sinceD, maxAvailabilityLookback)
			if clamped {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: availability history looks back at most 31 days; --since was clamped")
			}
			changeLookback := histEnd.Sub(histStart) + windowD
			if changeLookback > maxChangeLookback {
				return usageErr(fmt.Errorf("--since plus --window reaches back %s, past the change log's 365-day lookback; shorten --window", humanDuration(changeLookback)))
			}
			kind, value := classifyIdentifier(args[0])
			if kind != "network_id" && kind != "serial" {
				return usageErr(fmt.Errorf("%q is not a network ID (N_/L_...) or a Meraki serial (Qxxx-xxxx-xxxx)", args[0]))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			c.NoCache = true
			netID := value
			historyFilter := map[string]string{"t0": histStart.Format(time.RFC3339), "t1": histEnd.Format(time.RFC3339)}
			if kind == "serial" {
				raw, err := c.GetNoCache(ctx, "/devices/"+url.PathEscape(value), nil)
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				var d struct {
					NetworkID string `json:"networkId"`
				}
				if json.Unmarshal(raw, &d) != nil || d.NetworkID == "" {
					return notFoundErr(fmt.Errorf("device %s is not assigned to a network", value))
				}
				netID = d.NetworkID
				historyFilter["serials[]"] = value
			} else {
				historyFilter["networkIds[]"] = netID
			}
			raw, err := c.GetNoCache(ctx, "/networks/"+url.PathEscape(netID), nil)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			var n struct {
				OrganizationID string `json:"organizationId"`
			}
			if json.Unmarshal(raw, &n) != nil || n.OrganizationID == "" {
				return fmt.Errorf("could not resolve the organization for network %s", netID)
			}
			hist, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/devices/availabilities/changeHistory", n.OrganizationID), historyFilter, 0)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), fmt.Errorf("availability history: %w", err), flags)
			}
			history := make([]availabilityChange, 0, len(hist.Items))
			nets := map[string]bool{}
			for _, r := range hist.Items {
				var h availabilityChange
				if json.Unmarshal(r, &h) == nil {
					if h.Network.ID == "" {
						h.Network.ID = netID
					}
					history = append(history, h)
					if st := statusOf(h.Details.New); st == "offline" || st == "alerting" {
						nets[h.Network.ID] = true
					}
				}
			}
			// Changes are read per network the device was in, back to window
			// before the oldest possible outage, with the same fixed end.
			chgStart, chgEnd, _ := fixedWindow(now, changeLookback, maxChangeLookback)
			changesByNet := map[string][]rawChange{}
			for net := range nets {
				chg, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/configurationChanges", n.OrganizationID),
					map[string]string{"t0": chgStart.Format(time.RFC3339), "t1": chgEnd.Format(time.RFC3339), "networkId": net}, 0)
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), fmt.Errorf("configuration changes for %s: %w", net, err), flags)
				}
				for _, r := range chg.Items {
					var ch rawChange
					if json.Unmarshal(r, &ch) == nil {
						changesByNet[net] = append(changesByNet[net], ch)
					}
				}
			}
			outages := correlateOutages(n.OrganizationID, history, changesByNet, windowD)
			if onlySuspects {
				kept := make([]blameOutage, 0, len(outages))
				for _, o := range outages {
					if o.SuspectCount > 0 {
						kept = append(kept, o)
					}
				}
				outages = kept
			}
			return printFleetRows(cmd, flags, outages, []string{"outage_at", "serial", "device_name", "status", "suspect_count"}, "No offline or alerting transitions in the window.")
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "How far back to look for outages, e.g. 24h, 7d (max 31d)")
	cmd.Flags().StringVar(&window, "window", "2h", "How long before each outage a change counts as a suspect, e.g. 30m, 2h, 1d")
	cmd.Flags().BoolVar(&onlySuspects, "only-suspects", false, "Only outages with at least one suspect change")
	return cmd
}
