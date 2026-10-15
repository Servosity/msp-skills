// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type staleFinding struct {
	Check  string `json:"check"`
	Kind   string `json:"kind"` // user | admin | phone
	ID     string `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

var staleChecks = []string{"unenrolled", "inactive", "locked", "pending-deletion", "admin-inactive", "admin-expired", "phone-unseen"}

type duoAdmin struct {
	AdminID   string `json:"admin_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	Created   *int64 `json:"created"`
	LastLogin *int64 `json:"last_login"`
}

// staleFindingsFor runs the enabled checks over synced users, admins and phones.
func staleFindingsFor(users []duoUser, admins []duoAdmin, phones []duoPhone, days int, enabled map[string]bool, now time.Time) []staleFinding {
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	ds := strconv.Itoa(days)
	out := make([]staleFinding, 0)
	add := func(check, kind, id, name, detail string) {
		if enabled[check] {
			out = append(out, staleFinding{Check: check, Kind: kind, ID: id, Name: name, Detail: detail})
		}
	}
	for _, u := range users {
		status := strings.ToLower(u.Status)
		switch status {
		case "locked out":
			add("locked", "user", u.UserID, u.Username, "locked out ("+orDash(u.lockoutReason())+")")
		case "pending deletion":
			add("pending-deletion", "user", u.UserID, u.Username, "in the Trash pending permanent deletion")
		}
		if !u.IsEnrolled && status == "active" && u.Created > 0 && time.Unix(u.Created, 0).Before(cutoff) {
			add("unenrolled", "user", u.UserID, u.Username, "not enrolled "+ds+"+ days after creation (heading for a 'Not enrolled' lockout)")
		}
		if status == "active" || status == "bypass" {
			switch {
			case u.LastLogin == nil && u.Created > 0 && time.Unix(u.Created, 0).Before(cutoff):
				add("inactive", "user", u.UserID, u.Username, "never logged in; created "+unixTime(u.Created))
			case u.LastLogin != nil && time.Unix(*u.LastLogin, 0).Before(cutoff):
				add("inactive", "user", u.UserID, u.Username, "last login "+unixPtrTime(u.LastLogin))
			}
		}
	}
	for _, a := range admins {
		switch strings.ToLower(a.Status) {
		case "expired":
			add("admin-expired", "admin", a.AdminID, a.Email, "admin expired for inactivity")
		case "pending activation":
			add("admin-expired", "admin", a.AdminID, a.Email, "admin never activated")
		}
		switch {
		case a.LastLogin != nil && time.Unix(*a.LastLogin, 0).Before(cutoff):
			add("admin-inactive", "admin", a.AdminID, a.Email, "last login "+unixPtrTime(a.LastLogin))
		case a.LastLogin == nil && a.Created != nil && *a.Created > 0 && time.Unix(*a.Created, 0).Before(cutoff):
			add("admin-inactive", "admin", a.AdminID, a.Email, "never logged in; created "+unixPtrTime(a.Created))
		}
	}
	for _, p := range phones {
		if !p.Activated {
			continue
		}
		if seen, ok := duoParseTime(string(p.LastSeen)); ok && seen.Before(cutoff) {
			add("phone-unseen", "phone", p.PhoneID, p.Number, "Duo Mobile last seen "+seen.Format(time.RFC3339))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Check != out[j].Check {
			return out[i].Check < out[j].Check
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func parseChecks(s string) (map[string]bool, error) {
	parts := strings.Split(s, ",")
	if strings.TrimSpace(s) == "" {
		parts = staleChecks
	}
	enabled := map[string]bool{}
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if !slices.Contains(staleChecks, p) {
			return nil, usageErr(fmt.Errorf("unknown check %q (valid: %s)", p, strings.Join(staleChecks, ", ")))
		}
		enabled[p] = true
	}
	return enabled, nil
}

func newNovelAuditStaleCmd(flags *rootFlags) *cobra.Command {
	var days int
	var checks, dbPath string
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Find unenrolled, inactive, locked or pending-deletion users, stale admins and phones",
		Long: strings.Trim(`
Use this command for a read-only list of inactive, unenrolled, locked-out or
pending-deletion users, inactive admins and phones not seen recently in the
synced tenant. Checks (--check, comma-separated; default all):
  unenrolled        active users still unenrolled --days after creation
  inactive          active/bypass users with no login in --days (or never)
  locked            users whose status is locked out
  pending-deletion  users in the Trash
  admin-inactive    admins with no login in --days
  admin-expired     admins Expired or Pending Activation
  phone-unseen      activated phones whose Duo Mobile was last seen --days ago

Do NOT use it to send enrollment emails; use 'enrollment chase' instead. Do NOT
use it for MFA-skip paths; use 'audit bypass' instead.

Run 'cisco-duo-cli sync --resources users,admins,phones' first (add
--account-id to audit one subaccount). JSON output is {complete, unsynced,
rows}: complete=false means a resource was never synced or its last sync
stopped part-way, so an empty rows list is not a clean tenant.`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli audit stale --days 90 --agent
  cisco-duo-cli audit stale --days 30 --check unenrolled,admin-inactive`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit stale")
			}
			if days < 1 {
				return usageErr(fmt.Errorf("--days must be at least 1"))
			}
			enabled, err := parseChecks(checks)
			if err != nil {
				return err
			}
			data, unsynced, ok, err := loadAudit(cmd, flags, dbPath, "users", "admins", "phones")
			if err != nil {
				return err
			}
			if !ok {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), noMirrorView[staleFinding](unsynced), flags)
				}
				return nil
			}
			users, badU := decodeAllCount[duoUser](data["users"])
			admins, badA := decodeAllCount[duoAdmin](data["admins"])
			phones, badP := decodeAllCount[duoPhone](data["phones"])
			view := newAuditView(staleFindingsFor(users, admins, phones, days, enabled, time.Now()), unsynced, badU+badA+badP)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			warnUndecoded(cmd, view.UndecodedRows)
			switch {
			case len(view.Rows) == 0 && !view.Complete:
				fmt.Fprintln(w, "Nothing stale found, but the local data is incomplete (see the hint above).")
			case len(view.Rows) == 0:
				fmt.Fprintf(w, "Nothing stale in %d users, %d admins and %d phones.\n", len(data["users"]), len(data["admins"]), len(data["phones"]))
			}
			for _, r := range view.Rows {
				fmt.Fprintf(w, "%-17s %-6s %-26s %s\n", r.Check, r.Kind, r.Name, r.Detail)
			}
			return nil
		},
	}
	useAccountFlag(cmd)
	cmd.Flags().IntVar(&days, "days", 90, "Age threshold in days")
	cmd.Flags().StringVar(&checks, "check", "", "Comma-separated checks to run (default all): "+strings.Join(staleChecks, ","))
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: the CLI's synced store)")
	return cmd
}
