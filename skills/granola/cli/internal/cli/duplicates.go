// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
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
			// the meeting id - from the store, the file name, or the id the
			// file declares - and only a file with no id falls back to title
			// plus the YYYY-MM-DD in its name, matched to a stored meeting only
			// when exactly one fits.
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
				// Every stored meeting is its own identity: two meetings that
				// share a title and a day are still two meetings.
				rec := storeRec{id: id, title: d.Title, day: day, key: "id:" + id}
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
			// Reads are scoped to an os.Root on --root (gosec G122): a symlink
			// inside the tree cannot pull an outside file into the scan.
			if r, rerr := os.OpenRoot(root); rerr == nil {
				defer r.Close()
				_ = fs.WalkDir(r.FS(), ".", func(rel string, de fs.DirEntry, err error) error {
					if err != nil || de == nil || de.IsDir() {
						return nil
					}
					info, ierr := de.Info()
					if ierr != nil {
						return nil
					}
					native := filepath.FromSlash(rel)
					path := filepath.Join(root, native)
					name := de.Name()
					if !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
						return nil
					}
					title, day := fileTitleAndDay(name, info.ModTime())
					key := ""
					// 1. An id in the file name.
					for id, rec := range byID {
						if strings.Contains(name, id) {
							key = rec.key
							break
						}
					}
					// 2. The id the file declares (Meeting ID / Meeting Link /
					// frontmatter id), stored or not.
					if key == "" {
						if b, rerr := r.ReadFile(native); rerr == nil {
							for _, declared := range memoDeclaredIDs(string(b)) {
								if rec, ok := byID[declared]; ok {
									key = rec.key
									break
								}
								if key == "" {
									key = "id:" + declared
								}
							}
						}
					}
					// 3. Title + the day in the file name, only when exactly
					// one stored meeting fits; an ambiguous match is declined.
					// The title must match exactly under the naming convention
					// (exactMemoTitle): a bare prefix let "Plan" claim
					// "Planning - Org - Full".
					if key == "" {
						norm := strings.ToLower(strings.TrimSpace(title))
						var hits []storeRec
						for _, rec := range stored {
							rt := strings.ToLower(strings.TrimSpace(rec.title))
							if rec.day == day && exactMemoTitle(norm, rt) {
								hits = append(hits, rec)
							}
						}
						if len(hits) == 1 {
							key = hits[0].key
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
