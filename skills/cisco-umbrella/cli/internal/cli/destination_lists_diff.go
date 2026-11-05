// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// The add and remove endpoints accept at most 500 items per call.
const destinationBatch = 500

type diffRemoval struct {
	ID          string `json:"id"`
	Destination string `json:"destination"`
}

type destinationDiff struct {
	ListID     int64         `json:"list_id"`
	File       string        `json:"file"`
	ToAdd      []string      `json:"to_add"`
	ToRemove   []diffRemoval `json:"to_remove"`
	Unchanged  int           `json:"unchanged"`
	Skipped    []string      `json:"skipped_lines"`
	NotAdded   []string      `json:"not_added"`
	Applied    bool          `json:"applied"`
	Added      int           `json:"added"`
	Removed    int           `json:"removed"`
	RemoveMode string        `json:"remove_mode"`
}

func newNovelDestinationListsDiffCmd(flags *rootFlags) *cobra.Command {
	var file, entries, comment string
	var apply, noRemove bool
	cmd := &cobra.Command{
		Use:   "diff [destinationListId]",
		Short: "Reconcile a destination list against a source-of-truth file: see what to add and remove",
		Long: `Normalize a file of domains, URLs, and IPv4 addresses (one per line, # comments
allowed; '-' reads stdin), or the same values inline with --entries (comma or newline
separated), and diff it against the live destination list. Prints the
plan by default. With --apply it adds the missing entries and removes the extra ones
(resolving each to the destination ID the remove call needs), 500 per call, with no
confirmation prompt. It is not all-or-nothing: if a call fails, earlier batches stay
applied and the error reports how many were added or removed. A source with no entries
is refused with --apply: to empty a list on purpose, use the Umbrella dashboard. Needs the
Policies scope.

Use this command to reconcile one destination list against a file. Do NOT use this
command to check whether one domain is on a list; use 'verdict' instead. Over MCP,
--file is refused (it would read a file on the MCP host); pass --entries.`,
		Example: strings.Trim(`
  cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --agent
  cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --apply
  cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --no-remove --apply
  cisco-umbrella-cli destination-lists diff 1234567 --entries 'bad.example.com,198.51.100.7' --no-remove --apply`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only-unless-flags": "apply",
			// --file names a file on the host: an MCP caller passes --entries instead.
			"mcp:read-flags":        "file",
			"pp:data-source":        "live",
			"pp:preview-happy-path": "true",
			"pp:happy-args":         "destinationListId=1234567;--file=-",
			"pp:happy-stdin":        "api.umbrella.com",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "destination-lists diff")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("destinationListId is required (see 'destination-lists list')"))
			}
			listID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || listID <= 0 {
				return usageErr(fmt.Errorf("destinationListId must be a positive integer, got %q", args[0]))
			}
			if (file == "") == (entries == "") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("give exactly one of --file ('-' reads stdin) or --entries"))
			}
			var lines []string
			if entries != "" {
				lines, err = readLines(strings.NewReader(strings.ReplaceAll(entries, ",", "\n")))
				file = "--entries"
			} else if file == "-" {
				lines, err = readLines(cmd.InOrStdin())
			} else {
				var f *os.File
				f, err = os.Open(filepath.Clean(file)) // the operator names the file; MCP callers cannot (mcp:read-flags)
				if err == nil {
					lines, err = readLines(f)
					_ = f.Close()
				}
			}
			if err != nil {
				return fmt.Errorf("reading %s: %w", file, err)
			}

			want, skipped := normalizeAll(lines)
			if apply && len(skipped) > 0 {
				// An unparsed line would read as "absent", and its matching list entry
				// would be removed; refuse to write until every line parses.
				return usageErr(fmt.Errorf("%d line(s) in %s could not be parsed (first: %q); fix them before --apply", len(skipped), file, skipped[0]))
			}
			if apply && len(want) == 0 {
				// With nothing to keep, every entry on the list reads as "extra".
				return usageErr(fmt.Errorf("%s has no entries, so --apply would remove every entry on the list; refused. To empty a list on purpose, use the Umbrella dashboard", file))
			}
			// ponytail: no whole-command --timeout here; this fans out across every list/customer
			// and each request is already bounded by the client's per-request timeout.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if err := requireCredentials(c); err != nil {
				return err
			}
			current, err := fetchDestinations(ctx, c, listID)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			plan := planDestinationDiff(want, current)
			plan.NotAdded = []string{}
			plan.ListID, plan.File, plan.Skipped = listID, file, skipped
			plan.RemoveMode = "remove-extra"
			if noRemove {
				plan.RemoveMode = "add-only"
				plan.ToRemove = []diffRemoval{}
			}

			removeIDs := make([]int64, 0, len(plan.ToRemove))
			for _, r := range plan.ToRemove {
				// The remove call takes an integer array; check every ID before any write.
				n, err := strconv.ParseInt(strings.TrimSpace(r.ID), 10, 64)
				if err != nil || n <= 0 {
					return apiErr(fmt.Errorf("destination %q has id %q, not a positive integer; refusing to plan a removal", r.Destination, r.ID))
				}
				removeIDs = append(removeIDs, n)
			}
			if apply {
				path := "/policies/v2/destinationlists/" + strconv.FormatInt(listID, 10) + "/destinations"
				for start := 0; start < len(plan.ToAdd); start += destinationBatch {
					end := min(start+destinationBatch, len(plan.ToAdd))
					body := make([]map[string]string, 0, end-start)
					for _, d := range plan.ToAdd[start:end] {
						body = append(body, map[string]string{"destination": d, "comment": comment})
					}
					if _, _, err := c.Post(ctx, path, body); err != nil {
						return fmt.Errorf("adding destinations (%d of %d sent before the failure): %w", start, len(plan.ToAdd), classifyAPIErrorOnly(err))
					}
				}
				// Cisco documents that an add can return 200 yet not add (a URL on a
				// high-volume domain), so read the list back before trusting it and
				// remove nothing unless every addition landed.
				if len(plan.ToAdd) > 0 {
					after, err := fetchDestinations(ctx, c, listID)
					if err != nil {
						return fmt.Errorf("verifying additions: %w", classifyAPIErrorOnly(err))
					}
					landed := map[string]bool{}
					for _, d := range after {
						if v, _, ok := normalizeDestination(d.Destination); ok {
							landed[v] = true
						}
					}
					for _, d := range plan.ToAdd {
						if landed[d] {
							plan.Added++
						} else {
							plan.NotAdded = append(plan.NotAdded, d)
						}
					}
					if len(plan.NotAdded) > 0 {
						_ = printJSONFiltered(cmd.OutOrStdout(), plan, flags)
						return apiErr(fmt.Errorf("%d of %d additions did not land (Umbrella rejects URLs on high-volume domains; add the domain instead); no entries were removed", len(plan.NotAdded), len(plan.ToAdd)))
					}
				}
				for start := 0; start < len(removeIDs); start += destinationBatch {
					end := min(start+destinationBatch, len(removeIDs))
					if _, _, err := c.DeleteWithBody(ctx, path+"/remove", removeIDs[start:end]); err != nil {
						return fmt.Errorf("removing destinations (%d of %d removed before the failure): %w", plan.Removed, len(plan.ToRemove), classifyAPIErrorOnly(err))
					}
					plan.Removed += end - start
				}
				plan.Applied = true
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), plan, flags)
			}
			w := cmd.OutOrStdout()
			for _, d := range plan.ToAdd {
				fmt.Fprintf(w, "+ %s\n", d)
			}
			for _, r := range plan.ToRemove {
				fmt.Fprintf(w, "- %s (id %s)\n", r.Destination, r.ID)
			}
			verb := "plan"
			if plan.Applied {
				verb = "applied"
			}
			fmt.Fprintf(w, "%s: %d to add, %d to remove, %d unchanged", verb, len(plan.ToAdd), len(plan.ToRemove), plan.Unchanged)
			if !plan.Applied && (len(plan.ToAdd) > 0 || len(plan.ToRemove) > 0) {
				fmt.Fprint(w, " (rerun with --apply to make these changes)")
			}
			fmt.Fprintln(w)
			for _, s := range plan.Skipped {
				fmt.Fprintf(cmd.ErrOrStderr(), "skipped unparseable line: %q\n", s)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Source-of-truth file: one domain, URL, or IPv4 per line ('-' for stdin)")
	cmd.Flags().StringVar(&entries, "entries", "", "The source of truth inline instead of --file: domains, URLs, and IPv4 addresses separated by commas or newlines")
	cmd.Flags().BoolVar(&apply, "apply", false, "Make the changes (default prints the plan only)")
	cmd.Flags().BoolVar(&noRemove, "no-remove", false, "Only add missing entries; never remove entries absent from the file")
	cmd.Flags().StringVar(&comment, "comment", "added by cisco-umbrella-cli diff", "Comment stored on each added destination")
	return cmd
}

// normalizeAll canonicalizes and dedupes file lines, returning lines it could not parse.
func normalizeAll(lines []string) (map[string]bool, []string) {
	want := map[string]bool{}
	skipped := make([]string, 0)
	for _, line := range lines {
		v, _, ok := normalizeDestination(line)
		if !ok {
			skipped = append(skipped, line)
			continue
		}
		want[v] = true
	}
	return want, skipped
}

// planDestinationDiff compares the wanted set against the list's current entries.
// Every duplicate copy of an unwanted entry is scheduled for removal.
func planDestinationDiff(want map[string]bool, current []umbrellaDestination) destinationDiff {
	have := map[string]bool{}
	plan := destinationDiff{ToAdd: []string{}, ToRemove: []diffRemoval{}}
	for _, d := range current {
		v, _, ok := normalizeDestination(d.Destination)
		if !ok {
			v = d.Destination
		}
		if want[v] {
			if !have[v] {
				plan.Unchanged++
			}
			have[v] = true
			continue
		}
		plan.ToRemove = append(plan.ToRemove, diffRemoval{ID: d.ID, Destination: d.Destination})
	}
	for v := range want {
		if !have[v] {
			plan.ToAdd = append(plan.ToAdd, v)
		}
	}
	sort.Strings(plan.ToAdd)
	sort.Slice(plan.ToRemove, func(i, j int) bool { return plan.ToRemove[i].Destination < plan.ToRemove[j].Destination })
	return plan
}
