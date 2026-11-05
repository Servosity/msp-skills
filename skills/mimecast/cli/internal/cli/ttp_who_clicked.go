// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: who clicked this (possibly rewritten) link.
// pp:data-source live
// pp:client-call

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/cliutil"
)

type urlClick struct {
	Date          string `json:"date"`
	User          string `json:"user"`
	Sender        string `json:"sender,omitempty"`
	URL           string `json:"url"`
	Action        string `json:"action,omitempty"`
	ScanResult    string `json:"scan_result,omitempty"`
	Category      string `json:"category,omitempty"`
	UserOverride  string `json:"user_override,omitempty"`
	AdminOverride string `json:"admin_override,omitempty"`
	MessageID     string `json:"message_id,omitempty"`
	Subject       string `json:"subject,omitempty"`
	Definition    string `json:"ttp_definition,omitempty"`
}

type whoClickedView struct {
	Input         string     `json:"input"`
	DecodedURL    string     `json:"decoded_url"`
	MatchMode     string     `json:"match_mode"`
	Since         string     `json:"since"`
	Users         []string   `json:"users"`
	ClickedThru   int        `json:"clicked_through_warning"`
	EntryScans    int        `json:"entry_scans_skipped"`
	Clicks        []urlClick `json:"clicks"`
	ScannedClicks int        `json:"scanned_clicks"`
	MaxScanPages  int        `json:"max_scan_pages"`
	Note          string     `json:"note,omitempty"`
}

func newNovelTtpWhoClickedCmd(flags *rootFlags) *cobra.Command {
	var since, accountCode string
	var byHost bool
	var maxScanPages, limit int

	cmd := &cobra.Command{
		Use:   "who-clicked [url]",
		Short: "List who clicked a raw or Mimecast-rewritten URL, when, and whether it was blocked",
		Long: "Use this command to find which users clicked a URL (raw or Mimecast-rewritten) and whether the click was blocked. " +
			"Do NOT use it only to decode a rewritten link; use 'urls' instead.\n\n" +
			"A protect-*.mimecast.com link is decoded first. URL Protect click logs are then paged and matched client-side, because the API " +
			"has no URL filter: by default a click matches when it has the target's host and its path/query equals the target's or extends it at a / ? or & " +
			"boundary (paths are case-sensitive); --host matches every URL on that host or its subdomains. Delivery-time Entry Scan rows are skipped. " +
			"user_override shows users who clicked through a warning page. Use the message_id with 'why' to trace the original email.",
		Example:     "  mimecast-cli ttp who-clicked https://phish.example/login --since 7d --agent\n  mimecast-cli ttp who-clicked https://protect-us.mimecast.com/s/AbCdEfGh --since 3d\n  mimecast-cli ttp who-clicked phish.example --host --since 14d",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "ttp who-clicked")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one URL (or host with --host) is required"))
			}
			now := time.Now()
			start, err := parseSince(since, now)
			if err != nil {
				return usageErr(err)
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			acct := mcAccountCode(accountCode)
			input := strings.TrimSpace(args[0])
			target := input
			if isMimecastRewrite(input) {
				resp, err := mcRead(ctx, c, "/api/ttp/url/decode-url", mcBody([]any{map[string]any{"url": input}}, 0, "", acct))
				if err != nil {
					return classifyAPIError(cmd.ErrOrStderr(), err, flags)
				}
				for _, raw := range resp.Data {
					var d struct {
						URL     string `json:"url"`
						Success bool   `json:"success"`
					}
					if json.Unmarshal(raw, &d) == nil && d.Success && d.URL != "" {
						target = d.URL
					}
				}
				if target == input {
					return apiErr(fmt.Errorf("Mimecast could not decode %s", input))
				}
			}

			item := map[string]any{"from": mcTime(start), "to": mcTime(now), "route": "all", "scanResult": "all", "oldestFirst": false}
			page, err := mcReadAll(ctx, c, "/api/ttp/url/get-logs", []any{item}, acct, mcMaxPageSize, maxScanPages)
			if err != nil {
				return classifyAPIError(cmd.ErrOrStderr(), err, flags)
			}
			logs := unwrapList(page.Data, "clickLogs")
			view := whoClickedView{Input: input, DecodedURL: target, Since: mcTime(start), Users: []string{}, Clicks: []urlClick{},
				ScannedClicks: len(logs), MaxScanPages: maxScanPages, MatchMode: "url-prefix"}
			if byHost {
				view.MatchMode = "host"
			}
			users := map[string]bool{}
			for _, raw := range logs {
				var l struct {
					Date, UserEmailAddress, FromUserEmailAddress, URL, Action, ScanResult, Category, CreationMethod string
					UserOverride, AdminOverride, MessageID, Subject, TTPDefinition                                  string
				}
				if json.Unmarshal(raw, &l) != nil || !clickMatches(l.URL, target, byHost) {
					continue
				}
				// "Entry Scan" rows are delivery-time scans, not a person clicking.
				if l.CreationMethod != "" && !strings.EqualFold(l.CreationMethod, "User Click") {
					view.EntryScans++
					continue
				}
				click := urlClick{Date: l.Date, User: l.UserEmailAddress, Sender: l.FromUserEmailAddress, URL: l.URL, Action: l.Action,
					ScanResult: l.ScanResult, Category: l.Category, UserOverride: l.UserOverride, AdminOverride: l.AdminOverride,
					MessageID: l.MessageID, Subject: l.Subject, Definition: l.TTPDefinition}
				if strings.EqualFold(l.UserOverride, "continue") {
					view.ClickedThru++
				}
				if l.UserEmailAddress != "" && !users[strings.ToLower(l.UserEmailAddress)] {
					users[strings.ToLower(l.UserEmailAddress)] = true
					view.Users = append(view.Users, l.UserEmailAddress)
				}
				if limit <= 0 || len(view.Clicks) < limit {
					view.Clicks = append(view.Clicks, click)
				}
			}
			if len(view.Clicks) == 0 {
				view.Note = fmt.Sprintf("no clicks matched in %d click-log entries since %s", view.ScannedClicks, view.Since)
				if n := page.note("raise --max-scan-pages to search further"); n != "" {
					view.Note += "; " + n
				}
			} else if n := page.note("older clicks may exist"); n != "" {
				view.Note = n
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printNovelJSON(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s: %d click(s) by %d user(s), %d clicked through a warning\n", view.DecodedURL, len(view.Clicks), len(view.Users), view.ClickedThru)
			for _, k := range view.Clicks {
				fmt.Fprintf(w, "  %s  %-32s %-8s %-10s %s\n", k.Date, k.User, k.Action, k.ScanResult, k.Subject)
			}
			if view.Note != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %s\n", view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Click-log lookback (24h, 7d) or an RFC3339 start time")
	cmd.Flags().BoolVar(&byHost, "host", false, "Match every clicked URL on the same host instead of the URL prefix")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum click-log pages (100 entries each) to scan")
	cmd.Flags().IntVar(&limit, "limit", 200, "Maximum matching clicks to return (users are still counted across all matches)")
	cmd.Flags().StringVar(&accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}

// isMimecastRewrite reports whether u is a URL Protect rewritten link.
func isMimecastRewrite(u string) bool {
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	h := strings.ToLower(p.Hostname())
	return strings.HasPrefix(h, "protect") && strings.HasSuffix(h, ".mimecast.com")
}

// clickMatches compares a clicked URL with the target. Hosts compare
// case-insensitively (ignoring a leading www.); with byHost the host is
// enough. Otherwise the clicked path+query must equal the target's or extend
// it at a path or query boundary, case-sensitively.
func clickMatches(clicked, target string, byHost bool) bool {
	c, t := parseLoose(clicked), parseLoose(target)
	if c == nil || t == nil || c.host == "" {
		return false
	}
	if byHost {
		return c.host == t.host || strings.HasSuffix(c.host, "."+t.host)
	}
	if c.host != t.host {
		return false
	}
	if t.rest == "" || t.rest == "/" {
		return true
	}
	if strings.TrimSuffix(c.rest, "/") == strings.TrimSuffix(t.rest, "/") {
		return true
	}
	if !strings.HasPrefix(c.rest, t.rest) {
		return false
	}
	next := c.rest[len(t.rest)]
	return next == '/' || next == '?' || next == '#' || next == '&' || strings.HasSuffix(t.rest, "/")
}

type looseURL struct{ host, rest string }

func parseLoose(s string) *looseURL {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	p, err := url.Parse(s)
	if err != nil {
		return nil
	}
	rest := p.EscapedPath()
	if p.RawQuery != "" {
		rest += "?" + p.RawQuery
	}
	return &looseURL{host: strings.TrimPrefix(strings.ToLower(p.Hostname()), "www."), rest: rest}
}
