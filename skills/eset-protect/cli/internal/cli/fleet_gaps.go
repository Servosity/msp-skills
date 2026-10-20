// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// Hand-authored. `fleet gaps`: devices missing a component their peers have,
// running an older component version, unlicensed, or not enrolled.

package cli

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const (
	fleetGapsMissing    = "missing"
	fleetGapsOutdated   = "outdated"
	fleetGapsUnlicensed = "unlicensed"
	fleetGapsUnenrolled = "unenrolled"

	fleetGapsMinGroupSize     = 3
	fleetGapsDefaultThreshold = 0.5
	fleetGapsDefaultLimit     = 200
	fleetGapsScopeCustomer    = "customer"
	fleetGapsScopeFleet       = "fleet"
)

// fleetGapsKinds lists the gap kinds in report order.
var fleetGapsKinds = []string{fleetGapsMissing, fleetGapsOutdated, fleetGapsUnlicensed, fleetGapsUnenrolled}

type fleetGapsRow struct {
	Customer      string   `json:"customer"`
	Device        string   `json:"device"`
	DeviceUUID    string   `json:"device_uuid"`
	Gap           string   `json:"gap"`
	Component     string   `json:"component"`
	HaveVersion   string   `json:"have_version"`
	NewestVersion string   `json:"newest_version"`
	PeerShare     *float64 `json:"peer_share"`
	Detail        string   `json:"detail"`
	// Uncertain is true when the gap holds for some, not every, placement of
	// the devices whose customer is unknown: they may be this device's peers
	// (handfixes.json: spread-counts-uncertainty-before-thresholds).
	Uncertain bool `json:"uncertain"`
}

type fleetGapsView struct {
	Gaps               []fleetGapsRow `json:"gaps"`
	CountsByGap        map[string]int `json:"counts_by_gap"`
	DevicesChecked     int            `json:"devices_checked"`
	SkippedSmallGroups int            `json:"skipped_small_groups"`
	PeerThreshold      float64        `json:"peer_threshold"`
	Scope              string         `json:"scope"`
	// UncertainGaps counts the gaps (before --limit) marked uncertain.
	UncertainGaps int `json:"uncertain_gaps"`
	// UnattributedDevices counts synced devices whose customer is unknown
	// (their group, or an ancestor of it, is not in the synced tree). They
	// are never pooled into a peer group of their own: without --customer
	// they are checked only for unlicensed and unenrolled (as "(group not in
	// synced tree)"); with it they are not checked, because they may be that
	// customer's. In the customer scope every other customer's
	// missing/outdated verdict accounts for them joining its peers: a gap
	// that holds only for some placement is listed with uncertain true
	// (handfixes.json: customer-views-count-unattributed-rows,
	// spread-counts-uncertainty-before-thresholds).
	UnattributedDevices     int      `json:"unattributed_devices"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
	// SyncedHint is set when the mirror is missing or a needed resource has no
	// completed sync, so an empty gaps list is not read as full coverage.
	SyncedHint string `json:"synced_hint,omitempty"`

	customer string // --customer name for the human output
	// parentNotVisible names the synced groups whose parent this API user
	// cannot see (esetTree.parentNotVisible), for the unattributed line.
	parentNotVisible []string
	// ParentNotVisible is the same set for agents: each synced group (uuid
	// and name) whose parent this API user cannot see. Rows under them stay
	// unattributed after any sync (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	ParentNotVisible []esetGroupRef `json:"parent_not_visible,omitempty"`
}

type fleetGapsOptions struct {
	Kinds     map[string]bool
	Threshold float64
	Fleet     bool
	// Customer limits which devices are checked (peers are unaffected).
	Customer    esetGroup
	HasCustomer bool
}

// fleetGapsParseOnly turns the --only csv into a kind set; empty means all.
func fleetGapsParseOnly(s string) (map[string]bool, error) {
	kinds := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		p := strings.ToLower(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		valid := false
		for _, k := range fleetGapsKinds {
			if p == k {
				valid = true
			}
		}
		if !valid {
			return nil, fmt.Errorf("unknown --only value %q (valid: %s)", p, strings.Join(fleetGapsKinds, ", "))
		}
		kinds[p] = true
	}
	if len(kinds) == 0 {
		for _, k := range fleetGapsKinds {
			kinds[k] = true
		}
	}
	return kinds, nil
}

// fleetGapsParseDotted reads "11.1.2039.2" (an optional leading "v" allowed)
// into numeric segments; nil when the text is not a dotted number.
func fleetGapsParseDotted(s string) []int64 {
	s = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(s)), "v")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// fleetGapsVersionKey returns comparable numeric segments for a version:
// major.minor.patch, extended by a dotted name such as "11.1.2039.2" when
// the name agrees with those numbers, or the dotted name alone when the
// numeric fields are absent. nil means the version is unknown.
func fleetGapsVersionKey(v esetVersion) []int64 {
	named := fleetGapsParseDotted(v.Name)
	if v.Major == 0 && v.Minor == 0 && v.Patch == 0 {
		return named
	}
	num := []int64{int64(v.Major), int64(v.Minor), int64(v.Patch)}
	if len(named) > 3 && named[0] == num[0] && named[1] == num[1] && named[2] == num[2] {
		return named
	}
	return num
}

// fleetGapsVersionCmp compares two version keys; missing segments count as 0.
func fleetGapsVersionCmp(a, b []int64) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var x, y int64
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// fleetGapsComponentName keys a component by name, falling back to displayName.
func fleetGapsComponentName(c esetComponent) string {
	if c.Name != "" {
		return c.Name
	}
	return c.DisplayName
}

// fleetGapsComponents returns one entry per component name on a device,
// keeping the newest version when a name repeats.
func fleetGapsComponents(d esetDevice) map[string]esetComponent {
	out := map[string]esetComponent{}
	for _, c := range d.Components {
		name := fleetGapsComponentName(c)
		if name == "" {
			continue
		}
		if prev, ok := out[name]; ok && fleetGapsVersionCmp(fleetGapsVersionKey(c.Version), fleetGapsVersionKey(prev.Version)) <= 0 {
			continue
		}
		out[name] = c
	}
	return out
}

type fleetGapsResult struct {
	Gaps               []fleetGapsRow
	Counts             map[string]int
	DevicesChecked     int
	SkippedSmallGroups int
	Uncertain          int
	Unattributed       esetUnattributed
}

// fleetGapsMissingVerdict decides a missing component for a device that lacks
// it. p of the device's peers carry it, the device's known group has size
// members, and of the devices whose customer is unknown (any subset of which
// may be this group's) uWith carry it and uWithout do not. possible: some
// subset makes it a gap (a group of at least fleetGapsMinGroupSize with a
// peer share at or above threshold); proven: every subset does. Adding a
// peer that has the component never lowers the share, so the best case takes
// all of them (padded with ones that lack it only to reach the group size)
// and the worst case adds every one that lacks it.
func fleetGapsMissingVerdict(p, members, uWith, uWithout int, threshold float64) (possible, proven bool) {
	peers := members - 1
	if members >= fleetGapsMinGroupSize && peers+uWithout > 0 {
		proven = float64(p)/float64(peers+uWithout)+1e-9 >= threshold
	}
	need := fleetGapsMinGroupSize - members
	if need < 0 {
		need = 0
	}
	if uWith+uWithout < need {
		return false, false
	}
	take, pad := uWith, 0
	if take < need {
		pad = need - take
	}
	denom := peers + take + pad
	possible = denom > 0 && float64(p+take)/float64(denom)+1e-9 >= threshold
	return possible, proven
}

// fleetGapsIntrinsic adds the gaps a device has on its own (no peers needed).
func fleetGapsIntrinsic(res *fleetGapsResult, d esetDevice, c esetGroup, opts fleetGapsOptions) {
	add := func(gap, detail string) {
		res.Gaps = append(res.Gaps, fleetGapsRow{Customer: c.Name, Device: d.name(), DeviceUUID: d.UUID, Gap: gap, Detail: detail})
		res.Counts[gap]++
	}
	if opts.Kinds[fleetGapsUnlicensed] && len(d.ActiveProducts) == 0 {
		add(fleetGapsUnlicensed, "no active ESET product")
	}
	if opts.Kinds[fleetGapsUnenrolled] && fleetHealthUnenrolled(d.EnrollmentStatus) {
		add(fleetGapsUnenrolled, "enrollment "+shortEnum(d.EnrollmentStatus, "DEVICE_ENROLLMENT_STATUS_"))
	}
}

// fleetGapsFind compares every in-scope device with its peer group (its
// customer, or the whole fleet) and returns the gaps, sorted by customer,
// device, gap kind and component.
func fleetGapsFind(devices []esetDevice, tree *esetTree, opts fleetGapsOptions) fleetGapsResult {
	res := fleetGapsResult{Gaps: make([]fleetGapsRow, 0), Counts: map[string]int{}}
	for k := range opts.Kinds {
		res.Counts[k] = 0
	}
	type member struct {
		dev      esetDevice
		customer esetGroup
		comps    map[string]esetComponent
	}
	groups := map[string][]member{}
	groupOrder := make([]string, 0)
	// unknown holds, in the customer scope, the devices whose customer is
	// unknown. They are never a peer group of their own (they may belong to
	// different customers); each may be a peer of any known group.
	var unknown []member
	for _, d := range devices {
		// A device whose group's ancestry the tree cannot walk is
		// owner-unknown, not unassigned.
		c, known := tree.ownerOf(d.ParentGroupUUID)
		if !known {
			res.Unattributed.add(d.UUID)
			if !opts.Fleet {
				unknown = append(unknown, member{dev: d, customer: c, comps: fleetGapsComponents(d)})
				continue
			}
		}
		key := fleetGapsScopeFleet
		if !opts.Fleet {
			key = c.UUID
			if key == "" {
				key = "bucket:" + c.Name
			}
		}
		if _, ok := groups[key]; !ok {
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], member{dev: d, customer: c, comps: fleetGapsComponents(d)})
	}

	newer := func(into map[string]esetComponent, name string, c esetComponent) {
		if prev, ok := into[name]; !ok || fleetGapsVersionCmp(fleetGapsVersionKey(prev.Version), fleetGapsVersionKey(c.Version)) < 0 {
			into[name] = c
		}
	}
	// What the unknown-owner devices carry, for the uncertain verdicts.
	uPrevalence := map[string]int{}
	uNewest := map[string]esetComponent{}
	for _, m := range unknown {
		for name, c := range m.comps {
			uPrevalence[name]++
			newer(uNewest, name, c)
		}
	}
	for _, key := range groupOrder {
		members := groups[key]
		prevalence := map[string]int{}
		newest := map[string]esetComponent{}
		for _, m := range members {
			for name, c := range m.comps {
				prevalence[name]++
				newer(newest, name, c)
			}
		}
		// newestAll adds the unknown-owner devices, which may be peers.
		newestAll := map[string]esetComponent{}
		for name, c := range newest {
			newestAll[name] = c
		}
		for name, c := range uNewest {
			newer(newestAll, name, c)
		}
		seenName := map[string]bool{}
		names := make([]string, 0, len(prevalence)+len(uPrevalence))
		for name := range prevalence {
			seenName[name] = true
			names = append(names, name)
		}
		for name := range uPrevalence {
			if !seenName[name] {
				names = append(names, name)
			}
		}
		sort.Strings(names)

		inScope := 0
		for _, m := range members {
			if opts.HasCustomer && m.customer.UUID != opts.Customer.UUID {
				continue
			}
			inScope++
			res.DevicesChecked++
			row := func(gap, component, have, newestV, detail string, share *float64, uncertain bool) {
				if uncertain {
					detail += fmt.Sprintf("; uncertain: %d device(s) of unknown customer may be peers", len(unknown))
					res.Uncertain++
				}
				res.Gaps = append(res.Gaps, fleetGapsRow{
					Customer: m.customer.Name, Device: m.dev.name(), DeviceUUID: m.dev.UUID, Gap: gap,
					Component: component, HaveVersion: have, NewestVersion: newestV, PeerShare: share, Detail: detail, Uncertain: uncertain,
				})
				res.Counts[gap]++
			}
			if opts.Kinds[fleetGapsMissing] {
				peers := len(members) - 1
				for _, name := range names {
					if _, has := m.comps[name]; has {
						continue
					}
					uWith := uPrevalence[name]
					possible, proven := fleetGapsMissingVerdict(prevalence[name], len(members), uWith, len(unknown)-uWith, opts.Threshold)
					if !possible {
						continue
					}
					var share *float64
					if peers > 0 {
						rounded := math.Round(float64(prevalence[name])/float64(peers)*1000) / 1000
						share = &rounded
					}
					row(fleetGapsMissing, name, "", newestAll[name].Version.String(),
						fmt.Sprintf("on %d of %d peers", prevalence[name], peers), share, !proven)
				}
			}
			if opts.Kinds[fleetGapsOutdated] {
				for _, name := range names {
					have, ok := m.comps[name]
					if !ok {
						continue
					}
					hk := fleetGapsVersionKey(have.Version)
					if hk == nil {
						continue
					}
					nk := fleetGapsVersionKey(newest[name].Version)
					proven := nk != nil && fleetGapsVersionCmp(hk, nk) < 0
					nAll := newestAll[name]
					if !proven {
						ak := fleetGapsVersionKey(nAll.Version)
						if ak == nil || fleetGapsVersionCmp(hk, ak) >= 0 {
							continue
						}
					} else {
						nAll = newest[name]
					}
					hv, nv := have.Version.String(), nAll.Version.String()
					row(fleetGapsOutdated, name, hv, nv, fmt.Sprintf("%s is older than %s", hv, nv), nil, !proven)
				}
			}
			fleetGapsIntrinsic(&res, m.dev, m.customer, opts)
		}
		if opts.Kinds[fleetGapsMissing] && inScope > 0 && len(members) < fleetGapsMinGroupSize {
			res.SkippedSmallGroups++
		}
	}
	// Unknown-owner devices have no peer group; without --customer they are
	// still checked for the gaps that need none (with --customer they may be
	// that customer's or another's, so they are only counted).
	if !opts.HasCustomer {
		for _, m := range unknown {
			res.DevicesChecked++
			fleetGapsIntrinsic(&res, m.dev, m.customer, opts)
		}
	}

	rank := map[string]int{}
	for i, k := range fleetGapsKinds {
		rank[k] = i
	}
	sort.SliceStable(res.Gaps, func(i, j int) bool {
		a, b := res.Gaps[i], res.Gaps[j]
		if ac, bc := strings.ToLower(a.Customer), strings.ToLower(b.Customer); ac != bc {
			return ac < bc
		}
		if ad, bd := strings.ToLower(a.Device), strings.ToLower(b.Device); ad != bd {
			return ad < bd
		}
		if a.DeviceUUID != b.DeviceUUID {
			return a.DeviceUUID < b.DeviceUUID
		}
		if rank[a.Gap] != rank[b.Gap] {
			return rank[a.Gap] < rank[b.Gap]
		}
		return a.Component < b.Component
	})
	return res
}

func fleetGapsPrintTable(w io.Writer, v fleetGapsView) error {
	unattributed := ""
	switch {
	case v.customer != "":
		unattributed = esetUnattributedLine(v.UnattributedDevices, "synced device(s)", v.UnattributedDeviceUUIDs, v.customer, "They were not checked. Run '"+esetCLIName+" sync --resources device-groups' to attribute them.", v.parentNotVisible)
	case v.Scope == fleetGapsScopeCustomer:
		// Never an all-clear while devices with no known peer group exist.
		unattributed = esetUnattributedLine(v.UnattributedDevices, "synced device(s)", v.UnattributedDeviceUUIDs, "", "They were checked only for unlicensed and unenrolled, and gaps marked uncertain depend on them. Run '"+esetCLIName+" sync --resources device-groups' to attribute them.", v.parentNotVisible)
	}
	if len(v.Gaps) == 0 {
		msg := fmt.Sprintf("No gaps found across %d devices (scope: %s).", v.DevicesChecked, v.Scope)
		if unattributed != "" && v.customer != "" {
			// Never an all-clear while devices that may be the customer's went unchecked.
			msg = fmt.Sprintf("No gaps found across the %d device(s) attributed to %s (scope: %s).", v.DevicesChecked, esetFlatten(v.customer), v.Scope)
		}
		if v.SkippedSmallGroups > 0 {
			msg += fmt.Sprintf(" %d peer groups had fewer than %d devices and were not checked for missing components.", v.SkippedSmallGroups, fleetGapsMinGroupSize)
		}
		lines := []string{msg}
		if unattributed != "" {
			lines = append(lines, unattributed)
		}
		if v.SyncedHint != "" {
			lines = append(lines, v.SyncedHint)
		}
		for _, l := range lines {
			if _, err := esetFprintln(w, l); err != nil {
				return err
			}
		}
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CUSTOMER\tDEVICE\tGAP\tCOMPONENT\tHAVE\tNEWEST\tDETAIL")
	for _, g := range v.Gaps {
		esetFprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", g.Customer, g.Device, g.Gap, g.Component, g.HaveVersion, g.NewestVersion, g.Detail)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	parts := make([]string, 0, len(fleetGapsKinds))
	for _, k := range fleetGapsKinds {
		if n, ok := v.CountsByGap[k]; ok {
			parts = append(parts, fmt.Sprintf("%s=%d", k, n))
		}
	}
	_, err := esetFprintf(w, "\n%d devices checked (scope: %s); %s; %d small peer groups skipped for missing\n",
		v.DevicesChecked, v.Scope, strings.Join(parts, " "), v.SkippedSmallGroups)
	if err == nil && v.UncertainGaps > 0 {
		_, err = esetFprintf(w, "%d gap(s) are uncertain: they hold for some, not every, placement of the devices whose customer is unknown.\n", v.UncertainGaps)
	}
	if err == nil && unattributed != "" {
		_, err = esetFprintln(w, unattributed)
	}
	if err == nil && v.SyncedHint != "" {
		_, err = esetFprintln(w, v.SyncedHint)
	}
	return err
}

func newNovelFleetGapsCmd(flags *rootFlags) *cobra.Command {
	var only, customer, dbPath string
	var threshold float64
	var fleet bool
	var limit int

	cmd := &cobra.Command{
		Use:   "gaps",
		Short: "Devices missing a component or product their customer's other devices have, running an older component version",
		Long: "Peer-relative protection gaps from the local mirror of devices and device groups. Each\n" +
			"device is compared with its peer group: the other devices of the same customer (the\n" +
			"nearest COMPANY ancestor group, as in 'fleet health'), or with --fleet every other synced\n" +
			"device. --customer limits which devices are reported; it does not shrink the peer group.\n" +
			"A device whose group (or an ancestor of it, other than the hidden root group of a scoped API\n" +
			"user; see 'fleet health') is not in the synced tree has no known customer:\n" +
			"it is counted in unattributed_devices (uuids in unattributed_device_uuids), never pooled\n" +
			"into a peer group, checked only for unlicensed and unenrolled (\"(group not in synced\n" +
			"tree)\" or \"(parent group not visible)\"), and not checked under --customer. Since it may be\n" +
			"any customer's peer, a missing\n" +
			"or outdated gap that holds for some but not every placement of such devices is listed with\n" +
			"uncertain true (counted in uncertain_gaps); one that holds for no placement is not listed.\n\n" +
			"Gap kinds (--only, comma-separated; default all):\n" +
			"  missing     a component (deployedComponents[].name) is on at least --peer-threshold of\n" +
			"              the device's peers but not on the device. peer_share is the fraction of\n" +
			"              peers (the group minus the device itself) that have it. Peer groups with\n" +
			"              fewer than 3 devices are not checked for this and are counted in\n" +
			"              skipped_small_groups.\n" +
			"  outdated    the device runs an older version of a component than the newest version\n" +
			"              seen in its peer group.\n" +
			"  unlicensed  the device has no active ESET product (empty activeProducts).\n" +
			"  unenrolled  enrollment status is PENDING or FAILED.\n\n" +
			"counts_by_gap counts every gap found before --limit is applied.\n\n" +
			"Use this command to list devices missing a component, product or enrollment that their customer's other devices have, or running an older component version. Do NOT use it for devices that stopped checking in; use 'fleet stale' instead. Do NOT use it for per-customer totals; use 'fleet health' instead.",
		Example: strings.Trim(`
  eset-protect-cli fleet gaps --only missing,outdated
  eset-protect-cli fleet gaps --customer "Acme Dental" --peer-threshold 0.75
  eset-protect-cli fleet gaps --fleet --only outdated --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet gaps")
			}
			kinds, err := fleetGapsParseOnly(only)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			if threshold <= 0 || threshold > 1 || math.IsNaN(threshold) {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--peer-threshold must be greater than 0 and at most 1 (got %v)", threshold))
			}
			if limit < 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be 0 (no limit) or more (got %d)", limit))
			}
			scope := fleetGapsScopeCustomer
			if fleet {
				scope = fleetGapsScopeFleet
			}
			view := fleetGapsView{Gaps: make([]fleetGapsRow, 0), CountsByGap: map[string]int{}, PeerThreshold: threshold, Scope: scope}
			for k := range kinds {
				view.CountsByGap[k] = 0
			}
			emit := func() error {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return fleetGapsPrintTable(cmd.OutOrStdout(), view)
			}

			resources := []string{"devices", "device-groups"}
			db, err := openEsetStore(cmd, dbPath, resources...)
			if err != nil {
				return err
			}
			view.SyncedHint = esetMirrorHint(db, resources...)
			if db == nil {
				return emit()
			}
			defer db.Close()
			esetSyncHints(cmd, db, flags.maxAge, resources...)

			groups, err := loadEsetGroups(db)
			if err != nil {
				return fmt.Errorf("reading synced device groups: %w", err)
			}
			devices, err := loadEsetDevices(db)
			if err != nil {
				return fmt.Errorf("reading synced devices: %w", err)
			}
			tree := newEsetTree(groups)
			view.parentNotVisible = tree.parentNotVisible()
			view.ParentNotVisible = tree.parentNotVisibleGroups()
			opts := fleetGapsOptions{Kinds: kinds, Threshold: threshold, Fleet: fleet}
			if customer != "" {
				g, rerr := tree.resolveCustomer(customer)
				if rerr != nil {
					return usageErr(rerr)
				}
				opts.Customer, opts.HasCustomer = g, true
				view.customer = g.Name
			}
			res := fleetGapsFind(devices, tree, opts)
			view.UnattributedDevices, view.UnattributedDeviceUUIDs = res.Unattributed.N, res.Unattributed.UUIDs()
			view.CountsByGap = res.Counts
			view.DevicesChecked = res.DevicesChecked
			view.SkippedSmallGroups = res.SkippedSmallGroups
			view.UncertainGaps = res.Uncertain
			view.Gaps = res.Gaps
			if limit > 0 && len(view.Gaps) > limit {
				view.Gaps = view.Gaps[:limit]
			}
			return emit()
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "Comma-separated gap kinds to report: missing, outdated, unlicensed, unenrolled (default all)")
	cmd.Flags().StringVar(&customer, "customer", "", "Only report devices of this customer: COMPANY group name, unique name fragment, or UUID")
	cmd.Flags().Float64Var(&threshold, "peer-threshold", fleetGapsDefaultThreshold, "Flag a missing component when at least this fraction (0-1] of the device's peers have it")
	cmd.Flags().BoolVar(&fleet, "fleet", false, "Compare each device against the whole fleet instead of its own customer")
	cmd.Flags().IntVar(&limit, "limit", fleetGapsDefaultLimit, "Maximum gap rows to show (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", esetDBFlagUsage)
	return cmd
}
