# riverside-fm skill - the pain it closes

## The pain

Riverside.com records studio-quality podcast and video, but getting your own
content back out is a manual slog. There is no bulk export: every transcript,
audio track, and video file comes out one click at a time, per take, down the
Studio then Project then Take path. Back up a show with fifty episodes and you
are looking at hundreds of clicks. The friction is real enough that podcasters
in community groups openly ask how to "batch download Riverside recordings" just
to archive a series or free up storage - see the recurring requests in
podcasting communities such as the [Podcasting](https://www.reddit.com/r/podcasting/)
subreddit and the podcast-host Facebook groups where the batch-download question
keeps coming up.

The official Riverside API would solve this, but it is gated behind a
custom-priced Business plan. Pro, Live, and Webinar accounts - which is most
independent creators - get no supported programmatic way to back up, search, or
re-format their own recordings.

## What this skill does about it

- **`bulk export`** - walk a whole studio (or a date range) and write every
  take's transcript, per-participant assets manifest, and HLS manifest URL to
  disk, with a resume cursor so an interrupted run never restarts the walk.
- **`grab`** - one command that downloads whichever asset exists for a recording
  in priority order: transcript first, then audio tracks, then HLS video.
- **`search`** - full-text search across every transcript you have synced into
  the local SQLite mirror, so a quote that lived in one of last year's episodes
  is one query away instead of an afternoon of scrubbing.
- **`transcripts convert`** - turn Riverside's voice-activity transcript JSON
  into WebVTT, SRT, plain text, JSON, or speaker-grouped Markdown - caption
  formats the Riverside UI never exposes.
- **`clips harvest`** - gate on AI-generation status, list a project's Magic Clip
  exports, refresh each clip's short-lived signed URL, and optionally download
  the MP4s before the URL expires.

## Status

Beta. Validated against the Riverside API surface; the closed-loop receipt (a named
user running it live against their own production account at a Build Session) is
tracked separately and added here as `video.md` once it exists.
