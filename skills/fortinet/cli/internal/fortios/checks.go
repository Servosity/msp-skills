// Package fortios holds the pure, network-free grading logic behind the
// fleet and audit commands. Inputs are the decoded `results` payloads of
// FortiOS REST API responses (hyphenated keys, as the device sends them).
package fortios

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Status values, ordered by severity.
const (
	Pass    = "PASS"
	Warn    = "WARN"
	Fail    = "FAIL"
	Unknown = "UNKNOWN"
)

var severity = map[string]int{Pass: 0, Unknown: 1, Warn: 2, Fail: 3}

// Worst returns the most severe status in the list (PASS when empty).
func Worst(statuses ...string) string {
	out := Pass
	for _, s := range statuses {
		if severity[s] > severity[out] {
			out = s
		}
	}
	return out
}

// Finding is one graded observation about one device.
type Finding struct {
	Device string `json:"device"`
	VDOM   string `json:"vdom,omitempty"`
	Check  string `json:"check"`
	Status string `json:"status"`
	Code   string `json:"code"`
	Object string `json:"object,omitempty"`
	Detail string `json:"detail"`
}

// Key identifies a finding across runs (used by fleet audit --new).
func (f Finding) Key() string {
	return strings.Join([]string{f.Device, f.VDOM, f.Check, f.Code, f.Object}, "|")
}

// Status summarizes a check's findings for one device.
func Status(findings []Finding) string {
	statuses := make([]string, 0, len(findings))
	for _, f := range findings {
		statuses = append(statuses, f.Status)
	}
	return Worst(statuses...)
}

// Obj is a decoded FortiOS JSON object.
type Obj = map[string]any

// Str reads a string field ("" when absent or not a string).
func Str(o Obj, key string) string {
	if v, ok := o[key].(string); ok {
		return v
	}
	return ""
}

// Num reads a numeric field (FortiOS sometimes sends numbers as strings).
func Num(o Obj, key string) (float64, bool) {
	switch v := o[key].(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	return 0, false
}

// Names reads a FortiOS reference list such as srcintf: [{"name":"port1"}].
func Names(o Obj, key string) []string {
	arr, _ := o[key].([]any)
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			if n := Str(m, "name"); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// Objs converts a decoded JSON array into objects, skipping non-objects.
func Objs(v any) []Obj {
	arr, _ := v.([]any)
	out := make([]Obj, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// ---------- firmware ----------

// Version is a FortiOS major.minor.patch triple.
type Version struct{ Major, Minor, Patch int }

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// Branch is major.minor, e.g. "7.4".
func (v Version) Branch() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// Less reports v < o.
func (v Version) Less(o Version) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}

// ParseVersion accepts "v7.4.3", "7.4.3", "v7.4.3,build2573" or "7.4".
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(s)), "v")
	if i := strings.IndexAny(s, ", -"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Version{}, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, false
		}
		nums[i] = n
	}
	return Version{nums[0], nums[1], nums[2]}, true
}

// ParseFloors parses "7.2.10,7.4.5" into branch -> minimum version.
func ParseFloors(csv string) (map[string]Version, error) {
	out := map[string]Version{}
	for _, raw := range strings.Split(csv, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		v, ok := ParseVersion(raw)
		if !ok {
			return nil, fmt.Errorf("invalid firmware floor %q (want e.g. 7.4.5)", raw)
		}
		out[v.Branch()] = v
	}
	return out, nil
}

func imageVersion(o Obj) (Version, bool) {
	if v, ok := ParseVersion(Str(o, "version")); ok {
		return v, true
	}
	maj, ok1 := Num(o, "major")
	min, ok2 := Num(o, "minor")
	pat, ok3 := Num(o, "patch")
	if ok1 && ok2 && ok3 {
		return Version{int(maj), int(min), int(pat)}, true
	}
	return Version{}, false
}

// FirmwareReport is the per-device firmware view.
type FirmwareReport struct {
	Device         string    `json:"device"`
	Status         string    `json:"status"`
	Current        string    `json:"current"`
	Branch         string    `json:"branch"`
	Floor          string    `json:"floor,omitempty"`
	LatestInBranch string    `json:"latest_in_branch,omitempty"`
	PatchesBehind  int       `json:"patches_behind"`
	NewestOffered  string    `json:"newest_offered,omitempty"`
	Findings       []Finding `json:"findings"`
}

// Firmware grades monitor system/firmware results against per-branch floors.
// statusVersion is the envelope `version` from monitor system/status, used
// when the firmware payload has no current image.
func Firmware(device string, results Obj, statusVersion string, floors map[string]Version) FirmwareReport {
	r := FirmwareReport{Device: device, Findings: []Finding{}}
	add := func(status, code, detail string) {
		r.Findings = append(r.Findings, Finding{Device: device, Check: "firmware", Status: status, Code: code, Detail: detail})
	}
	cur, ok := Version{}, false
	if c, isObj := results["current"].(map[string]any); isObj {
		cur, ok = imageVersion(c)
	}
	if !ok {
		cur, ok = ParseVersion(statusVersion)
	}
	if !ok {
		add(Unknown, "version-unknown", "could not read the running FortiOS version")
		r.Status = Unknown
		return r
	}
	r.Current, r.Branch = cur.String(), cur.Branch()

	var latest, newest Version
	haveLatest, haveNewest := false, false
	for _, img := range Objs(results["available"]) {
		v, ok := imageVersion(img)
		if !ok {
			continue
		}
		if !haveNewest || newest.Less(v) {
			newest, haveNewest = v, true
		}
		if v.Branch() == cur.Branch() && (!haveLatest || latest.Less(v)) {
			latest, haveLatest = v, true
		}
	}
	if haveNewest {
		r.NewestOffered = newest.String()
	}
	if haveLatest && cur.Less(latest) {
		r.LatestInBranch = latest.String()
		r.PatchesBehind = latest.Patch - cur.Patch
	}

	graded := false
	if floor, ok := floors[cur.Branch()]; ok {
		graded = true
		r.Floor = floor.String()
		if cur.Less(floor) {
			add(Fail, "below-floor", fmt.Sprintf("running %s, below the %s floor %s", cur, cur.Branch(), floor))
		}
	} else if len(floors) > 0 {
		oldest := true
		for _, f := range floors {
			if !cur.Less(Version{f.Major, f.Minor, 0}) {
				oldest = false
			}
		}
		if oldest {
			graded = true
			add(Fail, "unsupported-branch", fmt.Sprintf("running %s, a branch older than every configured floor", cur))
		}
	}
	// Patch currency needs evidence from the same branch: a floor for it, or
	// images the device is offered in it. Images from other branches alone
	// do not show whether this branch is current.
	if r.PatchesBehind > 0 {
		graded = true
		add(Warn, "patches-behind", fmt.Sprintf("running %s, %d patch(es) behind %s in the same branch", cur, r.PatchesBehind, r.LatestInBranch))
	} else if haveLatest {
		graded = true
	}
	if !graded {
		add(Unknown, "no-upgrade-data", fmt.Sprintf("running %s; no --min-version floor covers branch %s and the device offered no images in it", cur, cur.Branch()))
	}
	if len(r.Findings) == 0 {
		add(Pass, "current", fmt.Sprintf("running %s", cur))
	}
	r.Status = Status(r.Findings)
	return r
}

// ---------- SSL-VPN ----------

// interfaceExposure resolves whether a FortiOS interface name (a physical or
// VLAN interface, a zone, or the SD-WAN pseudo-interface) faces the WAN.
// known=false means the response did not establish it either way.
type interfaceExposure struct {
	roles map[string]string
	zones map[string][]string
}

func newInterfaceExposure(interfaces, zones []Obj) interfaceExposure {
	e := interfaceExposure{roles: map[string]string{}, zones: map[string][]string{}}
	for _, it := range interfaces {
		e.roles[Str(it, "name")] = strings.ToLower(Str(it, "role"))
	}
	for _, z := range zones {
		for _, m := range Objs(z["interface"]) {
			e.zones[Str(z, "name")] = append(e.zones[Str(z, "name")], Str(m, "interface-name"))
		}
	}
	return e
}

func (e interfaceExposure) wan(name string) (wan, known bool) {
	switch name {
	case "virtual-wan-link", "sdwan":
		return true, true
	}
	if role, ok := e.roles[name]; ok {
		// "undefined" is FortiOS's default role: it says nothing about exposure.
		return role == "wan", role != "" && role != "undefined"
	}
	if members, ok := e.zones[name]; ok && len(members) > 0 {
		allKnown := true
		for _, m := range members {
			w, k := e.wan(m)
			if w {
				return true, true
			}
			allKnown = allKnown && k
		}
		return false, allKnown
	}
	return false, false
}

// sslvpnPath is one way a client can reach the SSL-VPN listener: the
// interfaces it listens on and the source lists that apply there.
type sslvpnPath struct {
	rule      string // "" for the global settings, else the authentication-rule id
	ifaces    []string
	src, src6 []string
	neg, neg6 bool
}

func isEnabled(o Obj, key string) bool { return strings.EqualFold(Str(o, key), "enable") }

// sslvpnPaths applies FortiOS precedence between vpn.ssl/settings and its
// authentication-rule table. A parameter set under config authentication-rule
// replaces the same global parameter for that rule (Fortinet community tip
// "Set source address in SSL VPN settings", 96303: a global source-address
// does not take effect when the rule sets one); a parameter the rule leaves
// unset inherits the global value. Once a rule names its own
// source-interface, the global interfaces no longer listen, even for the
// default portal (tip 99903, "SSL VPN source-interface setting in
// authentication rule taking precedence"), so the global settings are a path
// of their own only when there are no rules or some rule listens on the
// global interfaces.
func sslvpnPaths(settings Obj) []sslvpnPath {
	global := sslvpnPath{
		ifaces: Names(settings, "source-interface"),
		src:    Names(settings, "source-address"),
		neg:    isEnabled(settings, "source-address-negate"),
		src6:   Names(settings, "source-address6"),
		neg6:   isEnabled(settings, "source-address6-negate"),
	}
	rules := Objs(settings["authentication-rule"])
	globalListens := len(rules) == 0
	var paths []sslvpnPath
	for i, r := range rules {
		p := global
		p.rule = Str(r, "id")
		if p.rule == "" {
			if id, ok := Num(r, "id"); ok {
				p.rule = strconv.Itoa(int(id))
			} else {
				p.rule = fmt.Sprintf("#%d", i+1)
			}
		}
		if n := Names(r, "source-interface"); len(n) > 0 {
			p.ifaces = n
		} else {
			globalListens = true
		}
		// A negate the rule enables applies even to an inherited list: it can
		// only widen, so reading it never hides exposure.
		if n := Names(r, "source-address"); len(n) > 0 {
			p.src, p.neg = n, isEnabled(r, "source-address-negate")
		} else if isEnabled(r, "source-address-negate") {
			p.neg = true
		}
		if n := Names(r, "source-address6"); len(n) > 0 {
			p.src6, p.neg6 = n, isEnabled(r, "source-address6-negate")
		} else if isEnabled(r, "source-address6-negate") {
			p.neg6 = true
		}
		paths = append(paths, p)
	}
	if globalListens {
		paths = append([]sslvpnPath{global}, paths...)
	}
	return paths
}

// SSLVPNSourceNames lists every source address name the global settings and
// the authentication rules use, per family, so the collector reads the
// address books whenever any of them needs resolving.
func SSLVPNSourceNames(settings Obj) (v4, v6 []string) {
	v4, v6 = Names(settings, "source-address"), Names(settings, "source-address6")
	for _, r := range Objs(settings["authentication-rule"]) {
		v4 = append(v4, Names(r, "source-address")...)
		v6 = append(v6, Names(r, "source-address6")...)
	}
	return v4, v6
}

// SSLVPN grades cmdb vpn.ssl/settings for one VDOM. present=false means the
// settings table does not exist on this build (no SSL-VPN to expose). zones
// (cmdb system/zone) resolve zone names used as listening interfaces; book4
// and book6 resolve named source addresses (an object covering 0.0.0.0/0 is
// as open as "all"). The global settings and every authentication rule are
// graded as separate paths (see sslvpnPaths) and the worst one wins, so a
// rule that opens the listener cannot hide behind a restricted global list.
func SSLVPN(device, vdom string, settings Obj, present bool, interfaces, zones []Obj, book4, book6 *AddressBook, sessions int) []Finding {
	f := func(status, code, detail string) []Finding {
		return []Finding{{Device: device, VDOM: vdom, Check: "sslvpn", Status: status, Code: code, Detail: detail}}
	}
	if !present {
		return f(Pass, "not-available", "this FortiOS build has no SSL-VPN settings table")
	}
	if !strings.EqualFold(Str(settings, "status"), "enable") {
		return f(Pass, "disabled", "SSL-VPN is disabled")
	}
	exposure := newInterfaceExposure(interfaces, zones)
	port := ""
	if p, ok := Num(settings, "port"); ok {
		port = fmt.Sprintf(" on port %d", int(p))
	}
	// Outcomes in precedence order. A path on an interface of unknown role
	// whose source is open (or may be) could be a FAIL, so it outranks WARN.
	const (
		rFail = iota
		rSourceUnresolved
		rRoleUnknownOpen
		rWarn
		rRoleUnknown
		rPass
	)
	best := rPass
	var details []string
	note := func(rank int, detail string) {
		switch {
		case rank < best:
			best, details = rank, []string{detail}
		case rank == best:
			details = append(details, detail)
		}
	}
	for _, p := range sslvpnPaths(settings) {
		via := ""
		if p.rule != "" {
			via = " via authentication rule " + p.rule
		}
		var onWAN, unresolved []string
		for _, n := range p.ifaces {
			wan, known := exposure.wan(n)
			switch {
			case wan:
				onWAN = append(onWAN, n)
			case !known:
				unresolved = append(unresolved, n)
			}
		}
		// negate inverts the list: "every source except these". Empty means any.
		any4, unsure4 := sourceAny(book4, p.src, p.neg, false)
		if len(p.src) == 0 {
			any4, unsure4 = true, false
		}
		// IPv6: only an explicit list is judged; an empty source-address6 is
		// the FortiOS default on builds that never configured IPv6 SSL-VPN.
		any6, unsure6 := false, false
		if len(p.src6) > 0 {
			any6, unsure6 = sourceAny(book6, p.src6, p.neg6, true)
		}
		anySource, unsure := any4 || any6, unsure4 || unsure6
		who := "with source address any"
		switch {
		case !any4:
			who = "with IPv6 source address any"
		case p.neg:
			who = "open to every source except " + strings.Join(p.src, ",")
		case len(p.src) > 0 && !contains(p.src, "all"):
			who = "with source address " + strings.Join(p.src, ",") + " (covers every address)"
		}
		wanList := strings.Join(onWAN, ",")
		switch {
		case len(onWAN) > 0 && anySource:
			note(rFail, fmt.Sprintf("SSL-VPN listens on WAN %s%s %s%s", wanList, port, who, via))
		case len(onWAN) > 0 && unsure:
			note(rSourceUnresolved, fmt.Sprintf("SSL-VPN listens on WAN %s%s%s; source addresses %s could not be resolved, so it may be open to any source", wanList, port, via, strings.Join(append(append([]string{}, p.src...), p.src6...), ",")))
		case len(unresolved) > 0 && (anySource || unsure):
			note(rRoleUnknownOpen, fmt.Sprintf("SSL-VPN is enabled%s on %s, whose WAN exposure could not be established (no role, role undefined, or unknown zone), with a source list that is or may be open to any address", via, strings.Join(unresolved, ",")))
		case len(onWAN) > 0:
			note(rWarn, fmt.Sprintf("SSL-VPN listens on WAN %s%s%s, restricted to %s", wanList, port, via, strings.Join(append(append([]string{}, p.src...), p.src6...), ",")))
		case len(unresolved) > 0:
			note(rRoleUnknown, fmt.Sprintf("SSL-VPN is enabled%s on %s, whose WAN exposure could not be established (no role, role undefined, or unknown zone)", via, strings.Join(unresolved, ",")))
		}
	}
	sess := fmt.Sprintf("; %d live session(s)", sessions)
	detail := strings.Join(details, "; ") + sess
	switch best {
	case rFail:
		return f(Fail, "open-to-internet", detail)
	case rSourceUnresolved:
		return f(Unknown, "source-unresolved", detail)
	case rRoleUnknownOpen, rRoleUnknown:
		return f(Unknown, "interface-role-unknown", detail)
	case rWarn:
		return f(Warn, "restricted-source", detail)
	default:
		return f(Pass, "enabled-internal", "SSL-VPN is enabled but not listening on a WAN-role interface"+sess)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// ---------- admins ----------

// PrefixLen converts "10.0.0.0 255.0.0.0", "10.0.0.0/8" or "::/0" to a
// prefix length. ok=false for unparseable input.
func PrefixLen(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, "/") {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return 0, false
		}
		ones, _ := n.Mask.Size()
		return ones, true
	}
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return 0, false
	}
	mask := net.ParseIP(parts[1]).To4()
	if mask == nil || net.ParseIP(parts[0]) == nil {
		return 0, false
	}
	ones, bits := net.IPMask(mask).Size()
	if bits == 0 {
		return 0, false
	}
	return ones, true
}

func unrestricted(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	n, ok := PrefixLen(s)
	return ok && n == 0
}

// adminRestricted reports whether an admin has at least one trusted host
// narrower than any. FortiOS defaults trusthostN to 0.0.0.0 0.0.0.0 (any).
func adminRestricted(admin Obj) bool {
	for i := 1; i <= 10; i++ {
		if !unrestricted(Str(admin, fmt.Sprintf("trusthost%d", i))) {
			return true
		}
	}
	return false
}

func accessList(v any) []string {
	switch x := v.(type) {
	case string:
		return strings.Fields(x)
	case []any:
		out := make([]string, 0, len(x))
		for _, it := range x {
			if s, ok := it.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func mgmtProtocols(access []string) []string {
	var protos []string
	for _, p := range access {
		switch strings.ToLower(p) {
		case "https", "ssh", "http", "telnet":
			protos = append(protos, strings.ToLower(p))
		}
	}
	return protos
}

// ManagementByExposure returns the management protocols enabled per interface
// for IPv4 (allowaccess, plus each secondaryip entry's own allowaccess) and
// IPv6 (ipv6.ip6-allowaccess), split into WAN-role interfaces and interfaces
// whose role does not establish exposure: no role, or FortiOS's default
// "undefined" (the rule the SSL-VPN check applies). LAN and DMZ roles are
// neither.
func ManagementByExposure(interfaces []Obj) (wan4, wan6, unknown4, unknown6 map[string][]string) {
	wan4, wan6 = map[string][]string{}, map[string][]string{}
	unknown4, unknown6 = map[string][]string{}, map[string][]string{}
	for _, it := range interfaces {
		var v4, v6 map[string][]string
		switch strings.ToLower(Str(it, "role")) {
		case "wan":
			v4, v6 = wan4, wan6
		case "", "undefined":
			v4, v6 = unknown4, unknown6
		default:
			continue
		}
		access := accessList(it["allowaccess"])
		// A secondary IP listens with its own allowaccess list, so management
		// can be open there while the primary address allows only ping. Only a
		// secondary-IP table the interface has switched off is ignored.
		if !strings.EqualFold(Str(it, "secondary-IP"), "disable") {
			for _, sec := range Objs(it["secondaryip"]) {
				access = append(access, accessList(sec["allowaccess"])...)
			}
		}
		if p := dedupe(mgmtProtocols(access)); len(p) > 0 {
			v4[Str(it, "name")] = p
		}
		if ip6, ok := it["ipv6"].(map[string]any); ok {
			if p := mgmtProtocols(accessList(ip6["ip6-allowaccess"])); len(p) > 0 {
				v6[Str(it, "name")] = p
			}
		}
	}
	return wan4, wan6, unknown4, unknown6
}

// WANManagement returns the management protocols enabled on WAN-role
// interfaces for IPv4 and IPv6 (see ManagementByExposure).
func WANManagement(interfaces []Obj) (v4, v6 map[string][]string) {
	v4, v6, _, _ = ManagementByExposure(interfaces)
	return v4, v6
}

func dedupe(list []string) []string {
	seen := map[string]bool{}
	out := list[:0:0]
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func adminIPv6Open(admin Obj) bool {
	for i := 1; i <= 10; i++ {
		if v := Str(admin, fmt.Sprintf("ip6-trusthost%d", i)); v != "" && !unrestricted(v) {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Admins grades system/admin, system/accprofile, system/api-user and
// WAN-role interface management access.
func Admins(device string, admins, profiles, apiUsers, interfaces []Obj, broadPrefix int) []Finding {
	out := []Finding{}
	add := func(status, code, object, detail string) {
		out = append(out, Finding{Device: device, Check: "admins", Status: status, Code: code, Object: object, Detail: detail})
	}
	wan4, wan6, unknown4, unknown6 := ManagementByExposure(interfaces)
	for family, m := range map[string]map[string][]string{"IPv4": wan4, "IPv6": wan6} {
		for _, name := range sortedKeys(m) {
			for _, p := range m[name] {
				if p == "http" || p == "telnet" {
					add(Fail, "wan-plaintext-mgmt", name, fmt.Sprintf("WAN interface %s allows %s (unencrypted) management over %s", name, p, family))
				}
			}
		}
	}
	// An interface whose role does not establish exposure may face the
	// internet: plaintext management there is UNKNOWN, never PASS.
	for family, m := range map[string]map[string][]string{"IPv4": unknown4, "IPv6": unknown6} {
		for _, name := range sortedKeys(m) {
			for _, p := range m[name] {
				if p == "http" || p == "telnet" {
					add(Unknown, "plaintext-mgmt-unknown-role", name, fmt.Sprintf("interface %s allows %s (unencrypted) management over %s, and its WAN exposure could not be established (no role or role undefined)", name, p, family))
				}
			}
		}
	}
	wan4Names, wan6Names := sortedKeys(wan4), sortedKeys(wan6)
	unknown4Names, unknown6Names := sortedKeys(unknown4), sortedKeys(unknown6)
	superProfiles := map[string]bool{"super_admin": true}
	for _, p := range profiles {
		if isSuperProfile(p) {
			superProfiles[Str(p, "name")] = true
		}
	}
	for _, a := range admins {
		name := Str(a, "name")
		super := superProfiles[Str(a, "accprofile")]
		mfa := !strings.EqualFold(Str(a, "two-factor"), "disable") && Str(a, "two-factor") != ""
		remote := strings.EqualFold(Str(a, "remote-auth"), "enable")
		// Each address family is judged on its own: an IPv6 restriction does
		// not narrow who can reach the admin over IPv4, and vice versa.
		v4Open, v6Open := !adminRestricted(a), adminIPv6Open(a)
		exposed := false
		if v4Open && len(wan4Names) > 0 {
			exposed = true
			add(Fail, "admin-anywhere-wan-mgmt", name, fmt.Sprintf("admin %s has no IPv4 trusted hosts and management is reachable on WAN (%s)", name, strings.Join(wan4Names, ",")))
		} else if v4Open && len(unknown4Names) > 0 {
			add(Unknown, "admin-anywhere-mgmt-unknown-role", name, fmt.Sprintf("admin %s has no IPv4 trusted hosts and management is enabled on %s, whose WAN exposure could not be established (no role or role undefined)", name, strings.Join(unknown4Names, ",")))
		}
		if v6Open && len(wan6Names) > 0 {
			exposed = true
			add(Fail, "admin-anywhere-wan-mgmt-ipv6", name, fmt.Sprintf("admin %s has no IPv6 trusted hosts and IPv6 management is reachable on WAN (%s)", name, strings.Join(wan6Names, ",")))
		} else if v6Open && len(unknown6Names) > 0 {
			add(Unknown, "admin-anywhere-mgmt-unknown-role-ipv6", name, fmt.Sprintf("admin %s has no IPv6 trusted hosts and IPv6 management is enabled on %s, whose WAN exposure could not be established (no role or role undefined)", name, strings.Join(unknown6Names, ",")))
		}
		if v4Open && !exposed {
			add(Warn, "admin-no-trusthost", name, fmt.Sprintf("admin %s has no trusted-host restriction", name))
		}
		if !mfa && !remote {
			if super {
				add(Fail, "super-admin-no-2fa", name, fmt.Sprintf("super admin %s has no two-factor authentication", name))
			} else {
				add(Warn, "admin-no-2fa", name, fmt.Sprintf("admin %s has no two-factor authentication", name))
			}
		}
	}
	for _, u := range apiUsers {
		name := Str(u, "name")
		if superProfiles[Str(u, "accprofile")] {
			add(Warn, "api-user-super-admin", name, fmt.Sprintf("API user %s uses a super_admin profile; a read-only profile is enough for auditing", name))
		}
		hosts := Objs(u["trusthost"])
		if len(hosts) == 0 {
			add(Warn, "api-user-no-trusthost", name, fmt.Sprintf("API user %s lists no trusted hosts", name))
		}
		for _, h := range hosts {
			// Each entry's type selects which field is live (default IPv4); the
			// other field keeps its default and is ignored by FortiOS. IPv6 uses
			// the same breadth shifted by 32 bits (/16 -> /48).
			key, limit := "ipv4-trusthost", broadPrefix
			if strings.EqualFold(Str(h, "type"), "ipv6-trusthost") {
				key, limit = "ipv6-trusthost", broadPrefix+32
			}
			cidr := Str(h, key)
			if n, ok := PrefixLen(cidr); ok && n < limit {
				add(Warn, "api-user-broad-trusthost", name, fmt.Sprintf("API user %s trusts %s (/%d, broader than /%d)", name, cidr, n, limit))
			}
		}
	}
	if len(out) == 0 {
		add(Pass, "ok", "", fmt.Sprintf("%d admin(s) and %d API user(s) restricted", len(admins), len(apiUsers)))
	}
	return out
}

func isSuperProfile(p Obj) bool {
	if !strings.EqualFold(Str(p, "scope"), "global") {
		return false
	}
	for _, g := range []string{"secfabgrp", "ftviewgrp", "authgrp", "sysgrp", "netgrp", "loggrp", "fwgrp", "vpngrp", "utmgrp", "wifi"} {
		if !strings.EqualFold(Str(p, g), "read-write") {
			return false
		}
	}
	return true
}

// ---------- licences ----------

// LicenseRow is one entry of the fleet renewal queue.
type LicenseRow struct {
	Device   string `json:"device"`
	Serial   string `json:"serial,omitempty"`
	Model    string `json:"model,omitempty"`
	Service  string `json:"service"`
	Status   string `json:"license_status"`
	Expires  string `json:"expires_at"`
	DaysLeft int    `json:"days_left"`
	Grade    string `json:"status"`
}

// Licenses grades monitor license/status results: map of service -> state,
// with `expires` as Unix epoch seconds. FortiCare nests support contracts
// under support.{hardware,comprehensive,enhanced}.
func Licenses(device, serial, model string, results Obj, now time.Time, within time.Duration) ([]LicenseRow, []Finding) {
	rows := []LicenseRow{}
	findings := []Finding{}
	visit := func(service string, o Obj) {
		exp, ok := Num(o, "expires")
		if !ok || exp <= 0 {
			if strings.EqualFold(Str(o, "status"), "expired") {
				findings = append(findings, Finding{Device: device, Check: "licenses", Status: Fail, Code: "expired", Object: service, Detail: service + " is expired"})
			}
			return
		}
		at := time.Unix(int64(exp), 0).UTC()
		days := calendarDays(now, at)
		grade := Pass
		switch {
		case at.Before(now) || strings.EqualFold(Str(o, "status"), "expired"):
			grade = Fail
		case at.Sub(now) <= within:
			grade = Warn
		}
		rows = append(rows, LicenseRow{Device: device, Serial: serial, Model: model, Service: service, Status: Str(o, "status"), Expires: at.Format("2006-01-02"), DaysLeft: days, Grade: grade})
		if grade != Pass {
			code := "expiring"
			if grade == Fail {
				code = "expired"
			}
			detail := fmt.Sprintf("%s expires %s (in %d days)", service, at.Format("2006-01-02"), days)
			if days < 0 {
				detail = fmt.Sprintf("%s expired %s (%d days ago)", service, at.Format("2006-01-02"), -days)
			}
			findings = append(findings, Finding{Device: device, Check: "licenses", Status: grade, Code: code, Object: service, Detail: detail})
		}
	}
	keys := make([]string, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		o, ok := results[k].(map[string]any)
		if !ok {
			continue
		}
		visit(k, o)
		if support, ok := o["support"].(map[string]any); ok {
			for _, tier := range []string{"hardware", "comprehensive", "enhanced"} {
				if t, ok := support[tier].(map[string]any); ok {
					visit(k+"."+tier, t)
				}
			}
		}
	}
	if len(rows) == 0 && len(findings) == 0 {
		findings = append(findings, Finding{Device: device, Check: "licenses", Status: Unknown, Code: "no-expiry-data", Detail: "license/status returned no expiry dates"})
	} else if len(findings) == 0 {
		findings = append(findings, Finding{Device: device, Check: "licenses", Status: Pass, Code: "ok", Detail: fmt.Sprintf("%d licence(s), none expiring within %s", len(rows), within)})
	}
	return rows, findings
}

// calendarDays counts UTC calendar days from now to at (negative when past).
func calendarDays(now, at time.Time) int {
	d := func(t time.Time) time.Time {
		y, m, dd := t.UTC().Date()
		return time.Date(y, m, dd, 0, 0, 0, 0, time.UTC)
	}
	return int(math.Round(d(at).Sub(d(now)).Hours() / 24))
}

// ---------- policies ----------

// PolicyRow is one flagged firewall policy.
type PolicyRow struct {
	Device   string   `json:"device"`
	VDOM     string   `json:"vdom"`
	PolicyID int      `json:"policyid"`
	Name     string   `json:"name,omitempty"`
	Issues   []string `json:"issues"`
	HitCount *float64 `json:"hit_count,omitempty"`
	LastUsed string   `json:"last_used,omitempty"`
}

// Policies flags unused, any-any accept, logging-disabled and disabled
// policies. A policy is only "unused" when a counter row exists for it:
// without counters there is no evidence the rule is dead.
func Policies(device, vdom string, policies, stats []Obj, now time.Time, unusedFor time.Duration) []PolicyRow {
	byID := map[int]Obj{}
	for _, s := range stats {
		if id, ok := Num(s, "policyid"); ok {
			byID[int(id)] = s
		}
	}
	out := []PolicyRow{}
	for _, p := range policies {
		idf, _ := Num(p, "policyid")
		id := int(idf)
		row := PolicyRow{Device: device, VDOM: vdom, PolicyID: id, Name: Str(p, "name"), Issues: []string{}}
		accept := strings.EqualFold(Str(p, "action"), "accept")
		disabled := strings.EqualFold(Str(p, "status"), "disable")
		if disabled {
			row.Issues = append(row.Issues, "disabled")
		}
		// Any-any per address family (consolidated policies carry IPv6 in
		// srcaddr6/dstaddr6). A negated "all" matches nothing.
		on := func(k string) bool { return strings.EqualFold(Str(p, k), "enable") }
		anyFamily := func(src, dst string) bool {
			return contains(Names(p, src), "all") && contains(Names(p, dst), "all") && !on(src+"-negate") && !on(dst+"-negate")
		}
		if accept && !disabled && (anyFamily("srcaddr", "dstaddr") || anyFamily("srcaddr6", "dstaddr6")) && contains(Names(p, "service"), "ALL") && !on("service-negate") {
			row.Issues = append(row.Issues, "any-any-accept")
		}
		if accept && !disabled && strings.EqualFold(Str(p, "logtraffic"), "disable") {
			row.Issues = append(row.Issues, "logging-disabled")
		}
		// Usage is judged only from a counter row that actually carries hit_count.
		if s, ok := byID[id]; ok && !disabled {
			if hits, hasHits := Num(s, "hit_count"); hasHits {
				row.HitCount = &hits
				last, hasLast := Num(s, "last_used")
				if hasLast && last > 0 {
					row.LastUsed = time.Unix(int64(last), 0).UTC().Format(time.RFC3339)
				}
				if hits == 0 || (hasLast && last > 0 && now.Sub(time.Unix(int64(last), 0)) > unusedFor) {
					row.Issues = append(row.Issues, "unused")
				}
			}
		}
		if len(row.Issues) > 0 {
			out = append(out, row)
		}
	}
	return out
}
