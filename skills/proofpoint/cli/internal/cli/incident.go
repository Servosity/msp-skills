// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature for proofpoint-cli.

// pp:data-source auto
package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"proofpoint-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type threatSummaryView struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name,omitempty"`
	Type         string   `json:"type,omitempty"`
	Category     string   `json:"category,omitempty"`
	Status       string   `json:"status,omitempty"`
	Severity     *int     `json:"severity,omitempty"`
	AttackSpread *int     `json:"attack_spread,omitempty"`
	IdentifiedAt string   `json:"identified_at,omitempty"`
	Notable      bool     `json:"notable,omitempty"`
	Actors       []string `json:"actors,omitempty"`
	Families     []string `json:"families,omitempty"`
	Malware      []string `json:"malware,omitempty"`
	Techniques   []string `json:"techniques,omitempty"`
	Brands       []string `json:"brands,omitempty"`
}

func decodeThreatSummary(data json.RawMessage) (threatSummaryView, error) {
	var raw struct {
		ID           string           `json:"id"`
		Name         string           `json:"name"`
		Type         string           `json:"type"`
		Category     string           `json:"category"`
		Status       string           `json:"status"`
		Severity     *int             `json:"severity"`
		AttackSpread *int             `json:"attackSpread"`
		IdentifiedAt string           `json:"identifiedAt"`
		Notable      bool             `json:"notable"`
		Actors       []campaignIDName `json:"actors"`
		Families     []campaignIDName `json:"families"`
		Malware      []campaignIDName `json:"malware"`
		Techniques   []campaignIDName `json:"techniques"`
		Brands       []campaignIDName `json:"brands"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return threatSummaryView{}, fmt.Errorf("parsing threat summary: %w", err)
	}
	return threatSummaryView{
		ID:           raw.ID,
		Name:         raw.Name,
		Type:         raw.Type,
		Category:     raw.Category,
		Status:       raw.Status,
		Severity:     raw.Severity,
		AttackSpread: raw.AttackSpread,
		IdentifiedAt: raw.IdentifiedAt,
		Notable:      raw.Notable,
		Actors:       namesOf(raw.Actors),
		Families:     namesOf(raw.Families),
		Malware:      namesOf(raw.Malware),
		Techniques:   namesOf(raw.Techniques),
		Brands:       namesOf(raw.Brands),
	}, nil
}

// queryThreatEvents scans synced SIEM events for any tied to the threatId —
// clicks carry a scalar threatID; messages carry threatsInfoMap entries.
func queryThreatEvents(ctx context.Context, db *sql.DB, threatID string, limit int) ([]userEvent, error) {
	q := `
		SELECT resource_type, data FROM resources
		WHERE resource_type IN ('siem-clicks-blocked','siem-clicks-permitted','siem-messages-blocked','siem-messages-delivered')
		  AND (
			COALESCE(json_extract(data,'$.threatID'),'') = ?1
			OR EXISTS (SELECT 1 FROM json_each(data,'$.threatsInfoMap') je WHERE json_extract(je.value,'$.threatId') = ?1)
		  )
		ORDER BY COALESCE(json_extract(data,'$.clickTime'), json_extract(data,'$.messageTime'), '') DESC
		LIMIT ?2`
	rows, err := db.QueryContext(ctx, q, threatID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying local events: %w", err)
	}
	defer rows.Close()
	events := make([]userEvent, 0)
	for rows.Next() {
		var resourceType string
		var data []byte
		if err := rows.Scan(&resourceType, &data); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		ev := decodeUserEvent(resourceType, data)
		// Project the affected recipient into the sender column slot via
		// subject/url fields already carried; recipients matter most here.
		events = append(events, ev)
	}
	return events, rows.Err()
}

type incidentView struct {
	ThreatID      string            `json:"threat_id"`
	Summary       threatSummaryView `json:"summary"`
	Reports       int               `json:"reports"`
	IOCCount      int               `json:"ioc_count"`
	IOCs          []iocRow          `json:"iocs"`
	LocalEvents   int               `json:"local_events"`
	Events        []userEvent       `json:"events"`
	FetchFailures []string          `json:"fetch_failures,omitempty"`
	Note          string            `json:"note,omitempty"`
}

func newNovelIncidentCmd(flags *rootFlags) *cobra.Command {
	var flagIncludeCampaign bool
	var flagLimit int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "incident <threatId>",
		Short: "One incident brief from a threatId: severity, attribution, evidence, and local events",
		Long: strings.Trim(`
Compose a single incident brief from three sources: the threat summary
endpoint (severity, actors, malware, techniques), the forensics endpoint
(flattened indicator evidence), and locally synced events that touched the
threatId.

Use this command for a full single-incident brief from a threatId. Do NOT use
this command to extract just the raw indicators for blocking; use 'iocs'
instead.`, "\n"),
		Example: strings.Trim(`
  proofpoint-cli incident "abc123threataggregateid"
  proofpoint-cli incident "abc123threataggregateid" --include-campaign-forensics --agent
  proofpoint-cli incident "abc123threataggregateid" --agent --select summary.severity,summary.malware,ioc_count`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "example-threat-id"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compose threat summary + forensics + local events into one brief")
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a threatId argument is required"))
			}
			if err := validateDataSourceStrategy(flags, "auto"); err != nil {
				return err
			}
			threatID := strings.TrimSpace(args[0])

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			view := incidentView{
				ThreatID:      threatID,
				IOCs:          make([]iocRow, 0),
				Events:        make([]userEvent, 0),
				FetchFailures: make([]string, 0),
			}

			summaryPath := replacePathParam("/threat/summary/{threatId}", "threatId", threatID)
			summaryData, summaryErr := c.Get(cmd.Context(), summaryPath, nil)
			if summaryErr != nil {
				view.FetchFailures = append(view.FetchFailures, fmt.Sprintf("threat summary: %v", summaryErr))
			} else if summary, err := decodeThreatSummary(summaryData); err != nil {
				view.FetchFailures = append(view.FetchFailures, fmt.Sprintf("threat summary: %v", err))
			} else {
				view.Summary = summary
			}

			envelope, forensicsErr := fetchForensics(cmd.Context(), c, threatID, "", flagIncludeCampaign)
			if forensicsErr != nil {
				view.FetchFailures = append(view.FetchFailures, fmt.Sprintf("forensics: %v", forensicsErr))
			} else {
				view.Reports = len(envelope.Reports)
				view.IOCs = flattenForensicReports(envelope.Reports, false)
				view.IOCCount = len(view.IOCs)
			}

			if summaryErr != nil && forensicsErr != nil {
				return classifyAPIError(summaryErr, flags)
			}

			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("proofpoint-cli")
			}
			if db, dbErr := store.OpenWithContext(cmd.Context(), dbPath); dbErr == nil {
				defer db.Close()
				if !hintIfUnsynced(cmd, db, "siem-clicks-permitted") {
					hintIfStale(cmd, db, "siem-clicks-permitted", flags.maxAge)
				}
				if events, err := queryThreatEvents(cmd.Context(), db.DB(), threatID, flagLimit); err == nil {
					view.Events = events
					view.LocalEvents = len(events)
				} else {
					view.FetchFailures = append(view.FetchFailures, fmt.Sprintf("local events: %v", err))
				}
			}

			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d incident sources failed; the brief is partial\n", len(view.FetchFailures))
			}
			if view.LocalEvents == 0 {
				view.Note = "no local events reference this threatId — run 'backfill' to populate the store, or the threat may predate your lookback"
			}
			if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
				return err
			}
			// A partial brief (one live source failed) surfaces as exit 6
			// unless the caller opted into tolerance with --allow-partial-failure.
			if len(view.FetchFailures) > 0 && !flags.allowPartialFailure {
				return partialFailureErr(fmt.Errorf("incident brief is partial: %s (pass --allow-partial-failure to tolerate)", strings.Join(view.FetchFailures, "; ")))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagIncludeCampaign, "include-campaign-forensics", false, "Also aggregate evidence for the associated campaign")
	cmd.Flags().IntVar(&flagLimit, "limit", 100, "Maximum local events to include")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path")
	return cmd
}
