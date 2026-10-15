// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/cliutil"
	"threatdown-pp-cli/internal/fleet"
	"threatdown-pp-cli/internal/store"
)

type threatsSpreadView struct {
	Source  string            `json:"source"`
	Note    string            `json:"note,omitempty"`
	Since   string            `json:"since"`
	Count   int               `json:"count"`
	Threats []fleet.SpreadRow `json:"threats"`
}

// localThreatData is everything the triage reports join, read drain-first.
type localThreatData struct {
	synced     bool   // detections have been fleet-synced at least once
	behind     bool   // detections were last synced before endpoints were
	coverage   string // set when the requested window predates the synced one
	incomplete string // set when the latest fleet sync of what was read was not whole
	sites      []fleet.Site
	endpoints  []fleet.Endpoint
	detections []fleet.Detection
}

// latestRowSync is when any row of a resource was last written, including
// --site and partial syncs that do not advance the freshness stamp. An
// unreadable value is an error so callers fail safe.
func latestRowSync(ctx context.Context, db *store.Store, resource string) (time.Time, error) {
	var v any
	if err := db.DB().QueryRowContext(ctx, `SELECT MAX(synced_at) FROM resources WHERE resource_type = ?`, resource).Scan(&v); err != nil {
		return time.Time{}, err
	}
	s := ""
	switch x := v.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return x.UTC(), nil
	case string:
		s = x
	case []byte:
		s = string(x)
	}
	if t, ok := fleet.ParseTime(strings.Replace(s, " ", "T", 1)); ok {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unreadable synced_at %v for %s", v, resource)
}

// loadThreatData opens the fleet-synced store. ok=false means no mirror yet.
func loadThreatData(cmd *cobra.Command, dbPath string, needEndpoints bool, since time.Time) (localThreatData, bool, error) {
	var out localThreatData
	db, ok, err := openFleetStore(cmd, dbPath)
	if err != nil || !ok {
		return out, ok, err
	}
	defer db.Close()
	if !hintIfUnsynced(cmd, db, tdResSites) {
		hintIfStale(cmd, db, tdResSites, tdMaxAge(cmd))
	}
	fleetHint(cmd, db, tdResDetections, tdMaxAge(cmd))
	if state, err := readSyncHintState(db, tdResDetections); err == nil {
		out.synced = state.hasState
	}
	out.coverage = coverageNote(cmd.Context(), db, tdResDetections, since)
	out.incomplete = incompleteSyncNote(db, tdResDetections)
	ctx := cmd.Context()
	if out.sites, err = loadLocalSites(ctx, db); err != nil {
		return out, true, err
	}
	raws, err := loadLocal(ctx, db, tdResDetections)
	if err != nil {
		return out, true, err
	}
	out.detections = parseDetections(raws)
	if needEndpoints {
		fleetHint(cmd, db, tdResEndpoints, tdMaxAge(cmd))
		out.incomplete = joinNotes(out.incomplete, incompleteSyncNote(db, tdResEndpoints))
		det, derr := readSyncHintState(db, tdResDetections)
		epWrite, eerr := latestRowSync(ctx, db, tdResEndpoints)
		// ponytail: second-resolution stamps, so an endpoints-only refresh in the
		// same second as a detection sync reads as not behind. A restore it
		// suggests still previews and needs --confirm; use sync generations if
		// overlapping syncs become common.
		out.behind = derr != nil || eerr != nil || (!epWrite.IsZero() && det.lastSynced.Before(epWrite))
		if out.endpoints, err = loadLocalEndpoints(ctx, db); err != nil {
			return out, true, err
		}
	}
	return out, true, nil
}

func newNovelThreatsSpreadCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath string
	var minSites, limit int

	cmd := &cobra.Command{
		Use:   "spread",
		Short: "Which threats hit several endpoints or customer sites in a window, by hash or threat name.",
		Long: `Use this command to see which threats (by hash or threat name) hit multiple endpoints or customer sites in a window. Do NOT use this command for a per-endpoint list of what to act on next; use 'threats queue' instead.

Reads detections stored by 'fleet sync' and groups them by sha256 (falling
back to threat name): distinct sites, distinct endpoints, first and last
seen, and how often each action was taken.`,
		Example: strings.Trim(`
  threatdown-cli threats spread --since 7d --agent
  threatdown-cli threats spread --since 30d --min-sites 2 --json --select threats.threat,threats.sites,threats.site_names`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "threats spread")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--since must be a duration like 7d, got %q", since))
			}
			if minSites < 1 {
				return usageErr(fmt.Errorf("--min-sites must be at least 1"))
			}
			view := threatsSpreadView{Source: "local", Since: since, Threats: make([]fleet.SpreadRow, 0)}
			data, ok, err := loadThreatData(cmd, tdDBPath(dbPath), false, time.Now().UTC().Add(-window))
			if err != nil {
				return err
			}
			if !ok || !data.synced {
				view.Note = "no detections synced yet; run " + tdFleetSyncCommand
			} else if data.coverage != "" {
				view.Note = data.coverage
			}
			view.Note = joinNotes(view.Note, data.incomplete)
			if ok {
				view.Threats = fleet.Spread(data.detections, fleet.SiteIndex(data.sites), time.Now().UTC().Add(-window), minSites)
			}
			if limit > 0 && len(view.Threats) > limit {
				view.Threats = view.Threats[:limit]
			}
			view.Count = len(view.Threats)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "sha256", "category", "note")
			}
			if view.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "note: %s\n", view.Note)
			}
			if view.Count == 0 {
				if view.Note == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "No threats in the last %s touching %d+ site(s).\n", since, minSites)
				}
				return nil
			}
			return printColumns(cmd.OutOrStdout(), view.Threats, "threat", "category", "sites", "endpoints", "detections", "first_seen", "last_seen", "site_names")
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Window of detections to consider (e.g. 7d, 24h)")
	cmd.Flags().IntVar(&minSites, "min-sites", 1, "Only threats seen at this many sites or more")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum threats to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the store 'fleet sync' writes)")
	return cmd
}
