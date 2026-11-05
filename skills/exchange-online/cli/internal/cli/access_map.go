// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type accessEntry struct {
	Mailbox string   `json:"mailbox"`
	Access  []string `json:"access"`
}

type accessMapView struct {
	Tenant    string        `json:"tenant"`
	User      string        `json:"user"`
	Access    []accessEntry `json:"access"`
	Ambiguous []accessEntry `json:"ambiguous"`
	Scanned   int           `json:"scanned_mailboxes"`
	Total     int           `json:"total_mailboxes"`
	Note      string        `json:"note,omitempty"`
	Skipped   []string      `json:"skipped,omitempty"`
	Errors    []sourceErr   `json:"errors"`
}

func newNovelAccessMapCmd(flags *rootFlags) *cobra.Command {
	var calendars, skipFull bool
	var concurrency, maxMailboxes int
	cmd := &cobra.Command{
		Use:   "access-map <tenantId> <user>",
		Short: "List every mailbox one user can open, send as, or send on behalf of.",
		Long: `Resolves the user with Get-Recipient, then checks every mailbox for the user's access:
Send As (one tenant-wide Get-RecipientPermission -Trustee call), FullAccess (Get-MailboxPermission
-User per mailbox; skip with --skip-full-access on large tenants), Send on behalf (each mailbox's
GrantSendOnBehalfTo) and, with --calendars, rights on each mailbox's calendar folder (found by
folder type, so localized names work). Exchange filters the first two by the user itself; the last two
name trustees by display text, so each candidate name is resolved to a recipient and compared by
object ID. Inherited entries are ignored and a deny cancels an allow. On a calendar, the user's own
entry wins and otherwise the Default entry applies. A name that matches more than one recipient is
listed under ambiguous instead of access. Access through group membership is not expanded.

Use this command to list every mailbox ONE user can open (FullAccess), send as, send on behalf of,
or edit calendars in. Do NOT use this command to list who has access to one mailbox; use
'mailbox-permissions list' or 'send-as list' instead. Do NOT use this command for compromise
evidence; use 'bec triage' instead.`,
		Example:     "  exchange-online-cli access-map 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --agent\n  exchange-online-cli access-map 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --calendars --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "tenantId=0550b473-9fd2-4dbb-a058-a1640b4bf458;user=jane@example.com"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "access-map")
			}
			if concurrency < 1 || concurrency > 16 || maxMailboxes < 1 {
				return usageErr(fmt.Errorf("--concurrency must be 1-16 and --max-mailboxes at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxMailboxes > 10 {
				maxMailboxes = 10
			}
			tenant, err := exoTenantArg(cmd, args, 0)
			if err != nil {
				return err
			}
			if len(args) < 2 || !strings.Contains(args[1], "@") {
				return usageErr(fmt.Errorf("missing <user> (a UPN or email address)\nUsage: %s", cmd.UseLine()))
			}
			user := strings.ToLower(strings.TrimSpace(args[1]))
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			v := accessMapView{Tenant: tenant, User: user, Access: []accessEntry{}, Ambiguous: []accessEntry{}, Errors: []sourceErr{}}
			me, err := resolveRecipient(ctx, c, tenant, user)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if me == nil {
				return notFoundErr(fmt.Errorf("recipient %s not found", user))
			}
			tr := newTrusteeResolver(ctx, c, tenant, me, user)

			grants, maybe := map[string]map[string]bool{}, map[string]map[string]bool{}
			var mu sync.Mutex
			put := func(m map[string]map[string]bool, mbx, what string) {
				if m[mbx] == nil {
					m[mbx] = map[string]bool{}
				}
				m[mbx][what] = true
			}
			// named files access a cmdlet named by display text: confirmed, ambiguous or dropped.
			named := func(trustee, mbx, what string) {
				res := tr.match(trustee) // the resolver is goroutine-safe; the maps are not
				mu.Lock()
				defer mu.Unlock()
				switch res {
				case matchYes:
					put(grants, mbx, what)
				case matchAmbiguous:
					put(maybe, mbx, what)
				}
			}

			mbx, err := exoGet(ctx, c, exoBeta(tenant, "Get-Mailbox"), exoRow{"ResultSize": "Unlimited"}, "Name,Identity,UserPrincipalName,PrimarySmtpAddress,GrantSendOnBehalfTo")
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			var targets []string
			for _, m := range mbx {
				addr := smtpOf(firstNonEmpty(rs(m, "PrimarySmtpAddress"), rs(m, "UserPrincipalName")))
				if addr == "" || me.addrs[addr] {
					continue
				}
				targets = append(targets, addr)
				for _, d := range rl(m, "GrantSendOnBehalfTo") {
					named(d, addr, "SendOnBehalf")
				}
			}
			v.Total = len(targets)
			if len(targets) > maxMailboxes {
				targets = targets[:maxMailboxes]
				v.Note = fmt.Sprintf("checked FullAccess and calendars on the first %d of %d mailboxes; raise --max-mailboxes to cover the rest", maxMailboxes, v.Total)
			}
			v.Scanned = len(targets)
			// Exchange filters by the trustee itself, so every row returned is the user's.
			if sa, err := exoGet(ctx, c, exoBeta(tenant, "Get-RecipientPermission"), exoRow{"Trustee": user, "ResultSize": "Unlimited"}, "Identity,AccessRights,AccessControlType,IsInherited"); err != nil {
				v.Errors = append(v.Errors, sourceErr{Source: "Get-RecipientPermission", Error: err.Error()})
			} else {
				allow, deny := map[string]bool{}, map[string]bool{}
				for _, r := range sa {
					if !containsFold(rl(r, "AccessRights"), "SendAs") {
						continue
					}
					id := resolveIdentity(rs(r, "Identity"), mbx)
					if strings.EqualFold(rs(r, "AccessControlType"), "Deny") {
						deny[id] = true
					} else if !isTrue(rb(r, "IsInherited")) {
						allow[id] = true
					}
				}
				for m := range allow {
					if !deny[m] {
						put(grants, m, "SendAs")
					}
				}
			}
			fanOut(targets, concurrency, func(addr string) {
				if !skipFull {
					rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-MailboxPermission"), exoRow{"Identity": addr, "User": user}, "User,AccessRights,IsInherited,Deny")
					mu.Lock()
					if err != nil {
						v.Errors = append(v.Errors, sourceErr{Source: "Get-MailboxPermission " + addr, Error: err.Error()})
					} else if allowed, _ := permissionState(rows, "FullAccess", func(string) bool { return true }); allowed {
						put(grants, addr, "FullAccess")
					}
					mu.Unlock()
				}
				if calendars {
					// The calendar folder name is localized; find it by FolderType the way
					// CIPP's Push-GetCalendarPermissionsBatch.ps1 does, then read its permissions.
					folder := ""
					fs, err := exoGet(ctx, c, exoBeta(tenant, "Get-MailboxFolderStatistics"), exoRow{"Identity": addr, "FolderScope": "Calendar"}, "Name,FolderType")
					for _, f := range fs {
						if strings.EqualFold(rs(f, "FolderType"), "Calendar") && rs(f, "Name") != "" {
							folder = addr + ":\\" + rs(f, "Name")
							break
						}
					}
					if folder == "" { // never guess the folder name: a user folder can be called Calendar
						if err == nil {
							err = fmt.Errorf("no folder of type Calendar found")
						}
						mu.Lock()
						v.Errors = append(v.Errors, sourceErr{Source: "Get-MailboxFolderStatistics " + addr, Error: err.Error()})
						mu.Unlock()
						return
					}
					rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-MailboxFolderPermission"), exoRow{"Identity": folder}, "")
					if err != nil {
						mu.Lock()
						v.Errors = append(v.Errors, sourceErr{Source: "Get-MailboxFolderPermission " + addr, Error: err.Error()})
						mu.Unlock()
					}
					// An explicit entry for the user wins (even None); without one, the
					// Default entry applies to every signed-in user.
					var explicit, unsure, def []string
					hasExplicit := false
					for _, r := range rows {
						u := strings.TrimSpace(rs(r, "User"))
						switch {
						case strings.EqualFold(u, "Default"):
							def = rl(r, "AccessRights")
						case strings.EqualFold(u, "Anonymous"):
						default:
							switch tr.match(u) {
							case matchYes:
								hasExplicit = true
								explicit = append(explicit, rl(r, "AccessRights")...)
							case matchAmbiguous:
								unsure = append(unsure, rl(r, "AccessRights")...)
							}
						}
					}
					mu.Lock()
					switch {
					case hasExplicit:
						for _, ar := range calendarRights(explicit) {
							put(grants, addr, "Calendar:"+ar)
						}
					case len(unsure) > 0: // an entry that may be the user overrides Default, or not
						for _, ar := range calendarRights(append(unsure, def...)) {
							put(maybe, addr, "Calendar:"+ar)
						}
					default:
						for _, ar := range calendarRights(def) {
							put(grants, addr, "Calendar:"+ar+" (Default)")
						}
					}
					mu.Unlock()
				}
			})
			v.Errors = append(v.Errors, tr.errors()...)
			warnPartial(cmd, v.Errors, v.Scanned, "mailbox")
			if skipFull {
				v.Skipped = append(v.Skipped, "FullAccess (--skip-full-access)")
			}
			if !calendars {
				v.Skipped = append(v.Skipped, "Calendar rights (pass --calendars)")
			}
			v.Access, v.Ambiguous = accessEntries(grants), accessEntries(maybe)
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}
	cmd.Flags().BoolVar(&calendars, "calendars", false, "Also check Calendar folder rights in every mailbox (Admin API v2.0)")
	cmd.Flags().BoolVar(&skipFull, "skip-full-access", false, "Skip the per-mailbox FullAccess check (one call per mailbox)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Mailboxes queried in parallel (1-16)")
	cmd.Flags().IntVar(&maxMailboxes, "max-mailboxes", 2000, "Most mailboxes to check one by one; the rest are reported in note")
	return cmd
}

// recipient identifies one Exchange recipient.
type recipient struct {
	ids   map[string]bool // object IDs: ExternalDirectoryObjectId, Guid, primary SMTP
	addrs map[string]bool // every SMTP address
	names map[string]bool // Name, Alias, DisplayName (not unique on their own)
	keys  map[string]bool // DistinguishedName, Identity
	// external is a mail contact's or mail user's outside address, if any.
	external string
	typ      string // RecipientTypeDetails
	primary  string // primary SMTP address
}

// resolveRecipient returns the one recipient identity names, with its UPN when
// it is a mailbox. No match returns nil; more than one match is an error (an
// unmatched Get-Recipient -Identity can list many recipients).
func resolveRecipient(ctx context.Context, c *client.Client, tenant, identity string) (*recipient, error) {
	rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-Recipient"), exoRow{"Identity": identity},
		"Name,Alias,DisplayName,PrimarySmtpAddress,EmailAddresses,ExternalDirectoryObjectId,Guid,RecipientTypeDetails,ExternalEmailAddress,DistinguishedName,Identity")
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	switch len(rows) {
	case 0:
		return nil, nil
	case 1:
	default:
		return nil, fmt.Errorf("%q matches %d recipients", identity, len(rows))
	}
	rc := recipientOf(rows[0])
	if strings.HasSuffix(strings.ToLower(rc.typ), "mailbox") {
		if m, err := getMailbox(ctx, c, tenant, rc.primary, ""); err == nil && m != nil {
			if upn := smtpOf(rs(m, "UserPrincipalName")); upn != "" {
				rc.addrs[upn] = true
			}
		}
	}
	// An identity Exchange cannot match can come back as an unrelated recipient;
	// accept the row only if it really carries the identity asked for.
	if !rc.answers(identity) {
		return nil, nil
	}
	return rc, nil
}

// answers reports whether identity is one of this recipient's identifiers.
func (r *recipient) answers(identity string) bool {
	id := strings.ToLower(strings.TrimSpace(identity))
	if r.ids[id] || r.names[id] || r.keys[id] {
		return true // exact object ID, name, alias, DN or Identity (a DN can contain @)
	}
	return strings.Contains(id, "@") && r.addrs[smtpOf(id)]
}

func recipientOf(r exoRow) *recipient {
	rc := &recipient{ids: map[string]bool{}, addrs: smtpAddresses(r), names: map[string]bool{}, keys: map[string]bool{},
		external: smtpOf(rs(r, "ExternalEmailAddress")), typ: rs(r, "RecipientTypeDetails")}
	for _, k := range []string{"DistinguishedName", "Identity"} {
		if s := strings.ToLower(strings.TrimSpace(rs(r, k))); s != "" {
			rc.keys[s] = true
		}
	}
	for _, k := range []string{"ExternalDirectoryObjectId", "Guid"} {
		if s := strings.ToLower(rs(r, k)); s != "" {
			rc.ids[s] = true
		}
	}
	if p := smtpOf(rs(r, "PrimarySmtpAddress")); p != "" {
		rc.ids[p] = true
		rc.primary = p
	}
	for _, k := range []string{"Name", "Alias", "DisplayName"} {
		if s := strings.ToLower(strings.TrimSpace(rs(r, k))); s != "" {
			rc.names[s] = true
		}
	}
	return rc
}

func (r *recipient) same(o *recipient) bool {
	for id := range o.ids {
		if r.ids[id] {
			return true
		}
	}
	return false
}

type trusteeMatch int

const (
	matchNo trusteeMatch = iota
	matchYes
	matchAmbiguous
)

// trusteeResolver decides whether a trustee string a cmdlet rendered (an
// address, a name, an alias or a display name) is the user, resolving each
// candidate name once.
type trusteeResolver struct {
	ctx    context.Context
	c      *client.Client
	tenant string
	me     *recipient
	mu     sync.Mutex
	cache  map[string]trusteeMatch
	errs   []sourceErr
}

func newTrusteeResolver(ctx context.Context, c *client.Client, tenant string, me *recipient, user string) *trusteeResolver {
	me.addrs[user] = true
	return &trusteeResolver{ctx: ctx, c: c, tenant: tenant, me: me, cache: map[string]trusteeMatch{}}
}

func (t *trusteeResolver) errors() []sourceErr {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.errs
}

func (t *trusteeResolver) match(trustee string) trusteeMatch {
	s := strings.ToLower(strings.TrimSpace(trustee))
	if s == "" {
		return matchNo
	}
	// Only text equal to one of the user's identifiers can be the user: an
	// address, a name, or an object identifier (Exchange Online renders
	// GrantSendOnBehalfTo as GUIDs unless asked for display names; a DN or
	// Identity is possible too). recipient.answers is the one check for every
	// form. Each candidate is still checked for another recipient with that
	// display name.
	if !t.me.addrs[smtpOf(s)] && !t.me.answers(s) {
		return matchNo
	}
	t.mu.Lock()
	m, ok := t.cache[s]
	t.mu.Unlock()
	if ok {
		return m
	}
	m = t.resolve(strings.TrimSpace(trustee))
	t.mu.Lock()
	t.cache[s] = m
	t.mu.Unlock()
	return m
}

// resolve decides what rendered trustee text refers to. It is the user only
// when every recipient the text can name (by identity, and by display name) is
// the user; if it can also name someone else, it is ambiguous.
func (t *trusteeResolver) resolve(name string) trusteeMatch {
	var found []*recipient
	ambiguous := false
	rc, err := resolveRecipient(t.ctx, t.c, t.tenant, name)
	switch {
	case err != nil && strings.Contains(err.Error(), "matches"), err != nil && strings.Contains(strings.ToLower(err.Error()), "multiple"):
		ambiguous = true
	case err != nil:
		t.fail(name, err)
		return matchAmbiguous
	case rc != nil:
		found = append(found, rc)
	}
	same, err := exoGet(t.ctx, t.c, exoBeta(t.tenant, "Get-Recipient"),
		exoRow{"Filter": "DisplayName -eq '" + strings.ReplaceAll(name, "'", "''") + "'", "ResultSize": "Unlimited"}, "PrimarySmtpAddress,ExternalDirectoryObjectId,Guid")
	if err != nil {
		t.fail(name, err)
		return matchAmbiguous
	}
	for _, r := range same {
		found = append(found, recipientOf(r))
	}
	mine, other := false, false
	for _, f := range found {
		if t.me.same(f) {
			mine = true
		} else {
			other = true
		}
	}
	switch {
	case mine && !other && !ambiguous:
		return matchYes
	case mine || ambiguous:
		return matchAmbiguous
	}
	return matchNo
}

func (t *trusteeResolver) fail(name string, err error) {
	t.mu.Lock()
	t.errs = append(t.errs, sourceErr{Source: "Get-Recipient " + name, Error: err.Error()})
	t.mu.Unlock()
}

// calendarRights drops the rights that grant no real access to items.
func calendarRights(rights []string) []string {
	var out []string
	for _, ar := range rights {
		if ar = strings.TrimSpace(ar); ar != "" && !strings.EqualFold(ar, "None") && !strings.EqualFold(ar, "AvailabilityOnly") && !strings.EqualFold(ar, "LimitedDetails") {
			out = append(out, ar)
		}
	}
	return out
}

func accessEntries(m map[string]map[string]bool) []accessEntry {
	out := []accessEntry{}
	for addr, g := range m {
		e := accessEntry{Mailbox: addr}
		for k := range g {
			e.Access = append(e.Access, k)
		}
		sort.Strings(e.Access)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mailbox < out[j].Mailbox })
	return out
}

// resolveIdentity maps a permission row's Identity (often the mailbox name) to
// the mailbox's address when one of the listed mailboxes matches it.
func resolveIdentity(id string, mbx []exoRow) string {
	l := strings.ToLower(strings.TrimSpace(id))
	for _, m := range mbx {
		addr := smtpOf(firstNonEmpty(rs(m, "PrimarySmtpAddress"), rs(m, "UserPrincipalName")))
		if l == addr || l == strings.ToLower(rs(m, "UserPrincipalName")) || l == strings.ToLower(rs(m, "Name")) || l == strings.ToLower(rs(m, "Identity")) {
			return addr
		}
	}
	return id
}

// fanOut runs fn over items with at most n in flight.
func fanOut(items []string, n int, fn func(string)) {
	sem := make(chan struct{}, n)
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(s string) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(s)
		}(it)
	}
	wg.Wait()
}
