// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"riverside-fm-pp-cli/internal/store"
)

// newSearchCmd queries the resources_fts virtual table for full-text matches
// across every resource earlier live reads cached in the local store, with
// optional filters on resource_type and a speaker substring (transcripts only).
func newSearchCmd(flags *rootFlags) *cobra.Command {
	var resourceType string
	var speaker string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search across locally cached resources (recordings, projects, transcripts).",
		Long: `Runs a FTS5 query against the local store's resources_fts table. The store holds
what earlier live reads (projects, takes, transcriptions) cached; read a resource
first to make it searchable. Without a query, prints usage. Use --type to scope to a resource type (transcriptions,
projects, recordings); use --speaker to substring-match speaker names inside transcript
bodies; use --limit to cap result count.`,
		Example: `  riverside-fm-cli search "compounding loop" --json
  riverside-fm-cli search "network effects" --type transcriptions --speaker "Damien" --limit 50`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			query := strings.TrimSpace(strings.Join(args, " "))
			if query == "" {
				return usageErr(fmt.Errorf("search query required"))
			}
			if dbPath == "" {
				dbPath = defaultDBPath("riverside-fm-cli")
			}
			if _, statErr := os.Stat(dbPath); statErr != nil {
				return fmt.Errorf("no local store at %s yet: read some projects, takes or transcriptions first", dbPath)
			}
			s, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer s.Close()

			rawDB := s.DB()
			sql := `SELECT id, resource_type, snippet(resources_fts, 2, '[', ']', '...', 32) AS hit
FROM resources_fts WHERE resources_fts MATCH ?`
			params := []any{query}
			if resourceType != "" {
				sql += ` AND resource_type = ?`
				params = append(params, resourceType)
			}
			if speaker != "" {
				sql += ` AND content LIKE ?`
				params = append(params, "%"+speaker+"%")
			}
			sql += ` LIMIT ?`
			params = append(params, limit)

			rows, err := rawDB.QueryContext(cmd.Context(), sql, params...)
			if err != nil {
				return fmt.Errorf("query: %w", err)
			}
			defer rows.Close()

			type row struct {
				ID           string `json:"id"`
				ResourceType string `json:"resource_type"`
				Hit          string `json:"hit"`
			}
			out := []row{}
			for rows.Next() {
				var r row
				if err := rows.Scan(&r.ID, &r.ResourceType, &r.Hit); err != nil {
					return err
				}
				out = append(out, r)
			}

			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				j, _ := json.MarshalIndent(out, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(j))
				return nil
			}
			if len(out) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No matches for %q in the local store. Only resources you have read are searchable.\n", query)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Found %d matches:\n", len(out))
			for _, r := range out {
				fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s\n    %s\n", r.ResourceType, r.ID, r.Hit)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&resourceType, "type", "", "Scope to one resource type (e.g. transcriptions, projects)")
	cmd.Flags().StringVar(&speaker, "speaker", "", "Speaker substring filter (transcript-only matches)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the local store live reads cache into)")
	return cmd
}
