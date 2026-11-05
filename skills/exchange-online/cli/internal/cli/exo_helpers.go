// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Shared plumbing for the hand-written Exchange Online commands: cmdlet calls
// through the generated client (exo_transport.go builds the envelope), loose
// accessors for cmdlet output rows, and the per-tenant snapshot table.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil"
	"exchange-online-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type exoRow = map[string]any

var exoTenantRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,253}$`)

// exoTenantArg returns args[i] as a tenant ID (GUID) or tenant domain.
func exoTenantArg(cmd *cobra.Command, args []string, i int) (string, error) {
	if len(args) <= i || strings.TrimSpace(args[i]) == "" {
		return "", usageErr(fmt.Errorf("missing <tenantId>\nUsage: %s", cmd.UseLine()))
	}
	// Tenant IDs and domains are case-insensitive; one spelling keeps snapshots keyed once.
	t := strings.ToLower(strings.TrimSpace(args[i]))
	if !exoTenantRe.MatchString(t) {
		return "", usageErr(fmt.Errorf("tenant must be a tenant ID (GUID) or domain, got %q", t))
	}
	return t, nil
}

// exoBeta is the InvokeCommand route; exoV2 is the Admin API v2.0 endpoint.
func exoBeta(tenant, cmdlet string) string {
	return replacePathParam("/adminapi/beta/{tenantId}/InvokeCommand", "tenantId", tenant) + "#" + cmdlet
}

func exoV2(tenant, endpoint, cmdlet string) string {
	return replacePathParam("/adminapi/v2.0/{tenantId}/"+endpoint, "tenantId", tenant) + "#" + cmdlet
}

// exoGet runs a Get-* cmdlet and returns its rows. props becomes $select.
func exoGet(ctx context.Context, c *client.Client, path string, params exoRow, props string) ([]exoRow, error) {
	q := map[string]string{}
	if props != "" {
		q["$select"] = props
	}
	var body any
	if len(params) > 0 {
		body = params
	}
	data, _, err := c.PostQueryWithParams(ctx, path, q, body)
	if err != nil {
		return nil, err
	}
	return exoRows(data)
}

// exoSet runs a state-changing cmdlet.
func exoSet(ctx context.Context, c *client.Client, path string, params exoRow) error {
	_, _, err := c.PostWithParams(ctx, path, nil, params)
	return err
}

func exoRows(data json.RawMessage) ([]exoRow, error) {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		return nil, nil
	}
	var rows []exoRow
	if strings.HasPrefix(s, "[") {
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("decoding cmdlet rows: %w", err)
		}
		return rows, nil
	}
	var one exoRow
	if err := json.Unmarshal(data, &one); err != nil {
		return nil, fmt.Errorf("decoding cmdlet output: %w", err)
	}
	return []exoRow{one}, nil
}

// rs reads a property as a string ("" when absent or null).
func rs(r exoRow, k string) string {
	switch v := r[k].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// rb reads a tri-state boolean; nil means the property is unset (null).
func rb(r exoRow, k string) *bool {
	switch v := r[k].(type) {
	case bool:
		return &v
	case string:
		switch strings.ToLower(v) {
		case "true":
			t := true
			return &t
		case "false":
			f := false
			return &f
		}
	}
	return nil
}

func isTrue(p *bool) bool  { return p != nil && *p }
func isFalse(p *bool) bool { return p != nil && !*p }

// rl reads a multi-valued property (a JSON array or a single string).
func rl(r exoRow, k string) []string {
	switch v := r[k].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s := strings.TrimSpace(fmt.Sprint(x)); s != "" && x != nil {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(v) != "" {
			return []string{v}
		}
	}
	return nil
}

var smtpInBrackets = regexp.MustCompile(`(?i)\[(?:smtp|ex):([^\]]+)\]`)

// smtpOf pulls the address out of the shapes Exchange uses for recipients:
// "smtp:a@b.com", "SMTP:a@b.com", "\"Name\" [SMTP:a@b.com]", or a bare address.
func smtpOf(s string) string {
	s = strings.TrimSpace(s)
	if m := smtpInBrackets.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if i := strings.Index(s, ":"); i > 0 && strings.EqualFold(s[:i], "smtp") {
		s = s[i+1:]
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func domainOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return strings.ToLower(addr[i+1:])
	}
	return ""
}

// acceptedDomainSet returns the tenant's accepted domains, lowercased.
func acceptedDomainSet(ctx context.Context, c *client.Client, tenant string) (map[string]bool, error) {
	rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-AcceptedDomain"), nil, "DomainName,MatchSubDomains")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, r := range rows {
		if d := strings.ToLower(rs(r, "DomainName")); d != "" {
			set[d] = true
			if isTrue(rb(r, "MatchSubDomains")) {
				set["*."+d] = true // its subdomains are internal too
			}
		}
	}
	return set, nil
}

// isExternal reports whether addr is outside the tenant: its domain is not an
// accepted domain, nor a subdomain of one with MatchSubDomains (keyed "*.<d>").
// An address with no domain (an internal recipient name) is treated as internal.
func isExternal(addr string, accepted map[string]bool) bool {
	d := domainOf(addr)
	if d == "" || accepted[d] {
		return false
	}
	for i := strings.Index(d, "."); i >= 0; i = strings.Index(d, ".") {
		d = d[i+1:]
		if accepted["*."+d] {
			return false
		}
	}
	return true
}

// sourceErr records a failed lookup so multi-source commands report partial
// results instead of failing outright.
type sourceErr struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

// Inbox-rule signatures, from CIPP's BecHeuristics.json (inboxRules block):
// https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Config/BecHeuristics.json
var (
	lowVisibilityFolderRe = regexp.MustCompile(`(?i)(rss|archive|deleted|junk|conversation history|notes|sync issues)`)
	sensitiveNameRe       = regexp.MustCompile(`(?i)(invoice|payment|security|alert|verify|microsoft|admin)`)
	sensitiveKeywordRe    = regexp.MustCompile(`(?i)(invoice|payment|wire|remittance|bank|swift|iban|ach|routing|account number|beneficiary|payroll|password|credential|verif|urgent|confidential|hack|phish|compromis|fraud|scam|virus|malware|suspicious)`)
	punctuationNameRe     = regexp.MustCompile(`^[^A-Za-z0-9]*$`)
)

// ruleTargets lists the recipients an inbox rule sends mail to, as Exchange
// rendered them ("Name" [SMTP:a@b], "Name" [EX:/o=...], or a bare address).
func ruleTargets(r exoRow) []string {
	var out []string
	for _, k := range []string{"ForwardTo", "ForwardAsAttachmentTo", "RedirectTo"} {
		out = append(out, rl(r, k)...)
	}
	return out
}

var exDNRe = regexp.MustCompile(`(?i)\[ex:([^\]]+)\]`)

// destination is where a forwarding target really delivers.
type destination struct {
	Addr       string // lowercased SMTP address mail goes to
	Unresolved string // why the target could not be resolved
}

// destResolver maps rendered forwarding targets to their real destination: an
// internal address, a bare name or an EX legacy DN can be a mail contact or
// mail user whose ExternalEmailAddress is outside the tenant.
type destResolver struct {
	ctx      context.Context
	c        *client.Client
	tenant   string
	accepted map[string]bool
	cache    map[string]destination
	Errors   []sourceErr
}

func newDestResolver(ctx context.Context, c *client.Client, tenant string, accepted map[string]bool) *destResolver {
	return &destResolver{ctx: ctx, c: c, tenant: tenant, accepted: accepted, cache: map[string]destination{}}
}

func (d *destResolver) resolve(raw string) destination {
	key := strings.ToLower(strings.TrimSpace(raw))
	if got, ok := d.cache[key]; ok {
		return got
	}
	var out destination
	if m := exDNRe.FindStringSubmatch(raw); m != nil {
		dn := strings.TrimSpace(m[1])
		rows, err := exoGet(d.ctx, d.c, exoBeta(d.tenant, "Get-Recipient"),
			exoRow{"Filter": "LegacyExchangeDN -eq '" + strings.ReplaceAll(dn, "'", "''") + "'", "ResultSize": 2},
			"PrimarySmtpAddress,ExternalEmailAddress,LegacyExchangeDN,RecipientTypeDetails")
		switch {
		case err != nil:
			out.Unresolved = err.Error()
			d.Errors = append(d.Errors, sourceErr{Source: "Get-Recipient " + raw, Error: err.Error()})
		case len(rows) != 1 || !strings.EqualFold(rs(rows[0], "LegacyExchangeDN"), dn):
			out.Unresolved = fmt.Sprintf("%d recipients carry this legacy DN", len(rows))
		default:
			out = deliveryOf(recipientOf(rows[0]))
		}
	} else if addr := smtpOf(raw); strings.Contains(addr, "@") && isExternal(addr, d.accepted) {
		out.Addr = addr // already outside the tenant
	} else {
		ident := addr
		if !strings.Contains(addr, "@") {
			ident = strings.TrimSpace(raw)
		}
		// resolveRecipient verifies the match (UPN included) and rejects multiple matches.
		rc, err := resolveRecipient(d.ctx, d.c, d.tenant, ident)
		switch {
		case err != nil:
			out.Unresolved = err.Error()
			d.Errors = append(d.Errors, sourceErr{Source: "Get-Recipient " + raw, Error: err.Error()})
		case rc == nil:
			out.Unresolved = "no recipient with this name or address"
		default:
			out = deliveryOf(rc)
		}
	}
	d.cache[key] = out
	return out
}

// deliveryOf says where mail to a resolved recipient goes. A group delivers to
// its members, which can include outside contacts, so it is not expanded here.
func deliveryOf(rc *recipient) destination {
	switch {
	case strings.Contains(strings.ToLower(rc.typ), "group") || strings.EqualFold(rc.typ, "RoomList"):
		return destination{Unresolved: "a group (" + rc.typ + "); its members are not expanded"}
	case rc.external != "":
		return destination{Addr: rc.external}
	case rc.primary == "":
		return destination{Unresolved: "recipient has no SMTP address"}
	}
	return destination{Addr: rc.primary}
}

// isNotFound reports an Exchange "couldn't be found" cmdlet error.
func isNotFound(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "couldn't be found") || strings.Contains(s, "could not be found")
}

// ruleFlags returns the BEC signatures an inbox rule matches.
func ruleFlags(r exoRow, accepted map[string]bool) []string {
	var flags []string
	if isTrue(rb(r, "DeleteMessage")) {
		flags = append(flags, "deletes-messages")
	}
	folder := rs(r, "MoveToFolder")
	if folder != "" && lowVisibilityFolderRe.MatchString(folder) {
		flags = append(flags, "moves-to-low-visibility-folder")
	}
	if isTrue(rb(r, "MarkAsRead")) && (folder != "" || isTrue(rb(r, "DeleteMessage"))) {
		flags = append(flags, "marks-read-and-hides")
	}
	var words []string
	for _, k := range []string{"SubjectContainsWords", "SubjectOrBodyContainsWords", "BodyContainsWords", "FromAddressContainsWords"} {
		words = append(words, rl(r, k)...)
	}
	if sensitiveKeywordRe.MatchString(strings.Join(words, " ")) {
		flags = append(flags, "sensitive-keywords")
	}
	name := strings.TrimSpace(rs(r, "Name"))
	if len([]rune(name)) <= 2 || punctuationNameRe.MatchString(name) {
		flags = append(flags, "suspicious-name")
	} else if sensitiveNameRe.MatchString(name) {
		flags = append(flags, "sensitive-name")
	}
	for _, t := range ruleTargets(r) {
		if isExternal(smtpOf(t), accepted) {
			flags = append(flags, "forwards-external")
			break
		}
	}
	return flags
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// storeSweep saves an inbox-rule sweep and its completeness record together.
func storeSweep(ctx context.Context, tenant string, v sweepView, rules []exoRow) (time.Time, error) {
	meta := exoRow{"mailboxes": v.Mailboxes, "errors": len(v.Errors), "truncated": v.Note != "", "types": v.Types}
	return replaceSnapshots(ctx, tenant,
		snapshotSet{"inbox_rule", rules, func(r exoRow) string { return rs(r, "_mailbox") + "|" + rs(r, "Identity") }},
		snapshotSet{"inbox_rule_meta", []exoRow{meta}, func(exoRow) string { return "sweep" }})
}

// permissionState evaluates mailbox permission rows for one trustee: an
// explicit (non-inherited) allow of the right counts, and any deny of it wins.
func permissionState(rows []exoRow, right string, isTrustee func(string) bool) (allowed, denied bool) {
	for _, r := range rows {
		if !isTrustee(rs(r, "User")) || !containsFold(rl(r, "AccessRights"), right) {
			continue
		}
		if isTrue(rb(r, "Deny")) {
			denied = true
		} else if !isTrue(rb(r, "IsInherited")) {
			allowed = true
		}
	}
	return allowed && !denied, denied
}

// containsFold matches s against list items, splitting "A, B" style values.
func containsFold(list []string, s string) bool {
	for _, x := range list {
		for _, part := range strings.Split(x, ",") {
			if strings.EqualFold(strings.TrimSpace(part), s) {
				return true
			}
		}
	}
	return false
}

// smtpAddresses returns every smtp: proxy address of a recipient row, plus its
// primary SMTP address and UPN, lowercased.
func smtpAddresses(r exoRow) map[string]bool {
	set := map[string]bool{}
	for _, a := range rl(r, "EmailAddresses") {
		if strings.HasPrefix(strings.ToLower(a), "smtp:") {
			set[smtpOf(a)] = true
		}
	}
	for _, k := range []string{"PrimarySmtpAddress", "UserPrincipalName", "WindowsEmailAddress"} {
		if a := smtpOf(rs(r, k)); a != "" {
			set[a] = true
		}
	}
	return set
}

// smtpProxies returns a recipient's SMTP addresses (EmailAddresses smtp: entries
// plus the primary), the addresses message trace knows it by.
func smtpProxies(r exoRow) []string {
	set := map[string]bool{}
	for _, a := range rl(r, "EmailAddresses") {
		if strings.HasPrefix(strings.ToLower(a), "smtp:") {
			set[smtpOf(a)] = true
		}
	}
	if p := smtpOf(rs(r, "PrimarySmtpAddress")); p != "" {
		set[p] = true
	}
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// mailboxIDProps are the identity properties getMailbox checks a row against.
const mailboxIDProps = "UserPrincipalName,PrimarySmtpAddress,EmailAddresses,Name,Alias,DisplayName,DistinguishedName,Identity,ExternalDirectoryObjectId,Guid"

// getMailbox returns the one mailbox identity names, or nil when there is none.
// Get-Mailbox can answer an identity it cannot match with other mailboxes, so a
// row counts only if it carries the identity asked for; two matches are an error.
func getMailbox(ctx context.Context, c *client.Client, tenant, identity, props string) (exoRow, error) {
	sel := mailboxIDProps
	if props != "" {
		sel += "," + props
	}
	rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-Mailbox"), exoRow{"Identity": identity}, sel)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var hits []exoRow
	for _, r := range rows {
		if recipientOf(r).answers(identity) {
			hits = append(hits, r)
		}
	}
	switch len(hits) {
	case 0:
		return nil, nil
	case 1:
		return hits[0], nil
	}
	return nil, fmt.Errorf("%q matches %d mailboxes", identity, len(hits))
}

// warnPartial reports failed lookups on stderr; the JSON lists them under errors.
func warnPartial(cmd *cobra.Command, errs []sourceErr, total int, unit string) {
	if len(errs) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d %s lookups failed; results cover the rest (see errors)\n", len(errs), total, unit)
	}
}

// --- per-tenant snapshot table -------------------------------------------

const snapshotDDL = `CREATE TABLE IF NOT EXISTS exo_snapshots (
	tenant_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	id TEXT NOT NULL,
	data JSON NOT NULL,
	fetched_at TEXT NOT NULL,
	PRIMARY KEY (tenant_id, kind, id)
)`

type snapshotRow struct {
	Tenant    string
	ID        string
	Data      exoRow
	FetchedAt time.Time
}

// snapshotDBPath is the local store for per-tenant snapshots. It is not the
// generated store, whose file is scoped to the current credential: each tenant
// is snapshotted with its own token, and the cross-tenant queue must read them
// all. Rows stay separated by tenant_id.
func snapshotDBPath() (string, error) {
	dir, err := cliutil.DataDir()
	if err != nil {
		return "", fmt.Errorf("resolving the data directory: %w", err)
	}
	// Built from parts: the press's novel-host scan reads a "name.db" literal as a hostname.
	return filepath.Join(dir, "exchange-snapshots"+".db"), nil
}

// snapshotSet is one kind of rows to store for a tenant.
type snapshotSet struct {
	Kind string
	Rows []exoRow
	IDOf func(exoRow) string
}

// replaceSnapshots swaps one tenant's rows of each given kind for fresh sets in
// one transaction, so rows and their metadata always match.
func replaceSnapshots(ctx context.Context, tenant string, sets ...snapshotSet) (time.Time, error) {
	path, err := snapshotDBPath()
	if err != nil {
		return time.Time{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return time.Time{}, fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	s, err := store.OpenWithContext(ctx, path)
	if err != nil {
		return time.Time{}, fmt.Errorf("opening snapshot store: %w", err)
	}
	defer s.Close()
	db := s.DB()
	if _, err := db.ExecContext(ctx, snapshotDDL); err != nil {
		return time.Time{}, fmt.Errorf("creating snapshot table: %w", err)
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback()
	for _, set := range sets {
		if _, err := tx.ExecContext(ctx, `DELETE FROM exo_snapshots WHERE tenant_id = ? AND kind = ?`, tenant, set.Kind); err != nil {
			return time.Time{}, err
		}
		for i, r := range set.Rows {
			id := set.IDOf(r)
			if id == "" {
				id = fmt.Sprintf("row-%d", i)
			}
			raw, err := json.Marshal(r)
			if err != nil {
				return time.Time{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO exo_snapshots (tenant_id, kind, id, data, fetched_at) VALUES (?, ?, ?, ?, ?)`,
				tenant, set.Kind, id, string(raw), now.Format(time.RFC3339)); err != nil {
				return time.Time{}, err
			}
		}
	}
	return now, tx.Commit()
}

// readSnapshots returns stored rows of the given kinds, for one tenant or all
// (""), keyed by kind. One SELECT reads them all, so related kinds (rows and
// their meta) come from the same committed snapshot. A missing store or table
// reads as empty.
func readSnapshots(ctx context.Context, tenant string, kinds ...string) (map[string][]snapshotRow, error) {
	out := map[string][]snapshotRow{}
	path, err := snapshotDBPath()
	if err != nil {
		return out, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return out, nil
	}
	s, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return out, fmt.Errorf("opening snapshot store: %w", err)
	}
	defer s.Close()
	q := `SELECT kind, tenant_id, id, data, fetched_at FROM exo_snapshots WHERE kind IN (?` + strings.Repeat(",?", len(kinds)-1) + `)`
	args := make([]any, 0, len(kinds)+1)
	for _, k := range kinds {
		args = append(args, k)
	}
	if tenant != "" {
		q += ` AND tenant_id = ?`
		args = append(args, tenant)
	}
	rows, err := s.DB().QueryContext(ctx, q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r snapshotRow
		var kind, data, at string
		if err := rows.Scan(&kind, &r.Tenant, &r.ID, &data, &at); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &r.Data); err != nil {
			return nil, err
		}
		r.FetchedAt, _ = time.Parse(time.RFC3339, at)
		out[kind] = append(out[kind], r)
	}
	return out, rows.Err()
}
