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

type campaignIDName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type campaignDetail struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	StartDate       string           `json:"startDate"`
	Actors          []campaignIDName `json:"actors"`
	Malware         []campaignIDName `json:"malware"`
	Techniques      []campaignIDName `json:"techniques"`
	Families        []campaignIDName `json:"families"`
	CampaignMembers []struct {
		ID         string `json:"id"`
		Threat     string `json:"threat"`
		Type       string `json:"type"`
		SubType    string `json:"subType"`
		ThreatTime string `json:"threatTime"`
	} `json:"campaignMembers"`
}

type campaignThreatRow struct {
	ThreatID   string `json:"threat_id"`
	Threat     string `json:"threat,omitempty"`
	Type       string `json:"type,omitempty"`
	SubType    string `json:"sub_type,omitempty"`
	ThreatTime string `json:"threat_time,omitempty"`
	Severity   *int   `json:"severity,omitempty"`
	Category   string `json:"category,omitempty"`
	Status     string `json:"status,omitempty"`
	Enriched   bool   `json:"enriched"`
}

// enrichThreatRows joins campaign members against the local threat table.
// Threats never synced locally stay un-enriched rather than failing.
func enrichThreatRows(ctx context.Context, db *sql.DB, detail campaignDetail) ([]campaignThreatRow, int) {
	rows := make([]campaignThreatRow, 0, len(detail.CampaignMembers))
	enriched := 0
	for _, member := range detail.CampaignMembers {
		row := campaignThreatRow{
			ThreatID:   member.ID,
			Threat:     member.Threat,
			Type:       member.Type,
			SubType:    member.SubType,
			ThreatTime: member.ThreatTime,
		}
		if db != nil && member.ID != "" {
			var severity sql.NullInt64
			var category, status sql.NullString
			err := db.QueryRowContext(ctx,
				`SELECT severity, category, status FROM "threat" WHERE id = ?`, member.ID).
				Scan(&severity, &category, &status)
			if err == nil {
				if severity.Valid {
					v := int(severity.Int64)
					row.Severity = &v
				}
				row.Category = category.String
				row.Status = status.String
				row.Enriched = true
				enriched++
			}
		}
		rows = append(rows, row)
	}
	return rows, enriched
}

func namesOf(pairs []campaignIDName) []string {
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		if p.Name != "" {
			out = append(out, p.Name)
		}
	}
	return out
}

type campaignThreatsView struct {
	CampaignID    string              `json:"campaign_id"`
	Name          string              `json:"name,omitempty"`
	StartDate     string              `json:"start_date,omitempty"`
	Actors        []string            `json:"actors,omitempty"`
	Malware       []string            `json:"malware,omitempty"`
	Techniques    []string            `json:"techniques,omitempty"`
	Families      []string            `json:"families,omitempty"`
	ThreatCount   int                 `json:"threat_count"`
	EnrichedCount int                 `json:"enriched_count"`
	Threats       []campaignThreatRow `json:"threats"`
	Note          string              `json:"note,omitempty"`
}

func newNovelCampaignThreatsCmd(flags *rootFlags) *cobra.Command {
	var flagDB string

	cmd := &cobra.Command{
		Use:   "campaign-threats <campaignId>",
		Short: "Expand one campaign into its member threats, enriched from the local threat store",
		Long: strings.Trim(`
Expand a campaign into the threats inside it using the unlimited
campaign-detail endpoint, then enrich each threat with severity, category,
and status from the local threat store when present. Repeated campaign
questions never touch the 50-per-day campaign-ids quota.`, "\n"),
		Example: strings.Trim(`
  proofpoint-cli campaign-threats "5e9ac342-c6c2-4c4c-a342-19d6e0ea3b4e"
  proofpoint-cli campaign-threats "5e9ac342-c6c2-4c4c-a342-19d6e0ea3b4e" --agent --select threats.threat_id,threats.severity`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "example-campaign-id"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch campaign detail and enrich member threats from the local store")
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a campaignId argument is required"))
			}
			if err := validateDataSourceStrategy(flags, "auto"); err != nil {
				return err
			}
			campaignID := strings.TrimSpace(args[0])

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			path := replacePathParam("/campaign/{campaignId}", "campaignId", campaignID)
			data, err := c.Get(cmd.Context(), path, nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var detail campaignDetail
			if err := json.Unmarshal(data, &detail); err != nil {
				return fmt.Errorf("parsing campaign detail: %w", err)
			}

			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("proofpoint-cli")
			}
			var threatDB *sql.DB
			db, dbErr := store.OpenWithContext(cmd.Context(), dbPath)
			if dbErr == nil {
				defer db.Close()
				threatDB = db.DB()
			}

			rows, enriched := enrichThreatRows(cmd.Context(), threatDB, detail)
			view := campaignThreatsView{
				CampaignID:    campaignID,
				Name:          detail.Name,
				StartDate:     detail.StartDate,
				Actors:        namesOf(detail.Actors),
				Malware:       namesOf(detail.Malware),
				Techniques:    namesOf(detail.Techniques),
				Families:      namesOf(detail.Families),
				ThreatCount:   len(rows),
				EnrichedCount: enriched,
				Threats:       rows,
			}
			if enriched == 0 && len(rows) > 0 {
				view.Note = "no local threat summaries matched; severity/category come from 'threat summary' syncs — rows still list the campaign's members"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path")
	return cmd
}
