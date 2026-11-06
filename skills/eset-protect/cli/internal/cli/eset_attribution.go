// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "customer-views-count-unattributed-rows".
//
// One attribution rule for every customer-scoped view. A row belongs to the
// nearest COMPANY ancestor of its own device group, or of its device's synced
// parent group. When the mirror holds neither (the device was deleted, or
// enrolled after the last devices sync; its group was created after the last
// device-groups sync) the owner is UNKNOWN, which is not the same as "another
// customer's": the row may be this customer's. A customer-scoped view never
// credits such a row and never drops it silently either. It counts it in an
// unattributed_* field (with the uuids), marks the section it would have
// landed in incomplete, and never prints an all-clear while the count is
// non-zero. triage's unattributed_incidents/_detections follow the same rule.
package cli

import (
	"fmt"
	"sort"
	"strings"
)

// esetUnattributedShown caps how many uuids one human line names.
const esetUnattributedShown = 5

// ownerOf returns the customer owning groupUUID and whether the synced tree
// can prove it: the verdict of the one ancestry walk (esetTree.resolve). A
// tree with no COMPANY group is single-tenant: every group is the tenant's.
// Otherwise a group whose walk meets a group the tree does not hold (the
// group itself or an interior ancestor) or loops is owner-unknown ("(group
// not in synced tree)"), never "(unassigned)": unassigned means a walk that
// reached the root, an MSP group or the hidden scope root of a scoped API
// user through synced groups without passing a COMPANY.
func (t *esetTree) ownerOf(groupUUID string) (esetGroup, bool) {
	return t.resolve(groupUUID)
}

// esetDeviceOwner returns the customer owning the row of a device and whether
// the mirror can say. A tree with no COMPANY group is single-tenant: every
// row is the tenant's, whatever device it names. Otherwise a device missing
// from the synced devices is "(unknown device)", and a synced device whose
// group's ancestry the tree cannot walk is "(group not in synced tree)".
func esetDeviceOwner(tree *esetTree, byUUID map[string]esetDevice, deviceUUID string) (esetGroup, bool) {
	if tree != nil && !tree.hasCompanies {
		return esetGroup{Name: esetCustomerTenant}, true
	}
	d, ok := byUUID[deviceUUID]
	if deviceUUID == "" || !ok {
		return esetGroup{Name: detectionsSpreadUnknownDevice}, false
	}
	return tree.ownerOf(d.ParentGroupUUID)
}

// esetAttribution is how a customer-scoped view treats one row.
type esetAttribution int

const (
	esetAttribIn      esetAttribution = iota // the target customer's row
	esetAttribOther                          // a known owner that is not the target
	esetAttribUnknown                        // no synced device or group ties it to a customer
)

// esetAttribute classifies a row whose owner is (owner, known) against the
// target customer UUID.
func esetAttribute(owner esetGroup, known bool, targetUUID string) esetAttribution {
	switch {
	case !known:
		return esetAttribUnknown
	case owner.UUID != "" && owner.UUID == targetUUID:
		return esetAttribIn
	default:
		return esetAttribOther
	}
}

// esetCountsAsCustomer reports whether a known owner is counted as a
// customer: a COMPANY group, or the one "(tenant)" of a single-tenant
// account. "(unassigned)" (the MSP's own devices, or rows under the hidden
// scope root) is listed beside the customers but is never one of them, so a
// count or threshold of customers never includes it (handfixes.json:
// customer-counts-exclude-unassigned).
func esetCountsAsCustomer(g esetGroup) bool {
	return g.UUID != "" || g.Name == esetCustomerTenant
}

// esetOwnerUnknownBucket reports whether name is one of the owner-unknown
// buckets resolve returns.
func esetOwnerUnknownBucket(name string) bool {
	return name == esetCustomerUnknownGroup || name == esetCustomerParentHidden
}

// esetDevicesByUUID indexes synced devices by UUID.
func esetDevicesByUUID(devices []esetDevice) map[string]esetDevice {
	out := make(map[string]esetDevice, len(devices))
	for _, d := range devices {
		out[d.UUID] = d
	}
	return out
}

// owner is customerFor plus whether the owner is known (see customerFor and
// esetTree.resolve). A tree with no COMPANY group is single-tenant, so every
// row is the tenant's (known); with no tree at all the scope is unsynced and
// nothing is attributed (callers report that separately).
func (s exposureScope) owner(groupUUID, deviceUUID string) (esetGroup, bool) {
	if s.unsynced || s.tree == nil {
		return esetGroup{Name: exposureUnsyncedBucket}, true
	}
	g := groupUUID
	if g == "" {
		if d, ok := s.devices[deviceUUID]; ok {
			g = d.ParentGroupUUID
		}
	}
	if _, synced := s.tree.groups[g]; synced {
		return s.tree.resolve(g)
	}
	d, found := s.devices[deviceUUID]
	if found {
		if _, synced := s.tree.groups[d.ParentGroupUUID]; synced {
			return s.tree.resolve(d.ParentGroupUUID)
		}
	}
	// Neither group is synced. A row (or its device) in the hidden scope
	// root is no visible customer's (esetTree.resolve); anything else is
	// owner-unknown.
	if root := s.tree.scopeRoot; root != "" && (g == root || (found && d.ParentGroupUUID == root)) {
		return s.tree.resolve(root)
	}
	return s.tree.resolve("")
}

// esetUnattributed counts the rows a view could not tie to any customer.
// N counts rows; UUIDs lists each distinct id once, sorted.
type esetUnattributed struct {
	N    int
	seen map[string]bool
	ids  []string
}

func (u *esetUnattributed) add(id string) {
	u.N++
	if id == "" {
		return
	}
	if u.seen == nil {
		u.seen = map[string]bool{}
	}
	if !u.seen[id] {
		u.seen[id] = true
		u.ids = append(u.ids, id)
	}
}

// UUIDs returns the distinct ids, sorted (nil when there are none, so an
// omitempty field stays out of the JSON).
func (u esetUnattributed) UUIDs() []string {
	if len(u.ids) == 0 {
		return nil
	}
	out := append([]string(nil), u.ids...)
	sort.Strings(out)
	return out
}

// esetUnattributedLine is the human line for n unattributed rows: what they
// are, why they could not be placed, the first few ids and what to do. scope
// is the customer the view was limited to ("" when unscoped). hidden names
// the synced groups whose parent this API user cannot see
// (esetTree.parentNotVisible): the line then says so, and that a sync does
// not attribute rows under them, instead of letting the resync advice in
// next read as a promise for them.
func esetUnattributedLine(n int, what string, ids []string, scope, next string, hidden []string) string {
	if n <= 0 {
		return ""
	}
	where := "any customer"
	if scope != "" {
		where = scope + " or another customer"
	}
	shown := ids
	more := ""
	if len(shown) > esetUnattributedShown {
		more = fmt.Sprintf(" (+%d more)", len(shown)-esetUnattributedShown)
		shown = shown[:esetUnattributedShown]
	}
	list := ""
	if len(shown) > 0 {
		list = ": " + strings.Join(shown, ", ") + more
	}
	line := fmt.Sprintf("%d %s could not be tied to %s, because %s%s%s.", n, what, where, esetUnattributedCause, esetHiddenParentCause(hidden), list)
	if scope != "" {
		line += " They are not in this customer's totals."
	}
	if next != "" {
		line += " " + next
	}
	if h := esetHiddenParentRemedy(hidden); h != "" {
		line += " " + h
	}
	return line
}

// esetHiddenParentCause is the cause clause for rows under groups whose
// parent group this API user cannot see (handfixes.json:
// unattributed-cause-names-hidden-parent); "" when hidden is empty.
func esetHiddenParentCause(hidden []string) string {
	if len(hidden) == 0 {
		return ""
	}
	return "; and the parent group of " + esetQuotedList(hidden) + " is not visible to this API user (outside its permission scope), so rows under " + esetPronoun(hidden) + " cannot be tied to a customer"
}

// esetHiddenParentRemedy replaces the resync promise for those rows.
func esetHiddenParentRemedy(hidden []string) string {
	if len(hidden) == 0 {
		return ""
	}
	possessive := "its"
	if len(hidden) > 1 {
		possessive = "their"
	}
	return "A sync does not attribute rows under " + esetQuotedList(hidden) + ": give the API user access to " + possessive + " parent group in ESET PROTECT (or accept them as unattributed)."
}

func esetQuotedList(names []string) string {
	shown := names
	more := ""
	if len(shown) > esetUnattributedShown {
		more = fmt.Sprintf(" (+%d more)", len(shown)-esetUnattributedShown)
		shown = shown[:esetUnattributedShown]
	}
	q := make([]string, len(shown))
	for i, n := range shown {
		q[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(q, ", ") + more
}

func esetPronoun(names []string) string {
	if len(names) == 1 {
		return "it"
	}
	return "them"
}

// esetUnattributedCause names every way a row ends owner-unknown
// (esetTree.resolve, esetDeviceOwner), without claiming which one: the
// device and its own group can both be in the mirror while a parent group
// is not. A row under the hidden root of a scoped API user, or under an MSP
// group, is never owner-unknown, so the cause never blames either. When a
// synced group's parent is not visible to the API user, the line adds
// esetHiddenParentCause and esetHiddenParentRemedy.
const esetUnattributedCause = "they name no device, or the local mirror does not hold their device, its device group or a parent group of it (deleted, or added after the last sync), or the group ancestry loops"

// esetUnattributedResync is the usual remedy for owner-unknown rows.
const esetUnattributedResync = "Run '" + esetCLIName + " sync --resources devices,device-groups' to attribute them."

// esetCustomerUUID is a customer's uuid for JSON output, nil (null) for a
// bucket ("(unassigned)", "(group not in synced tree)", "(parent group not
// visible)", "(tenant)"), whose group carries no uuid (handfixes.json:
// customer-uuid-beside-customer-name).
func esetCustomerUUID(g esetGroup) *string {
	if g.UUID == "" {
		return nil
	}
	u := g.UUID
	return &u
}
