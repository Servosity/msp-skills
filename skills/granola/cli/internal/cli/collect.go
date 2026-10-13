// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

func newCollectCmd(flags *rootFlags) *cobra.Command {
	var since, until, outDir string
	var minWords int
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect microphone-source segments across meetings into daily files",
		Long: `For each meeting since DATE, writes daily-named files (YYYY-MM-DD.md)
containing only microphone-source transcript segments, one paragraph
per segment, filtered to segments with >= --min-words words.`,
		Example: strings.Trim(`
  granola-cli collect --out ./daily-notes --since 7d
  granola-cli collect -o ./daily-notes --since 2026-07-01 --until 2026-07-15
  granola-cli collect -o ./daily-notes --since 30d --min-words 8`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			// collect writes daily files, so it needs an --out; give the
			// verifier realistic args (bounded by --since) instead of the
			// bare help fall-through.
			"pp:happy-args": "--out=/tmp/granola-dogfood-collect;--since=7d",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if outDir == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := ensureSafeDir(outDir); err != nil {
				return ioErr(err)
			}
			var from, to time.Time
			if since != "" {
				t, err := parseAnyDate(since)
				if err != nil {
					return usageErr(err)
				}
				from = t
			}
			if until != "" {
				t, err := parseAnyDate(until)
				if err != nil {
					return usageErr(err)
				}
				to = t
			}
			c, err := openGranolaCache()
			if err != nil {
				return err
			}
			perDay := map[string][]string{}
			perDayOrder := []string{}
			for _, id := range c.SortedDocumentIDs() {
				d := c.Documents[id]
				segs := c.TranscriptByID(id)
				if len(segs) == 0 {
					continue
				}
				ts, _ := granola.ParseISO(d.CreatedAt)
				if from != (time.Time{}) && ts.Before(from) {
					continue
				}
				if to != (time.Time{}) && ts.After(to) {
					continue
				}
				day := ts.Format("2006-01-02")
				if _, seen := perDay[day]; !seen {
					perDayOrder = append(perDayOrder, day)
				}
				for _, s := range segs {
					if !strings.EqualFold(s.Source, "microphone") {
						continue
					}
					if minWords > 0 && len(strings.Fields(s.Text)) < minWords {
						continue
					}
					perDay[day] = append(perDay[day], s.Text)
				}
			}
			w := cmd.OutOrStdout()
			for _, day := range perDayOrder {
				path, err := safeJoinContained(outDir, day+".md")
				if err != nil {
					fmt.Fprintf(w, `{"day":%q,"error":%q}`+"\n", day, err.Error())
					continue
				}
				body := "# " + day + "\n\n" + strings.Join(perDay[day], "\n\n") + "\n"
				if err := writeSafeFile(path, []byte(body)); err != nil {
					fmt.Fprintf(w, `{"day":%q,"error":%q}`+"\n", day, err.Error())
					continue
				}
				fmt.Fprintf(w, `{"day":%q,"file":%q,"segments":%d}`+"\n", day, path, len(perDay[day]))
			}
			if len(perDayOrder) == 0 {
				// No meetings with microphone segments in range — emit a valid
				// empty array rather than empty stdout.
				fmt.Fprintln(w, "[]")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Start date (default: all time)")
	cmd.Flags().StringVar(&until, "until", "", "End date")
	cmd.Flags().StringVarP(&outDir, "out", "o", "", "Output directory")
	cmd.Flags().IntVar(&minWords, "min-words", 0, "Minimum words per segment")
	return cmd
}
