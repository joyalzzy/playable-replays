#!/usr/bin/env python3
"""Serve the exported policy through the Playable Replays /v1/actions contract."""
from __future__ import annotations

import argparse
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

from infer import MODEL_PATH, infer_snapshot, load_model

MAX_BODY_BYTES = 64 * 1024


class PolicyHandler(BaseHTTPRequestHandler):
    server_version = "PlayableReplaysUnitPolicy/1.0"
    model: dict[str, Any]

    def _json(self, status: int, value: Any) -> None:
        body = json.dumps(value, separators=(",", ":"), ensure_ascii=True).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("X-Content-Type-Options", "nosniff")
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/healthz":
            self._json(200, {"status": "ok", "modelVersion": self.model["policyVersion"]})
            return
        self._json(404, {"error": {"code": "not_found", "message": "route not found"}})

    def do_POST(self) -> None:  # noqa: N802
        if self.path != "/v1/actions":
            self._json(404, {"error": {"code": "not_found", "message": "route not found"}})
            return
        content_type = self.headers.get_content_type()
        if content_type != "application/json":
            self._json(415, {"error": {"code": "unsupported_media_type", "message": "Content-Type must be application/json"}})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            length = -1
        if length < 1 or length > MAX_BODY_BYTES:
            self._json(413, {"error": {"code": "body_too_large", "message": "request body must be between 1 byte and 64 KiB"}})
            return
        try:
            raw = json.loads(self.rfile.read(length))
            result = infer_snapshot(self.model, raw)
        except (ValueError, KeyError, TypeError, json.JSONDecodeError) as exc:
            self._json(400, {"error": {"code": "invalid_snapshot", "message": str(exc)}})
            return
        self._json(200, {"actions": result["actions"]})

    def log_message(self, fmt: str, *args: Any) -> None:
        # Deliberately avoid logging request bodies or model outputs.
        super().log_message(fmt, *args)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--listen", default="127.0.0.1:9000", help="host:port; default 127.0.0.1:9000")
    parser.add_argument("--model", type=Path, default=MODEL_PATH)
    args = parser.parse_args()
    host, separator, port_text = args.listen.rpartition(":")
    if not separator or not host:
        parser.error("--listen must be host:port")
    try:
        port = int(port_text)
    except ValueError:
        parser.error("port must be an integer")
    PolicyHandler.model = load_model(args.model)
    server = ThreadingHTTPServer((host, port), PolicyHandler)
    print(f"serving {PolicyHandler.model['policyVersion']} on http://{host}:{port}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
