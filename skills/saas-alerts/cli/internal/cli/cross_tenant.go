// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source local
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type saasCrossTenantCustomer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type saasCrossTenantRow struct {
	Key         string                    `json:"key"`
	Label       string                    `json:"label,omitempty"` // ASN name for --by asn
	Customers   []saasCrossTenantCustomer `json:"customers"`
	Users       []string                  `json:"users"`
	JointDescs  []string                  `json:"joint_descs"`
	MaxSeverity string                    `json:"max_severity"`
	EventCount  int                       `json:"event_count"`
	Threat      any                       `json:"threat"`

	customers  map[string]string
	users      map[string]bool
	descs      map[string]bool
	rank       int
	threatTime time.Time
}

type saasCrossTenantResult struct {
	WindowStart         string               `json:"window_start"`
	WindowEnd           string               `json:"window_end"`
	By                  string               `json:"by"`
	MinCustomers        int                  `json:"min_customers"`
	Synced              bool                 `json:"synced"`
	Note                string               `json:"note,omitempty"`
	ScannedEvents       int                  `json:"scanned_events"`
	WhitelistExclusions int                  `json:"whitelist_exclusions"`
	WhitelistNote       string               `json:"whitelist_note,omitempty"`
	TotalSources        int                  `json:"total_sources"`
	Sources             []saasCrossTenantRow `json:"sources"`
}

const saasCrossTenantSyncResources = "customers,events"

var saasCrossTenantKeep = []string{"key", "label", "customers", "users", "joint_descs", "max_severity", "event_count", "threat"}

func newNovelCrossTenantCmd(flags *rootFlags) *cobra.Command {
	var flagBy string
	var flagSince string
	var flagMinCustomers int
	var flagIncludeWhitelisted bool
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "cross-tenant",
		Short: "Find IPs, ASNs, or countries hitting two or more customers in a window, minus sources on each customer's own whitelist.",
		Long: "Use this command to find one IP, ASN or country hitting several customers. Do NOT use this command for a single account's sign-in sources; use 'investigate' instead.\n\n" +
			"An event is excluded only when its source is on that event's own customer whitelist for the chosen --by dimension: ipRanges for ip, asnList for asn (whitelist entries carry no AS number, so they match by ASN name), countries for country. " +
			"Partner-inherited whitelists are not applied. --include-whitelisted keeps every event.\n\n" +
			"Reads the local store only: run 'saas-alerts-cli sync --resources customers,events' first. synced is false (with a note) when events were never synced or the last events sync did not finish.",
		Example: strings.Trim(`
  saas-alerts-cli cross-tenant --by ip --since 24h --min-customers 2 --agent
  saas-alerts-cli cross-tenant --by asn --since 7d --min-customers 3
  saas-alerts-cli cross-tenant --by country --since 24h --include-whitelisted --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--by=ip;--since=24h"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cross-tenant")
			}
			by := strings.ToLower(strings.TrimSpace(flagBy))
			if by != "ip" && by != "asn" && by != "country" {
				return usageErr(fmt.Errorf("invalid --by %q: use ip, asn or country", flagBy))
			}
			since, err := saasParseWindow("since", flagSince)
			if err != nil {
				return err
			}
			if flagMinCustomers < 1 {
				return usageErr(fmt.Errorf("--min-customers must be at least 1, got %d", flagMinCustomers))
			}
			if flagLimit < 0 {
				return usageErr(fmt.Errorf("--limit must be 0 (no limit) or greater, got %d", flagLimit))
			}
			now := saasLocalNow().UTC()
			start := now.Add(-since)
			result := saasCrossTenantResult{
				WindowStart:  saasFormatTime(start),
				WindowEnd:    saasFormatTime(now),
				By:           by,
				MinCustomers: flagMinCustomers,
				Sources:      make([]saasCrossTenantRow, 0),
				Note:         saasUnsyncedNote([]string{"events"}, saasCrossTenantSyncResources),
			}

			dbPath = saasResolveDBPath(dbPath)
			if handled, err := saasLocalMirrorMissing(cmd, flags, dbPath, saasCrossTenantSyncResources, result, saasCrossTenantKeep); handled {
				return err
			}
			db, err := saasOpenLocalStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			events, err := saasLoadLocalEvents(cmd.Context(), db, saasEventQuery{Start: start, End: now})
			if err != nil {
				return err
			}
			customers, _, err := saasLoadLocalCustomers(cmd.Context(), db)
			if err != nil {
				return err
			}
			saasLocalSyncHints(cmd, db, flags, "events")
			unsynced := saasUnsyncedResources(db, "events")
			gaps, incomplete := saasSyncGaps(db, unsynced, "events") // saas-alerts hand-fix: an unfinished sync is not synced
			result.Synced = len(gaps) == 0
			result.Note = saasSyncGapNote(unsynced, incomplete, saasCrossTenantSyncResources)
			saasBuildCrossTenant(&result, events, customers, flagIncludeWhitelisted, flagLimit)

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				return saasRenderCrossTenantHuman(cmd, result)
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), result, flags, saasCrossTenantKeep...)
		},
	}
	cmd.Flags().StringVar(&flagBy, "by", "ip", "Group sources by ip, asn or country")
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Look-back window (24h, 7d, 1w)")
	cmd.Flags().IntVar(&flagMinCustomers, "min-customers", 2, "Only report sources seen at this many distinct customers or more")
	cmd.Flags().BoolVar(&flagIncludeWhitelisted, "include-whitelisted", false, "Keep events whose source is on the customer's own whitelist for the --by dimension (default: excluded; partner-inherited whitelists are never applied)")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum sources to return (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// saasCrossTenantKey extracts the grouping key for one event.
func saasCrossTenantKey(ev saasLocalEvent, by string) (key, label string) {
	switch by {
	case "ip":
		return ev.IP, ""
	case "asn":
		// location.ipInfo.asn.asn is the AS number string; fall back to the
		// ASN name when the number is missing.
		if ev.ASN != "" {
			return ev.ASN, ev.ASNName
		}
		return ev.ASNName, ev.ASNName
	case "country":
		return ev.Country, ""
	}
	return "", ""
}

// saasBuildCrossTenant groups windowed events by source and keeps sources
// seen at >= MinCustomers distinct customers. Whitelist exclusion uses the
// event's own customer whitelist for the same dimension as --by (ipRanges for
// ip, asnList by name for asn, countries for country). Partner-level
// inherited whitelists (ISCustomer.inheritanceWl) are not synced locally and
// are not applied.
func saasBuildCrossTenant(result *saasCrossTenantResult, events []saasLocalEvent, customers map[string]saasLocalCustomer, includeWhitelisted bool, limit int) {
	result.ScannedEvents = len(events)
	withLists := 0
	for _, c := range customers {
		if c.HasWhitelistFields {
			withLists++
		}
	}
	switch {
	case includeWhitelisted:
		result.WhitelistNote = "whitelist exclusion disabled by --include-whitelisted"
	case len(customers) == 0:
		result.WhitelistNote = "no synced customers; whitelist exclusion skipped (run sync --resources customers)"
	case withLists == 0:
		result.WhitelistNote = "synced customers carry no ipRanges/asnList/countries fields; whitelist exclusion skipped"
	}

	rows := map[string]*saasCrossTenantRow{}
	for _, ev := range events {
		key, label := saasCrossTenantKey(ev, result.By)
		if key == "" {
			continue
		}
		if !includeWhitelisted {
			if c, ok := customers[ev.CustomerID]; ok {
				var listed bool
				switch result.By {
				case "ip":
					listed = c.whitelistsIP(ev.IP)
				case "asn":
					listed = c.whitelistsASN(ev.ASN, ev.ASNName)
				case "country":
					listed = c.whitelistsCountry(ev.Country)
				}
				if listed {
					result.WhitelistExclusions++
					continue
				}
			}
		}
		groupKey := key
		if result.By == "country" || result.By == "asn" {
			groupKey = strings.ToUpper(key)
		}
		row, ok := rows[groupKey]
		if !ok {
			row = &saasCrossTenantRow{
				Key:       key,
				Label:     label,
				customers: map[string]string{},
				users:     map[string]bool{},
				descs:     map[string]bool{},
				rank:      -1,
			}
			rows[groupKey] = row
		}
		custID := ev.CustomerID
		if custID == "" {
			custID = "name:" + ev.CustomerName
		}
		if existing := row.customers[custID]; existing == "" {
			row.customers[custID] = ev.CustomerName
		}
		if ev.UserName != "" {
			row.users[ev.UserName] = true
		}
		desc := ev.JointDesc
		if desc == "" {
			desc = ev.JointType
		}
		if desc != "" {
			row.descs[desc] = true
		}
		if r := saasSeverityRank(ev.Severity); r > row.rank {
			row.rank = r
			row.MaxSeverity = ev.Severity
		}
		row.EventCount++
		if row.Label == "" && label != "" {
			row.Label = label
		}
		// Keep the most recent non-null ipInfo.threat seen for this source.
		if ev.Threat != nil && (row.Threat == nil || ev.Time.After(row.threatTime)) {
			row.Threat = ev.Threat
			row.threatTime = ev.Time
		}
	}

	list := make([]saasCrossTenantRow, 0, len(rows))
	for _, row := range rows {
		if len(row.customers) < result.MinCustomers {
			continue
		}
		row.Customers = make([]saasCrossTenantCustomer, 0, len(row.customers))
		for id, name := range row.customers {
			if strings.HasPrefix(id, "name:") {
				id = ""
			}
			row.Customers = append(row.Customers, saasCrossTenantCustomer{ID: id, Name: name})
		}
		sort.Slice(row.Customers, func(i, j int) bool {
			if row.Customers[i].Name != row.Customers[j].Name {
				return row.Customers[i].Name < row.Customers[j].Name
			}
			return row.Customers[i].ID < row.Customers[j].ID
		})
		row.Users = saasSortedKeys(row.users)
		row.JointDescs = saasSortedKeys(row.descs)
		if row.MaxSeverity == "" {
			row.MaxSeverity = "unknown"
		}
		list = append(list, *row)
	}
	// Breadth first (customers hit), then the worst severity seen, then
	// volume: a critical source must not rank below a benign high-volume one
	// at the same breadth.
	sort.SliceStable(list, func(i, j int) bool {
		if len(list[i].Customers) != len(list[j].Customers) {
			return len(list[i].Customers) > len(list[j].Customers)
		}
		if list[i].rank != list[j].rank {
			return list[i].rank > list[j].rank
		}
		if list[i].EventCount != list[j].EventCount {
			return list[i].EventCount > list[j].EventCount
		}
		return list[i].Key < list[j].Key
	})
	result.TotalSources = len(list)
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	result.Sources = list
}

func saasRenderCrossTenantHuman(cmd *cobra.Command, r saasCrossTenantResult) error {
	out := cmd.OutOrStdout()
	if r.Note != "" {
		fmt.Fprintf(out, "note: %s\n", r.Note)
	}
	fmt.Fprintf(out, "Window %s .. %s: %d events scanned, %d excluded by customer whitelists\n",
		r.WindowStart, r.WindowEnd, r.ScannedEvents, r.WhitelistExclusions)
	if r.WhitelistNote != "" {
		fmt.Fprintf(out, "note: %s\n", r.WhitelistNote)
	}
	if len(r.Sources) == 0 {
		fmt.Fprintf(out, "No %s seen at %d or more customers.\n", strings.ToUpper(r.By), r.MinCustomers)
		return nil
	}
	tw := newTabWriter(out)
	fmt.Fprintf(tw, "%s\tCUSTOMERS\tEVENTS\tMAX SEVERITY\tUSERS\tCUSTOMER NAMES\n", strings.ToUpper(r.By))
	for _, row := range r.Sources {
		names := make([]string, 0, len(row.Customers))
		for _, c := range row.Customers {
			names = append(names, c.Name)
		}
		key := row.Key
		if row.Label != "" && row.Label != row.Key {
			key += " (" + row.Label + ")"
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%d\t%s\n", truncate(key, 40), len(row.Customers), row.EventCount, row.MaxSeverity, len(row.Users), truncate(strings.Join(names, ", "), 60))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if r.TotalSources > len(r.Sources) {
		fmt.Fprintf(out, "showing %d of %d sources (raise --limit for more)\n", len(r.Sources), r.TotalSources)
	}
	return nil
}
