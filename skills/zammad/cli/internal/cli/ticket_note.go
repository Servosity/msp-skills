// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelTicketNoteCmd(flags *rootFlags) *cobra.Command {
	var flagBody string
	var flagInternal bool
	var flagPublic bool
	var flagSubject string

	cmd := &cobra.Command{
		Use:         "note <ticket_id>",
		Short:       "Add an internal or partner-visible note to a ticket in one line, with correct content-type defaults.",
		Example:     "  zammad-cli ticket note 12345 --body \"Investigated, awaiting logs\" --internal --dry-run",
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			internal := flagInternal
			if flagPublic {
				internal = false
			}
			if dryRunOK(flags) {
				if len(args) == 0 {
					return nil
				}
				if _, err := positiveTicketID(args[0]); err != nil {
					return err
				}
				if strings.TrimSpace(flagBody) == "" {
					return usageErr(fmt.Errorf("--body is required"))
				}
				return printJSONFiltered(cmd.OutOrStdout(), buildTicketNoteBody(args, flagBody, internal, flagSubject), flags)
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return usageErr(fmt.Errorf("ticket_id is required\nUsage: %s <ticket_id> --body <body>", cmd.CommandPath()))
			}
			ticketID, err := positiveTicketID(args[0])
			if err != nil {
				return err
			}
			if strings.TrimSpace(flagBody) == "" {
				return usageErr(fmt.Errorf("--body is required"))
			}
			body := ticketNoteBody{
				TicketID:    ticketID,
				Body:        flagBody,
				Type:        "note",
				Internal:    internal,
				ContentType: "text/html",
				Subject:     strings.TrimSpace(flagSubject),
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			data, status, err := c.Post(cmd.Context(), "/ticket_articles", body.toMap())
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if status < 200 || status >= 300 {
				return fmt.Errorf("POST /ticket_articles returned HTTP %d", status)
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
				"action":  "post",
				"path":    "/ticket_articles",
				"status":  status,
				"success": true,
				"data":    data,
			}, flags)
		},
	}
	cmd.Flags().StringVar(&flagBody, "body", "", "Note body to add to the ticket")
	cmd.Flags().BoolVar(&flagInternal, "internal", true, "Add the note as an internal article")
	cmd.Flags().BoolVar(&flagPublic, "public", false, "Add the note as a public article")
	cmd.Flags().StringVar(&flagSubject, "subject", "", "Optional note subject")
	return cmd
}

func positiveTicketID(value string) (int, error) {
	ticketID, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, usageErr(fmt.Errorf("ticket_id must be an integer"))
	}
	if ticketID <= 0 {
		return 0, usageErr(fmt.Errorf("ticket_id must be a positive integer"))
	}
	return ticketID, nil
}

type ticketNoteBody struct {
	TicketID    int    `json:"ticket_id"`
	Body        string `json:"body"`
	Type        string `json:"type"`
	Internal    bool   `json:"internal"`
	ContentType string `json:"content_type"`
	Subject     string `json:"subject,omitempty"`
}

func buildTicketNoteBody(args []string, body string, internal bool, subject string) map[string]any {
	ticketID := 0
	if len(args) > 0 {
		if parsed, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil {
			ticketID = parsed
		}
	}
	payload := ticketNoteBody{
		TicketID:    ticketID,
		Body:        body,
		Type:        "note",
		Internal:    internal,
		ContentType: "text/html",
		Subject:     strings.TrimSpace(subject),
	}
	return payload.toMap()
}

func (b ticketNoteBody) toMap() map[string]any {
	payload := map[string]any{
		"ticket_id":    b.TicketID,
		"body":         b.Body,
		"type":         b.Type,
		"internal":     b.Internal,
		"content_type": b.ContentType,
	}
	if strings.TrimSpace(b.Subject) != "" {
		payload["subject"] = strings.TrimSpace(b.Subject)
	}
	return payload
}
