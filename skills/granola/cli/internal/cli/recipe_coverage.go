// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

func newRecipeCoverageCmd(flags *rootFlags) *cobra.Command {
	var since, until, last string
	var limit int
	cmd := &cobra.Command{
		Use:   "coverage [recipe-slug]",
		Short: "List meetings in the window that do NOT have a named panel applied",
		Long: `For each meeting in the window, calls /v1/get-document-panels and
emits ndjson for meetings where the slug is missing.`,
		Example: strings.Trim(`
  granola-cli recipes coverage action-items --last 30d
  granola-cli recipes coverage exec-summary --since 2026-07-01 --limit 50`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			slug := args[0]
			from, to, err := parseTimeWindow(last, since, until)
			if err != nil {
				return usageErr(err)
			}
			c, err := openGranolaCache()
			if err != nil {
				return err
			}
			ids := selectDocsInWindow(c, from, to, limit)
			// Named panel templates (recipes) live ONLY in Granola's internal
			// panels API, which is sealed on v7.4x+. The public API exposes just
			// the single AI summary, not named templates — so coverage cannot be
			// computed. Fail honestly rather than report every meeting as
			// "missing" (a fabricated negative).
			ic, ierr := granola.NewInternalClient()
			if ierr != nil {
				return apiErr(fmt.Errorf("recipe coverage is unavailable: named-panel data lives only in Granola's internal API, which is sealed on Granola v7.4x+ (the public API exposes the AI summary but not named panel templates)"))
			}
			w := cmd.OutOrStdout()
			for _, id := range ids {
				d := c.DocumentByID(id)
				panels, perr := ic.GetDocumentPanels(id)
				if perr != nil {
					// Don't fabricate "missing" from a fetch failure — the named
					// panels endpoint is sealed on v7.4x+.
					return apiErr(fmt.Errorf("recipe coverage is unavailable: named-panel data lives only in Granola's internal API, which is sealed on Granola v7.4x+"))
				}
				if _, has := panels[slug]; has {
					continue
				}
				_ = emitNDJSONLine(w, map[string]any{
					"id":             id,
					"title":          d.Title,
					"started_at":     d.CreatedAt,
					"missing_recipe": slug,
				})
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Start date")
	cmd.Flags().StringVar(&until, "until", "", "End date")
	cmd.Flags().StringVar(&last, "last", "", "Time window")
	cmd.Flags().IntVar(&limit, "limit", 0, "Cap meetings checked")
	return cmd
}

// Ensure time/fmt referenced.
var (
	_ = time.Now
	_ = fmt.Sprintf
)
