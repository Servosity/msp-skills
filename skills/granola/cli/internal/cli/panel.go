// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/client"
)

func newPanelCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "panel",
		Short: "Read AI panels for a meeting",
	}
	cmd.AddCommand(newPanelGetCmd(flags))
	return cmd
}

func newPanelGetCmd(flags *rootFlags) *cobra.Command {
	var template string
	var asMarkdown, asPlain bool
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Fetch the AI panel/summary for a meeting",
		Long: `Returns the meeting's AI summary from the public API (synced into the
local store as the "summary" panel). On Granola v7.4x+ the internal
named-panel-templates endpoint is sealed; the public API exposes the
single AI summary, which this serves. --template summary selects it.`,
		Example: strings.Trim(`
  granola-cli panel get not_06Yq6JtogRihEr
  granola-cli panel get not_06Yq6JtogRihEr --template summary --markdown
  granola-cli panel get not_06Yq6JtogRihEr --template summary --plain`, "\n"),
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
			id := args[0]
			if flags.dataSource == "local" {
				// The AI summary is only in the live detail endpoint, not the
				// local store; --data-source local can't serve it.
				return notFoundErr(fmt.Errorf("AI summary for %s is not stored locally; omit --data-source local to fetch it", id))
			}
			panels, err := restSummaryPanel(cmd.Context(), id)
			if err != nil {
				return err
			}
			if template != "" {
				val, ok := panels[template]
				if !ok {
					return notFoundErr(fmt.Errorf("panel template %q not present for meeting %s", template, id))
				}
				if asPlain || asMarkdown {
					fmt.Fprintln(cmd.OutOrStdout(), val)
					return nil
				}
				return emitJSON(cmd, flags, map[string]string{template: val})
			}
			if asMarkdown {
				for slug, content := range panels {
					fmt.Fprintf(cmd.OutOrStdout(), "## %s\n\n%s\n\n", slug, content)
				}
				return nil
			}
			return emitJSON(cmd, flags, panels)
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "Select one panel by template slug")
	cmd.Flags().BoolVar(&asMarkdown, "markdown", false, "Render as markdown sections")
	cmd.Flags().BoolVar(&asPlain, "plain", false, "Print the selected panel as plain text")
	return cmd
}

// restSummaryPanel fetches the meeting's AI summary live from the public API
// (the /v1/notes/{id} detail carries summary_markdown/summary_text; the list
// sync does not), shaped as a {"summary": ...} panel map.
func restSummaryPanel(ctx context.Context, id string) (map[string]string, error) {
	cfg, err := granolaRESTConfig()
	if err != nil {
		return nil, configErr(err)
	}
	c := client.New(cfg, 30*time.Second, 0)
	// id is an opaque single-segment note id; escape it so reserved characters
	// stay inside this path segment (matches rest_hydrate.go).
	raw, err := c.Get(ctx, "/v1/notes/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, apiErr(fmt.Errorf("fetch AI summary for %s from the public API: %w", id, err))
	}
	var d restNoteDetail
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, apiErr(fmt.Errorf("parse summary for %s: %w", id, err))
	}
	if d.ID != "" && d.ID != id {
		return nil, apiErr(fmt.Errorf("summary for %s: the API answered for a different meeting (%q)", id, d.ID))
	}
	summary := d.SummaryMD
	if summary == "" {
		summary = d.SummaryText
	}
	if summary == "" {
		return nil, notFoundErr(fmt.Errorf("no AI summary available for meeting %s", id))
	}
	return map[string]string{"summary": summary}, nil
}
