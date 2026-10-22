// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// fleet changes: structured diff between two saved `fleet backup` files.
// pp:data-source local

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/cliutil"
	"fortinet-pp-cli/internal/fortios"
)

type deviceChanges struct {
	Device string `json:"device"`
	// DeviceID is the short API-URL hash fleet backup puts in the directory
	// name; backups are only ever diffed against the same firewall's.
	DeviceID string           `json:"device_id,omitempty"`
	From     string           `json:"from,omitempty"`
	To       string           `json:"to,omitempty"`
	Changes  []fortios.Change `json:"changes"`
	Note     string           `json:"note,omitempty"`
}

type backupFile struct {
	path string
	at   time.Time
}

// backupTime reads the UTC stamp fleet backup puts in the file name,
// falling back to the file's modification time.
func backupTime(path string) time.Time {
	base := strings.TrimSuffix(filepath.Base(path), ".conf")
	if i := strings.LastIndex(base, "-"); i >= 0 {
		for _, layout := range []string{backupStamp, "20060102T150405Z"} {
			if t, err := time.Parse(layout, base[i+1:]); err == nil {
				return t
			}
		}
	}
	if info, err := os.Stat(path); err == nil {
		return info.ModTime().UTC()
	}
	return time.Time{}
}

// pickPair returns the newest backup and the latest one at least `since`
// older than it; with no such backup, the oldest one (noted).
func pickPair(files []backupFile, since time.Duration) (from, to backupFile, note string, ok bool) {
	if len(files) < 2 {
		return from, to, "need at least two backups; run 'fleet backup' again later", false
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.Before(files[j].at) })
	to = files[len(files)-1]
	from = files[0]
	found := false
	for i := len(files) - 2; i >= 0; i-- {
		if to.at.Sub(files[i].at) >= since {
			from, found = files[i], true
			break
		}
	}
	if !found {
		note = fmt.Sprintf("no backup is %s older than the newest; compared with the oldest backup instead", since)
	}
	return from, to, note, true
}

// diffBackups parses and diffs two backup files; a read or parse failure is
// reported in the note instead of a change list.
func diffBackups(fromPath, toPath, note string) (string, string, []fortios.Change, string) {
	from, to := filepath.Base(fromPath), filepath.Base(toPath)
	a, errA := os.ReadFile(fromPath) // #nosec G304 -- backup file enumerated from the operator's backup dir
	b, errB := os.ReadFile(toPath)   // #nosec G304 -- backup file enumerated from the operator's backup dir
	if errA != nil || errB != nil {
		return "", "", []fortios.Change{}, fmt.Sprintf("could not read backups: %v %v", errA, errB)
	}
	oldCfg, errA := fortios.ParseConfig(string(a))
	newCfg, errB := fortios.ParseConfig(string(b))
	if errA != nil || errB != nil {
		return "", "", []fortios.Change{}, fmt.Sprintf("could not parse backups (%s, %s): %v %v", from, to, errA, errB)
	}
	return from, to, fortios.Diff(oldCfg, newCfg), note
}

func newNovelFleetChangesCmd(flags *rootFlags) *cobra.Command {
	var since, dir, only string
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Show which FortiOS objects were added, removed or modified between two saved backups",
		Long: strings.Trim(`
Use this command for what changed in device configuration between backups. Do
NOT use this command to grade current risk; use 'fleet audit' or 'policies
hygiene' instead.

Reads the files 'fleet backup' wrote (<dir>/<device>@<id>/*.conf, one directory
per firewall, so two that share a name are never diffed against each other),
parses the FortiOS
config / edit / set / next / end text into objects keyed by VDOM, table and
name, and diffs the newest backup against the latest one at least --since
older. Security-relevant tables (system admin, api-user, firewall policy,
vpn ssl settings, local-in-policy, interfaces, local users, VIPs) sort first.
Passwords, ENC values and keys are redacted before diffing, so a change that
only rotates a secret is not reported. Needs at least two backups per device
(run 'fleet backup' on a schedule); with none the result is an empty list and a
hint on stderr, which does not mean "nothing changed". No API call is made.`, "\n"),
		Example:     "  fortinet-cli fleet changes --since 7d --agent\n  fortinet-cli fleet changes --device acme-hq --since 30d",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "mcp:write-flags": "dir"}, // --dir is a directory on the MCP host; operators only
		Args:        rejectExtraArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet changes")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil {
				return usageErr(fmt.Errorf("--since: %w", err))
			}
			root, err := backupDir(dir)
			if err != nil {
				return err
			}
			want := map[string]bool{}
			for _, n := range splitCSV(only) {
				want[deviceDirName(n)] = true
			}
			entries, err := os.ReadDir(root)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			results := make([]deviceChanges, 0)
			for _, e := range entries {
				name, id, _ := strings.Cut(e.Name(), "@")
				if !e.IsDir() || (len(want) > 0 && !want[name]) {
					continue
				}
				matches, _ := filepath.Glob(filepath.Join(root, e.Name(), "*.conf"))
				files := make([]backupFile, 0, len(matches))
				for _, m := range matches {
					files = append(files, backupFile{path: m, at: backupTime(m)})
				}
				dc := deviceChanges{Device: name, DeviceID: id, Changes: []fortios.Change{}}
				from, to, note, ok := pickPair(files, window)
				if id == "" {
					// A directory without "@<id>" was not written by this
					// version of fleet backup, so nothing proves its files
					// all came from one firewall.
					note = strings.TrimPrefix(note+"; no device identity in the directory name, so backups of different firewalls sharing this name cannot be told apart", "; ")
				}
				dc.Note = note
				if ok {
					dc.From, dc.To, dc.Changes, dc.Note = diffBackups(from.path, to.path, note)
				}
				results = append(results, dc)
			}
			if len(results) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: no backups under %s; run: fortinet-cli fleet backup\n", root)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), results, flags)
			}
			for _, d := range results {
				who := d.Device
				if d.DeviceID != "" {
					who += " [" + d.DeviceID + "]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "== %s  %s -> %s  (%d change(s))\n", who, dash(d.From), dash(d.To), len(d.Changes))
				if d.Note != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "   note: %s\n", d.Note)
				}
				for _, ch := range d.Changes {
					where := ch.Table
					if ch.VDOM != "" {
						where = ch.VDOM + ": " + where
					}
					if ch.Key != "" {
						where += " " + ch.Key
					}
					fmt.Fprintf(cmd.OutOrStdout(), "   %-8s %s\n", ch.Kind, where)
					attrs := make([]string, 0, len(ch.Attributes))
					for a := range ch.Attributes {
						attrs = append(attrs, a)
					}
					sort.Strings(attrs)
					for _, a := range attrs {
						v := ch.Attributes[a]
						fmt.Fprintf(cmd.OutOrStdout(), "            %s: %s -> %s\n", a, dash(v[0]), dash(v[1]))
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Compare the newest backup with the latest one at least this much older (e.g. 7d, 30d)")
	cmd.Flags().StringVar(&dir, "dir", "", "Backup directory written by fleet backup (default <data dir>/backups)")
	cmd.Flags().StringVar(&only, "device", "", "Only these devices (comma-separated)")
	return cmd
}
