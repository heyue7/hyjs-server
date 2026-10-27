#!/usr/bin/env python3
"""Serve a local admin build with a narrowly gated production API proxy.

Set HYJS_PREVIEW_UPSTREAM to the approved origin before starting. By default
the proxy forwards GET/HEAD and exactly POST /admin/api/auth/login. An explicit
isolated-test mode permits two PUT paths only when the upstream is the fixed
loopback test service. It never logs request targets, cookies or bodies.
"""

from __future__ import annotations

import argparse
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import mimetypes
import os
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit


PROJECT_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_ASSETS = PROJECT_ROOT / "build/feedback78/admin-preview"
HOP_HEADERS = {
    "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
    "te", "trailer", "transfer-encoding", "upgrade", "access-control-allow-origin",
    "access-control-allow-credentials", "strict-transport-security",
}
ISOLATED_UPSTREAM = "127.0.0.1:18861"
ISOLATED_PORT = 18779
ISOLATED_PUT_PATHS = frozenset({
    "/admin/api/sprout-settings",
    "/admin/api/shops/vip/products/p3vip10_growth_local",
})


def upstream_origin(value: str):
    parsed = urlsplit(value)
    try:
        port = parsed.port
    except ValueError as exc:
        raise ValueError("Invalid upstream origin") from exc
    if (
        parsed.scheme not in {"http", "https"}
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.path not in {"", "/"}
        or parsed.query
        or parsed.fragment
        or (port is not None and not 1 <= port <= 65535)
    ):
        raise ValueError("Upstream must be an HTTP(S) origin without credentials or a path")
    return parsed


def request_target(method: str, target: str, isolated_save_check: bool = False):
    """Return the decoded path and whether it may reach the upstream."""
    parsed = urlsplit(target)
    if parsed.scheme or parsed.netloc or parsed.fragment or not parsed.path.startswith("/"):
        raise ValueError("Invalid request target")
    if re.search(r"%(?:2f|5c|00|0a|0d)", parsed.path, re.IGNORECASE):
        raise ValueError("Invalid encoded path")
    path = unquote(parsed.path, errors="strict")
    if "\\" in path or any(part in {".", ".."} for part in path.split("/")):
        raise ValueError("Invalid path")
    if any(ord(char) < 32 or ord(char) == 127 for char in path):
        raise ValueError("Invalid path")
    is_api = path == "/admin/api" or path.startswith("/admin/api/")
    permitted = is_api and (
        method in {"GET", "HEAD"}
        or (method == "POST" and target == "/admin/api/auth/login")
        or (isolated_save_check and method == "PUT" and target in ISOLATED_PUT_PATHS)
    )
    return path, is_api, permitted


def local_origin_allowed(value: str, port: int) -> bool:
    try:
        parsed = urlsplit(value)
        return (
            parsed.scheme == "http"
            and parsed.hostname in {"127.0.0.1", "localhost"}
            and parsed.port == port
            and parsed.path == ""
            and not parsed.username
            and not parsed.password
            and not parsed.query
            and not parsed.fragment
        )
    except ValueError:
        return False


def local_cookie(value: str) -> str:
    # The app issues host-only /admin cookies. If an upstream edge adds a
    # Domain attribute, retain host-only semantics for the local preview.
    parts = value.split(";")
    return ";".join(part for part in parts if not part.strip().lower().startswith("domain="))


class PreviewServer(ThreadingHTTPServer):
    daemon_threads = True
    request_queue_size = 128

    def __init__(self, address, assets: Path, upstream, isolated_save_check: bool = False):
        if isolated_save_check and (
            address != ("127.0.0.1", ISOLATED_PORT)
            or upstream.scheme != "http"
            or upstream.netloc != ISOLATED_UPSTREAM
        ):
            raise ValueError("Isolated save checks require the fixed loopback port and test upstream")
        super().__init__(address, PreviewHandler)
        self.assets = assets.resolve()
        self.upstream = upstream
        self.isolated_save_check = isolated_save_check

    def handle_error(self, request, client_address):
        # Never print request data or a traceback that may contain credentials.
        return


class PreviewHandler(BaseHTTPRequestHandler):
    server: PreviewServer

    def log_message(self, fmt, *args):
        return

    def reply(self, status: int, message: str):
        body = json.dumps({"code": status, "message": message}, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Connection", "close")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)
        self.close_connection = True

    def valid_host(self):
        port = self.server.server_address[1]
        return self.headers.get("Host", "").lower() in {f"127.0.0.1:{port}", f"localhost:{port}"}

    def handle_request(self):
        if not self.valid_host():
            self.reply(403, "无效的本地预览地址")
            return
        try:
            path, is_api, permitted = request_target(self.command, self.path, self.server.isolated_save_check)
        except (ValueError, UnicodeError):
            self.reply(400, "无效的请求路径")
            return
        if is_api:
            if not permitted:
                self.reply(405, "只读预览已阻止此操作")
                return
            origin = self.headers.get("Origin")
            if (origin and not local_origin_allowed(origin, self.server.server_address[1])) or (
                self.command in {"POST", "PUT"} and not origin
            ):
                self.reply(403, "无效的请求来源")
                return
            self.proxy_api()
            return
        if self.command not in {"GET", "HEAD"}:
            self.reply(405, "只读预览已阻止此操作")
            return
        self.serve_static(path)

    def serve_static(self, path: str):
        if path == "/admin":
            self.send_response(302)
            self.send_header("Location", "/admin/")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if path.startswith("/admin/"):
            relative = path[len("/admin/"):]
        elif path.startswith("/media/"):
            relative = path[1:]
        elif path == "/favicon.svg":
            relative = "favicon.svg"
        else:
            self.reply(404, "页面不存在")
            return
        root = self.server.assets
        candidate = (root / relative).resolve()
        if not candidate.is_relative_to(root):
            self.reply(403, "无效的文件路径")
            return
        if candidate.is_dir():
            candidate = candidate / "index.html"
        if not candidate.is_file():
            if Path(relative).suffix:
                self.reply(404, "文件不存在")
                return
            candidate = root / "index.html"
        try:
            size = candidate.stat().st_size
            content_type = mimetypes.guess_type(candidate.name)[0] or "application/octet-stream"
            self.send_response(200)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(size))
            self.send_header("Cache-Control", "no-store")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.end_headers()
            if self.command != "HEAD":
                with candidate.open("rb") as source:
                    while chunk := source.read(65536):
                        self.wfile.write(chunk)
        except OSError:
            self.close_connection = True

    def proxy_api(self):
        upstream = self.server.upstream
        body = None
        headers = {"Host": upstream.netloc, "Accept": "application/json"}
        if cookie := self.headers.get("Cookie"):
            headers["Cookie"] = cookie
        if self.command == "PUT":
            csrf = self.headers.get("X-CSRF-Token", "")
            if not csrf or len(csrf) > 512 or any(ord(char) < 32 or ord(char) == 127 for char in csrf):
                self.reply(400, "缺少有效的安全令牌")
                return
            headers["X-CSRF-Token"] = csrf
        if self.command in {"POST", "PUT"}:
            try:
                length = int(self.headers.get("Content-Length", "-1"))
            except ValueError:
                length = -1
            if (
                length < 1 or length > (4096 if self.command == "POST" else 131072)
                or self.headers.get("Transfer-Encoding")
                or self.headers.get_content_type() != "application/json"
            ):
                self.reply(400, "请求格式无效")
                return
            body = self.rfile.read(length)
            if len(body) != length:
                self.reply(400, "请求不完整")
                return
            headers["Content-Type"] = "application/json"
            headers["Origin"] = f"{upstream.scheme}://{upstream.netloc}"
        connection_type = http.client.HTTPSConnection if upstream.scheme == "https" else http.client.HTTPConnection
        connection = connection_type(upstream.hostname, upstream.port, timeout=20)
        started = False
        try:
            connection.request(self.command, self.path, body=body, headers=headers)
            response = connection.getresponse()
            if 300 <= response.status < 400:
                self.reply(502, "上游返回重定向，预览已阻止跳转")
                return
            self.send_response(response.status)
            for key, value in response.getheaders():
                lower = key.lower()
                if lower in HOP_HEADERS or lower in {"server", "date", "cache-control"}:
                    continue
                self.send_header(key, local_cookie(value) if lower == "set-cookie" else value)
            self.send_header("Cache-Control", "no-store")
            self.send_header("Connection", "close")
            self.end_headers()
            started = True
            if self.command != "HEAD":
                while chunk := response.read(65536):
                    self.wfile.write(chunk)
        except (OSError, http.client.HTTPException):
            if not started:
                self.reply(502, "无法连接上游服务")
        finally:
            self.close_connection = True
            connection.close()

    do_GET = handle_request
    do_HEAD = handle_request
    do_POST = handle_request
    do_PUT = handle_request
    do_PATCH = handle_request
    do_DELETE = handle_request
    do_OPTIONS = handle_request


def main():
    parser = argparse.ArgumentParser(description="Local read-only admin preview")
    parser.add_argument("--port", type=int, default=18778)
    parser.add_argument("--assets", type=Path, default=DEFAULT_ASSETS)
    parser.add_argument("--isolated-save-check", action="store_true", help="Allow two fixed PUTs only against 127.0.0.1:18861")
    args = parser.parse_args()
    if not 1 <= args.port <= 65535:
        parser.error("port must be between 1 and 65535")
    if not (args.assets / "index.html").is_file():
        parser.error("preview assets are missing index.html")
    try:
        origin = upstream_origin(os.environ.get("HYJS_PREVIEW_UPSTREAM", ""))
    except ValueError as exc:
        parser.error(str(exc))
    try:
        server = PreviewServer(("127.0.0.1", args.port), args.assets, origin, args.isolated_save_check)
    except ValueError as exc:
        parser.error(str(exc))
    mode = "Isolated save-check" if args.isolated_save_check else "Read-only"
    print(f"{mode} admin preview: http://127.0.0.1:{args.port}/admin/", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
