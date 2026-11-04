#!/usr/bin/env python3
"""Small, read-only client for the Nizlo RMM API."""

from __future__ import annotations

import argparse
import json
import os
import sys
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlsplit, urlunsplit
from urllib.request import Request, urlopen


DEFAULT_BASE_URL = "https://app.nizlo.com/api/v1"
ALLOWED_PATHS = {
    "dashboard": "/dashboard/summary",
    "customers": "/customers",
    "sites": "/sites",
    "devices": "/devices",
    "alerts": "/alerts",
}


class ClientError(RuntimeError):
    """A safe-to-print client or HTTP error."""


def normalized_base_url(value: str) -> str:
    value = value.strip().rstrip("/")
    parts = urlsplit(value)
    if parts.scheme not in {"http", "https"} or not parts.netloc:
        raise ClientError("NIZLO_BASE_URL must be an absolute HTTP(S) URL")
    if parts.query or parts.fragment:
        raise ClientError("NIZLO_BASE_URL must not contain a query or fragment")
    return urlunsplit((parts.scheme, parts.netloc, parts.path, "", ""))


def build_url(base_url: str, command: str, params: dict[str, Any]) -> str:
    if command not in ALLOWED_PATHS:
        raise ClientError(f"Unsupported read-only command: {command}")
    query = urlencode(
        [(key, str(value)) for key, value in params.items() if value is not None]
    )
    url = normalized_base_url(base_url) + ALLOWED_PATHS[command]
    return f"{url}?{query}" if query else url


def redact(text: str, token: str) -> str:
    safe = text.replace(token, "[REDACTED]") if token else text
    return safe[:1000]


def request_json(url: str, token: str) -> Any:
    if not token:
        raise ClientError("NIZLO_API_TOKEN is required for live queries")
    request = Request(
        url,
        headers={
            "Accept": "application/json",
            "Authorization": f"Bearer {token}",
            "User-Agent": "msp-skills-nizlo/0.1.0",
        },
        method="GET",
    )
    try:
        with urlopen(request, timeout=30) as response:
            return json.loads(response.read().decode("utf-8"))
    except HTTPError as exc:
        body = exc.read(4096).decode("utf-8", errors="replace")
        raise ClientError(
            f"Nizlo API returned HTTP {exc.code}: {redact(body, token)}"
        ) from None
    except URLError as exc:
        raise ClientError(f"Unable to reach the Nizlo API: {exc.reason}") from None
    except json.JSONDecodeError:
        raise ClientError("Nizlo API returned a non-JSON response") from None


def add_paging(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--page", type=int, default=1)
    parser.add_argument("--page-size", type=int, default=50)


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(description=__doc__)
    root.add_argument(
        "--base-url",
        default=os.environ.get("NIZLO_BASE_URL", DEFAULT_BASE_URL),
        help="Nizlo API root (default: %(default)s)",
    )
    root.add_argument(
        "--selfcheck",
        action="store_true",
        help="run offline routing and redaction checks",
    )
    commands = root.add_subparsers(dest="command")

    commands.add_parser("dashboard", help="get the fleet dashboard summary")

    customers = commands.add_parser("customers", help="list customers")
    add_paging(customers)
    customers.add_argument("--search")
    customers.add_argument("--lifecycle", choices=("active", "archived"))

    sites = commands.add_parser("sites", help="list sites")
    add_paging(sites)
    sites.add_argument("--search")
    sites.add_argument("--customer-id")
    sites.add_argument("--lifecycle", choices=("active", "archived"))

    devices = commands.add_parser("devices", help="list endpoints")
    add_paging(devices)
    devices.add_argument("--search")
    devices.add_argument("--customer-id")
    devices.add_argument("--site-id")
    devices.add_argument("--status", choices=("online", "offline", "unknown"))
    devices.add_argument("--kind", choices=("server", "workstation"))
    devices.add_argument("--platform", choices=("windows", "macos", "linux"))
    devices.add_argument(
        "--attention",
        choices=(
            "alerts",
            "reboot",
            "low_disk",
            "agent_outdated",
            "agent_unknown",
            "maintenance",
            "patch_scan_due",
        ),
    )
    devices.add_argument("--sort", choices=("hostname", "customer", "lastSeen"))
    devices.add_argument("--lifecycle", choices=("active", "archived"))

    alerts = commands.add_parser("alerts", help="list alerts")
    add_paging(alerts)
    alerts.add_argument(
        "--status",
        choices=("active", "open", "acknowledged", "snoozed", "resolved"),
    )
    alerts.add_argument("--severity", choices=("info", "warning", "critical"))
    alerts.add_argument("--search")
    alerts.add_argument("--device-id")
    alerts.add_argument("--customer-id")
    alerts.add_argument("--site-id")
    alerts.add_argument("--assigned")
    return root


def command_params(args: argparse.Namespace) -> dict[str, Any]:
    ignored = {"base_url", "selfcheck", "command"}
    params = {
        key.replace("_", "-") if key == "page_size" else key: value
        for key, value in vars(args).items()
        if key not in ignored and value is not None
    }
    if "page-size" in params:
        params["pageSize"] = params.pop("page-size")
    for key in ("customer_id", "site_id", "device_id"):
        if key in params:
            params[key.replace("_id", "Id")] = params.pop(key)
    if "attention" in params:
        params["attention"] = params["attention"].upper()
    return params


def selfcheck() -> None:
    base = "https://example.invalid/api/v1/"
    assert build_url(base, "dashboard", {}) == (
        "https://example.invalid/api/v1/dashboard/summary"
    )
    assert build_url(base, "devices", {"status": "offline", "pageSize": 25}) == (
        "https://example.invalid/api/v1/devices?status=offline&pageSize=25"
    )
    secret = "nizlo_pat_selfcheck_secret"
    assert secret not in redact(f"failed for {secret}", secret)
    try:
        build_url(base, "mutate", {})
    except ClientError:
        pass
    else:
        raise AssertionError("route allowlist accepted an unknown command")
    print("nizlo_api.py selfcheck OK (offline, read-only routes + redaction)")


def main() -> int:
    args = parser().parse_args()
    if args.selfcheck:
        selfcheck()
        return 0
    if not args.command:
        parser().error("a command is required unless --selfcheck is used")
    token = os.environ.get("NIZLO_API_TOKEN", "")
    url = build_url(args.base_url, args.command, command_params(args))
    result = request_json(url, token)
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ClientError as exc:
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(2) from None
