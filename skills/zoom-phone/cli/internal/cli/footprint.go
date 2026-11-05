// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: one phone user's full footprint.
// pp:data-source auto
// Base entities (users, devices, numbers, emergency addresses, group lists)
// come from the local store; group memberships are read live because sync
// does not store any member list.

package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newNovelFootprintCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var noLive bool

	cmd := &cobra.Command{
		Use:   "footprint [user]",
		Short: "Show everything one phone user holds: extension, numbers, calling plans, queues, shared lines, pickup and monitoring groups, desk phones",
		Long: strings.TrimSpace(`
Use this command to see everything one phone user holds: extension, numbers, calling plans, call queues, shared line groups, pickup and monitoring groups, desk phones and effective emergency address. Do NOT use this command to remove those assignments; use 'offboard' instead. Do NOT use it to find where an inbound phone number rings; use 'routing trace' instead. Do NOT use it for account-wide emergency address gaps; use 'e911 audit' instead.

The user is matched in the local store by user id, email, extension number,
phone number, or a case-insensitive name substring. The user row, numbers,
calling plans, desk phones and emergency address come from the local store.
Call queue, shared line group, group call pickup and monitoring group
memberships are not stored by sync, so they are read live from each group's
member endpoint (4 at a time). --no-live skips that fetch and lists the
skipped families under skipped_live.

The effective emergency address is the user's personal (level 1) address,
else their site's default company address, else an account-wide default.

Phone numbers match in any common format ("4155550123", "(415) 555-0123",
"+1 415 555 0123"). "assessed" is true only when every resource footprint
reads has a completed, full, unfiltered sync (zoom-phone-cli sync --full;
a plain sync does not delete rows the API no longer returns), i.e.
missing_local is empty, with missing_reasons per resource; a --db path that
does not exist is a usage error (exit 2). meta.source is "live" only when the live
membership fetch ran.`),
		Example: strings.Trim(`
  zoom-phone-cli footprint jane@example.com --agent
  zoom-phone-cli footprint 1001 --json
  zoom-phone-cli footprint "Jane Doe" --no-live`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
			"pp:happy-args":  "user=jane@example.com",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "footprint")
			}
			query := strings.TrimSpace(strings.Join(args, " "))
			if query == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a user is required: pass an email, extension number or name"))
			}
			skipLive := noLive || flags.dataSource == "local"

			resolved, err := localResolveFootprintUser(cmd, dbPath, query, false)
			if err != nil {
				return err
			}
			var fp *localFootprint
			switch {
			case resolved.data == nil:
				fp = localUnassessedFootprint(query)
			case resolved.user == nil:
				fp = localEmptyFootprint(query)
				fp.MissingLocal = append(fp.MissingLocal, resolved.data.missing...)
				fp.MissingReasons = resolved.data.reasons
				fp.Assessed = len(fp.MissingLocal) == 0
				fp.Note = "no phone users in the local store; run: " + localSyncCommand(localFootprintSyncResources)
				if resolved.note != "" {
					fp.Note = resolved.note
				}
			default:
				// The generated client applies --timeout per request; the
				// fan-out runs on the command context.
				fp = localBuildFootprint(cmd.Context(), cmd, flags, resolved.data, *resolved.user, query, skipLive, false)
				fp.MatchedBy = resolved.matchedBy
			}
			// An unassessed result from a store says so first in its note
			// (localBuildFootprint already does for a matched user).
			if resolved.data != nil && !fp.Assessed && !strings.HasPrefix(fp.Note, "not assessed: ") {
				fp.Note = localNotAssessedNote(fp.MissingLocal, fp.Note)
			}
			// meta.source is "live" only when the live membership scan ran.
			flags.agentSource = "local"
			if fp.liveRan {
				flags.agentSource = "live"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), fp, flags)
			}
			return localPrintFootprint(cmd, fp)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: the synced store for the active account)")
	cmd.Flags().BoolVar(&noLive, "no-live", false, "Skip the live membership fetch (queues, shared line groups, pickup and monitoring groups)")
	return cmd
}

func localPrintFootprint(cmd *cobra.Command, fp *localFootprint) error {
	localWriteNotAssessed(cmd.OutOrStdout(), fp.Assessed, fp.Note)
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
	if fp.User == nil {
		fmt.Fprintf(w, "No phone user found for %q.\n", fp.Query)
		if fp.Note != "" && fp.Assessed {
			fmt.Fprintln(w, fp.Note)
		}
		return w.Flush()
	}
	u := fp.User
	fmt.Fprintf(w, "User\t%s <%s>\n", u.Name, u.Email)
	fmt.Fprintf(w, "Extension\t%s (id %s)\n", u.ExtensionNumber, u.ExtensionID)
	fmt.Fprintf(w, "Status\t%s / %s\n", u.Status, u.ActivationStatus)
	if u.Site != nil {
		fmt.Fprintf(w, "Site\t%s\n", u.Site.Name)
	}
	if a := fp.EmergencyAddress; a != nil {
		fmt.Fprintf(w, "Emergency address\t%s %s [%s]\n", a.Source, a.Address, a.Status)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "KIND\tNAME\tDETAIL")
	for _, n := range fp.PhoneNumbers {
		fmt.Fprintf(w, "number\t%s\t%s %s\n", n.Number, n.Source, n.Status)
	}
	for _, p := range fp.CallingPlans {
		fmt.Fprintf(w, "calling plan\t%s\ttype %s\n", p.Name, p.Type)
	}
	for _, list := range [][]localMembership{fp.CallQueues, fp.SharedLineGroups, fp.GroupCallPickup, fp.MonitoringGroups} {
		for _, m := range list {
			detail := "ext " + m.GroupExtensionNumber
			if m.MemberType != "" {
				detail += " as " + m.MemberType
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", m.Family, m.GroupName, detail)
		}
	}
	for _, d := range fp.DeskPhones {
		fmt.Fprintf(w, "desk phone\t%s\t%s %s %s\n", d.DisplayName, d.DeviceType, d.MacAddress, d.Status)
	}
	if len(fp.SkippedLive) > 0 {
		fmt.Fprintf(w, "\nskipped (live): %s\n", strings.Join(fp.SkippedLive, ", "))
	}
	if fp.Note != "" && fp.Assessed {
		fmt.Fprintf(w, "\nnote: %s\n", fp.Note)
	}
	return w.Flush()
}
