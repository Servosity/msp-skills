// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelContextTicketCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "context-ticket <ticketId>",
		Short: "Assemble a ticket plus its worklogs, client, and SLA into one agent-shaped JSON blob.",
		Long: `Bundle a single ticket with the worklog entries logged against it and the
client/SLA sub-objects embedded on the ticket into one --select-friendly payload
an AI triage agent can read in a single call. Accepts a ticketId or displayId.

Note: conversation and note threads are not synced locally (no list resource);
fetch those live with 'superops-cli tickets get <id>'. See README "Known Gaps".`,
		Example: strings.Trim(`
  superops-cli context-ticket 12345 --agent
  superops-cli context-ticket 12345 --agent --select ticket.subject,client.name,sla.name
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "12345"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			id := strings.TrimSpace(args[0])
			db, err := openStoreForNovel(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			tickets, err := queryRecs(db, "tickets")
			if err != nil {
				return err
			}
			worklogs, _ := queryRecs(db, "worklogs")
			ctx, ok := buildTicketContext(id, tickets, worklogs)
			if !ok {
				return fmt.Errorf("no ticket matching %q in the local store (run 'sync' first, or fetch live with 'tickets get %s')", id, id)
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), ctx, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Ticket %s: %s\n", recStr(ctx.Ticket, "displayId"), recStr(ctx.Ticket, "subject"))
			fmt.Fprintf(out, "  Client:   %s\n", recStr(ctx.Client, "name"))
			fmt.Fprintf(out, "  SLA:      %s\n", recStr(ctx.SLA, "name"))
			fmt.Fprintf(out, "  Worklogs: %d\n", len(ctx.Worklogs))
			fmt.Fprintln(out, "\n(use --agent/--json for the full agent-shaped bundle)")
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/superops-cli/data.db)")
	return cmd
}
