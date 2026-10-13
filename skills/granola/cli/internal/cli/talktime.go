// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

func newTalktimeCmd(flags *rootFlags) *cobra.Command {
	var by, since, until, last string
	cmd := &cobra.Command{
		Use:   "talktime [<meeting-id>]",
		Short: "Per-source talk time for one meeting or rolled up across meetings",
		Long: `Without args: returns microphone vs system seconds for one meeting
(microphone is the person running Granola; system is everyone else).
With --by participant --since DATE: aggregates the same split across
meetings. Granola's public API names no meeting owner, so the roll-up is
always your microphone versus everyone else, not a per-person ranking.`,
		Example: strings.Trim(`
  granola-cli talktime not_06Yq6JtogRihEr
  granola-cli talktime --by participant --since 30d
  granola-cli talktime --last 7d --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			c, err := openGranolaCache()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				id := args[0]
				segs := c.TranscriptByID(id)
				if len(segs) == 0 {
					return notFoundErr(fmt.Errorf("no cached transcript for %s", id))
				}
				agg := aggregateBySources(segs)
				return emitJSON(cmd, flags, agg)
			}
			from, to, err := parseTimeWindow(last, since, until)
			if err != nil {
				return usageErr(err)
			}
			ids := selectDocsInWindow(c, from, to, 0)
			if by == "" || by == "participant" {
				// Per-email aggregation. For 2-attendee meetings, microphone =
				// the "creator" email; system = the other attendee. For meetings
				// with >2 attendees we cannot disambiguate system-source speakers
				// from the transcript alone, so system seconds split evenly across
				// non-creator attendees and the key is prefixed "rollup:" so the
				// user knows attribution is heuristic, not from speaker diarization.
				type row struct {
					micSec float64
					sysSec float64
					unkSec float64
				}
				out := map[string]*row{}
				// unk is tracked separately from sys: folding unclassified time
				// into system_seconds would let a consumer summing that field
				// count dead vocabulary as far-end talk time.
				bump := func(key string, mic, sys, unk float64) {
					r, ok := out[key]
					if !ok {
						r = &row{}
						out[key] = r
					}
					r.micSec += mic
					r.sysSec += sys
					r.unkSec += unk
				}
				for _, id := range ids {
					segs := c.TranscriptByID(id)
					if len(segs) == 0 {
						continue
					}
					md := c.MeetingMetadataByID(id)
					micOwner := ""
					if md != nil && md.Creator != nil {
						micOwner = strings.ToLower(md.Creator.Email)
					}
					var others []string
					if md != nil {
						for _, a := range md.Attendees {
							e := strings.ToLower(a.Email)
							if e != "" && e != micOwner {
								others = append(others, e)
							}
						}
					}
					micSec, sysSec, unkSec := sourceSeconds(segs)
					switch {
					case micOwner != "" && len(others) == 1:
						bump(micOwner, micSec, 0, 0)
						bump(others[0], 0, sysSec, 0)
					case micOwner != "" && len(others) > 1:
						bump(micOwner, micSec, 0, 0)
						share := sysSec / float64(len(others))
						for _, e := range others {
							bump("rollup:"+e, 0, share, 0)
						}
					default:
						bump("rollup:source:microphone", micSec, 0, 0)
						bump("rollup:source:system", 0, sysSec, 0)
					}
					// Time we couldn't attribute to a channel can't be
					// attributed to a person either -- bucket it visibly
					// rather than dropping it out of the totals.
					if unkSec > 0 {
						bump("rollup:source:unknown", 0, 0, unkSec)
					}
				}
				resp := []map[string]any{}
				for k, r := range out {
					total := r.micSec + r.sysSec + r.unkSec
					resp = append(resp, map[string]any{
						"key":             k,
						"mic_seconds":     r.micSec,
						"system_seconds":  r.sysSec,
						"unknown_seconds": r.unkSec,
						"total_seconds":   total,
						"total_minutes":   total / 60.0,
					})
				}
				return emitJSON(cmd, flags, resp)
			}
			return usageErr(fmt.Errorf("invalid --by %q", by))
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Aggregate by: participant (default)")
	cmd.Flags().StringVar(&since, "since", "", "Start date")
	cmd.Flags().StringVar(&until, "until", "", "End date")
	cmd.Flags().StringVar(&last, "last", "", "Time window (e.g. 7d)")
	return cmd
}

// aggregateBySources returns per-source totals for one transcript.
func aggregateBySources(segs []granola.TranscriptSegment) map[string]any {
	micSec, sysSec, unknownSec := sourceSeconds(segs)
	avgConf := 0.0
	if len(segs) > 0 {
		sum := 0.0
		for _, s := range segs {
			sum += s.Confidence
		}
		avgConf = sum / float64(len(segs))
	}
	return map[string]any{
		"microphone_seconds": micSec,
		"system_seconds":     sysSec,
		// Time from segments whose source label didn't classify. Non-zero
		// means upstream changed its grammar -- without this it vanished out
		// of total_seconds silently.
		"unknown_seconds":    unknownSec,
		"total_seconds":      micSec + sysSec + unknownSec,
		"microphone_minutes": micSec / 60.0,
		"system_minutes":     sysSec / 60.0,
		"segment_count":      len(segs),
		"confidence_avg":     avgConf,
	}
}

// sourceSeconds totals seconds per channel. Returns (micSec, farSec, unknownSec).
// Shares classifySource with preflight so talktime can never disagree with the
// gate about what a label means -- the duplicated switch here is what made
// talktime report system_seconds: 0 for every meeting.
func sourceSeconds(segs []granola.TranscriptSegment) (float64, float64, float64) {
	var mic, far, unknown float64
	for _, s := range segs {
		dur := segSeconds(s)
		switch classifySource(s.Source) {
		case "near":
			mic += dur
		case "far":
			far += dur
		default:
			unknown += dur
		}
	}
	return mic, far, unknown
}

func segSeconds(s granola.TranscriptSegment) float64 {
	st, err1 := granola.ParseISO(s.StartTimestamp)
	en, err2 := granola.ParseISO(s.EndTimestamp)
	if err1 != nil || err2 != nil || st.IsZero() || en.IsZero() {
		return 0
	}
	d := en.Sub(st).Seconds()
	if d < 0 {
		return 0
	}
	return d
}

// Ensure time imported.
var _ = time.Now
