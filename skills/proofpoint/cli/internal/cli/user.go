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

// userEvent is one local-store event involving the requested person.
type userEvent struct {
	Kind           string `json:"kind"`
	Time           string `json:"time,omitempty"`
	ThreatID       string `json:"threat_id,omitempty"`
	Classification string `json:"classification,omitempty"`
	ThreatStatus   string `json:"threat_status,omitempty"`
	Subject        string `json:"subject,omitempty"`
	Sender         string `json:"sender,omitempty"`
	URL            string `json:"url,omitempty"`
}

// userEventResourceKinds maps local resource types to human event kinds.
var userEventResourceKinds = map[string]string{
	"siem-clicks-blocked":     "click_blocked",
	"siem-clicks-permitted":   "click_permitted",
	"siem-messages-blocked":   "message_blocked",
	"siem-messages-delivered": "message_delivered",
}

// queryUserEvents scans synced SIEM events for any involving the email.
// Click events carry a scalar recipient; message events carry recipient,
// toAddresses, and ccAddresses arrays. SELECT-only.
func queryUserEvents(ctx context.Context, db *sql.DB, email string, limit int) ([]userEvent, error) {
	q := `
		SELECT resource_type, data FROM resources
		WHERE resource_type IN ('siem-clicks-blocked','siem-clicks-permitted','siem-messages-blocked','siem-messages-delivered')
		  AND (
			lower(COALESCE(json_extract(data,'$.recipient'),'')) = lower(?1)
			OR EXISTS (SELECT 1 FROM json_each(data,'$.recipient') je WHERE lower(je.value) = lower(?1))
			OR EXISTS (SELECT 1 FROM json_each(data,'$.toAddresses') jt WHERE lower(jt.value) = lower(?1))
			OR EXISTS (SELECT 1 FROM json_each(data,'$.ccAddresses') jc WHERE lower(jc.value) = lower(?1))
		  )
		ORDER BY COALESCE(json_extract(data,'$.clickTime'), json_extract(data,'$.messageTime'), '') DESC
		LIMIT ?2`
	rows, err := db.QueryContext(ctx, q, email, limit)
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
		events = append(events, decodeUserEvent(resourceType, data))
	}
	return events, rows.Err()
}

// decodeUserEvent projects a raw stored event into the timeline shape.
func decodeUserEvent(resourceType string, data []byte) userEvent {
	kind := userEventResourceKinds[resourceType]
	if kind == "" {
		kind = resourceType
	}
	ev := userEvent{Kind: kind}
	var obj struct {
		ClickTime      string `json:"clickTime"`
		MessageTime    string `json:"messageTime"`
		ThreatID       string `json:"threatID"`
		Classification string `json:"classification"`
		ThreatStatus   string `json:"threatStatus"`
		Subject        string `json:"subject"`
		Sender         string `json:"sender"`
		URL            string `json:"url"`
		ThreatsInfoMap []struct {
			ThreatID       string `json:"threatId"`
			Classification string `json:"classification"`
			ThreatStatus   string `json:"threatStatus"`
		} `json:"threatsInfoMap"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return ev
	}
	ev.Time = obj.ClickTime
	if ev.Time == "" {
		ev.Time = obj.MessageTime
	}
	ev.ThreatID = obj.ThreatID
	ev.Classification = obj.Classification
	ev.ThreatStatus = obj.ThreatStatus
	ev.Subject = obj.Subject
	ev.Sender = obj.Sender
	ev.URL = obj.URL
	if ev.ThreatID == "" && len(obj.ThreatsInfoMap) > 0 {
		ev.ThreatID = obj.ThreatsInfoMap[0].ThreatID
		if ev.Classification == "" {
			ev.Classification = obj.ThreatsInfoMap[0].Classification
		}
		if ev.ThreatStatus == "" {
			ev.ThreatStatus = obj.ThreatsInfoMap[0].ThreatStatus
		}
	}
	return ev
}

type userVapStatus struct {
	AttackIndex int      `json:"attack_index"`
	Families    []string `json:"families,omitempty"`
}

type userClickerStatus struct {
	ClickCount int      `json:"click_count"`
	Families   []string `json:"families,omitempty"`
}

type userView struct {
	Email         string             `json:"email"`
	Window        int                `json:"window"`
	Status        string             `json:"status"`
	Vap           *userVapStatus     `json:"vap"`
	TopClicker    *userClickerStatus `json:"top_clicker"`
	EventCount    int                `json:"event_count"`
	Events        []userEvent        `json:"events"`
	FetchFailures []string           `json:"fetch_failures,omitempty"`
	Note          string             `json:"note,omitempty"`
}

// userViewStatus derives the top-level result status. "partial" means at
// least one live People lookup failed (the JSON's fetch_failures carries
// detail); "ok" means every attempted source succeeded. It exists so an
// agent can distinguish a clean empty result from a degraded one without
// inspecting fetch_failures, mirroring backfill/incident's partial-failure
// contract on a read-only command that intentionally still exits 0.
func userViewStatus(v userView) string {
	if len(v.FetchFailures) > 0 {
		return "partial"
	}
	return "ok"
}

// matchIdentityEmail reports whether the identity contains the email.
func matchIdentityEmail(id peopleIdentity, email string) bool {
	for _, e := range id.Emails {
		if strings.EqualFold(e, email) {
			return true
		}
	}
	return false
}

func newNovelUserCmd(flags *rootFlags) *cobra.Command {
	var flagWindow int
	var flagLimit int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "user <email>",
		Short: "Everything known about one person: clicks, threat messages, VAP status, clicker status",
		Long: strings.Trim(`
One view of a person across every source: locally synced click and message
events (run 'backfill' or 'sync' first), plus live VAP and top-clicker status
for the window. Use this during investigations to answer "show me every event
touching this user" without re-spending SIEM quota.

Use --data-source local to skip the live People lookups and read only the
local store.`, "\n"),
		Example: strings.Trim(`
  proofpoint-cli user "jane.doe@example.com"
  proofpoint-cli user "jane.doe@example.com" --window 90 --agent
  proofpoint-cli user "jane.doe@example.com" --data-source local --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "jane.doe@example.com"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			// Validate flag shape before the dry-run short-circuit so a
			// preview rejects exactly what a real run would reject.
			if flagWindow != 14 && flagWindow != 30 && flagWindow != 90 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--window must be 14, 30, or 90 (the API accepts only these)"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would query local events and live People status for the address")
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an email address argument is required"))
			}
			if err := validateDataSourceStrategy(flags, "auto"); err != nil {
				return err
			}
			email := strings.TrimSpace(args[0])

			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("proofpoint-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "siem-clicks-permitted") {
				hintIfStale(cmd, db, "siem-clicks-permitted", flags.maxAge)
			}

			view := userView{Email: email, Window: flagWindow, FetchFailures: make([]string, 0)}
			events, err := queryUserEvents(cmd.Context(), db.DB(), email, flagLimit)
			if err != nil {
				return err
			}
			view.Events = events
			view.EventCount = len(events)

			if flags.dataSource != "local" {
				c, clientErr := flags.newClient()
				if clientErr != nil {
					view.FetchFailures = append(view.FetchFailures, fmt.Sprintf("people lookup skipped: %v", clientErr))
				} else {
					if vap, err := fetchVap(cmd.Context(), c, flagWindow, 1000); err != nil {
						view.FetchFailures = append(view.FetchFailures, fmt.Sprintf("vap: %v", err))
					} else {
						for _, u := range vap.Users {
							if matchIdentityEmail(u.Identity, email) {
								st := userVapStatus{AttackIndex: u.ThreatStatistics.AttackIndex}
								for _, fam := range u.ThreatStatistics.Families {
									st.Families = append(st.Families, fam.Name)
								}
								view.Vap = &st
								break
							}
						}
					}
					if clickers, err := fetchTopClickers(cmd.Context(), c, flagWindow, 200); err != nil {
						view.FetchFailures = append(view.FetchFailures, fmt.Sprintf("top-clickers: %v", err))
					} else {
						for _, u := range clickers.Users {
							if matchIdentityEmail(u.Identity, email) {
								st := userClickerStatus{ClickCount: u.ClickStatistics.ClickCount}
								for _, fam := range u.ClickStatistics.Families {
									st.Families = append(st.Families, fam.Name)
								}
								view.TopClicker = &st
								break
							}
						}
					}
				}
				if len(view.FetchFailures) > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of 2 live People lookups failed; local event timeline is unaffected\n", len(view.FetchFailures))
				}
			} else {
				view.Note = "people status skipped (--data-source local); event timeline served from the local store"
			}
			if view.EventCount == 0 {
				if view.Note != "" {
					view.Note += "; "
				}
				view.Note += "no local events for this address — run 'backfill --since 24h' to populate the store"
			}
			// Top-level status lets an agent distinguish a clean empty
			// result (status:ok, event_count:0, no live failures) from a
			// degraded one where live People sources were unavailable
			// (status:partial, fetch_failures populated). This is the
			// read-only analog of backfill/incident's --allow-partial-failure
			// contract: the command still exits 0 by design, but the JSON
			// no longer lets "all sources failed" masquerade as success.
			view.Status = userViewStatus(view)
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&flagWindow, "window", 30, "Days to look back for live VAP/clicker status: 14, 30, or 90")
	cmd.Flags().IntVar(&flagLimit, "limit", 100, "Maximum local events to return")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path")
	return cmd
}
