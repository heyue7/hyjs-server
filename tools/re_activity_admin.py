#!/usr/bin/env python3
"""Audit the issued Re catalogue, or archive one explicitly selected template."""
import argparse
import collections
import getpass
import http.cookiejar
import json
import os
import urllib.error
import urllib.parse
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True, help="Origin including optional /admin/api suffix")
    parser.add_argument("--username", required=True)
    parser.add_argument("--server-id", type=int, default=1)
    parser.add_argument("--delete-template", help="Exact reviewed template ID; omit for read-only catalogue audit")
    parser.add_argument("--expected-version", help="64-character version from a previous reviewed audit")
    args = parser.parse_args()
    if args.delete_template and (not args.expected_version or len(args.expected_version) != 64):
        parser.error("--delete-template requires --expected-version from the reviewed template")
    base = args.base_url.rstrip("/")
    if not base.endswith("/admin/api"):
        base += "/admin/api"
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    csrf = ""

    def call(method, path, body=None):
        headers = {"Content-Type": "application/json"}
        if method != "GET" and csrf:
            headers["X-CSRF-Token"] = csrf
        request = urllib.request.Request(base + path, method=method, headers=headers,
                                         data=None if body is None else json.dumps(body).encode())
        try:
            with opener.open(request, timeout=30) as response:
                result = json.load(response)
        except urllib.error.HTTPError as error:
            raise RuntimeError(f"{method} {path}: HTTP {error.code}; no credential or response body logged") from None
        return result.get("data", result)

    password = os.environ.get("HYJS_ADMIN_PASSWORD") or getpass.getpass("Admin password: ")
    login = call("POST", "/auth/login", {"username": args.username, "password": password})
    csrf = login["csrfToken"]
    try:
        if args.delete_template:
            result = call("DELETE", "/activity-templates/" + urllib.parse.quote(args.delete_template, safe=""),
                          {"version": args.expected_version})
            print(json.dumps({"templateId": args.delete_template, "result": result}, ensure_ascii=False))
            return
        templates = call("GET", "/activity-templates")["items"]
        schedule = call("GET", f"/activity-schedules?serverId={args.server_id}")
        now = schedule["gameTimeMs"]
        active = [row for row in schedule["items"] if row["enabled"] and row["startAt"] <= now < row["endAt"]]
        required = {"login-total", "single-spend", "arena-wins", "plunder-count", "wanted-attempts",
                    "wanted-kills", "copy-win", "diamond-spent", "limited-sign", "work",
                    "fortune-cat", "continued-spend"}
        counts = collections.Counter(row["mode"] for row in active)
        checks = {mode: counts[mode] == 1 for mode in sorted(required)}
        tier_counts = {"login-total": 7, "single-spend": 6, "arena-wins": 6, "plunder-count": 6,
                       "wanted-attempts": 6, "wanted-kills": 6, "copy-win": 6, "diamond-spent": 5,
                       "limited-sign": 7, "work": 12, "fortune-cat": 5, "continued-spend": 7}
        content_checks = {}
        for mode in sorted(required):
            rows = [row for row in active if row["mode"] == mode]
            form = rows[0].get("form", {}) if len(rows) == 1 else {}
            content_checks[mode] = ("Re v1.0.5.1" in form.get("source", "")
                                    and len(form.get("milestones", [])) == tier_counts[mode])
        report = {"serverId": args.server_id, "gameTimeMs": now, "managed": schedule["managed"],
                  "oneEnabledCurrentInstancePerReType": checks,
                  "reSourceAndCompleteTierCounts": content_checks,
                  "activeModes": dict(counts),
                  "templates": [{key: row.get(key) for key in ("id", "name", "mode", "enabled", "version")}
                                for row in templates],
                  "sourceBoundary": "Checks management state; original protocol transactions and phone rendering require separate evidence."}
        print(json.dumps(report, ensure_ascii=False, indent=2))
        if not all(checks.values()) or not all(content_checks.values()):
            raise SystemExit(2)
    finally:
        call("POST", "/auth/logout", {})


if __name__ == "__main__":
    main()
