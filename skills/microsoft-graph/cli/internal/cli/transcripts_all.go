// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/spf13/cobra"
)

// organizerIDPattern keeps --organizer to a user id or UPN, so it cannot break
// out of the quoted OData function parameter it is placed in.
var organizerIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@._-]*$`)

func newNovelTranscriptsAllCmd(flags *rootFlags) *cobra.Command {
	var flagOrganizer, flagSince string

	cmd := &cobra.Command{
		Use:   "all",
		Short: "Lists every transcript from the meetings one user organized, in the credential's own tenant only (Graph offers this to application tokens only, behind an explicit opt-in).",
		Long: `Lists every transcript from the meetings one user organized
(getAllTranscripts). Graph offers this call to application tokens only, so it
needs MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1 and a Teams application access
policy granted for the organizer; a delegated token is refused. Own tenant only;
see 'transcripts --help'.`,
		Example:     "  MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1 microsoft-graph-cli transcripts all --organizer '<user-id>' --since 7d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := transcriptsKeepNothing(flags); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "transcripts all")
			}
			if !organizerIDPattern.MatchString(flagOrganizer) {
				return usageErr(errors.New("--organizer is required: the organizer's user id or userPrincipalName"))
			}
			var since time.Time
			if flagSince != "" {
				t, err := parseSinceDuration(flagSince)
				if err != nil {
					return usageErr(fmt.Errorf("--since: %w", err))
				}
				since = t
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			claims, err := transcriptsGate(c)
			if err != nil {
				return err
			}
			if !claims.appOnly() {
				return authErr(errors.New("transcripts all needs an application token: Graph's getAllTranscripts does not accept a delegated token. Use 'transcripts list --meeting' for your own meetings"))
			}
			fn := fmt.Sprintf("getAllTranscripts(meetingOrganizerUserId='%s'", flagOrganizer)
			if !since.IsZero() {
				fn += ",startDateTime=" + since.UTC().Format(time.RFC3339)
			}
			path := replacePathParam("/users/{user}/onlineMeetings/", "user", flagOrganizer) + fn + ")"
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			items, err := graphGetAllPages(ctx, c, path, nil)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			return printJSONFiltered(cmd.OutOrStdout(), items, flags)
		},
	}
	cmd.Flags().StringVar(&flagOrganizer, "organizer", "", "Organizer's user id or userPrincipalName")
	cmd.Flags().StringVar(&flagSince, "since", "", "Only meetings since this long ago: 7d, 24h, 1w or 30m")
	return cmd
}
