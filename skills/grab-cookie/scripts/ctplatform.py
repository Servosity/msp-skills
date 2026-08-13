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
"""
from __future__ import annotations

import os

WINDOWS = os.name == "nt"
"""True on Windows (Credential Manager), False elsewhere (macOS Keychain)."""
