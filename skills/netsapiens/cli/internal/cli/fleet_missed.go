// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"netsapiens-pp-cli/internal/cliutil"
	"netsapiens-pp-cli/internal/store"
)

type missedGroup struct {
	Domain            string `json:"domain"`
	User              string `json:"user"`
	UserName          string `json:"user_name,omitempty"`
	Missed            int    `json:"missed"`
	RegisteredDevices *int   `json:"registered_devices,omitempty"` // nil when users/devices are not synced
	LastMissed        string `json:"last_missed,omitempty"`
}

type missedReport struct {
	WindowStart string        `json:"window_start"`
	WindowEnd   string        `json:"window_end"`
	ScannedCDRs int           `json:"scanned_cdrs"`
	PagesRead   int           `json:"pages_read"`
	Truncated   bool          `json:"truncated"`
	Groups      []missedGroup `json:"groups"`
	Note        string        `json:"note,omitempty"`
	// Complete is false when the CDR scan stopped at --max-scan-pages, or a
	// joined mirror table (or the parent it was walked over) is stale.
	Complete bool     `json:"complete"`
	Stale    []string `json:"stale_resources,omitempty"`
}

// groupMissed de-duplicates CDR legs by originating call id (one call can
// write several legs) and counts unanswered inbound calls per target user.
func groupMissed(cdrs []fleetRow, users map[string]fleetRow, reg regMap, haveUsers, haveDevices bool, scope string) []missedGroup {
	seen := map[string]bool{}
	groups := map[string]*missedGroup{}
	for _, c := range cdrs {
		if c.fstr("call-answer-datetime") != "" {
			continue // answered leg; type=Missed already filters upstream, this guards servers that ignore it
		}
		dom := c.fstr("call-term-domain", "domain")
		if dom == "" {
			dom = scope // a --domain request knows the tenant even when the record omits it
		}
		user := c.fstr("call-term-user", "call-orig-to-user")
		k := key(dom, user)
		// One call writes several legs per target; a ring group can miss several users
		// with one call id, so legs dedupe per target, not globally.
		if id := c.fstr("call-orig-call-id", "id"); id != "" {
			if seen[k+"\x00"+id] {
				continue
			}
			seen[k+"\x00"+id] = true
		}
		g := groups[k]
		if g == nil {
			g = &missedGroup{Domain: dom, User: user}
			if haveUsers {
				g.UserName = fullName(users[k])
			}
			// only a completed devices sync with every state known can say "zero registered phones"
			if ri := reg.forUser(dom, user); haveDevices && (ri.registered > 0 || ri.knownZero()) {
				n := ri.registered
				g.RegisteredDevices = &n
			}
			groups[k] = g
		}
		g.Missed++
		if t := c.fstr("call-start-datetime", "call-batch-start-datetime"); t > g.LastMissed {
			g.LastMissed = t
		}
	}
	out := make([]missedGroup, 0, len(groups))
	for _, k := range sortedKeys(groups) {
		out = append(out, *groups[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Missed > out[j].Missed })
	return out
}

func newNovelFleetMissedCmd(flags *rootFlags) *cobra.Command {
	var window, domain, dbPath string
	var maxScanPages, pageSize int
	cmd := &cobra.Command{
		Use:   "missed",
		Short: "Unanswered inbound calls per user and client over a window, with each user's registered phones",
		Long: `Use this command for unanswered direct inbound calls per user from CDRs.
Do NOT use this command for call-queue abandonment or service level; use 'fleet queues' instead.

Reads CDRs live (type=Missed) for the window, de-duplicates legs by originating call id, and
groups by the target user. When users and devices are synced locally, each row also shows the
user's name and registered-device count, which separates "nobody's phone was on" from "nobody answered".`,
		Example: strings.Trim(`
  netsapiens-cli fleet missed --window 7d --agent
  netsapiens-cli fleet missed --window 24h --domain acme.example`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet missed")
			}
			span, err := cliutil.ParseDurationLoose(window)
			if err != nil || span <= 0 {
				return usageErr(fmt.Errorf("--window must be a positive duration like 7d or 24h, got %q", window))
			}
			if maxScanPages < 1 {
				return usageErr(fmt.Errorf("--max-scan-pages must be at least 1, got %d", maxScanPages))
			}
			if pageSize < 1 || pageSize > 1000 {
				return usageErr(fmt.Errorf("--page-size must be 1..1000 (the API maximum), got %d", pageSize))
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
			end := time.Now().UTC().Truncate(time.Second)
			rep := missedReport{WindowStart: end.Add(-span).Format(time.RFC3339), WindowEnd: end.Format(time.RFC3339), Groups: []missedGroup{}}
			path := "/cdrs"
			if domain != "" {
				path = "/domains/" + url.PathEscape(domain) + "/cdrs"
			}
			var cdrs []fleetRow
			for page := 0; page < maxScanPages; page++ {
				data, err := c.Get(ctx, path, map[string]string{
					"datetime-start": rep.WindowStart, "datetime-end": rep.WindowEnd, "type": "Missed",
					"limit": strconv.Itoa(pageSize), "start": strconv.Itoa(page * pageSize),
				})
				if err != nil {
					return classifyAPIError(cmd.ErrOrStderr(), fmt.Errorf("reading CDRs page %d: %w", page+1, err), flags)
				}
				dec := json.NewDecoder(strings.NewReader(string(data)))
				dec.UseNumber()
				var items []fleetRow
				if err := dec.Decode(&items); err != nil {
					return fmt.Errorf("CDR response is not a JSON array: %w", err)
				}
				rep.PagesRead++
				rep.ScannedCDRs += len(items)
				cdrs = append(cdrs, items...)
				if len(items) < pageSize {
					break
				}
				if page == maxScanPages-1 {
					rep.Truncated = true
				}
			}
			users, reg, haveUsers, haveDevices, stale := missedLocalJoin(ctx, cmd, flags, dbPath)
			rep.Stale = stale
			rep.Groups = groupMissed(cdrs, users, reg, haveUsers, haveDevices, domain)
			if rep.Truncated {
				rep.Note = fmt.Sprintf("stopped after %d pages of %d CDRs; raise --max-scan-pages or narrow --window for complete counts", rep.PagesRead, pageSize)
			} else if !haveUsers || !haveDevices {
				rep.Note = "users or devices are not synced, so names or registered-phone counts are omitted; run: " + fleetSyncHint
			}
			// netsapiens hand-fix (netsapiens-audits-judge-every-dependency): mirror-freshness;
			// a stale join's names and registered-phone counts are not complete.
			if len(rep.Stale) > 0 {
				stale := "names and registered-phone counts come from a stale mirror (" + strings.Join(rep.Stale, ", ") + "); run: " + fleetSyncHint
				rep.Note = strings.TrimPrefix(rep.Note+"; "+stale, "; ")
			}
			rep.Complete = !rep.Truncated && len(rep.Stale) == 0
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				ac := auditCoverage{Complete: rep.Complete, Stale: rep.Stale, remedy: "raise --max-scan-pages or narrow --window"}
				if rep.Truncated {
					ac.Missing = []string{"CDR scan truncated at --max-scan-pages"}
				}
				if len(rep.Stale) > 0 && !rep.Truncated {
					ac.remedy = ""
				}
				// keep: --agent/--compact drop keys present on few rows.
				return printFleetAudit(cmd, flags, ac, rep, rep.Groups, "user_name", "registered_devices", "last_missed")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Missed calls %s to %s (%d CDR legs scanned)\n", rep.WindowStart, rep.WindowEnd, rep.ScannedCDRs)
			if len(rep.Groups) == 0 {
				if rep.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), "Note: "+rep.Note)
				}
				if rep.Truncated {
					fmt.Fprintln(cmd.OutOrStdout(), "No missed calls in the pages read (the scan was truncated; see the note).")
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "No missed calls in the window.")
				}
				return nil
			}
			table := make([]map[string]any, 0, len(rep.Groups))
			for _, g := range rep.Groups {
				regText := "-"
				if g.RegisteredDevices != nil {
					regText = strconv.Itoa(*g.RegisteredDevices)
				}
				table = append(table, map[string]any{"domain": g.Domain, "user": strings.TrimSpace(g.User + " " + g.UserName), "missed": g.Missed, "registered_phones": regText, "last": g.LastMissed})
			}
			if rep.Note != "" {
				defer fmt.Fprintln(cmd.OutOrStdout(), "Note: "+rep.Note)
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&window, "window", "7d", "How far back to look (e.g. 7d, 24h, 2w)")
	cmd.Flags().StringVar(&domain, "domain", "", "Only this client domain (default: every domain the key can see)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum CDR pages to read before stopping")
	cmd.Flags().IntVar(&pageSize, "page-size", 1000, "CDRs per page (API maximum 1000)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror used for names and registered phones (optional)")
	return cmd
}

// missedLocalJoin loads users and device registration from the mirror when it
// exists; the command still works without one. stale names every joined
// table (or its parent) older than --max-age or walked behind its parent.
func missedLocalJoin(ctx context.Context, cmd *cobra.Command, flags *rootFlags, dbPath string) (users map[string]fleetRow, reg regMap, haveUsers, haveDevices bool, stale []string) {
	path := fleetDBPath(dbPath)
	if _, err := os.Stat(path); err != nil {
		return nil, nil, false, false, nil
	}
	db, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, nil, false, false, nil
	}
	defer db.Close()
	cov := fleetCov{}
	urows, uerr := loadCovered(ctx, db, cov, rtUsers)
	drows, derr := loadCovered(ctx, db, cov, rtDevices)
	haveUsers, haveDevices = uerr == nil && cov[rtUsers], derr == nil && cov[rtDevices]
	if haveUsers {
		users = indexBy(urows, "user")
	}
	if haveDevices {
		reg = registeredDevices(drows)
	}
	return users, reg, haveUsers, haveDevices, fleetStale(cmd, flags, db, cov.tables()...)
}
