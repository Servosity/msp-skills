// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-duo-pp-cli/internal/cliutil"
)

type chaseTarget struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Created  string `json:"created"`
	Status   string `json:"status"` // planned | sent | failed
	Error    string `json:"error,omitempty"`
}

// selectChaseTargets keeps active, unenrolled users older than minAge who
// have an email, are not Duo Directory users (has_password) and are not held
// by a Disabled group (an enrollment link is a way through MFA), oldest first.
func selectChaseTargets(users []duoUser, minAge time.Duration, limit int, now time.Time) []chaseTarget {
	picked := make([]duoUser, 0)
	for _, u := range users {
		if u.IsEnrolled || !strings.EqualFold(u.Status, "active") || strings.TrimSpace(u.Email) == "" || u.HasPassword || disabledGroup(u) != nil {
			continue
		}
		if u.Created == 0 || now.Sub(time.Unix(u.Created, 0)) < minAge {
			continue
		}
		picked = append(picked, u)
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].Created < picked[j].Created })
	if limit > 0 && len(picked) > limit {
		picked = picked[:limit]
	}
	out := make([]chaseTarget, 0, len(picked))
	for _, u := range picked {
		out = append(out, chaseTarget{UserID: u.UserID, Username: u.Username, Email: u.Email, Created: unixTime(u.Created), Status: "planned"})
	}
	return out
}

func newNovelEnrollmentChaseCmd(flags *rootFlags) *cobra.Command {
	var olderThan string
	var limit, validDays, maxScanPages int
	var apply bool
	cmd := &cobra.Command{
		Use:   "chase",
		Short: "Re-send Duo enrollment emails to users still unenrolled after a given age",
		Long: strings.Trim(`
Use this command to re-send Duo enrollment emails to users still unenrolled
after a given age. It pages the live user list (so anyone who already enrolled
is never emailed), keeps active, unenrolled users older than --older-than who
have an email address, are not Duo Directory users and are not in a Disabled
group (an enrollment link would be a way through that hold), and sends at most
--limit enrollment emails, oldest first. Without --apply it only lists the
recipients.

Do NOT use it for a read-only stale-account report; use 'audit stale' instead.`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli enrollment chase --older-than 72h --limit 25
  cisco-duo-cli enrollment chase --older-than 7d --limit 10 --apply --agent`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:preview-happy-path": "true", "pp:happy-args": "--older-than=72h;--limit=5"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "enrollment chase")
			}
			age, err := duoParseSince(olderThan, 72*time.Hour)
			if err != nil {
				return err
			}
			if limit < 1 || limit > 500 {
				return usageErr(fmt.Errorf("--limit must be between 1 and 500"))
			}
			if validDays < 0 || validDays > 30 {
				return usageErr(fmt.Errorf("--valid-days must be between 0 and 30"))
			}
			if maxScanPages < 1 {
				return usageErr(fmt.Errorf("--max-scan-pages must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() {
				maxScanPages = 1
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			raw, pages, capHit, err := duoPageAll(ctx, c, "", "/admin/v1/users", 300, maxScanPages)
			if err != nil {
				return err
			}
			targets := selectChaseTargets(decodeAll[duoUser](raw), age, limit, time.Now())
			view := struct {
				AccountID    string        `json:"account_id,omitempty"`
				Applied      bool          `json:"applied"`
				UsersScanned int           `json:"users_scanned"`
				Pages        int           `json:"pages"`
				ScanCapHit   bool          `json:"scan_cap_hit"`
				Targets      []chaseTarget `json:"targets"`
				Sent         int           `json:"sent"`
				Failed       int           `json:"failed"`
				Note         string        `json:"note,omitempty"`
			}{AccountID: currentAccountID(), UsersScanned: len(raw), Pages: pages, ScanCapHit: capHit, Targets: targets}
			var notes []string
			if capHit {
				notes = append(notes, "user scan stopped at --max-scan-pages; older unenrolled users beyond it were not considered")
			}
			if apply {
				view.Applied = true
				for i := range targets {
					body := map[string]any{"username": targets[i].Username, "email": targets[i].Email}
					if validDays > 0 {
						body["valid_secs"] = strconv.Itoa(validDays * 86400)
					}
					if _, err := duoPost(ctx, c, "/admin/v1/users/enroll", body); err != nil {
						targets[i].Status, targets[i].Error = "failed", firstLine(err)
						view.Failed++
						continue
					}
					targets[i].Status = "sent"
					view.Sent++
				}
			} else if len(targets) > 0 {
				notes = append(notes, "preview only, re-run with --apply to send")
			}
			view.Note = strings.Join(notes, "; ")
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printChaseJSON(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
			} else {
				w := cmd.OutOrStdout()
				if len(targets) == 0 {
					fmt.Fprintf(w, "No active unenrolled users older than %s among %d scanned.\n", age, len(raw))
				}
				for _, t := range targets {
					fmt.Fprintf(w, "[%s] %-24s %-32s created %s %s\n", t.Status, t.Username, t.Email, t.Created, t.Error)
				}
				if view.Note != "" {
					fmt.Fprintln(w, view.Note)
				}
			}
			if view.Failed > 0 {
				return apiErr(fmt.Errorf("%d of %d enrollment emails failed", view.Failed, len(targets)))
			}
			return nil
		},
	}
	useAccountFlag(cmd)
	cmd.Flags().StringVar(&olderThan, "older-than", "72h", "Only users created at least this long ago, e.g. 72h, 7d")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum enrollment emails to send (1-500)")
	cmd.Flags().IntVar(&validDays, "valid-days", 0, "Enrollment link lifetime in days (0 = account default, max 30)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum pages of users (300 each) to scan")
	cmd.Flags().BoolVar(&apply, "apply", false, "Send the emails (default is a read-only list of recipients)")
	return cmd
}

// printChaseJSON keeps a failed target's error under --agent/--compact (see
// receiptKeepKeys).
func printChaseJSON(w io.Writer, view any, flags *rootFlags) error {
	return printJSONFilteredKeep(w, view, flags, receiptKeepKeys...)
}
