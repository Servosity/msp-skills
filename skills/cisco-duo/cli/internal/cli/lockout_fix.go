// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"cisco-duo-pp-cli/internal/client"
)

type fixStep struct {
	Step    string `json:"step"`
	Method  string `json:"method,omitempty"`
	Path    string `json:"path,omitempty"`
	Status  string `json:"status"` // planned | skipped | blocked | done | failed
	Detail  string `json:"detail,omitempty"`
	Result  any    `json:"result,omitempty"`
	phoneID string // phone the step acts on
}

// activationRequest is the Duo Mobile activation call for a phone: texted
// (send_sms_activation) or returned as a link (activation_url).
func activationRequest(phoneID string, asURL bool) (string, map[string]any) {
	body := map[string]any{"valid_secs": "86400"}
	if asURL {
		body["install"] = "1"
		return "/admin/v1/phones/" + url.PathEscape(phoneID) + "/activation_url", body
	}
	return "/admin/v1/phones/" + url.PathEscape(phoneID) + "/send_sms_activation", body
}

// activationResult keeps the activation response minus the QR image URL (the
// link itself is enough); ok is false when the response is not an object.
func activationResult(raw json.RawMessage) (map[string]any, bool) {
	var r map[string]any
	if json.Unmarshal(raw, &r) != nil || r == nil {
		return nil, false
	}
	delete(r, "activation_barcode")
	return r, true
}

// firstBlocked turns the first blocked step into the command's error.
func firstBlocked(steps []fixStep) error {
	for _, s := range steps {
		if s.Status == "blocked" {
			return apiErr(fmt.Errorf("%s blocked: %s", s.Step, s.Detail))
		}
	}
	return nil
}

// runPlanned runs each planned step in order and stops at the first failure,
// recording done or failed on the step.
func runPlanned(steps []fixStep, do func(*fixStep) error) error {
	for i := range steps {
		s := &steps[i]
		if s.Status != "planned" {
			continue
		}
		if err := do(s); err != nil {
			s.Status, s.Detail = "failed", firstLine(err)
			return err
		}
		s.Status = "done"
	}
	return nil
}

// printPlanSteps renders plan steps and the note for human output.
func printPlanSteps(w io.Writer, steps []fixStep, note string) {
	for _, s := range steps {
		fmt.Fprintf(w, "  [%s] %s %s\n", s.Status, s.Step, s.Detail)
		if s.Result != nil {
			b, _ := json.Marshal(s.Result)
			fmt.Fprintf(w, "         %s\n", b)
		}
	}
	if note != "" {
		fmt.Fprintln(w, note)
	}
}

type fixView struct {
	AccountID string    `json:"account_id,omitempty"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Applied   bool      `json:"applied"`
	Before    string    `json:"status_before"`
	After     string    `json:"status_after,omitempty"`
	Steps     []fixStep `json:"steps"`
	Note      string    `json:"note,omitempty"`
}

type fixOptions struct {
	bypass, reactivate, force, url, apply bool
	validSecs                             int
	phoneID                               string
	owners                                map[string][]duoPhoneUser // users of the phone to reactivate
}

// planFix decides which guarded writes apply to this user. It never calls the API.
func planFix(u duoUser, opt fixOptions) []fixStep {
	steps := make([]fixStep, 0, 3)
	userPath := "/admin/v1/users/" + url.PathEscape(u.UserID)
	_, basis, _ := effectiveStatus(u)
	group := disabledGroup(u)

	unlock := fixStep{Step: "unlock", Method: "POST", Path: userPath}
	hold := userHold(u)
	switch strings.ToLower(u.Status) {
	case "locked out", "disabled":
		switch {
		case hold != "" && !opt.force:
			unlock.Status, unlock.Detail = "blocked", hold+"; review, then re-run with --force"
		case hold != "":
			unlock.Status, unlock.Detail = "planned", "set status active (was "+u.Status+": "+hold+"; --force)"
		default:
			unlock.Status, unlock.Detail = "planned", "set status active (was locked out: "+orDash(u.lockoutReason())+")"
		}
	default:
		unlock.Status, unlock.Detail = "skipped", "status is "+orDash(u.Status)+"; nothing to unlock"
	}
	steps = append(steps, unlock)
	if group != nil {
		steps = append(steps, fixStep{Step: "group-override", Status: "blocked",
			Detail: basis + "; the user stays blocked until group " + group.GroupID + " is set Active or the user leaves it"})
	}

	// An administrator hold (userHold) or a Disabled group blocks access; a
	// bypass code or a fresh activation would hand that same person a way
	// through MFA anyway, so they are blocked with it (--force lifts them).
	held := !opt.force && (hold != "" || group != nil)
	heldDetail := "not issued while the user is held (" + hold + "); review, then re-run with --force"
	if hold == "" && group != nil {
		heldDetail = "not issued while group " + group.GroupID + " is Disabled; it would work the moment the group changes. Re-run with --force to issue it anyway"
	}
	if opt.bypass {
		st := fixStep{Step: "bypass-code", Method: "POST", Path: userPath + "/bypass_codes", Status: "planned",
			Detail: "one single-use code valid " + strconv.Itoa(opt.validSecs) + "s; existing codes are kept"}
		if held {
			st.Status, st.Detail = "blocked", heldDetail
		}
		steps = append(steps, st)
	}
	if opt.reactivate {
		st := fixStep{Step: "reactivate-phone", Method: "POST", Status: "planned"}
		phone, why := pickReactivationPhone(u, opt.phoneID)
		shared := ""
		if phone != nil {
			shared = phoneShareBlock(phone.PhoneID, u.UserID, opt.owners)
		}
		if held {
			st.Status, st.Detail = "blocked", heldDetail
			if shared != "" { // show both, so a --force approved for the hold is not blind to the sharing
				st.Detail += "; also: " + shared
			}
		} else if phone == nil {
			st.Status, st.Detail = "blocked", why
		} else if shared != "" && !opt.force {
			st.Status, st.Detail, st.phoneID = "blocked", shared+"; review, then re-run with --force", phone.PhoneID
		} else {
			st.phoneID = phone.PhoneID
			st.Path, _ = activationRequest(phone.PhoneID, opt.url)
			if opt.url {
				st.Detail = "return a Duo Mobile activation link for " + phone.Number
			} else {
				st.Detail = "text a Duo Mobile activation link to " + phone.Number
			}
			if shared != "" {
				st.Detail += " (--force overrides: " + shared + ")"
			}
		}
		steps = append(steps, st)
	}
	return steps
}

// pickReactivationPhone chooses the phone to re-send activation to. Duo
// rejects activation for phones whose type or platform is Unknown.
func pickReactivationPhone(u duoUser, want string) (*duoPhone, string) {
	if len(u.Phones) == 0 {
		return nil, "user has no phone; use 'phone-swap' to add one"
	}
	usable := func(p duoPhone) bool {
		return !p.Quarantined && !strings.EqualFold(p.Platform, "unknown") && !strings.EqualFold(p.Type, "unknown") && !strings.EqualFold(p.Type, "landline")
	}
	for i := range u.Phones {
		p := u.Phones[i]
		if want != "" && p.PhoneID != want {
			continue
		}
		if usable(p) {
			return &p, ""
		}
		if want != "" {
			return nil, "phone " + want + " is quarantined, a landline, or has an Unknown platform; Duo cannot activate it"
		}
	}
	if want != "" {
		return nil, "phone " + want + " is not associated with this user"
	}
	return nil, "no smartphone with a known platform; set the platform with 'phones update' or use 'phone-swap'"
}

func applyFix(ctx context.Context, c *client.Client, steps []fixStep, opt fixOptions) error {
	return runPlanned(steps, func(s *fixStep) error {
		var body map[string]any
		switch s.Step {
		case "unlock":
			body = map[string]any{"status": "active"}
		case "bypass-code":
			body = map[string]any{"count": "1", "reuse_count": "1", "valid_secs": strconv.Itoa(opt.validSecs), "preserve_existing": true}
		case "reactivate-phone":
			s.Path, body = activationRequest(s.phoneID, opt.url)
		}
		raw, err := duoPost(ctx, c, s.Path, body)
		if err != nil {
			return err
		}
		switch s.Step {
		case "bypass-code":
			var codes []string
			if json.Unmarshal(raw, &codes) == nil {
				s.Result = map[string]any{"codes": codes, "valid_secs": opt.validSecs, "uses": 1}
			}
		case "reactivate-phone":
			if r, ok := activationResult(raw); ok {
				s.Result = r
			}
		}
		return nil
	})
}

func newNovelLockoutFixCmd(flags *rootFlags) *cobra.Command {
	var opt fixOptions
	cmd := &cobra.Command{
		Use:   "fix [user]",
		Short: "Unlock a locked-out user, issue a one-time bypass code, re-send Duo Mobile activation",
		Long: strings.Trim(`
Use this command to restore MFA access for one locked-out user: unlock, issue
a single-use expiring bypass code, or re-send Duo Mobile activation to the
phone they already have. Without --apply it reads the user and prints the
plan; with --apply it applies the plan and returns a before/after receipt.

Guards: it only unlocks users whose status is 'locked out'; it refuses an
administrator hold (a Trust Monitor lock 'Admin disabled', an Admin API lock
'Admin API disabled', a disabled user, or a user in the Trash) without --force, and while the user
is held it issues no bypass code or activation either; a Disabled group that
overrides the user is reported as blocked, also withholds the bypass code and
activation without --force, and the command exits non-zero when applied. A
phone shared with other users is not reactivated without --force (activation
would bind Duo Mobile for all of them). Bypass codes are created with
preserve_existing=true so earlier codes are not wiped.

Do NOT use it when the user has a new or lost phone number; use 'phone-swap'
instead. Do NOT use it only to diagnose; use 'lockout triage' instead.`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli lockout fix jdoe --bypass --reactivate-phone
  cisco-duo-cli lockout fix jdoe --account-id DA9VZOC5X63I2W72NRP9 --bypass --apply --agent
  cisco-duo-cli lockout fix jdoe@example.com --reactivate-phone --url --apply`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:preview-happy-path": "true", "pp:happy-args": "user=jdoe;--bypass"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "lockout fix")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("user is required (username, email or user_id)"))
			}
			if opt.validSecs < 60 || opt.validSecs > 7*24*3600 {
				return usageErr(fmt.Errorf("--valid-secs must be between 60 and 604800"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			u, err := duoFindUser(ctx, c, args[0])
			if err != nil {
				return err
			}
			if phone, _ := pickReactivationPhone(u, opt.phoneID); opt.reactivate && phone != nil {
				opt.owners = duoPhoneOwners(ctx, c, phone.PhoneID)
			}
			view := fixView{AccountID: currentAccountID(), UserID: u.UserID, Username: u.Username, Before: u.Status, Steps: planFix(u, opt)}
			blockedErr := firstBlocked(view.Steps)
			if !opt.apply {
				view.Note = "preview only; re-run with --apply to make these changes"
				return writeFixView(cmd, flags, view)
			}
			if blockedErr != nil && !hasPlanned(view.Steps) {
				view.Note = "nothing applied"
				_ = writeFixView(cmd, flags, view)
				return blockedErr
			}
			applyErr := applyFix(ctx, c, view.Steps, opt)
			view.Applied = true
			if after, err := duoFindUser(ctx, c, u.UserID); err == nil {
				view.After = after.Status
			}
			if err := writeFixView(cmd, flags, view); err != nil {
				return err
			}
			if applyErr != nil {
				return applyErr
			}
			return blockedErr
		},
	}
	useAccountFlag(cmd)
	cmd.Flags().BoolVar(&opt.bypass, "bypass", false, "Issue one single-use bypass code (existing codes are kept)")
	cmd.Flags().IntVar(&opt.validSecs, "valid-secs", 3600, "Bypass code lifetime in seconds (60-604800)")
	cmd.Flags().BoolVar(&opt.reactivate, "reactivate-phone", false, "Re-send Duo Mobile activation to the user's existing smartphone")
	cmd.Flags().StringVar(&opt.phoneID, "phone", "", "phone_id to reactivate when the user has several")
	cmd.Flags().BoolVar(&opt.url, "url", false, "Return an activation link instead of texting it")
	cmd.Flags().BoolVar(&opt.apply, "apply", false, "Make the changes (default is a read-only preview of the plan)")
	cmd.Flags().BoolVar(&opt.force, "force", false, "Unlock a Trust Monitor or Admin API lock or re-enable a disabled user, issue the bypass code and activation otherwise withheld from a held user (including one in the Trash, which this command does not restore) or a Disabled-group user, and reactivate a phone shared with other users")
	return cmd
}

func hasPlanned(steps []fixStep) bool {
	for _, s := range steps {
		if s.Status == "planned" {
			return true
		}
	}
	return false
}

// receiptKeepKeys are step/target keys that --agent/--compact must keep even
// when only a minority of rows carry them: a step's result (an issued bypass
// code, a created phone_id, an activation link) and a target's error exist
// only on the rows they describe, and the compact 80%-frequency rule would
// otherwise drop them from exactly the partially applied receipts that need
// them.
var receiptKeepKeys = []string{"result", "error"}

func writeFixView(cmd *cobra.Command, flags *rootFlags, v fixView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFilteredKeep(cmd.OutOrStdout(), v, flags, receiptKeepKeys...)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s (%s) status: %s", v.Username, v.UserID, v.Before)
	if v.After != "" {
		fmt.Fprintf(w, " -> %s", v.After)
	}
	fmt.Fprintln(w)
	printPlanSteps(w, v.Steps, v.Note)
	return nil
}
