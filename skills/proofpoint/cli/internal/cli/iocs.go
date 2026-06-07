// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature for proofpoint-cli.

// pp:data-source live
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"proofpoint-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

// iocRow is one flattened indicator extracted from forensic evidence.
type iocRow struct {
	IndicatorType string `json:"indicator_type"`
	Value         string `json:"value"`
	EvidenceType  string `json:"evidence_type"`
	Malicious     bool   `json:"malicious"`
	Platforms     string `json:"platforms,omitempty"`
	ReportID      string `json:"report_id"`
	ReportName    string `json:"report_name"`
}

type forensicsEnvelope struct {
	Generated string           `json:"generated"`
	Reports   []forensicReport `json:"reports"`
}

type forensicReport struct {
	Name      string             `json:"name"`
	Scope     string             `json:"scope"`
	Type      string             `json:"type"`
	ID        string             `json:"id"`
	Forensics []forensicEvidence `json:"forensics"`
}

type forensicEvidence struct {
	Type      string             `json:"type"`
	Display   string             `json:"display"`
	Malicious bool               `json:"malicious"`
	What      map[string]any     `json:"what"`
	Platforms []forensicPlatform `json:"platforms"`
}

type forensicPlatform struct {
	Name    string `json:"name"`
	OS      string `json:"os"`
	Version string `json:"version"`
}

func evidencePlatforms(platforms []forensicPlatform) string {
	names := make([]string, 0, len(platforms))
	for _, p := range platforms {
		if p.Name != "" {
			names = append(names, p.Name)
		}
	}
	return strings.Join(names, ";")
}

func whatString(what map[string]any, key string) string {
	v, ok := what[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func whatStrings(what map[string]any, key string) []string {
	v, ok := what[key]
	if !ok || v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// flattenForensicReports walks the nested evidence tree and emits one row per
// concrete indicator. Evidence types that carry no shareable indicator
// (behavior, ids, cookie, screenshot) are skipped.
func flattenForensicReports(reports []forensicReport, maliciousOnly bool) []iocRow {
	rows := make([]iocRow, 0)
	add := func(report forensicReport, ev forensicEvidence, indicatorType, value string) {
		if value == "" {
			return
		}
		rows = append(rows, iocRow{
			IndicatorType: indicatorType,
			Value:         value,
			EvidenceType:  ev.Type,
			Malicious:     ev.Malicious,
			Platforms:     evidencePlatforms(ev.Platforms),
			ReportID:      report.ID,
			ReportName:    report.Name,
		})
	}
	for _, report := range reports {
		for _, ev := range report.Forensics {
			if maliciousOnly && !ev.Malicious {
				continue
			}
			switch ev.Type {
			case "attachment":
				add(report, ev, "sha256", whatString(ev.What, "sha256"))
				add(report, ev, "md5", whatString(ev.What, "md5"))
			case "url":
				add(report, ev, "url", whatString(ev.What, "url"))
				add(report, ev, "ip", whatString(ev.What, "ip"))
				add(report, ev, "sha256", whatString(ev.What, "sha256"))
				add(report, ev, "md5", whatString(ev.What, "md5"))
			case "dns":
				add(report, ev, "domain", whatString(ev.What, "host"))
				for _, cname := range whatStrings(ev.What, "cnames") {
					add(report, ev, "domain", cname)
				}
				for _, ip := range whatStrings(ev.What, "ips") {
					add(report, ev, "ip", ip)
				}
			case "network":
				add(report, ev, "ip", whatString(ev.What, "ip"))
			case "file":
				add(report, ev, "file_path", whatString(ev.What, "path"))
				add(report, ev, "sha256", whatString(ev.What, "sha256"))
				add(report, ev, "md5", whatString(ev.What, "md5"))
			case "dropper":
				add(report, ev, "file_path", whatString(ev.What, "path"))
				add(report, ev, "url", whatString(ev.What, "url"))
			case "registry":
				add(report, ev, "registry_key", whatString(ev.What, "key"))
			case "process":
				add(report, ev, "process_path", whatString(ev.What, "path"))
			case "mutex":
				add(report, ev, "mutex", whatString(ev.What, "name"))
			}
		}
	}
	return rows
}

// fetchForensics calls GET /forensics for a threat or campaign.
func fetchForensics(ctx context.Context, c *client.Client, threatID, campaignID string, includeCampaign bool) (forensicsEnvelope, error) {
	params := map[string]string{}
	if threatID != "" {
		params["threatId"] = threatID
		if includeCampaign {
			params["includeCampaignForensics"] = "true"
		}
	} else {
		params["campaignId"] = campaignID
	}
	var envelope forensicsEnvelope
	data, err := c.Get(ctx, "/forensics", params)
	if err != nil {
		return envelope, err
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return envelope, fmt.Errorf("parsing forensics response: %w", err)
	}
	return envelope, nil
}

type iocsView struct {
	ThreatID   string   `json:"threat_id,omitempty"`
	CampaignID string   `json:"campaign_id,omitempty"`
	Generated  string   `json:"generated,omitempty"`
	Reports    int      `json:"reports"`
	IOCCount   int      `json:"ioc_count"`
	IOCs       []iocRow `json:"iocs"`
	Note       string   `json:"note,omitempty"`
}

func newNovelIocsCmd(flags *rootFlags) *cobra.Command {
	var flagThreatID string
	var flagCampaignID string
	var flagIncludeCampaign bool
	var flagMaliciousOnly bool

	cmd := &cobra.Command{
		Use:   "iocs",
		Short: "Flatten forensic evidence into a paste-ready indicator table: hashes, URLs, domains, IPs",
		Long: strings.Trim(`
Flatten TAP's nested forensic evidence tree into a flat indicator table:
sha256, md5, url, domain, ip, file_path, registry_key, process_path, mutex.
Output pipes straight into a blocklist import or EDR hunt.

Use this command to extract only the raw indicators for blocking or hunting.
Do NOT use this command for the full narrative incident brief; use 'incident'
instead.`, "\n"),
		Example: strings.Trim(`
  proofpoint-cli iocs --threat-id "abc123threataggregateid"
  proofpoint-cli iocs --campaign-id "5e9ac342-c6c2-4c4c-a342-19d6e0ea3b4e" --csv
  proofpoint-cli iocs --threat-id "abc123threataggregateid" --malicious-only --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "--threat-id=example-threat-id"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			// Validate input shape before the dry-run short-circuit so a
			// preview rejects exactly what a real run would reject.
			if (flagThreatID == "") == (flagCampaignID == "") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("provide exactly one of --threat-id or --campaign-id"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch forensics and flatten evidence into indicator rows")
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			envelope, err := fetchForensics(cmd.Context(), c, flagThreatID, flagCampaignID, flagIncludeCampaign)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			rows := flattenForensicReports(envelope.Reports, flagMaliciousOnly)
			view := iocsView{
				ThreatID:   flagThreatID,
				CampaignID: flagCampaignID,
				Generated:  envelope.Generated,
				Reports:    len(envelope.Reports),
				IOCCount:   len(rows),
				IOCs:       rows,
			}
			if len(rows) == 0 {
				view.Note = "no concrete indicators in the returned evidence; rerun without --malicious-only or check the id"
			}
			if flags.csv {
				data, err := json.Marshal(rows)
				if err != nil {
					return err
				}
				return printCSV(cmd.OutOrStdout(), data)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagThreatID, "threat-id", "", "Threat identifier (mutually exclusive with --campaign-id; 50 forensics lookups per 24h)")
	cmd.Flags().StringVar(&flagCampaignID, "campaign-id", "", "Campaign identifier (mutually exclusive with --threat-id)")
	cmd.Flags().BoolVar(&flagIncludeCampaign, "include-campaign-forensics", false, "With --threat-id, also aggregate evidence for the associated campaign")
	cmd.Flags().BoolVar(&flagMaliciousOnly, "malicious-only", false, "Keep only evidence Proofpoint marked malicious")
	return cmd
}
