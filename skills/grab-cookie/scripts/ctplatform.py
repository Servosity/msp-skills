# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Minimal platform shim for credgrab.

The vendored ``credstore.py`` (from Servosity's connect-tool) imports
``ctplatform`` for one thing: the ``WINDOWS`` boolean that selects the Windows
Credential Manager backend over the macOS Keychain. connect-tool's full
ctplatform also carries browser/run helpers credgrab does not use, so we ship
just the flag here rather than the whole module.

``MACOS`` is tested separately rather than inferred as "not Windows": credgrab
supports exactly two credential stores, and a Linux run must fail with a clear
message instead of shelling out to a ``security`` binary that is not there.
"""
from __future__ import annotations

import os
import sys

WINDOWS = os.name == "nt"
"""True on Windows (Credential Manager)."""

MACOS = sys.platform == "darwin"
"""True on macOS (Keychain). Every other platform is unsupported."""
