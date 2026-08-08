from __future__ import annotations

import json
import sys
import threading
import time
import unittest
from contextlib import contextmanager
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any, Callable, Iterator
from urllib.error import HTTPError
from urllib.request import Request, urlopen


sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import server as daemon  # noqa: E402


Responder = Callable[[dict[str, Any]], tuple[int, dict[str, Any] | bytes]]


class FakeResponsesServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, responder: Responder) -> None:
        self.responder = responder
        self.requests: list[dict[str, Any]] = []
        super().__init__(("127.0.0.1", 0), FakeResponsesHandler)


class FakeResponsesHandler(BaseHTTPRequestHandler):
    def log_message(self, _format: str, *args: Any) -> None:
        return

    def do_POST(self) -> None:
        length = int(self.headers["Content-Length"])
        payload = json.loads(self.rfile.read(length))
        self.server.requests.append(  # type: ignore[attr-defined]
            {"path": self.path, "headers": dict(self.headers), "payload": payload}
        )
        status, value = self.server.responder(payload)  # type: ignore[attr-defined]
        body = value if isinstance(value, bytes) else json.dumps(value).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        try:
            self.wfile.write(body)
        except BrokenPipeError:
            pass


@contextmanager
def running(server: ThreadingHTTPServer) -> Iterator[ThreadingHTTPServer]:
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield server
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


def snapshot() -> dict[str, Any]:
    return {
        "schemaVersion": "2.0",
        "stateScope": "authoritative_server_state",
        "sessionId": "session-1",
        "momentId": "moment-1",
        "turn": 1,
        "mapBounds": {"minX": 0, "maxX": 100, "minY": 0, "maxY": 100},
        "controlledUnitId": "blue-carry",
        "legalActions": ["move", "hold", "contest", "retreat"],
        "objective": None,
        "projectiles": [],
        "units": [
            {
                "id": "blue-carry",
                "team": "blue",
                "role": "carry",
                "class": "marksman",
                "fallbackPolicy": "controlled",
                "position": {"x": 30, "y": 50},
                "hp": 80,
                "maxHp": 90,
                "moveRange": 11,
                "attackRange": 28,
                "cooldownTurns": 0,
                "shield": 4,
                "guarded": True,
                "visible": True,
                "alive": True,
            },
            {
                "id": "blue-support",
                "team": "blue",
                "role": "support",
                "class": "support",
                "fallbackPolicy": "support",
                "position": {"x": 38, "y": 50},
                "hp": 100,
                "maxHp": 110,
                "moveRange": 8,
                "attackRange": 20,
                "cooldownTurns": 0,
                "shield": 0,
                "guarded": False,
                "visible": True,
                "alive": True,
            },
            {
                "id": "red-fighter",
                "team": "red",
                "role": "jungle",
                "class": "fighter",
                "fallbackPolicy": "aggressive",
                "position": {"x": 60, "y": 50},
                "hp": 110,
                "maxHp": 125,
                "moveRange": 10,
                "attackRange": 14,
                "cooldownTurns": 0,
                "shield": 0,
                "guarded": False,
                "visible": True,
                "alive": True,
            },
        ],
    }


def completed_response(output: dict[str, Any]) -> dict[str, Any]:
    return {
        "id": "resp-test",
        "status": "completed",
        "output": [
            {
                "type": "message",
                "status": "completed",
                "role": "assistant",
                "content": [{"type": "output_text", "text": json.dumps(output), "annotations": []}],
            }
        ],
    }


def valid_model_output() -> dict[str, Any]:
    return {
        "actions": [
            {"unitId": "red-fighter", "action": {"type": "move", "target": {"x": 52, "y": 50}}},
            {"unitId": "blue-support", "action": {"type": "hold", "target": None}},
        ]
    }


def request_json(url: str, value: Any) -> tuple[int, dict[str, Any]]:
    body = json.dumps(value).encode()
    request = Request(url, data=body, method="POST", headers={"Content-Type": "application/json"})
    try:
        with urlopen(request, timeout=2) as response:
            return response.status, json.loads(response.read())
    except HTTPError as error:
        with error:
            return error.code, json.loads(error.read())


class ActionBridgeTests(unittest.TestCase):
    def bridge(self, upstream: FakeResponsesServer, *, key: str = "test-key", timeout: float = 1) -> daemon.ActionBridge:
        host, port = upstream.server_address
        return daemon.ActionBridge(
            daemon.Config(
                api_key=key,
                model="gpt-5.6",
                base_url=f"http://{host}:{port}/v1",
                timeout_seconds=timeout,
                prompt="Return NPC actions.",
            )
        )

    def test_real_upstream_request_uses_structured_outputs_and_normalizes_order(self) -> None:
        upstream = FakeResponsesServer(lambda _payload: (200, completed_response(valid_model_output())))
        with running(upstream):
            result = self.bridge(upstream).next_actions(snapshot())

        self.assertEqual(
            result,
            {
                "actions": [
                    {"unitId": "blue-support", "action": {"type": "hold"}},
                    {"unitId": "red-fighter", "action": {"type": "move", "target": {"x": 52, "y": 50}}},
                ]
            },
        )
        self.assertEqual(len(upstream.requests), 1)
        captured = upstream.requests[0]
        self.assertEqual(captured["path"], "/v1/responses")
        self.assertEqual(captured["headers"]["Authorization"], "Bearer test-key")
        payload = captured["payload"]
        self.assertEqual(payload["model"], "gpt-5.6")
        self.assertFalse(payload["store"])
        self.assertEqual(payload["text"]["format"]["type"], "json_schema")
        self.assertTrue(payload["text"]["format"]["strict"])
        self.assertEqual(payload["text"]["format"]["schema"]["properties"]["actions"]["minItems"], 2)

    def test_missing_key_never_calls_upstream(self) -> None:
        with FakeResponsesServer(lambda _payload: self.fail("upstream must not be called")) as upstream:
            with self.assertRaises(daemon.BridgeError) as caught:
                self.bridge(upstream, key="").next_actions(snapshot())
            self.assertEqual(caught.exception.status, HTTPStatus.SERVICE_UNAVAILABLE)
            self.assertEqual(upstream.requests, [])

    def test_refusal_is_a_bad_gateway(self) -> None:
        refusal = {
            "status": "completed",
            "output": [{"type": "message", "content": [{"type": "refusal", "refusal": "no"}]}],
        }
        upstream = FakeResponsesServer(lambda _payload: (200, refusal))
        with running(upstream), self.assertRaises(daemon.BridgeError) as caught:
            self.bridge(upstream).next_actions(snapshot())
        self.assertEqual(caught.exception.code, "model_refusal")

    def test_incomplete_response_is_a_bad_gateway(self) -> None:
        upstream = FakeResponsesServer(
            lambda _payload: (200, {"status": "incomplete", "incomplete_details": {"reason": "max_output_tokens"}, "output": []})
        )
        with running(upstream), self.assertRaises(daemon.BridgeError) as caught:
            self.bridge(upstream).next_actions(snapshot())
        self.assertEqual(caught.exception.code, "model_incomplete")

    def test_missing_unit_action_is_rejected(self) -> None:
        incomplete = {"actions": [{"unitId": "blue-support", "action": {"type": "hold", "target": None}}]}
        upstream = FakeResponsesServer(lambda _payload: (200, completed_response(incomplete)))
        with running(upstream), self.assertRaises(daemon.BridgeError) as caught:
            self.bridge(upstream).next_actions(snapshot())
        self.assertEqual(caught.exception.code, "invalid_model_output")

    def test_move_beyond_unit_range_is_rejected(self) -> None:
        invalid = valid_model_output()
        invalid["actions"][0]["action"]["target"] = {"x": 20, "y": 20}
        upstream = FakeResponsesServer(lambda _payload: (200, completed_response(invalid)))
        with running(upstream), self.assertRaises(daemon.BridgeError) as caught:
            self.bridge(upstream).next_actions(snapshot())
        self.assertEqual(caught.exception.code, "invalid_model_output")

    def test_malformed_upstream_json_is_rejected(self) -> None:
        upstream = FakeResponsesServer(lambda _payload: (200, b"{not-json"))
        with running(upstream), self.assertRaises(daemon.BridgeError) as caught:
            self.bridge(upstream).next_actions(snapshot())
        self.assertEqual(caught.exception.code, "invalid_model_output")

    def test_upstream_http_error_is_not_a_success(self) -> None:
        upstream = FakeResponsesServer(lambda _payload: (500, {"error": {"message": "failed"}}))
        with running(upstream), self.assertRaises(daemon.BridgeError) as caught:
            self.bridge(upstream).next_actions(snapshot())
        self.assertEqual(caught.exception.code, "upstream_error")

    def test_timeout_is_reported_without_fallback_output(self) -> None:
        def delayed(_payload: dict[str, Any]) -> tuple[int, dict[str, Any]]:
            time.sleep(0.15)
            return 200, completed_response(valid_model_output())

        upstream = FakeResponsesServer(delayed)
        with running(upstream), self.assertRaises(daemon.BridgeError) as caught:
            self.bridge(upstream, timeout=0.03).next_actions(snapshot())
        self.assertEqual(caught.exception.status, HTTPStatus.GATEWAY_TIMEOUT)


class ConfigTests(unittest.TestCase):
    def test_base_url_rejects_cleartext_remote_key_destination(self) -> None:
        with self.assertRaisesRegex(ValueError, "cleartext HTTP"):
            daemon.validate_base_url("http://model.example/v1")

    def test_base_url_allows_https_and_loopback_test_servers(self) -> None:
        daemon.validate_base_url("https://api.openai.com/v1")
        daemon.validate_base_url("http://127.0.0.1:9001/v1")
        daemon.validate_base_url("http://[::1]:9001/v1")


class HTTPHandlerTests(unittest.TestCase):
    def test_post_success_runs_the_complete_http_bridge(self) -> None:
        upstream = FakeResponsesServer(lambda _payload: (200, completed_response(valid_model_output())))
        with running(upstream):
            app = daemon.create_server(("127.0.0.1", 0), ActionBridgeTests().bridge(upstream))
            with running(app):
                host, port = app.server_address
                status, value = request_json(f"http://{host}:{port}/v1/actions", snapshot())
        self.assertEqual(status, HTTPStatus.OK)
        self.assertEqual([item["unitId"] for item in value["actions"]], ["blue-support", "red-fighter"])
        self.assertEqual(len(upstream.requests), 1)

    def test_health_is_live_without_an_api_key_and_post_returns_non_200(self) -> None:
        with FakeResponsesServer(lambda _payload: (500, {})) as upstream:
            bridge = ActionBridgeTests().bridge(upstream, key="")
            app = daemon.create_server(("127.0.0.1", 0), bridge)
            with running(app):
                host, port = app.server_address
                with urlopen(f"http://{host}:{port}/healthz", timeout=2) as response:
                    self.assertEqual(response.status, 200)
                    self.assertEqual(json.loads(response.read()), {"status": "ok"})
                status, value = request_json(f"http://{host}:{port}/v1/actions", snapshot())
        self.assertEqual(status, HTTPStatus.SERVICE_UNAVAILABLE)
        self.assertEqual(value["error"]["code"], "model_unavailable")

    def test_request_body_is_capped(self) -> None:
        with FakeResponsesServer(lambda _payload: (500, {})) as upstream:
            app = daemon.create_server(("127.0.0.1", 0), ActionBridgeTests().bridge(upstream))
            with running(app):
                host, port = app.server_address
                body = b'"' + (b"x" * daemon.MAX_BODY_BYTES) + b'"'
                request = Request(
                    f"http://{host}:{port}/v1/actions",
                    data=body,
                    method="POST",
                    headers={"Content-Type": "application/json"},
                )
                with self.assertRaises(HTTPError) as caught:
                    urlopen(request, timeout=2)
                self.assertEqual(caught.exception.code, HTTPStatus.REQUEST_ENTITY_TOO_LARGE)
                caught.exception.close()


if __name__ == "__main__":
    unittest.main()
