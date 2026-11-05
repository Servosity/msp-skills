// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-umbrella-pp-cli/internal/store"
)

var hygieneKinds = []string{"roaming-off", "roaming-uninstalled", "roaming-stale", "swg-unprotected", "va-not-healthy", "va-upgrade-available", "tunnel-down", "network-unverified"}

type hygieneFinding struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Detail   string `json:"detail"`
}

type hygieneReport struct {
	StaleDays int              `json:"stale_days"`
	Findings  []hygieneFinding `json:"findings"`
	Counts    map[string]int   `json:"counts"`
	Scanned   map[string]int   `json:"scanned"`
	NotSynced []string         `json:"not_synced"`
	Unparsed  int              `json:"unparseable_records"`
	Unchecked map[string]int   `json:"unchecked_checks"` // check kind -> records missing its field
	Note      string           `json:"note,omitempty"`
}

func newNovelHygieneCmd(flags *rootFlags) *cobra.Command {
	var staleDays int
	var kind, dbPath string
	cmd := &cobra.Command{
		Use:   "hygiene",
		Short: "Device-level findings in one org: stale or offline roaming computers, unhealthy virtual appliances, down tunnels",
		Long: `Read the synced roaming computers, virtual appliances, tunnel state, and networks
from the local store and list every deployment problem as a typed finding:
` + strings.Join(hygieneKinds, ", ") + `.
Run 'sync --resources roaming_computers,virtual_appliances,tunnels-tunnels-state,networks' first
(add --org to sync one child org into its own store).

Use this command for device-level deployment findings (stale/offline roaming
computers, unhealthy VAs, down tunnels, unverified networks) in one org. Do NOT use
this command for a cross-customer rollup; use 'fleet health' instead.`,
		Example: strings.Trim(`
  cisco-umbrella-cli hygiene --stale-days 14 --agent
  cisco-umbrella-cli hygiene --kind roaming-stale --json --select findings.name,findings.detail`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--stale-days=14"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "hygiene")
			}
			if staleDays < 1 {
				return usageErr(fmt.Errorf("--stale-days must be at least 1"))
			}
			if kind != "" && !contains(hygieneKinds, kind) {
				return usageErr(fmt.Errorf("--kind must be one of %s", strings.Join(hygieneKinds, ", ")))
			}
			if dbPath == "" {
				dbPath = defaultDBPath("cisco-umbrella-cli")
			}
			syncHint := "nothing synced: run 'cisco-umbrella-cli sync --resources roaming_computers,virtual_appliances,tunnels-tunnels-state,networks' (with the same --org) first"
			empty := hygieneReport{StaleDays: staleDays, Findings: []hygieneFinding{}, Counts: map[string]int{}, Scanned: map[string]int{}, Unchecked: map[string]int{}, NotSynced: append([]string{}, hygieneSyncResources...), Note: syncHint}
			if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
				fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nwarning: %s\n", dbPath, syncHint)
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), empty, flags)
				}
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := store.OpenReadOnlyContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "roaming_computers") {
				hintIfStale(cmd, db, "roaming_computers", flags.maxAge)
			}

			type rawRow struct{ typ, id, data string }
			rows, err := db.DB().QueryContext(ctx, `SELECT resource_type, id, data FROM resources
				WHERE resource_type IN ('roaming_computers','roaming-computers','virtual_appliances','virtual-appliances',
				'tunnels-tunnels-state','tunnels_tunnels_state','networks')`)
			if err != nil {
				return fmt.Errorf("query: %w", err)
			}
			raws := make([]rawRow, 0)
			for rows.Next() {
				var r rawRow
				if err := rows.Scan(&r.typ, &r.id, &r.data); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scan: %w", err)
				}
				raws = append(raws, r)
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("iterate: %w", err)
			}
			if err := rows.Close(); err != nil {
				return err
			}

			report := empty
			report.NotSynced = []string{}
			for _, res := range hygieneSyncResources {
				if !syncedOnce(db, res) {
					report.NotSynced = append(report.NotSynced, res)
				}
			}
			report.Note = ""
			if len(report.NotSynced) > 0 {
				// A partial sync must not claim "nothing synced" or ask for a
				// re-sync of resources that already synced; name only the gaps.
				hint := syncHint
				if len(report.NotSynced) < len(hygieneSyncResources) {
					hint = "run 'cisco-umbrella-cli sync --resources " + strings.Join(report.NotSynced, ",") + "' (with the same --org) first"
				}
				report.Note = "never synced: " + strings.Join(report.NotSynced, ", ") + "; findings for those cover nothing. " + hint
			}
			cutoff := time.Now().Add(-time.Duration(staleDays) * 24 * time.Hour)
			for _, r := range raws {
				res := strings.ReplaceAll(r.typ, "-", "_")
				findings, unchecked, ok := hygieneFindingsFor(res, r.id, json.RawMessage(r.data), cutoff)
				for _, k := range unchecked {
					report.Unchecked[k]++
				}
				if !ok {
					report.Unparsed++
					continue
				}
				report.Scanned[res]++
				for _, f := range findings {
					if kind == "" || f.Kind == kind {
						report.Findings = append(report.Findings, f)
						report.Counts[f.Kind]++
					}
				}
			}
			if report.Unparsed > 0 {
				report.Note = strings.TrimSpace(fmt.Sprintf("%d synced record(s) could not be read and were not checked; re-sync. %s", report.Unparsed, report.Note))
			}
			for _, k := range sortedKeys(report.Unchecked) {
				report.Note = strings.TrimSpace(fmt.Sprintf("%s %d record(s) lack the field for %s, so that check did not run.", report.Note, report.Unchecked[k], k))
			}
			sort.SliceStable(report.Findings, func(i, j int) bool {
				if report.Findings[i].Kind != report.Findings[j].Kind {
					return report.Findings[i].Kind < report.Findings[j].Kind
				}
				return report.Findings[i].Name < report.Findings[j].Name
			})

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}
			if report.Note != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", report.Note)
			}
			if len(report.Findings) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No findings across %d synced records.\n", len(raws))
				return nil
			}
			out := make([]map[string]any, 0, len(report.Findings))
			for _, f := range report.Findings {
				out = append(out, map[string]any{"kind": f.Kind, "name": f.Name, "id": f.ID, "detail": f.Detail})
			}
			return printAutoTable(cmd.OutOrStdout(), out)
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 14, "Flag roaming computers whose last sync is older than this many days")
	cmd.Flags().StringVar(&kind, "kind", "", "Only report one finding kind: "+strings.Join(hygieneKinds, ", "))
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the synced store for the current key and --org)")
	return cmd
}

// hygieneFindingsFor turns one synced record into findings plus the finding kinds
// that could not be checked (their field was unusable). ok=false means the record
// is not a JSON object at all.
func hygieneFindingsFor(resource, id string, data json.RawMessage, staleCutoff time.Time) ([]hygieneFinding, []string, bool) {
	out := make([]hygieneFinding, 0)
	add := func(kind, name, detail string) {
		out = append(out, hygieneFinding{Kind: kind, Resource: resource, ID: id, Name: name, Detail: detail})
	}
	switch resource {
	case "roaming_computers":
		ev, ok := evalRoaming(data, staleCutoff)
		if !ok {
			return nil, nil, false
		}
		switch {
		case ev.Off:
			add("roaming-off", ev.Name, "DNS-layer status "+ev.Status)
		case ev.Uninstalled:
			add("roaming-uninstalled", ev.Name, "client uninstalled")
		}
		if ev.SWGUnprotected {
			add("swg-unprotected", ev.Name, "secure web gateway status Unprotected")
		}
		if ev.Stale {
			detail := "no sync recorded"
			if ev.LastSync != "" {
				detail = "last sync " + ev.LastSync
			}
			add("roaming-stale", ev.Name, detail)
		}
		return out, ev.Unchecked, true
	case "virtual_appliances":
		ev, ok := evalVA(data)
		if !ok {
			return nil, nil, false
		}
		if ev.NotHealthy {
			add("va-not-healthy", ev.Name, "health "+ev.Health)
		}
		if ev.Upgradable {
			add("va-upgrade-available", ev.Name, "an upgrade is available")
		}
		return out, ev.Unchecked, true
	case "tunnels_tunnels_state":
		ev, ok := evalTunnel(data)
		if !ok {
			return nil, nil, false
		}
		if ev.Down {
			add("tunnel-down", ev.ID, ev.Detail)
		}
		return out, ev.Unchecked, true
	case "networks":
		ev, ok := evalNetwork(data)
		if !ok {
			return nil, nil, false
		}
		if ev.Unverified {
			add("network-unverified", ev.Name, "network "+ev.IPAddress+" is not verified")
		}
		return out, ev.Unchecked, true
	}
	return out, nil, true
}

// hygieneSyncResources are the sync keys hygiene reads (sync uses snake_case,
// except the tunnel-state resource).
var hygieneSyncResources = []string{"roaming_computers", "virtual_appliances", "tunnels-tunnels-state", "networks"}

// syncedOnce reports whether sync ever recorded state for a resource, which tells
// "synced and empty" apart from "never synced".
func syncedOnce(db *store.Store, resource string) bool {
	for _, name := range []string{resource, strings.ReplaceAll(resource, "-", "_"), strings.ReplaceAll(resource, "_", "-")} {
		if st, err := readSyncHintState(db, name); err == nil && st.hasState {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
