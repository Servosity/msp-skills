// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelTranscriptsListCmd(flags *rootFlags) *cobra.Command {
	var flagMeeting, flagUser string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists the transcripts of one Teams meeting the signed-in user can already open, in the credential's own tenant only.",
		Long: `Lists the transcripts of one Teams online meeting: id, created and end
time, and organizer. A delegated token reads the signed-in user's meeting
(/me/onlineMeetings); an opted-in app-only token reads the organizer named by
--user. Own tenant only; see 'transcripts --help'.`,
		Example: `  microsoft-graph-cli transcripts list --meeting '<meeting-id>' --agent
  MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1 microsoft-graph-cli transcripts list --meeting '<meeting-id>' --user '<organizer-id>'`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := transcriptsKeepNothing(flags); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "transcripts list")
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			claims, err := transcriptsGate(c)
			if err != nil {
				return err
			}
			path, err := meetingTranscriptsPath(claims, flagUser, flagMeeting)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			items, err := graphGetAllPages(ctx, c, path, nil)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			return printJSONFiltered(cmd.OutOrStdout(), items, flags)
		},
	}
	cmd.Flags().StringVar(&flagMeeting, "meeting", "", "Online meeting id (the onlineMeeting resource id, e.g. MSo1N2Y5ZGFjYy03MWJmLTQ3NDMtYjQxMy01M2EdFGkdRWHJlQ)")
	cmd.Flags().StringVar(&flagUser, "user", "", "Meeting organizer's user id; app-only tokens only (a delegated token reads the signed-in user's meetings)")
	return cmd
}
