// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

type noteResult struct {
	Key       string `json:"key"`
	CommentID string `json:"comment_id,omitempty"`
	Public    bool   `json:"public"`
	Body      string `json:"body"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

func newNovelRequestNoteCmd(flags *rootFlags) *cobra.Command {
	var fromStdin bool

	cmd := &cobra.Command{
		Use:   "note <issueIdOrKey> [text]",
		Short: "Add an agent-only internal note to a request: always posts public=false and errors if JSM stores it as public.",
		Long: strings.Trim(`
Posts a comment with public=false, so only agents see it. Text comes from the
second argument or, with --stdin, from standard input.

Use this command to add an agent-only internal note to a request.
Do NOT use this command to reply to the customer; use 'request comment add' with public=true instead.`, "\n"),
		Example: strings.Trim(`
  jira-service-management-cli request note SD-123 "Escalated to network team" --dry-run
  echo "Vendor RMA pending" | jira-service-management-cli request note SD-123 --stdin`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:happy-args": "issueIdOrKey=SD-1;text=example note;--dry-run"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				if dryRunOK(flags) {
					return writeDryRun(cmd.OutOrStdout(), flags, "request note")
				}
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a request key or id is required, e.g. SD-123"))
			}
			key := strings.ToUpper(strings.TrimSpace(args[0]))
			// Strict: a key like SD-123 or a numeric id. Anything else (e.g.
			// "SD-1/../../..") could route the POST to a public comment API.
			if !noteKeyOK(key) {
				return usageErr(fmt.Errorf("%q is not a request key (SD-123) or numeric id", key))
			}
			var text string
			switch {
			case len(args) > 1:
				text = strings.Join(args[1:], " ")
			case fromStdin:
				if dryRunOK(flags) {
					return writeDryRun(cmd.OutOrStdout(), flags, "request note")
				}
				const maxNote = 1 << 20
				b, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxNote+1))
				if err != nil {
					return fmt.Errorf("reading note from stdin: %w", err)
				}
				if len(b) > maxNote {
					return usageErr(fmt.Errorf("note on stdin is larger than %d bytes; nothing was posted", maxNote))
				}
				text = string(b)
			}
			text = strings.TrimSpace(text)
			if text == "" {
				if dryRunOK(flags) {
					return writeDryRun(cmd.OutOrStdout(), flags, "request note")
				}
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("note text is required: pass it as the second argument or use --stdin"))
			}
			// The whole point of this verb: never public.
			body := map[string]any{"body": text, "public": false}
			if dryRunOK(flags) {
				res := noteResult{Key: key, Public: false, Body: text, DryRun: true}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"dry_run": true, "action": "request note", "would": map[string]any{"method": "POST", "path": "/request/" + key + "/comment", "body": body}, "result": res}, flags)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would POST /request/%s/comment {\"public\": false} with %d chars\n", key, len(text))
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			raw, _, err := c.Post(ctx, "/request/"+key+"/comment", body)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			var created struct {
				ID     flexString `json:"id"`
				Public *bool      `json:"public"`
			}
			if err := json.Unmarshal(raw, &created); err != nil || created.Public == nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: JSM's response did not confirm the comment's visibility; check %s in the agent view\n", key)
			}
			if created.Public != nil && *created.Public {
				return fmt.Errorf("JSM stored comment %s on %s as PUBLIC despite public=false; check the request type's comment settings", termSafe(string(created.ID)), key)
			}
			res := noteResult{Key: key, CommentID: termSafe(string(created.ID)), Public: false, Body: text}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), res, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "internal note %s added to %s\n", res.CommentID, key)
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "Read the note text from standard input")
	return cmd
}

// noteKeyOK accepts a request key (SD-123, MY_PROJ-7) or a numeric issue id.
func noteKeyOK(key string) bool {
	return key != "" && (issueKeyLike(key) || strings.Trim(key, "0123456789") == "")
}
