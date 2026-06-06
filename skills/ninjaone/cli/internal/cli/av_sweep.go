// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// avHit is one device flagged by the AV sweep.
type avHit struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	OrgID      string `json:"orgId"`
	Org        string `json:"org"`
	Kind       string `json:"kind"` // "threat" or "stale-definitions"
	Detail     string `json:"detail"`
}

// pp:data-source local
func newNovelAvSweepCmd(flags *rootFlags) *cobra.Command {
	var threat string
	var staleDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "av-sweep",
		Short: "Fleet-wide AV blast-radius: devices with a threat or stale definitions",
		Long: `Joins the antivirus-threats and antivirus-status reports to the
device->org/location map for a blast-radius view. Filter by threat name to see
every device fleet-wide carrying it, or by --definition-stale-days to list
devices whose AV definitions are older than N days. NinjaOne returns these as
per-device rows; this turns one detection into a fleet answer.

Reads the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Every device carrying a named threat
  ninjaone-cli av-sweep --threat "Trojan.Generic" --json

  # Devices with AV definitions older than 7 days
  ninjaone-cli av-sweep --definition-stale-days 7 --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtAVStatus) {
				hintIfStale(cmd, db, rtAVStatus, flags.maxAge)
			}

			devices, err := buildDeviceIndex(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			orgNames, err := buildOrgNames(db)
			if err != nil {
				return fmt.Errorf("loading organizations: %w", err)
			}
			threatRows, err := loadRows(db, rtAVThreats)
			if err != nil {
				return fmt.Errorf("loading av threats: %w", err)
			}
			statusRows, err := loadRows(db, rtAVStatus)
			if err != nil {
				return fmt.Errorf("loading av status: %w", err)
			}

			hits := computeAVSweep(threatRows, statusRows, devices, orgNames, threat, staleDays, time.Now().UTC())

			if wantsStructured(flags) {
				return flags.printJSON(cmd, hits)
			}
			if note := emptyStoreNote(devices); note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), note)
			}
			if len(hits) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No AV hits matched.")
				return nil
			}
			rows := make([][]string, 0, len(hits))
			for _, h := range hits {
				rows = append(rows, []string{h.Org, h.DeviceName, h.Kind, h.Detail})
			}
			return flags.printTable(cmd, []string{"ORG", "DEVICE", "KIND", "DETAIL"}, rows)
		},
	}
	cmd.Flags().StringVar(&threat, "threat", "", "Case-insensitive substring match on threat name")
	cmd.Flags().IntVar(&staleDays, "definition-stale-days", 0, "Flag devices whose AV definitions are older than N days (0 = off)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}

// computeAVSweep filters threat rows by name and/or status rows by definition
// age, joining each to its device. Split out for table-driven testing.
func computeAVSweep(threatRows, statusRows []map[string]any, devices map[string]nvDevice, orgNames map[string]string, threat string, staleDays int, now time.Time) []avHit {
	var hits []avHit
	join := func(did string) (string, string, string) {
		if d, ok := devices[did]; ok {
			return d.Name, d.OrgID, orgLabel(orgNames, d.OrgID)
		}
		return did, "", "(unknown org)"
	}
	pat := strings.ToLower(strings.TrimSpace(threat))

	// Always include threats; if a threat filter is set, match it.
	for _, r := range threatRows {
		name := nvStr(r, "threatName", "name", "threat", "displayName")
		if pat != "" && !strings.Contains(strings.ToLower(name), pat) {
			continue
		}
		did := rowDeviceID(r)
		dn, oid, org := join(did)
		detail := name
		if detail == "" {
			detail = "(unnamed threat)"
		}
		hits = append(hits, avHit{DeviceID: did, DeviceName: dn, OrgID: oid, Org: org, Kind: "threat", Detail: detail})
	}

	// Stale definitions only when requested.
	if staleDays > 0 {
		cutoff := now.AddDate(0, 0, -staleDays)
		for _, r := range statusRows {
			defTime, ok := nvEpoch(r, "definitionDate", "definitionsDate", "lastUpdate", "lastDefinitionUpdate")
			if !ok || !defTime.Before(cutoff) {
				continue
			}
			did := rowDeviceID(r)
			dn, oid, org := join(did)
			age := int(now.Sub(defTime).Hours() / 24)
			hits = append(hits, avHit{
				DeviceID: did, DeviceName: dn, OrgID: oid, Org: org,
				Kind: "stale-definitions", Detail: fmt.Sprintf("defs %d days old", age),
			})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Org != hits[j].Org {
			return hits[i].Org < hits[j].Org
		}
		return hits[i].DeviceName < hits[j].DeviceName
	})
	return hits
}
