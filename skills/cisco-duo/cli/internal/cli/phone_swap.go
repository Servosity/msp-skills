// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"cisco-duo-pp-cli/internal/client"
)

// e164 requires an explicit leading +country code: Duo assumes +1 for
// numbers without one, which would silently turn an international number
// into a different US number.
var e164 = regexp.MustCompile(`^\+[1-9][0-9 ()\-]{6,}$`)

type swapView struct {
	AccountID string    `json:"account_id,omitempty"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Number    string    `json:"number"`
	Status    string    `json:"status"`
	Applied   bool      `json:"applied"`
	Steps     []fixStep `json:"steps"`
	Note      string    `json:"note,omitempty"`
}

type swapOptions struct {
	number                           string
	removeOld, deleteOld, url, apply bool
	force                            bool
	platform                         string
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// samePhoneNumber compares a stored Duo number (country code included, e.g.
// 15555550100) with an E.164 input (+15555550100) digit for digit; no
// country-code guessing, so +47... can never match a +1... number.
func samePhoneNumber(a, b string) bool {
	da, db := digitsOnly(a), digitsOnly(b)
	return da != "" && da == db
}

// phoneActivatable reports whether Duo can send activation to p, and the
// fix: Duo rejects activation when type or platform is Unknown and cannot
// text a landline.
func phoneActivatable(p duoPhone) (ok bool, needsUpdate bool, why string) {
	switch {
	case strings.EqualFold(p.Type, "landline"):
		return false, false, "phone " + p.PhoneID + " is a landline; Duo Mobile cannot be activated on it"
	case p.Quarantined:
		return false, false, "phone " + p.PhoneID + " is quarantined; release it first ('phones set-quarantine-status')"
	case p.Type == "" || strings.EqualFold(p.Type, "unknown") || p.Platform == "" || strings.EqualFold(p.Platform, "unknown"):
		return true, true, "phone " + p.PhoneID + " has an Unknown type or platform"
	}
	return true, false, ""
}

// planSwap builds the swap plan: get a usable phone for the new number,
// attach it, activate it, and only then retire old phones (so a failed
// activation never leaves the user without their working device).
// existing is the phone already registered under the new number, or nil;
// owners maps each old phone_id to its user list (shared phones are kept).
func planSwap(u duoUser, opt swapOptions, existing *duoPhone, owners map[string][]duoPhoneUser) []fixStep {
	steps := make([]fixStep, 0, 8)
	// A new phone plus an activation link is a way through MFA, so a user an
	// administrator deliberately held, or a Disabled group blocks, gets none
	// without --force: the person asking for the swap may be the one held.
	hold := userHold(u)
	if g := disabledGroup(u); hold == "" && g != nil {
		hold = "group " + g.Name + " is Disabled"
	}
	if hold != "" && !opt.force {
		steps = append(steps, fixStep{Step: "hold", Status: "blocked",
			Detail: hold + "; a new phone and activation would give a held user a way through MFA. Review, then re-run with --force"})
	}
	userPhones := "/admin/v1/users/" + url.PathEscape(u.UserID) + "/phones"
	var target *duoPhone
	targetID, sharedNote := "", "" // sharedNote: why the reused phone reaches other users
	for i := range u.Phones {
		if samePhoneNumber(u.Phones[i].Number, opt.number) {
			target = &u.Phones[i]
			steps = append(steps, fixStep{Step: "associate", Status: "skipped", Detail: "number already on this user as " + target.PhoneID, phoneID: target.PhoneID})
			break
		}
	}
	if target == nil && existing != nil {
		target = existing
		steps = append(steps, fixStep{Step: "reuse-phone", Status: "skipped", Detail: "phone " + existing.PhoneID + " already exists for this number", phoneID: existing.PhoneID})
	}
	if target != nil {
		targetID = target.PhoneID
		ok, update, why := phoneActivatable(*target)
		inUse := onUser(u, targetID) && target.Activated && !target.Quarantined // nothing to attach or activate
		if !inUse {
			sharedNote = phoneShareBlock(targetID, u.UserID, owners)
		}
		switch {
		case !ok:
			steps = append(steps, fixStep{Step: "activate", Status: "blocked", Detail: why, phoneID: target.PhoneID})
			return steps
		case sharedNote != "" && !opt.force:
			steps = append(steps, fixStep{Step: "shared-phone", Status: "blocked", Detail: sharedNote + "; review, then re-run with --force", phoneID: target.PhoneID})
			return steps
		case update:
			steps = append(steps, fixStep{Step: "update-phone", Method: "POST", Path: "/admin/v1/phones/" + url.PathEscape(target.PhoneID),
				Status: "planned", Detail: why + "; set type mobile, platform " + opt.platform, phoneID: target.PhoneID})
		}
	} else {
		steps = append(steps, fixStep{Step: "create-phone", Method: "POST", Path: "/admin/v1/phones", Status: "planned",
			Detail: "create a mobile phone " + opt.number + " (" + opt.platform + ")"})
	}
	if target == nil || !onUser(u, targetID) {
		steps = append(steps, fixStep{Step: "associate", Method: "POST", Path: userPhones, Status: "planned",
			Detail: "attach the new phone to " + u.Username, phoneID: targetID})
	}
	// Activation is complete only when the user finishes it on the device,
	// so old phones are retired on a later run, once Duo reports the new
	// phone as activated.
	activated := target != nil && target.Activated && !target.Quarantined && onUser(u, targetID)
	act := fixStep{Step: "activate", Method: "POST", Status: "planned", phoneID: targetID}
	switch {
	case activated:
		act.Status, act.Detail = "skipped", "phone "+targetID+" is already activated"
	case opt.url:
		act.Detail = "return a Duo Mobile activation link"
	default:
		act.Detail = "text a Duo Mobile activation link to " + opt.number
	}
	if sharedNote != "" && act.Status == "planned" { // only reachable with --force
		act.Detail += " (--force overrides: " + sharedNote + ")"
	}
	steps = append(steps, act)
	if opt.removeOld {
		for _, p := range u.Phones {
			if p.PhoneID == targetID || samePhoneNumber(p.Number, opt.number) {
				continue
			}
			detach := fixStep{Step: "disassociate-old", Method: "DELETE", Path: userPhones + "/" + url.PathEscape(p.PhoneID),
				Status: "planned", Detail: "detach old phone " + p.PhoneID + " " + p.Number, phoneID: p.PhoneID}
			if !activated {
				detach.Status, detach.Detail = "deferred", "kept until the new phone is activated; re-run with --remove-old --apply afterwards"
			}
			steps = append(steps, detach)
			if opt.deleteOld {
				others := 0
				for _, o := range owners[p.PhoneID] {
					if o.UserID != u.UserID {
						others++
					}
				}
				del := fixStep{Step: "delete-old", Method: "DELETE", Path: "/admin/v1/phones/" + url.PathEscape(p.PhoneID), phoneID: p.PhoneID}
				if others > 0 {
					del.Status, del.Detail = "skipped", fmt.Sprintf("phone %s is shared with %d other user(s); kept", p.PhoneID, others)
				} else if _, known := owners[p.PhoneID]; !known {
					del.Status, del.Detail = "skipped", "could not confirm the phone's owners; kept"
				} else if !activated {
					del.Status, del.Detail = "deferred", "kept until the new phone is activated; re-run with --remove-old --delete-old --apply afterwards"
				} else {
					del.Status, del.Detail = "planned", "delete phone "+p.PhoneID+" (no other users)"
				}
				steps = append(steps, del)
			}
		}
	}
	return steps
}

func onUser(u duoUser, phoneID string) bool {
	for _, p := range u.Phones {
		if p.PhoneID == phoneID {
			return true
		}
	}
	return false
}

// applySwap runs planned steps in order and stops at the first failure, so
// old phones are only retired after the new one activated.
func applySwap(ctx context.Context, c *client.Client, steps []fixStep, opt swapOptions) error {
	if err := firstBlocked(steps); err != nil {
		return err
	}
	newPhone := "" // the activate step names the target; empty until create-phone runs
	for _, s := range steps {
		if s.Step == "activate" {
			newPhone = s.phoneID
		}
	}
	return runPlanned(steps, func(s *fixStep) error {
		switch s.Step {
		case "create-phone":
			raw, err := duoPost(ctx, c, s.Path, map[string]any{"number": opt.number, "type": "mobile", "platform": opt.platform})
			if err != nil {
				return err
			}
			var p duoPhone
			if json.Unmarshal(raw, &p) != nil || p.PhoneID == "" {
				return fmt.Errorf("create phone returned no phone_id")
			}
			newPhone = p.PhoneID
			s.Result = map[string]string{"phone_id": p.PhoneID}
		case "update-phone":
			_, err := duoPost(ctx, c, s.Path, map[string]any{"type": "mobile", "platform": opt.platform})
			return err
		case "associate":
			_, err := duoPost(ctx, c, s.Path, map[string]any{"phone_id": newPhone})
			return err
		case "disassociate-old", "delete-old":
			return duoDelete(ctx, c, s.Path)
		case "activate":
			if newPhone == "" {
				return fmt.Errorf("no phone to activate")
			}
			path, body := activationRequest(newPhone, opt.url)
			s.Path = path
			raw, err := duoPost(ctx, c, path, body)
			if err != nil {
				return err
			}
			if r, ok := activationResult(raw); ok {
				s.Result = r
			}
		}
		return nil
	})
}

func newNovelPhoneSwapCmd(flags *rootFlags) *cobra.Command {
	var opt swapOptions
	cmd := &cobra.Command{
		Use:   "phone-swap [user]",
		Short: "Move a user to a new phone number and text the Duo Mobile activation link",
		Long: strings.Trim(`
Use this command when a user has a new or lost phone number and needs a
different phone record. It reuses the phone already registered for that
number (or creates one; a reused phone with an Unknown type or platform is
set to mobile first), attaches it to the user and texts a Duo Mobile
activation link (or returns it with --url). Old phones are only detached
(--remove-old) or deleted (--delete-old) once Duo reports the new phone as
activated: run the swap, let the user activate, then re-run the same command
to retire the old phones. An old phone is only deleted when no other user
shares it; a landline cannot be activated, and a phone shared with other
users is not reused without --force (activating it would bind Duo Mobile for
all of them to whoever completes activation). A user an administrator holds
(disabled, in the Trash, a Trust Monitor or Admin API lock, or a Disabled group) gets no
new phone or activation without --force.

Without --apply it prints the plan; with --apply it makes the changes.

Do NOT use it when the user kept the same number and only needs Duo Mobile
reactivated or is locked out; use 'lockout fix' instead.`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli phone-swap jdoe --number +15551234567 --remove-old
  cisco-duo-cli phone-swap jdoe --number +15551234567 --remove-old --delete-old --apply --agent
  cisco-duo-cli phone-swap jdoe@example.com --number +15551234567 --url --apply`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:preview-happy-path": "true", "pp:happy-args": "user=jdoe;--number=+15551234567"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "phone-swap")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("user is required (username, email or user_id)"))
			}
			if !e164.MatchString(strings.TrimSpace(opt.number)) {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--number is required in E.164 form with a leading + and country code, e.g. +15551234567"))
			}
			switch opt.platform {
			case "Generic Smartphone", "Apple iOS", "Google Android":
			default:
				return usageErr(fmt.Errorf("--platform must be Generic Smartphone, Apple iOS or Google Android"))
			}
			if opt.deleteOld && !opt.removeOld {
				return usageErr(fmt.Errorf("--delete-old needs --remove-old"))
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
			onUserAlready := false
			for _, p := range u.Phones {
				onUserAlready = onUserAlready || samePhoneNumber(p.Number, opt.number)
			}
			var existing *duoPhone
			if !onUserAlready { // planSwap only consults existing when the number is not on the user
				raw, _, err := duoGet(ctx, c, "", "/admin/v1/phones", map[string]string{"number": opt.number})
				if err != nil {
					return err
				}
				var phones []duoPhone
				if json.Unmarshal(raw, &phones) == nil && len(phones) > 0 {
					existing = &phones[0]
				}
			}
			// Owners of the phone being reused (a shared phone is not activated
			// without --force) and, with --delete-old, of the old phones.
			ownerIDs := make([]string, 0, len(u.Phones)+1)
			if existing != nil {
				ownerIDs = append(ownerIDs, existing.PhoneID)
			}
			for _, p := range u.Phones {
				if samePhoneNumber(p.Number, opt.number) || opt.deleteOld {
					ownerIDs = append(ownerIDs, p.PhoneID)
				}
			}
			owners := duoPhoneOwners(ctx, c, ownerIDs...)
			view := swapView{AccountID: currentAccountID(), UserID: u.UserID, Username: u.Username, Number: opt.number, Status: u.Status,
				Steps: planSwap(u, opt, existing, owners)}
			if !opt.apply {
				view.Note = "preview only; re-run with --apply to make these changes"
				return writeSwapView(cmd, flags, view)
			}
			err = applySwap(ctx, c, view.Steps, opt)
			view.Applied = true
			if werr := writeSwapView(cmd, flags, view); werr != nil {
				return werr
			}
			return err
		},
	}
	useAccountFlag(cmd)
	cmd.Flags().StringVar(&opt.number, "number", "", "New phone number in E.164 form with a leading + and country code, e.g. +15551234567")
	cmd.Flags().BoolVar(&opt.removeOld, "remove-old", false, "Detach the user's other phones")
	cmd.Flags().BoolVar(&opt.deleteOld, "delete-old", false, "Also delete detached phones that no other user shares (needs --remove-old)")
	cmd.Flags().BoolVar(&opt.url, "url", false, "Return the activation link instead of texting it")
	cmd.Flags().StringVar(&opt.platform, "platform", "Generic Smartphone", "Platform for a newly created phone: Generic Smartphone, Apple iOS or Google Android")
	cmd.Flags().BoolVar(&opt.force, "force", false, "Swap even when an administrator holds the user (disabled, in the Trash, a Trust Monitor or Admin API lock, or a Disabled group), or when the phone for the number is shared with other users")
	cmd.Flags().BoolVar(&opt.apply, "apply", false, "Make the changes (default is a read-only preview of the plan)")
	return cmd
}

func writeSwapView(cmd *cobra.Command, flags *rootFlags, v swapView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFilteredKeep(cmd.OutOrStdout(), v, flags, receiptKeepKeys...)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s (%s, status %s) -> %s\n", v.Username, v.UserID, orDash(v.Status), v.Number)
	printPlanSteps(w, v.Steps, v.Note)
	return nil
}
