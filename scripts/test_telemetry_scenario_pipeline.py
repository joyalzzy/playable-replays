"""Exercise the detector-to-preview workflow across the Python and Go boundaries."""

from __future__ import annotations

import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[1]
BACKEND = ROOT / "backend"
FIXTURES = ROOT / "fixtures" / "moments.json"
GO = os.environ.get("GO_BINARY") or shutil.which("go")


def run(
    *command: str, cwd: Path = ROOT, expect_success: bool = True
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        command,
        cwd=cwd,
        check=False,
        text=True,
        capture_output=True,
        env=os.environ.copy(),
    )
    if expect_success and result.returncode != 0:
        raise AssertionError(
            f"command failed ({result.returncode}): {' '.join(command)}\n"
            f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
        )
    if not expect_success and result.returncode == 0:
        raise AssertionError(f"command unexpectedly succeeded: {' '.join(command)}")
    return result


def unit(unit_id: str, team: str, x: float, gold: int) -> dict[str, object]:
    return {
        "id": unit_id,
        "team": team,
        "position": {"x": x, "y": 50},
        "hp": 100,
        "maxHp": 100,
        "gold": gold,
        "alive": True,
    }


def synthetic_telemetry() -> dict[str, object]:
    frames = []
    for second in range(13):
        probability = 0.2 if second == 6 else 0.85 if second == 12 else 0.75
        frames.append(
            {
                "second": second,
                "winProbability": probability,
                "events": ["damage", "kill"],
                "units": [
                    unit("blue-carry", "blue", 50, 9000),
                    unit("blue-support", "blue", 90, 9000),
                    unit("red-top", "red", 44, 9300),
                    unit("red-jungle", "red", 56, 9300),
                ],
            }
        )
    return {"version": "1.0", "frames": frames}


def complete_draft(bundle: dict[str, object], base_pack: dict[str, object]) -> None:
    drafts = bundle["drafts"]
    assert isinstance(drafts, list) and len(drafts) == 1
    draft = drafts[0]
    assert isinstance(draft, dict)
    scenario = draft["scenario"]
    assert isinstance(scenario, dict)
    authoring = scenario["authoring"]
    assert isinstance(authoring, dict)
    category = authoring["category"]

    moments = base_pack["moments"]
    assert isinstance(moments, list)
    template = next(
        moment
        for moment in moments
        if isinstance(moment, dict)
        and isinstance(moment.get("authoring"), dict)
        and moment["authoring"].get("category") == category
    )

    preserved = {
        key: copy.deepcopy(scenario[key])
        for key in (
            "id",
            "slug",
            "startTimeSeconds",
            "reasonTags",
            "signals",
            "sourceDetection",
        )
    }
    completed = copy.deepcopy(template)
    completed.update(preserved)
    completed["title"] = f"Telemetry draft: {template['title']}"
    completed["description"] = (
        "Synthetic analyst completion used to verify the local detector-to-preview workflow. "
        + str(template["description"])
    )
    completed["authoring"]["category"] = category
    draft["scenario"] = completed


def main() -> None:
    if GO is None:
        raise SystemExit("Go must be available on PATH for the telemetry scenario pipeline test")

    with tempfile.TemporaryDirectory(prefix="playable-replays-pipeline-") as directory:
        temporary = Path(directory)
        telemetry_path = temporary / "telemetry.json"
        detections_path = temporary / "detections.ndjson"
        draft_path = temporary / "drafts.json"
        preview_path = temporary / "preview.json"

        telemetry_path.write_text(json.dumps(synthetic_telemetry()), encoding="utf-8")
        detector = run(
            sys.executable,
            "-m",
            "ml.telemetry",
            str(telemetry_path),
            "--window-seconds",
            "12",
        )
        records = [json.loads(line) for line in detector.stdout.splitlines() if line.strip()]
        assert len(records) == 1, records
        detection = records[0]
        assert detection["schemaVersion"] == "1.0"
        assert detection["startSecond"] == 0 and detection["endSecond"] == 12
        assert "team-fight-reversal" in detection["reasonTags"]
        assert detection["semanticEvidence"]["teamFightReversalSecond"] == 6
        detections_path.write_text(detector.stdout, encoding="utf-8")

        run(
            GO,
            "run",
            "./cmd/scenario-draft",
            "create",
            "--input",
            str(detections_path),
            "--output",
            str(draft_path),
            cwd=BACKEND,
        )
        bundle = json.loads(draft_path.read_text(encoding="utf-8"))
        scenario = bundle["drafts"][0]["scenario"]
        assert bundle["version"] == "2.1"
        assert scenario["sourceDetection"] == detection
        assert scenario["signals"] == detection["signals"]
        assert scenario["startTimeSeconds"] == detection["startSecond"]
        assert scenario["authoring"]["analystRationale"] == ""

        blocked = run(
            GO,
            "run",
            "./cmd/scenario-draft",
            "publish",
            "--draft",
            str(draft_path),
            "--base",
            str(FIXTURES),
            "--output",
            str(preview_path),
            cwd=BACKEND,
            expect_success=False,
        )
        for expected in (
            "analyst rationale is incomplete",
            "intended tradeoffs are incomplete",
            "plausible alternatives are incomplete",
            "acceptance tests are incomplete",
        ):
            assert expected in blocked.stderr, blocked.stderr
        assert not preview_path.exists()

        base_pack = json.loads(FIXTURES.read_text(encoding="utf-8"))
        complete_draft(bundle, base_pack)
        draft_path.write_text(json.dumps(bundle, indent=2) + "\n", encoding="utf-8")

        run(
            GO,
            "run",
            "./cmd/scenario-draft",
            "validate",
            "--draft",
            str(draft_path),
            cwd=BACKEND,
        )
        preview = run(
            GO,
            "run",
            "./cmd/scenario-draft",
            "preview",
            "--draft",
            str(draft_path),
            "--base",
            str(FIXTURES),
            "--output",
            str(preview_path),
            cwd=BACKEND,
        )
        assert "Open http://127.0.0.1:5173/?moment=" in preview.stdout

        preview_pack = json.loads(preview_path.read_text(encoding="utf-8"))
        assert len(preview_pack["moments"]) == len(base_pack["moments"]) + 1
        published = preview_pack["moments"][-1]
        assert published["sourceDetection"] == detection
        assert published["signals"] == detection["signals"]
        assert published["authoring"]["analystRationale"]
        print("Telemetry detector-to-preview pipeline passed.")


if __name__ == "__main__":
    main()
