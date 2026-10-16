// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package posture holds the pure evaluation logic behind the SonicWall posture
// commands: firmware version ordering, MFA coverage from SonicOS config, SSL-VPN
// zone exposure, MySonicWall licence rows, life-cycle dates and snapshot diffs.
// Field names follow the vendor-published NSM SonicOS passthrough and MySonicWall
// specs; nothing here touches the network.
package posture

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Status values shared by every check. Unknown means the data could not be read,
// never that the check passed.
const (
	Pass    = "pass"
	Warn    = "warn"
	Fail    = "fail"
	Unknown = "unknown"
)

// Worst returns the most severe status: fail > unknown > warn > pass.
func Worst(statuses ...string) string {
	rank := map[string]int{Pass: 0, Warn: 1, Unknown: 2, Fail: 3}
	worst := Pass
	for _, s := range statuses {
		if s == "" {
			continue
		}
		if rank[s] > rank[worst] {
			worst = s
		}
	}
	return worst
}

var digitRun = regexp.MustCompile(`[0-9]+`)

// versionParts extracts every digit run, so "SonicOS 7.0.1-5145-R5175" orders as
// 7,0,1,5145,5175 and "6.5.4.15-116n" as 6,5,4,15,116.
// ponytail: numeric-run ordering; swap for a real SonicOS grammar if build suffixes ever reorder.
func versionParts(v string) []int {
	runs := digitRun.FindAllString(v, -1)
	out := make([]int, 0, len(runs))
	for _, r := range runs {
		n, err := strconv.Atoi(r)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// CompareFirmware orders two SonicOS firmware strings: -1 when a is older, 0 when
// equal, 1 when a is newer.
func CompareFirmware(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(pa) < len(pb):
		return -1
	case len(pa) > len(pb):
		return 1
	}
	return 0
}

// FirmwareGap counts available builds newer than current and returns the newest.
// A blank current version yields ok=false.
func FirmwareGap(current string, available []string) (behind int, newest string, ok bool) {
	if strings.TrimSpace(current) == "" || len(versionParts(current)) == 0 {
		return 0, "", false
	}
	newest = current
	for _, v := range available {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if CompareFirmware(v, current) > 0 {
			behind++
		}
		if CompareFirmware(v, newest) > 0 {
			newest = v
		}
	}
	return behind, newest, true
}

// AvailableVersions reads /devices/availableVersions/{serial}:
// {"availableVersions":[{"version":"...","releaseDate":"..."}]}. ok is false when
// the body is malformed, the availableVersions array is missing or null, or any
// entry lacks a usable version, so an unreadable answer is never mistaken for
// "no newer builds".
func AvailableVersions(raw json.RawMessage) ([]string, bool) {
	var doc struct {
		AvailableVersions *[]struct {
			Version string `json:"version"`
		} `json:"availableVersions"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.AvailableVersions == nil {
		return nil, false
	}
	out := make([]string, 0, len(*doc.AvailableVersions))
	for _, v := range *doc.AvailableVersions {
		if strings.TrimSpace(v.Version) == "" || len(versionParts(v.Version)) == 0 {
			return nil, false // an entry without a usable version makes the whole answer unreadable
		}
		out = append(out, v.Version)
	}
	return out, true
}

// OTPMethod names a SonicOS one_time_password object: {"totp":true} -> totp,
// {"otp":true} -> otp-email, null/{} -> none (the vendor schema documents null
// and {} as the unconfigured state). An absent key or an unreadable value is
// "unknown": absence is never read as "no MFA".
func OTPMethod(raw json.RawMessage) string {
	var m struct {
		TOTP bool `json:"totp"`
		OTP  bool `json:"otp"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return "unknown"
	}
	switch {
	case m.TOTP:
		return "totp"
	case m.OTP:
		return "otp-email"
	}
	return "none"
}

// AdminTOTP reads /firewall/administration/global and reports whether
// administration.admin.one_time_password.totp is enabled.
func AdminTOTP(raw json.RawMessage) (enabled bool, ok bool) {
	m, ok := AdminOTP(raw)
	return m == "totp", ok
}

// AdminOTP names the admin one-time password method (see OTPMethod); ok is
// false when the admin settings or their OTP setting were not reported.
func AdminOTP(raw json.RawMessage) (method string, ok bool) {
	var doc struct {
		Administration *struct {
			Admin *struct {
				OneTimePassword json.RawMessage `json:"one_time_password"`
			} `json:"admin"`
		} `json:"administration"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.Administration == nil {
		return "", false
	}
	if doc.Administration.Admin == nil {
		return "", false // admin settings not reported: unknown, not "no TOTP"
	}
	m := OTPMethod(doc.Administration.Admin.OneTimePassword)
	return m, m != "unknown"
}

// SSLVPNGroup is the SonicOS built-in group whose members may connect over SSL VPN.
const SSLVPNGroup = "SSLVPN Services"

// Account is one local user's MFA coverage on one firewall. Method "unknown"
// means coverage could not be decided (the group list was unreadable and the
// user has no one-time password of their own).
type Account struct {
	Name          string `json:"account"`
	SSLVPNMember  *bool  `json:"sslvpn_member"` // null when membership data was not reported
	Method        string `json:"mfa_method"`
	MethodSource  string `json:"mfa_source,omitempty"`
	ForceNewLogin bool   `json:"force_password_change,omitempty"`
}

type nameRef struct {
	Name string `json:"name"`
}

type localUser struct {
	Name                string          `json:"name"`
	OneTimePassword     json.RawMessage `json:"one_time_password"`
	ForcePasswordChange bool            `json:"force_password_change"`
	MemberOf            []*nameRef      `json:"member_of"`
}

type localGroup struct {
	Name            string          `json:"name"`
	OneTimePassword json.RawMessage `json:"one_time_password"`
	Member          []*nameRef      `json:"member"`
}

// Redacted is the string the client's secret redaction leaves in place of a
// value (cli/sonicwall_secret_redact.go).
const Redacted = "[redacted]"

// hasIdentity reports whether name can identify a user, group or zone: it is
// not empty and not shaped like a redaction placeholder ("[redacted", any
// case or suffix), which two different names can share.
func hasIdentity(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n != "" && !strings.HasPrefix(n, "[redacted")
}

// namedRefs reports whether every membership reference is a non-null row
// naming an identity.
func namedRefs(refs []*nameRef) bool {
	for _, r := range refs {
		if r == nil || !hasIdentity(r.Name) {
			return false
		}
	}
	return true
}

// parseLocalUsers requires user.local.user to be present (an explicit [] is a
// verified empty list; a missing or null collection is not), and every row to
// be a named user: a null or nameless row, a redacted name or a repeated name
// (which would join two accounts as one) makes the whole list unreadable.
func parseLocalUsers(raw json.RawMessage) ([]localUser, bool) {
	var doc struct {
		User *struct {
			Local *struct {
				User *[]*localUser `json:"user"`
			} `json:"local"`
		} `json:"user"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.User == nil || doc.User.Local == nil || doc.User.Local.User == nil {
		return nil, false
	}
	out := make([]localUser, 0, len(*doc.User.Local.User))
	seen := map[string]bool{}
	for _, u := range *doc.User.Local.User {
		if u == nil || !hasIdentity(u.Name) || seen[strings.ToLower(u.Name)] || !namedRefs(u.MemberOf) {
			return nil, false
		}
		seen[strings.ToLower(u.Name)] = true
		out = append(out, *u)
	}
	return out, true
}

// parseLocalGroups applies the same rule to user.local.group: a null,
// nameless, redacted or repeated group row would erase or merge membership
// evidence, so it makes the list unreadable.
func parseLocalGroups(raw json.RawMessage) ([]localGroup, bool) {
	var doc struct {
		User *struct {
			Local *struct {
				Group *[]*localGroup `json:"group"`
			} `json:"local"`
		} `json:"user"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &doc) != nil || doc.User == nil || doc.User.Local == nil || doc.User.Local.Group == nil {
		return nil, false
	}
	out := make([]localGroup, 0, len(*doc.User.Local.Group))
	seen := map[string]bool{}
	for _, g := range *doc.User.Local.Group {
		if g == nil || !hasIdentity(g.Name) || seen[strings.ToLower(g.Name)] || !namedRefs(g.Member) {
			return nil, false
		}
		seen[strings.ToLower(g.Name)] = true
		out = append(out, *g)
	}
	return out, true
}

// LocalAccounts joins /firewall/user/local/users with /firewall/user/local/groups.
// A user is an SSL-VPN user when it belongs to "SSLVPN Services" directly or
// through groups nested inside it (group member[] lists). A user is covered by
// its own one_time_password or by the one_time_password of a group it belongs
// to directly. When groupsRaw is unreadable, users without their own OTP get
// Method "unknown" instead of a verdict. ok is false when the user list is
// missing or malformed.
// ponytail: group OTP is not inherited through nesting; that can only under-count coverage (false fail), never over-count it.
func LocalAccounts(usersRaw, groupsRaw json.RawMessage) ([]Account, bool) {
	users, ok := parseLocalUsers(usersRaw)
	if !ok {
		return nil, false
	}
	groups, groupsOK := parseLocalGroups(groupsRaw)
	isUser := map[string]bool{}
	for _, u := range users {
		isUser[strings.ToLower(u.Name)] = true
	}

	groupOTP := map[string]string{}
	members := map[string][]string{} // group -> member names (users or groups)
	isGroup := map[string]bool{}
	memberListed := map[string]bool{} // group -> its member key was present
	allGroupsListed := groupsOK
	for _, g := range groups {
		k := strings.ToLower(g.Name)
		isGroup[k] = true
		groupOTP[k] = OTPMethod(g.OneTimePassword)
		memberListed[k] = g.Member != nil
		if g.Member == nil {
			allGroupsListed = false
		}
		for _, m := range g.Member {
			members[k] = append(members[k], strings.ToLower(m.Name))
		}
	}
	// A member name that is neither a listed group nor a listed user may be a
	// group the group list did not return; its own members and OTP are unknown.
	unresolvedAny := false
	for _, ms := range members {
		for _, m := range ms {
			if !isGroup[m] && !isUser[m] {
				unresolvedAny = true
			}
		}
	}
	// Groups nested (transitively) inside SSLVPN Services, plus users listed there.
	vpnGroups := map[string]bool{strings.ToLower(SSLVPNGroup): true}
	vpnUsers := map[string]bool{}
	queue := []string{strings.ToLower(SSLVPNGroup)}
	unresolved := false // a nested name that is neither a listed group nor a listed user
	for len(queue) > 0 {
		g := queue[0]
		queue = queue[1:]
		for _, m := range members[g] {
			if isGroup[m] {
				if !vpnGroups[m] {
					vpnGroups[m] = true
					queue = append(queue, m)
				}
				continue
			}
			vpnUsers[m] = true
			if !isUser[m] {
				unresolved = true // could be a group the group list did not return
			}
		}
	}
	// "Not an SSL-VPN user" needs complete evidence: every group nested in
	// SSLVPN Services reported its member list, and the user's own direct
	// groups are known (its member_of key, or every group's member list).
	// Anything less leaves membership unknown, never "not a member".
	vpnGroupsComplete := groupsOK && isGroup[strings.ToLower(SSLVPNGroup)] && !unresolved
	for g := range vpnGroups {
		if !memberListed[g] {
			vpnGroupsComplete = false
		}
	}
	// Direct groups of each user, from either side of the relationship.
	userGroups := map[string][]string{}
	for g, ms := range members {
		for _, m := range ms {
			if !isGroup[m] {
				userGroups[m] = append(userGroups[m], g)
			}
		}
	}

	out := make([]Account, 0, len(users))
	for _, u := range users {
		key := strings.ToLower(u.Name)
		direct := append([]string{}, userGroups[key]...)
		for _, g := range u.MemberOf {
			direct = append(direct, strings.ToLower(g.Name))
		}
		a := Account{Name: u.Name, Method: OTPMethod(u.OneTimePassword), ForceNewLogin: u.ForcePasswordChange}
		if a.Method != "none" && a.Method != "unknown" {
			a.MethodSource = "user"
		}
		member := vpnUsers[key]
		for _, g := range direct {
			// vpnUsers holds every name nested in SSLVPN Services that is not a
			// listed group; a member_of entry naming one is a group nested there.
			if vpnGroups[g] || vpnUsers[g] {
				member = true
			}
			// A group OTP covers the user; a group whose OTP setting was not
			// reported, or that the group list did not return, makes uncovered
			// coverage unknown, but never outranks a real one-time password
			// found on another group.
			switch m := groupOTP[g]; {
			case (m == "totp" || m == "otp-email") && (a.Method == "none" || a.Method == "unknown"):
				a.Method, a.MethodSource = m, "group:"+groupName(groups, g)
			case (m == "unknown" || !isGroup[g]) && a.Method == "none":
				a.Method, a.MethodSource = "unknown", ""
			}
		}
		if a.Method == "none" && !groupsOK {
			a.Method = "unknown"
		}
		// When the user's direct groups are not fully reported (no member_of,
		// and some group did not report its members), a group that does carry
		// a one-time password might cover the user: coverage is unknown.
		if a.Method == "none" && u.MemberOf == nil && !allGroupsListed {
			for _, g := range groups {
				if g.Member == nil && OTPMethod(g.OneTimePassword) != "none" {
					a.Method, a.MethodSource = "unknown", ""
					break
				}
			}
		}
		// Without member_of, a group the list did not return might also hold
		// the user and carry a one-time password.
		if a.Method == "none" && u.MemberOf == nil && unresolvedAny {
			a.Method, a.MethodSource = "unknown", ""
		}
		if member || (vpnGroupsComplete && (u.MemberOf != nil || allGroupsListed)) {
			a.SSLVPNMember = &member
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, true
}

func groupName(groups []localGroup, lower string) string {
	for _, g := range groups {
		if strings.ToLower(g.Name) == lower {
			return g.Name
		}
	}
	return lower
}

// SSLVPNZones reads /firewall/ssl-vpn/server/accesses and returns the zones with
// SSL VPN enabled, plus whether any of them is a WAN zone.
// ponytail: WAN = zone name contains "WAN"; custom-named untrusted zones are missed.
func SSLVPNZones(raw json.RawMessage) (zones []string, wan bool, ok bool) {
	var doc struct {
		SSLVPN *struct {
			Server *struct {
				Access *[]*struct {
					Zone   *string `json:"zone"`
					Enable *bool   `json:"enable"`
				} `json:"access"`
			} `json:"server"`
		} `json:"ssl_vpn"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.SSLVPN == nil || doc.SSLVPN.Server == nil || doc.SSLVPN.Server.Access == nil {
		return nil, false, false
	}
	zones = make([]string, 0)
	for _, a := range *doc.SSLVPN.Server.Access {
		// A null entry, or one missing its zone or enable flag, cannot be graded.
		if a == nil || a.Zone == nil || a.Enable == nil || !hasIdentity(*a.Zone) {
			return nil, false, false
		}
		if !*a.Enable {
			continue
		}
		zones = append(zones, *a.Zone)
		if strings.Contains(strings.ToUpper(*a.Zone), "WAN") {
			wan = true
		}
	}
	return zones, wan, true
}

// SSLVPNAuth reads /firewall/ssl-vpn/server/base and returns its auth_type
// (password, certificate or saml); saml is true when SSL-VPN sign-in goes to a
// SAML identity provider, where MFA is enforced upstream. ok is false when
// auth_type was not reported. use_radius is not read: SonicWall documents it
// as the MSCHAP mode for RADIUS password changes, not as the sign-in source;
// UserAuthMethods reads that.
func SSLVPNAuth(raw json.RawMessage) (authType string, saml bool, ok bool) {
	var doc struct {
		SSLVPN *struct {
			Server *struct {
				AuthType string `json:"auth_type"`
			} `json:"server"`
		} `json:"ssl_vpn"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.SSLVPN == nil || doc.SSLVPN.Server == nil {
		return "", false, false
	}
	authType = strings.ToLower(strings.TrimSpace(doc.SSLVPN.Server.AuthType))
	switch authType {
	case "password", "certificate", "saml":
	default:
		return "", false, false // not reported, or a value the schema does not document
	}
	return authType, authType == "saml", true
}

// UserAuthMethods reads /firewall/user/authentication/methods: the firewall's
// user.auth.auth_method (local, radius, radius-local, ldap, ldap-local, tacacs
// or tacacs-local) plus the method of every authentication partition. ok is
// false when the top-level method, or any listed partition's method, was not
// reported: an unreported method is never read as "local only".
func UserAuthMethods(raw json.RawMessage) (methods []string, ok bool) {
	var doc struct {
		User *struct {
			Auth *struct {
				AuthMethod string `json:"auth_method"`
				Partition  []*struct {
					AuthMethod string `json:"auth_method"`
				} `json:"partition"`
			} `json:"auth"`
		} `json:"user"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.User == nil || doc.User.Auth == nil {
		return nil, false
	}
	a := doc.User.Auth
	methods = []string{strings.ToLower(strings.TrimSpace(a.AuthMethod))}
	for _, p := range a.Partition {
		if p == nil {
			return nil, false
		}
		methods = append(methods, strings.ToLower(strings.TrimSpace(p.AuthMethod)))
	}
	for _, m := range methods {
		if !authMethodEnum[m] {
			return nil, false // not reported, or a value the schema does not document
		}
	}
	return methods, true
}

var authMethodEnum = map[string]bool{"local": true, "radius": true, "radius-local": true, "ldap": true,
	"ldap-local": true, "tacacs": true, "tacacs-local": true}

// SignInSources summarises UserAuthMethods: external names the non-local
// directories users can sign in through (radius, ldap, tacacs), and local is
// true when local accounts can sign in ("local" or an "-local" method).
func SignInSources(methods []string) (external []string, local bool) {
	seen := map[string]bool{}
	for _, m := range methods {
		base := strings.TrimSuffix(m, "-local")
		if m == "local" || base != m {
			local = true
		}
		if base != "local" && !seen[base] {
			seen[base] = true
			external = append(external, base)
		}
	}
	sort.Strings(external)
	return external, local
}

// Licence is one MySonicWall service row for a serial. Undated is true when
// the service is not marked expired and neither nDaysToExpiry nor expirydate
// gives a usable expiry.
type Licence struct {
	Service      string `json:"service"`
	Family       string `json:"service_family,omitempty"`
	Expiry       string `json:"expiry_date,omitempty"`
	DaysToExpiry *int   `json:"days_to_expiry,omitempty"`
	Expired      bool   `json:"expired"`
	Undated      bool   `json:"expiry_unknown,omitempty"`
	Status       string `json:"service_status,omitempty"`
}

// Licences reads MySonicWall /api/product/serviceInfo: serviceTypeList[].serviceList[]
// with serviceName, expirydate, nDaysToExpiry and expired. The body may also arrive
// wrapped in {"content":{...}} like other MySonicWall responses. ok is false when
// serviceTypeList, or any serviceList inside it, is missing or null. When
// nDaysToExpiry is absent the days are computed from expirydate against now.
func Licences(raw json.RawMessage, now time.Time) ([]Licence, bool) {
	type svc struct {
		ServiceName   string `json:"serviceName"`
		ServiceFamily string `json:"serviceFamily"`
		ExpiryDate    string `json:"expirydate"`
		NDaysToExpiry *int   `json:"nDaysToExpiry"`
		Expired       string `json:"expired"`
		ServiceStatus string `json:"serviceStatus"`
		// Bundled services (Capture ATP inside a security suite) carry their own
		// expiry and are graded as rows of their own.
		SecondaryServiceList []*svc `json:"secondaryServiceList"`
	}
	type body struct {
		ServiceTypeList *[]struct {
			ServiceList *[]*svc `json:"serviceList"`
		} `json:"serviceTypeList"`
		Content json.RawMessage `json:"content"`
	}
	var doc body
	if json.Unmarshal(raw, &doc) != nil {
		return nil, false
	}
	if doc.ServiceTypeList == nil && len(doc.Content) > 0 {
		var inner body
		if json.Unmarshal(doc.Content, &inner) == nil {
			doc = inner
		}
	}
	if doc.ServiceTypeList == nil {
		return nil, false
	}
	out := make([]Licence, 0)
	for _, t := range *doc.ServiceTypeList {
		if t.ServiceList == nil {
			return nil, false
		}
		flat := make([]*svc, 0, len(*t.ServiceList))
		for _, s := range *t.ServiceList {
			if s == nil {
				return nil, false // a null row could be an expired service; never drop it silently
			}
			flat = append(flat, s)
			for _, sec := range s.SecondaryServiceList {
				if sec == nil {
					return nil, false
				}
				flat = append(flat, sec)
			}
		}
		for i, s := range flat {
			name := strings.TrimSpace(s.ServiceName)
			if name == "" {
				name = fmt.Sprintf("(unnamed service %d", i+1)
				if s.ServiceFamily != "" {
					name += ", family " + s.ServiceFamily
				}
				name += ")"
			}
			l := Licence{Service: name, Family: s.ServiceFamily, Expiry: s.ExpiryDate, DaysToExpiry: s.NDaysToExpiry, Status: s.ServiceStatus}
			l.Expired = strings.EqualFold(s.Expired, "true") || strings.EqualFold(s.Expired, "yes") || s.Expired == "1"
			// The expiry date is checked even when nDaysToExpiry is given, and the
			// more conservative of the two wins, so a rounded or stale day count
			// cannot hide a lapse.
			if t, ok := ParseLifecycleDate(s.ExpiryDate); ok {
				var days int
				if strings.ContainsAny(s.ExpiryDate, ":T") {
					// A timestamp: floor, not truncation, so a service that lapsed
					// 12 hours ago is day -1 (expired), not day 0.
					days = int(math.Floor(t.Sub(now).Hours() / 24))
					if !t.After(now) {
						l.Expired = true
					}
				} else {
					// A calendar date (MySonicWall's nDaysToExpiry convention):
					// whole days from today, still valid on the day itself.
					today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, t.Location())
					days = int(math.Round(t.Sub(today).Hours() / 24))
				}
				if l.DaysToExpiry == nil || days < *l.DaysToExpiry {
					l.DaysToExpiry = &days
				}
			}
			if l.DaysToExpiry != nil && *l.DaysToExpiry < 0 {
				l.Expired = true
			}
			l.Undated = !l.Expired && l.DaysToExpiry == nil
			out = append(out, l)
		}
	}
	return out, true
}

// SoonestExpiry returns the licence closest to expiry. A service flagged
// expired always ranks first, whatever its day count says, so contradictory
// metadata can never hide an expired service behind an active one. Undated
// services are skipped; callers must treat their presence as incomplete
// coverage (see AnyUndated).
func SoonestExpiry(ls []Licence) (Licence, bool) {
	best, found := Licence{}, false
	for _, l := range ls {
		if l.DaysToExpiry == nil && !l.Expired {
			continue
		}
		if !found || expiryRank(l) < expiryRank(best) {
			best, found = l, true
		}
	}
	return best, found
}

// AnyUndated reports whether any service has no usable expiry.
func AnyUndated(ls []Licence) bool {
	for _, l := range ls {
		if l.Undated {
			return true
		}
	}
	return false
}

// expiryRank orders expired services before every active one, then by days left.
func expiryRank(l Licence) int {
	if l.Expired {
		d := 0
		if l.DaysToExpiry != nil && *l.DaysToExpiry < 0 {
			d = *l.DaysToExpiry
		}
		return d - (1 << 30)
	}
	if l.DaysToExpiry != nil {
		return *l.DaysToExpiry
	}
	return 1 << 30
}

var lifecycleLayouts = []string{
	"2006-01-02", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "01/02/2006", "1/2/2006",
	"2006/01/02", "Jan 2, 2006", "January 2, 2006", "02-Jan-2006", "2-Jan-2006", "Jan 2006", "January 2006",
}

// ParseLifecycleDate parses the free-form milestone dates in productLifeCycle.
func ParseLifecycleDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range lifecycleLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// SupportStatus grades an end-of-support date against now: fail when past, warn
// within warnWindow, pass otherwise, unknown when unparseable or blank.
func SupportStatus(eos string, now time.Time, warnWindow time.Duration) string {
	t, ok := ParseLifecycleDate(eos)
	if !ok {
		return Unknown
	}
	switch {
	case !t.After(now):
		return Fail
	case t.Before(now.Add(warnWindow)):
		return Warn
	}
	return Pass
}

// MatchesAdvisory reports whether query names the advisory: an exact advisory ID
// (case-insensitive) or a substring of its title or notice link.
func MatchesAdvisory(query, advisoryID, title, noticeLink string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	if strings.EqualFold(q, strings.TrimSpace(advisoryID)) {
		return true
	}
	return strings.Contains(strings.ToLower(title), q) || strings.Contains(strings.ToLower(noticeLink), q)
}

// Change is one per-firewall transition between two posture snapshots.
type Change struct {
	Serial string `json:"serial"`
	Name   string `json:"name,omitempty"`
	Check  string `json:"check"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// DiffSnapshots compares two serial->check->status maps. Firewalls that appear or
// disappear are reported under the "present" check.
func DiffSnapshots(before, after map[string]map[string]string, names map[string]string) []Change {
	out := make([]Change, 0)
	serials := map[string]bool{}
	for s := range before {
		serials[s] = true
	}
	for s := range after {
		serials[s] = true
	}
	keys := make([]string, 0, len(serials))
	for s := range serials {
		keys = append(keys, s)
	}
	sort.Strings(keys)
	for _, s := range keys {
		b, inBefore := before[s]
		a, inAfter := after[s]
		switch {
		case !inBefore:
			out = append(out, Change{Serial: s, Name: names[s], Check: "present", Before: "absent", After: "present"})
			continue
		case !inAfter:
			out = append(out, Change{Serial: s, Name: names[s], Check: "present", Before: "present", After: "absent"})
			continue
		}
		checks := map[string]bool{}
		for k := range b {
			checks[k] = true
		}
		for k := range a {
			checks[k] = true
		}
		ck := make([]string, 0, len(checks))
		for k := range checks {
			ck = append(ck, k)
		}
		sort.Strings(ck)
		for _, k := range ck {
			if b[k] != a[k] {
				out = append(out, Change{Serial: s, Name: names[s], Check: k, Before: b[k], After: a[k]})
			}
		}
	}
	return out
}
