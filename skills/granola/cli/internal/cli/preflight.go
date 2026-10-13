// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

// PreflightDefaultRoot is the directory granola.py expects.
const PreflightDefaultRoot = "Documents/Dev/meeting-transcripts"

func newPreflightCmd(flags *rootFlags) *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   "preflight <id>",
		Short: "Validate a meeting is ready for extract (transcript + no duplicate)",
		Long: `Returns exit 0 when the meeting has at least 5 microphone-source
transcript segments AND at least 10 CLASSIFIED segments (microphone + speaker)
AND no file in --root already covers this meeting -- matched by id (in the
filename or the file body) or by title AND day. A same-title file from a
DIFFERENT day is a recurring meeting, not a duplicate: it is reported as
title_collision and does not affect the exit code.

Segments whose source label is unrecognized do NOT count toward the threshold:
an upstream relabel must not be able to manufacture a passing verdict.

A meeting with no far-end ("speaker") segments still passes -- solo voice
memos are one-sided by nature. The JSON reports one_sided so callers that
care can act on it.

Exit codes:
  0  ready to extract
  2  no transcript, or below the mic>=5 / classified>=10 threshold
  3  a file already covers this meeting (exact identity: id, or title+day)
  4  transcript source labels are unrecognized, so the gate cannot classify
     the transcript (upstream changed its grammar; expected
     microphone/speaker). Distinct from 2 on purpose: a relabel is not the
     same as a missing transcript.`,
		Example: `  # Pass-fail gate before running extract
  granola-cli preflight ff1186df-593b-4ce5-bb1d-70e265f4a811

  # Use a non-default duplicate-scan root
  granola-cli preflight ff1186df-593b-4ce5-bb1d-70e265f4a811 --root ~/work/meeting-notes

  # JSON for scripted pipelines
  granola-cli preflight ff1186df-593b-4ce5-bb1d-70e265f4a811 --json`,
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:typed-exit-codes": "0,2,3,4",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			id := args[0]
			if root == "" {
				home, _ := os.UserHomeDir()
				root = filepath.Join(home, PreflightDefaultRoot)
			}
			c, err := openGranolaCache()
			if err != nil {
				return err
			}
			d := c.DocumentByID(id)
			if d == nil {
				return notFoundErr(fmt.Errorf("meeting %s not in cache", id))
			}
			segs := c.TranscriptByID(id)
			mic, far, unknownSegs, unknown := countSources(segs)
			transcriptOK := transcriptUsable(mic, mic+far) && !unknownIsMaterial(unknownSegs)
			dupOf, collisions := findDuplicates(root, id, d.Title, meetingDay(d.CreatedAt))

			transcript := map[string]any{
				"present":      transcriptOK,
				"system_count": far,
				"mic_count":    mic,
				"total_count":  len(segs),
			}
			if len(unknown) > 0 {
				transcript["unknown_sources"] = unknown
				transcript["unknown_count"] = unknownSegs
			} else {
				// Advisory, never gated: a solo voice memo legitimately has no
				// far end. Only meaningful when every label classified -- with
				// unknown labels present we cannot tell a solo memo from a
				// relabelled far end, so the key is omitted rather than
				// asserting something false.
				transcript["one_sided"] = far == 0
			}
			res := map[string]any{
				"id":           id,
				"title":        d.Title,
				"transcript":   transcript,
				"duplicate_of": dupOf,
				"ok":           transcriptOK && dupOf == "",
			}
			// Advisory, never gated: a recurring meeting legitimately shares
			// its title with earlier occurrences. Surfaced so a caller can
			// eyeball the near-miss; gating on it dropped real meetings.
			if len(collisions) > 0 {
				res["title_collision"] = collisions
			}
			_ = emitJSON(cmd, flags, res)
			// Always surface drift, even when the transcript still passes --
			// a silent unknown label is what hid the dead-grammar bug.
			if len(unknown) > 0 {
				stderr("warning: %s: unrecognized transcript source label(s): %s", id, strings.Join(unknown, ", "))
			}
			switch {
			case len(segs) == 0:
				return &cliError{code: 2, err: fmt.Errorf("no transcript for %s (the meeting may not have been recorded/transcribed)", id)}
			case unknownIsMaterial(unknownSegs), len(unknown) > 0 && !transcriptUsable(mic, mic+far):
				// Drift is why we can't judge this transcript: distinct exit so
				// the caller never confuses a relabel with a missing one. Fires
				// on a material relabel even when the segment count would
				// otherwise clear the threshold.
				return &cliError{code: 4, err: fmt.Errorf("unrecognized transcript source label(s) %s on %d/%d segments for %s: the gate cannot classify this transcript (upstream grammar changed; expected microphone/speaker)", strings.Join(unknown, ", "), unknownSegs, len(segs), id)}
			case !transcriptOK:
				return &cliError{code: 2, err: fmt.Errorf("transcript below threshold for %s (mic=%d, classified=%d of %d segments; need mic>=5 and classified>=10)", id, mic, mic+far, len(segs))}
			}
			if dupOf != "" {
				return &cliError{code: 3, err: fmt.Errorf("duplicate file %s already covers %s", dupOf, id)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "Root directory to scan for duplicate files (default: ~/Documents/Dev/meeting-transcripts)")
	return cmd
}

// classifySource normalizes a transcript segment's source label to a channel.
//
// The live grammar is the public REST API's speaker.source: "microphone" (near
// end) and "speaker" (far end). The pre-7.4x desktop cache's "system"/"mixed"
// vocabulary is deliberately NOT accepted -- Granola 7.4x made that store
// app-private, so a label from it means the upstream shape changed and the
// caller must be told rather than have it silently counted as nothing. That
// silent-zero is exactly how the far end went missing unnoticed.
//
// Returns "near", "far", or "" for an unrecognized label.
func classifySource(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "microphone":
		return "near"
	case "speaker":
		return "far"
	default:
		return ""
	}
}

// countSources tallies near/far segments, counts the ones it could not
// classify, and collects their distinct labels. unknownSegs/unknownLabels are
// what make "upstream relabelled the channel" distinguishable from "this
// meeting has no transcript" -- previously both presented as a zero count.
func countSources(segs []granola.TranscriptSegment) (mic, far, unknownSegs int, unknownLabels []string) {
	seen := map[string]bool{}
	for _, s := range segs {
		switch classifySource(s.Source) {
		case "near":
			mic++
		case "far":
			far++
		default:
			unknownSegs++
			if !seen[s.Source] {
				seen[s.Source] = true
				unknownLabels = append(unknownLabels, s.Source)
			}
		}
	}
	sort.Strings(unknownLabels)
	return
}

// materialUnknownSegments is the point at which unrecognized labels stop being
// noise and start invalidating the gate's judgement.
const materialUnknownSegments = 5

// unknownIsMaterial reports whether unrecognized labels account for enough of
// the transcript that the gate can no longer be trusted.
//
// transcriptUsable already refuses to count unclassified segments toward the
// threshold, so a relabel can no longer manufacture a pass. This is the second,
// independent guard: a transcript whose near side alone clears the bar (say 50
// microphone segments) would otherwise return exit 0 while its entire far
// channel sat unreadable. A stray label or two stays a warning; a channel-wide
// relabel (what Granola 7.4x did) fails loudly.
func unknownIsMaterial(unknownSegs int) bool {
	return unknownSegs >= materialUnknownSegments
}

// transcriptUsable is the gate: enough near-side speech to write a memo from,
// and enough CLASSIFIED content to be worth extracting. Deliberately NOT
// "mic >= 5 && far >= 5" -- that encoded "every meeting is two-sided", which
// fails every solo voice memo. One-sidedness is reported instead (see
// one_sided in the preflight JSON), not gated on.
//
// classified must be mic+far, NOT len(segs). Counting unclassified segments
// toward the threshold lets an upstream relabel manufacture a green result:
// 6 microphone + 4 relabelled segments reaches 10 and passes, which is the
// original bug wearing a different hat. Only segments we can actually account
// for may satisfy the gate.
func transcriptUsable(mic, classified int) bool {
	return mic >= 5 && classified >= 10
}

// findDuplicates checks --root for a file that already covers this meeting.
//
// It returns two things deliberately kept apart:
//
//	dup        an EXACT identity match -- gates (preflight exit 3)
//	collisions same title, DIFFERENT day -- reported, never gated
//
// The split exists because the old single fuzzy signal could not tell a
// re-run from a recurring meeting. It matched any file whose name contained
// the lowercased title, and the transcripts convention names files
// "<YYYY-MM-DD> <Title> - <Org> - Full.md" -- date in the name, never the id.
// So the id branch never fired for that layout and the title branch fired on
// every recurrence: "Weekly Feedback Loop" collided with its own five prior
// occurrences. Callers read exit 3 as "skip this meeting", so real meetings
// were dropped. A gate that cannot discriminate returns no signal.
//
// Identity is established by, in order: the id in the filename, the id in the
// file body (current-format memos carry "Meeting ID:" and a notes.granola.ai
// link; the filename carries neither), or title AND day agreeing -- the last
// rung covering legacy files written before ids were recorded at all.
func findDuplicates(root, id, title, day string) (dup string, collisions []string) {
	if root == "" {
		return "", nil
	}
	// Every read is scoped to an os.Root opened on --root: a symlink that
	// points outside the transcripts tree cannot pull a foreign file into the
	// identity check, and the path handed to the reader is always relative to
	// the root rather than re-resolved from a string between the walk and the
	// read. Reported paths are re-joined onto root so output is unchanged.
	r, err := os.OpenRoot(root)
	if err != nil {
		return "", nil
	}
	defer r.Close()
	titleKey := strings.ToLower(strings.TrimSpace(title))
	_ = fs.WalkDir(r.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || dup != "" {
			return nil
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		name := d.Name()
		if id != "" && strings.Contains(name, id) {
			dup = path
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		// Normalize both sides so underscores in filenames match spaces in titles.
		base := strings.ReplaceAll(strings.ToLower(strings.TrimSuffix(name, ".md")), "_", " ")
		titleHit := titleKey != "" && strings.Contains(base, titleKey)
		// Read the body only for same-title candidates. Scanning every file in
		// the root would be correct but needlessly expensive; a file that does
		// not even share the title is not a plausible duplicate.
		if titleHit && id != "" {
			if b, rerr := r.ReadFile(filepath.FromSlash(rel)); rerr == nil && strings.Contains(string(b), id) {
				dup = path
				return nil
			}
		}
		if !titleHit {
			return nil
		}
		// Same title. Same day means the same occurrence -- gate. A different
		// day is a different occurrence of a recurring meeting -- report it so
		// the caller can see the near-miss, but never block on it.
		if day != "" && strings.Contains(name, day) {
			dup = path
			return nil
		}
		collisions = append(collisions, path)
		return nil
	})
	if dup != "" {
		return dup, nil
	}
	return "", collisions
}

// meetingDay renders an RFC3339 timestamp as YYYY-MM-DD, the prefix the
// transcripts naming convention uses. Empty when the stamp is unparseable --
// callers then simply skip the day rung rather than matching on garbage.
func meetingDay(createdAt string) string {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}
