#!/usr/bin/env python3
"""Small, read-only client for the Nizlo RMM API."""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timedelta, timezone
from http.client import HTTPException
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlsplit, urlunsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


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


class RejectRedirects(HTTPRedirectHandler):
    """Convert every redirect into HTTPError before credentials can be resent."""

    def redirect_request(
        self,
        request: Request,
        fp: Any,
        code: int,
        msg: str,
        headers: Any,
        newurl: str,
    ) -> None:
        return None


_OPENER = build_opener(RejectRedirects())


def open_request(request: Request, timeout: int) -> Any:
    """Open one request through a redirect-rejecting HTTPS client."""
    return _OPENER.open(request, timeout=timeout)


def normalized_base_url(value: str) -> str:
    """Return a validated API root without a trailing slash."""
    value = value.strip().rstrip("/")
    parts = urlsplit(value)
    if parts.scheme != "https" or not parts.netloc:
        raise ClientError("NIZLO_BASE_URL must be an absolute HTTPS URL")
    if parts.query or parts.fragment:
        raise ClientError("NIZLO_BASE_URL must not contain a query or fragment")
    return urlunsplit((parts.scheme, parts.netloc, parts.path, "", ""))


def build_url(base_url: str, command: str, params: dict[str, Any]) -> str:
    """Build a URL for one allowlisted read-only API route."""
    if command not in ALLOWED_PATHS:
        raise ClientError(f"Unsupported read-only command: {command}")
    query = urlencode(
        [(key, str(value)) for key, value in params.items() if value is not None]
    )
    url = normalized_base_url(base_url) + ALLOWED_PATHS[command]
    return f"{url}?{query}" if query else url


def redact(text: str, token: str) -> str:
    """Remove a bearer token and bound diagnostic response text."""
    safe = text.replace(token, "[REDACTED]") if token else text
    return safe[:1000]


def request_json(url: str, token: str) -> Any:
    """GET one API URL and return its decoded JSON response."""
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
        with open_request(request, timeout=30) as response:
            return json.loads(response.read().decode("utf-8"))
    except HTTPError as exc:
        body = exc.read(4096).decode("utf-8", errors="replace")
        raise ClientError(
            f"Nizlo API returned HTTP {exc.code}: {redact(body, token)}"
        ) from None
    except URLError as exc:
        raise ClientError(f"Unable to reach the Nizlo API: {exc.reason}") from None
    except (OSError, HTTPException) as exc:
        raise ClientError(f"Nizlo API response was interrupted: {exc}") from None
    except (json.JSONDecodeError, UnicodeDecodeError):
        raise ClientError("Nizlo API returned a non-JSON response") from None


def page_size(value: str) -> int:
    """Parse an API page size in Nizlo's supported 1-200 range."""
    parsed = int(value)
    if parsed < 1 or parsed > 200:
        raise argparse.ArgumentTypeError("page size must be between 1 and 200")
    return parsed


def positive_days(value: str) -> int:
    """Parse a positive whole-day stale threshold."""
    parsed = int(value)
    if parsed < 1 or parsed > 100000:
        raise argparse.ArgumentTypeError("days must be between 1 and 100000")
    return parsed


def positive_page(value: str) -> int:
    """Parse a positive API page number."""
    parsed = int(value)
    if parsed < 1:
        raise argparse.ArgumentTypeError("page must be at least 1")
    return parsed


def add_paging(parser: argparse.ArgumentParser) -> None:
    """Add the common pagination flags to a command parser."""
    parser.add_argument("--page", type=positive_page, default=1)
    parser.add_argument("--page-size", type=page_size, default=50)


def parser() -> argparse.ArgumentParser:
    """Build the command-line parser for supported read-only workflows."""
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

    stale = commands.add_parser(
        "stale-endpoints",
        help="group endpoints not seen for a number of days by customer",
    )
    stale.add_argument("--days", type=positive_days, default=7)
    stale.add_argument("--page-size", type=page_size, default=100)

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
    """Translate parsed CLI flags into Nizlo API query parameters."""
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


def parse_timestamp(value: Any) -> datetime | None:
    """Parse an API ISO-8601 timestamp as an aware UTC datetime."""
    if value is None:
        return None
    if not isinstance(value, str):
        raise ClientError("Nizlo API returned an invalid lastSeenAt timestamp")
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        raise ClientError(
            "Nizlo API returned an invalid lastSeenAt timestamp"
        ) from None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def build_stale_report(
    devices: list[dict[str, Any]], days: int, now: datetime | None = None
) -> dict[str, Any]:
    """Group endpoints at or beyond a last-seen cutoff by customer."""
    generated_at = now or datetime.now(timezone.utc)
    if generated_at.tzinfo is None:
        generated_at = generated_at.replace(tzinfo=timezone.utc)
    generated_at = generated_at.astimezone(timezone.utc)
    cutoff = generated_at - timedelta(days=days)
    grouped: dict[tuple[str | None, str], list[dict[str, Any]]] = {}
    for device in devices:
        last_seen_value = device.get("lastSeenAt")
        last_seen = parse_timestamp(last_seen_value)
        if last_seen is not None and last_seen > cutoff:
            continue
        customer = device.get("customer")
        customer_name = (
            customer.get("name")
            if isinstance(customer, dict) and customer.get("name")
            else "Unassigned"
        )
        customer_id = (
            str(customer.get("id"))
            if isinstance(customer, dict) and customer.get("id")
            else None
        )
        inventory = device.get("inventory")
        os_name = (
            inventory.get("osName")
            if isinstance(inventory, dict) and inventory.get("osName")
            else device.get("agentPlatform") or "Unknown"
        )
        endpoint_name = (
            device.get("displayName") or device.get("hostname") or device.get("id")
        )
        grouped.setdefault((customer_id, str(customer_name)), []).append(
            {
                "id": device.get("id"),
                "name": endpoint_name,
                "os": os_name,
                "lastSeenAt": last_seen_value,
            }
        )
    customers = []
    for customer_id, customer_name in sorted(
        grouped, key=lambda item: (item[1].casefold(), item[0] or "")
    ):
        endpoints = sorted(
            grouped[(customer_id, customer_name)],
            key=lambda item: str(item.get("name") or "").casefold(),
        )
        customers.append(
            {
                "customerId": customer_id,
                "customer": customer_name,
                "endpoints": endpoints,
            }
        )
    return {
        "generatedAt": generated_at.isoformat().replace("+00:00", "Z"),
        "cutoff": cutoff.isoformat().replace("+00:00", "Z"),
        "days": days,
        "total": sum(len(item["endpoints"]) for item in customers),
        "customers": customers,
    }


def fetch_stale_endpoints(
    base_url: str, token: str, days: int, requested_page_size: int
) -> dict[str, Any]:
    """Fetch every active endpoint page and build a grouped stale report."""
    devices_by_id: dict[str, dict[str, Any]] = {}
    expected_total: int | None = None
    page = 1
    while True:
        url = build_url(
            base_url,
            "devices",
            {
                "page": page,
                "pageSize": requested_page_size,
                "sort": "lastSeen",
                "lifecycle": "active",
            },
        )
        payload = request_json(url, token)
        if not isinstance(payload, dict) or not isinstance(payload.get("items"), list):
            raise ClientError("Nizlo API returned an invalid device-list response")
        items = payload["items"]
        if not all(isinstance(item, dict) for item in items):
            raise ClientError("Nizlo API returned an invalid device-list item")
        total = payload.get("total")
        if isinstance(total, int):
            if expected_total is None:
                expected_total = total
            elif total != expected_total:
                raise ClientError(
                    "Nizlo device inventory changed during pagination; retry"
                )
        for item in items:
            device_id = item.get("id")
            if not isinstance(device_id, str) or not device_id:
                raise ClientError("Nizlo API returned a device without an ID")
            if device_id in devices_by_id:
                raise ClientError(
                    "Nizlo device inventory changed during pagination; retry"
                )
            devices_by_id[device_id] = item
        if len(items) < requested_page_size:
            break
        if expected_total is not None and len(devices_by_id) >= expected_total:
            break
        page += 1
        if page > 1000:
            raise ClientError("Nizlo API pagination exceeded the safety limit")
    if expected_total is not None and len(devices_by_id) != expected_total:
        raise ClientError("Nizlo device inventory changed during pagination; retry")
    return build_stale_report(list(devices_by_id.values()), days)


def selfcheck() -> None:
    """Validate routing, redaction, and stale-report logic without network I/O."""
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
    report = build_stale_report(
        [
            {
                "id": "device-old",
                "hostname": "ACME-OLD",
                "lastSeenAt": "2026-01-01T00:00:00Z",
                "customer": {"id": "customer-one", "name": "Acme"},
                "inventory": {"osName": "Windows 11"},
            },
            {
                "id": "device-current",
                "hostname": "ACME-CURRENT",
                "lastSeenAt": "2026-01-10T11:00:00Z",
                "customer": {"id": "customer-one", "name": "Acme"},
                "agentPlatform": "WINDOWS",
            },
            {
                "id": "device-other-acme",
                "hostname": "OTHER-ACME-OLD",
                "lastSeenAt": None,
                "customer": {"id": "customer-two", "name": "Acme"},
                "agentPlatform": "MACOS",
            },
        ],
        7,
        datetime(2026, 1, 10, 12, 0, tzinfo=timezone.utc),
    )
    assert report["total"] == 2
    assert len(report["customers"]) == 2
    assert report["customers"][0]["customerId"] == "customer-one"
    assert report["customers"][0]["endpoints"][0]["os"] == "Windows 11"
    seen: dict[str, str | None] = {}

    class Response:
        """Minimal context-managed response for an offline request check."""

        def __enter__(self) -> "Response":
            return self

        def __exit__(self, *exc: object) -> None:
            return None

        def read(self) -> bytes:
            return b"{}"

    def fake_open_request(request: Request, timeout: int) -> Response:
        seen["authorization"] = request.get_header("Authorization")
        seen["timeout"] = str(timeout)
        return Response()

    original_open_request = globals()["open_request"]
    globals()["open_request"] = fake_open_request
    try:
        assert request_json(base + "dashboard/summary", secret) == {}
    finally:
        globals()["open_request"] = original_open_request
    assert seen["authorization"] == f"Bearer {secret}"
    assert seen["timeout"] == "30"

    class InvalidUtf8Response(Response):
        """Return bytes that cannot be decoded as UTF-8."""

        def read(self) -> bytes:
            return b"\xff"

    def fake_invalid_utf8(request: Request, timeout: int) -> InvalidUtf8Response:
        return InvalidUtf8Response()

    globals()["open_request"] = fake_invalid_utf8
    try:
        try:
            request_json(base + "dashboard/summary", secret)
        except ClientError as exc:
            assert str(exc) == "Nizlo API returned a non-JSON response"
        else:
            raise AssertionError("invalid UTF-8 escaped the controlled error path")
    finally:
        globals()["open_request"] = original_open_request

    class InterruptedResponse(Response):
        """Simulate a connection failure while reading the response body."""

        def read(self) -> bytes:
            raise ConnectionResetError("connection reset during self-check")

    def fake_interrupted_read(request: Request, timeout: int) -> InterruptedResponse:
        return InterruptedResponse()

    globals()["open_request"] = fake_interrupted_read
    try:
        try:
            request_json(base + "dashboard/summary", secret)
        except ClientError as exc:
            assert str(exc).startswith("Nizlo API response was interrupted:")
        else:
            raise AssertionError("read interruption escaped the controlled error path")
    finally:
        globals()["open_request"] = original_open_request
    print(
        "nizlo_api.py selfcheck OK "
        "(offline, HTTPS routes + bearer header + controlled errors + "
        "redaction + stale report)"
    )


def main() -> int:
    """Run the requested CLI workflow and print its JSON result."""
    args = parser().parse_args()
    if args.selfcheck:
        selfcheck()
        return 0
    if not args.command:
        parser().error("a command is required unless --selfcheck is used")
    token = os.environ.get("NIZLO_API_TOKEN", "")
    if args.command == "stale-endpoints":
        result = fetch_stale_endpoints(args.base_url, token, args.days, args.page_size)
    else:
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
