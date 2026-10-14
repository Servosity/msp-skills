// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-meraki-pp-cli/internal/client"
)

type renewalRow struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	LicensingModel   string `json:"licensing_model"`
	Kind             string `json:"kind"` // co-term | license | subscription | end-of-sale | end-of-support
	Item             string `json:"item"`
	Date             string `json:"date"`
	DaysRemaining    int    `json:"days_remaining"`
	DeviceCount      int    `json:"device_count,omitempty"`
	Status           string `json:"status,omitempty"`
	ObservedAt       string `json:"observed_at"`
	at               time.Time
}

func daysUntil(t, now time.Time) int {
	return int(math.Floor(t.Sub(now).Hours() / 24))
}

// renewalsFromCoterm turns the co-term overview into one org-wide row.
func renewalsFromCoterm(org fleetOrg, raw json.RawMessage, now time.Time) []renewalRow {
	var ov struct {
		Status               string         `json:"status"`
		ExpirationDate       string         `json:"expirationDate"`
		LicensedDeviceCounts map[string]int `json:"licensedDeviceCounts"`
	}
	if json.Unmarshal(raw, &ov) != nil {
		return nil
	}
	t, ok := parseMerakiTime(ov.ExpirationDate)
	if !ok {
		return nil
	}
	devices := 0
	for _, n := range ov.LicensedDeviceCounts {
		devices += n
	}
	return []renewalRow{{OrganizationID: org.ID, OrganizationName: org.Name, LicensingModel: org.LicensingModel,
		Kind: "co-term", Item: "co-termination date", Date: t.Format("2006-01-02"), DaysRemaining: daysUntil(t, now),
		DeviceCount: devices, Status: ov.Status, at: t}}
}

// renewalsFromLicenses groups per-device licenses by type and expiration day.
func renewalsFromLicenses(org fleetOrg, items []json.RawMessage, now time.Time) []renewalRow {
	type key struct{ typ, day string }
	counts := map[key]int{}
	when := map[key]time.Time{}
	for _, raw := range items {
		var l struct {
			LicenseType    string `json:"licenseType"`
			State          string `json:"state"`
			ExpirationDate string `json:"expirationDate"`
		}
		if json.Unmarshal(raw, &l) != nil {
			continue
		}
		if s := strings.ToLower(l.State); s == "unused" || s == "recentlyqueued" {
			continue // not running yet; nothing is about to lapse
		}
		t, ok := parseMerakiTime(l.ExpirationDate)
		if !ok {
			continue
		}
		k := key{l.LicenseType, t.Format("2006-01-02")}
		counts[k]++
		if prev, ok := when[k]; !ok || t.Before(prev) {
			when[k] = t
		}
	}
	rows := make([]renewalRow, 0, len(counts))
	for k, n := range counts {
		rows = append(rows, renewalRow{OrganizationID: org.ID, OrganizationName: org.Name, LicensingModel: org.LicensingModel,
			Kind: "license", Item: k.typ, Date: k.day, DaysRemaining: daysUntil(when[k], now), DeviceCount: n, at: when[k]})
	}
	return rows
}

func renewalsFromSubscriptions(org fleetOrg, items []json.RawMessage, now time.Time) []renewalRow {
	rows := make([]renewalRow, 0)
	for _, raw := range items {
		var s struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			EndDate string `json:"endDate"`
		}
		if json.Unmarshal(raw, &s) != nil {
			continue
		}
		t, ok := parseMerakiTime(s.EndDate)
		if !ok {
			continue
		}
		rows = append(rows, renewalRow{OrganizationID: org.ID, OrganizationName: org.Name, LicensingModel: org.LicensingModel,
			Kind: "subscription", Item: s.Name, Date: t.Format("2006-01-02"), DaysRemaining: daysUntil(t, now), Status: s.Status, at: t})
	}
	return rows
}

// renewalsFromEoX groups inventory devices by model and EoX milestone.
func renewalsFromEoX(org fleetOrg, items []json.RawMessage, now time.Time) []renewalRow {
	type key struct{ kind, model, day string }
	counts := map[key]int{}
	when := map[key]time.Time{}
	for _, raw := range items {
		var d struct {
			Model string `json:"model"`
			EoX   *struct {
				EndOfSaleAt    string `json:"endOfSaleAt"`
				EndOfSupportAt string `json:"endOfSupportAt"`
			} `json:"eox"`
		}
		if json.Unmarshal(raw, &d) != nil || d.EoX == nil {
			continue
		}
		for kind, v := range map[string]string{"end-of-sale": d.EoX.EndOfSaleAt, "end-of-support": d.EoX.EndOfSupportAt} {
			if t, ok := parseMerakiTime(v); ok {
				k := key{kind, d.Model, t.Format("2006-01-02")}
				counts[k]++
				if prev, ok := when[k]; !ok || t.Before(prev) {
					when[k] = t
				}
			}
		}
	}
	rows := make([]renewalRow, 0, len(counts))
	for k, n := range counts {
		rows = append(rows, renewalRow{OrganizationID: org.ID, OrganizationName: org.Name, LicensingModel: org.LicensingModel,
			Kind: k.kind, Item: k.model, Date: k.day, DaysRemaining: daysUntil(when[k], now), DeviceCount: n, at: when[k]})
	}
	return rows
}

// fetchOrgRenewals reads only the sources the requested kinds need: the
// licensing source for license kinds, inventory for end-of-sale/support.
func fetchOrgRenewals(ctx context.Context, c *client.Client, org fleetOrg, includeLicense, includeEoX bool, now time.Time) (partialRows[renewalRow], error) {
	var out partialRows[renewalRow]
	if !org.APIEnabled {
		return out, errOrgAPIDisabled
	}
	rows := make([]renewalRow, 0)
	model := strings.ToLower(org.LicensingModel)
	if !includeLicense {
		model = ""
	}
	switch model {
	case "co-term":
		raw, err := c.GetNoCache(ctx, orgPath("/organizations/{organizationId}/licenses/overview", org.ID), nil)
		if err != nil {
			return out, fmt.Errorf("license overview: %w", err)
		}
		rows = append(rows, renewalsFromCoterm(org, raw, now)...)
	case "per-device":
		res, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/licenses", org.ID), nil, 0)
		if err != nil {
			return out, fmt.Errorf("licenses: %w", err)
		}
		rows = append(rows, renewalsFromLicenses(org, res.Items, now)...)
	case "subscription":
		res, err := merakiGetAll(ctx, c, "/administered/licensing/subscription/subscriptions", map[string]string{"organizationIds[]": org.ID}, 0)
		if err != nil {
			return out, fmt.Errorf("subscriptions: %w", err)
		}
		rows = append(rows, renewalsFromSubscriptions(org, res.Items, now)...)
	}
	if includeEoX {
		res, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/inventory/devices", org.ID), map[string]string{"usedState": "used"}, 0)
		if err != nil {
			out.Rows, out.Partial = rows, "inventory (EoX dates): "+err.Error()
			return out, nil
		}
		rows = append(rows, renewalsFromEoX(org, res.Items, now)...)
	}
	out.Rows = rows
	return out, nil
}

// filterRenewals keeps deadlines no later than now+within, comparing exact
// timestamps (rounded days_remaining would admit a 36h deadline under 24h).
func filterRenewals(rows []renewalRow, now time.Time, within time.Duration, includeExpired bool, kinds map[string]bool) []renewalRow {
	cutoff := now.Add(within)
	out := make([]renewalRow, 0, len(rows))
	for _, r := range rows {
		if r.at.After(cutoff) || (!includeExpired && r.at.Before(now)) {
			continue
		}
		if len(kinds) > 0 && !kinds[r.Kind] {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DaysRemaining != out[j].DaysRemaining {
			return out[i].DaysRemaining < out[j].DaysRemaining
		}
		return strings.ToLower(out[i].OrganizationName) < strings.ToLower(out[j].OrganizationName)
	})
	return out
}

func newNovelFleetRenewalsCmd(flags *rootFlags) *cobra.Command {
	var orgFilter []string
	var within string
	var includeExpired, noEoX bool
	var kindFlag []string
	cmd := &cobra.Command{
		Use:   "renewals",
		Short: "List license, subscription and end-of-sale/support deadlines across every org, soonest first (--within, --kind)",
		Long: strings.TrimSpace(`
One date-sorted list of every Meraki deadline across all organizations the API
key can see. The license source follows each org's licensing model: the
co-termination date for co-term orgs, per-license expirations (grouped by type
and day) for per-device orgs, and subscription end dates for subscription orgs.
End-of-sale and end-of-support dates come from the org's inventory, grouped by
model. Expired items (negative days_remaining) are included unless
--include-expired=false.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet renewals --within 90d --agent
  cisco-meraki-cli fleet renewals --within 365d --kind end-of-support --csv
  cisco-meraki-cli fleet renewals --org 549236 --no-eox --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet renewals")
			}
			win, err := parseWindow("within", within)
			if err != nil {
				return err
			}
			kinds := map[string]bool{}
			valid := map[string]bool{"co-term": true, "license": true, "subscription": true, "end-of-sale": true, "end-of-support": true}
			for _, k := range kindFlag {
				for _, part := range strings.Split(k, ",") {
					part = strings.ToLower(strings.TrimSpace(part))
					if part == "" {
						continue
					}
					if !valid[part] {
						return usageErr(fmt.Errorf("--kind %q: use co-term, license, subscription, end-of-sale or end-of-support", part))
					}
					kinds[part] = true
				}
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, orgs, err := fleetClientOrgs(ctx, cmd, flags, orgFilter)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			anyKind := func(ks ...string) bool {
				if len(kinds) == 0 {
					return true
				}
				for _, k := range ks {
					if kinds[k] {
						return true
					}
				}
				return false
			}
			needLicense := anyKind("co-term", "license", "subscription")
			needEoX := !noEoX && anyKind("end-of-sale", "end-of-support")
			outcomes := fanOrgs(ctx, orgs, func(ctx context.Context, o fleetOrg) (partialRows[renewalRow], error) {
				return fetchOrgRenewals(ctx, c, o, needLicense, needEoX, now)
			})
			rows := make([]renewalRow, 0)
			for _, oc := range outcomes {
				warnPartial(cmd.ErrOrStderr(), oc.Org, oc.Value.Partial)
				for _, r := range oc.Value.Rows {
					r.ObservedAt = now.Format(time.RFC3339)
					rows = append(rows, r)
				}
			}
			reportFleetFailures(cmd.ErrOrStderr(), outcomes)
			rows = filterRenewals(rows, now, win, includeExpired, kinds)
			return printFleetRows(cmd, flags, rows, []string{"days_remaining", "date", "organization_name", "kind", "item", "device_count"}, fmt.Sprintf("No license, subscription or EoX deadlines within %s.", within))
		},
	}
	addOrgFlag(cmd, &orgFilter)
	cmd.Flags().StringVar(&within, "within", "90d", "Look-ahead window, e.g. 30d, 90d, 365d")
	cmd.Flags().BoolVar(&includeExpired, "include-expired", true, "Include items already past their date (negative days_remaining)")
	cmd.Flags().BoolVar(&noEoX, "no-eox", false, "Skip the inventory end-of-sale/support lookup (one fewer call per org)")
	cmd.Flags().StringSliceVar(&kindFlag, "kind", nil, "Only these kinds: co-term, license, subscription, end-of-sale, end-of-support")
	return cmd
}
