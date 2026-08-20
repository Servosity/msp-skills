// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// `mirror` — ingest the query-shaped resources into the local store.
//
// pp:data-source live
//
// The generated `sync` command handles list-shaped GET resources (scopes and
// the MSP objects). Security events, SaaS entities, and the seven exception
// sub-systems are POST-query or multi-endpoint surfaces that `sync` cannot
// walk, so they get a dedicated ingester. Everything offline in this CLI reads
// what this command writes.

package cli

import (
	"fmt"
	"strings"
	"time"

	"avanan-pp-cli/internal/avananmirror"
	"avanan-pp-cli/internal/cliutil"
	"avanan-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newAvananMirrorCmd(flags))
	})
}

type mirrorReport struct {
	Since    string                 `json:"since"`
	Scopes   []string               `json:"scopes,omitempty"`
	Results  []avananmirror.Result  `json:"results"`
	Warnings []string               `json:"warnings,omitempty"`
	DBPath   string                 `json:"db_path"`
	Totals   map[string]int         `json:"totals"`
	Meta     map[string]interface{} `json:"-"`
}

func newAvananMirrorCmd(flags *rootFlags) *cobra.Command {
	var (
		since     string
		scopes    []string
		resources string
		maxPages  int
		dbPath    string
	)

	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Ingest events, entities, and exceptions into the local store",
		Long: strings.Trim(`
Populate the local SQLite mirror with the resources the generated 'sync'
command cannot reach.

Security events and SaaS entities are POST-body queries paged by an opaque
scrollId, and the seven exception sub-systems each live behind a different
path shape. This command walks all of them and writes a unified local copy,
which is what 'triage', 'campaign', 'timeline', 'exceptions find',
'exceptions audit', and 'msp fleet' read.

Run 'sync' as well to mirror scopes and the MSP objects.

Use this command to refresh local data before running the offline commands.
Do NOT use this command to query the API directly; use 'event query' or
'avanan-search query-saas-entity' instead.
`, "\n"),
		Example: strings.Trim(`
  avanan-cli mirror --since 7d
  avanan-cli mirror --resources events --since 24h
  avanan-cli mirror --since 30d --scope farm1:tenant-a --agent
`, "\n"),
		Annotations: map[string]string{
			"pp:happy-args": "--since=24h",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "mirror")
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			window := 7 * 24 * time.Hour
			if since != "" {
				d, err := cliutil.ParseDurationLoose(since)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("invalid --since %q: %w (try 24h, 7d, or 4w)", since, err))
				}
				window = d
			}

			wanted, err := parseMirrorResources(resources)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}

			if maxPages == 0 && cliutil.IsDogfoodEnv() {
				// The live-dogfood matrix runs every command under a flat
				// per-command timeout; an unbounded first mirror against a
				// real tenant would blow it.
				maxPages = 1
			}

			if dbPath == "" {
				dbPath = defaultDBPath("avanan-cli")
			}
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Options.Scopes documents empty as "every scope the credential
			// can reach", but the exception and sectool paths cannot express
			// that in one call: a multi-scope app client answers an unscoped
			// request with HTTP 400. Resolve it to the real list up front so
			// the documented behaviour is what actually happens.
			if len(scopes) == 0 {
				discovered, err := avananmirror.DiscoverScopes(ctx, c)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"warning: could not discover scopes (%v); continuing unscoped, which a multi-scope client will reject on the exception paths\n", err)
				} else if len(discovered) > 0 {
					scopes = discovered
				}
			}

			opts := avananmirror.Options{
				Since:    time.Now().Add(-window),
				Scopes:   scopes,
				MaxPages: maxPages,
			}

			report := mirrorReport{
				Since:   opts.Since.UTC().Format(time.RFC3339),
				Scopes:  scopes,
				DBPath:  dbPath,
				Results: make([]avananmirror.Result, 0, 3),
				Totals:  map[string]int{},
			}

			// throttled records a rate-limit failure. A throttled mirror is
			// partial by definition, and a partial mirror makes the offline
			// commands answer confidently from data they never received — so
			// this must surface as a non-zero exit, not a warning buried in
			// stderr.
			var throttled error

			if wanted[avananmirror.ResourceEvents] {
				res, err := avananmirror.Events(ctx, c, db, opts)
				report.Results = append(report.Results, res)
				if err != nil {
					report.Warnings = append(report.Warnings, fmt.Sprintf("events: %v", err))
					if avananmirror.IsRateLimited(err) && throttled == nil {
						throttled = err
					}
				}
			}
			if wanted[avananmirror.ResourceEntities] {
				res, err := avananmirror.Entities(ctx, c, db, opts)
				report.Results = append(report.Results, res)
				if err != nil {
					report.Warnings = append(report.Warnings, fmt.Sprintf("entities: %v", err))
					if avananmirror.IsRateLimited(err) && throttled == nil {
						throttled = err
					}
				}
			}
			if wanted[avananmirror.ResourceExceptions] {
				res, problems := avananmirror.Exceptions(ctx, c, db, opts)
				report.Results = append(report.Results, res)
				for _, p := range problems {
					report.Warnings = append(report.Warnings, fmt.Sprintf("exceptions: %v", p))
					if avananmirror.IsRateLimited(p) && throttled == nil {
						throttled = p
					}
				}
			}

			for _, r := range report.Results {
				report.Totals["fetched"] += r.Fetched
				report.Totals["stored"] += r.Stored
				report.Totals["skipped_no_id"] += r.Skipped
			}

			// Record sync state per mirrored resource. Without this the
			// freshness helpers have nothing to read: every offline command
			// would print "local store has not been synced yet" immediately
			// after a successful mirror, and --max-age could never fire.
			// Skipped when throttled, because a partial mirror should not be
			// stamped as a good one.
			if throttled == nil {
				for _, r := range report.Results {
					if err := db.SaveSyncState(r.Resource, "", r.Stored); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not record sync state for %s (%v); freshness hints will be wrong\n", r.Resource, err)
					}
				}
			}

			for _, w := range report.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), report, flags); err != nil {
					return err
				}
				if throttled != nil {
					return fmt.Errorf("mirror is incomplete because the API throttled it: %w", throttled)
				}
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Mirrored since %s into %s\n\n", report.Since, dbPath)
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s %8s %8s %6s\n", "RESOURCE", "FETCHED", "STORED", "PAGES")
			for _, r := range report.Results {
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s %8d %8d %6d\n", r.Resource, r.Fetched, r.Stored, r.Pages)
				if r.Capped {
					fmt.Fprintf(cmd.OutOrStdout(), "  (page cap hit; raise --max-pages to widen)\n")
				}
				if r.Skipped > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  (%d records had no usable identifier and were not stored)\n", r.Skipped)
				}
			}
			if throttled != nil {
				return fmt.Errorf("mirror is incomplete because the API throttled it: %w", throttled)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "How far back to mirror: 24h, 7d, 4w (default 7d)")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "Limit to specific {farm}:{tenant} scopes (repeatable; default all reachable scopes)")
	cmd.Flags().StringVar(&resources, "resources", "", "Comma-separated subset to mirror: events, entities, exceptions (default all)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 0, "Maximum scrollId pages per resource (0 = unlimited)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// mirrorResourceNames maps what a user types to the resource type the store
// uses. The two deliberately differ: store keys are namespaced (avanan_events)
// so they cannot collide with the generated syncer's tables, while the flag
// vocabulary stays the short domain word. Conflating them is exactly the bug
// this indirection exists to prevent — comparing user input against the
// namespaced constant makes every value invalid and every default empty, which
// silently turns `mirror` into a no-op.
var mirrorResourceNames = map[string]string{
	"events":     avananmirror.ResourceEvents,
	"entities":   avananmirror.ResourceEntities,
	"exceptions": avananmirror.ResourceExceptions,
}

// parseMirrorResources resolves the --resources CSV into the set of store
// resource types to ingest. An empty selection means all of them.
func parseMirrorResources(csv string) (map[string]bool, error) {
	wanted := map[string]bool{}
	for _, raw := range strings.Split(csv, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		resource, ok := mirrorResourceNames[name]
		if !ok {
			return nil, fmt.Errorf("unknown resource %q; valid values are events, entities, exceptions", name)
		}
		wanted[resource] = true
	}
	if len(wanted) == 0 {
		for _, resource := range mirrorResourceNames {
			wanted[resource] = true
		}
	}
	return wanted, nil
}
