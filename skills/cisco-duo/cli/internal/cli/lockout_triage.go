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
)

type triagePhone struct {
	PhoneID      string   `json:"phone_id"`
	Number       string   `json:"number"`
	Name         string   `json:"name"`
	Platform     string   `json:"platform"`
	Activated    bool     `json:"activated"`
	CanPush      bool     `json:"can_push"`
	CanSMS       bool     `json:"can_sms"`
	Quarantined  bool     `json:"quarantined"`
	LastSeen     string   `json:"last_seen"`
	Capabilities []string `json:"capabilities"`
}

type triageCode struct {
	BypassCodeID string `json:"bypass_code_id"`
	Created      string `json:"created"`
	Expires      string `json:"expires"`
	ReuseCount   *int64 `json:"reuse_count"`
	AdminEmail   string `json:"admin_email"`
}

type triageFailure struct {
	Reason   string `json:"reason"`
	Factor   string `json:"factor"`
	Count    int    `json:"count"`
	LastSeen string `json:"last_seen"`
}

type triageAction struct {
	Action  string `json:"action"`
	Command string `json:"command"`
	Why     string `json:"why"`
}

type triageView struct {
	AccountID            string          `json:"account_id,omitempty"`
	UserID               string          `json:"user_id"`
	Username             string          `json:"username"`
	Realname             string          `json:"realname"`
	Email                string          `json:"email"`
	Status               string          `json:"status"`
	LockoutReason        string          `json:"lockout_reason"`
	IsEnrolled           bool            `json:"is_enrolled"`
	LastLogin            string          `json:"last_login"`
	EffectiveStatus      string          `json:"effective_status"`
	EffectiveStatusBasis string          `json:"effective_status_basis"`
	OverridingGroup      *duoGroup       `json:"overriding_group,omitempty"`
	Phones               []triagePhone   `json:"phones"`
	BypassCodes          []triageCode    `json:"bypass_codes"`
	RecentFailures       []triageFailure `json:"recent_failures"`
	TelephonyCredits     *int            `json:"telephony_credits_remaining,omitempty"`
	Findings             []string        `json:"findings"`
	NextActions          []triageAction  `json:"next_actions"`
	Warnings             []string        `json:"warnings,omitempty"`
}

// groupWithStatus returns the first of the user's groups with this status.
func groupWithStatus(u duoUser, status string) *duoGroup {
	for i := range u.Groups {
		if strings.EqualFold(u.Groups[i].Status, status) {
			g := u.Groups[i]
			return &g
		}
	}
	return nil
}

// disabledGroup returns the first group whose status is Disabled.
func disabledGroup(u duoUser) *duoGroup { return groupWithStatus(u, "disabled") }

// effectiveStatus applies Duo's precedence: a locked-out user stays locked
// out whatever their groups say; otherwise a Disabled group overrides the
// user's own status (not shown on the user record), an explicit user disable
// or pending deletion comes next, then a Bypass group. A Disabled group on a
// locked-out user is still returned so callers can flag it as the blocker
// that remains after unlocking.
func effectiveStatus(u duoUser) (string, string, *duoGroup) {
	status := strings.ToLower(u.Status)
	disabled := disabledGroup(u)
	switch {
	case status == "locked out" && disabled != nil:
		return "locked out", "user status; group " + disabled.Name + " is Disabled and will still block after unlock", disabled
	case status == "locked out":
		return "locked out", "user status", nil
	case disabled != nil:
		return "disabled", "group " + disabled.Name + " is Disabled", disabled
	case status == "disabled" || status == "pending deletion":
		return status, "user status", nil
	}
	if g := groupWithStatus(u, "bypass"); g != nil {
		return "bypass", "group " + g.Name + " is Bypass", g
	}
	return status, "user status", nil
}

// buildTriage turns the fetched facts into findings and ordered next actions.
func buildTriage(u duoUser, codes []duoBypassCode, failures []triageFailure, credits *int, account string, now time.Time) triageView {
	v := triageView{
		AccountID: account, UserID: u.UserID, Username: u.Username, Realname: u.Realname, Email: u.Email,
		Status: u.Status, LockoutReason: u.lockoutReason(), IsEnrolled: u.IsEnrolled, LastLogin: unixPtrTime(u.LastLogin),
		Phones: make([]triagePhone, 0), BypassCodes: make([]triageCode, 0), RecentFailures: failures,
		TelephonyCredits: credits, Findings: make([]string, 0), NextActions: make([]triageAction, 0),
	}
	if v.RecentFailures == nil {
		v.RecentFailures = make([]triageFailure, 0)
	}
	v.EffectiveStatus, v.EffectiveStatusBasis, v.OverridingGroup = effectiveStatus(u)
	// Suggested commands use the user_id (unambiguous) and shell-quote every
	// argument so a copied command targets exactly this user and account.
	who := shellQuote(u.UserID)
	if u.UserID == "" {
		who = shellQuote(u.Username)
	}
	acctFlag := ""
	if account != "" {
		acctFlag = " --account-id " + shellQuote(account)
	}
	add := func(action, command, why string) {
		v.NextActions = append(v.NextActions, triageAction{Action: action, Command: command, Why: why})
	}

	// Never suggest anything that grants a way through MFA (a phone, an
	// activation, a quarantine release, an enrollment code, a bypass code) for
	// a user an administrator deliberately held or a Disabled group blocks:
	// the only suggestions for them are the reviewed --force unlock or the
	// group change.
	held := userHold(u) != "" || disabledGroup(u) != nil
	pushReady := false
	for _, p := range u.Phones {
		tp := triagePhone{PhoneID: p.PhoneID, Number: p.Number, Name: p.Name, Platform: p.Platform, Activated: p.Activated,
			CanPush: hasCapability(p, "push"), CanSMS: hasCapability(p, "sms"), Quarantined: p.Quarantined, LastSeen: string(p.LastSeen),
			Capabilities: p.Capabilities}
		if tp.Capabilities == nil {
			tp.Capabilities = []string{}
		}
		if tp.Activated && tp.CanPush && !p.Quarantined {
			pushReady = true
		}
		if p.Quarantined {
			v.Findings = append(v.Findings, "Phone "+p.PhoneID+" is quarantined, so it cannot authenticate.")
		}
		if p.Quarantined && !held {
			add("release the quarantined phone after review", "cisco-duo-cli phones set-quarantine-status "+shellQuote(p.PhoneID)+" --quarantine-status false"+acctFlag,
				"quarantined devices are blocked from Duo authentication")
		}
		v.Phones = append(v.Phones, tp)
	}
	for _, c := range codes {
		if c.expired(now) {
			continue // expired codes cannot be used
		}
		expires := "never"
		if c.Expiration != nil {
			expires = unixPtrTime(c.Expiration)
		}
		v.BypassCodes = append(v.BypassCodes, triageCode{BypassCodeID: c.BypassCodeID, Created: unixTime(c.Created),
			Expires: expires, ReuseCount: c.ReuseCount, AdminEmail: c.AdminEmail})
	}

	if v.OverridingGroup != nil && strings.EqualFold(v.OverridingGroup.Status, "disabled") {
		v.Findings = append(v.Findings, "Group "+v.OverridingGroup.Name+" is Disabled, which blocks this user no matter what their own status is.")
		add("after review, take the user out of the disabled group", "cisco-duo-cli users remove-group "+shellQuote(u.UserID)+" "+shellQuote(v.OverridingGroup.GroupID)+acctFlag,
			"a Disabled group is usually a deliberate hold, and unlocking the user alone will not restore access while it overrides them; 'groups update --status Active' would lift it for every member")
	}
	switch strings.ToLower(u.Status) {
	case "locked out":
		reason := u.lockoutReason()
		v.Findings = append(v.Findings, "User is locked out (reason: "+orDash(reason)+").")
		switch reason {
		case "Admin disabled", "Admin API disabled":
			v.Findings = append(v.Findings, "An administrator deliberately held this user ("+userHold(u)+"); confirm it is safe before unlocking.")
			add("unlock after review", "cisco-duo-cli lockout fix "+who+acctFlag+" --force --apply", "administrator holds need an explicit override")
		case "Not enrolled":
			add("unlock", "cisco-duo-cli lockout fix "+who+acctFlag+" --apply", "the user never enrolled before the enrollment window closed")
			if u.Email != "" && !held {
				add("email a new enrollment link", "cisco-duo-cli users enroll --username "+shellQuote(u.Username)+" --email "+shellQuote(u.Email)+acctFlag, "lets the user enroll a device")
			}
		default:
			add("unlock", "cisco-duo-cli lockout fix "+who+acctFlag+" --apply", "status is locked out after "+orDash(reason))
		}
	case "disabled":
		v.Findings = append(v.Findings, "User status is disabled; an administrator or directory sync disabled the account.")
		add("re-enable deliberately", "cisco-duo-cli lockout fix "+who+acctFlag+" --force --apply", "disabled is an intentional state; --force is required")
	case "pending deletion":
		v.Findings = append(v.Findings, "User is in the Trash pending deletion.")
		add("restore from Trash", "cisco-duo-cli users bulk-restore --user-id-list "+shellQuote(`["`+u.UserID+`"]`)+acctFlag, "users in the Trash cannot authenticate")
	case "bypass":
		v.Findings = append(v.Findings, "User status is bypass: they skip MFA entirely.")
	}
	if !u.IsEnrolled {
		v.Findings = append(v.Findings, "User has no enrolled authentication device.")
	}
	if held {
		// the unlock-after-review / group actions above are the only path
	} else if len(u.Phones) == 0 {
		if !u.IsEnrolled { // an enrolled user with no phone uses a token or WebAuthn
			add("attach a phone", "cisco-duo-cli phone-swap "+who+" --number NEW_NUMBER_E164"+acctFlag, "the user has no phone to approve pushes")
		}
	} else if !pushReady {
		v.Findings = append(v.Findings, "No phone is ready for Duo Push (activated, push-capable and not quarantined).")
		if phone, _ := pickReactivationPhone(u, ""); phone != nil { // a landline or quarantined phone cannot be reactivated
			add("reactivate Duo Mobile", "cisco-duo-cli lockout fix "+who+acctFlag+" --reactivate-phone --apply", "re-sends Duo Mobile activation to the existing phone")
		}
	}
	smsFailures := 0
	for _, f := range v.RecentFailures {
		if f.Factor == "sms_passcode" || f.Factor == "phone_call" || f.Factor == "sms_refresh" {
			smsFailures += f.Count
		}
	}
	if credits != nil && *credits <= 0 {
		v.Findings = append(v.Findings, "The account has no telephony credits left, so SMS and phone-call MFA fail.")
		if smsFailures > 0 && account != "" {
			add("move telephony credits to this subaccount", "cisco-duo-cli billing set-telephony-credits --account-id "+shellQuote(account)+" --credits 1000",
				"recent failures used SMS or voice, which need credits; --credits is the new total, taken from the parent")
		} else if smsFailures > 0 {
			add("buy telephony credits", "", "recent failures used SMS or voice, which need credits; add them in the Duo Admin Panel billing page")
		}
	}
	if len(v.BypassCodes) > 0 {
		v.Findings = append(v.Findings, strconv.Itoa(len(v.BypassCodes))+" unexpired bypass code(s) already exist for this user.")
	}
	if !held && v.EffectiveStatus != "bypass" && (len(v.NextActions) > 0 || v.EffectiveStatus != "active") { // only when something is wrong; a bypass user already skips MFA
		add("issue a temporary bypass code", "cisco-duo-cli lockout fix "+who+acctFlag+" --bypass --apply",
			"single-use, one-hour code that keeps existing codes; use when the user must get in now")
	}
	if len(v.Findings) == 0 && pushReady {
		v.Findings = append(v.Findings, "No lockout detected: the user is active with a push-ready phone.")
	} else if len(v.Findings) == 0 {
		v.Findings = append(v.Findings, "No lockout detected: the user is active and enrolled with a factor other than a phone.")
	}
	return v
}

// rollupTriageFailures counts recent non-success auth events by reason/factor.
func rollupTriageFailures(events []authLogEvent) []triageFailure {
	type key struct{ reason, factor string }
	agg := map[key]*triageFailure{}
	for _, e := range events {
		if strings.EqualFold(e.Result, "success") {
			continue
		}
		k := key{e.Reason, e.Factor}
		f := agg[k]
		if f == nil {
			f = &triageFailure{Reason: e.Reason, Factor: e.Factor}
			agg[k] = f
		}
		f.Count++
		if e.ISOTimestamp > f.LastSeen {
			f.LastSeen = e.ISOTimestamp
		}
	}
	out := make([]triageFailure, 0, len(agg))
	for _, f := range agg {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// fetchTriage gathers user, bypass codes, last-24h auth failures and credits
// in the account the request is routed to (--account-id). Optional facts
// degrade to warnings when the integration lacks a permission.
func fetchTriage(ctx context.Context, c *client.Client, who string) (triageView, error) {
	u, err := duoFindUser(ctx, c, who)
	if err != nil {
		return triageView{}, err
	}
	var warnings []string
	codes := make([]duoBypassCode, 0)
	if raw, _, err := duoGet(ctx, c, "", "/admin/v1/users/"+url.PathEscape(u.UserID)+"/bypass_codes", map[string]string{"limit": "500"}); err != nil {
		warnings = append(warnings, "bypass codes unavailable: "+firstLine(err))
	} else if err := json.Unmarshal(raw, &codes); err != nil {
		warnings = append(warnings, "bypass codes unparseable: "+err.Error())
	}
	now := time.Now()
	events, _, capHit, err := fetchAuthLogs(ctx, c, url.Values{"users": {u.UserID}}, now.Add(-24*time.Hour), now, 5)
	if err != nil {
		warnings = append(warnings, "auth log unavailable (needs 'Grant read log'): "+firstLine(err))
	} else if capHit {
		warnings = append(warnings, "more than 5000 auth events in 24h; failure counts cover the newest 5000 only")
	}
	var credits *int
	if raw, _, err := duoGet(ctx, c, "", "/admin/v1/info/summary", nil); err != nil {
		warnings = append(warnings, "account summary unavailable (needs 'Grant read information'): "+firstLine(err))
	} else {
		var sum struct {
			Credits *int `json:"telephony_credits_remaining"`
		}
		if json.Unmarshal(raw, &sum) == nil {
			credits = sum.Credits
		}
	}
	v := buildTriage(u, codes, rollupTriageFailures(events), credits, currentAccountID(), now)
	v.Warnings = warnings // omitted from JSON when nil
	return v, nil
}

func firstLine(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func newNovelLockoutTriageCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triage [user]",
		Short: "Diagnose why one user cannot complete Duo MFA and name the fix",
		Long: strings.Trim(`
Use this command to diagnose why one known user cannot complete Duo MFA in one
tenant; it changes nothing. It reads the user (with groups and phones), their
bypass codes, the last 24 hours of authentication failures and the account's
telephony credits, then reports the effective status (a Disabled or Bypass
group can override the user's own status) and ordered next_actions.

Do NOT use it to unlock, issue a bypass code or reactivate a phone; use
'lockout fix' instead. Do NOT use it to find which customer tenant a person
belongs to; use 'fleet find-user' instead.

The user can be a username, alias, email address or user_id.`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli lockout triage jdoe --agent
  cisco-duo-cli lockout triage jdoe@example.com --account-id DA9VZOC5X63I2W72NRP9
  cisco-duo-cli lockout triage jdoe --agent --select effective_status,lockout_reason,next_actions`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "user=jdoe"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "lockout triage")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("user is required (username, email or user_id)"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view, err := fetchTriage(ctx, c, args[0])
			if err != nil {
				return err
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			return printTriageHuman(cmd, view)
		},
	}
	useAccountFlag(cmd)
	return cmd
}

func printTriageHuman(cmd *cobra.Command, v triageView) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s (%s) %s\n", v.Username, v.UserID, v.Email)
	fmt.Fprintf(w, "status: %s  lockout_reason: %s  effective: %s (%s)\n", v.Status, orDash(v.LockoutReason), v.EffectiveStatus, v.EffectiveStatusBasis)
	fmt.Fprintf(w, "enrolled: %t  last login: %s\n", v.IsEnrolled, orDash(v.LastLogin))
	for _, p := range v.Phones {
		fmt.Fprintf(w, "phone %s %s %s activated=%t push=%t last_seen=%s\n", p.PhoneID, p.Number, p.Platform, p.Activated, p.CanPush, orDash(p.LastSeen))
	}
	fmt.Fprintf(w, "unexpired bypass codes: %d\n", len(v.BypassCodes))
	for _, f := range v.RecentFailures {
		fmt.Fprintf(w, "failure (24h): %s via %s x%d, last %s\n", f.Reason, f.Factor, f.Count, f.LastSeen)
	}
	fmt.Fprintln(w, "\nFindings:")
	for _, f := range v.Findings {
		fmt.Fprintln(w, "  - "+f)
	}
	fmt.Fprintln(w, "\nNext actions:")
	for i, a := range v.NextActions {
		fmt.Fprintf(w, "  %d. %s\n     %s\n     (%s)\n", i+1, a.Action, a.Command, a.Why)
	}
	for _, warn := range v.Warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+warn)
	}
	return nil
}
