// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type driftOrgRow struct {
	OrgID         string `json:"org_id"`
	OrgName       string `json:"org_name"`
	AutoUpdate    *bool  `json:"auto_update"`
	Agents        int    `json:"agents"`
	BehindLatest  int    `json:"behind_latest"`
	OnBadVersion  int    `json:"on_bad_version"`
	UnknownLatest int    `json:"unknown_latest"`
	// VersionUnknown counts clients that report no agent_version, or one
	// compareSemver cannot order; they are never compared against a release.
	VersionUnknown int `json:"version_unknown"`
	// MembershipUnknown counts clients --org could neither match nor rule
	// out (site or org missing from the mirror); they are grouped under
	// "(org membership unknown)".
	MembershipUnknown int `json:"membership_unknown"`
	// TypeUnknown counts clients --agent-type could neither match nor rule
	// out (agent_type absent, null or not a string); they are not compared.
	TypeUnknown    int      `json:"type_unknown"`
	BehindVersions []string `json:"behind_versions"`
	SampleHosts    []string `json:"sample_hosts"`
}

type driftView struct {
	Channel       string            `json:"channel"`
	ChannelSource string            `json:"channel_source"`
	Latest        map[string]string `json:"latest"`
	// ReleasesSource is "live" (the vendor's current release feed) or
	// "unavailable" when the feed could not be read.
	ReleasesSource string        `json:"releases_source"`
	BadVersion     string        `json:"bad_version,omitempty"`
	AgentsScanned  int           `json:"agents_scanned"`
	Orgs           []driftOrgRow `json:"orgs"`
	Note           string        `json:"note,omitempty"`
	// IncompleteCollections names joined collections the mirror has no
	// complete sync of; set, the result is not an all-clear.
	IncompleteCollections []string `json:"incomplete_collections,omitempty"`
}

// driftCollections is every collection the drift join reads from the mirror.
var driftCollections = []string{"organizations", "networks", "user-agents"}

func newNovelAgentsDriftCmd(flags *rootFlags) *cobra.Command {
	var badVersion, agentType, channel, orgFilter, dbPath string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "See which client orgs have roaming clients behind the latest agent release or on a bad build",
		Long: `Use this command for roaming-client agent version questions (who is behind the latest release, who is on a specific bad version) across every client org.
Do NOT use this command for roaming clients that stopped checking in or lost protection; use 'agents stale' instead.

The roaming-client fleet comes from the local mirror (run 'dnsfilter-cli sync --full --resources organizations,networks,user-agents' first); the release feed is read live on every run, because the vendor publishes it as a current "latest per agent type, architecture, channel and white label" snapshot and a mirrored copy would keep withdrawn releases after a rollback. Latest is the highest non-white-label release in the chosen channel (default stable) per agent type, across architectures; an in-channel release whose version cannot be compared, or a release whose release_channels cannot be read and that could be newer, makes that type's latest unknown, and a client version that cannot be compared (e.g. "999unknown", or a number instead of a string) counts as version_unknown, never current. With --agent-type, a client whose agent_type cannot be read is counted as type_unknown, never dropped. Roaming clients map to an org through their site (network); with --org, a client whose site or org is missing from the mirror is counted under (org membership unknown) as membership_unknown instead of being dropped. --data-source local and live are both rejected: the fleet is always the local mirror and the release feed is always live.`,
		Example: strings.Trim(`
  dnsfilter-cli agents drift --agent
  dnsfilter-cli agents drift --bad-version 3.1.0 --agent-type windows`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "agents drift")
			}
			if err := rejectExtraArgs(args, 0); err != nil {
				return err
			}
			if flags.dataSource == "local" || flags.dataSource == "live" {
				return usageErr(fmt.Errorf("agents drift joins the local roaming-client mirror with the live release feed, so neither --data-source local nor live applies; omit --data-source"))
			}
			if badVersion != "" && !isComparableVersion(badVersion) {
				return usageErr(fmt.Errorf("--bad-version %q is not a comparable version (dotted numbers, optional -prerelease)", badVersion))
			}
			opts := driftOptions{BadVersion: badVersion, AgentType: agentType, Channel: channel, Org: orgFilter}
			empty := computeDrift(nil, nil, nil, nil, opts)
			empty.ReleasesSource = "unavailable"
			empty.Note = "no local mirror; run: dnsfilter-cli sync --full --resources organizations,networks,user-agents"
			empty.IncompleteCollections = driftCollections
			db, ok, err := openLocalMirror(cmd, flags, dbPath, "organizations,networks,user-agents", empty)
			if !ok {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "user-agents") {
				hintIfStale(cmd, db, "user-agents", flags.maxAge)
			}
			ctx := cmd.Context()
			// Read the live feed first so the snapshot is not held across it.
			releases, feedErr := fetchReleaseFeed(cmd, flags)
			// Rows and their completeness come from one snapshot.
			snap, err := mirrorSnapshot(ctx, db)
			if err != nil {
				return err
			}
			defer snap.Rollback()
			agents, err := loadLocal(ctx, snap, "user-agents")
			if err != nil {
				return err
			}
			networks, err := loadLocal(ctx, snap, "networks")
			if err != nil {
				return err
			}
			orgs, err := loadLocal(ctx, snap, "organizations")
			if err != nil {
				return err
			}
			afterMirrorRowsRead()
			incomplete, err := incompleteCollections(ctx, snap, driftCollections...)
			if err != nil {
				return err
			}
			view := computeDrift(agents, releases, networks, orgs, opts)
			view.ReleasesSource = "live"
			if feedErr != nil {
				view.ReleasesSource = "unavailable"
				// computeDrift's empty-feed note describes a feed that was read and
				// listed nothing; it does not apply when the feed was never read.
				view.Note = fmt.Sprintf("Release feed unavailable (%v); versions cannot be compared.", feedErr)
			}
			if n := driftMembershipUnknown(view.Orgs); n > 0 {
				view.Note = joinNotes(view.Note, fmt.Sprintf("%d roaming client(s) could not be placed in or out of --org %q (their site or organization is missing from the local mirror); they are counted under %s.", n, orgFilter, membershipUnknownName))
			}
			if len(incomplete) > 0 {
				view.IncompleteCollections = incomplete
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if len(incomplete) > 0 {
					note := incompleteSyncNote(incomplete)
					fmt.Fprintln(cmd.ErrOrStderr(), "hint: "+note)
					if view.Note != "" {
						note = strings.TrimSuffix(view.Note, ".") + ". " + note
					}
					view.Note = note
				}
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "channel %s (%s), latest %s, %d roaming clients scanned\n", humanText(view.Channel), humanText(view.ChannelSource), humanText(formatDriftLatest(view.Latest)), view.AgentsScanned)
			switch {
			case len(incomplete) > 0 && view.AgentsScanned == 0:
				// An unwalked site or org list empties --org; do not blame the filter.
				fmt.Fprintln(w, humanText("No synced roaming clients matched; nothing was compared. "+incompleteSyncNote(incomplete)))
			case len(incomplete) > 0:
				fmt.Fprintln(w, humanText(incompleteSyncNote(incomplete)))
			case view.AgentsScanned == 0:
				// Zero scanned is not an all-clear: the mirror may be unsynced
				// or --org / --agent-type matched nothing.
				fmt.Fprintln(w, "No synced roaming clients matched; nothing was compared. Run: dnsfilter-cli sync --full --resources organizations,networks,user-agents (or widen --org / --agent-type).")
			case len(view.Orgs) == 0 && view.ReleasesSource == "live" && len(view.Latest) > 0:
				fmt.Fprintln(w, "No client org has roaming clients behind the latest release.")
			}
			for _, r := range view.Orgs {
				fmt.Fprintf(w, "  %-30s agents=%-4d behind=%-4d bad=%-4d no-known-release=%-4d version-unknown=%-4d versions=%s", humanText(r.OrgName), r.Agents, r.BehindLatest, r.OnBadVersion, r.UnknownLatest, r.VersionUnknown, humanText(strings.Join(r.BehindVersions, ",")))
				if r.MembershipUnknown > 0 {
					fmt.Fprintf(w, " membership-unknown=%d", r.MembershipUnknown)
				}
				if r.TypeUnknown > 0 {
					fmt.Fprintf(w, " type-unknown=%d", r.TypeUnknown)
				}
				fmt.Fprintln(w)
			}
			if view.Note != "" {
				fmt.Fprintln(w, "note:", humanText(view.Note))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&badVersion, "bad-version", "", "Agent version to flag (e.g. a release that was pulled), counted per org")
	cmd.Flags().StringVar(&agentType, "agent-type", "", "Only count this agent type (windows, macos)")
	cmd.Flags().StringVar(&channel, "channel", "", "Release channel to compare against: stable (default), beta, preview")
	cmd.Flags().StringVar(&orgFilter, "org", "", "Only this client org (name substring or id)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI's synced database)")
	return cmd
}

type driftOptions struct{ BadVersion, AgentType, Channel, Org string }

// formatDriftLatest renders the per-agent-type latest release as sorted
// "type version" pairs for the human table, or "unknown" when none is known.
func formatDriftLatest(latest map[string]string) string {
	if len(latest) == 0 {
		return "unknown"
	}
	types := make([]string, 0, len(latest))
	for t := range latest {
		types = append(types, t)
	}
	sort.Strings(types)
	parts := make([]string, 0, len(types))
	for _, t := range types {
		parts = append(parts, t+" "+latest[t])
	}
	return strings.Join(parts, ", ")
}

// fetchReleaseFeed reads the vendor's current release snapshot
// ({"user_agent_releases":[...]}); it is deliberately never cached or mirrored.
func fetchReleaseFeed(cmd *cobra.Command, flags *rootFlags) ([]jaRes, error) {
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	raw, err := c.GetNoCache(ctx, "/v1/user_agent_releases", nil)
	if err != nil {
		return nil, err
	}
	return parseReleaseFeed(raw)
}

func parseReleaseFeed(raw []byte) ([]jaRes, error) {
	var feed struct {
		Releases []map[string]any `json:"user_agent_releases"`
	}
	if err := json.Unmarshal(raw, &feed); err != nil {
		return nil, fmt.Errorf("release feed: %w", err)
	}
	if feed.Releases == nil {
		return nil, fmt.Errorf("release feed: response has no user_agent_releases list")
	}
	out := make([]jaRes, 0, len(feed.Releases))
	for i, r := range feed.Releases {
		rel := toJARes(r)
		_, hasChannels := rel.Attrs["release_channels"].([]any)
		if rel.str("agent_type") == "" || rel.str("agent_version") == "" || !hasChannels {
			return nil, fmt.Errorf("release feed: entry %d lacks agent_type, agent_version or release_channels", i)
		}
		out = append(out, rel)
	}
	return out, nil
}

// computeDrift is the pure join behind `agents drift`.
func computeDrift(agents, releases, networks, orgs []jaRes, o driftOptions) driftView {
	view := driftView{Channel: "stable", ChannelSource: "default", Latest: map[string]string{}, BadVersion: o.BadVersion, Orgs: make([]driftOrgRow, 0)}
	if o.Channel != "" {
		view.Channel, view.ChannelSource = strings.ToLower(o.Channel), "flag"
	}
	// HAND-FIX(agents-drift-incomparable-version-unknown): an in-channel
	// release whose version cannot be ordered could be the latest one, so its
	// agent type has no known latest release.
	incomparable := map[string]bool{}
	// HAND-FIX(agents-drift-unknown-release-channel): a release whose
	// release_channels holds an unreadable entry was read as outside the
	// channel. Its versions are kept per agent type, and once latest is known
	// one that could be newer makes that type's latest unknown.
	channelUnknown := map[string][]string{}
	for _, r := range releases {
		inChannel := false
		channels, channelsKnown := releaseChannels(r)
		for _, ch := range channels {
			if ch == view.Channel {
				inChannel = true
			}
		}
		t := strings.ToLower(r.str("agent_type"))
		// ponytail: white-label builds ship to white-label tenants only; the
		// client record carries no architecture or white-label marker to match on.
		if r.boolean("white_label") {
			continue
		}
		if !inChannel && !channelsKnown && t != "" {
			channelUnknown[t] = append(channelUnknown[t], r.str("agent_version"))
			continue
		}
		if inChannel && t != "" && !isComparableVersion(r.str("agent_version")) {
			incomparable[t] = true
			continue
		}
		if inChannel && t != "" && compareSemver(r.str("agent_version"), view.Latest[t]) > 0 {
			view.Latest[t] = r.str("agent_version")
		}
	}
	unknownChannelTypes := map[string]bool{}
	for t, versions := range channelUnknown {
		for _, v := range versions {
			if !isComparableVersion(v) || view.Latest[t] == "" || compareSemver(v, view.Latest[t]) > 0 {
				unknownChannelTypes[t] = true
			}
		}
	}
	for t := range unknownChannelTypes {
		delete(view.Latest, t)
	}
	for t := range incomparable {
		delete(view.Latest, t)
	}
	netOrg := siteOrgs(networks)
	orgByID := byID(orgs)
	rows := map[string]*driftOrgRow{}
	versionsSeen := map[string]map[string]bool{}
	for _, a := range agents {
		if isUninstalled(a) {
			continue
		}
		// HAND-FIX(agents-drift-unknown-agent-type): --agent-type dropped a
		// client whose agent_type is unreadable; it is counted as type_unknown.
		t, typeKnown := a.strKnown("agent_type")
		t = strings.ToLower(t)
		if o.AgentType != "" && typeKnown && t != strings.ToLower(o.AgentType) {
			continue
		}
		orgID, match, known := orgMembership(a, netOrg, orgByID, o.Org)
		if known && !match {
			continue
		}
		if !known {
			// HAND-FIX(agents-org-membership-unknown): --org can neither match
			// nor rule out this client; count it apart instead of dropping it.
			orgID = ""
		}
		org := orgByID[orgID]
		view.AgentsScanned++
		row := rows[orgID]
		if row == nil {
			row = &driftOrgRow{OrgID: orgID, OrgName: org.str("name"), BehindVersions: make([]string, 0), SampleHosts: make([]string, 0)}
			if orgID == "" {
				row.OrgName = "(site not synced)"
				if o.Org != "" {
					row.OrgName = membershipUnknownName
				}
			}
			if v, ok := org.Attrs["user_agents_auto_update"].(bool); ok {
				row.AutoUpdate = &v
			}
			rows[orgID] = row
			versionsSeen[orgID] = map[string]bool{}
		}
		row.Agents++
		if !known {
			row.MembershipUnknown++
		}
		if o.AgentType != "" && !typeKnown {
			row.TypeUnknown++
			continue
		}
		// agent_version must be a JSON string: str() never turns 999 into "999".
		ver := a.str("agent_version")
		if ver == "" || !isComparableVersion(ver) {
			row.VersionUnknown++
			continue
		}
		if o.BadVersion != "" && compareSemver(ver, o.BadVersion) == 0 {
			row.OnBadVersion++
		}
		latest := view.Latest[t]
		switch {
		case latest == "":
			row.UnknownLatest++
		case compareSemver(ver, latest) < 0:
			row.BehindLatest++
			if !versionsSeen[orgID][ver] {
				versionsSeen[orgID][ver] = true
				row.BehindVersions = append(row.BehindVersions, ver)
			}
			if len(row.SampleHosts) < 5 {
				row.SampleHosts = append(row.SampleHosts, a.str("hostname"))
			}
		}
	}
	for _, r := range rows {
		// Orgs whose clients have no known latest release (an agent type the
		// feed does not list) are reported too, so coverage gaps stay visible.
		if r.BehindLatest > 0 || r.OnBadVersion > 0 || r.UnknownLatest > 0 || r.VersionUnknown > 0 || r.MembershipUnknown > 0 || r.TypeUnknown > 0 {
			sort.Slice(r.BehindVersions, func(i, j int) bool { return compareSemver(r.BehindVersions[i], r.BehindVersions[j]) < 0 })
			view.Orgs = append(view.Orgs, *r)
		}
	}
	sort.Slice(view.Orgs, func(i, j int) bool {
		a, b := view.Orgs[i], view.Orgs[j]
		if a.OnBadVersion != b.OnBadVersion {
			return a.OnBadVersion > b.OnBadVersion
		}
		if a.BehindLatest != b.BehindLatest {
			return a.BehindLatest > b.BehindLatest
		}
		return a.OrgName < b.OrgName
	})
	switch {
	case len(incomparable) > 0:
		view.Note = fmt.Sprintf("the release feed lists an agent_version that cannot be compared for agent type(s) %s in channel %s, so their latest release is unknown", strings.Join(sortedKeys(incomparable), ", "), view.Channel)
	case len(view.Latest) == 0 && len(unknownChannelTypes) == 0:
		view.Note = "the release feed lists no non-white-label release for channel " + view.Channel
	}
	if len(unknownChannelTypes) > 0 {
		view.Note = joinNotes(view.Note, fmt.Sprintf("The release feed lists a release for agent type(s) %s whose release_channels cannot be read and that could be newer, so their latest release in channel %s is unknown.", strings.Join(sortedKeys(unknownChannelTypes), ", "), view.Channel))
	}
	if n := driftTypeUnknown(view.Orgs); n > 0 {
		view.Note = joinNotes(view.Note, fmt.Sprintf("%d roaming client(s) report no readable agent_type, so --agent-type %q can neither match nor rule them out; they are counted as type_unknown and not compared.", n, o.AgentType))
	}
	return view
}

// driftTypeUnknown totals the clients --agent-type could not place.
func driftTypeUnknown(rows []driftOrgRow) int {
	n := 0
	for _, r := range rows {
		n += r.TypeUnknown
	}
	return n
}

// driftMembershipUnknown totals the clients --org could not place.
func driftMembershipUnknown(rows []driftOrgRow) int {
	n := 0
	for _, r := range rows {
		n += r.MembershipUnknown
	}
	return n
}

// releaseChannels returns the channel values a release is published to;
// known is false when release_channels is not a list or any entry is not a
// channel object with a non-empty value (or a non-empty string).
func releaseChannels(r jaRes) (channels []string, known bool) {
	arr, known := r.Attrs["release_channels"].([]any)
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		switch t := it.(type) {
		case map[string]any:
			if v, _ := t["value"].(string); v != "" {
				out = append(out, strings.ToLower(v))
			} else {
				known = false
			}
		case string:
			if t != "" {
				out = append(out, strings.ToLower(t))
			} else {
				known = false
			}
		default:
			known = false
		}
	}
	return out, known
}
