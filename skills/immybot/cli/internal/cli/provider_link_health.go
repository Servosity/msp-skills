// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: every broken RMM/PSA provider link in one list.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// providerLinkRecord is the slice of GET /api/v1/provider-links
// (GetProviderLinkResponse) this command reads.
type providerLinkRecord struct {
	ID                                 json.Number     `json:"id"`
	Name                               string          `json:"name"`
	Disabled                           bool            `json:"disabled"`
	HealthStatus                       json.RawMessage `json:"healthStatus"`
	HealthStatusMessage                string          `json:"healthStatusMessage"`
	ErrorMessage                       string          `json:"errorMessage"`
	IncludeClientsFailedMessage        string          `json:"includeClientsFailedMessage"`
	GetFormSchemaFailedMessage         string          `json:"getFormSchemaFailedMessage"`
	GetProviderFailedMessage           string          `json:"getProviderFailedMessage"`
	GetLatestAgentVersionFailedMessage string          `json:"getLatestAgentVersionFailedMessage"`
	OwnerTenantID                      *json.Number    `json:"ownerTenantId"`
	ProviderTypeID                     string          `json:"providerTypeId"`
	UpdatedDateUTC                     string          `json:"updatedDateUTC"`
}

type providerLinkIssue struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Health         string            `json:"health"`
	Disabled       bool              `json:"disabled"`
	Reasons        []string          `json:"reasons"`
	Messages       map[string]string `json:"messages"`
	OwnerTenantID  string            `json:"owner_tenant_id,omitempty"`
	OwnerTenant    string            `json:"owner_tenant,omitempty"`
	ProviderTypeID string            `json:"provider_type_id,omitempty"`
	UpdatedDate    string            `json:"updated_date,omitempty"`
}

type providerLinkHealthView struct {
	Scanned          int                 `json:"scanned"`
	Healthy          int                 `json:"healthy"`
	FlaggedCount     int                 `json:"flagged_count"`
	Unhealthy        int                 `json:"unhealthy"`
	Degraded         int                 `json:"degraded"`
	Disabled         int                 `json:"disabled"`
	HealthUnreported int                 `json:"health_unreported"`
	Unreadable       int                 `json:"unreadable_rows"`
	Links            []providerLinkIssue `json:"links"`
	Note             string              `json:"note,omitempty"`
}

// providerLinkHealthName normalizes healthStatus, which ImmyBot documents as
// "the string or integer value may be used. Unhealthy = 0, Degraded = 1,
// Healthy = 2". Anything else is "unknown" and is flagged, never read as
// healthy; an absent field is "unreported".
func providerLinkHealthName(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return "unreported"
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		s = str
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "unhealthy":
		return "Unhealthy"
	case "1", "degraded":
		return "Degraded"
	case "2", "healthy":
		return "Healthy"
	}
	return "unknown"
}

// providerLinkFindings lists every failure signal on a link. Any one of them
// flags the link: a populated failure message counts whatever the health
// field says, because ImmyBot records them independently.
func providerLinkFindings(r providerLinkRecord) (health string, reasons []string, messages map[string]string) {
	health = providerLinkHealthName(r.HealthStatus)
	reasons = []string{}
	messages = map[string]string{}
	switch health {
	case "Unhealthy", "Degraded":
		reasons = append(reasons, "health "+strings.ToLower(health))
	case "unknown":
		reasons = append(reasons, "health status not recognised")
	case "unreported":
		reasons = append(reasons, "health not reported")
	}
	if r.Disabled {
		reasons = append(reasons, "disabled")
	}
	for _, m := range []struct{ key, text, reason string }{
		{"error", r.ErrorMessage, "error recorded"},
		{"include_clients", r.IncludeClientsFailedMessage, "client sync failed"},
		{"form_schema", r.GetFormSchemaFailedMessage, "form schema load failed"},
		{"provider", r.GetProviderFailedMessage, "provider load failed"},
		{"latest_agent_version", r.GetLatestAgentVersionFailedMessage, "agent version check failed"},
	} {
		if t := strings.TrimSpace(m.text); t != "" {
			messages[m.key] = t
			reasons = append(reasons, m.reason)
		}
	}
	if t := strings.TrimSpace(r.HealthStatusMessage); t != "" && len(reasons) > 0 {
		messages["health"] = t
	}
	return health, reasons, messages
}

func providerLinkSeverity(l providerLinkIssue) int {
	switch l.Health {
	case "Unhealthy":
		return 0
	case "Degraded":
		return 1
	case "unknown":
		return 2
	}
	if len(l.Messages) > 0 {
		return 3
	}
	if l.Disabled {
		return 4
	}
	return 5
}

func newNovelProviderLinkHealthCmd(flags *rootFlags) *cobra.Command {
	var (
		flagIncludeDisabled bool
		flagLimit           int
		dbPath              string
	)
	cmd := &cobra.Command{
		Use:   "provider-link-health",
		Short: "Every RMM or PSA provider link that is disabled, unhealthy or reporting a sync failure",
		Long: "List provider links from the local mirror whose healthStatus is Unhealthy or Degraded, that are " +
			"disabled, or that carry any failure message ImmyBot recorded (errorMessage and the client, form-schema, " +
			"provider and agent-version failure messages on GET /api/v1/provider-links). A broken link silently stops " +
			"agent and client syncing. Run 'sync --resources provider-links,tenants' first.",
		Example: strings.Trim(`
  immybot-cli provider-link-health --agent
  immybot-cli provider-link-health --include-disabled=false
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "provider-link-health")
			}
			if flagLimit < 0 {
				return usageErr(fmt.Errorf("--limit must be 0 (all) or more, got %d", flagLimit))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			view := providerLinkHealthView{Links: make([]providerLinkIssue, 0)}
			dbPath = immyMirrorPath(dbPath)
			if immyMirrorMissing(cmd, dbPath, "provider-links,tenants") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			db, err := immyOpenMirror(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "provider-links") {
				hintIfStale(cmd, db, "provider-links", flags.maxAge)
			}
			tenants, err := immyTenantNames(ctx, db)
			if err != nil {
				return err
			}
			rows, err := db.DB().QueryContext(ctx, `SELECT id, data FROM resources WHERE resource_type = 'provider-links'`)
			if err != nil {
				if immyNoSuchTableErr(err) {
					view.Note = "no provider links in the local mirror; run 'immybot-cli sync --resources provider-links,tenants'"
					return printProviderLinkHealth(cmd, flags, view)
				}
				return fmt.Errorf("querying provider links: %w", err)
			}
			for rows.Next() {
				var id string
				var data []byte
				if err := rows.Scan(&id, &data); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning provider link row: %w", err)
				}
				view.Scanned++
				var rec providerLinkRecord
				dec := json.NewDecoder(strings.NewReader(string(data)))
				dec.UseNumber()
				if err := dec.Decode(&rec); err != nil {
					view.Unreadable++
					continue
				}
				health, reasons, messages := providerLinkFindings(rec)
				onlyDisabled := len(reasons) == 1 && reasons[0] == "disabled"
				if len(reasons) == 0 || (onlyDisabled && !flagIncludeDisabled) {
					if len(reasons) == 0 {
						view.Healthy++
					}
					continue
				}
				l := providerLinkIssue{
					ID: rec.ID.String(), Name: rec.Name, Health: health, Disabled: rec.Disabled,
					Reasons: reasons, Messages: messages, ProviderTypeID: rec.ProviderTypeID,
					UpdatedDate: rec.UpdatedDateUTC,
				}
				if l.ID == "" {
					l.ID = id
				}
				if rec.OwnerTenantID != nil {
					l.OwnerTenantID = rec.OwnerTenantID.String()
					l.OwnerTenant = tenants[l.OwnerTenantID]
				}
				view.FlaggedCount++
				switch health {
				case "Unhealthy":
					view.Unhealthy++
				case "Degraded":
					view.Degraded++
				case "unreported":
					view.HealthUnreported++
				}
				if rec.Disabled {
					view.Disabled++
				}
				view.Links = append(view.Links, l)
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("iterating provider links: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("closing provider links: %w", err)
			}
			sort.SliceStable(view.Links, func(i, j int) bool {
				a, b := view.Links[i], view.Links[j]
				if sa, sb := providerLinkSeverity(a), providerLinkSeverity(b); sa != sb {
					return sa < sb
				}
				if a.Name != b.Name {
					return a.Name < b.Name
				}
				return a.ID < b.ID
			})
			if flagLimit > 0 && len(view.Links) > flagLimit {
				view.Links = view.Links[:flagLimit]
			}
			switch {
			case view.Scanned == 0:
				view.Note = "no provider links in the local mirror; run 'immybot-cli sync --resources provider-links,tenants'"
			case view.FlaggedCount == 0:
				view.Note = fmt.Sprintf("scanned %d provider link(s); none is unhealthy, degraded or reporting a failure", view.Scanned)
			}
			if view.Unreadable > 0 {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" %d provider link row(s) could not be read and are not reported on.", view.Unreadable))
			}
			return printProviderLinkHealth(cmd, flags, view)
		},
	}
	cmd.Flags().BoolVar(&flagIncludeDisabled, "include-disabled", true, "List links that are only disabled")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum links to list (0 for all); totals count every match")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

func printProviderLinkHealth(cmd *cobra.Command, flags *rootFlags, view providerLinkHealthView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), view, flags)
	}
	w := cmd.OutOrStdout()
	if len(view.Links) == 0 {
		fmt.Fprintf(w, "No broken provider links (scanned %d).\n", view.Scanned)
		if view.Note != "" {
			fmt.Fprintln(w, immyPrintable(view.Note))
		}
		return nil
	}
	fmt.Fprintf(w, "%-8s %-30s %-11s %s\n", "ID", "PROVIDER LINK", "HEALTH", "REASONS")
	for _, l := range view.Links {
		fmt.Fprintf(w, "%-8s %-30s %-11s %s\n", immyPrintable(l.ID), immyTruncate(immyPrintable(l.Name), 30),
			l.Health, strings.Join(l.Reasons, ", "))
		keys := make([]string, 0, len(l.Messages))
		for k := range l.Messages {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "         %s: %s\n", k, immyTruncate(immyPrintable(l.Messages[k]), 160))
		}
	}
	fmt.Fprintf(w, "\n%d flagged: %d unhealthy, %d degraded, %d disabled (scanned %d).\n",
		view.FlaggedCount, view.Unhealthy, view.Degraded, view.Disabled, view.Scanned)
	if view.Note != "" {
		fmt.Fprintln(w, immyPrintable(view.Note))
	}
	return nil
}
