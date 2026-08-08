#!/usr/bin/env python3
"""Bounded OpenAI Responses API bridge for Playable Replays NPC actions."""

from __future__ import annotations

import json
import math
import os
import socket
from ipaddress import ip_address
from dataclasses import dataclass
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any, Iterable
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen


SCHEMA_VERSION = "2.0"
STATE_SCOPE = "authoritative_server_state"
MAX_BODY_BYTES = 64 * 1024
MAX_UPSTREAM_REQUEST_BYTES = 128 * 1024
MAX_UNITS = 64
MAX_PROJECTILES = 128
ACTION_TYPES = ("move", "hold", "contest", "retreat")
UNIT_CLASSES = {"tank", "fighter", "marksman", "mage", "support", "assassin"}
UNIT_POLICIES = {"controlled", "support", "protector", "aggressive", "skirmisher"}
DEFAULT_MODEL = "gpt-5.6"
DEFAULT_BASE_URL = "https://api.openai.com/v1"
DEFAULT_TIMEOUT_SECONDS = 8.0
DEFAULT_LISTEN_ADDR = "127.0.0.1:9000"
PROMPT_PATH = Path(__file__).with_name("prompt.txt")


class BridgeError(Exception):
    """A deliberately bounded error that is safe to return to the backend."""

    def __init__(self, status: int, code: str, message: str) -> None:
        super().__init__(code)
        self.status = status
        self.code = code
        self.message = message


@dataclass(frozen=True, slots=True)
class Config:
    api_key: str
    model: str
    base_url: str
    timeout_seconds: float
    prompt: str

    @classmethod
    def from_environment(cls) -> "Config":
        timeout_text = os.getenv("OPENAI_TIMEOUT_SECONDS", str(DEFAULT_TIMEOUT_SECONDS)).strip()
        try:
            timeout_seconds = float(timeout_text)
        except ValueError as error:
            raise ValueError("OPENAI_TIMEOUT_SECONDS must be numeric") from error
        if not math.isfinite(timeout_seconds) or not 0.1 <= timeout_seconds <= 120:
            raise ValueError("OPENAI_TIMEOUT_SECONDS must be between 0.1 and 120")

        model = os.getenv("OPENAI_MODEL", DEFAULT_MODEL).strip()
        if not model:
            raise ValueError("OPENAI_MODEL must not be empty")
        base_url = os.getenv("OPENAI_BASE_URL", DEFAULT_BASE_URL).strip()
        validate_base_url(base_url)
        return cls(
            api_key=os.getenv("OPENAI_API_KEY", "").strip(),
            model=model,
            base_url=base_url.rstrip("/"),
            timeout_seconds=timeout_seconds,
            prompt=PROMPT_PATH.read_text(encoding="utf-8").strip(),
        )

    @property
    def responses_url(self) -> str:
        return f"{self.base_url.rstrip('/')}/responses"


@dataclass(frozen=True, slots=True)
class ValidatedSnapshot:
    raw: dict[str, Any]
    eligible_ids: tuple[str, ...]
    units_by_id: dict[str, dict[str, Any]]
    legal_actions: tuple[str, ...]
    min_x: float
    max_x: float
    min_y: float
    max_y: float


def validate_base_url(value: str) -> None:
    parsed = urlsplit(value)
    if (
        parsed.scheme not in {"http", "https"}
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError("OPENAI_BASE_URL must be an HTTP(S) origin or path without credentials, query, or fragment")
    if parsed.scheme == "http":
        host = parsed.hostname or ""
        try:
            loopback = host == "localhost" or ip_address(host).is_loopback
        except ValueError:
            loopback = False
        if not loopback:
            raise ValueError("OPENAI_BASE_URL may use cleartext HTTP only for a loopback test server")


def strict_json_loads(data: bytes | str) -> Any:
    def reject_constant(value: str) -> None:
        raise ValueError(f"invalid JSON constant {value}")

    def unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON key {key}")
            result[key] = value
        return result

    if isinstance(data, bytes):
        data = data.decode("utf-8")
    return json.loads(data, parse_constant=reject_constant, object_pairs_hook=unique_object)


def is_number(value: Any) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(float(value))


def require_string(owner: dict[str, Any], key: str, *, maximum: int = 256) -> str:
    value = owner.get(key)
    if not isinstance(value, str) or not value.strip() or len(value) > maximum:
        raise ValueError(f"{key} must be a non-empty string")
    return value


def require_number(owner: dict[str, Any], key: str) -> float:
    value = owner.get(key)
    if not is_number(value):
        raise ValueError(f"{key} must be a finite number")
    return float(value)


def require_integer(owner: dict[str, Any], key: str, minimum: int = 0) -> int:
    value = owner.get(key)
    if not isinstance(value, int) or isinstance(value, bool) or value < minimum:
        raise ValueError(f"{key} must be an integer of at least {minimum}")
    return value


def validate_snapshot(value: Any) -> ValidatedSnapshot:
    if not isinstance(value, dict):
        raise ValueError("snapshot must be an object")
    if value.get("schemaVersion") != SCHEMA_VERSION:
        raise ValueError(f"schemaVersion must be {SCHEMA_VERSION}")
    if value.get("stateScope") != STATE_SCOPE:
        raise ValueError(f"stateScope must be {STATE_SCOPE}")
    require_string(value, "sessionId")
    require_string(value, "momentId")
    require_integer(value, "turn", 1)
    controlled_id = require_string(value, "controlledUnitId")

    bounds = value.get("mapBounds")
    if not isinstance(bounds, dict):
        raise ValueError("mapBounds must be an object")
    min_x = require_number(bounds, "minX")
    max_x = require_number(bounds, "maxX")
    min_y = require_number(bounds, "minY")
    max_y = require_number(bounds, "maxY")
    if min_x >= max_x or min_y >= max_y:
        raise ValueError("mapBounds minimums must be below maximums")

    legal_value = value.get("legalActions")
    if not isinstance(legal_value, list) or not legal_value:
        raise ValueError("legalActions must be a non-empty array")
    legal_actions: list[str] = []
    for action in legal_value:
        if not isinstance(action, str) or action not in ACTION_TYPES or action in legal_actions:
            raise ValueError("legalActions must contain unique supported action names")
        legal_actions.append(action)

    objective = value.get("objective")
    if objective is not None and not isinstance(objective, dict):
        raise ValueError("objective must be an object or null")
    projectiles = value.get("projectiles")
    if not isinstance(projectiles, list) or len(projectiles) > MAX_PROJECTILES:
        raise ValueError(f"projectiles must be an array of at most {MAX_PROJECTILES} entries")

    units = value.get("units")
    if not isinstance(units, list) or not 2 <= len(units) <= MAX_UNITS:
        raise ValueError(f"units must contain between 2 and {MAX_UNITS} entries")

    units_by_id: dict[str, dict[str, Any]] = {}
    controlled_alive = False
    for unit in units:
        if not isinstance(unit, dict):
            raise ValueError("each unit must be an object")
        unit_id = require_string(unit, "id")
        if unit_id in units_by_id:
            raise ValueError("unit IDs must be unique")
        if unit.get("team") not in {"blue", "red"}:
            raise ValueError("unit team must be blue or red")
        require_string(unit, "role")
        if unit.get("class") not in UNIT_CLASSES:
            raise ValueError("unit class is unsupported")
        if unit.get("fallbackPolicy") not in UNIT_POLICIES:
            raise ValueError("unit fallbackPolicy is unsupported")
        position = unit.get("position")
        if not isinstance(position, dict):
            raise ValueError("unit position must be an object")
        x = require_number(position, "x")
        y = require_number(position, "y")
        if not min_x <= x <= max_x or not min_y <= y <= max_y:
            raise ValueError("unit position is outside map bounds")
        hp = require_integer(unit, "hp")
        max_hp = require_integer(unit, "maxHp", 1)
        if hp > max_hp:
            raise ValueError("unit hp must not exceed maxHp")
        move_range = require_number(unit, "moveRange")
        attack_range = require_number(unit, "attackRange")
        if move_range < 0 or attack_range < 0:
            raise ValueError("unit ranges must not be negative")
        require_integer(unit, "cooldownTurns")
        require_integer(unit, "shield")
        if (
            not isinstance(unit.get("guarded"), bool)
            or not isinstance(unit.get("visible"), bool)
            or not isinstance(unit.get("alive"), bool)
        ):
            raise ValueError("unit guarded, visible, and alive fields must be booleans")
        units_by_id[unit_id] = unit
        if unit_id == controlled_id:
            controlled_alive = unit["alive"]

    if controlled_id not in units_by_id or not controlled_alive:
        raise ValueError("controlledUnitId must identify a live unit")
    eligible_ids = tuple(
        unit["id"] for unit in units if unit["alive"] and unit["id"] != controlled_id
    )
    if not eligible_ids:
        raise ValueError("snapshot must contain at least one live non-controlled unit")

    return ValidatedSnapshot(
        raw=value,
        eligible_ids=eligible_ids,
        units_by_id=units_by_id,
        legal_actions=tuple(legal_actions),
        min_x=min_x,
        max_x=max_x,
        min_y=min_y,
        max_y=max_y,
    )


def action_output_schema(snapshot: ValidatedSnapshot) -> dict[str, Any]:
    point_schema = {
        "type": "object",
        "properties": {
            "x": {"type": "number", "minimum": snapshot.min_x, "maximum": snapshot.max_x},
            "y": {"type": "number", "minimum": snapshot.min_y, "maximum": snapshot.max_y},
        },
        "required": ["x", "y"],
        "additionalProperties": False,
    }
    return {
        "type": "object",
        "properties": {
            "actions": {
                "type": "array",
                "minItems": len(snapshot.eligible_ids),
                "maxItems": len(snapshot.eligible_ids),
                "items": {
                    "type": "object",
                    "properties": {
                        "unitId": {"type": "string", "enum": list(snapshot.eligible_ids)},
                        "action": {
                            "type": "object",
                            "properties": {
                                "type": {"type": "string", "enum": list(snapshot.legal_actions)},
                                "target": {"anyOf": [point_schema, {"type": "null"}]},
                            },
                            "required": ["type", "target"],
                            "additionalProperties": False,
                        },
                    },
                    "required": ["unitId", "action"],
                    "additionalProperties": False,
                },
            }
        },
        "required": ["actions"],
        "additionalProperties": False,
    }


def has_refusal(value: Any) -> bool:
    if isinstance(value, dict):
        if value.get("type") == "refusal" or isinstance(value.get("refusal"), str):
            return True
        return any(has_refusal(item) for item in value.values())
    if isinstance(value, list):
        return any(has_refusal(item) for item in value)
    return False


def extract_output_text(response: Any) -> str:
    if not isinstance(response, dict):
        raise ValueError("upstream response must be an object")
    if has_refusal(response):
        raise BridgeError(HTTPStatus.BAD_GATEWAY, "model_refusal", "The model refused the action request.")
    if response.get("status") != "completed":
        raise BridgeError(HTTPStatus.BAD_GATEWAY, "model_incomplete", "The model response was incomplete.")
    output = response.get("output")
    if not isinstance(output, list):
        raise ValueError("upstream output must be an array")
    text_parts: list[str] = []
    for item in output:
        if not isinstance(item, dict) or item.get("type") != "message":
            continue
        if item.get("status", "completed") != "completed":
            raise BridgeError(HTTPStatus.BAD_GATEWAY, "model_incomplete", "The model response was incomplete.")
        content = item.get("content")
        if not isinstance(content, list):
            raise ValueError("message content must be an array")
        for part in content:
            if isinstance(part, dict) and part.get("type") == "output_text" and isinstance(part.get("text"), str):
                text_parts.append(part["text"])
    if not text_parts:
        raise ValueError("upstream response contained no output text")
    return "".join(text_parts)


def exact_keys(value: dict[str, Any], allowed: Iterable[str]) -> bool:
    return set(value) == set(allowed)


def validate_actions(value: Any, snapshot: ValidatedSnapshot) -> dict[str, Any]:
    if not isinstance(value, dict) or not exact_keys(value, ("actions",)):
        raise ValueError("model output must contain only actions")
    actions = value.get("actions")
    if not isinstance(actions, list) or len(actions) != len(snapshot.eligible_ids):
        raise ValueError("model output must contain exactly one action per eligible unit")

    accepted: dict[str, dict[str, Any]] = {}
    for suggestion in actions:
        if not isinstance(suggestion, dict) or not exact_keys(suggestion, ("unitId", "action")):
            raise ValueError("each action suggestion must contain unitId and action")
        unit_id = suggestion.get("unitId")
        if not isinstance(unit_id, str) or unit_id not in snapshot.eligible_ids or unit_id in accepted:
            raise ValueError("action unitId must be unique and eligible")
        action = suggestion.get("action")
        if not isinstance(action, dict) or set(action) - {"type", "target"} or "type" not in action:
            raise ValueError("action has an invalid shape")
        action_type = action.get("type")
        if action_type not in snapshot.legal_actions:
            raise ValueError("action type is not legal for this snapshot")

        target = action.get("target")
        normalized: dict[str, Any] = {"type": action_type}
        if action_type == "move":
            if not isinstance(target, dict) or not exact_keys(target, ("x", "y")):
                raise ValueError("move requires exactly one x/y target")
            x = target.get("x")
            y = target.get("y")
            if not is_number(x) or not is_number(y):
                raise ValueError("move target coordinates must be finite numbers")
            x_float, y_float = float(x), float(y)
            if not snapshot.min_x <= x_float <= snapshot.max_x or not snapshot.min_y <= y_float <= snapshot.max_y:
                raise ValueError("move target is outside map bounds")
            unit = snapshot.units_by_id[unit_id]
            origin = unit["position"]
            distance = math.hypot(x_float - float(origin["x"]), y_float - float(origin["y"]))
            if distance > float(unit["moveRange"]) + 1e-6:
                raise ValueError("move target exceeds the unit movement limit")
            normalized["target"] = {"x": x, "y": y}
        elif target is not None:
            raise ValueError("only move may include a target")
        accepted[unit_id] = normalized

    if set(accepted) != set(snapshot.eligible_ids):
        raise ValueError("model output omitted an eligible unit")
    return {
        "actions": [
            {"unitId": unit_id, "action": accepted[unit_id]}
            for unit_id in snapshot.eligible_ids
        ]
    }


class ActionBridge:
    def __init__(self, config: Config) -> None:
        self.config = config

    def next_actions(self, raw_snapshot: Any) -> dict[str, Any]:
        if not self.config.api_key:
            raise BridgeError(
                HTTPStatus.SERVICE_UNAVAILABLE,
                "model_unavailable",
                "The OpenAI API key is not configured.",
            )
        try:
            snapshot = validate_snapshot(raw_snapshot)
        except ValueError as error:
            raise BridgeError(HTTPStatus.BAD_REQUEST, "invalid_snapshot", "The schema 2.0 snapshot is invalid.") from error

        payload = {
            "model": self.config.model,
            "instructions": self.config.prompt,
            "input": [
                {
                    "role": "user",
                    "content": [
                        {
                            "type": "input_text",
                            "text": "Authoritative simulator snapshot JSON:\n"
                            + json.dumps(snapshot.raw, separators=(",", ":"), sort_keys=True, ensure_ascii=True),
                        }
                    ],
                }
            ],
            "text": {
                "format": {
                    "type": "json_schema",
                    "name": "npc_actions_v2",
                    "description": "Exactly one bounded tactical action for every live non-controlled unit.",
                    "strict": True,
                    "schema": action_output_schema(snapshot),
                }
            },
            "max_output_tokens": 4096,
            "store": False,
        }
        request_body = json.dumps(payload, separators=(",", ":"), ensure_ascii=True).encode("utf-8")
        if len(request_body) > MAX_UPSTREAM_REQUEST_BYTES:
            raise BridgeError(
                HTTPStatus.REQUEST_ENTITY_TOO_LARGE,
                "model_request_too_large",
                "The expanded model request exceeds 128 KiB.",
            )
        request = Request(
            self.config.responses_url,
            data=request_body,
            method="POST",
            headers={
                "Authorization": f"Bearer {self.config.api_key}",
                "Content-Type": "application/json",
                "Accept": "application/json",
                "User-Agent": "playable-replays-model-daemon/2.0",
            },
        )
        try:
            with urlopen(request, timeout=self.config.timeout_seconds) as response:
                upstream_body = response.read(MAX_BODY_BYTES + 1)
        except HTTPError as error:
            error.close()
            raise BridgeError(HTTPStatus.BAD_GATEWAY, "upstream_error", "The OpenAI API rejected the request.") from error
        except (TimeoutError, socket.timeout) as error:
            raise BridgeError(HTTPStatus.GATEWAY_TIMEOUT, "upstream_timeout", "The OpenAI API request timed out.") from error
        except URLError as error:
            if isinstance(error.reason, (TimeoutError, socket.timeout)):
                raise BridgeError(HTTPStatus.GATEWAY_TIMEOUT, "upstream_timeout", "The OpenAI API request timed out.") from error
            raise BridgeError(HTTPStatus.BAD_GATEWAY, "upstream_error", "The OpenAI API request failed.") from error
        if len(upstream_body) > MAX_BODY_BYTES:
            raise BridgeError(HTTPStatus.BAD_GATEWAY, "upstream_too_large", "The OpenAI API response was too large.")

        try:
            upstream_response = strict_json_loads(upstream_body)
            output_text = extract_output_text(upstream_response)
            output_value = strict_json_loads(output_text)
            return validate_actions(output_value, snapshot)
        except BridgeError:
            raise
        except (UnicodeDecodeError, ValueError, json.JSONDecodeError) as error:
            raise BridgeError(HTTPStatus.BAD_GATEWAY, "invalid_model_output", "The model returned invalid actions.") from error


class BridgeHTTPServer(ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True

    def __init__(self, address: tuple[str, int], bridge: ActionBridge) -> None:
        self.bridge = bridge
        super().__init__(address, BridgeRequestHandler)

    def handle_error(self, _request: Any, _client_address: Any) -> None:
        # Avoid traceback logging at this privileged server-to-server boundary.
        return


class BridgeRequestHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server_version = "PlayableReplaysModelBridge/2.0"
    sys_version = ""

    @property
    def bridge(self) -> ActionBridge:
        return self.server.bridge  # type: ignore[attr-defined, no-any-return]

    def log_message(self, _format: str, *args: Any) -> None:
        # Requests and privileged snapshots are intentionally not logged.
        return

    def _send_json(self, status: int, value: dict[str, Any], *, allow: str | None = None) -> None:
        body = json.dumps(value, separators=(",", ":"), ensure_ascii=True).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("Referrer-Policy", "no-referrer")
        self.send_header("X-Content-Type-Options", "nosniff")
        if allow is not None:
            self.send_header("Allow", allow)
        self.end_headers()
        self.wfile.write(body)

    def _error(self, error: BridgeError) -> None:
        self._send_json(error.status, {"error": {"code": error.code, "message": error.message}})

    def _path(self) -> str:
        parsed = urlsplit(self.path)
        if parsed.query or parsed.fragment:
            return ""
        return parsed.path

    def do_GET(self) -> None:
        if self._path() == "/healthz":
            self._send_json(HTTPStatus.OK, {"status": "ok"})
            return
        if self._path() == "/v1/actions":
            self._send_json(
                HTTPStatus.METHOD_NOT_ALLOWED,
                {"error": {"code": "method_not_allowed", "message": "Use POST for this endpoint."}},
                allow="POST",
            )
            return
        self._send_json(HTTPStatus.NOT_FOUND, {"error": {"code": "not_found", "message": "Endpoint not found."}})

    def do_POST(self) -> None:
        if self._path() != "/v1/actions":
            self.close_connection = True
            self._send_json(HTTPStatus.NOT_FOUND, {"error": {"code": "not_found", "message": "Endpoint not found."}})
            return
        if self.headers.get("Transfer-Encoding"):
            self.close_connection = True
            self._error(BridgeError(HTTPStatus.BAD_REQUEST, "invalid_request", "Transfer encoding is not supported."))
            return
        content_type = self.headers.get_content_type()
        if content_type != "application/json":
            self.close_connection = True
            self._error(BridgeError(HTTPStatus.UNSUPPORTED_MEDIA_TYPE, "invalid_content_type", "Content-Type must be application/json."))
            return
        content_length = self.headers.get("Content-Length")
        if content_length is None:
            self.close_connection = True
            self._error(BridgeError(HTTPStatus.LENGTH_REQUIRED, "length_required", "Content-Length is required."))
            return
        try:
            length = int(content_length)
        except ValueError:
            length = -1
        if length < 1:
            self.close_connection = True
            self._error(BridgeError(HTTPStatus.BAD_REQUEST, "invalid_request", "Request body length is invalid."))
            return
        if length > MAX_BODY_BYTES:
            self.close_connection = True
            self._error(BridgeError(HTTPStatus.REQUEST_ENTITY_TOO_LARGE, "request_too_large", "Request body exceeds 64 KiB."))
            return
        body = self.rfile.read(length)
        if len(body) != length:
            self.close_connection = True
            self._error(BridgeError(HTTPStatus.BAD_REQUEST, "invalid_request", "Request body was incomplete."))
            return
        try:
            snapshot = strict_json_loads(body)
        except (UnicodeDecodeError, ValueError, json.JSONDecodeError):
            self._error(BridgeError(HTTPStatus.BAD_REQUEST, "invalid_json", "Request body must contain one valid JSON object."))
            return
        try:
            result = self.bridge.next_actions(snapshot)
        except BridgeError as error:
            self._error(error)
            return
        except Exception:
            self._error(BridgeError(HTTPStatus.INTERNAL_SERVER_ERROR, "internal_error", "The model bridge failed."))
            return
        self._send_json(HTTPStatus.OK, result)

    def _method_not_allowed(self) -> None:
        allow = "GET" if self._path() == "/healthz" else "POST" if self._path() == "/v1/actions" else None
        if allow is None:
            self._send_json(HTTPStatus.NOT_FOUND, {"error": {"code": "not_found", "message": "Endpoint not found."}})
            return
        self._send_json(
            HTTPStatus.METHOD_NOT_ALLOWED,
            {"error": {"code": "method_not_allowed", "message": f"Use {allow} for this endpoint."}},
            allow=allow,
        )

    do_DELETE = _method_not_allowed
    do_PATCH = _method_not_allowed
    do_PUT = _method_not_allowed


def parse_listen_addr(value: str) -> tuple[str, int]:
    try:
        host, port_text = value.rsplit(":", 1)
        port = int(port_text)
    except (ValueError, AttributeError) as error:
        raise ValueError("LISTEN_ADDR must use host:port form") from error
    host = host.strip()
    if host.startswith("[") and host.endswith("]"):
        host = host[1:-1]
    if not host or not 1 <= port <= 65535:
        raise ValueError("LISTEN_ADDR must contain a host and valid port")
    return host, port


def create_server(address: tuple[str, int], bridge: ActionBridge) -> BridgeHTTPServer:
    return BridgeHTTPServer(address, bridge)


def main() -> int:
    try:
        config = Config.from_environment()
        address = parse_listen_addr(os.getenv("LISTEN_ADDR", DEFAULT_LISTEN_ADDR).strip())
        server = create_server(address, ActionBridge(config))
    except (OSError, ValueError) as error:
        print(f"model daemon configuration error: {error}", file=os.sys.stderr)
        return 2
    try:
        server.serve_forever(poll_interval=0.25)
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
