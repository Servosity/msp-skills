// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"auvik-pp-cli/internal/cliutil"
)

// WHY THIS COMMAND IS NOT `configuration grep`
//
// The original plan was a fleet-wide regex over device config bodies, including
// an inverted mode to prove which devices were MISSING a required line. That
// cannot be built on this API: Auvik's Configuration resource publishes exactly
// two attributes -- backupTime and isRunning. Config bodies are not exposed
// anywhere in either OpenAPI document. A grep implemented against those fields
// would have reported every device as non-compliant while never reading a byte
// of configuration.
//
// What IS answerable from backup metadata is the question underneath it: which
// devices are not protected. No backup at all, a backup that has gone stale, or
// no running-config backup are all real fleet-wide compliance findings, and all
// three are invisible in Auvik's per-device UI.
//
// TestNoConfigBodyExists fails if Auvik ever adds a body field, which is the
// signal to revisit a real grep.

type configFinding struct {
	Finding    string `json:"finding"`
	Client     string `json:"client"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	DeviceType string `json:"device_type,omitempty"`
	LastBackup string `json:"last_backup,omitempty"`
	AgeDays    *int   `json:"age_days,omitempty"`
	Backups    int    `json:"backups"`
	Detail     string `json:"detail"`
}

type configAuditReport struct {
	DevicesTotal   int             `json:"devices_total"`
	ConfigsTotal   int             `json:"configs_total"`
	Protected      int             `json:"protected"`
	StaleAfterDays int             `json:"stale_after_days"`
	Counts         map[string]int  `json:"counts"`
	Findings       []configFinding `json:"findings"`
	Note           string          `json:"note,omitempty"`
}

const (
	findingNoBackup   = "no_backup"
	findingStale      = "stale_backup"
	findingNotRunning = "no_running_config"
)

func newNovelConfigurationAuditCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath  string
		staleIn string
		client  string
		finding string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Find devices with no configuration backup, a stale backup, or no running-config backup, across every client.",
		Long: strings.Trim(`
Audits configuration-backup coverage across every client from the local mirror
and reports three findings:

  no_backup          the device has no stored configuration backup at all
  stale_backup       its newest backup is older than --stale
  no_running_config  it has backups, but none is flagged as the running config

NOTE ON SCOPE: Auvik's Configuration API exposes backup METADATA only
(backupTime, isRunning). Configuration bodies are not available from the API, so
this command cannot search config text -- it audits whether devices are backed
up at all, which is the fleet-wide question the per-device UI cannot answer.

Use this command for configuration-backup coverage and staleness across
clients. Do NOT use this command for the change history of one device; use
'changes' instead.

Reads the local mirror. Run this first:
  auvik-cli sync --resources tenants,inventory,inventory-configuration --full
`, "\n"),
		Example: strings.Trim(`
  # Everything not properly backed up, worst first
  auvik-cli configuration audit --agent

  # Only devices with no backup whatsoever
  auvik-cli configuration audit --finding no_backup --agent

  # Treat anything older than a week as stale, one client
  auvik-cli configuration audit --stale 7d --client acme
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--limit=5",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "configuration audit")
			}
			if finding != "" && finding != findingNoBackup && finding != findingStale && finding != findingNotRunning {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--finding must be one of %s, %s, %s",
					findingNoBackup, findingStale, findingNotRunning))
			}
			stale, err := cliutil.ParseDurationLoose(staleIn)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--stale %q: %w", staleIn, err))
			}

			empty := configAuditReport{Counts: map[string]int{}, Findings: []configFinding{}}
			db, handled, err := openLocalMirror(cmd, flags, dbPath, empty)
			if err != nil || handled {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "inventory-configuration") {
				hintIfStale(cmd, db, "inventory-configuration", flags.maxAge)
			}

			ctx := cmd.Context()
			devices, err := loadResources(ctx, db, rtDevices)
			if err != nil {
				return err
			}
			configs, err := loadResources(ctx, db, rtConfigurations)
			if err != nil {
				return err
			}
			names := tenantNames(ctx, db)

			report := buildConfigAuditReport(devices, configs, names, time.Now().UTC(), stale)

			out := report.Findings
			if finding != "" {
				filtered := make([]configFinding, 0, len(out))
				for _, f := range out {
					if f.Finding == finding {
						filtered = append(filtered, f)
					}
				}
				out = filtered
			}
			if client != "" {
				needle := strings.ToLower(client)
				filtered := make([]configFinding, 0, len(out))
				for _, f := range out {
					if strings.Contains(strings.ToLower(f.Client), needle) {
						filtered = append(filtered, f)
					}
				}
				out = filtered
			}
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			report.Findings = out

			if report.DevicesTotal == 0 {
				report.Note = "no devices in the local mirror; run 'auvik-cli sync --resources inventory --full'"
			} else if report.ConfigsTotal == 0 {
				report.Note = "no configuration backups in the local mirror; run 'auvik-cli sync --resources inventory-configuration --full'. Every device is reported as no_backup until then."
			}

			done, err := emitLocalReport(cmd, flags, report, report.Findings)
			if err != nil || done {
				return err
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Configuration backup audit across %d device(s), %d backup record(s)\n",
				report.DevicesTotal, report.ConfigsTotal)
			fmt.Fprintf(w, "  protected: %d   no backup: %d   stale (>%s): %d   no running config: %d\n\n",
				report.Protected, report.Counts[findingNoBackup], staleIn,
				report.Counts[findingStale], report.Counts[findingNotRunning])
			if len(report.Findings) == 0 {
				if report.Note != "" {
					fmt.Fprintln(w, report.Note)
				} else {
					fmt.Fprintln(w, "No findings match the given filters.")
				}
				return nil
			}
			fmt.Fprintf(w, "%-18s %-22s %-28s %6s  %s\n", "FINDING", "CLIENT", "DEVICE", "AGE(d)", "DETAIL")
			for _, f := range report.Findings {
				age := "-"
				if f.AgeDays != nil {
					age = fmt.Sprintf("%d", *f.AgeDays)
				}
				fmt.Fprintf(w, "%-18s %-22s %-28s %6s  %s\n",
					f.Finding, truncate(f.Client, 22), truncate(f.DeviceName, 28), age, f.Detail)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&staleIn, "stale", "30d", "Backups older than this count as stale (e.g. 24h, 7d, 4w)")
	cmd.Flags().StringVar(&client, "client", "", "Only show findings whose client name contains this substring")
	cmd.Flags().StringVar(&finding, "finding", "", "Only one finding kind: no_backup, stale_backup, no_running_config")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum findings to return (0 = all)")
	return cmd
}

// buildConfigAuditReport is the pure core, split out for table tests.
func buildConfigAuditReport(devices, configs []auvikRow, tenants map[string]string, now time.Time, stale time.Duration) configAuditReport {
	report := configAuditReport{
		DevicesTotal:   len(devices),
		ConfigsTotal:   len(configs),
		StaleAfterDays: int(stale.Hours() / 24),
		Counts: map[string]int{
			findingNoBackup: 0, findingStale: 0, findingNotRunning: 0,
		},
		Findings: make([]configFinding, 0),
	}

	type backupFacts struct {
		count      int
		newest     time.Time
		newestRaw  string
		hasRunning bool
	}
	byDevice := map[string]*backupFacts{}
	for _, c := range configs {
		id := c.rel(rConfigDevice.Name)
		if id == "" {
			continue
		}
		f := byDevice[id]
		if f == nil {
			f = &backupFacts{}
			byDevice[id] = f
		}
		f.count++
		if truthy(c.attr(fConfigIsRunning.Field)) {
			f.hasRunning = true
		}
		raw := c.attrString(fConfigBackupTime.Field)
		if t, ok := parseAuvikTime(raw); ok && t.After(f.newest) {
			f.newest = t
			f.newestRaw = raw
		}
	}

	ordered := make([]auvikRow, len(devices))
	copy(ordered, devices)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	for _, dev := range ordered {
		base := configFinding{
			Client:     tenantLabel(tenants, dev.rel("tenant")),
			DeviceID:   dev.ID,
			DeviceName: firstNonEmpty(dev.attrString(fDeviceName.Field), dev.ID),
			DeviceType: dev.attrString(fDeviceType.Field),
		}
		f := byDevice[dev.ID]

		if f == nil || f.count == 0 {
			row := base
			row.Finding = findingNoBackup
			row.Detail = "no configuration backup stored for this device"
			report.Findings = append(report.Findings, row)
			report.Counts[findingNoBackup]++
			continue
		}

		base.Backups = f.count
		base.LastBackup = f.newestRaw
		var ageDays *int
		if !f.newest.IsZero() {
			d := int(now.Sub(f.newest).Hours() / 24)
			ageDays = &d
			base.AgeDays = ageDays
		}

		flagged := false
		if !f.newest.IsZero() && stale > 0 && now.Sub(f.newest) > stale {
			row := base
			row.Finding = findingStale
			row.Detail = fmt.Sprintf("newest backup is %d day(s) old", *ageDays)
			report.Findings = append(report.Findings, row)
			report.Counts[findingStale]++
			flagged = true
		}
		if !f.hasRunning {
			row := base
			row.Finding = findingNotRunning
			row.Detail = fmt.Sprintf("%d backup(s) stored, none flagged as the running config", f.count)
			report.Findings = append(report.Findings, row)
			report.Counts[findingNotRunning]++
			flagged = true
		}
		if !flagged {
			report.Protected++
		}
	}

	order := map[string]int{findingNoBackup: 0, findingStale: 1, findingNotRunning: 2}
	sort.SliceStable(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if order[a.Finding] != order[b.Finding] {
			return order[a.Finding] < order[b.Finding]
		}
		if a.Client != b.Client {
			return a.Client < b.Client
		}
		return a.DeviceName < b.DeviceName
	})
	return report
}
