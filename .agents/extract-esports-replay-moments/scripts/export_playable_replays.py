#!/usr/bin/env python3
"""Build and validate a Playable Replays v3.0 scenario pack."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any, NoReturn


DEFAULT_SCHEMA = Path(__file__).resolve().parent.parent / "references" / "playable-replays-moment.schema.json"
CLASS_PROFILES = {
    "tank": {"maxHp": 160, "moveRange": 7, "attackRange": 10},
    "fighter": {"maxHp": 125, "moveRange": 10, "attackRange": 14},
    "marksman": {"maxHp": 90, "moveRange": 11, "attackRange": 28},
    "mage": {"maxHp": 95, "moveRange": 9, "attackRange": 24},
    "support": {"maxHp": 110, "moveRange": 8, "attackRange": 20},
    "assassin": {"maxHp": 100, "moveRange": 13, "attackRange": 12},
}
ACTION_TYPES = ("move", "hold", "contest", "retreat")
CANONICAL_TERRAIN = {
    "base-gate": {"x": 16, "y": 82},
    "tower-zone": {"x": 30, "y": 69},
    "lane-pocket": {"x": 20, "y": 78},
    "exit-pocket": {"x": 11, "y": 63},
    "exit-zone": {"x": 12, "y": 78},
    "river": {"x": 50, "y": 52},
}
SCENARIO_SPECIFIC_MECHANICS = {"baron-pit"}
MOMENT_KEY_ORDER = (
    "id",
    "slug",
    "title",
    "description",
    "map",
    "startTimeSeconds",
    "seed",
    "maxTurns",
    "controlledUnitId",
    "reasonTags",
    "signals",
    "replayEvidence",
    "mechanicBriefing",
    "units",
    "rules",
    "authoring",
)


class ValidationError(ValueError):
    """Raised when an export does not satisfy the project contract."""


def fail(path: str, message: str) -> NoReturn:
    raise ValidationError(f"{path}: {message}")


def reject_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ValidationError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def reject_constant(value: str) -> NoReturn:
    raise ValidationError(f"non-finite JSON number: {value}")


def load_json(path: Path) -> Any:
    try:
        return json.loads(
            path.read_text(encoding="utf-8"),
            object_pairs_hook=reject_duplicate_keys,
            parse_constant=reject_constant,
        )
    except (OSError, json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise ValidationError(f"cannot read JSON {path}: {exc}") from exc


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def json_equal(left: Any, right: Any) -> bool:
    if isinstance(left, bool) or isinstance(right, bool):
        return type(left) is type(right) and left == right
    return left == right


class SchemaValidator:
    """Small validator for the keyword subset used by the canonical schema."""

    def __init__(self, root: dict[str, Any]) -> None:
        self.root = root

    def resolve(self, reference: str) -> dict[str, Any]:
        if not reference.startswith("#/"):
            raise ValidationError(f"unsupported non-local schema reference: {reference}")
        value: Any = self.root
        for raw_part in reference[2:].split("/"):
            part = raw_part.replace("~1", "/").replace("~0", "~")
            if not isinstance(value, dict) or part not in value:
                raise ValidationError(f"unresolved schema reference: {reference}")
            value = value[part]
        if not isinstance(value, dict):
            raise ValidationError(f"schema reference is not an object: {reference}")
        return value

    def validate(self, value: Any, schema: dict[str, Any] | None = None, path: str = "$") -> None:
        schema = self.root if schema is None else schema
        if "$ref" in schema:
            self.validate(value, self.resolve(schema["$ref"]), path)
            return

        if "oneOf" in schema:
            successes = 0
            messages: list[str] = []
            for option in schema["oneOf"]:
                try:
                    self.validate(value, option, path)
                except ValidationError as exc:
                    messages.append(str(exc))
                else:
                    successes += 1
            if successes != 1:
                fail(path, f"must match exactly one schema branch; matched {successes}; {messages[:2]}")
            return

        expected_type = schema.get("type")
        if expected_type is not None and not self._matches_type(value, expected_type):
            fail(path, f"expected {expected_type}, got {type(value).__name__}")
        if "const" in schema and not json_equal(value, schema["const"]):
            fail(path, f"expected constant {schema['const']!r}")
        if "enum" in schema and not any(json_equal(value, item) for item in schema["enum"]):
            fail(path, f"value {value!r} is outside the allowed enum")

        if isinstance(value, str):
            if len(value) < schema.get("minLength", 0):
                fail(path, f"string is shorter than {schema['minLength']}")
            pattern = schema.get("pattern")
            if pattern is not None and re.search(pattern, value) is None:
                fail(path, f"string does not match {pattern!r}")

        if isinstance(value, (int, float)) and not isinstance(value, bool):
            if not math.isfinite(value):
                fail(path, "number must be finite")
            if "minimum" in schema and value < schema["minimum"]:
                fail(path, f"number is below minimum {schema['minimum']}")
            if "maximum" in schema and value > schema["maximum"]:
                fail(path, f"number is above maximum {schema['maximum']}")
            if "exclusiveMinimum" in schema and value <= schema["exclusiveMinimum"]:
                fail(path, f"number must be greater than {schema['exclusiveMinimum']}")
            if "exclusiveMaximum" in schema and value >= schema["exclusiveMaximum"]:
                fail(path, f"number must be less than {schema['exclusiveMaximum']}")

        if isinstance(value, list):
            if len(value) < schema.get("minItems", 0):
                fail(path, f"array has fewer than {schema['minItems']} items")
            if "maxItems" in schema and len(value) > schema["maxItems"]:
                fail(path, f"array has more than {schema['maxItems']} items")
            if schema.get("uniqueItems"):
                encoded = [json.dumps(item, sort_keys=True, separators=(",", ":")) for item in value]
                if len(encoded) != len(set(encoded)):
                    fail(path, "array items must be unique")
            item_schema = schema.get("items")
            if isinstance(item_schema, dict):
                for index, item in enumerate(value):
                    self.validate(item, item_schema, f"{path}[{index}]")

        if isinstance(value, dict):
            properties = schema.get("properties", {})
            required = schema.get("required", [])
            missing = [key for key in required if key not in value]
            if missing:
                fail(path, f"missing required keys {missing}")
            for key, item in value.items():
                if key in properties:
                    self.validate(item, properties[key], f"{path}.{key}")
                elif schema.get("additionalProperties") is False:
                    fail(path, f"unknown key {key!r}")
                elif isinstance(schema.get("additionalProperties"), dict):
                    self.validate(item, schema["additionalProperties"], f"{path}.{key}")

    @staticmethod
    def _matches_type(value: Any, expected: str) -> bool:
        return {
            "object": isinstance(value, dict),
            "array": isinstance(value, list),
            "string": isinstance(value, str),
            "boolean": isinstance(value, bool),
            "integer": isinstance(value, int) and not isinstance(value, bool),
            "number": isinstance(value, (int, float)) and not isinstance(value, bool),
        }.get(expected, False)


def require_exact_keys(value: Any, keys: set[str], path: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        fail(path, "expected object")
    actual = set(value)
    if actual != keys:
        fail(path, f"expected keys {sorted(keys)}, got {sorted(actual)}")
    return value


def require_text(value: Any, path: str) -> str:
    if not isinstance(value, str) or not value.strip():
        fail(path, "expected non-empty string")
    return value


def parse_game_time(value: Any, path: str) -> int:
    text = require_text(value, path)
    match = re.fullmatch(r"([0-9]{1,3}):([0-5][0-9])", text)
    if match is None:
        fail(path, "expected M:SS game time")
    return int(match.group(1)) * 60 + int(match.group(2))


def unique_text(items: Any, path: str) -> list[str]:
    if not isinstance(items, list):
        fail(path, "expected string array")
    result: list[str] = []
    seen: set[str] = set()
    for index, item in enumerate(items):
        text = require_text(item, f"{path}[{index}]")
        if text not in seen:
            seen.add(text)
            result.append(text)
    return result


def assemble_pack(manifest: Any, drafts: Any) -> dict[str, Any]:
    manifest = require_exact_keys(manifest, {"schema_version", "source", "moments"}, "manifest")
    if manifest["schema_version"] != "1.0.0" or not isinstance(manifest["moments"], list):
        fail("manifest", "expected replay manifest schema_version 1.0.0")
    source_by_id: dict[str, dict[str, Any]] = {}
    for index, source in enumerate(manifest["moments"]):
        if not isinstance(source, dict):
            fail(f"manifest.moments[{index}]", "expected object")
        source_id = require_text(source.get("id"), f"manifest.moments[{index}].id")
        if source_id in source_by_id:
            fail("manifest.moments", f"duplicate source moment {source_id!r}")
        source_by_id[source_id] = source

    drafts = require_exact_keys(drafts, {"draft_version", "bundle", "moments"}, "drafts")
    if drafts["draft_version"] != "1.0":
        fail("drafts.draft_version", "expected 1.0")
    bundle = require_exact_keys(drafts["bundle"], {"id", "sha256"}, "drafts.bundle")
    bundle_id = require_text(bundle["id"], "drafts.bundle.id")
    bundle_sha = require_text(bundle["sha256"], "drafts.bundle.sha256")
    if re.fullmatch(r"[a-f0-9]{64}", bundle_sha) is None:
        fail("drafts.bundle.sha256", "expected lowercase SHA-256")
    if not isinstance(drafts["moments"], list) or not 1 <= len(drafts["moments"]) <= 3:
        fail("drafts.moments", "expected one to three project-ready drafts")

    exported: list[dict[str, Any]] = []
    used_sources: set[str] = set()
    for index, draft_value in enumerate(drafts["moments"]):
        path = f"drafts.moments[{index}]"
        draft = require_exact_keys(
            draft_value,
            {"source_moment_id", "game", "coordinate_method", "fixture"},
            path,
        )
        source_id = require_text(draft["source_moment_id"], f"{path}.source_moment_id")
        if source_id in used_sources:
            fail(path, f"source moment {source_id!r} is selected more than once")
        used_sources.add(source_id)
        source = source_by_id.get(source_id)
        if source is None:
            fail(path, f"unknown source moment {source_id!r}")
        if source.get("project_ready") is not True:
            fail(path, f"source moment {source_id!r} is not project_ready")
        game = draft["game"]
        if isinstance(game, bool) or not isinstance(game, int) or game < 1:
            fail(f"{path}.game", "expected positive integer")
        coordinate_method = require_text(draft["coordinate_method"], f"{path}.coordinate_method")
        if "approx" not in coordinate_method.lower():
            fail(f"{path}.coordinate_method", "must explicitly contain 'approx'")

        fixture = draft["fixture"]
        if not isinstance(fixture, dict):
            fail(f"{path}.fixture", "expected object")
        forbidden = sorted({"startTimeSeconds", "replayEvidence"}.intersection(fixture))
        if forbidden:
            fail(f"{path}.fixture", f"derived keys are forbidden: {forbidden}")
        allowed_fixture_keys = set(MOMENT_KEY_ORDER) - {"startTimeSeconds", "replayEvidence"}
        unknown_fixture_keys = sorted(set(fixture) - allowed_fixture_keys)
        if unknown_fixture_keys:
            fail(f"{path}.fixture", f"unknown keys: {unknown_fixture_keys}")

        game_time = source.get("game_time")
        vod = source.get("vod")
        assessment = source.get("assessment")
        transcript = source.get("transcript")
        caster = source.get("caster_context")
        evidence = source.get("evidence")
        for name, value in (
            ("game_time", game_time),
            ("vod", vod),
            ("assessment", assessment),
            ("transcript", transcript),
            ("caster_context", caster),
            ("evidence", evidence),
        ):
            if not isinstance(value, dict):
                fail(f"manifest moment {source_id}.{name}", "expected object")
        decision_time = require_text(game_time.get("decision"), f"manifest moment {source_id}.game_time.decision")
        start_time_seconds = parse_game_time(decision_time, f"manifest moment {source_id}.game_time.decision")
        source_vod_seconds = vod.get("decision_s")
        if isinstance(source_vod_seconds, bool) or not isinstance(source_vod_seconds, (int, float)) or source_vod_seconds < 0:
            fail(f"manifest moment {source_id}.vod.decision_s", "expected non-negative number")
        judgment = require_text(assessment.get("label"), f"manifest moment {source_id}.assessment.label")
        assessment_text = require_text(assessment.get("summary"), f"manifest moment {source_id}.assessment.summary")
        correction = require_text(assessment.get("better_alternative"), f"manifest moment {source_id}.assessment.better_alternative")
        caption_ids = unique_text(transcript.get("caption_ids"), f"manifest moment {source_id}.transcript.caption_ids")
        caption_ids += [
            item
            for item in unique_text(caster.get("caption_ids"), f"manifest moment {source_id}.caster_context.caption_ids")
            if item not in caption_ids
        ]
        external_ids = unique_text(evidence.get("external_event_ids"), f"manifest moment {source_id}.evidence.external_event_ids")
        if not caption_ids or not external_ids:
            fail(path, "project export requires caption and external evidence IDs")

        replay_evidence = {
            "bundleId": bundle_id,
            "bundleSha256": bundle_sha,
            "sourceMomentId": source_id,
            "game": game,
            "decisionTime": decision_time,
            "sourceVodSeconds": source_vod_seconds,
            "judgment": judgment,
            "assessment": assessment_text,
            "coachingCorrection": correction,
            "captionEvidence": caption_ids,
            "externalEvidence": external_ids,
            "coordinateMethod": coordinate_method,
        }
        merged = dict(fixture)
        merged["startTimeSeconds"] = start_time_seconds
        merged["replayEvidence"] = replay_evidence
        exported.append({key: merged[key] for key in MOMENT_KEY_ORDER if key in merged})

    return {"version": "3.0", "moments": exported}


def validate_action(action: dict[str, Any], path: str) -> None:
    action_type = action["type"]
    if action_type not in ACTION_TYPES:
        fail(path, f"unknown tactical action {action_type!r}")
    if action_type == "move" and "target" not in action:
        fail(path, "move requires a target")
    if action_type != "move" and "target" in action:
        fail(path, f"{action_type} cannot include a target")


def validate_semantics(pack: dict[str, Any]) -> None:
    ids: set[str] = set()
    slugs: set[str] = set()
    for moment_index, moment in enumerate(pack["moments"]):
        path = f"$.moments[{moment_index}]"
        if moment["id"] in ids or moment["slug"] in slugs:
            fail(path, "moment IDs and slugs must be unique across the pack")
        ids.add(moment["id"])
        slugs.add(moment["slug"])
        replay_evidence = moment["replayEvidence"]
        for key in (
            "bundleId",
            "sourceMomentId",
            "decisionTime",
            "judgment",
            "assessment",
            "coachingCorrection",
            "coordinateMethod",
        ):
            if not replay_evidence[key].strip():
                fail(f"{path}.replayEvidence.{key}", "must contain non-whitespace text")
        if "approx" not in replay_evidence["coordinateMethod"].lower():
            fail(f"{path}.replayEvidence.coordinateMethod", "must explicitly contain 'approx'")

        team_counts = {"blue": 0, "red": 0}
        marksmen = {"blue": 0, "red": 0}
        unit_ids: set[str] = set()
        unit_teams: dict[str, str] = {}
        controlled_found = False
        for unit_index, unit in enumerate(moment["units"]):
            unit_path = f"{path}.units[{unit_index}]"
            if unit["id"] in unit_ids:
                fail(unit_path, f"duplicate unit ID {unit['id']!r}")
            unit_ids.add(unit["id"])
            unit_teams[unit["id"]] = unit["team"]
            team_counts[unit["team"]] += 1
            if unit["class"] == "marksman":
                marksmen[unit["team"]] += 1
            profile = CLASS_PROFILES[unit["class"]]
            for key, expected in profile.items():
                if unit[key] != expected:
                    fail(unit_path, f"{key} must equal the {unit['class']} profile value {expected}")
            if unit["moveSpeed"] != profile["moveRange"]:
                fail(unit_path, "moveSpeed must equal the class moveRange")
            if unit["hp"] > unit["maxHp"] or unit["alive"] != (unit["hp"] > 0):
                fail(unit_path, "hp and alive state are inconsistent")
            controlled_found = controlled_found or (
                unit["id"] == moment["controlledUnitId"]
                and unit["team"] == "blue"
                and unit["policy"] == "controlled"
                and unit["alive"]
            )
        if team_counts != {"blue": 5, "red": 5}:
            fail(path, f"expected a complete 5v5 roster, got {team_counts}")
        if marksmen != {"blue": 1, "red": 1}:
            fail(path, f"expected exactly one marksman per team, got {marksmen}")
        if not controlled_found:
            fail(path, "controlledUnitId must identify a live blue controlled unit")

        rules = moment["rules"]
        victory = rules["victory"]
        target_id = victory.get("targetUnitId")
        if target_id is not None and unit_teams.get(target_id) != "red":
            fail(f"{path}.rules.victory.targetUnitId", "target must identify a red unit")
        if victory["kind"] == "secure-objective" and "objective" not in rules:
            fail(f"{path}.rules", "secure-objective requires objective rules")
        if victory["kind"] == "eliminate-target" and target_id is None:
            fail(f"{path}.rules.victory", "eliminate-target requires targetUnitId")
        if victory["allowEscape"] and (
            victory["safeRadius"] <= 0 or victory["escapeTurns"] < 1
        ):
            fail(f"{path}.rules.victory", "escape rules are incomplete")

        scenario_elements: set[str] = set()
        objective = rules.get("objective")
        if objective is not None:
            scenario_elements.add(objective["id"])
        river_found = False
        for terrain_index, terrain in enumerate(rules["terrain"]):
            terrain_path = f"{path}.rules.terrain[{terrain_index}]"
            scenario_elements.add(terrain["id"])
            expected = CANONICAL_TERRAIN.get(terrain["id"])
            if expected is not None and terrain["position"] != expected:
                fail(terrain_path, f"canonical position must be {expected}")
            if expected is not None and terrain["kind"] == "safe-zone" and (
                not victory["allowEscape"] or victory["safeZone"] != expected
            ):
                fail(terrain_path, "safe-zone terrain must match enabled escape rules")
            river_found = river_found or terrain["id"] == "river"
        if objective is not None and objective["id"] == "river-core" and not river_found:
            fail(f"{path}.rules.objective", "river-core requires canonical river terrain")

        covered_mechanics: set[str] = set()
        briefing = moment.get("mechanicBriefing")
        if briefing is not None:
            for mechanic in briefing["mechanics"]:
                element_id = mechanic["elementId"]
                if element_id not in scenario_elements or element_id in covered_mechanics:
                    fail(f"{path}.mechanicBriefing", f"invalid or repeated element {element_id!r}")
                for key in ("elementId", "name", "description", "roleInScenario"):
                    if not mechanic[key].strip():
                        fail(f"{path}.mechanicBriefing.{key}", "must contain non-whitespace text")
                covered_mechanics.add(element_id)
        missing_briefings = sorted(SCENARIO_SPECIFIC_MECHANICS.intersection(scenario_elements) - covered_mechanics)
        if missing_briefings:
            fail(path, f"missing mechanic briefing for {missing_briefings}")

        max_turns = moment["maxTurns"]
        if len(rules["referencePlan"]) != max_turns or len(rules["referenceReasons"]) != max_turns:
            fail(f"{path}.rules", f"reference plan and reasons must cover exactly {max_turns} turns")
        for action_index, action in enumerate(rules["referencePlan"]):
            validate_action(action, f"{path}.rules.referencePlan[{action_index}]")
        if any(not reason.strip() for reason in rules["referenceReasons"]):
            fail(f"{path}.rules.referenceReasons", "reasons must contain non-whitespace text")
        for action_type in ACTION_TYPES:
            default = rules["actionDefaults"][action_type]
            if default["type"] != action_type:
                fail(f"{path}.rules.actionDefaults.{action_type}", "default type must match its key")
            validate_action(default, f"{path}.rules.actionDefaults.{action_type}")
            continuation = rules["referenceContinuations"][action_type]
            if len(continuation) != max_turns - 1:
                fail(
                    f"{path}.rules.referenceContinuations.{action_type}",
                    f"expected {max_turns - 1} actions",
                )
            for action_index, action in enumerate(continuation):
                validate_action(action, f"{path}.rules.referenceContinuations.{action_type}[{action_index}]")

        authoring = moment["authoring"]
        if not authoring["analystRationale"].strip():
            fail(f"{path}.authoring.analystRationale", "must contain non-whitespace text")
        if any(not tradeoff.strip() for tradeoff in authoring["intendedTradeoffs"]):
            fail(f"{path}.authoring.intendedTradeoffs", "tradeoffs must contain non-whitespace text")
        alternative_types: set[str] = set()
        for alternative_index, alternative in enumerate(authoring["plausibleAlternatives"]):
            alternative_path = f"{path}.authoring.plausibleAlternatives[{alternative_index}].action"
            validate_action(alternative["action"], alternative_path)
            if not alternative["when"].strip() or not alternative["tradeoff"].strip():
                fail(alternative_path, "when and tradeoff must contain non-whitespace text")
            action_type = alternative["action"]["type"]
            if action_type in alternative_types:
                fail(alternative_path, f"repeated alternative action {action_type!r}")
            alternative_types.add(action_type)
        test_names: set[str] = set()
        expected_statuses: set[str] = set()
        for test_index, test in enumerate(authoring["acceptanceTests"]):
            test_path = f"{path}.authoring.acceptanceTests[{test_index}]"
            if not test["name"].strip() or not test["expectedOutcomeContains"].strip():
                fail(test_path, "name and expectedOutcomeContains must contain non-whitespace text")
            if test["name"] in test_names:
                fail(test_path, f"duplicate acceptance-test name {test['name']!r}")
            test_names.add(test["name"])
            expected_statuses.add(test["expectedStatus"])
            if test["expectedTerminalTurn"] > 0 and len(test["actions"]) != test["expectedTerminalTurn"]:
                fail(test_path, "actions length must equal non-zero expectedTerminalTurn")
            for action_index, action in enumerate(test["actions"]):
                validate_action(action, f"{test_path}.actions[{action_index}]")
            if any(turn > len(test["actions"]) for turn in test["dodgeBeforeTurns"]):
                fail(test_path, "Dodge turn exceeds the acceptance action sequence")
        if expected_statuses != {"won", "lost"}:
            fail(f"{path}.authoring.acceptanceTests", "tests must cover both won and lost")


def validate_pack(pack: Any, schema_path: Path) -> str:
    schema = load_json(schema_path)
    if not isinstance(schema, dict):
        fail("schema", "root must be an object")
    SchemaValidator(schema).validate(pack)
    validate_semantics(pack)
    return sha256(schema_path)


def run_project_validation(pack_path: Path, project_root: Path) -> str:
    backend = project_root.resolve() / "backend"
    validator = backend / "cmd" / "validate-fixtures"
    if not validator.is_dir():
        raise ValidationError(f"Playable Replays validator not found: {validator}")
    go = shutil.which("go")
    if go is None:
        raise ValidationError("Go is required for executable project validation")
    completed = subprocess.run(
        [go, "run", "./cmd/validate-fixtures", "-path", str(pack_path.resolve())],
        cwd=backend,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    if completed.returncode:
        raise ValidationError(f"project fixture validation failed:\n{completed.stdout.strip()}")
    return completed.stdout.strip()


def write_atomic(path: Path, pack: dict[str, Any], overwrite: bool, project_root: Path | None) -> str | None:
    if path.exists() and not overwrite:
        raise ValidationError(f"output exists; pass --overwrite to replace it: {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    handle = tempfile.NamedTemporaryFile(
        mode="w",
        encoding="utf-8",
        prefix=f".{path.name}.",
        suffix=".partial",
        dir=path.parent,
        delete=False,
    )
    temporary = Path(handle.name)
    try:
        with handle:
            json.dump(pack, handle, indent=2, ensure_ascii=False)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        project_result = run_project_validation(temporary, project_root) if project_root else None
        os.replace(temporary, path)
        return project_result
    except Exception:
        temporary.unlink(missing_ok=True)
        raise


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    export = subparsers.add_parser("export", help="assemble a v3.0 fixture pack from replay evidence and authored drafts")
    export.add_argument("--manifest", type=Path, required=True, help="analysis/moments.json from the replay bundle")
    export.add_argument("--drafts", type=Path, required=True, help="Playable Replays authoring drafts")
    export.add_argument("--schema", type=Path, help="canonical moment.schema.json; defaults to the project checkout or bundled snapshot")
    export.add_argument("--output", type=Path, required=True, help="output moments.json")
    export.add_argument("--project-root", type=Path, help="run the project's Go fixture and acceptance validator")
    export.add_argument("--overwrite", action="store_true", help="replace an existing output after all checks pass")

    validate = subparsers.add_parser("validate", help="validate an already assembled v3.0 fixture pack")
    validate.add_argument("--input", type=Path, required=True, help="fixture pack to validate")
    validate.add_argument("--schema", type=Path, help="canonical moment.schema.json; defaults to the project checkout or bundled snapshot")
    validate.add_argument("--project-root", type=Path, help="run the project's Go fixture and acceptance validator")
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        schema_path = args.schema
        if schema_path is None and args.project_root is not None:
            schema_path = args.project_root / "contracts" / "moment.schema.json"
        if schema_path is None:
            schema_path = DEFAULT_SCHEMA
        if args.command == "export":
            manifest = load_json(args.manifest)
            drafts = load_json(args.drafts)
            pack = assemble_pack(manifest, drafts)
            schema_hash = validate_pack(pack, schema_path)
            project_result = write_atomic(args.output, pack, args.overwrite, args.project_root)
            output = args.output.resolve()
        else:
            pack = load_json(args.input)
            schema_hash = validate_pack(pack, schema_path)
            project_result = run_project_validation(args.input, args.project_root) if args.project_root else None
            output = args.input.resolve()
        print(
            json.dumps(
                {
                    "status": "succeeded",
                    "output": str(output),
                    "moments": len(pack["moments"]),
                    "schemaSha256": schema_hash,
                    "schemaValidation": "passed",
                    "semanticValidation": "passed",
                    "projectValidation": "passed" if project_result is not None else "not-requested",
                },
                indent=2,
            )
        )
        return 0
    except ValidationError as exc:
        raise SystemExit(f"error: {exc}") from exc


if __name__ == "__main__":
    raise SystemExit(main())
