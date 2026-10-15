// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (not generated): shared Duo Admin API helpers for the novel
// commands (envelope unwrap, user lookup, offset paging, subaccount fan-out).

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cisco-duo-pp-cli/internal/client"
	"cisco-duo-pp-cli/internal/cliutil"
)

// duoEnvelope is Duo's {stat, response, metadata} wrapper; failures carry
// code/message/message_detail.
type duoEnvelope struct {
	Stat          string          `json:"stat"`
	Code          int             `json:"code"`
	Message       string          `json:"message"`
	MessageDetail string          `json:"message_detail"`
	Response      json.RawMessage `json:"response"`
	Metadata      json.RawMessage `json:"metadata"`
}

// duoUnwrap returns the response member of a Duo envelope. Bodies without a
// stat key (already-unwrapped fixtures) pass through unchanged.
func duoUnwrap(raw json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	var env duoEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Stat == "" {
		return raw, nil, nil
	}
	if !strings.EqualFold(env.Stat, "OK") {
		msg := env.Message
		if env.MessageDetail != "" {
			msg += " (" + env.MessageDetail + ")"
		}
		return nil, nil, apiErr(fmt.Errorf("duo error %d: %s", env.Code, msg))
	}
	return env.Response, env.Metadata, nil
}

type duoGroup struct {
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
}

type duoPhoneUser struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type duoPhone struct {
	PhoneID      string         `json:"phone_id"`
	Number       string         `json:"number"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Platform     string         `json:"platform"`
	Model        string         `json:"model"`
	Activated    bool           `json:"activated"`
	Quarantined  bool           `json:"quarantined"`
	Capabilities []string       `json:"capabilities"`
	LastSeen     flexTime       `json:"last_seen"`
	Users        []duoPhoneUser `json:"users,omitempty"`
}

type duoUser struct {
	UserID        string     `json:"user_id"`
	Username      string     `json:"username"`
	Realname      string     `json:"realname"`
	Email         string     `json:"email"`
	Status        string     `json:"status"`
	LockoutReason *string    `json:"lockout_reason"`
	IsEnrolled    bool       `json:"is_enrolled"`
	HasPassword   bool       `json:"has_password"`
	Created       int64      `json:"created"`
	LastLogin     *int64     `json:"last_login"`
	Groups        []duoGroup `json:"groups"`
	Phones        []duoPhone `json:"phones"`
}

func (u duoUser) lockoutReason() string {
	if u.LockoutReason == nil {
		return ""
	}
	return *u.LockoutReason
}

// userHold names why an administrator deliberately holds this user (disabled,
// in the Trash, or locked out from Trust Monitor or through the Admin API), or "" when no
// such hold applies. Anything that grants a way through MFA refuses a held
// user without --force: the person asking for access may be the one held.
func userHold(u duoUser) string {
	switch {
	case strings.EqualFold(u.Status, "disabled"):
		return "user is disabled on purpose"
	case strings.EqualFold(u.Status, "pending deletion"):
		return "user is in the Trash pending deletion"
	case !strings.EqualFold(u.Status, "locked out"):
		return ""
	case u.lockoutReason() == "Admin disabled":
		return "locked by an administrator in Trust Monitor"
	case u.lockoutReason() == "Admin API disabled":
		return "locked out through the Admin API by an administrator or automation"
	}
	return ""
}

type duoBypassCode struct {
	BypassCodeID string        `json:"bypass_code_id"`
	AdminEmail   string        `json:"admin_email"`
	Created      int64         `json:"created"`
	Expiration   *int64        `json:"expiration"`
	ReuseCount   *int64        `json:"reuse_count"`
	User         *duoPhoneUser `json:"user,omitempty"`
}

// expired reports whether the code can no longer be used.
func (c duoBypassCode) expired(now time.Time) bool {
	return c.Expiration != nil && time.Unix(*c.Expiration, 0).Before(now)
}

// standing reports a code that never expires or has unlimited reuse (Duo
// returns null for both).
func (c duoBypassCode) standing() bool { return c.Expiration == nil || c.ReuseCount == nil }

var duoUserIDRE = regexp.MustCompile(`^DU[A-Z0-9]{18}$`)
var duoAccountIDRE = regexp.MustCompile(`^D[A-Z0-9]{19}$`)

// duoGet issues a fresh (uncached) GET, optionally scoped to a subaccount,
// and returns the unwrapped response plus metadata.
func duoGet(ctx context.Context, c *client.Client, account, path string, params map[string]string) (json.RawMessage, json.RawMessage, error) {
	p := map[string]string{}
	for k, v := range params {
		p[k] = v
	}
	if account != "" {
		p["account_id"] = account
	}
	raw, err := c.GetNoCache(ctx, path, p)
	if err != nil {
		return nil, nil, classifyAPIErrorOnly(err)
	}
	return duoUnwrap(raw)
}

// duoGetValues is duoGet for repeatable query parameters (log filters).
func duoGetValues(ctx context.Context, c *client.Client, path string, params url.Values) (json.RawMessage, json.RawMessage, error) {
	raw, err := c.GetWithHeadersNoCacheValues(ctx, path, params, nil)
	if err != nil {
		return nil, nil, classifyAPIErrorOnly(err)
	}
	return duoUnwrap(raw)
}

// duoPost sends a JSON body (v5 signing) and unwraps the response. A nil body
// is sent as {} (a nil map would marshal to null).
func duoPost(ctx context.Context, c *client.Client, path string, body map[string]any) (json.RawMessage, error) {
	if body == nil {
		body = map[string]any{}
	}
	raw, _, err := c.Post(ctx, path, body)
	if err != nil {
		return nil, classifyAPIErrorOnly(err)
	}
	resp, _, err := duoUnwrap(raw)
	return resp, err
}

// duoDelete issues a DELETE and unwraps the response.
func duoDelete(ctx context.Context, c *client.Client, path string) error {
	raw, _, err := c.DeleteWithParams(ctx, path, nil)
	if err != nil {
		return classifyAPIErrorOnly(err)
	}
	_, _, err = duoUnwrap(raw)
	return err
}

// duoPageAll walks a v1 offset-paged list. It stops on a short page, when
// metadata has no next_offset, or after maxPages (capHit reports the cap).
func duoPageAll(ctx context.Context, c *client.Client, account, path string, pageSize, maxPages int) (items []json.RawMessage, pages int, capHit bool, err error) {
	items = make([]json.RawMessage, 0)
	offset := 0
	for pages < maxPages {
		p := map[string]string{"limit": strconv.Itoa(pageSize), "offset": strconv.Itoa(offset)}
		resp, meta, gerr := duoGet(ctx, c, account, path, p)
		if gerr != nil {
			return items, pages, false, gerr
		}
		pages++
		var page []json.RawMessage
		if err := json.Unmarshal(resp, &page); err != nil {
			return items, pages, false, fmt.Errorf("parsing %s page: %w", path, err)
		}
		items = append(items, page...)
		var m struct {
			NextOffset *int `json:"next_offset"`
		}
		_ = json.Unmarshal(meta, &m)
		if len(page) < pageSize || (len(meta) > 0 && m.NextOffset == nil) {
			return items, pages, false, nil
		}
		if m.NextOffset != nil {
			offset = *m.NextOffset
		} else {
			offset += len(page)
		}
	}
	return items, pages, true, nil
}

// duoFindUser resolves a user by user_id, email, or username/alias in the
// account the request is routed to (--account-id / CISCO_DUO_ACCOUNT_ID).
func duoFindUser(ctx context.Context, c *client.Client, who string) (duoUser, error) {
	who = strings.TrimSpace(who)
	if duoUserIDRE.MatchString(who) {
		raw, _, err := duoGet(ctx, c, "", "/admin/v1/users/"+url.PathEscape(who), nil)
		if err != nil {
			return duoUser{}, err
		}
		var u duoUser
		if err := json.Unmarshal(raw, &u); err != nil {
			return duoUser{}, fmt.Errorf("parsing user: %w", err)
		}
		return u, nil
	}
	users, err := duoLookupUsers(ctx, c, "", who)
	if err != nil {
		return duoUser{}, err
	}
	switch len(users) {
	case 0:
		return duoUser{}, notFoundErr(fmt.Errorf("no Duo user matches %q (try 'fleet find-user %s' to search every subaccount)", who, who))
	case 1:
		return users[0], nil
	default:
		return duoUser{}, usageErr(fmt.Errorf("%d users match %q; pass the user_id instead", len(users), who))
	}
}

// duoLookupUsers finds users by username/alias, and also by email when the
// input contains '@' (an email-shaped value can be a username or alias),
// de-duplicated by user_id.
func duoLookupUsers(ctx context.Context, c *client.Client, account, who string) ([]duoUser, error) {
	keys := []string{"username"}
	if strings.Contains(who, "@") {
		keys = append(keys, "email")
	}
	seen := map[string]bool{}
	out := make([]duoUser, 0)
	for _, key := range keys {
		raw, _, err := duoGet(ctx, c, account, "/admin/v1/users", map[string]string{key: who})
		if err != nil {
			return nil, err
		}
		var users []duoUser
		if err := json.Unmarshal(raw, &users); err != nil {
			return nil, fmt.Errorf("parsing users: %w", err)
		}
		for _, u := range users {
			if !seen[u.UserID] {
				seen[u.UserID] = true
				out = append(out, u)
			}
		}
	}
	return out, nil
}

// duoListAccounts returns the MSP parent's subaccounts.
func duoListAccounts(ctx context.Context, c *client.Client) ([]client.DuoSubaccount, error) {
	raw, err := duoPost(ctx, c, "/accounts/v1/account/list", nil)
	if err != nil {
		return nil, err
	}
	accounts := make([]client.DuoSubaccount, 0)
	if err := json.Unmarshal(raw, &accounts); err != nil {
		return nil, fmt.Errorf("parsing subaccounts: %w", err)
	}
	return accounts, nil
}

// duoSelectAccounts filters accounts by a comma list of IDs or names
// (case-insensitive) and returns, in input order, every selector that matched
// no account so the caller can refuse it instead of silently reporting fewer
// customers than were asked for.
func duoSelectAccounts(accounts []client.DuoSubaccount, only string) ([]client.DuoSubaccount, []string) {
	if strings.TrimSpace(only) == "" {
		return accounts, nil
	}
	want := map[string]string{}
	order := make([]string, 0)
	for _, part := range strings.Split(only, ",") {
		raw := strings.TrimSpace(part)
		p := strings.ToLower(raw)
		if p == "" {
			continue
		}
		if _, dup := want[p]; !dup {
			want[p] = raw
			order = append(order, p)
		}
	}
	matched := map[string]bool{}
	out := make([]client.DuoSubaccount, 0)
	for _, a := range accounts {
		id, name := strings.ToLower(a.AccountID), strings.ToLower(a.Name)
		_, byID := want[id]
		_, byName := want[name]
		if byID {
			matched[id] = true
		}
		if byName {
			matched[name] = true
		}
		if byID || byName {
			out = append(out, a)
		}
	}
	unmatched := make([]string, 0)
	for _, p := range order {
		if !matched[p] {
			unmatched = append(unmatched, want[p])
		}
	}
	return out, unmatched
}

// duoFleetPrep clears a pinned --account-id so fan-out commands query every
// subaccount explicitly, and reports whether one was set. Fan-out commands
// never read or write the local store, so their requests may each name a
// subaccount (refuseUnpinnedRequestAccount stands down for this run).
func duoFleetPrep() string {
	pinned := currentAccountID()
	if pinned != "" {
		_ = os.Unsetenv(client.DuoAccountEnv)
	}
	storeScopedRun.Store(false)
	return pinned
}

// currentAccountID is the subaccount pinned by --account-id / CISCO_DUO_ACCOUNT_ID.
func currentAccountID() string { return client.PinnedAccountID() }

// duoParseSince accepts 7d / 24h / 1w style durations.
func duoParseSince(s string, def time.Duration) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return def, nil
	}
	d, err := cliutil.ParseDurationLoose(s)
	if err != nil {
		return 0, usageErr(fmt.Errorf("invalid duration %q (use forms like 24h, 7d, 2w)", s))
	}
	if d <= 0 {
		return 0, usageErr(fmt.Errorf("duration %q must be positive", s))
	}
	return d, nil
}

// flexTime accepts a Duo timestamp sent as a string or a number (the docs
// disagree for phone dates), keeping its text form.
type flexTime string

func (f *flexTime) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flexTime(v)
		return nil
	}
	*f = flexTime(s)
	return nil
}

// duoTimeLayouts covers the formats Duo uses for phone last_seen.
var duoTimeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05"}

// duoParseTime parses a Duo timestamp that may be ISO text or Unix seconds.
func duoParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return time.Unix(n, 0).UTC(), true
	}
	for _, layout := range duoTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func unixPtrTime(v *int64) string {
	if v == nil {
		return ""
	}
	return unixTime(*v)
}

func unixTime(v int64) string {
	if v == 0 {
		return ""
	}
	return time.Unix(v, 0).UTC().Format(time.RFC3339)
}

// shellQuote quotes a command-line argument only when the shell would split
// or expand it, so suggested commands are safe to copy and run.
func shellQuote(s string) string {
	// Allowlist (shlex's safe set minus '='; zsh expands a leading =word).
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+,./:_-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func hasCapability(p duoPhone, capability string) bool {
	for _, c := range p.Capabilities {
		if strings.EqualFold(c, capability) {
			return true
		}
	}
	return false
}

// duoPhoneOwners fetches each phone's user list. A phone whose lookup fails
// is left out of the map, which phoneShareBlock reads as "unknown".
func duoPhoneOwners(ctx context.Context, c *client.Client, ids ...string) map[string][]duoPhoneUser {
	owners := make(map[string][]duoPhoneUser, len(ids))
	for _, id := range ids {
		raw, _, err := duoGet(ctx, c, "", "/admin/v1/phones/"+url.PathEscape(id), nil)
		if err != nil {
			continue
		}
		var full duoPhone
		if json.Unmarshal(raw, &full) == nil {
			owners[id] = full.Users
		}
	}
	return owners
}

// phoneShareBlock says why activating phoneID for userID would reach other
// users: Duo Mobile activation binds the phone, and so every user on it, to
// whichever device completes it (and --url hands that link to the caller).
// "" when userID is the phone's only user. Callers add the --force advice.
func phoneShareBlock(phoneID, userID string, owners map[string][]duoPhoneUser) string {
	users, known := owners[phoneID]
	if !known {
		return "could not confirm who else uses phone " + phoneID
	}
	others := 0
	for _, o := range users {
		if o.UserID != userID {
			others++
		}
	}
	if others == 0 {
		return ""
	}
	return fmt.Sprintf("phone %s is shared with %d other user(s); activating it would bind Duo Mobile for all of them to whoever completes activation", phoneID, others)
}
