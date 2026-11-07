// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"errors"
	"fmt"
	"microsoft-graph-pp-cli/internal/credkit"
	"strings"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/client"
	"microsoft-graph-pp-cli/internal/cliutil"
)

func newNovelTranscriptsDownloadCmd(flags *rootFlags) *cobra.Command {
	var flagMeeting, flagTranscript, flagUser, flagOut string
	var noSpeakers bool

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Downloads one Teams meeting transcript as WebVTT to stdout or a named file, in the credential's own tenant only; nothing is kept on disk by the CLI.",
		Long: `Downloads one Teams meeting transcript as WebVTT. It goes to stdout, or to
the file --out names (created 0600); the CLI keeps no other copy. With --json
or --agent the text comes back inside a JSON object. Through MCP the text is
always returned and no file is ever written.

When a Teams administrator has turned off speaker attribution, Graph answers
403 SpeakerAttributionNotAllowed: pass --no-speakers to download the text
without speaker names. Own tenant only; see 'transcripts --help'.`,
		Example: `  microsoft-graph-cli transcripts download --meeting '<meeting-id>' --transcript '<transcript-id>' --out meeting.vtt
  microsoft-graph-cli transcripts download --meeting '<meeting-id>' --transcript '<transcript-id>' --agent`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := transcriptsKeepNothing(flags); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "transcripts download")
			}
			if flagOut != "" && cliutil.IsMCPChildEnv() {
				return usageErr(errors.New("--out is not available through MCP: the transcript text is returned instead of written to a file"))
			}
			if strings.TrimSpace(flagTranscript) == "" {
				return usageErr(errors.New("--transcript is required (the id 'transcripts list' prints)"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			claims, err := transcriptsGate(c)
			if err != nil {
				return err
			}
			base, err := meetingTranscriptsPath(claims, flagUser, flagMeeting)
			if err != nil {
				return err
			}
			path := replacePathParam(base+"/{transcript}/content", "transcript", flagTranscript)
			params := map[string]string{"$format": "text/vtt"}
			var headers map[string]string
			if noSpeakers {
				params = nil
				headers = map[string]string{"Accept": "application/vnd.microsoft.graph.transcript+text"}
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			body, err := c.GetWithHeaders(ctx, path, params, headers)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			text := []byte(body)
			if raw, _, ok := client.UnwrapBinaryResponse(text); ok {
				text = raw
			}
			if flagOut != "" && flagOut != "-" {
				if err := credkit.WriteFile(flagOut, text, 0o600); err != nil { // #nosec G306 G703 -- the operator named this CLI-only output file
					return fmt.Errorf("writing %s: %w", flagOut, err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "wrote %d bytes to %s\n", len(text), flagOut)
				return nil
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"meetingId": flagMeeting, "transcriptId": flagTranscript, "format": transcriptFormat(noSpeakers), "content": string(text),
				}, flags)
			}
			_, err = cmd.OutOrStdout().Write(text)
			return err
		},
	}
	cmd.Flags().StringVar(&flagMeeting, "meeting", "", "Online meeting id the transcript belongs to")
	cmd.Flags().StringVar(&flagTranscript, "transcript", "", "Transcript id, from 'transcripts list'")
	cmd.Flags().StringVar(&flagUser, "user", "", "Meeting organizer's user id; app-only tokens only")
	cmd.Flags().StringVar(&flagOut, "out", "", "Write the WebVTT text to this file (0600) instead of stdout; CLI only, never through MCP")
	cmd.Flags().BoolVar(&noSpeakers, "no-speakers", false, "Download without speaker names (for tenants where speaker attribution is off)")
	return cmd
}

// transcriptFormat names the format the request asked Graph for: WebVTT with
// speaker voice tags, or the unattributed transcript text --no-speakers gets.
func transcriptFormat(noSpeakers bool) string {
	if noSpeakers {
		return "application/vnd.microsoft.graph.transcript+text"
	}
	return "text/vtt"
}
