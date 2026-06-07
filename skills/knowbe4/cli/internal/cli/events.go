// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built command group for the KnowBe4 User Event API (separate key/host).

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/cliutil"
	"knowbe4-pp-cli/internal/userevents"
)

func newEventsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "KnowBe4 User Event API — push and read custom user risk events",
		Long: strings.TrimSpace(`
The User Event API is a separate KnowBe4 product from the Reporting API: it has its
own host and its own bearer key, KNOWBE4_USER_EVENT_API_KEY. Use it to push custom
security events (PhishRIP detections, SOC findings, external training) onto a user's
timeline so they feed the KnowBe4 risk score.

Set KNOWBE4_USER_EVENT_API_KEY (and KNOWBE4_REGION for non-US tenants) before use.`),
		RunE: parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(
		newEventsListCmd(flags),
		newEventsGetCmd(flags),
		newEventsTypesCmd(flags),
		newEventsStatusesCmd(flags),
		newEventsStatusCmd(flags),
		newEventsCreateCmd(flags),
		newEventsDeleteCmd(flags),
	)
	return cmd
}

// emitRaw decodes an API response (JSON array or object) and prints it through the
// shared filtered-JSON path so --select/--compact/--csv work.
func emitRaw(cmd *cobra.Command, flags *rootFlags, raw json.RawMessage) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// not JSON (e.g. empty 204 body) — print as-is
		fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(raw)))
		return nil
	}
	return flags.printJSON(cmd, v)
}

func newEventsListCmd(flags *rootFlags) *cobra.Command {
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List custom user risk events from the User Event API, paginated by --page and --per-page",
		Example:     "  knowbe4-cli events list --per-page 50 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			c, err := userevents.New(flags.timeout)
			if err != nil {
				return err
			}
			raw, err := c.ListEvents(cmd.Context(), page, perPage)
			if err != nil {
				return err
			}
			return emitRaw(cmd, flags, raw)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Items per page")
	return cmd
}

func newEventsGetCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get [event-id]",
		Short:       "Get a single user event by id",
		Example:     "  knowbe4-cli events get 513f46ac-c3d7-4682-ad0d-0c149c0728a2 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("exactly one event id is required: events get <event-id>")
			}
			c, err := userevents.New(flags.timeout)
			if err != nil {
				return err
			}
			raw, err := c.GetEvent(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return emitRaw(cmd, flags, raw)
		},
	}
	return cmd
}

func newEventsTypesCmd(flags *rootFlags) *cobra.Command {
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "types",
		Short:       "List configured user-event types",
		Example:     "  knowbe4-cli events types --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			c, err := userevents.New(flags.timeout)
			if err != nil {
				return err
			}
			raw, err := c.ListEventTypes(cmd.Context(), page, perPage)
			if err != nil {
				return err
			}
			return emitRaw(cmd, flags, raw)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Items per page")
	return cmd
}

func newEventsStatusesCmd(flags *rootFlags) *cobra.Command {
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "statuses",
		Short:       "List user-event request statuses",
		Example:     "  knowbe4-cli events statuses --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			c, err := userevents.New(flags.timeout)
			if err != nil {
				return err
			}
			raw, err := c.ListStatuses(cmd.Context(), page, perPage)
			if err != nil {
				return err
			}
			return emitRaw(cmd, flags, raw)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	cmd.Flags().IntVar(&perPage, "per-page", 0, "Items per page")
	return cmd
}

func newEventsStatusCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "status [request-id]",
		Short:       "Get a user-event request status by request id",
		Example:     "  knowbe4-cli events status abcdefgh-843c-4fc8-bb2f-decf89876f7b --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("exactly one request id is required: events status <request-id>")
			}
			c, err := userevents.New(flags.timeout)
			if err != nil {
				return err
			}
			raw, err := c.GetStatus(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return emitRaw(cmd, flags, raw)
		},
	}
	return cmd
}

func newEventsCreateCmd(flags *rootFlags) *cobra.Command {
	var in userevents.CreateEventInput
	var riskLevel int
	var riskLevelSet bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user event on a user's timeline",
		Long: strings.TrimSpace(`
Push a custom risk event onto a user's KnowBe4 timeline. --target-user and
--event-type are required; --event-type must match a type from 'events types'.

This is a write operation. Use --dry-run to preview the payload without sending.`),
		Example: "  knowbe4-cli events create --target-user user@acme.com --event-type \"PhishRIP Message Found\" --source SOC --risk-level 25",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("risk-level") {
				riskLevelSet = true
			}
			if riskLevelSet {
				in.RiskLevel = &riskLevel
			}
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "would create user event: target_user=%q event_type=%q source=%q\n", in.TargetUser, in.EventType, in.Source)
				return nil
			}
			if strings.TrimSpace(in.TargetUser) == "" || strings.TrimSpace(in.EventType) == "" {
				return fmt.Errorf("--target-user and --event-type are required")
			}
			c, err := userevents.New(flags.timeout)
			if err != nil {
				return err
			}
			raw, err := c.CreateEvent(cmd.Context(), in)
			if err != nil {
				return err
			}
			return emitRaw(cmd, flags, raw)
		},
	}
	cmd.Flags().StringVar(&in.TargetUser, "target-user", "", "Email or id of the user the event applies to (required)")
	cmd.Flags().StringVar(&in.EventType, "event-type", "", "Event type name, must match an existing type (required)")
	cmd.Flags().StringVar(&in.ExternalID, "external-id", "", "Your external identifier for this event")
	cmd.Flags().StringVar(&in.Source, "source", "", "Source system that produced the event")
	cmd.Flags().StringVar(&in.Description, "description", "", "Free-text description")
	cmd.Flags().StringVar(&in.OccurredDate, "occurred-date", "", "When the event occurred (ISO 8601)")
	cmd.Flags().IntVar(&riskLevel, "risk-level", 0, "Risk level for the event")
	cmd.Flags().StringVar(&in.RiskDecayMode, "risk-decay-mode", "", "Risk decay mode (e.g. linear)")
	cmd.Flags().StringVar(&in.RiskExpireDate, "risk-expire-date", "", "When the event's risk contribution expires (ISO 8601)")
	return cmd
}

func newEventsDeleteCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [event-id]",
		Short: "Delete a user event by id",
		Long: strings.TrimSpace(`
Remove a user event from a user's timeline. This is a write operation; use
--dry-run to preview without sending.`),
		Example: "  knowbe4-cli events delete 513f46ac-c3d7-4682-ad0d-0c149c0728a2 --dry-run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				id := "<event-id>"
				if len(args) == 1 {
					id = args[0]
				}
				fmt.Fprintf(cmd.OutOrStdout(), "would delete user event: %s\n", id)
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("exactly one event id is required: events delete <event-id>")
			}
			c, err := userevents.New(flags.timeout)
			if err != nil {
				return err
			}
			raw, err := c.DeleteEvent(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if len(strings.TrimSpace(string(raw))) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "deleted user event %s\n", args[0])
				return nil
			}
			return emitRaw(cmd, flags, raw)
		},
	}
	return cmd
}
