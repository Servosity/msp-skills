// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: offboard - one leaver, the right order.
//
// Order is forced by the API: backup-code invalidation and most security
// calls fail on a suspended account, and Drive/Calendar transfers need a
// live owner, so suspension runs LAST. Licence release waits for the async
// transfer. Without --yes the command previews the plan using live reads
// and changes nothing; --dry-run makes no network calls at all.

// pp:data-source live

package cli

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/client"
)

var offboardScopes = []string{
	"https://www.googleapis.com/auth/admin.directory.user",
	"https://www.googleapis.com/auth/admin.directory.user.security",
	"https://www.googleapis.com/auth/admin.directory.group.member",
}

type offboardStep struct {
	Step   string `json:"step"`
	Status string `json:"status"` // planned | done | skipped | failed
	Detail string `json:"detail,omitempty"`
	Count  int    `json:"count,omitempty"`
}

type offboardReceipt struct {
	Tenant     string         `json:"tenant"`
	User       string         `json:"user"`
	UserID     string         `json:"user_id,omitempty"`
	Mode       string         `json:"mode"` // dry-run | preview | executed
	TransferID string         `json:"transfer_id,omitempty"`
	Steps      []offboardStep `json:"steps"`
	Next       []string       `json:"next,omitempty"`
}

type offboardOpts struct {
	ou, successor                       string
	keepGroups, noSuspend, keepPassword bool
	releaseLicense                      bool
	products                            string
}

func userPath(key string) string {
	return "/admin/directory/v1/users/" + url.PathEscape(key)
}

func randomPassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newNovelOffboardCmd(flags *rootFlags) *cobra.Command {
	var o offboardOpts
	cmd := &cobra.Command{
		Use:   "offboard <user-email>",
		Short: "Offboard a leaver in the right order: revoke access, remove groups, transfer data, suspend last",
		Long: "Runs the offboarding runbook for one account in the order the API requires:\n" +
			"  move OU, scramble password, sign out, revoke OAuth tokens, delete app passwords,\n" +
			"  invalidate backup codes, remove from groups, transfer Drive+Calendar to --successor,\n" +
			"  release licences (only when no transfer is pending), and suspend LAST.\n" +
			"The tenant is resolved from the email domain unless --tenant is given.\n" +
			"Without --yes it previews the plan with live reads and changes nothing.\n" +
			"Needs the tenant's 'full' scope profile (tenants add ... --scopes full). Over MCP it can\n" +
			"only preview, because MCP clients cannot pass --yes.\n" +
			"Use this command to offboard one leaver in the correct order. Do NOT use this command to\n" +
			"audit accounts that are already suspended; use 'leavers' instead.",
		Example: "  google-workspace-cli offboard jane@example.com --successor manager@example.com --ou /Departed\n" +
			"  google-workspace-cli offboard jane@example.com --successor manager@example.com --ou /Departed --yes --agent",
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				if len(args) != 1 {
					return writeDryRun(cmd.OutOrStdout(), flags, "offboard a user")
				}
				// No registry, key or network access in dry-run: the plan only,
				// stamped with the dry_run/action keys of the dry-run contract.
				plan, err := json.Marshal(offboardReceipt{User: strings.ToLower(args[0]), Mode: "dry-run", Steps: plannedOffboardSteps(o)})
				if err != nil {
					return err
				}
				return printStampedDryRunOutput(cmd.OutOrStdout(), plan, flags, map[string]any{"source": "live"}, "offboard "+strings.ToLower(args[0]))
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("offboard takes exactly one user email"))
			}
			email := strings.ToLower(strings.TrimSpace(args[0]))
			if !strings.Contains(email, "@") {
				return usageErr(fmt.Errorf("offboard takes the user's primary email"))
			}
			t, err := offboardTenant(email, flags.configPath)
			if err != nil {
				return err
			}
			if strings.EqualFold(email, t.Admin) {
				return usageErr(fmt.Errorf("refusing to offboard %s: it is the admin tenant %s impersonates", email, t.Name))
			}
			if o.successor != "" && strings.EqualFold(o.successor, email) {
				return usageErr(fmt.Errorf("--successor must be a different user"))
			}
			rec := offboardReceipt{Tenant: t.Name, User: email}
			needed := append([]string{}, offboardScopes...)
			if o.successor != "" {
				needed = append(needed, "https://www.googleapis.com/auth/admin.datatransfer")
			}
			if o.releaseLicense {
				needed = append(needed, "https://www.googleapis.com/auth/apps.licensing")
			}
			execute := confirmed(cmd, flags)
			// The preview only reads (read profile is enough); applying needs
			// the write scopes. The implicit bearer tenant has no recorded
			// scopes, so its token's own grants decide.
			if missing := hasScopes(t, needed); execute && t.KeyFile != "" && len(missing) > 0 {
				return authErr(fmt.Errorf("tenant %s is missing offboarding scopes %s: re-register with 'tenants add %s ... --scopes full --yes', authorize them in the Admin console, then 'tenants test %s'",
					t.Name, strings.Join(missing, ","), t.Name, t.Name))
			}
			c, err := flags.newTenantClient(cmd.Context(), t)
			if err != nil {
				return err
			}
			if execute {
				rec.Mode = "executed"
			} else {
				rec.Mode = "preview"
			}
			runErr := runOffboard(cmd.Context(), c, t, email, o, execute, &rec)
			if !execute {
				rec.Next = append(rec.Next, "re-run with --yes to apply these steps")
			}
			return printOffboard(cmd, flags, rec, runErr)
		},
	}
	cmd.Flags().StringVar(&o.ou, "ou", "", "Org unit to move the account to (e.g. /Departed)")
	cmd.Flags().StringVar(&o.successor, "successor", "", "Successor email who receives the leaver's Drive files and Calendar events")
	cmd.Flags().BoolVar(&o.keepGroups, "keep-groups", false, "Leave group memberships in place")
	cmd.Flags().BoolVar(&o.keepPassword, "keep-password", false, "Do not scramble the password")
	cmd.Flags().BoolVar(&o.noSuspend, "no-suspend", false, "Leave the account active (e.g. mailbox kept for delegation)")
	cmd.Flags().BoolVar(&o.releaseLicense, "release-license", false, "Release the user's licences once no Drive/Calendar transfer from them is pending")
	cmd.Flags().StringVar(&o.products, "products", "Google-Apps", "Licensing product IDs to release, comma-separated")
	return cmd
}

func offboardTenant(email, configPath string) (gwsTenant, error) {
	if strings.TrimSpace(tenantFlag) != "" || getenvTenant() != "" {
		t, err := selectedTenant()
		if err != nil {
			return gwsTenant{}, err
		}
		if t == nil {
			return gwsTenant{}, usageErr(fmt.Errorf("no tenant selected"))
		}
		return *t, nil
	}
	reg, err := loadTenantRegistry()
	if err != nil {
		return gwsTenant{}, configErr(err)
	}
	if t, ok := reg.forEmail(email); ok {
		return t, nil
	}
	if defaultTenantAvailable(configPath, reg) {
		return gwsTenant{Name: "default"}, nil
	}
	return gwsTenant{}, usageErr(fmt.Errorf("no registered tenant owns %s's domain (pass --tenant, or run 'tenants test' to refresh domain lists)", email))
}

func plannedOffboardSteps(o offboardOpts) []offboardStep {
	var s []offboardStep
	add := func(name string, on bool, detail string) {
		st := offboardStep{Step: name, Status: "planned", Detail: detail}
		if !on {
			st.Status, st.Detail = "skipped", "not requested"
		}
		s = append(s, st)
	}
	add("move_ou", o.ou != "", o.ou)
	add("scramble_password", !o.keepPassword, "random 32-char password, not printed")
	add("sign_out", true, "")
	add("revoke_oauth_tokens", true, "")
	add("delete_app_passwords", true, "")
	add("invalidate_backup_codes", true, "")
	add("remove_groups", !o.keepGroups, "")
	add("transfer_drive_calendar", o.successor != "", o.successor)
	add("release_licenses", o.releaseLicense && o.successor == "", "")
	add("suspend", !o.noSuspend, "last, because earlier steps fail on suspended accounts")
	return s
}

// runOffboard performs (or, when !execute, previews) every step, recording
// a receipt per step and continuing past failures so one broken call does
// not leave the rest undone.
func runOffboard(ctx context.Context, c *client.Client, t gwsTenant, email string, o offboardOpts, execute bool, rec *offboardReceipt) error {
	raw, err := c.GetNoCache(ctx, userPath(email), map[string]string{"projection": "basic"})
	if err != nil {
		return fmt.Errorf("looking up %s: %w", email, err)
	}
	var u dirUser
	if err := json.Unmarshal(raw, &u); err != nil || u.ID == "" {
		return fmt.Errorf("looking up %s: unexpected user record", email)
	}
	rec.UserID = u.ID
	// The input may be an alias; licence assignments are keyed by the
	// primary address.
	primary := strings.ToLower(u.PrimaryEmail)
	if primary == "" {
		primary = email
	}
	if u.IsAdmin {
		return usageErr(fmt.Errorf("%s is a super admin; remove the role first (users make-admin %s --status=false)", email, email))
	}
	failed := 0
	step := func(name string, on bool, fn func() (int, string, error)) {
		if !on {
			rec.Steps = append(rec.Steps, offboardStep{Step: name, Status: "skipped", Detail: "not requested"})
			return
		}
		n, detail, err := fn()
		st := offboardStep{Step: name, Status: "done", Detail: detail, Count: n}
		switch {
		case err != nil:
			st.Status, st.Detail = "failed", err.Error()
			failed++
		case !execute:
			st.Status = "planned"
		}
		rec.Steps = append(rec.Steps, st)
	}
	if u.Suspended {
		rec.Next = append(rec.Next, "account was already suspended; backup-code and token steps may fail until it is unsuspended")
	}
	var successor dirUser
	if o.successor != "" {
		if successor, err = resolveSuccessor(ctx, c, o.successor, u.ID); err != nil {
			return err
		}
	}
	uk := userPath(u.ID)

	step("move_ou", o.ou != "", func() (int, string, error) {
		if !execute {
			return 0, u.OrgUnitPath + " -> " + o.ou, nil
		}
		_, _, err := c.Patch(ctx, uk, map[string]any{"orgUnitPath": o.ou})
		return 0, o.ou, err
	})
	step("scramble_password", !o.keepPassword, func() (int, string, error) {
		if !execute {
			return 0, "random password, not printed", nil
		}
		pw, err := randomPassword()
		if err != nil {
			return 0, "", err
		}
		_, _, err = c.Patch(ctx, uk, map[string]any{"password": pw, "changePasswordAtNextLogin": false})
		return 0, "random password set, not printed", err
	})
	step("sign_out", true, func() (int, string, error) {
		if !execute {
			return 0, "", nil
		}
		_, _, err := c.Post(ctx, uk+"/signOut", nil)
		return 0, "", err
	})
	step("revoke_oauth_tokens", true, func() (int, string, error) {
		return deleteEach(ctx, c, uk+"/tokens", "items", "clientId", "displayText", uk+"/tokens/", execute)
	})
	step("delete_app_passwords", true, func() (int, string, error) {
		return deleteEach(ctx, c, uk+"/asps", "items", "codeId", "name", uk+"/asps/", execute)
	})
	step("invalidate_backup_codes", true, func() (int, string, error) {
		if !execute {
			return 0, "", nil
		}
		_, _, err := c.Post(ctx, uk+"/verificationCodes/invalidate", nil)
		return 0, "", err
	})
	step("remove_groups", !o.keepGroups, func() (int, string, error) {
		groups, err := listAll(ctx, c, "/admin/directory/v1/groups", map[string]string{"userKey": u.ID, "maxResults": "200"}, "groups", 0)
		if err != nil {
			return 0, "", err
		}
		var names []string
		for _, g := range groups {
			var grp struct {
				ID    string `json:"id"`
				Email string `json:"email"`
			}
			if err := json.Unmarshal(g, &grp); err != nil {
				return 0, "", err
			}
			names = append(names, grp.Email)
			if execute {
				if _, _, err := c.Delete(ctx, "/admin/directory/v1/groups/"+url.PathEscape(grp.ID)+"/members/"+url.PathEscape(u.ID)); err != nil {
					return len(names) - 1, strings.Join(names, ","), fmt.Errorf("removing from %s: %w", grp.Email, err)
				}
			}
		}
		return len(names), strings.Join(names, ","), nil
	})
	step("transfer_drive_calendar", o.successor != "", func() (int, string, error) {
		id, detail, err := startTransfer(ctx, c, u.ID, successor.ID, execute)
		if err != nil {
			return 0, "", err
		}
		rec.TransferID = id
		if id != "" {
			rec.Next = append(rec.Next, "transfer "+id+" runs asynchronously: check it with 'transfers get "+id+" --tenant "+t.Name+"'")
		}
		return 0, detail + " to " + o.successor, nil
	})
	if o.releaseLicense && o.successor != "" {
		rec.Steps = append(rec.Steps, offboardStep{Step: "release_licenses", Status: "skipped", Detail: "transfer pending; releasing a licence mid-transfer can strand data"})
		rec.Next = append(rec.Next, "after the transfer completes: offboard "+email+" --tenant "+t.Name+" --release-license --keep-groups --yes")
	} else {
		step("release_licenses", o.releaseLicense, func() (int, string, error) {
			id, status, err := pendingTransfer(ctx, c, u.ID)
			if err != nil {
				return 0, "", err
			}
			if id != "" {
				return 0, "", fmt.Errorf("transfer %s is %s; licences not released (re-run after it completes)", id, status)
			}
			return releaseLicenses(ctx, c, t, primary, o.products, execute)
		})
	}
	step("suspend", !o.noSuspend, func() (int, string, error) {
		if u.Suspended {
			return 0, "already suspended", nil
		}
		if !execute {
			return 0, "", nil
		}
		_, _, err := c.Patch(ctx, uk, map[string]any{"suspended": true})
		return 0, "", err
	})
	if failed > 0 {
		return apiErr(fmt.Errorf("%d offboarding step(s) failed; see receipt", failed))
	}
	return nil
}

// deleteEach lists a per-user collection and deletes every item by idKey.
func deleteEach(ctx context.Context, c *client.Client, listPath, itemsKey, idKey, labelKey, deletePrefix string, execute bool) (int, string, error) {
	raw, err := c.GetNoCache(ctx, listPath, nil)
	if err != nil {
		return 0, "", err
	}
	// Google's list envelopes carry string kind/etag next to the items, so
	// decode only the collection.
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, "", fmt.Errorf("decoding %s: %w", listPath, err)
	}
	var items []map[string]json.RawMessage
	if rawItems, ok := env[itemsKey]; ok && string(rawItems) != "null" {
		if err := json.Unmarshal(rawItems, &items); err != nil {
			return 0, "", fmt.Errorf("decoding %s %s: %w", listPath, itemsKey, err)
		}
	}
	var labels []string
	for _, item := range items {
		id := jsonScalar(item[idKey])
		labels = append(labels, jsonScalar(item[labelKey]))
		if execute {
			if _, _, err := c.Delete(ctx, deletePrefix+url.PathEscape(id)); err != nil {
				return len(labels) - 1, strings.Join(labels, ","), err
			}
		}
	}
	return len(labels), strings.Join(labels, ","), nil
}

// jsonScalar renders a JSON string or number without quotes or float
// reformatting (codeId 1234567 must not become 1.234567e+06).
func jsonScalar(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

// pendingTransfer returns the ID and status of any transfer from this user
// that has not completed. Release is refused while one exists, including a
// transfer started by an earlier run.
func pendingTransfer(ctx context.Context, c *client.Client, userID string) (string, string, error) {
	raw, err := listAll(ctx, c, "/admin/datatransfer/v1/transfers", map[string]string{"oldOwnerUserId": userID}, "dataTransfers", 0)
	if err != nil {
		return "", "", fmt.Errorf("checking transfers before releasing licences: %w", err)
	}
	for _, r := range raw {
		var tr struct {
			ID     string `json:"id"`
			Status string `json:"overallTransferStatusCode"`
		}
		if err := json.Unmarshal(r, &tr); err != nil {
			return "", "", err
		}
		if s := strings.ToLower(tr.Status); s != "completed" && s != "failed" {
			return tr.ID, tr.Status, nil
		}
	}
	return "", "", nil
}

// resolveSuccessor looks the successor up before any step runs, so an alias
// of the leaver or a suspended account is refused before anything changes.
func resolveSuccessor(ctx context.Context, c *client.Client, toEmail, fromID string) (dirUser, error) {
	raw, err := c.GetNoCache(ctx, userPath(toEmail), map[string]string{"projection": "basic"})
	if err != nil {
		return dirUser{}, fmt.Errorf("looking up successor %s: %w", toEmail, err)
	}
	var to dirUser
	if err := json.Unmarshal(raw, &to); err != nil || to.ID == "" {
		return dirUser{}, fmt.Errorf("looking up successor %s: unexpected user record", toEmail)
	}
	if to.ID == fromID {
		return dirUser{}, usageErr(fmt.Errorf("--successor %s is the same account as the leaver", toEmail))
	}
	if to.Suspended {
		return dirUser{}, fmt.Errorf("successor %s is suspended", toEmail)
	}
	return to, nil
}

// startTransfer moves Drive (shared+private) and, when the tenant offers it,
// Calendar to the successor. Drive is required; a missing Calendar app is
// reported in the step detail rather than hidden.
func startTransfer(ctx context.Context, c *client.Client, fromID, toID string, execute bool) (string, string, error) {
	apps, err := listAll(ctx, c, "/admin/datatransfer/v1/applications", nil, "applications", 0)
	if err != nil {
		return "", "", fmt.Errorf("listing transferable applications: %w", err)
	}
	var transfers []map[string]any
	hasDrive, hasCalendar := false, false
	for _, a := range apps {
		var app struct {
			ID   json.RawMessage `json:"id"` // int64, may arrive quoted; passed through verbatim
			Name string          `json:"name"`
		}
		if err := json.Unmarshal(a, &app); err != nil {
			return "", "", err
		}
		switch app.Name {
		case "Drive and Docs":
			hasDrive = true
			transfers = append(transfers, map[string]any{"applicationId": app.ID,
				"applicationTransferParams": []map[string]any{{"key": "PRIVACY_LEVEL", "value": []string{"SHARED", "PRIVATE"}}}})
		case "Calendar":
			hasCalendar = true
			transfers = append(transfers, map[string]any{"applicationId": app.ID,
				"applicationTransferParams": []map[string]any{{"key": "RELEASE_RESOURCES", "value": []string{"TRUE"}}}})
		}
	}
	if !hasDrive {
		return "", "", fmt.Errorf("the Drive and Docs transfer application is not available for this tenant")
	}
	detail := "Drive and Calendar"
	if !hasCalendar {
		detail = "Drive only (Calendar transfer application not available)"
	}
	if !execute {
		return "", detail, nil
	}
	body := map[string]any{"oldOwnerUserId": fromID, "newOwnerUserId": toID, "applicationDataTransfers": transfers}
	out, _, err := c.Post(ctx, "/admin/datatransfer/v1/transfers", body)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(out, &resp)
	return resp.ID, detail, nil
}

func releaseLicenses(ctx context.Context, c *client.Client, t gwsTenant, email, products string, execute bool) (int, string, error) {
	custID, err := tenantCustomerID(ctx, c, t)
	if err != nil {
		return 0, "", err
	}
	var released []string
	for _, p := range splitTenantCSV(products) {
		ls, err := listLicenses(ctx, c, custID, p)
		if err != nil {
			return len(released), strings.Join(released, ","), err
		}
		for _, l := range ls {
			if !strings.EqualFold(l.UserID, email) {
				continue
			}
			released = append(released, l.SkuName)
			if execute {
				path := licensingBase + "/product/" + url.PathEscape(l.ProductID) + "/sku/" + url.PathEscape(l.SkuID) + "/user/" + url.PathEscape(email)
				if _, _, err := c.Delete(ctx, path); err != nil {
					return len(released) - 1, strings.Join(released, ","), err
				}
			}
		}
	}
	return len(released), strings.Join(released, ","), nil
}

func printOffboard(cmd *cobra.Command, flags *rootFlags, rec offboardReceipt, runErr error) error {
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "Offboard %s (tenant %s) [%s]\n", rec.User, rec.Tenant, rec.Mode)
		rows := [][]string{}
		for _, s := range rec.Steps {
			rows = append(rows, []string{s.Step, s.Status, s.Detail})
		}
		if err := flags.printTable(cmd, []string{"STEP", "STATUS", "DETAIL"}, rows); err != nil {
			return err
		}
		for _, n := range rec.Next {
			fmt.Fprintln(w, "next: "+n)
		}
	} else if err := printJSONFiltered(cmd.OutOrStdout(), rec, flags); err != nil {
		return err
	}
	return runErr
}
