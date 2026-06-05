// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mspbots-pp-cli/internal/cliutil"
	"mspbots-pp-cli/internal/store"
)

func newNovelSnapshotCmd(flags *rootFlags) *cobra.Command {
	var flagWhere []string
	var maxPages int
	var pageSize int
	var typeOverride string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "snapshot [alias-or-resourceId]",
		Short: "Capture a point-in-time copy of a dataset or widget into local SQLite — the history MSPbots doesn't keep",
		Long: strings.Trim(`
Use this command to capture a point-in-time copy of a resource into the local
snapshot store. Do NOT use it to read past snapshots; use 'diff' or 'trend' instead.

Each run pulls every page (bounded by --max-pages) and stores the rows with a
timestamp. Run it on a schedule (cron, agent loop) and 'diff'/'trend' turn the
captures into the week-over-week history the MSPbots API cannot answer.`, "\n"),
		Example: strings.Trim(`
  mspbots-cli snapshot open-tickets
  mspbots-cli snapshot open-tickets --where "Status = Open" --json`, "\n"),
		Annotations: map[string]string{
			// No mcp:read-only: snapshot writes rows into the local SQLite
			// snapshot store (SnapshotInsert), so it mutates persistent local
			// state. AGENTS.md: skip read-only for commands that do store updates.
			"pp:happy-args": "alias-or-resourceId=1534956341424005122;--max-pages=1",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("snapshot needs <alias-or-resourceId>"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would snapshot %q (up to %d pages of %d rows)\n", args[0], maxPages, pageSize)
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if err := validateResourceType(typeOverride, true); err != nil {
				return usageErr(err)
			}
			filters, err := compileWhere(flagWhere)
			if err != nil {
				return usageErr(err)
			}
			if cliutil.IsDogfoodEnv() && maxPages > 1 {
				maxPages = 1
			}
			db, err := openMspbotsStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := resolveResource(cmd.Context(), db, args[0], typeOverride)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			var allRows []json.RawMessage
			pages, maxPagesHit, err := walkResourcePages(cmd.Context(), c, res, maxPages, pageSize, filters, func(rows []json.RawMessage) error {
				allRows = append(allRows, rows...)
				return nil
			})
			if err != nil {
				return err
			}
			if maxPagesHit {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: snapshot stopped at --max-pages %d with full pages still coming; the capture is partial — raise --max-pages for a complete snapshot\n", maxPages)
			}
			keys := make([]string, len(allRows))
			for i, row := range allRows {
				keys[i] = rowIdentityKey(row)
			}
			alias := res.Alias
			if alias == "" {
				alias = res.ResourceID
			}
			meta := store.SnapshotMeta{
				Alias:        alias,
				ResourceID:   res.ResourceID,
				ResourceType: res.ResourceType,
				TakenAt:      time.Now().UTC().Format(time.RFC3339),
				RowCount:     len(allRows),
				Pages:        pages,
			}
			id, err := db.SnapshotInsert(cmd.Context(), meta, allRows, keys)
			if err != nil {
				return fmt.Errorf("storing snapshot: %w", err)
			}
			meta.ID = id
			return flags.printJSON(cmd, meta)
		},
	}
	// StringArrayVar, not StringSliceVar: slice mode comma-splits a single
	// value, which would shred "Col between A,B" into two broken predicates.
	cmd.Flags().StringArrayVar(&flagWhere, "where", nil, `Readable filter predicate, repeatable (applies to what gets captured)`)
	cmd.Flags().IntVar(&maxPages, "max-pages", 40, "Maximum pages to fetch into the snapshot")
	cmd.Flags().IntVar(&pageSize, "page-size", 200, "Rows per page request (wire key: size)")
	cmd.Flags().StringVar(&typeOverride, "type", "", "Resource type when passing a raw ID: dataset or widget (default dataset)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the CLI's local store)")
	return cmd
}
