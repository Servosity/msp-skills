// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-duo-pp-cli/internal/client"
	"cisco-duo-pp-cli/internal/cliutil"
)

type authLogEvent struct {
	TxID         string `json:"txid"`
	ISOTimestamp string `json:"isotimestamp"`
	Timestamp    int64  `json:"timestamp"`
	Result       string `json:"result"`
	Reason       string `json:"reason"`
	Factor       string `json:"factor"`
	EventType    string `json:"event_type"`
	User         struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"user"`
	Application struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"application"`
	AccessDevice struct {
		IP       string `json:"ip"`
		Location struct {
			Country string `json:"country"`
		} `json:"location"`
	} `json:"access_device"`
}

// fetchAuthLogs pages GET /admin/v2/logs/authentication over [from, to]
// using the documented "<ms>,<txid>" next_offset cursor.
func fetchAuthLogs(ctx context.Context, c *client.Client, filters url.Values, from, to time.Time, maxPages int) (events []authLogEvent, pages int, capHit bool, err error) {
	events = make([]authLogEvent, 0)
	q := url.Values{}
	for k, v := range filters {
		q[k] = append([]string(nil), v...)
	}
	q.Set("mintime", strconv.FormatInt(from.UnixMilli(), 10))
	q.Set("maxtime", strconv.FormatInt(to.UnixMilli(), 10))
	q.Set("limit", "1000")
	for pages < maxPages {
		raw, _, gerr := duoGetValues(ctx, c, "/admin/v2/logs/authentication", q)
		if gerr != nil {
			return events, pages, false, gerr
		}
		pages++
		var page struct {
			AuthLogs []authLogEvent `json:"authlogs"`
			Metadata struct {
				NextOffset []string `json:"next_offset"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return events, pages, false, fmt.Errorf("parsing auth log page: %w", err)
		}
		events = append(events, page.AuthLogs...)
		if len(page.Metadata.NextOffset) == 0 || len(page.AuthLogs) == 0 {
			return events, pages, false, nil
		}
		q.Set("next_offset", strings.Join(page.Metadata.NextOffset, ","))
	}
	return events, pages, true, nil
}

type failureRow struct {
	User        string   `json:"user"`
	UserID      string   `json:"user_id"`
	Reason      string   `json:"reason"`
	Factor      string   `json:"factor"`
	Application string   `json:"application"`
	Result      string   `json:"result"`
	Count       int      `json:"count"`
	FirstSeen   string   `json:"first_seen"`
	LastSeen    string   `json:"last_seen"`
	IPs         []string `json:"ips"`
	Countries   []string `json:"countries"`
	Flag        string   `json:"flag,omitempty"`
}

type failuresView struct {
	AccountID      string       `json:"account_id,omitempty"`
	Since          string       `json:"since"`
	EventsScanned  int          `json:"events_scanned"`
	FailureEvents  int          `json:"failure_events"`
	Pages          int          `json:"pages"`
	ScanCapHit     bool         `json:"scan_cap_hit"`
	MaxScanPages   int          `json:"max_scan_pages"`
	Rows           []failureRow `json:"rows"`
	Note           string       `json:"note,omitempty"`
	UserFilterUsed string       `json:"user_filter,omitempty"`
}

// flaggedReasons are Duo auth-log reasons worth an analyst's attention.
var flaggedReasons = map[string]string{
	"locked_out":           "user is locked out",
	"user_marked_fraud":    "user reported fraud",
	"frequent_attempts":    "too many attempts",
	"deny_unenrolled_user": "unenrolled user denied",
}

// rollupFailures groups non-success events by user x reason x factor x app.
func rollupFailures(events []authLogEvent, top int) ([]failureRow, int) {
	type key struct{ user, reason, factor, app, result string }
	type acc struct {
		row       failureRow
		ips, ctry map[string]bool
	}
	agg := map[key]*acc{}
	failures := 0
	for _, e := range events {
		if strings.EqualFold(e.Result, "success") {
			continue
		}
		failures++
		k := key{e.User.Name, e.Reason, e.Factor, e.Application.Name, e.Result}
		a := agg[k]
		if a == nil {
			a = &acc{row: failureRow{User: e.User.Name, UserID: e.User.Key, Reason: e.Reason, Factor: e.Factor,
				Application: e.Application.Name, Result: e.Result, FirstSeen: e.ISOTimestamp, LastSeen: e.ISOTimestamp,
				Flag: flaggedReasons[e.Reason]}, ips: map[string]bool{}, ctry: map[string]bool{}}
			agg[k] = a
		}
		a.row.Count++
		if e.ISOTimestamp != "" && (a.row.FirstSeen == "" || e.ISOTimestamp < a.row.FirstSeen) {
			a.row.FirstSeen = e.ISOTimestamp
		}
		if e.ISOTimestamp > a.row.LastSeen {
			a.row.LastSeen = e.ISOTimestamp
		}
		if e.AccessDevice.IP != "" {
			a.ips[e.AccessDevice.IP] = true
		}
		if e.AccessDevice.Location.Country != "" {
			a.ctry[e.AccessDevice.Location.Country] = true
		}
	}
	rows := make([]failureRow, 0, len(agg))
	for _, a := range agg {
		a.row.IPs = sortedKeys(a.ips)
		a.row.Countries = sortedKeys(a.ctry)
		rows = append(rows, a.row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		if rows[i].User != rows[j].User {
			return rows[i].User < rows[j].User
		}
		return rows[i].Reason < rows[j].Reason
	})
	if top > 0 && len(rows) > top {
		rows = rows[:top]
	}
	return rows, failures
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func newNovelFailuresCmd(flags *rootFlags) *cobra.Command {
	var since, user string
	var top, maxScanPages int
	cmd := &cobra.Command{
		Use:   "failures",
		Short: "Roll up failed and fraud authentications per user, reason, factor and app",
		Long: strings.Trim(`
Use this command to roll up failed and fraud authentications per user, reason,
factor and application over a time window. It pages the v2 authentication log
(13-digit millisecond window, '<ms>,<txid>' cursor) and groups every
non-success event, with first/last seen, source IPs and countries, and flags
reasons such as locked_out and user_marked_fraud.

Do NOT use it to diagnose and fix a single user's lockout end to end; use
'lockout triage' instead.

Duo recommends polling logs at most once per minute; requires the Admin API
"Grant read log" permission.`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli failures --since 7d --top 20 --agent
  cisco-duo-cli failures --since 24h --user jdoe
  cisco-duo-cli failures --since 7d --account-id DA9VZOC5X63I2W72NRP9 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "--since=24h"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "failures")
			}
			window, err := duoParseSince(since, 24*time.Hour)
			if err != nil {
				_ = cmd.Usage()
				return err
			}
			if window > 180*24*time.Hour {
				return usageErr(fmt.Errorf("--since cannot exceed 180d (Duo log retention for the v2 API)"))
			}
			if maxScanPages < 1 {
				return usageErr(fmt.Errorf("--max-scan-pages must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			filters := url.Values{}
			view := failuresView{AccountID: currentAccountID(), Since: window.String(), MaxScanPages: maxScanPages, Rows: make([]failureRow, 0)}
			if strings.TrimSpace(user) != "" {
				u, err := duoFindUser(ctx, c, user)
				if err != nil {
					return err
				}
				filters.Set("users", u.UserID)
				view.UserFilterUsed = u.Username
			}
			now := time.Now()
			events, pages, capHit, err := fetchAuthLogs(ctx, c, filters, now.Add(-window), now, maxScanPages)
			if err != nil {
				return err
			}
			view.Rows, view.FailureEvents = rollupFailures(events, top)
			view.EventsScanned, view.Pages, view.ScanCapHit = len(events), pages, capHit
			if capHit {
				view.Note = fmt.Sprintf("stopped after %d page(s); counts cover only the newest events. Raise --max-scan-pages or narrow --since.", pages)
			} else if view.FailureEvents == 0 {
				view.Note = "no failed or fraud authentications in this window"
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printMarkedRowsJSON(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d failure events out of %d scanned (%s window)\n", view.FailureEvents, view.EventsScanned, view.Since)
			for _, r := range view.Rows {
				fmt.Fprintf(w, "%5d  %-24s %-28s %-18s %s  %s\n", r.Count, r.User, r.Reason, r.Factor, r.Application, r.Flag)
			}
			if view.Note != "" {
				fmt.Fprintln(w, view.Note)
			}
			return nil
		},
	}
	useAccountFlag(cmd)
	cmd.Flags().StringVar(&since, "since", "24h", "Window to scan, e.g. 24h, 7d (max 180d)")
	cmd.Flags().StringVar(&user, "user", "", "Only this user (username, email or user_id)")
	cmd.Flags().IntVar(&top, "top", 20, "Return at most this many rows, highest count first (0 = all)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum log pages (1000 events each) to scan")
	return cmd
}
