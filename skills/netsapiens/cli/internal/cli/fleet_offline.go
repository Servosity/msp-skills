// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"netsapiens-pp-cli/internal/cliutil"
)

type offlineIssue struct {
	Domain            string   `json:"domain"`
	Kind              string   `json:"kind"` // device | phone
	ID                string   `json:"id"`   // device name or MAC
	User              string   `json:"user,omitempty"`
	UserName          string   `json:"user_name,omitempty"`
	Reasons           []string `json:"reasons"`
	RegistrationState string   `json:"registration_state,omitempty"`
	ExpiresAt         string   `json:"registration_expires,omitempty"`
	LatencyAvgSeconds float64  `json:"latency_avg_seconds,omitempty"`
	LatencyThreshold  float64  `json:"latency_threshold_seconds,omitempty"`
	UserAgent         string   `json:"user_agent,omitempty"`
	Model             string   `json:"model,omitempty"`
	LastRetrieved     string   `json:"last_provisioned,omitempty"`
}

// offlineIssues flags devices that are unregistered, whose registration had
// already expired when they were synced, or whose average SIP latency exceeds
// maxLatency; and NDP phones never provisioned or not provisioned within stale.
func offlineIssues(devices, users, phones []fleetRow, maxLatency float64, stale time.Duration, now time.Time) []offlineIssue {
	byUser := indexBy(users, "user")
	out := make([]offlineIssue, 0)
	for _, d := range devices {
		if d.fyes("device-sip-registration-ignore-report-enabled") {
			continue
		}
		var reasons []string
		state := d.fstr("device-sip-registration-state")
		switch {
		case strings.EqualFold(state, "unregistered"):
			reasons = append(reasons, "unregistered")
		case state != "" && !strings.EqualFold(state, "registered"):
			reasons = append(reasons, "registration-state:"+state) // report the observed value, not a guess
		}
		exp := d.fstr("device-sip-registration-expires-datetime")
		if t, ok := parseNSTime(exp); ok && strings.EqualFold(state, "registered") {
			ref := now
			if s, ok := parseNSTime(d.fstr("_synced_at")); ok {
				ref = s
			}
			if t.Before(ref) {
				reasons = append(reasons, "registration-expired")
			}
		}
		lat, _ := d.fnum("device-sip-latency-seconds-average")
		if maxLatency > 0 && lat > maxLatency {
			reasons = append(reasons, "high-latency")
		}
		if len(reasons) == 0 {
			continue
		}
		dom, user := d.domainOf(), d.fstr("user")
		out = append(out, offlineIssue{
			Domain: dom, Kind: "device", ID: d.fstr("device"), User: user,
			UserName: fullName(byUser[key(dom, user)]), Reasons: reasons,
			RegistrationState: state, ExpiresAt: exp, LatencyAvgSeconds: lat, LatencyThreshold: maxLatency,
			UserAgent: d.fstr("device-sip-registration-user-agent"),
			Model:     d.fstr("device-models-model"),
		})
	}
	// /phones and /domains/{d}/phones overlap: merge each MAC's rows by their best
	// known observation (a valid timestamp beats an explicit empty one; a NULL,
	// absent or unparseable timestamp is unknown and never means "never").
	type ndpObs struct {
		row      fleetRow
		latest   time.Time
		haveTime bool
		empty    bool
	}
	byMAC := map[string]*ndpObs{}
	var macs []string
	for _, p := range phones {
		mac := normMAC(p.fstr("device-provisioning-mac-address"))
		if mac == "" {
			continue
		}
		o := byMAC[mac]
		if o == nil {
			o = &ndpObs{row: p}
			byMAC[mac], macs = o, append(macs, mac)
		}
		if !has(p, "device-provisioning-last-retrieved-datetime") {
			continue
		}
		last := p.fstr("device-provisioning-last-retrieved-datetime")
		if t, ok := parseNSTime(last); ok {
			if !o.haveTime || t.After(o.latest) {
				o.latest, o.haveTime, o.row = t, true, p
			}
		} else if strings.TrimSpace(last) == "" {
			o.empty = true
		}
	}
	for _, mac := range macs {
		o := byMAC[mac]
		var reason string
		switch {
		case o.haveTime:
			if stale > 0 && now.Sub(o.latest) > stale {
				reason = "provisioning-stale"
			}
		case o.empty:
			reason = "never-provisioned"
		}
		if reason == "" {
			continue
		}
		out = append(out, offlineIssue{
			Domain: o.row.domainOf(), Kind: "phone", ID: mac, Reasons: []string{reason},
			Model: o.row.fstr("device-models-brand-and-model"), LastRetrieved: o.row.fstr("device-provisioning-last-retrieved-datetime"),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// offlineReport is the machine result: the issues plus whether the device and
// MAC tables behind them were fully synced.
type offlineReport struct {
	auditCoverage
	Issues []offlineIssue `json:"issues"`
}

func newNovelFleetOfflineCmd(flags *rootFlags) *cobra.Command {
	var dbPath, domain, staleProvision string
	var maxLatency float64
	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Phones and devices that are unregistered, expired, high-latency, or never provisioned, by client",
		Long: `Use this command for a fleet-wide list of phones or devices that are unregistered, stale, high-latency, or never provisioned.
Do NOT use this command to look up one specific number, extension, or MAC; use 'trace' instead.

Devices flagged with device-sip-registration-ignore-report-enabled are skipped. Registration
expiry is judged against the time each device was synced.`,
		Example: strings.Trim(`
  netsapiens-cli fleet offline --agent
  netsapiens-cli fleet offline --domain acme.example --max-latency 0.25
  netsapiens-cli fleet offline --stale-provision 30d --select issues.domain,issues.id,issues.reasons`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet offline")
			}
			var stale time.Duration
			if staleProvision != "" && staleProvision != "0" {
				d, err := cliutil.ParseDurationLoose(staleProvision)
				if err != nil {
					return usageErr(fmt.Errorf("--stale-provision: %w (use values like 14d or 72h)", err))
				}
				stale = d
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openFleetStore(ctx, cmd, flags, dbPath, offlineReport{auditCoverage: noMirrorCoverage(), Issues: []offlineIssue{}})
			if db == nil || err != nil {
				return err
			}
			defer db.Close()
			hintCoverage(cmd, db, rtDevices, rtUsers)
			// Only covered tables are read: a row left by a cut-short or filtered sync
			// could report a phone that has since registered or been removed.
			cov := fleetCov{}
			devices, err := loadCovered(ctx, db, cov, rtDevices)
			if err != nil {
				return err
			}
			users, err := loadCovered(ctx, db, cov, rtUsers)
			if err != nil {
				return err
			}
			phones, err := loadCovered(ctx, db, cov, rtPhones...)
			if err != nil {
				return err
			}
			if !cov.anyCovered(rtPhones...) { // either phone table covers the MACs
				hintCoverage(cmd, db, "phones")
			}
			ac := fleetAuditCoverage(cmd, flags, db, cov, []string{rtDevices}, rtPhones)
			issues := offlineIssues(filterDomain(devices, domain), users, filterDomain(phones, domain), maxLatency, stale, time.Now().UTC())
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				// keep: --agent/--compact drop keys present on few rows; these are the evidence for each reason.
				return printFleetAudit(cmd, flags, ac, offlineReport{auditCoverage: ac, Issues: issues}, issues, "user", "user_name", "registration_expires", "latency_avg_seconds", "latency_threshold_seconds", "user_agent", "model", "last_provisioned")
			}
			printIncomplete(cmd, ac)
			if len(issues) == 0 {
				if ac.Complete {
					fmt.Fprintln(cmd.OutOrStdout(), "No offline, expired, high-latency or unprovisioned phones in the local mirror.")
				}
				return nil
			}
			table := make([]map[string]any, 0, len(issues))
			for _, i := range issues {
				table = append(table, map[string]any{"domain": i.Domain, "kind": i.Kind, "id": i.ID, "user": strings.TrimSpace(i.User + " " + i.UserName), "reasons": strings.Join(i.Reasons, ",")})
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI data directory)")
	cmd.Flags().StringVar(&domain, "domain", "", "Only this client domain (for example acme.example)")
	cmd.Flags().Float64Var(&maxLatency, "max-latency", 0.3, "Flag devices whose average SIP latency exceeds this many seconds; 0 disables")
	cmd.Flags().StringVar(&staleProvision, "stale-provision", "14d", "Flag MACs whose last config pull is older than this (e.g. 14d, 72h); 0 disables")
	return cmd
}
