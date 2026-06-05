#!/usr/bin/env python3
"""Gate: a skill that claims a demo video must actually ship the video assets.

Opt-in per skill via the registry "has_video": true flag. When set, the skill
must ship two files under docs/assets/video/<slug>/:

  demo-30s.mp4     the ~30-second demo clip   (> 100 KB)
  animated-og.webm a looping social preview    (>  50 KB)

If ffprobe is on PATH, demo-30s.mp4's duration is also asserted to be in the
28.0-32.0s window (a "30-second" demo that is 8s or 90s is a mistake). When
ffprobe is absent, the duration check is skipped with a WARN line so the gate
still passes on machines without ffmpeg installed.

Skills without "has_video" (or with it false) are skipped. If no skill opts in,
the gate is a clean pass.

Pure stdlib (subprocess for the optional ffprobe call). Run locally:
    python3 tools/maintainer/check_video_assets.py
"""

from __future__ import annotations

import shutil
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
VIDEO_DIR = ROOT / "docs" / "assets" / "video"

MIN_MP4_BYTES = 100 * 1024
MIN_WEBM_BYTES = 50 * 1024
DURATION_LO = 28.0
DURATION_HI = 32.0


def ffprobe_duration(path: Path) -> float | None:
    """Return media duration in seconds via ffprobe, or None if unavailable."""
    try:
        out = subprocess.run(
            [
                "ffprobe", "-v", "error",
                "-show_entries", "format=duration",
                "-of", "csv=p=0",
                str(path),
            ],
            capture_output=True,
            text=True,
            timeout=20,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    if out.returncode != 0:
        return None
    raw = out.stdout.strip()
    try:
        return float(raw)
    except ValueError:
        return None


def check_size(errors: list[str], path: Path, min_bytes: int) -> bool:
    rel = path.relative_to(ROOT)
    if not path.exists():
        errors.append(f"missing {rel}")
        return False
    size = path.stat().st_size
    if size < min_bytes:
        errors.append(f"{rel} is {size} bytes (expected > {min_bytes})")
        return False
    return True


def main() -> int:
    errors: list[str] = []
    warns: list[str] = []
    opted_in = 0
    have_ffprobe = shutil.which("ffprobe") is not None

    for slug, entry in registry.skills().items():
        if not entry.get("has_video"):
            continue
        opted_in += 1
        sdir = VIDEO_DIR / slug
        mp4 = sdir / "demo-30s.mp4"
        webm = sdir / "animated-og.webm"

        mp4_ok = check_size(errors, mp4, MIN_MP4_BYTES)
        check_size(errors, webm, MIN_WEBM_BYTES)

        if mp4_ok:
            if not have_ffprobe:
                warns.append(
                    f"ffprobe not on PATH - skipped duration check for "
                    f"{mp4.relative_to(ROOT)}"
                )
            else:
                dur = ffprobe_duration(mp4)
                if dur is None:
                    errors.append(
                        f"{mp4.relative_to(ROOT)}: ffprobe could not read duration"
                    )
                elif not (DURATION_LO <= dur <= DURATION_HI):
                    errors.append(
                        f"{mp4.relative_to(ROOT)} duration {dur:.1f}s is outside "
                        f"{DURATION_LO}-{DURATION_HI}s"
                    )

    for w in warns:
        print(f"WARN: {w}")

    if errors:
        print("check_video_assets FAILED:")
        for e in errors:
            print(f"  {e}")
        return 1

    if opted_in == 0:
        print("PASS: video assets (no skills opted in)")
    else:
        print(f"PASS: video assets for {opted_in} skill(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
