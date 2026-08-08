# OpenAI NPC action bridge

This optional Python 3.12 service translates one privileged Playable Replays
schema `2.0` snapshot into advisory actions for every live non-controlled unit.
It makes a real OpenAI Responses API request with strict Structured Outputs;
there is no local or simulated success path. The Go backend validates accepted
actions and owns deterministic fallback whenever this service returns non-`200`.

The bridge follows the official OpenAI
[Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs)
and uses `text.format` with a strict JSON Schema.

## Run locally

```bash
export OPENAI_API_KEY='set-this-outside-the-repository'
python3 model-daemon/server.py
```

Configure the Go server's model endpoint separately from the OpenAI credential:

```bash
cd backend
BOT_MODEL_URL=http://127.0.0.1:9000/v1/actions \
BOT_MODEL_NAME=openai-npc-actions \
BOT_MODEL_VERSION=gpt-5.6 \
go run ./cmd/server
```

Never put the API key in a browser request, fixture, source file, image, or
committed `.env` file. Keep the daemon's upstream timeout below the Go model
client deadline so a failure reaches the backend in time for fallback.

| Variable | Default | Purpose |
| --- | --- | --- |
| `OPENAI_API_KEY` | none | Required for `POST /v1/actions`; a missing key returns `503`. |
| `OPENAI_MODEL` | `gpt-5.6` | Responses API model ID. |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Operator-owned OpenAI-compatible base path; HTTPS is required except for a loopback test server. |
| `OPENAI_TIMEOUT_SECONDS` | `8` | Upstream timeout, below the Go connector deadline and bounded to `0.1..120` seconds. |
| `LISTEN_ADDR` | `127.0.0.1:9000` | HTTP bind address. Docker sets `0.0.0.0:9000`. |

`GET /healthz` is a liveness check and returns `200` even when no key is
configured. `POST /v1/actions` accepts at most 64 KiB and requires
`application/json`. Its schema `2.0` snapshot includes authoritative units,
map bounds, legal actions, optional objective state, and projectiles. Expanded
OpenAI requests are capped at 128 KiB and OpenAI responses at 64 KiB.

A successful response is deliberately small:

```json
{
  "actions": [
    {"unitId": "blue-support", "action": {"type": "hold"}},
    {"unitId": "red-marksman", "action": {"type": "move", "target": {"x": 54, "y": 43}}}
  ]
}
```

The service returns a non-`200` response for a missing key, invalid snapshot,
upstream HTTP or network failure, timeout, refusal, incomplete response,
oversized response, malformed JSON, duplicate/unknown/omitted unit, illegal
action, or invalid movement target. It never retries or silently substitutes an
answer, allowing the Go backend to apply its deterministic fallback exactly
once. Request snapshots, model output, and the API key are never logged.

## Test

The tests use a local fake Responses API; they never need a key or network:

```bash
python3 -m unittest discover -s model-daemon/tests -v
```

## Container

```bash
docker build -t playable-replays-model ./model-daemon
docker run --rm -p 127.0.0.1:9000:9000 \
  -e OPENAI_API_KEY \
  -e OPENAI_MODEL=gpt-5.6 \
  playable-replays-model
```

The image runs as an unprivileged user and contains only the standard-library
service and its prompt.
