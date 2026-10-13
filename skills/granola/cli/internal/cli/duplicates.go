// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

func newDuplicatesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "duplicates",
		Short: "Detect duplicate meeting captures between cache and filesystem",
	}
	cmd.AddCommand(newDuplicatesScanCmd(flags))
	return cmd
}

func newDuplicatesScanCmd(flags *rootFlags) *cobra.Command {
	var root, since string
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Group stored meetings and memo files by meeting id or (title, day) and surface duplicates",
		Example: strings.Trim(`
  granola-cli duplicates scan
  granola-cli duplicates scan --since 30d
  granola-cli duplicates scan --root ~/Documents/transcripts --since 2026-07-01`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			home, _ := os.UserHomeDir()
			if root == "" {
				root = filepath.Join(home, PreflightDefaultRoot)
			}
			warnIfRootMissing(root)
			var from time.Time
			if since != "" {
				t, err := parseAnyDate(since)
				if err != nil {
					return usageErr(err)
				}
				from = t
			}
			c, err := openGranolaCache()
			if err != nil {
				return err
			}
			groups := map[string][]map[string]any{}
			// Hand-wired (handfixes.json: duplicates-cross-source-identity):
			// the generated scan fingerprinted (title, day, attendee set) on
			// both sides, but a file on disk never carries an attendee set and
			// its day came from the file's mtime, so a stored meeting and its
			// own memo file could never land in the same group. Identity is now
			// the meeting id when a file name carries it, else (title, day),
			// with the day read from the file name's YYYY-MM-DD prefix.
			type storeRec struct{ id, title, day, key string }
			var stored []storeRec
			byID := map[string]storeRec{}
			for id, d := range c.Documents {
				ts, _ := granola.ParseISO(d.CreatedAt)
				if !from.IsZero() && ts.Before(from) {
					continue
				}
				day := ""
				if !ts.IsZero() {
					day = ts.UTC().Format("2006-01-02")
				}
				rec := storeRec{id: id, title: d.Title, day: day, key: fingerprintMeeting(d.Title, ts, nil)}
				stored = append(stored, rec)
				byID[id] = rec
				groups[rec.key] = append(groups[rec.key], map[string]any{
					"source": "cache",
					"id":     id,
					"title":  d.Title,
					"date":   day,
				})
			}
			// Filesystem contributions.
			if _, err := os.Stat(root); err == nil {
				_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
					if err != nil || info == nil || info.IsDir() {
						return nil
					}
					name := info.Name()
					if !strings.HasSuffix(name, ".md") {
						return nil
					}
					title, day := fileTitleAndDay(name, info.ModTime())
					key := ""
					for id, rec := range byID {
						if strings.Contains(name, id) {
							key = rec.key
							break
						}
					}
					if key == "" {
						norm := strings.ToLower(strings.TrimSpace(title))
						for _, rec := range stored {
							rt := strings.ToLower(strings.TrimSpace(rec.title))
							if rec.day == day && rt != "" && strings.HasPrefix(norm, rt) {
								key = rec.key
								break
							}
						}
					}
					if key == "" {
						t, _ := time.Parse("2006-01-02", day)
						key = fingerprintMeeting(title, t, nil)
					}
					groups[key] = append(groups[key], map[string]any{
						"source": "fs",
						"path":   path,
						"title":  title,
						"date":   day,
					})
					return nil
				})
			}
			w := cmd.OutOrStdout()
			keys := make([]string, 0, len(groups))
			for k, g := range groups {
				if len(g) >= 2 {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				_ = emitNDJSONLine(w, map[string]any{
					"fingerprint": k,
					"count":       len(groups[k]),
					"items":       groups[k],
				})
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "Filesystem root to scan")
	cmd.Flags().StringVar(&since, "since", "", "Lower-bound date")
	return cmd
}

// fingerprintMeeting returns a stable identity string for dedup detection.
// Attendees are sorted internally so callers may pass them in any order.
func fingerprintMeeting(title string, t time.Time, attendees []string) string {
	day := ""
	if !t.IsZero() {
		day = t.UTC().Format("2006-01-02")
	}
	tk := strings.ToLower(strings.TrimSpace(title))
	sorted := append([]string(nil), attendees...)
	for i := range sorted {
		sorted[i] = strings.ToLower(strings.TrimSpace(sorted[i]))
	}
	sort.Strings(sorted)
	att := strings.Join(sorted, ",")
	h := sha256.New()
	h.Write([]byte(tk))
	h.Write([]byte{0})
	h.Write([]byte(day))
	h.Write([]byte{0})
	h.Write([]byte(att))
	return hex.EncodeToString(h.Sum(nil))
}

func nameToTitle(name string) string {
	base := strings.TrimSuffix(name, ".md")
	base = strings.TrimPrefix(base, "full_")
	base = strings.TrimPrefix(base, "summary_")
	base = strings.TrimPrefix(base, "metadata_")
	return strings.ReplaceAll(base, "_", " ")
}

// Ensure fmt used.
var _ = fmt.Sprintf

// memoDatePrefix matches the "YYYY-MM-DD " prefix of the transcripts naming
// convention ("<YYYY-MM-DD> <Title> - <Org> - Full.md").
var memoDatePrefix = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})[ _]+(.+)$`)

// fileTitleAndDay reads a memo file's title and day from its name. The day
// comes from a leading YYYY-MM-DD when present (the file's mtime is when it
// was written, not when the meeting happened) and falls back to the mtime.
func fileTitleAndDay(name string, mtime time.Time) (string, string) {
	title := nameToTitle(name)
	if m := memoDatePrefix.FindStringSubmatch(title); m != nil {
		return strings.TrimSpace(m[2]), m[1]
	}
	return title, mtime.UTC().Format("2006-01-02")
}
