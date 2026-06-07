// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature for proofpoint-cli.

// pp:data-source live
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"proofpoint-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

type peopleIdentity struct {
	GUID           string   `json:"guid"`
	CustomerUserID string   `json:"customerUserId"`
	Emails         []string `json:"emails"`
	Name           string   `json:"name"`
	Department     string   `json:"department"`
	Location       string   `json:"location"`
	Title          string   `json:"title"`
	VIP            bool     `json:"vip"`
}

type vapAPIUser struct {
	Identity         peopleIdentity `json:"identity"`
	ThreatStatistics struct {
		AttackIndex int `json:"attackIndex"`
		Families    []struct {
			Name  string `json:"name"`
			Score int    `json:"score"`
		} `json:"families"`
	} `json:"threatStatistics"`
}

type clickerAPIUser struct {
	Identity        peopleIdentity `json:"identity"`
	ClickStatistics struct {
		ClickCount int `json:"clickCount"`
		Families   []struct {
			Name   string `json:"name"`
			Clicks int    `json:"clicks"`
		} `json:"families"`
	} `json:"clickStatistics"`
}

type vapResponse struct {
	Users                   []vapAPIUser `json:"users"`
	TotalVapUsers           int          `json:"totalVapUsers"`
	Interval                string       `json:"interval"`
	AverageAttackIndex      int          `json:"averageAttackIndex"`
	VapAttackIndexThreshold int          `json:"vapAttackIndexThreshold"`
}

type clickersResponse struct {
	Users            []clickerAPIUser `json:"users"`
	TotalTopClickers int              `json:"totalTopClickers"`
	Interval         string           `json:"interval"`
}

// peopleJoinKey returns the identity key used to match a person across the
// VAP and top-clicker lists: GUID when present, else the first email
// lowercased.
func peopleJoinKey(id peopleIdentity) string {
	if id.GUID != "" {
		return "guid:" + id.GUID
	}
	if len(id.Emails) > 0 {
		return "email:" + strings.ToLower(id.Emails[0])
	}
	return ""
}

type overlapRow struct {
	Name           string   `json:"name,omitempty"`
	Emails         []string `json:"emails"`
	Department     string   `json:"department,omitempty"`
	Title          string   `json:"title,omitempty"`
	VIP            bool     `json:"vip"`
	AttackIndex    int      `json:"attack_index"`
	ClickCount     int      `json:"click_count"`
	ThreatFamilies []string `json:"threat_families,omitempty"`
	ClickFamilies  []string `json:"click_families,omitempty"`
}

// joinRiskOverlap intersects VAP users with top clickers on identity.
func joinRiskOverlap(vaps []vapAPIUser, clickers []clickerAPIUser) []overlapRow {
	byKey := make(map[string]vapAPIUser, len(vaps))
	for _, u := range vaps {
		if key := peopleJoinKey(u.Identity); key != "" {
			byKey[key] = u
		}
	}
	rows := make([]overlapRow, 0)
	for _, clicker := range clickers {
		key := peopleJoinKey(clicker.Identity)
		if key == "" {
			continue
		}
		vap, ok := byKey[key]
		if !ok {
			continue
		}
		row := overlapRow{
			Name:        vap.Identity.Name,
			Emails:      vap.Identity.Emails,
			Department:  vap.Identity.Department,
			Title:       vap.Identity.Title,
			VIP:         vap.Identity.VIP,
			AttackIndex: vap.ThreatStatistics.AttackIndex,
			ClickCount:  clicker.ClickStatistics.ClickCount,
		}
		for _, fam := range vap.ThreatStatistics.Families {
			row.ThreatFamilies = append(row.ThreatFamilies, fam.Name)
		}
		for _, fam := range clicker.ClickStatistics.Families {
			row.ClickFamilies = append(row.ClickFamilies, fam.Name)
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].AttackIndex != rows[j].AttackIndex {
			return rows[i].AttackIndex > rows[j].AttackIndex
		}
		return rows[i].ClickCount > rows[j].ClickCount
	})
	return rows
}

func fetchVap(ctx context.Context, c *client.Client, window int, size int) (vapResponse, error) {
	var resp vapResponse
	data, err := c.Get(ctx, "/people/vap", map[string]string{
		"window": fmt.Sprintf("%d", window),
		"size":   fmt.Sprintf("%d", size),
	})
	if err != nil {
		return resp, err
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return resp, fmt.Errorf("parsing vap response: %w", err)
	}
	return resp, nil
}

func fetchTopClickers(ctx context.Context, c *client.Client, window int, size int) (clickersResponse, error) {
	var resp clickersResponse
	data, err := c.Get(ctx, "/people/top-clickers", map[string]string{
		"window": fmt.Sprintf("%d", window),
		"size":   fmt.Sprintf("%d", size),
	})
	if err != nil {
		return resp, err
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return resp, fmt.Errorf("parsing top-clickers response: %w", err)
	}
	return resp, nil
}

type riskOverlapView struct {
	Window        int          `json:"window"`
	VapTotal      int          `json:"vap_total"`
	ClickersTotal int          `json:"clickers_total"`
	OverlapCount  int          `json:"overlap_count"`
	Users         []overlapRow `json:"users"`
	Note          string       `json:"note,omitempty"`
}

func newNovelRiskOverlapCmd(flags *rootFlags) *cobra.Command {
	var flagWindow int
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "risk-overlap",
		Short: "People who are both Very Attacked AND top clickers — attack index beside click count",
		Long: strings.Trim(`
Intersect the Very Attacked People list with the top-clickers list for the
same window. The result is the highest-risk set: people drawing heavy
targeting who also click. No single TAP endpoint can answer this; the CLI
joins the two People feeds on identity.`, "\n"),
		Example: strings.Trim(`
  proofpoint-cli risk-overlap --window 30
  proofpoint-cli risk-overlap --window 90 --csv
  proofpoint-cli risk-overlap --window 14 --agent --select users.emails,users.attack_index,users.click_count`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "--window=30"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			// Validate input shape before the dry-run short-circuit so a
			// preview rejects exactly what a real run would reject.
			if flagWindow != 14 && flagWindow != 30 && flagWindow != 90 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--window must be 14, 30, or 90 (the API accepts only these)"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch VAP and top-clicker lists and intersect them on identity")
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			vap, err := fetchVap(cmd.Context(), c, flagWindow, 1000)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			clickers, err := fetchTopClickers(cmd.Context(), c, flagWindow, 200)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			rows := joinRiskOverlap(vap.Users, clickers.Users)
			view := riskOverlapView{
				Window:        flagWindow,
				VapTotal:      vap.TotalVapUsers,
				ClickersTotal: clickers.TotalTopClickers,
				OverlapCount:  len(rows),
				Users:         rows,
			}
			if flagLimit > 0 && len(view.Users) > flagLimit {
				view.Users = view.Users[:flagLimit]
			}
			if len(rows) == 0 {
				view.Note = fmt.Sprintf("no overlap between %d VAPs and %d top clickers in the %d-day window — that is a finding, not an error", vap.TotalVapUsers, clickers.TotalTopClickers, flagWindow)
			}
			if flags.csv {
				data, err := json.Marshal(view.Users)
				if err != nil {
					return err
				}
				return printCSV(cmd.OutOrStdout(), data)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&flagWindow, "window", 30, "Days to look back: 14, 30, or 90")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum overlap rows to return (0 = all)")
	return cmd
}
