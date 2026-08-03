import json
import tempfile
import unittest
from pathlib import Path

from ml.telemetry import (
    TelemetryFrame,
    TelemetryUnit,
    extract_signals,
    load_frames,
    select_pivotal_windows,
    telemetry_windows,
    validate_frames,
)


def unit(
    unit_id: str,
    team: str,
    *,
    x: float,
    y: float,
    hp: float = 100,
    gold: float = 1000,
) -> TelemetryUnit:
    return TelemetryUnit(unit_id, team, x, y, hp, 100, gold, hp > 0)


def frame(
    second: int,
    probability: float,
    *,
    blue_x: float = 50,
    red_x: float = 50,
    blue_hp: float = 100,
    red_hp: float = 100,
    blue_gold: float = 1000,
    red_gold: float = 1000,
    events: tuple[str, ...] = (),
) -> TelemetryFrame:
    return TelemetryFrame(
        second,
        probability,
        (
            unit(
                "blue-carry",
                "blue",
                x=blue_x,
                y=50,
                hp=blue_hp,
                gold=blue_gold,
            ),
            unit(
                "red-carry",
                "red",
                x=red_x,
                y=50,
                hp=red_hp,
                gold=red_gold,
            ),
        ),
        events,
    )


class TelemetryTests(unittest.TestCase):
    def test_extracts_all_four_normalized_signals(self):
        frames = (
            frame(0, 0.4, blue_x=0, red_x=100),
            frame(
                10,
                0.8,
                blue_hp=50,
                red_hp=100,
                blue_gold=500,
                red_gold=1500,
                events=("damage",) * 10,
            ),
        )

        signals = extract_signals(frames)

        self.assertAlmostEqual(signals.win_probability_swing, 0.4)
        self.assertAlmostEqual(signals.event_density, 0.5)
        self.assertAlmostEqual(signals.entity_proximity, 0.0)
        self.assertAlmostEqual(signals.resource_asymmetry, 0.5)

    def test_rejects_out_of_order_frames_and_roster_drift(self):
        with self.assertRaisesRegex(ValueError, "strictly increasing"):
            validate_frames((frame(1, 0.5), frame(1, 0.6)))

        changed_team = TelemetryFrame(
            2,
            0.6,
            (
                unit("blue-carry", "red", x=50, y=50),
                unit("red-carry", "red", x=50, y=50),
            ),
        )
        with self.assertRaisesRegex(ValueError, "stable across frames"):
            validate_frames((frame(1, 0.5), changed_team))

    def test_irregular_windows_are_fully_covered(self):
        frames = tuple(
            frame(second, 0.5)
            for second in (0, 3, 7, 13, 15, 21)
        )

        windows = telemetry_windows(frames, window_seconds=12, stride_seconds=4)

        self.assertEqual([(start, end) for start, end, _ in windows], [(0, 13), (7, 21)])

    def test_overlapping_pivotal_windows_are_suppressed(self):
        frames = tuple(
            frame(
                second,
                0.9 if second == 12 else 0.1,
                events=("damage",),
            )
            for second in range(25)
        )

        raw = telemetry_windows(frames, window_seconds=12, stride_seconds=2)
        candidates = select_pivotal_windows(
            frames,
            threshold=0.6,
            window_seconds=12,
            stride_seconds=2,
            max_overlap_fraction=0.49,
        )

        self.assertGreater(len(raw), len(candidates))
        self.assertEqual(
            [(candidate.start_second, candidate.end_second) for candidate in candidates],
            [(0, 12), (8, 20)],
        )
        self.assertIn("win-probability-swing", candidates[0].reason_tags)

    def test_load_frames_enforces_version_and_unknown_fields(self):
        payload = {
            "version": "1.0",
            "frames": [
                {
                    "second": second,
                    "winProbability": 0.5,
                    "events": ["damage"],
                    "units": [
                        {
                            "id": "blue-carry",
                            "team": "blue",
                            "position": {"x": 40, "y": 50},
                            "hp": 100,
                            "maxHp": 100,
                            "gold": 1000,
                        },
                        {
                            "id": "red-carry",
                            "team": "red",
                            "position": {"x": 60, "y": 50},
                            "hp": 100,
                            "maxHp": 100,
                            "gold": 1000,
                        },
                    ],
                }
                for second in (0, 12)
            ],
        }
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telemetry.json"
            path.write_text(json.dumps(payload), encoding="utf-8")
            loaded = load_frames(path)
            self.assertEqual([item.second for item in loaded], [0, 12])

            payload["frames"][0]["unexpected"] = True
            path.write_text(json.dumps(payload), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "unknown fields"):
                load_frames(path)

    def test_rejects_unknown_event_types(self):
        with self.assertRaisesRegex(ValueError, "unknown event types"):
            validate_frames(
                (
                    frame(0, 0.5, events=("chat",)),
                    frame(1, 0.5),
                )
            )

    def test_selection_parameters_are_bounded(self):
        frames = (frame(0, 0.5), frame(12, 0.5))
        with self.assertRaisesRegex(ValueError, "threshold"):
            select_pivotal_windows(frames, threshold=1.1)
        with self.assertRaisesRegex(ValueError, "max_overlap_fraction"):
            select_pivotal_windows(frames, max_overlap_fraction=-0.1)


if __name__ == "__main__":
    unittest.main()
