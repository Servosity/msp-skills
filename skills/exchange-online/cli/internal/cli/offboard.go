// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

// Steps and parameters follow CIPP's offboarding helpers (Set-CIPPMailboxType,
// Set-CIPPForwarding, Set-CIPPMailboxPermission, Set-CIPPOutOfOffice,
// Remove-CIPPMobileDevice) and MSPbots exchange_online_mcp's offboarding tools.

type offboardStep struct {
	Step   string `json:"step"`
	Status string `json:"status"` // done, already-done, pending, unverified, failed
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type offboardView struct {
	Tenant     string         `json:"tenant"`
	Mailbox    string         `json:"mailbox"`
	Mode       string         `json:"mode"`
	Steps      []offboardStep `json:"steps"`
	Failed     int            `json:"failed"`
	Unverified int            `json:"unverified"`
	NotCovered []string       `json:"not_covered"`
}

type offboardOpts struct {
	manager, autoReply, forwardTo string
	keepDevices, keepCopy, plan   bool
}

func newNovelOffboardCmd(flags *rootFlags) *cobra.Command {
	var o offboardOpts
	cmd := &cobra.Command{
		Use:   "offboard <tenantId> <mailbox>",
		Short: "Run the Exchange side of a leaver: shared mailbox, GAL hide, auto-reply, manager access, device removal.",
		Long: `Runs the Exchange steps of an offboard in order, skipping any already in place:
convert to a shared mailbox, hide from the address list, set an auto-reply (--auto-reply), grant
the manager FullAccess with automapping and Send As (--manager), remove mobile device
partnerships (unless --keep-devices), and forward to an internal recipient (--forward-to,
--keep-copy). Each step reports before and after state. --plan reads the current state and
reports which steps are pending without changing anything. User sign-in, licenses and sessions are
Microsoft Graph tasks and are listed under not_covered.

Use this command to run the Exchange side of a leaver in one pass (shared conversion, GAL hide,
auto-reply, manager FullAccess/SendAs, mobile device removal, optional forwarding) with a per-step
receipt. Do NOT use this command to see what the leaver could access; use 'access-map' instead. To
check a past offboard, re-run 'offboard' with --plan, which reports already-completed steps.`,
		Example:     "  exchange-online-cli offboard 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --manager boss@example.com --plan --agent\n  exchange-online-cli offboard 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --manager boss@example.com --auto-reply \"Jane has left; contact boss@example.com\"",
		Annotations: map[string]string{"mcp:read-only": "false", "exo:preview-flag": "plan", "pp:data-source": "live", "pp:happy-args": "tenantId=0550b473-9fd2-4dbb-a058-a1640b4bf458;mailbox=jane@example.com;--plan"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "offboard")
			}
			// Never change a real mailbox from a test harness: read-only plan instead.
			if cliutil.IsAnyHarness() {
				o.plan = true
			}
			tenant, err := exoTenantArg(cmd, args, 0)
			if err != nil {
				return err
			}
			if len(args) < 2 || !strings.Contains(args[1], "@") {
				return usageErr(fmt.Errorf("missing <mailbox> (a UPN or email address)\nUsage: %s", cmd.UseLine()))
			}
			for name, v := range map[string]string{"--manager": o.manager, "--forward-to": o.forwardTo} {
				if v != "" && !strings.Contains(v, "@") {
					return usageErr(fmt.Errorf("%s must be an email address", name))
				}
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			v, err := runOffboard(cmd.Context(), c, tenant, strings.TrimSpace(args[1]), o)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if perr := printJSONFiltered(cmd.OutOrStdout(), v, flags); perr != nil {
				return perr
			}
			if v.Unverified > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d step(s) unverified; see steps\n", v.Unverified)
			}
			if v.Failed > 0 {
				return apiErr(fmt.Errorf("%d offboard step(s) failed; see steps", v.Failed))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&o.manager, "manager", "", "Grant this user FullAccess (automapped) and Send As on the mailbox")
	cmd.Flags().StringVar(&o.autoReply, "auto-reply", "", "Turn on an automatic reply with this text for internal and external senders")
	cmd.Flags().StringVar(&o.forwardTo, "forward-to", "", "Forward new mail to this recipient inside the tenant (ForwardingAddress)")
	cmd.Flags().BoolVar(&o.keepCopy, "keep-copy", false, "With --forward-to, keep a copy in the shared mailbox")
	cmd.Flags().BoolVar(&o.keepDevices, "keep-devices", false, "Leave mobile device partnerships in place")
	cmd.Flags().BoolVar(&o.plan, "plan", false, "Report which steps are done or pending without changing anything")
	return cmd
}

func runOffboard(ctx context.Context, c *client.Client, tenant, upn string, o offboardOpts) (offboardView, error) {
	v := offboardView{Tenant: tenant, Mailbox: upn, Mode: "apply", Steps: []offboardStep{},
		NotCovered: []string{"block sign-in", "revoke sessions", "remove licenses (after the shared conversion)", "reset password"}}
	if o.plan {
		v.Mode = "plan"
	}
	failStep := func(name, what string, err error) {
		v.Steps = append(v.Steps, offboardStep{Step: name, Status: "failed", Detail: what + ": " + err.Error()})
		v.Failed++
	}
	// check reads the current state: (state for the receipt, target reached?, error).
	type check func() (any, bool, error)
	// step runs one idempotent change and confirms it by re-reading the state.
	step := func(name string, read check, apply func() error) {
		before, done, err := read()
		if err != nil {
			failStep(name, "reading current state", err)
			return
		}
		s := offboardStep{Step: name, Before: before}
		switch {
		case done:
			s.Status = "already-done"
		case o.plan:
			s.Status = "pending"
		default:
			if err := apply(); err != nil {
				s.Status, s.Detail = "failed", err.Error()
				v.Failed++
				break
			}
			after, ok, err := read()
			s.After = after
			switch {
			case err != nil:
				s.Status, s.Detail = "unverified", "applied; re-read failed: "+err.Error()
				v.Unverified++
			case !ok:
				s.Status, s.Detail = "unverified", "applied, but the re-read does not show the change yet (Exchange can take a few minutes); re-run with --plan to confirm"
				v.Unverified++
			default:
				s.Status = "done"
			}
		}
		v.Steps = append(v.Steps, s)
	}
	readMbx := func() (exoRow, error) {
		m, err := getMailbox(ctx, c, tenant, upn, "RecipientTypeDetails,HiddenFromAddressListsEnabled,ForwardingAddress,ForwardingSmtpAddress,DeliverToMailboxAndForward")
		if err != nil {
			return nil, err
		}
		if m == nil {
			return nil, fmt.Errorf("mailbox %s not found", upn)
		}
		return m, nil
	}
	if _, err := readMbx(); err != nil {
		return v, err
	}
	// Validate every input before the first write, so a bad target never leaves
	// the mailbox half offboarded.
	if o.manager != "" {
		mgr, err := resolveRecipient(ctx, c, tenant, o.manager)
		if err != nil {
			return v, fmt.Errorf("resolving --manager %s: %w", o.manager, err)
		}
		if mgr == nil {
			return v, fmt.Errorf("--manager %s is not a recipient in this tenant", o.manager)
		}
		// Add-MailboxPermission -User and Add-RecipientPermission -Trustee take mailbox
		// users, mail users and security groups; offboard promises automapping, which
		// Exchange never applies to group grants, so groups are refused too.
		switch strings.ToLower(mgr.typ) {
		case "usermailbox", "sharedmailbox", "mailuser":
		default:
			return v, fmt.Errorf("--manager %s is a %s; offboard grants automapped access to a mailbox user or mail user (grant a group with 'mailbox-permissions add')", o.manager, orDash(mgr.typ))
		}
	}
	if o.forwardTo != "" {
		// --forward-to is internal-only: refuse a target whose mail leaves the tenant
		// (an outside address, or a contact or mail user pointing outside).
		accepted, err := acceptedDomainSet(ctx, c, tenant)
		if err != nil {
			return v, err
		}
		d := newDestResolver(ctx, c, tenant, accepted).resolve(o.forwardTo)
		switch {
		case d.Unresolved != "":
			return v, fmt.Errorf("--forward-to %s could not be resolved: %s", o.forwardTo, d.Unresolved)
		case isExternal(d.Addr, accepted):
			return v, fmt.Errorf("--forward-to %s delivers outside the tenant (%s); it only forwards internally", o.forwardTo, d.Addr)
		}
	}
	setMailbox := func(params exoRow) func() error {
		return func() error { params["Identity"] = upn; return exoSet(ctx, c, exoBeta(tenant, "Set-Mailbox"), params) }
	}

	step("convert-to-shared", func() (any, bool, error) {
		m, err := readMbx()
		if err != nil {
			return nil, false, err
		}
		return m["RecipientTypeDetails"], strings.EqualFold(rs(m, "RecipientTypeDetails"), "SharedMailbox"), nil
	}, setMailbox(exoRow{"Type": "Shared"}))
	step("hide-from-address-list", func() (any, bool, error) {
		m, err := readMbx()
		if err != nil {
			return nil, false, err
		}
		return m["HiddenFromAddressListsEnabled"], isTrue(rb(m, "HiddenFromAddressListsEnabled")), nil
	}, setMailbox(exoRow{"HiddenFromAddressListsEnabled": true}))

	if o.autoReply != "" {
		want := normalizeReply(o.autoReply)
		step("auto-reply", func() (any, bool, error) {
			r, err := exoGet(ctx, c, exoBeta(tenant, "Get-MailboxAutoReplyConfiguration"), exoRow{"Identity": upn}, "AutoReplyState,InternalMessage,ExternalMessage,ExternalAudience")
			if err != nil {
				return nil, false, err
			}
			if len(r) == 0 {
				return nil, false, nil
			}
			st := map[string]any{"AutoReplyState": r[0]["AutoReplyState"], "ExternalAudience": r[0]["ExternalAudience"]}
			done := strings.EqualFold(rs(r[0], "AutoReplyState"), "Enabled") && strings.EqualFold(rs(r[0], "ExternalAudience"), "All") &&
				normalizeReply(rs(r[0], "InternalMessage")) == want && normalizeReply(rs(r[0], "ExternalMessage")) == want
			return st, done, nil
		}, func() error {
			return exoSet(ctx, c, exoBeta(tenant, "Set-MailboxAutoReplyConfiguration"), exoRow{"Identity": upn, "AutoReplyState": "Enabled",
				"InternalMessage": o.autoReply, "ExternalMessage": o.autoReply, "ExternalAudience": "All"})
		})
	}

	if o.manager != "" {
		// Exchange filters both reads by the manager itself (-User / -Trustee), so
		// every row returned is the manager's whatever alias the caller passed.
		anyone := func(string) bool { return true }
		readFull := func() (any, bool, error) {
			rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-MailboxPermission"), exoRow{"Identity": upn, "User": o.manager}, "User,AccessRights,IsInherited,Deny")
			if err != nil {
				return nil, false, err
			}
			allowed, denied := permissionState(rows, "FullAccess", anyone)
			return map[string]bool{"full_access": allowed, "denied": denied}, allowed, nil
		}
		st, has, err := readFull()
		switch {
		case err != nil:
			failStep("manager-full-access", "reading permissions", err)
		case has:
			// Get-MailboxPermission does not expose automapping, so an existing grant
			// cannot be confirmed as automapped; say so instead of claiming done.
			v.Steps = append(v.Steps, offboardStep{Step: "manager-full-access", Status: "unverified", Before: st,
				Detail: "FullAccess already granted; Exchange does not report whether it was automapped. To force automapping, remove it with 'mailbox-permissions remove' and re-run offboard."})
			v.Unverified++
		default:
			step("manager-full-access", readFull, func() error {
				return exoSet(ctx, c, exoBeta(tenant, "Add-MailboxPermission"), exoRow{"Identity": upn, "User": o.manager,
					"AccessRights": []string{"FullAccess"}, "AutoMapping": true, "InheritanceType": "All"})
			})
		}
		step("manager-send-as", func() (any, bool, error) {
			rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-RecipientPermission"), exoRow{"Identity": upn, "Trustee": o.manager}, "Trustee,AccessRights,AccessControlType")
			if err != nil {
				return nil, false, err
			}
			has, denied := false, false
			for _, r := range rows {
				if !containsFold(rl(r, "AccessRights"), "SendAs") {
					continue
				}
				if strings.EqualFold(rs(r, "AccessControlType"), "Deny") {
					denied = true
				} else {
					has = true
				}
			}
			return map[string]bool{"send_as": has && !denied, "denied": denied}, has && !denied, nil
		}, func() error {
			return exoSet(ctx, c, exoBeta(tenant, "Add-RecipientPermission"), exoRow{"Identity": upn, "Trustee": o.manager, "AccessRights": []string{"SendAs"}})
		})
	}

	if !o.keepDevices {
		devs, err := exoGet(ctx, c, exoBeta(tenant, "Get-MobileDevice"), exoRow{"Mailbox": upn}, "Identity,FriendlyName,DeviceModel,DeviceId")
		if err != nil {
			failStep("remove-mobile-devices", "listing devices", err)
		} else if len(devs) == 0 {
			v.Steps = append(v.Steps, offboardStep{Step: "remove-mobile-devices", Status: "already-done", Before: 0})
		}
		for _, d := range devs {
			id := rs(d, "Identity")
			name := firstNonEmpty(rs(d, "FriendlyName"), rs(d, "DeviceModel"), rs(d, "DeviceId"), id)
			step("remove-mobile-device "+name, func() (any, bool, error) {
				rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-MobileDevice"), exoRow{"Mailbox": upn}, "Identity")
				if err != nil {
					return nil, false, err
				}
				for _, r := range rows {
					if rs(r, "Identity") == id {
						return id, false, nil
					}
				}
				return id, true, nil
			}, func() error {
				return exoSet(ctx, c, exoBeta(tenant, "Remove-MobileDevice"), exoRow{"Identity": id})
			})
		}
	}

	if o.forwardTo != "" {
		step("forward-to", func() (any, bool, error) {
			m, err := readMbx()
			if err != nil {
				return nil, false, err
			}
			cur := rs(m, "ForwardingAddress")
			st := map[string]any{"ForwardingAddress": m["ForwardingAddress"], "ForwardingSmtpAddress": m["ForwardingSmtpAddress"], "DeliverToMailboxAndForward": m["DeliverToMailboxAndForward"]}
			if cur == "" || isTrue(rb(m, "DeliverToMailboxAndForward")) != o.keepCopy || rs(m, "ForwardingSmtpAddress") != "" {
				return st, false, nil
			}
			same, err := sameRecipient(ctx, c, tenant, cur, o.forwardTo)
			return st, same, err
		}, setMailbox(exoRow{"ForwardingAddress": o.forwardTo, "ForwardingSmtpAddress": nil, "DeliverToMailboxAndForward": o.keepCopy}))
	}
	return v, nil
}

// sameRecipient reports whether two recipient identities resolve to the same
// object; a lookup failure is returned so the step fails instead of guessing.
func sameRecipient(ctx context.Context, c *client.Client, tenant, a, b string) (bool, error) {
	ra, err := resolveRecipient(ctx, c, tenant, a)
	if err != nil {
		return false, err
	}
	rb2, err := resolveRecipient(ctx, c, tenant, b)
	if err != nil {
		return false, err
	}
	return ra != nil && rb2 != nil && ra.same(rb2), nil
}

var (
	htmlTagRe  = regexp.MustCompile(`<[^>]*>`)
	htmlLinkRe = regexp.MustCompile(`(?i)\b(?:href|src)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	spaceRe    = regexp.MustCompile(`\s+`)
)

// normalizeReply compares auto-reply content the way a reader meets it:
// Exchange stores replies as HTML and rewraps them, so markup and spacing are
// dropped, but link and image targets are kept because they change what the
// reply does.
func normalizeReply(s string) string {
	var links []string
	for _, m := range htmlLinkRe.FindAllStringSubmatch(s, -1) {
		links = append(links, html.UnescapeString(m[1]+m[2]+m[3]))
	}
	text := html.UnescapeString(htmlTagRe.ReplaceAllString(s, " "))
	text = strings.TrimSpace(spaceRe.ReplaceAllString(text, " "))
	if len(links) > 0 {
		text += " [" + strings.Join(links, " ") + "]"
	}
	return text
}
